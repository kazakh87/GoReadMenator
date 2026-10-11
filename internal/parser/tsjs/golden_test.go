package tsjs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goreadmenator/internal/parser"
	"goreadmenator/internal/parser/golden"
)

// TestGolden parst jede Datei in testdata/ und vergleicht das Ergebnis mit der zugehörigen .golden.txt.
// Jede Eingabedatei beschreibt ihr Szenario in den ersten Kommentarzeilen (SZENARIO und ABDECKUNG).
// Der Pfad der Datei ist Teil der Symbol-IDs, deshalb heißt die Datei im Test wie im Repository.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Fatal(err)
	}
	var cases []golden.Case
	for _, f := range files {
		ext := filepath.Ext(f)
		if ext != ".ts" && ext != ".tsx" && ext != ".js" {
			continue
		}
		f := f
		c, err := golden.ReadCase(f)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
		t.Run(strings.TrimSuffix(filepath.Base(f), ext), func(t *testing.T) {
			content, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			repoPath := "src/" + filepath.Base(f)
			res, err := New().Parse(context.Background(), parser.Source{Path: repoPath, Content: content})
			if err != nil {
				t.Fatal(err)
			}
			golden.Check(t, golden.Dump(res), strings.TrimSuffix(f, ext)+".golden.txt")
		})
	}
	if len(cases) == 0 {
		t.Fatal("keine Testfälle in testdata/")
	}
	if golden.Update() {
		if err := golden.WriteCoverage("testdata", "TypeScript/JavaScript", cases); err != nil {
			t.Fatal(err)
		}
	}
}
