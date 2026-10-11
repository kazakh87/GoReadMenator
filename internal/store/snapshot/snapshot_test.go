package snapshot

import (
	"context"
	"path/filepath"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
	"goreadmenator/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.Store {
		s, err := store.Open(context.Background(), "snapshot", filepath.Join(t.TempDir(), "x.snap"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestPersistsAcrossOpen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p.snap")
	s, err := store.Open(ctx, "snapshot", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	id, _ := s.PutFile(ctx, model.File{Path: "a.cs", Lang: model.LangCSharp})
	if err := s.WriteBatch(ctx, store.Batch{
		Symbols:    []model.Symbol{{ID: "csharp:A", Kind: model.KindClass, Name: "A", FQN: "A"}},
		Locations:  []model.Location{{SymbolID: "csharp:A", FileID: id}},
		References: []model.Reference{{FileID: id, TargetName: "A", Kind: model.RefTypeRef}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := store.Open(ctx, "snapshot", path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if v, ok, _ := s2.Meta(ctx, "k"); !ok || v != "v" {
		t.Fatalf("Meta nicht persistiert: %q %v", v, ok)
	}
	if _, ok, _ := s2.Symbol(ctx, "csharp:A"); !ok {
		t.Fatal("Symbol nicht persistiert")
	}
	refs, _ := s2.AllReferences(ctx)
	if len(refs) != 1 || refs[0].ID == 0 {
		t.Fatalf("Verwendungen = %+v", refs)
	}
	f, ok, _ := s2.FileByPath(ctx, "a.cs")
	if !ok || f.ID != id {
		t.Fatalf("Datei nicht persistiert: %+v", f)
	}
	// Die ID-Zähler müssen weiterlaufen.
	nid, _ := s2.PutFile(ctx, model.File{Path: "b.cs"})
	if nid <= id {
		t.Fatalf("Datei-ID wiederverwendet: %d <= %d", nid, id)
	}
	_ = memory.New
}
