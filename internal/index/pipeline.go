// Package index verbindet Scanner, Parser und Store zu einer Pipeline.
//
// Aufbau:
//
//	Producer ──jobs──▶ Worker × N ──results──▶ Writer (einziger Schreiber)
//
// Jeder Worker besitzt seinen eigenen ContentReader (git cat-file), weil ein
// solcher Prozess nicht nebenläufig nutzbar ist. Der Writer ist die einzige
// Stelle, die in den Store schreibt. Damit gibt es keinen Schreibwettlauf.
//
// Fehler einzelner Dateien stoppen den Lauf nicht. Sie werden im Report
// gesammelt. Fehler des Writers (Store) brechen den Lauf ab. Dabei wird
// weiter gelesen, damit keine Goroutine auf einen Kanal wartet, der nie
// mehr gelesen wird.
package index

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
	"goreadmenator/internal/scanner"
	"goreadmenator/internal/store"
)

// ContentReader liefert den Inhalt eines Blobs. *gitx.CatFile erfüllt es.
type ContentReader interface {
	Blob(sha string) ([]byte, error)
	Close() error
}

// ReaderFactory erstellt je Worker einen eigenen ContentReader.
type ReaderFactory func(ctx context.Context) (ContentReader, error)

// Options steuern den Lauf.
type Options struct {
	Workers   int
	BatchSize int // Datensätze pro WriteBatch
	// Progress wird nach jeder verarbeiteten Datei aufgerufen, vom Writer aus.
	Progress func(done, total int)
}

// FileError beschreibt eine Datei, die nicht indiziert werden konnte.
type FileError struct {
	Path string
	Err  error
}

// Warning ist eine Warnung aus dem Parser (etwa ein Konstrukt, das nur teilweise erfasst wird).
type Warning struct {
	Path    string
	Line    int
	Message string
}

// maxWarnings begrenzt die gespeicherten Warnungen. Gezählt werden alle.
const maxWarnings = 200

// Report fasst den Lauf zusammen.
type Report struct {
	Files        int // erfolgreich indizierte Dateien
	Symbols      int // Symbol-Datensätze inklusive Partial-Mehrfachzählung
	References   int
	Relations    int
	Imports      int
	Errors       []FileError
	Warnings     []Warning // die ersten maxWarnings Warnungen
	WarningCount int       // alle Warnungen
	Duration     time.Duration
}

// Run indiziert die übergebenen Dateien in den Store.
func Run(ctx context.Context, st store.Store, files []scanner.ScannedFile, reg *parser.Registry,
	newReader ReaderFactory, opt Options) (Report, error) {

	start := time.Now()
	if opt.Workers < 1 {
		opt.Workers = 1
	}
	if opt.BatchSize < 1 {
		opt.BatchSize = 1
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan scanner.ScannedFile, opt.Workers*4)
	results := make(chan fileResult, opt.Workers*4)

	// Producer: schreibt Jobs, bis alle Dateien vergeben oder der Lauf abgebrochen ist.
	go func() {
		defer close(jobs)
		for _, f := range files {
			select {
			case jobs <- f:
			case <-runCtx.Done():
				return
			}
		}
	}()

	// Worker: parsen. Jeder hat seinen eigenen ContentReader.
	var wg sync.WaitGroup
	readerErrs := make(chan error, opt.Workers)
	for i := 0; i < opt.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader, err := newReader(runCtx)
			if err != nil {
				readerErrs <- fmt.Errorf("worker: ContentReader: %w", err)
				cancel()
				// Jobs weiter leeren, sonst blockiert der Producer.
				for range jobs {
				}
				return
			}
			defer reader.Close()
			for f := range jobs {
				results <- parseOne(runCtx, f, reg, reader)
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
		close(readerErrs)
	}()

	// Writer: einziger Schreiber, sammelt Batches.
	rep, werr := write(runCtx, cancel, st, results, len(files), opt)

	for err := range readerErrs {
		if werr == nil || errors.Is(werr, context.Canceled) {
			werr = err
		}
	}

	rep.Duration = time.Since(start)
	if ctx.Err() != nil {
		return rep, ctx.Err()
	}
	return rep, werr
}

type fileResult struct {
	file scanner.ScannedFile
	res  parser.Result
	err  error
}

// parseOne liest und parst eine Datei. Fehler werden im Ergebnis transportiert.
func parseOne(ctx context.Context, f scanner.ScannedFile, reg *parser.Registry, reader ContentReader) fileResult {
	out := fileResult{file: f}
	if err := ctx.Err(); err != nil {
		out.err = err
		return out
	}
	p, ok := reg.For(f.Lang)
	if !ok {
		out.err = fmt.Errorf("kein Parser für Sprache %q", f.Lang)
		return out
	}
	content, err := reader.Blob(f.BlobSHA)
	if err != nil {
		out.err = fmt.Errorf("lesen: %w", err)
		return out
	}
	res, err := p.Parse(ctx, parser.Source{Path: f.Path, BlobSHA: f.BlobSHA, Content: content})
	if err != nil {
		out.err = fmt.Errorf("parsen: %w", err)
		return out
	}
	out.res = res
	return out
}

