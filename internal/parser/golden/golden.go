// Package golden stellt Golden-File-Tests für die Parser bereit.
//
// Jeder Testfall besteht aus einer Eingabedatei (testdata/*.cs bzw. *.ts) und einer erwarteten
// Ausgabe (*.golden.txt). Die Eingabedatei beginnt mit Kommentaren, die beschreiben, welche
// Sprachkonstrukte sie abdeckt. Die Ausgabe ist deterministisch sortiert, damit ein Unterschied
// sofort auffällt.
//
// Aktualisieren nach einer bewussten Verhaltensänderung:
//
//	READMENATOR_UPDATE_GOLDEN=1 go test ./internal/parser/...
//
// Dabei wird außerdem testdata/COVERAGE.md neu erzeugt, eine Übersicht aller Fälle.
package golden

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// Update ist true, wenn die Golden-Dateien neu geschrieben werden sollen.
func Update() bool { return os.Getenv("READMENATOR_UPDATE_GOLDEN") == "1" }

// Dump wandelt ein Parse-Ergebnis in eine stabile Textform um. Jede Zeile beginnt mit einer
// Kategorie, innerhalb der Kategorie wird sortiert. Dateiendungen und Nebenläufigkeit spielen
// dadurch keine Rolle.
func Dump(res parser.Result) string {
	var sections = map[string][]string{}
	add := func(k, line string) { sections[k] = append(sections[k], line) }

	add("file", fmt.Sprintf("file loc=%d filedoc=%q", res.LOC, res.FileDoc))
	for _, s := range res.Symbols {
		parts := []string{
			"sym " + string(s.ID),
			"kind=" + string(s.Kind),
			"name=" + s.Name,
			"fqn=" + s.FQN,
		}
		if s.Container != "" {
			parts = append(parts, "container="+string(s.Container))
		}
		if len(s.Modifiers) > 0 {
			parts = append(parts, "mods="+strings.Join(s.Modifiers, ","))
		}
		if s.Signature != "" {
			parts = append(parts, fmt.Sprintf("sig=%q", s.Signature))
		}
		if s.Summary != "" {
			parts = append(parts, fmt.Sprintf("summary=%q", s.Summary))
		}
		if s.Remarks != "" {
			parts = append(parts, fmt.Sprintf("remarks=%q", s.Remarks))
		}
		if s.Returns != "" {
			parts = append(parts, fmt.Sprintf("returns=%q", s.Returns))
		}
		for _, p := range s.ParamDocs {
			parts = append(parts, fmt.Sprintf("param[%s]=%q", p.Name, p.Text))
		}
		if s.DocSource != "" {
			parts = append(parts, "doc="+s.DocSource)
		}
		add("sym", strings.Join(parts, " "))
	}
	for _, l := range res.Locations {
		add("loc", fmt.Sprintf("loc %s %s %d:%d-%d:%d", l.SymbolID, l.Role,
			l.Span.StartLine, l.Span.StartCol, l.Span.EndLine, l.Span.EndCol))
	}
	for _, r := range res.References {
		parts := []string{
			"ref " + string(r.Kind),
			fmt.Sprintf("target=%q", r.TargetName),
			fmt.Sprintf("at=%d:%d-%d:%d", r.Span.StartLine, r.Span.StartCol, r.Span.EndLine, r.Span.EndCol),
			"conf=" + string(r.Confidence),
		}
		if r.Enclosing != "" {
			parts = append(parts, "in="+string(r.Enclosing))
		}
		if r.Arity != 0 {
			parts = append(parts, fmt.Sprintf("arity=%d", r.Arity))
		}
		if r.Role != "" {
			parts = append(parts, "role="+r.Role)
		}
		if r.Qualifier != "" {
			parts = append(parts, "qual="+r.Qualifier)
		}
		add("ref", strings.Join(parts, " "))
	}
	for _, r := range res.Relations {
		add("rel", fmt.Sprintf("rel %s %s -> %s", r.Kind, r.From, r.To))
	}
	for _, im := range res.Imports {
		var b []string
		for _, bd := range im.Bindings {
			b = append(b, bd.Local+"="+bd.Imported)
		}
		line := fmt.Sprintf("imp %s %q", im.Kind, im.Raw)
		if len(b) > 0 {
			line += " bindings=[" + strings.Join(b, ",") + "]"
		}
		add("imp", line)
	}
	for _, d := range res.Diagnostics {
		kind := "diag"
		if d.Severity == parser.SeverityWarning {
			kind = "warn"
		}
		add("diag", fmt.Sprintf("%s %d:%d %s", kind, d.Span.StartLine, d.Span.StartCol, d.Message))
	}

	var out []string
	for _, k := range []string{"file", "sym", "loc", "ref", "rel", "imp", "diag"} {
		lines := sections[k]
		sort.Strings(lines)
		if len(lines) == 0 && k != "file" {
			out = append(out, "# "+k+": keine")
			continue
		}
		out = append(out, lines...)
	}
	return strings.Join(out, "\n") + "\n"
}

