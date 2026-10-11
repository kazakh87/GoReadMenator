package resolve_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser/golden"
	"goreadmenator/internal/parsers"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
	"goreadmenator/internal/update"
)

// TestGoldenResolve indiziert jedes Verzeichnis in testdata/ als eigenes Repository, lässt den Resolver
// laufen und vergleicht die aufgelösten Verwendungen, Beziehungen und Imports mit expected.golden.txt.
// Die Szenarien stehen in scenario.txt, jeder Fall hat eine Zeile pro Ergebnis.
func TestGoldenResolve(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	dirs, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Fatal(err)
	}
	var cases []golden.Case
	for _, dir := range dirs {
		fi, err := os.Stat(dir)
		if err != nil || !fi.IsDir() {
			continue
		}
		c, err := golden.ReadScenarioFile(dir)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		cases = append(cases, c)
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			st := indexRepo(t, dir)
			got := dumpResolution(t, st)
			golden.Check(t, got, filepath.Join(dir, "expected.golden.txt"))
		})
	}
	if golden.Update() {
		if err := golden.WriteCoverageDirs("testdata", "Auflösung", cases); err != nil {
			t.Fatal(err)
		}
	}
}

// indexRepo kopiert den Fall in ein temporäres Git-Repository, committet und indiziert es.
func indexRepo(t *testing.T, src string) store.Store {
	t.Helper()
	repo := t.TempDir()
	if err := golden.CopyTree(src, repo); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@e.x", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "fall")
	st := memory.New()
	reg, err := parsers.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := update.Run(context.Background(), st, update.Options{RepoDir: repo, Registry: reg, Workers: 2, BatchSize: 100, MaxFileSizeBytes: 5 << 20}); err != nil {
		t.Fatal(err)
	}
	return st
}

// dumpResolution erzeugt eine sortierte, stabile Textform der Auflösung.
// Verwendungen erscheinen mit Datei, Zeile, Ziel und Konfidenz. Unaufgelöste Verwendungen stehen ebenfalls drin,
// damit man sieht, was bewusst offen bleibt.
func dumpResolution(t *testing.T, st store.Store) string {
	t.Helper()
	ctx := context.Background()
	files, _ := st.Files(ctx)
	path := map[model.FileID]string{}
	for _, f := range files {
		path[f.ID] = f.Path
	}
	var lines []string
	refs, _ := st.AllReferences(ctx)
	for _, r := range refs {
		target := r.TargetID
		if target == "" {
			target = "-"
		}
		lines = append(lines, fmt.Sprintf("ref %s:%d %s %s -> %s [%s] in %s",
			path[r.FileID], r.Span.StartLine, r.Kind, r.TargetName, target, r.Confidence, r.Enclosing))
	}
	rels, _ := st.AllRelations(ctx)
	for _, r := range rels {
		lines = append(lines, fmt.Sprintf("rel %s %s -> %s", r.Kind, r.From, r.To))
	}
	imps, _ := st.AllImports(ctx)
	for _, im := range imps {
		target := "extern"
		if im.ResolvedID != 0 {
			target = path[im.ResolvedID]
		}
		lines = append(lines, fmt.Sprintf("imp %s %s %q -> %s", path[im.FileID], im.Kind, im.Raw, target))
	}
	sort.Strings(lines)
	header := []string{"# Verwendungen, Beziehungen und Imports (sortiert)"}
	return strings.Join(append(header, lines...), "\n") + "\n"
}

var _ store.Store = (*memory.Store)(nil)
