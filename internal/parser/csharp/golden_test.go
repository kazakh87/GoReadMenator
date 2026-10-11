package csharp

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
func TestGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*.cs")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("keine Testfälle in testdata/")
	}
	var cases []golden.Case
	for _, f := range files {
		f := f
		c, err := golden.ReadCase(f)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
		t.Run(strings.TrimSuffix(filepath.Base(f), ".cs"), func(t *testing.T) {
			content, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			res, err := New().Parse(context.Background(), parser.Source{Path: filepath.Base(f), Content: content})
			if err != nil {
				t.Fatal(err)
			}
			got := golden.Dump(res)
			golden.Check(t, got, strings.TrimSuffix(f, ".cs")+".golden.txt")
		})
	}
	if golden.Update() {
		if err := golden.WriteCoverage("testdata", "C#", cases); err != nil {
			t.Fatal(err)
		}
	}
}
