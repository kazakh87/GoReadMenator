package razor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goreadmenator/internal/parser"
	"goreadmenator/internal/parser/golden"
)

// TestGolden parst jede Datei in testdata/ und vergleicht das Ergebnis mit der .golden.txt.
// Der Pfad im Test ist "Pages/<Datei>", damit Namensraum und Klasse stabil bleiben.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Fatal(err)
	}
	var cases []golden.Case
	for _, f := range files {
		ext := filepath.Ext(f)
		if ext != ".cshtml" && ext != ".razor" && ext != ".aspx" {
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
			var p parser.Parser = NewRazor()
			if ext == ".aspx" {
				p = NewASPX()
			}
			res, err := p.Parse(context.Background(), parser.Source{Path: "Pages/" + filepath.Base(f), Content: content})
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
		if err := golden.WriteCoverage("testdata", "Razor/ASPX", cases); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNamespaceAndClassFromPath(t *testing.T) {
	cases := map[string][2]string{
		"Pages/Index.cshtml":            {"Pages", "Index"},
		"Shared/Sub Dir/_Layout.cshtml": {"Shared.Sub_Dir", "_Layout"},
		"Home.razor":                    {"", "Home"},
		"2025/Report.cshtml":            {"_2025", "Report"},
	}
	for p, want := range cases {
		ns, cls := namespaceAndClass(p)
		if ns != want[0] || cls != want[1] {
			t.Errorf("namespaceAndClass(%q) = (%q, %q), want (%q, %q)", p, ns, cls, want[0], want[1])
		}
	}
}
