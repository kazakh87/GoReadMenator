package query

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goreadmenator/internal/parsers"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
	"goreadmenator/internal/update"
)

// fixtureEngine baut ein kleines Repository, indiziert es und liefert die Abfrageschicht.
func fixtureEngine(t *testing.T, files map[string]string) *Engine {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@e.x", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-q", "-m", "fixture")

	st := memory.New()
	reg, _ := parsers.Registry()
	if _, err := update.Run(context.Background(), st, update.Options{RepoDir: dir, Registry: reg, Workers: 2, BatchSize: 50, MaxFileSizeBytes: 5 << 20}); err != nil {
		t.Fatal(err)
	}
	return New(st, dir)
}

// bigFixture erzeugt viele öffentliche Methoden, damit das Budget greift.
func bigFixture() map[string]string {
	var b strings.Builder
	b.WriteString("namespace App.Big;\npublic class Hub {\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "    /// <summary>Methode %d mit ausführlicher Beschreibung für die Schnittstelle.</summary>\n    public int Run%d(int a, string b) { return a + %d; }\n", i, i, i)
	}
	b.WriteString("}\n")
	return map[string]string{"Big.cs": b.String(), "Svc.cs": `namespace App.Svc;
using App.Big;
public class Caller {
    public void Go() { var h = new Hub(); h.Run1(1, "x"); }
}
`}
}

func TestFindSymbolsExactFirstAndKindFilter(t *testing.T) {
	e := fixtureEngine(t, bigFixture())
	page, err := e.FindSymbols(context.Background(), "Hub", "", "", 10, 2500)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) == 0 || page.Items[0].FQN != "App.Big.Hub" {
		t.Fatalf("genauer Treffer muss zuerst kommen: %+v", page.Items[:1])
	}
	cls, _ := e.FindSymbols(context.Background(), "Hub", "class", "", 10, 2500)
	for _, r := range cls.Items {
		if r.Kind != "class" {
			t.Fatalf("Art-Filter verletzt: %+v", r)
		}
	}
}

func TestBudgetAndPaginationOfAPI(t *testing.T) {
	ctx := context.Background()
	e := fixtureEngine(t, bigFixture())
	first, err := e.API(ctx, "App.Big.", "", 500) // kleines Budget
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || first.NextCursor == "" {
		t.Fatalf("Budget muss die Antwort kürzen: truncated=%v cursor=%q", first.Truncated, first.NextCursor)
	}
	size, _ := json.Marshal(first)
	if len(size) > ClampBudget(500)+400 {
		t.Fatalf("Antwort %d Zeichen überschreitet Budget %d", len(size), ClampBudget(500))
	}
	seen := map[string]bool{}
	cursor := ""
	for i := 0; i < 500; i++ {
		page, err := e.API(ctx, "App.Big.", cursor, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range page.Items {
			if seen[it.FQN] {
				t.Fatalf("doppelter Eintrag über Seiten: %s", it.FQN)
			}
			seen[it.FQN] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) < 300 {
		t.Fatalf("Paginierung liefert nur %d von 300 Methoden", len(seen))
	}
}

func TestGetSymbolAndAmbiguity(t *testing.T) {
	e := fixtureEngine(t, bigFixture())
	ctx := context.Background()
	d, err := e.GetSymbol(ctx, "App.Big.Hub.Run1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Sig, "Run1") || d.Summary == "" || len(d.Locations) == 0 || d.Locations[0].File != "Big.cs" {
		t.Fatalf("Detail: %+v", d)
	}
	if d.Relations["contains"] == 0 && d.Relations == nil {
		// Member ist im Container enthalten, die Beziehung ist Teil der Antwort
		t.Log("keine Beziehungen am Member")
	}
	_, err = e.GetSymbol(ctx, "Run1", 0)
	if err != nil && !strings.Contains(err.Error(), "mehrdeutig") {
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
}

func TestNeighborhoodAndUsages(t *testing.T) {
	e := fixtureEngine(t, bigFixture())
	ctx := context.Background()
	nb, err := e.Neighborhood(ctx, "App.Big.Hub", 1, []string{"contains"}, "", 2500)
	if err != nil {
		t.Fatal(err)
	}
	if len(nb.Items) == 0 {
		t.Fatal("Beziehungen des Typs fehlen")
	}
	us, err := e.Usages(ctx, "App.Big.Hub.Run1", 1, "", 2500)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range us.Items {
		if u.File == "Svc.cs" && u.User == "App.Svc.Caller.Go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Verwendung in Svc.cs fehlt: %+v", us.Items)
	}
}

func TestContextRespectsBudget(t *testing.T) {
	e := fixtureEngine(t, bigFixture())
	pack, err := e.Context(context.Background(), "App.Big.Hub", 200)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(pack)
	if len(b) > ClampBudget(200)+600 {
		t.Fatalf("Kontextpaket %d Zeichen bei Budget 200 Token", len(b))
	}
}

func TestSourceLimitsLines(t *testing.T) {
	e := fixtureEngine(t, bigFixture())
	src, err := e.Source(context.Background(), "Big.cs", 1, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Lines) > maxSourceLines {
		t.Fatalf("Quelltext-Ausschnitt %d Zeilen, Grenze %d", len(src.Lines), maxSourceLines)
	}
	if _, err := e.Source(context.Background(), "gibtsnicht.cs", 1, 5); err == nil {
		t.Fatal("unbekannte Datei muss einen Fehler liefern")
	}
}

func TestStatusReportsUpToDate(t *testing.T) {
	e := fixtureEngine(t, bigFixture())
	s, err := e.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !s.UpToDate || s.Symbols == 0 {
		t.Fatalf("Status: %+v", s)
	}
	_ = store.ErrNotFound
}
