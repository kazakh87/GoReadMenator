package index

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
	"goreadmenator/internal/scanner"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
)

// fakeParser liefert pro Datei ein Symbol, dessen ID aus dem Pfad entsteht.
// Dateien, deren Pfad "bad" enthält, schlagen fehl.
type fakeParser struct {
	calls  atomic.Int64
	onCall func(n int64) // optionaler Hook, etwa zum Abbrechen
}

func (p *fakeParser) Language() model.Language { return model.LangCSharp }

func (p *fakeParser) Parse(_ context.Context, src parser.Source) (parser.Result, error) {
	n := p.calls.Add(1)
	if p.onCall != nil {
		p.onCall(n)
	}
	if strings.Contains(src.Path, "bad") {
		return parser.Result{}, errors.New("syntaxfehler")
	}
	id := model.NewSymbolID(model.LangCSharp, "N."+strings.ReplaceAll(src.Path, "/", "_"), nil)
	return parser.Result{
		LOC:       len(src.Content),
		Symbols:   []model.Symbol{{ID: id, Kind: model.KindClass, Name: src.Path}},
		Locations: []model.Location{{SymbolID: id, Role: model.RoleDecl, Span: model.Span{StartLine: 1, EndLine: 1}}},
		Imports:   []model.Import{{Raw: "x", Kind: model.ImportUsing}},
	}, nil
}

// memReader liefert Inhalte aus einer Map. Jeder Worker bekommt eine eigene Instanz.
type memReader struct{ blobs map[string][]byte }

func (r memReader) Blob(sha string) ([]byte, error) {
	b, ok := r.blobs[sha]
	if !ok {
		return nil, fmt.Errorf("blob %s fehlt", sha)
	}
	return b, nil
}
func (r memReader) Close() error { return nil }

func makeFiles(n int) ([]scanner.ScannedFile, map[string][]byte) {
	files := make([]scanner.ScannedFile, n)
	blobs := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		sha := fmt.Sprintf("%040x", i+1)
		path := fmt.Sprintf("dir%d/file%05d.cs", i%37, i)
		files[i] = scanner.ScannedFile{Path: path, Lang: model.LangCSharp, BlobSHA: sha}
		blobs[sha] = []byte("class X {}")
	}
	return files, blobs
}

func factory(blobs map[string][]byte) ReaderFactory {
	return func(context.Context) (ContentReader, error) { return memReader{blobs}, nil }
}

func newReg(t *testing.T, p parser.Parser) *parser.Registry {
	t.Helper()
	reg, err := parser.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// runWithTimeout verhindert, dass ein Deadlock den ganzen Testlauf hängen lässt.
func runWithTimeout(t *testing.T, d time.Duration, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("Lauf hängt länger als %s (Deadlock?)", d)
		return nil
	}
}

