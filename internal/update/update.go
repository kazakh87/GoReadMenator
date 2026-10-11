// Package update hält den Index aktuell. Es gibt zwei Wege:
//
//   - Vollaufbau: alle Dateien des HEAD werden geparst. Das passiert beim ersten Lauf,
//     nach einem Parser-Update oder wenn der gespeicherte Commit nicht mehr existiert.
//   - Inkrement: nur die Dateien, die sich zwischen dem gespeicherten und dem neuen Commit
//     geändert haben, werden neu geparst. Gelöschte Dateien verschwinden samt ihren Daten.
//     Verwendungen, deren Name von der Änderung betroffen sein kann, werden neu aufgelöst.
//
// Umbenennungen werden als Löschen plus Neuanlage behandelt. Das ist einfacher und
// für die Ergebnisse gleichwertig, weil der Inhalt ohnehin neu geparst wird.
package update

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"goreadmenator/internal/gitx"
	"goreadmenator/internal/index"
	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
	"goreadmenator/internal/resolve"
	"goreadmenator/internal/scanner"
	"goreadmenator/internal/store"
	"goreadmenator/internal/xmlimport"
)

// Meta-Schlüssel im Store.
const (
	MetaLastCommit    = "last_commit"
	MetaParserVersion = "parser_version"
)

// Options steuern den Lauf.
type Options struct {
	RepoDir          string
	Registry         *parser.Registry
	Workers          int
	BatchSize        int
	MaxFileSizeBytes int64
	// Force erzwingt den Vollaufbau, auch wenn ein Inkrement möglich wäre.
	Force bool
	// Progress wird vom Indexer aufgerufen (optional).
	Progress func(done, total int)
}

// Result beschreibt, was der Lauf getan hat.
type Result struct {
	Mode     string // "full", "incremental" oder "noop"
	From     string // vorheriger Commit (leer bei Vollaufbau)
	To       string
	Added    int // Dateien, die neu geparst wurden
	Removed  int // Dateien, die aus dem Index entfernt wurden
	Parsed   int // tatsächlich geparste Dateien
	Index    index.Report
	Resolve  resolve.Stats
	Duration time.Duration
}

// Run aktualisiert den Index auf den HEAD-Commit des Repositories.
func Run(ctx context.Context, st store.Store, opt Options) (Result, error) {
	start := time.Now()
	res := Result{}
	g := gitx.Runner{Dir: opt.RepoDir}

	head, err := g.Head(ctx)
	if err != nil {
		return res, err
	}
	res.To = head

	last, hasLast, err := st.Meta(ctx, MetaLastCommit)
	if err != nil {
		return res, err
	}
	pv, hasPV, err := st.Meta(ctx, MetaParserVersion)
	if err != nil {
		return res, err
	}

	fullNeeded := opt.Force || !hasLast || !hasPV || pv != index.ParserVersion || !g.CommitExists(ctx, last)
	if fullNeeded {
		if err := fullBuild(ctx, st, g, head, opt, &res); err != nil {
			return res, err
		}
	} else if last == head {
		res.Mode = "noop"
		res.From = last
	} else {
		res.From = last
		if err := incremental(ctx, st, g, last, head, opt, &res); err != nil {
			return res, err
		}
	}

	if res.Mode != "noop" {
		if err := st.SetMeta(ctx, MetaLastCommit, head); err != nil {
			return res, err
		}
		if err := st.SetMeta(ctx, MetaParserVersion, index.ParserVersion); err != nil {
			return res, err
		}
	} else if !hasPV {
		if err := st.SetMeta(ctx, MetaParserVersion, index.ParserVersion); err != nil {
			return res, err
		}
	}
	res.Duration = time.Since(start)
	return res, nil
}

// fullBuild entfernt alle Dateien und parst den kompletten HEAD neu.
func fullBuild(ctx context.Context, st store.Store, g gitx.Runner, head string, opt Options, res *Result) error {
	res.Mode = "full"
	existing, err := st.Files(ctx)
	if err != nil {
		return err
	}
	for _, f := range existing {
		if strings.HasPrefix(f.BlobSHA, xmlimport.Prefix) {
			continue // Importe aus XML-Dokumentation gehören nicht zu Git und bleiben erhalten
		}
		if err := st.DeleteFile(ctx, f.ID); err != nil {
			return err
		}
		res.Removed++
	}
	files, err := selectFiles(ctx, opt, head, nil)
	if err != nil {
		return err
	}
	return indexAndResolve(ctx, st, g, files, opt, res, nil)
}

