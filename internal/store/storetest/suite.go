// Package storetest enthält die Vertragstests für store.Store. Jedes Backend
// ruft Run mit seiner Factory auf. Damit ist sichergestellt, dass ein neues
// Backend (etwa SQLite) sich exakt wie das Referenz-Backend verhält.
package storetest

import (
	"context"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

// Run führt alle Vertragstests gegen die von newStore gelieferte Implementierung aus.
func Run(t *testing.T, newStore func(t *testing.T) store.Store) {
	t.Helper()
	cases := map[string]func(*testing.T, store.Store){
		"MetaRoundtrip":             testMeta,
		"PutFileUpsertsByPath":      testPutFileUpsert,
		"SymbolMergeKeepsExisting":  testSymbolMerge,
		"PartialAcrossFiles":        testPartial,
		"DeleteFileCascades":        testDeleteCascade,
		"ReferencesByTargetAndName": testReferences,
		"RelationsBothDirections":   testRelations,
		"LocationForUnknownSymbol":  testUnknownSymbol,
	}
	for name, fn := range cases {
		fn := fn
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			defer s.Close()
			fn(t, s)
		})
	}
}

func ctx() context.Context { return context.Background() }

func testMeta(t *testing.T, s store.Store) {
	if _, ok, _ := s.Meta(ctx(), "commit"); ok {
		t.Fatal("fehlender Schlüssel darf nicht ok sein")
	}
	if err := s.SetMeta(ctx(), "commit", "abc"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := s.Meta(ctx(), "commit")
	if err != nil || !ok || v != "abc" {
		t.Fatalf("Meta = %q,%v,%v", v, ok, err)
	}
}

func testPutFileUpsert(t *testing.T, s store.Store) {
	id1, err := s.PutFile(ctx(), model.File{Path: "a.cs", Lang: model.LangCSharp, BlobSHA: "1"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.PutFile(ctx(), model.File{Path: "a.cs", Lang: model.LangCSharp, BlobSHA: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("Upsert vergab neue ID: %d vs %d", id1, id2)
	}
	f, ok, _ := s.FileByPath(ctx(), "a.cs")
	if !ok || f.BlobSHA != "2" {
		t.Fatalf("FileByPath = %+v, %v", f, ok)
	}
}

func testSymbolMerge(t *testing.T, s store.Store) {
	fid := mustFile(t, s, "a.cs")
	id := model.SymbolID("csharp:A.B")
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols:   []model.Symbol{{ID: id, Kind: model.KindClass, Name: "B", Summary: "erste"}},
		Locations: []model.Location{{SymbolID: id, FileID: fid, Role: model.RoleDecl}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols: []model.Symbol{{ID: id, Kind: model.KindClass, Name: "B", Summary: "zweite", Remarks: "neu"}},
	}); err != nil {
		t.Fatal(err)
	}
	sym, ok, _ := s.Symbol(ctx(), id)
	if !ok {
		t.Fatal("Symbol fehlt")
	}
	if sym.Summary != "erste" {
		t.Fatalf("bestehendes Feld wurde überschrieben: %q", sym.Summary)
	}
	if sym.Remarks != "neu" {
		t.Fatalf("leeres Feld wurde nicht ergänzt: %q", sym.Remarks)
	}
}

func testPartial(t *testing.T, s store.Store) {
	f1 := mustFile(t, s, "A.Part1.cs")
	f2 := mustFile(t, s, "A.Part2.cs")
	id := model.SymbolID("csharp:N.A")
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols: []model.Symbol{{ID: id, Kind: model.KindClass, Name: "A"}},
		Locations: []model.Location{
			{SymbolID: id, FileID: f1, Role: model.RoleDecl, Span: model.Span{StartLine: 3, EndLine: 10}},
			{SymbolID: id, FileID: f2, Role: model.RolePartial, Span: model.Span{StartLine: 1, EndLine: 5}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	locs, err := s.Locations(ctx(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 2 {
		t.Fatalf("erwartet 2 Fundorte, bekommen %d", len(locs))
	}
}

func testDeleteCascade(t *testing.T, s store.Store) {
	f1 := mustFile(t, s, "one.cs")
	f2 := mustFile(t, s, "two.cs")
	solo := model.SymbolID("csharp:N.Solo")
	shared := model.SymbolID("csharp:N.Shared")
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols: []model.Symbol{
			{ID: solo, Kind: model.KindClass, Name: "Solo"},
			{ID: shared, Kind: model.KindClass, Name: "Shared"},
		},
		Locations: []model.Location{
			{SymbolID: solo, FileID: f1, Role: model.RoleDecl},
			{SymbolID: shared, FileID: f1, Role: model.RoleDecl},
			{SymbolID: shared, FileID: f2, Role: model.RolePartial},
		},
		References: []model.Reference{{FileID: f1, TargetID: solo, TargetName: "Solo", Kind: model.RefTypeRef, Confidence: model.ConfExact}},
		Imports:    []model.Import{{FileID: f1, Raw: "x", Kind: model.ImportUsing}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFile(ctx(), f1); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Symbol(ctx(), solo); ok {
		t.Fatal("Symbol ohne Fundorte muss entfernt werden")
	}
	if _, ok, _ := s.Symbol(ctx(), shared); !ok {
		t.Fatal("Partial-Symbol mit verbleibendem Fundort darf nicht verschwinden")
	}
	locs, _ := s.Locations(ctx(), shared)
	if len(locs) != 1 || locs[0].FileID != f2 {
		t.Fatalf("Fundorte nach Delete = %+v", locs)
	}
	refs, _ := s.ReferencesByName(ctx(), "Solo")
	if len(refs) != 0 {
		t.Fatalf("Verwendungen der gelöschten Datei bleiben: %+v", refs)
	}
	if _, ok, _ := s.FileByPath(ctx(), "one.cs"); ok {
		t.Fatal("Datei muss entfernt sein")
	}
}

func testReferences(t *testing.T, s store.Store) {
	fid := mustFile(t, s, "use.cs")
	target := model.SymbolID("csharp:N.Target")
	enc := model.SymbolID("csharp:N.Caller.Run()")
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols:   []model.Symbol{{ID: target, Kind: model.KindClass, Name: "Target"}},
		Locations: []model.Location{{SymbolID: target, FileID: fid, Role: model.RoleDecl}},
		References: []model.Reference{
			{FileID: fid, TargetID: target, TargetName: "Target", Kind: model.RefConstruct,
				Span: model.Span{StartLine: 7, StartCol: 9, EndLine: 7, EndCol: 15}, Enclosing: enc, Confidence: model.ConfExact},
			{FileID: fid, TargetName: "Target", Kind: model.RefCall, Confidence: model.ConfHeuristic},
		},
	}); err != nil {
		t.Fatal(err)
	}
	byID, _ := s.ReferencesTo(ctx(), target)
	if len(byID) != 1 || byID[0].Span.StartLine != 7 || byID[0].Enclosing != enc {
		t.Fatalf("ReferencesTo = %+v", byID)
	}
	byName, _ := s.ReferencesByName(ctx(), "Target")
	if len(byName) != 2 {
		t.Fatalf("ReferencesByName erwartet 2, bekommen %d", len(byName))
	}
}

func testRelations(t *testing.T, s store.Store) {
	fid := mustFile(t, s, "rel.cs")
	cls := model.SymbolID("csharp:N.Impl")
	iface := model.SymbolID("csharp:N.IFoo")
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols: []model.Symbol{
			{ID: cls, Kind: model.KindClass, Name: "Impl"},
			{ID: iface, Kind: model.KindInterface, Name: "IFoo"},
		},
		Locations: []model.Location{
			{SymbolID: cls, FileID: fid, Role: model.RoleDecl},
			{SymbolID: iface, FileID: fid, Role: model.RoleDecl},
		},
		Relations: []model.Relation{{FileID: fid, From: cls, To: iface, Kind: model.RelImplements}},
	}); err != nil {
		t.Fatal(err)
	}
	fromCls, _ := s.Relations(ctx(), cls)
	toIface, _ := s.Relations(ctx(), iface)
	if len(fromCls) != 1 || len(toIface) != 1 {
		t.Fatalf("Beziehung in beide Richtungen: from=%d to=%d", len(fromCls), len(toIface))
	}
}

func testUnknownSymbol(t *testing.T, s store.Store) {
	fid := mustFile(t, s, "x.cs")
	err := s.WriteBatch(ctx(), store.Batch{
		Locations: []model.Location{{SymbolID: "csharp:Ghost", FileID: fid}},
	})
	if err == nil {
		t.Fatal("Fundort für unbekanntes Symbol muss einen Fehler liefern")
	}
}

func mustFile(t *testing.T, s store.Store, path string) model.FileID {
	t.Helper()
	id, err := s.PutFile(ctx(), model.File{Path: path, Lang: model.LangCSharp})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testAllAndUpdate(t *testing.T, s store.Store) {
	fid := mustFile(t, s, "u.cs")
	target := model.SymbolID("csharp:N.T")
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols:    []model.Symbol{{ID: target, Kind: model.KindClass, Name: "T", FQN: "N.T"}},
		Locations:  []model.Location{{SymbolID: target, FileID: fid, Role: model.RoleDecl}},
		References: []model.Reference{{FileID: fid, TargetName: "T", Kind: model.RefTypeRef, Confidence: model.ConfHeuristic}},
		Imports:    []model.Import{{FileID: fid, Raw: "./x", Kind: model.ImportESM, Bindings: []model.ImportBinding{{Local: "T", Imported: "T"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	syms, err := s.AllSymbols(ctx())
	if err != nil || len(syms) != 1 {
		t.Fatalf("AllSymbols = %d (%v)", len(syms), err)
	}
	refs, _ := s.AllReferences(ctx())
	if len(refs) != 1 || refs[0].ID == 0 {
		t.Fatalf("AllReferences ohne ID: %+v", refs)
	}
	refs[0].TargetID = target
	refs[0].Confidence = model.ConfExact
	if err := s.UpdateReferences(ctx(), refs); err != nil {
		t.Fatal(err)
	}
	again, _ := s.ReferencesTo(ctx(), target)
	if len(again) != 1 || again[0].Confidence != model.ConfExact {
		t.Fatalf("Update nicht persistiert: %+v", again)
	}
	imps, _ := s.AllImports(ctx())
	if len(imps) != 1 || imps[0].ID == 0 || len(imps[0].Bindings) != 1 {
		t.Fatalf("AllImports = %+v", imps)
	}
	imps[0].ResolvedID = fid
	if err := s.UpdateImports(ctx(), imps); err != nil {
		t.Fatal(err)
	}
	imps2, _ := s.AllImports(ctx())
	if imps2[0].ResolvedID != fid {
		t.Fatalf("UpdateImports nicht persistiert: %+v", imps2[0])
	}
}

func testDerivedRelations(t *testing.T, s store.Store) {
	fid := mustFile(t, s, "d.cs")
	cls := model.SymbolID("csharp:N.C")
	base := model.SymbolID("csharp:N.B")
	if err := s.WriteBatch(ctx(), store.Batch{
		Symbols:   []model.Symbol{{ID: cls, Kind: model.KindClass, Name: "C"}, {ID: base, Kind: model.KindClass, Name: "B"}},
		Locations: []model.Location{{SymbolID: cls, FileID: fid}, {SymbolID: base, FileID: fid}},
		Relations: []model.Relation{{FileID: fid, From: cls, To: base, Kind: model.RelContains}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDerivedRelations(ctx(), []model.Relation{{FileID: fid, From: cls, To: base, Kind: model.RelInherits}}); err != nil {
		t.Fatal(err)
	}
	// Ersetzen darf die geparste Beziehung nicht entfernen.
	if err := s.SetDerivedRelations(ctx(), nil); err != nil {
		t.Fatal(err)
	}
	all, _ := s.AllRelations(ctx())
	if len(all) != 1 || all[0].Kind != model.RelContains || all[0].Derived {
		t.Fatalf("nach Ersetzen: %+v", all)
	}
	if err := s.SetDerivedRelations(ctx(), []model.Relation{{FileID: fid, From: cls, To: base, Kind: model.RelInherits}}); err != nil {
		t.Fatal(err)
	}
	rels, _ := s.Relations(ctx(), cls)
	if len(rels) != 2 {
		t.Fatalf("Relations(C) = %d, want 2 (geparst + abgeleitet)", len(rels))
	}
	if err := s.DeleteFile(ctx(), fid); err != nil {
		t.Fatal(err)
	}
	all, _ = s.AllRelations(ctx())
	if len(all) != 0 {
		t.Fatalf("Löschen der Datei muss abgeleitete Beziehungen entfernen: %+v", all)
	}
}