func TestRunTenThousandFiles(t *testing.T) {
	files, blobs := makeFiles(10000)
	st := memory.New()
	reg := newReg(t, &fakeParser{})
	var progressCalls atomic.Int64

	var rep Report
	err := runWithTimeout(t, 60*time.Second, func() error {
		var err error
		rep, err = Run(context.Background(), st, files, reg, factory(blobs), Options{
			Workers:   8,
			BatchSize: 500,
			Progress:  func(done, total int) { progressCalls.Add(1) },
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 10000 || len(rep.Errors) != 0 {
		t.Fatalf("Report: files=%d errors=%d", rep.Files, len(rep.Errors))
	}
	if progressCalls.Load() != 10000 {
		t.Fatalf("Progress-Aufrufe = %d, want 10000", progressCalls.Load())
	}
	stored, err := st.Files(context.Background())
	if err != nil || len(stored) != 10000 {
		t.Fatalf("gespeicherte Dateien = %d (%v)", len(stored), err)
	}
	// Stichprobe: jede Datei hat genau ein Symbol mit Fundort.
	sample := files[1234]
	id := model.NewSymbolID(model.LangCSharp, "N."+strings.ReplaceAll(sample.Path, "/", "_"), nil)
	locs, _ := st.Locations(context.Background(), id)
	if len(locs) != 1 {
		t.Fatalf("Fundorte für Stichprobe = %d", len(locs))
	}
}

func TestParseErrorsDoNotStopRun(t *testing.T) {
	files := []scanner.ScannedFile{
		{Path: "ok1.cs", Lang: model.LangCSharp, BlobSHA: "a"},
		{Path: "bad.cs", Lang: model.LangCSharp, BlobSHA: "b"},
		{Path: "ok2.cs", Lang: model.LangCSharp, BlobSHA: "c"},
		{Path: "unknown.ts", Lang: model.LangTSJS, BlobSHA: "d"}, // kein Parser registriert
	}
	blobs := map[string][]byte{"a": []byte("x"), "b": []byte("y"), "c": []byte("z"), "d": []byte("w")}
	st := memory.New()
	rep, err := Run(context.Background(), st, files, newReg(t, &fakeParser{}), factory(blobs), Options{Workers: 2, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 2 {
		t.Fatalf("Files = %d, want 2", rep.Files)
	}
	if len(rep.Errors) != 2 {
		t.Fatalf("Errors = %d, want 2: %+v", len(rep.Errors), rep.Errors)
	}
}

func TestRerunIsIdempotent(t *testing.T) {
	files, blobs := makeFiles(300)
	st := memory.New()
	reg := newReg(t, &fakeParser{})
	for i := 0; i < 2; i++ {
		if _, err := Run(context.Background(), st, files, reg, factory(blobs), Options{Workers: 4, BatchSize: 50}); err != nil {
			t.Fatal(err)
		}
	}
	stored, _ := st.Files(context.Background())
	if len(stored) != 300 {
		t.Fatalf("nach zweitem Lauf %d Dateien, want 300", len(stored))
	}
	id := model.NewSymbolID(model.LangCSharp, "N."+strings.ReplaceAll(files[7].Path, "/", "_"), nil)
	locs, _ := st.Locations(context.Background(), id)
	if len(locs) != 1 {
		t.Fatalf("Fundorte nach Re-Index = %d, want 1 (keine Duplikate)", len(locs))
	}
}

func TestCancelStopsRun(t *testing.T) {
	files, blobs := makeFiles(5000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &fakeParser{onCall: func(n int64) {
		if n == 200 {
			cancel()
		}
	}}
	err := runWithTimeout(t, 30*time.Second, func() error {
		_, err := Run(ctx, memory.New(), files, newReg(t, p), factory(blobs), Options{Workers: 4, BatchSize: 100})
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n := p.calls.Load(); n >= 5000 {
		t.Fatalf("Abbruch wirkte nicht: %d von 5000 Dateien geparst", n)
	}
}

// failingStore lässt WriteBatch nach einer Anzahl Aufrufe fehlschlagen.
type failingStore struct {
	*memory.Store
	left atomic.Int64
}

func (f *failingStore) WriteBatch(ctx context.Context, b store.Batch) error {
	if f.left.Add(-1) < 0 {
		return errors.New("disk full")
	}
	return f.Store.WriteBatch(ctx, b)
}

func TestStoreErrorAbortsWithoutDeadlock(t *testing.T) {
	files, blobs := makeFiles(2000)
	st := &failingStore{Store: memory.New()}
	st.left.Store(3)
	err := runWithTimeout(t, 30*time.Second, func() error {
		_, err := Run(context.Background(), st, files, newReg(t, &fakeParser{}), factory(blobs), Options{Workers: 6, BatchSize: 10})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v, want disk full", err)
	}
}

func TestReaderFactoryErrorAborts(t *testing.T) {
	files, _ := makeFiles(100)
	bad := func(context.Context) (ContentReader, error) { return nil, errors.New("git fehlt") }
	err := runWithTimeout(t, 30*time.Second, func() error {
		_, err := Run(context.Background(), memory.New(), files, newReg(t, &fakeParser{}), bad, Options{Workers: 3})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "git fehlt") {
		t.Fatalf("err = %v", err)
	}
}