// Case beschreibt einen Testfall. Title und Notes stammen aus den Kommentaren der Eingabedatei.
type Case struct {
	File   string
	Title  string
	Covers []string
}

// ReadCase liest Titel und Abdeckung aus den Kommentarzeilen am Dateianfang.
// Erwartetes Format: Zeilen mit "// SZENARIO:" (Titel) und "// ABDECKUNG:" (Stichpunkte, Komma getrennt).
// Zeilen mit "//" und ähnlichen Präfixen (# für TS-Kommentare nicht nötig) sind erlaubt.
func ReadCase(path string) (Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return Case{}, err
	}
	defer f.Close()
	c := Case{File: filepath.Base(path)}
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if first {
			first = false
		}
		// Kommentarzeichen je Sprache: // (C#, TS, JS) und ' (VB.NET)
		var body string
		switch {
		case strings.HasPrefix(line, "//"):
			body = strings.TrimSpace(strings.TrimPrefix(line, "//"))
		case strings.HasPrefix(line, "'"):
			body = strings.TrimSpace(strings.TrimPrefix(line, "'"))
		default:
			if line == "" {
				continue
			}
			break
		}
		switch {
		case strings.HasPrefix(body, "SZENARIO:"):
			c.Title = strings.TrimSpace(strings.TrimPrefix(body, "SZENARIO:"))
		case strings.HasPrefix(body, "ABDECKUNG:"):
			for _, p := range strings.Split(strings.TrimPrefix(body, "ABDECKUNG:"), ",") {
				if p = strings.TrimSpace(p); p != "" {
					c.Covers = append(c.Covers, p)
				}
			}
		}
	}
	return c, sc.Err()
}

// Check vergleicht die Ausgabe eines Parsers mit der Golden-Datei. Im Aktualisierungsmodus
// wird die Datei geschrieben.
func Check(t *testing.T, got, goldenPath string) {
	t.Helper()
	if Update() {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Golden-Datei fehlt (mit READMENATOR_UPDATE_GOLDEN=1 erzeugen): %v", err)
	}
	if string(want) != got {
		t.Errorf("Ausgabe weicht von %s ab.\n%s", filepath.Base(goldenPath), diff(string(want), got))
	}
}

// diff liefert die ersten abweichenden Zeilen, damit der Fehler lesbar bleibt.
func diff(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	var b strings.Builder
	shown := 0
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			fmt.Fprintf(&b, "Zeile %d\n  erwartet: %s\n  erhalten: %s\n", i+1, wl, gl)
			shown++
			if shown >= 8 {
				b.WriteString("  … weitere Abweichungen ausgelassen\n")
				break
			}
		}
	}
	return b.String()
}

// WriteCoverage erzeugt die Übersicht aller Fälle als Markdown.
func WriteCoverage(dir string, lang string, cases []Case) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Testabdeckung %s\n\n", lang)
	fmt.Fprintf(&b, "Automatisch erzeugt. Jeder Fall ist eine Eingabedatei mit Golden-Datei.\n\n")
	fmt.Fprintf(&b, "| Datei | Szenario | Abgedeckt |\n|---|---|---|\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c.File, c.Title, strings.Join(c.Covers, ", "))
	}
	return os.WriteFile(filepath.Join(dir, "COVERAGE.md"), []byte(b.String()), 0o644)
}

// Model ist ein Verweis, damit Importe nicht als ungenutzt gelten, wenn Testfälle erweitert werden.
var _ = model.KindClass

// ReadScenarioFile liest Titel und Abdeckung aus einer scenario.txt-Datei. Für Testfälle, die
// aus einem ganzen Verzeichnis bestehen (Auflösung, Abfragen). Format: Zeilen "SZENARIO: ..."
// und "ABDECKUNG: ...".
func ReadScenarioFile(dir string) (Case, error) {
	c := Case{File: filepath.Base(dir) + "/"}
	data, err := os.ReadFile(filepath.Join(dir, "scenario.txt"))
	if err != nil {
		return c, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "SZENARIO:"):
			c.Title = strings.TrimSpace(strings.TrimPrefix(line, "SZENARIO:"))
		case strings.HasPrefix(line, "ABDECKUNG:"):
			for _, p := range strings.Split(strings.TrimPrefix(line, "ABDECKUNG:"), ",") {
				if p = strings.TrimSpace(p); p != "" {
					c.Covers = append(c.Covers, p)
				}
			}
		}
	}
	return c, nil
}

// CopyTree kopiert ein Verzeichnis, ohne Szenario- und Golden-Dateien.
func CopyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		base := filepath.Base(p)
		if base == "scenario.txt" || base == "expected.golden.txt" || base == "queries.txt" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}

// WriteCoverageDirs erzeugt die Übersicht für Testfälle aus Verzeichnissen.
func WriteCoverageDirs(dir, lang string, cases []Case) error {
	return WriteCoverage(dir, lang, cases)
}
