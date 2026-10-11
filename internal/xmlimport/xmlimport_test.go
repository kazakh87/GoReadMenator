package xmlimport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
)

const xmlDoc = "<doc><assembly><name>Shop.Core</name></assembly><members>" +
	`<member name="T:Shop.Order"><summary>Bestellung.</summary></member>` +
	`<member name="M:Shop.Order.Save(System.Int32)"><summary>Speichert.</summary><param name="n">Anzahl.</param></member>` +
	`<member name="M:Shop.Order.Load"><summary>Lädt <see cref="T:Shop.Order"/>.</summary></member>` +
	"</members></doc>"

func writeXML(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "Shop.Core.xml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportWritesSymbolsAndVirtualAssembly(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	res, err := Import(ctx, st, writeXML(t, xmlDoc))
	if err != nil {
		t.Fatal(err)
	}
	if res.Assembly != "Shop.Core" || res.Path != "bin/Shop.Core.dll" || res.Members != 3 {
		t.Fatalf("Result = %+v", res)
	}
	s, ok, _ := st.Symbol(ctx, "csharp:Shop.Order.Save(System.Int32)")
	if !ok || s.Summary != "Speichert." || s.DocSource != "xml" {
		t.Fatalf("Symbol: %+v", s)
	}
	f, ok, _ := st.FileByPath(ctx, "bin/Shop.Core.dll")
	if !ok || f.BlobSHA != Prefix+"Shop.Core" {
		t.Fatalf("virtuelle Datei fehlt: %+v", f)
	}
	locs, _ := st.Locations(ctx, s.ID)
	if len(locs) != 1 || locs[0].FileID != f.ID {
		t.Fatalf("Fundort zeigt nicht auf das Assembly: %+v", locs)
	}
	refs, _ := st.AllReferences(ctx)
	if len(refs) != 1 || refs[0].Kind != model.RefCref || refs[0].TargetName != "Shop.Order" {
		t.Fatalf("cref-Verweis: %+v", refs)
	}
}

func TestReimportReplacesItsOwnStateOnly(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	if _, err := Import(ctx, st, writeXML(t, xmlDoc)); err != nil {
		t.Fatal(err)
	}
	smaller := "<doc><assembly><name>Shop.Core</name></assembly><members>" +
		`<member name="T:Shop.Order"><summary>Neu.</summary></member></members></doc>`
	res, err := Import(ctx, st, writeXML(t, smaller))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replaced {
		t.Fatal("zweiter Import muss den ersten ersetzen")
	}
	if _, ok, _ := st.Symbol(ctx, "csharp:Shop.Order.Save(System.Int32)"); ok {
		t.Fatal("Member aus dem alten Import müssen verschwinden")
	}
	if s, _, _ := st.Symbol(ctx, "csharp:Shop.Order"); s.Summary != "Neu." {
		t.Fatalf("Summary nach Reimport = %q", s.Summary)
	}
}

func TestMergeWithSourceKeepsSourceFields(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	fid, _ := st.PutFile(ctx, model.File{Path: "src/Order.cs", Lang: model.LangCSharp})
	st.WriteBatch(ctx, storeBatch(model.Symbol{ID: "csharp:Shop.Order", Kind: model.KindClass, Name: "Order",
		FQN: "Shop.Order", Summary: "Aus dem Quelltext.", DocSource: "inline"}, fid))
	if _, err := Import(ctx, st, writeXML(t, xmlDoc)); err != nil {
		t.Fatal(err)
	}
	s, _, _ := st.Symbol(ctx, "csharp:Shop.Order")
	if s.Summary != "Aus dem Quelltext." {
		t.Fatalf("Quelltext-Doku darf nicht überschrieben werden: %q", s.Summary)
	}
	if s.Kind != model.KindClass {
		t.Fatalf("Art darf durch Import nicht verloren gehen: %s", s.Kind)
	}
	locs, _ := st.Locations(ctx, "csharp:Shop.Order")
	if len(locs) != 2 {
		t.Fatalf("Fundorte = %d, want Quelltext und Assembly", len(locs))
	}
}

func storeBatch(s model.Symbol, fid model.FileID) store.Batch {
	return store.Batch{
		Symbols:   []model.Symbol{s},
		Locations: []model.Location{{SymbolID: s.ID, FileID: fid, Role: model.RoleDecl}},
	}
}
