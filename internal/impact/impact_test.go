package impact

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parsers"
	"goreadmenator/internal/store/memory"
	"goreadmenator/internal/update"
)

func setupRepo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@e.x", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	return dir, git
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rev(t *testing.T, dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out[:40])
}

func indexAt(t *testing.T, dir string, st *memory.Store) {
	t.Helper()
	reg, _ := parsers.Registry()
	if _, err := update.Run(context.Background(), st, update.Options{RepoDir: dir, Registry: reg, Workers: 2, BatchSize: 50, MaxFileSizeBytes: 5 << 20}); err != nil {
		t.Fatal(err)
	}
}

const v1 = `namespace App;
public class Calc {
    public int Add(int a, int b) {
        return a + b;
    }
    public int Twice(int x) {
        return Add(x, x);
    }
}
`

const v2 = `namespace App;
public class Calc {
    public int Add(int a, int b) {
        var sum = a + b;
        return sum;
    }
    public int Twice(int x) {
        return Add(x, x);
    }
}
`

const caller = `namespace App;
public class Report {
    public void Print() {
        var c = new Calc();
        c.Twice(3);
    }
}
`

func TestChangedSymbolsAndImpact(t *testing.T) {
	dir, git := setupRepo(t)
	write(t, dir, "Calc.cs", v1)
	write(t, dir, "Report.cs", caller)
	git("add", "-A")
	git("commit", "-q", "-m", "A")
	a := rev(t, dir)

	write(t, dir, "Calc.cs", v2)
	git("add", "-A")
	git("commit", "-q", "-m", "B")
	b := rev(t, dir)

	st := memory.New()
	indexAt(t, dir, st)
	changed, err := Changed(context.Background(), st, dir, a, b)
	if err != nil {
		t.Fatal(err)
	}
	var direct []string
	for _, c := range changed {
		if c.Direct {
			direct = append(direct, string(c.Symbol.ID))
		}
	}
	if len(direct) != 1 || direct[0] != "csharp:App.Calc.Add(System.Int32,System.Int32)" {
		t.Fatalf("direkt betroffen = %v, want nur Add", direct)
	}
	// Die Klasse enthält die Änderung, ist aber nicht direkt betroffen.
	foundClass := false
	for _, c := range changed {
		if c.Symbol.ID == "csharp:App.Calc" {
			foundClass = true
			if c.Direct {
				t.Fatal("Klasse darf nicht als direkt betroffen gelten")
			}
		}
	}
	if !foundClass {
		t.Fatal("Klasse sollte als Container geführt werden")
	}
}

func TestImpactFollowsCallChain(t *testing.T) {
	dir, git := setupRepo(t)
	write(t, dir, "Calc.cs", v1)
	write(t, dir, "Report.cs", caller)
	git("add", "-A")
	git("commit", "-q", "-m", "A")

	st := memory.New()
	indexAt(t, dir, st)
	ctx := context.Background()

	hits, err := Analyze(ctx, st, "csharp:App.Calc.Add(System.Int32,System.Int32)", 3)
	if err != nil {
		t.Fatal(err)
	}
	// Add wird direkt von Twice verwendet (Tiefe 1), Twice von Print (Tiefe 2).
	var d1, d2 bool
	for _, h := range hits {
		if h.Depth == 1 && h.User == "csharp:App.Calc.Twice(System.Int32)" {
			d1 = true
		}
		if h.Depth == 2 && h.User == "csharp:App.Report.Print()" && h.File == "Report.cs" {
			d2 = true
		}
	}
	if !d1 || !d2 {
		t.Fatalf("Aufrufkette nicht gefunden (d1=%v d2=%v): %+v", d1, d2, hits)
	}

	// Tiefe 1 darf nur die direkte Verwendung liefern.
	shallow, _ := Analyze(ctx, st, "csharp:App.Calc.Add(System.Int32,System.Int32)", 1)
	for _, h := range shallow {
		if h.Depth > 1 {
			t.Fatalf("Tiefe 1 liefert Tiefe %d", h.Depth)
		}
	}
	_ = model.KindMethod
}

func TestDeletionInsideMethodMarksIt(t *testing.T) {
	dir, git := setupRepo(t)
	write(t, dir, "Calc.cs", v1)
	git("add", "-A")
	git("commit", "-q", "-m", "A")
	a := rev(t, dir)
	write(t, dir, "Calc.cs", `namespace App;
public class Calc {
    public int Add(int a, int b) {
        return a + b;
    }
    public int Twice(int x) {
        return x * 2;
    }
}
`)
	git("add", "-A")
	git("commit", "-q", "-m", "B")
	b := rev(t, dir)

	st := memory.New()
	indexAt(t, dir, st)
	changed, _ := Changed(context.Background(), st, dir, a, b)
	found := false
	for _, c := range changed {
		if c.Symbol.ID == "csharp:App.Calc.Twice(System.Int32)" && c.Direct {
			found = true
		}
	}
	if !found {
		t.Fatalf("Twice muss als direkt betroffen gelten: %+v", changed)
	}
}

func TestUnchangedCommitHasNoChanges(t *testing.T) {
	dir, git := setupRepo(t)
	write(t, dir, "Calc.cs", v1)
	git("add", "-A")
	git("commit", "-q", "-m", "A")
	a := rev(t, dir)
	st := memory.New()
	indexAt(t, dir, st)
	changed, err := Changed(context.Background(), st, dir, a, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("ohne Änderung keine Treffer: %+v", changed)
	}
	_ = filepath.Join
}
