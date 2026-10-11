// Package impact beantwortet Fragen zu Änderungen: welche Symbole hat ein Diff getroffen,
// und welche Stellen verwenden sie. Grundlage sind der Index und die Zeilenbereiche aus git diff.
package impact

import (
	"context"
	"sort"
	"strconv"

	"goreadmenator/internal/gitx"
	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

// ChangedSymbol ist ein Symbol, dessen Fundort einen geänderten Bereich überlappt.
// Direct ist true, wenn kein Kind-Symbol die Änderung enthält (die innerste Stelle).
type ChangedSymbol struct {
	Symbol model.Symbol
	File   string
	Span   model.Span
	Direct bool
}

// Hit ist eine Verwendung eines betroffenen Symbols.
type Hit struct {
	Depth  int            // 1 = direkte Verwendung
	Target model.SymbolID // das betroffene Symbol, von dem aus gesucht wurde
	User   model.SymbolID // umgebendes Symbol der Verwendung (leer auf Datei-Ebene)
	File   string
	Span   model.Span
	Kind   model.RefKind
	Name   string
}

// Changed liefert die Symbole, die zwischen from und to geändert wurden. Der Index muss den Stand `to` haben.
func Changed(ctx context.Context, st store.Store, repoDir, from, to string) ([]ChangedSymbol, error) {
	hunks, err := gitx.Runner{Dir: repoDir}.DiffHunks(ctx, from, to)
	if err != nil {
		return nil, err
	}
	files, err := st.Files(ctx)
	if err != nil {
		return nil, err
	}
	idOf := map[string]model.FileID{}
	for _, f := range files {
		idOf[f.Path] = f.ID
	}
	locs, err := st.AllLocations(ctx)
	if err != nil {
		return nil, err
	}
	syms, err := st.AllSymbols(ctx)
	if err != nil {
		return nil, err
	}
	symByID := map[model.SymbolID]model.Symbol{}
	for _, s := range syms {
		symByID[s.ID] = s
	}

	var out []ChangedSymbol
	for path, ranges := range hunks {
		fid, ok := idOf[path]
		if !ok {
			continue
		}
		// Kandidaten: Symbole mit Fundort in dieser Datei, die einen Bereich überlappen.
		var cands []model.Location
		for _, l := range locs {
			if l.FileID != fid || l.Role == "" {
				continue
			}
			for _, rg := range ranges {
				if rg.Start <= l.Span.EndLine && rg.End >= l.Span.StartLine {
					cands = append(cands, l)
					break
				}
			}
		}
		// Direkt = kein anderer Kandidat ist ein Nachfahre des Symbols.
		hitContainer := map[model.SymbolID]bool{}
		for _, c := range cands {
			if s, ok := symByID[c.SymbolID]; ok && s.Container != "" {
				hitContainer[s.Container] = true
			}
		}
		for _, c := range cands {
			s, ok := symByID[c.SymbolID]
			if !ok {
				continue
			}
			out = append(out, ChangedSymbol{Symbol: s, File: path, Span: c.Span, Direct: !hitContainer[s.ID]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Span.StartLine < out[j].Span.StartLine
	})
	return out, nil
}

// Analyze liefert die Verwendungen eines Symbols bis zur Tiefe depth. Verwendungen über
// umgebende Symbole werden rekursiv weiterverfolgt, damit die Wirkung über Aufrufketten sichtbar wird.
func Analyze(ctx context.Context, st store.Store, target model.SymbolID, depth int) ([]Hit, error) {
	if depth < 1 {
		depth = 1
	}
	files, err := st.Files(ctx)
	if err != nil {
		return nil, err
	}
	pathOf := map[model.FileID]string{}
	for _, f := range files {
		pathOf[f.ID] = f.Path
	}

	var hits []Hit
	seenHit := map[string]bool{}
	visited := map[model.SymbolID]bool{target: true}
	frontier := []model.SymbolID{target}
	for d := 1; d <= depth && len(frontier) > 0; d++ {
		var next []model.SymbolID
		for _, sid := range frontier {
			refs, err := st.ReferencesTo(ctx, sid)
			if err != nil {
				return nil, err
			}
			for _, r := range refs {
				key := string(sid) + "|" + string(r.Enclosing) + "|" + pathOf[r.FileID] + "|" + itoa(r.Span.StartLine)
				if seenHit[key] {
					continue
				}
				seenHit[key] = true
				hits = append(hits, Hit{
					Depth: d, Target: sid, User: r.Enclosing, File: pathOf[r.FileID],
					Span: r.Span, Kind: r.Kind, Name: r.TargetName,
				})
				if r.Enclosing != "" && !visited[r.Enclosing] {
					visited[r.Enclosing] = true
					next = append(next, r.Enclosing)
				}
			}
		}
		frontier = next
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Depth != hits[j].Depth {
			return hits[i].Depth < hits[j].Depth
		}
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Span.StartLine < hits[j].Span.StartLine
	})
	return hits, nil
}

// ByName wendet Analyze auf alle Symbole mit diesem Namen an.
func ByName(ctx context.Context, st store.Store, name string, depth int) (map[model.SymbolID][]Hit, error) {
	syms, err := st.SymbolsByName(ctx, name)
	if err != nil {
		return nil, err
	}
	out := map[model.SymbolID][]Hit{}
	for _, s := range syms {
		hits, err := Analyze(ctx, st, s.ID, depth)
		if err != nil {
			return nil, err
		}
		out[s.ID] = hits
	}
	return out, nil
}

func itoa(v int) string { return strconv.Itoa(v) }
