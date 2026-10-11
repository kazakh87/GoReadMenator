// Package export schreibt den Index in Formate für andere Werkzeuge:
// JSON (als Dokument oder als JSON Lines), Mermaid-Klassendiagramme und Markdown je Datei.
//
// Hinweis zum Speicher: Die Store-Schnittstelle liefert den Bestand heute vollständig
// (AllSymbols usw.). Geschrieben wird aber Datensatz für Datensatz, es entsteht kein
// zweites großes Objekt im Speicher. Mit einem Backend, das Cursor liefert, wird der
// Export auch dort speicherschonend.
package export

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

// Graph ist der vollständige Bestand, wie er geladen wurde.
type Graph struct {
	Files     []model.File
	Symbols   []model.Symbol
	Locations []model.Location
	Refs      []model.Reference
	Relations []model.Relation
	Imports   []model.Import
}

// Load lädt den Bestand aus dem Store.
func Load(ctx context.Context, st store.Store) (Graph, error) {
	var g Graph
	var err error
	if g.Files, err = st.Files(ctx); err != nil {
		return g, err
	}
	if g.Symbols, err = st.AllSymbols(ctx); err != nil {
		return g, err
	}
	if g.Locations, err = st.AllLocations(ctx); err != nil {
		return g, err
	}
	if g.Refs, err = st.AllReferences(ctx); err != nil {
		return g, err
	}
	if g.Relations, err = st.AllRelations(ctx); err != nil {
		return g, err
	}
	if g.Imports, err = st.AllImports(ctx); err != nil {
		return g, err
	}
	return g, nil
}

// WriteJSON schreibt den Graphen als JSON-Dokument. Jeder Datensatz wird einzeln kodiert,
// die Abschnitte werden nacheinander geschrieben.
func WriteJSON(w io.Writer, g Graph) error {
	bw := bufio.NewWriter(w)
	if _, err := fmt.Fprintf(bw, "{\n  \"meta\": {\"format\": \"goreadmenator-graph\", \"version\": 1, \"symbols\": %d, \"references\": %d, \"relations\": %d}",
		len(g.Symbols), len(g.Refs), len(g.Relations)); err != nil {
		return err
	}
	enc := json.NewEncoder(bw)
	sections := []struct {
		name string
		n    int
		each func(i int) any
	}{
		{"files", len(g.Files), func(i int) any { return g.Files[i] }},
		{"symbols", len(g.Symbols), func(i int) any { return g.Symbols[i] }},
		{"locations", len(g.Locations), func(i int) any { return g.Locations[i] }},
		{"references", len(g.Refs), func(i int) any { return g.Refs[i] }},
		{"relations", len(g.Relations), func(i int) any { return g.Relations[i] }},
		{"imports", len(g.Imports), func(i int) any { return g.Imports[i] }},
	}
	for _, sec := range sections {
		if _, err := fmt.Fprintf(bw, ",\n  %q: [", sec.name); err != nil {
			return err
		}
		for i := 0; i < sec.n; i++ {
			if i > 0 {
				bw.WriteByte(',')
			}
			bw.WriteByte('\n')
			if err := enc.Encode(sec.each(i)); err != nil {
				return err
			}
		}
		if _, err := bw.WriteString("]"); err != nil {
			return err
		}
	}
	if _, err := bw.WriteString("\n}\n"); err != nil {
		return err
	}
	return bw.Flush()
}

// WriteJSONL schreibt eine Zeile pro Datensatz: {"type": "...", "data": {...}}.
// Geeignet für Werkzeuge, die zeilenweise verarbeiten (jq, Streams).
func WriteJSONL(w io.Writer, g Graph) error {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	emit := func(typ string, v any) error {
		return enc.Encode(struct {
			Type string `json:"type"`
			Data any    `json:"data"`
		}{typ, v})
	}
	for _, f := range g.Files {
		if err := emit("file", f); err != nil {
			return err
		}
	}
	for _, s := range g.Symbols {
		if err := emit("symbol", s); err != nil {
			return err
		}
	}
	for _, l := range g.Locations {
		if err := emit("location", l); err != nil {
			return err
		}
	}
	for _, r := range g.Refs {
		if err := emit("reference", r); err != nil {
			return err
		}
	}
	for _, r := range g.Relations {
		if err := emit("relation", r); err != nil {
			return err
		}
	}
	for _, im := range g.Imports {
		if err := emit("import", im); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// MermaidOptions steuern das Klassendiagramm.
type MermaidOptions struct {
	// Scope begrenzt auf Typen, deren FQN mit diesem Präfix beginnt (leer = alle).
	Scope string
	// Members aktiviert die Liste der Methoden und Properties in den Klassenkästen.
	Members bool
}

// WriteMermaid schreibt ein Mermaid-Klassendiagramm mit Vererbung, Implementierung
// und Typbeziehungen. Die Knotennamen sind neutral (c1, c2 ...), der Anzeigename steht im Label.
func WriteMermaid(w io.Writer, g Graph, opt MermaidOptions) error {
	typeKinds := map[model.SymbolKind]bool{
		model.KindClass: true, model.KindStruct: true, model.KindInterface: true,
		model.KindRecord: true, model.KindEnum: true, model.KindTypeAlias: true,
	}
	byID := map[model.SymbolID]model.Symbol{}
	for _, s := range g.Symbols {
		byID[s.ID] = s
	}
	include := map[model.SymbolID]bool{}
	var types []model.Symbol
	for _, s := range g.Symbols {
		if !typeKinds[s.Kind] {
			continue
		}
		if opt.Scope != "" && !strings.HasPrefix(s.FQN, opt.Scope) {
			continue
		}
		include[s.ID] = true
		types = append(types, s)
	}
	sort.Slice(types, func(i, j int) bool { return types[i].FQN < types[j].FQN })

	node := map[model.SymbolID]string{}
	bw := bufio.NewWriter(w)
	bw.WriteString("classDiagram\n")
	for i, t := range types {
		id := fmt.Sprintf("c%d", i+1)
		node[t.ID] = id
		fmt.Fprintf(bw, "    class %s[\"%s\"]\n", id, mermaidLabel(t.Name))
		if opt.Members {
			for _, m := range g.Symbols {
				if m.Container != t.ID {
					continue
				}
				switch m.Kind {
				case model.KindMethod, model.KindProperty, model.KindField, model.KindConstructor:
					fmt.Fprintf(bw, "    %s : %s\n", id, mermaidLabel(memberLabel(m)))
				}
			}
		}
	}
	type edge struct{ from, to, arrow string }
	var edges []edge
	seen := map[string]bool{}
	for _, r := range g.Relations {
		if !include[r.From] || !include[r.To] {
			continue
		}
		var arrow string
		switch r.Kind {
		case model.RelInherits:
			arrow = "<|--"
		case model.RelImplements:
			arrow = "<|.."
		case model.RelHasType:
			arrow = "-->"
		default:
			continue
		}
		// Pfeil zeigt vom Basistyp zum Ableitenden: from ist ableitend, to ist die Basis.
		e := edge{from: node[r.To], to: node[r.From], arrow: arrow}
		if r.Kind == model.RelHasType {
			e = edge{from: node[r.From], to: node[r.To], arrow: arrow}
		}
		key := e.from + e.arrow + e.to
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].from+edges[i].to < edges[j].from+edges[j].to
	})
	for _, e := range edges {
		fmt.Fprintf(bw, "    %s %s %s\n", e.from, e.arrow, e.to)
	}
	return bw.Flush()
}