// incremental verarbeitet nur die Änderungen zwischen last und head.
func incremental(ctx context.Context, st store.Store, g gitx.Runner, last, head string, opt Options, res *Result) error {
	res.Mode = "incremental"
	changes, err := g.DiffRaw(ctx, last, head)
	if err != nil {
		return err
	}

	// Pfade, die sich geändert haben oder entfallen.
	touched := map[string]bool{}
	for _, c := range changes {
		touched[c.Path] = true
		if c.OldPath != "" {
			touched[c.OldPath] = true
		}
	}

	// Namen vor der Änderung: Symbole und Verwendungen der betroffenen Dateien.
	// Nur so werden Verwendungen gefunden, deren Ziel verschwindet oder sich ändert.
	affected := map[string]bool{}
	if err := collectNames(ctx, st, touched, affected); err != nil {
		return err
	}

	// Betroffene Dateien entfernen (Umbenennung = Löschen + Neuanlage).
	for p := range touched {
		f, ok, err := st.FileByPath(ctx, p)
		if err != nil {
			return err
		}
		if ok {
			if err := st.DeleteFile(ctx, f.ID); err != nil {
				return err
			}
			res.Removed++
		}
	}

	// Neu zu parsende Dateien: alle geänderten Pfade, die im HEAD existieren und unterstützt sind.
	added := map[string]bool{}
	for _, c := range changes {
		if c.Status != 'D' {
			added[c.Path] = true
		}
	}
	files, err := selectFiles(ctx, opt, head, added)
	if err != nil {
		return err
	}
	return indexAndResolve(ctx, st, g, files, opt, res, affected)
}

// selectFiles liefert die indizierbaren Dateien des Commits. only begrenzt auf bestimmte Pfade.
func selectFiles(ctx context.Context, opt Options, commit string, only map[string]bool) ([]scanner.ScannedFile, error) {
	sc, err := scanner.Scan(ctx, opt.RepoDir, scanner.Options{Commit: commit, MaxFileSizeBytes: opt.MaxFileSizeBytes})
	if err != nil {
		return nil, err
	}
	var out []scanner.ScannedFile
	for _, f := range sc.Files {
		if only != nil && !only[f.Path] {
			continue
		}
		if _, ok := opt.Registry.For(f.Lang); !ok {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// collectNames sammelt Symbolnamen und Verwendungsnamen der Dateien mit den gegebenen Pfaden.
func collectNames(ctx context.Context, st store.Store, paths map[string]bool, names map[string]bool) error {
	files, err := st.Files(ctx)
	if err != nil {
		return err
	}
	ids := map[model.FileID]bool{}
	for _, f := range files {
		if paths[f.Path] {
			ids[f.ID] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	locs, err := st.AllLocations(ctx)
	if err != nil {
		return err
	}
	symIDs := map[model.SymbolID]bool{}
	for _, l := range locs {
		if ids[l.FileID] {
			symIDs[l.SymbolID] = true
		}
	}
	syms, err := st.AllSymbols(ctx)
	if err != nil {
		return err
	}
	for _, s := range syms {
		if symIDs[s.ID] {
			names[s.Name] = true
		}
	}
	refs, err := st.AllReferences(ctx)
	if err != nil {
		return err
	}
	for _, r := range refs {
		if ids[r.FileID] && r.TargetName != "" {
			names[r.TargetName] = true
		}
	}
	return nil
}

// indexAndResolve parst die Dateien, schreibt sie und löst die betroffenen Namen neu auf.
// affected ist nil bei Vollaufbau (alles wird aufgelöst).
func indexAndResolve(ctx context.Context, st store.Store, g gitx.Runner, files []scanner.ScannedFile,
	opt Options, res *Result, affected map[string]bool) error {

	res.Added = len(files)
	if len(files) > 0 {
		cat := func(c context.Context) (index.ContentReader, error) { return g.OpenCatFile(c) }
		rep, err := index.Run(ctx, st, files, opt.Registry, cat, index.Options{
			Workers: opt.Workers, BatchSize: opt.BatchSize, Progress: opt.Progress,
		})
		res.Index = rep
		res.Parsed = rep.Files
		if err != nil {
			return err
		}
		if len(rep.Errors) > 0 {
			return fmt.Errorf("%d Datei(en) konnten nicht indiziert werden, zuerst: %s: %v",
				len(rep.Errors), rep.Errors[0].Path, rep.Errors[0].Err)
		}
	}

	// Neue Symbolnamen gehören ebenfalls zu den betroffenen Namen.
	if affected != nil {
		newPaths := map[string]bool{}
		for _, f := range files {
			newPaths[f.Path] = true
		}
		if err := collectNames(ctx, st, newPaths, affected); err != nil {
			return err
		}
	}

	var only map[string]bool
	if affected != nil {
		only = affected
	}
	rstats, err := resolve.Run(ctx, st, resolve.Options{RepoDir: opt.RepoDir, OnlyNames: only})
	res.Resolve = rstats
	return err
}

// Names liefert die sortierte Liste der Namen (für Ausgaben und Tests).
func Names(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
