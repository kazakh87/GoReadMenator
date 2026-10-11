// Package scanner ermittelt die zu indizierenden Dateien eines Repositories.
//
// Grundlage ist `git ls-tree` auf dem gewählten Commit. Es zählen nur getrackte
// Dateien. Dadurch fallen .gitignore-Einträge automatisch weg, und Monorepo-
// Verzeichnisse wie `packages/` werden nicht fälschlich ausgeschlossen (siehe P0).
package scanner

import (
	"context"
	"path"
	"strings"

	"goreadmenator/internal/gitx"
	"goreadmenator/internal/model"
)

// SkipReason erklärt, warum eine getrackte Datei nicht indiziert wird.
type SkipReason string

const (
	SkipUnsupported SkipReason = "unsupported_ext"
	SkipSymlink     SkipReason = "symlink"
	SkipSubmodule   SkipReason = "submodule"
	SkipTooLarge    SkipReason = "too_large"
	SkipUnsafePath  SkipReason = "unsafe_path"
	SkipNotBlob     SkipReason = "not_blob"
)

// Skip dokumentiert eine übersprungene Datei.
type Skip struct {
	Path   string
	Reason SkipReason
}

// ScannedFile ist eine Datei, die indiziert wird.
type ScannedFile struct {
	Path    string
	Lang    model.Language
	BlobSHA string
	Size    int64
}

// Result ist das Ergebnis eines Scans.
type Result struct {
	Commit  string
	Files   []ScannedFile
	Skipped []Skip
}

// SkipCounts zählt die Übersprünge pro Grund (für die Abschlussmeldung).
func (r Result) SkipCounts() map[SkipReason]int {
	out := map[SkipReason]int{}
	for _, s := range r.Skipped {
		out[s.Reason]++
	}
	return out
}

// Options steuern den Scan.
type Options struct {
	// Commit ist der zu scannende Commit. Leer = HEAD.
	Commit string
	// MaxFileSizeBytes überspringt größere Blobs. 0 = kein Limit.
	MaxFileSizeBytes int64
}

// extLang ordnet Dateiendungen den unterstützten Sprachen zu.
var extLang = map[string]model.Language{
	".cs":     model.LangCSharp,
	".ts":     model.LangTSJS,
	".tsx":    model.LangTSJS,
	".mts":    model.LangTSJS,
	".cts":    model.LangTSJS,
	".js":     model.LangTSJS,
	".jsx":    model.LangTSJS,
	".mjs":    model.LangTSJS,
	".cjs":    model.LangTSJS,
	".vb":     model.LangVBNet,
	".cshtml": model.LangRazor,
	".razor":  model.LangRazor,
	".aspx":   model.LangASPX,
	".ascx":   model.LangASPX,
	".master": model.LangASPX,
}

// LangForPath liefert die Sprache einer Datei anhand der Endung (Groß-/Kleinschreibung egal).
func LangForPath(p string) (model.Language, bool) {
	lang, ok := extLang[strings.ToLower(path.Ext(p))]
	return lang, ok
}

// SafePath prüft einen Repo-relativen Pfad. Abgelehnt werden absolute Pfade,
// Pfade mit ".." und Pfade mit Backslash-Trennern (Windows-Eingaben im Git-Tree).
func SafePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "" {
			return false
		}
	}
	return true
}

// Scan listet die indizierbaren Dateien des Repositories in dir.
func Scan(ctx context.Context, dir string, opt Options) (Result, error) {
	git := gitx.Runner{Dir: dir}

	commit := opt.Commit
	if commit == "" {
		head, err := git.Head(ctx)
		if err != nil {
			return Result{}, err
		}
		commit = head
	}

	entries, err := git.ListTree(ctx, commit)
	if err != nil {
		return Result{}, err
	}

	res := Result{Commit: commit}
	for _, e := range entries {
		if e.Type != "blob" {
			// Trees gibt ls-tree -r nicht aus, Commits (Submodule) aber schon.
			res.Skipped = append(res.Skipped, Skip{Path: e.Path, Reason: SkipNotBlob})
			continue
		}
		if !SafePath(e.Path) {
			res.Skipped = append(res.Skipped, Skip{Path: e.Path, Reason: SkipUnsafePath})
			continue
		}
		switch e.Mode {
		case gitx.ModeSymlink:
			res.Skipped = append(res.Skipped, Skip{Path: e.Path, Reason: SkipSymlink})
			continue
		case gitx.ModeGitlink:
			res.Skipped = append(res.Skipped, Skip{Path: e.Path, Reason: SkipSubmodule})
			continue
		}
		lang, ok := LangForPath(e.Path)
		if !ok {
			res.Skipped = append(res.Skipped, Skip{Path: e.Path, Reason: SkipUnsupported})
			continue
		}
		if opt.MaxFileSizeBytes > 0 && e.Size > opt.MaxFileSizeBytes {
			res.Skipped = append(res.Skipped, Skip{Path: e.Path, Reason: SkipTooLarge})
			continue
		}
		res.Files = append(res.Files, ScannedFile{
			Path:    e.Path,
			Lang:    lang,
			BlobSHA: e.BlobSHA,
			Size:    e.Size,
		})
	}
	return res, nil
}