// write konsumiert die Ergebnisse und schreibt sie in Batches. Tritt ein
// Schreibfehler auf, wird ctx abgebrochen (cancel) und danach nur noch
// gelesen, damit die Worker nicht blockieren.
func write(ctx context.Context, cancel context.CancelFunc, st store.Store,
	results <-chan fileResult, total int, opt Options) (Report, error) {

	var rep Report
	var firstErr error
	var pending store.Batch
	pendingRecs := 0
	done := 0

	fail := func(err error) {
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	flush := func() {
		if firstErr == nil && pendingRecs > 0 {
			if err := st.WriteBatch(ctx, pending); err != nil {
				fail(fmt.Errorf("WriteBatch: %w", err))
			}
		}
		// Die Slices werden wiederverwendet. Ein Store kopiert die Datensätze beim Schreiben
		// und darf die Slices nicht selbst behalten (Vertrag von store.Store).
		pending.Symbols = pending.Symbols[:0]
		pending.Locations = pending.Locations[:0]
		pending.References = pending.References[:0]
		pending.Relations = pending.Relations[:0]
		pending.Imports = pending.Imports[:0]
		pendingRecs = 0
	}

	for r := range results {
		done++
		switch {
		case r.err != nil:
			// Abbruch durch den Aufrufer ist kein Dateifehler.
			if !(errors.Is(r.err, context.Canceled) && ctx.Err() != nil) {
				rep.Errors = append(rep.Errors, FileError{Path: r.file.Path, Err: r.err})
			}
		case firstErr == nil:
			// Re-Index: bestehende Datei samt Daten zuerst entfernen.
			// Ausstehende Daten werden vorher geschrieben, damit die Reihenfolge stimmt.
			old, ok, err := st.FileByPath(ctx, r.file.Path)
			if err != nil {
				fail(err)
				break
			}
			if ok {
				flush()
				if firstErr == nil {
					if err := st.DeleteFile(ctx, old.ID); err != nil {
						fail(err)
					}
				}
			}
			if firstErr != nil {
				break
			}
			fid, err := st.PutFile(ctx, model.File{
				Path:    r.file.Path,
				Lang:    r.file.Lang,
				BlobSHA: r.file.BlobSHA,
				LOC:     r.res.LOC,
				FileDoc: r.res.FileDoc,
			})
			if err != nil {
				fail(err)
				break
			}
			pendingRecs += appendResult(&pending, fid, r.res)
			rep.Files++
			rep.Symbols += len(r.res.Symbols)
			rep.References += len(r.res.References)
			rep.Relations += len(r.res.Relations)
			rep.Imports += len(r.res.Imports)
			for _, d := range r.res.Diagnostics {
				if d.Severity != parser.SeverityWarning {
					continue
				}
				rep.WarningCount++
				if len(rep.Warnings) < maxWarnings {
					rep.Warnings = append(rep.Warnings, Warning{Path: r.file.Path, Line: d.Span.StartLine, Message: d.Message})
				}
			}
			if pendingRecs >= opt.BatchSize {
				flush()
			}
		}
		if opt.Progress != nil {
			opt.Progress(done, total)
		}
	}
	flush()
	return rep, firstErr
}

// appendResult hängt die Datensätze einer Datei an den Batch an und setzt die Datei-ID.
// Liefert die Anzahl der Datensätze.
func appendResult(b *store.Batch, fid model.FileID, r parser.Result) int {
	b.Symbols = append(b.Symbols, r.Symbols...)
	for _, l := range r.Locations {
		l.FileID = fid
		b.Locations = append(b.Locations, l)
	}
	for _, x := range r.References {
		x.FileID = fid
		b.References = append(b.References, x)
	}
	for _, x := range r.Relations {
		x.FileID = fid
		b.Relations = append(b.Relations, x)
	}
	for _, x := range r.Imports {
		x.FileID = fid
		b.Imports = append(b.Imports, x)
	}
	return len(r.Symbols) + len(r.Locations) + len(r.References) + len(r.Relations) + len(r.Imports)
}

// ParserVersion kennzeichnet den Stand der Parser und Regeln. Ändert sich der Wert,
// wird der Index beim nächsten Lauf vollständig neu aufgebaut, weil alte und neue Ergebnisse
// sonst gemischt würden. Bei jeder Änderung an Parsern oder Auflösungsregeln erhöhen.
const ParserVersion = "p21-1"