func memberLabel(m model.Symbol) string {
	if m.Signature != "" {
		return m.Signature
	}
	return m.Name
}

// mermaidLabel entfernt Zeichen, die in Mermaid-Labels stören.
func mermaidLabel(s string) string {
	r := strings.NewReplacer("\"", "'", "<", "&lt;", ">", "&gt;", "\n", " ")
	return r.Replace(s)
}

// WriteMarkdown schreibt je Quelldatei eine Markdown-Seite nach dir: Symbole mit Signatur,
// Dokumentation, Fundorten und der Anzahl eingehender Verwendungen.
func WriteMarkdown(dir string, g Graph) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	pathOf := map[model.FileID]string{}
	for _, f := range g.Files {
		pathOf[f.ID] = f.Path
	}
	incoming := map[model.SymbolID]int{}
	for _, r := range g.Refs {
		if r.TargetID != "" {
			incoming[r.TargetID]++
		}
	}
	byFile := map[string][]model.Symbol{}
	symFiles := map[model.SymbolID]map[string]model.Location{}
	for _, l := range g.Locations {
		p := pathOf[l.FileID]
		if symFiles[l.SymbolID] == nil {
			symFiles[l.SymbolID] = map[string]model.Location{}
		}
		symFiles[l.SymbolID][p] = l
	}
	symByID := map[model.SymbolID]model.Symbol{}
	for _, s := range g.Symbols {
		symByID[s.ID] = s
	}
	for id, locs := range symFiles {
		s, ok := symByID[id]
		if !ok {
			continue
		}
		for p := range locs {
			byFile[p] = append(byFile[p], s)
		}
	}

	var written []string
	paths := make([]string, 0, len(byFile))
	for p := range byFile {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		syms := byFile[p]
		sort.Slice(syms, func(i, j int) bool {
			return symFiles[syms[i].ID][p].Span.StartLine < symFiles[syms[j].ID][p].Span.StartLine
		})
		out := filepath.Join(dir, safeName(p)+".md")
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return written, err
		}
		f, err := os.Create(out)
		if err != nil {
			return written, err
		}
		bw := bufio.NewWriter(f)
		fmt.Fprintf(bw, "# %s\n\n", p)
		for _, s := range syms {
			loc := symFiles[s.ID][p]
			fmt.Fprintf(bw, "## %s `%s`\n\n", s.Kind, s.FQN)
			if s.Signature != "" {
				fmt.Fprintf(bw, "```\n%s\n```\n\n", s.Signature)
			}
			if s.Summary != "" {
				fmt.Fprintf(bw, "%s\n\n", s.Summary)
			}
			if s.Returns != "" {
				fmt.Fprintf(bw, "**Rückgabe:** %s\n\n", s.Returns)
			}
			for _, pd := range s.ParamDocs {
				fmt.Fprintf(bw, "- `%s`: %s\n", pd.Name, pd.Text)
			}
			if len(s.ParamDocs) > 0 {
				bw.WriteString("\n")
			}
			fmt.Fprintf(bw, "Zeilen %d–%d", loc.Span.StartLine, loc.Span.EndLine)
			if n := incoming[s.ID]; n > 0 {
				fmt.Fprintf(bw, " · %d eingehende Verwendung(en)", n)
			}
			bw.WriteString("\n\n")
		}
		if err := bw.Flush(); err != nil {
			f.Close()
			return written, err
		}
		if err := f.Close(); err != nil {
			return written, err
		}
		written = append(written, out)
	}
	return written, nil
}

func safeName(p string) string {
	return strings.NewReplacer("/", "__", "\\", "__", " ", "_").Replace(p)
}
