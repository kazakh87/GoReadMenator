package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	t    *testing.T
	dir  string
	git  func(args ...string)
	outs []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	dir := t.TempDir()
	f := &fixture{t: t, dir: dir}
	f.git = func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@e.x", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	f.git("init", "-q")
	return f
}

func (f *fixture) write(rel, content string) {
	f.t.Helper()
	p := filepath.Join(f.dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// run führt einen Befehl aus und liefert stdout, stderr und Exit-Code.
func (f *fixture) run(args ...string) (string, string, int) {
	f.t.Helper()
	var out, errb bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errb}
	code := app.Run(context.Background(), append([]string{"-repo", f.dir}, args...))
	return out.String(), errb.String(), code
}

const calcV1 = `namespace App;
public class Calc {
    public int Add(int a, int b) { return a + b; }
    public int Twice(int x) { return Add(x, x); }
}
`

const reportCS = `namespace App;
public class Report {
    public void Print() { var c = new Calc(); c.Twice(3); }
}
`

func TestIndexSymbolRefsImpact(t *testing.T) {
	f := newFixture(t)
	f.write("Calc.cs", calcV1)
	f.write("Report.cs", reportCS)
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "A")

	out, errs, code := f.run("index")
	if code != ExitOK {
		t.Fatalf("index: code %d, err %q", code, errs)
	}
	if !strings.Contains(out, "Vollaufbau") && !strings.Contains(out, "full") {
		t.Fatalf("index-Ausgabe: %q", out)
	}
	if _, err := os.Stat(filepath.Join(f.dir, ".git", "goreadmenator", "index.snap")); err != nil {
		t.Fatalf("Index-Datei im .git-Verzeichnis fehlt: %v", err)
	}

	out, _, code = f.run("symbol", "Add")
	if code != ExitOK || !strings.Contains(out, "csharp:App.Calc.Add(System.Int32,System.Int32)") {
		t.Fatalf("symbol: code %d out %q", code, out)
	}

	out, _, code = f.run("refs", "--id", "csharp:App.Calc.Add(System.Int32,System.Int32)", "Add")
	if code != ExitOK || !strings.Contains(out, "Calc.cs") {
		t.Fatalf("refs: code %d out %q", code, out)
	}

	out, _, code = f.run("-json", "impact", "--id", "csharp:App.Calc.Add(System.Int32,System.Int32)", "Add")
	if code != ExitOK {
		t.Fatalf("impact json: code %d", code)
	}
	var doc struct {
		Hits []struct {
			Depth int    `json:"depth"`
			File  string `json:"file"`
		} `json:"hits"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("JSON ungültig: %v\n%s", err, out)
	}
	found := false
	for _, h := range doc.Hits {
		if h.File == "Report.cs" && h.Depth == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("Kette Add->Twice->Print nicht in Tiefe 2: %+v", doc.Hits)
	}
}

func TestExitCodes(t *testing.T) {
	f := newFixture(t)
	f.write("Calc.cs", calcV1)
	f.write("Other.cs", "namespace Other;\npublic class Calc { }\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "A")
	if _, _, code := f.run("index"); code != ExitOK {
		t.Fatalf("index: %d", code)
	}
	if _, errs, code := f.run("refs", "Calc"); code != ExitUsage || !strings.Contains(errs, "mehrdeutig") {
		t.Fatalf("mehrdeutiger Name muss Exit 2 liefern: code %d err %q", code, errs)
	}
	if _, _, code := f.run("symbol", "GibtEsNicht"); code != ExitError {
		t.Fatalf("unbekanntes Symbol: code %d, want 1", code)
	}
	if _, _, code := f.run("unbekannt"); code != ExitUsage {
		t.Fatalf("unbekannter Befehl: code %d, want 2", code)
	}
	if _, _, code := f.run("symbol"); code != ExitUsage {
		t.Fatalf("fehlendes Argument: code %d, want 2", code)
	}
}

func TestDiffShowsDirectChangesAndImpact(t *testing.T) {
	f := newFixture(t)
	f.write("Calc.cs", calcV1)
	f.write("Report.cs", reportCS)
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "A")
	a := gitRev(t, f.dir)

	f.write("Calc.cs", strings.Replace(calcV1, "return a + b;", "var s = a + b; return s;", 1))
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "B")

	out, errs, code := f.run("diff", a+"..HEAD")
	if code != ExitOK {
		t.Fatalf("diff: code %d err %q", code, errs)
	}
	if !strings.Contains(out, "* method csharp:App.Calc.Add") {
		t.Fatalf("Add muss als direkt betroffen markiert sein:\n%s", out)
	}
	if !strings.Contains(out, "Report.cs") {
		t.Fatalf("Wirkung in Report.cs fehlt:\n%s", out)
	}
}

func TestDiffRejectsWrongTarget(t *testing.T) {
	f := newFixture(t)
	f.write("a.ts", "export function f() { }\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "A")
	f.write("a.ts", "export function f() { return 1; }\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "B")
	a := gitRev(t, f.dir)
	_, errs, code := f.run("diff", a+"..deadbeef")
	if code != ExitUsage || !strings.Contains(errs, "Index steht") {
		t.Fatalf("falsches Ziel: code %d err %q", code, errs)
	}
}

func gitRev(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestImportXMLSurvivesFullIndex(t *testing.T) {
	f := newFixture(t)
	xmlPath := filepath.Join(t.TempDir(), "Shop.Core.xml")
	xmlDoc := "<doc><assembly><name>Shop.Core</name></assembly><members>" +
		`<member name="M:Shop.Api.Ping"><summary>Antwortet.</summary></member></members></doc>`
	if err := os.WriteFile(xmlPath, []byte(xmlDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errs, code := f.run("import-xml", xmlPath)
	if code != ExitOK || !strings.Contains(out, "Shop.Core") {
		t.Fatalf("import-xml: code %d out %q err %q", code, out, errs)
	}
	f.write("Calc.cs", calcV1)
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "A")
	if _, errs, code := f.run("index"); code != ExitOK {
		t.Fatalf("index: %d %s", code, errs)
	}
	out, _, code = f.run("symbol", "Ping")
	if code != ExitOK || !strings.Contains(out, "Antwortet.") || !strings.Contains(out, "bin/Shop.Core.dll") {
		t.Fatalf("XML-Symbol nach Vollaufbau verloren: code %d out %q", code, out)
	}
}
