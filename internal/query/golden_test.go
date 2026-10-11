package query_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"goreadmenator/internal/parser/golden"
	"goreadmenator/internal/parsers"
	"goreadmenator/internal/query"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
	"goreadmenator/internal/update"
)

// TestGoldenQueries führt die Befehle aus queries.txt gegen den indizierten Fall aus und vergleicht die
// JSON-Antworten mit expected.golden.txt. Jede Zeile ist ein Werkzeugaufruf: Name und JSON-Argumente.
// "$NEXT" in den Argumenten wird durch den nextCursor der vorherigen Antwort ersetzt. So lässt sich die
// Paginierung in einem Durchlauf prüfen.
func TestGoldenQueries(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	dirs, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Fatal(err)
	}
	var cases []golden.Case
	for _, dir := range dirs {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		c, err := golden.ReadScenarioFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			repo, st := indexCase(t, dir)
			out := runQueries(t, query.New(st, repo), filepath.Join(dir, "queries.txt"))
			golden.Check(t, out, filepath.Join(dir, "expected.golden.txt"))
		})
	}
	if golden.Update() {
		if err := golden.WriteCoverageDirs("testdata", "Abfragen", cases); err != nil {
			t.Fatal(err)
		}
	}
}

// indexCase kopiert den Fall in ein Git-Repository, committet und indiziert es.
func indexCase(t *testing.T, src string) (string, store.Store) {
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
	return repo, st
}

// runQueries führt jede Zeile aus und formatiert Anfrage und Antwort.
func runQueries(t *testing.T, e *query.Engine, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	cursor := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, args, _ := strings.Cut(line, " ")
		args = strings.ReplaceAll(args, "$NEXT", cursor)
		fmt.Fprintf(&b, "## %s %s\n", name, args)
		result, err := call(e, name, []byte(args))
		if err != nil {
			fmt.Fprintf(&b, "FEHLER: %v\n\n", err)
			cursor = ""
			continue
		}
		cursor = nextCursor(result)
		out, _ := json.MarshalIndent(result, "", "  ")
		// Commit-Hashes ändern sich mit jedem Lauf (Zeitstempel im Fixture): auf einen Platzhalter normalisieren.
		b.Write(sha.ReplaceAll(out, []byte("<SHA>")))
		b.WriteString("\n\n")
	}
	return b.String()
}

// call bildet den Werkzeugnamen auf die Abfrageschicht ab. Dieselben Namen wie im MCP-Server.
func call(e *query.Engine, name string, raw []byte) (any, error) {
	ctx := context.Background()
	var a struct {
		Q      string   `json:"q"`
		Kind   string   `json:"kind"`
		Ref    string   `json:"ref"`
		Scope  string   `json:"scope"`
		Cursor string   `json:"cursor"`
		Limit  int      `json:"limit"`
		Budget int      `json:"budget_tokens"`
		Depth  int      `json:"depth"`
		Kinds  []string `json:"relation_kinds"`
		File   string   `json:"file"`
		Start  int      `json:"start_line"`
		End    int      `json:"end_line"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, fmt.Errorf("ungültige Argumente: %v", err)
		}
	}
	switch name {
	case "find_symbols":
		return e.FindSymbols(ctx, a.Q, a.Kind, a.Cursor, a.Limit, a.Budget)
	case "get_symbol":
		return e.GetSymbol(ctx, a.Ref, a.Budget)
	case "api":
		return e.API(ctx, a.Scope, a.Cursor, a.Budget)
	case "neighborhood":
		return e.Neighborhood(ctx, a.Ref, a.Depth, a.Kinds, a.Cursor, a.Budget)
	case "usages":
		return e.Usages(ctx, a.Ref, a.Depth, a.Cursor, a.Budget)
	case "context":
		return e.Context(ctx, a.Ref, a.Budget)
	case "source":
		return e.Source(ctx, a.File, a.Start, a.End)
	case "endpoints":
		return e.Endpoints(ctx, a.Cursor, a.Budget)
	case "status":
		return e.Status(ctx)
	}
	return nil, fmt.Errorf("unbekanntes Werkzeug %q", name)
}

var sha = regexp.MustCompile(`[0-9a-f]{40}`)

// nextCursor liest nextCursor aus einer Antwort, falls vorhanden.
func nextCursor(v any) string {
	b, _ := json.Marshal(v)
	var m struct {
		NextCursor string `json:"nextCursor"`
	}
	_ = json.Unmarshal(b, &m)
	return m.NextCursor
}
