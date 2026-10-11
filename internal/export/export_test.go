package export

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
)

// sampleStore baut einen kleinen Graphen: Basis, abgeleitete Klasse, Interface, Methode mit Doku.
func sampleStore(t *testing.T) store.Store {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	fid, _ := st.PutFile(ctx, model.File{Path: "src/a.cs", Lang: model.LangCSharp})
	base := model.Symbol{ID: "csharp:N.Base", Kind: model.KindClass, Name: "Base", FQN: "N.Base"}
	der := model.Symbol{ID: "csharp:N.Der", Kind: model.KindClass, Name: "Der", FQN: "N.Der", Container: "csharp:N"}
	iface := model.Symbol{ID: "csharp:N.IFoo", Kind: model.KindInterface, Name: "IFoo", FQN: "N.IFoo"}
	m := model.Symbol{ID: "csharp:N.Der.Run()", Kind: model.KindMethod, Name: "Run", FQN: "N.Der.Run",
		Container: "csharp:N.Der", Signature: "public void Run()", Summary: "Startet.", Returns: "nichts"}
	if err := st.WriteBatch(ctx, store.Batch{
		Symbols: []model.Symbol{base, der, iface, m},
		Locations: []model.Location{
			{SymbolID: base.ID, FileID: fid, Span: model.Span{StartLine: 1, EndLine: 2}, Role: model.RoleDecl},
			{SymbolID: der.ID, FileID: fid, Span: model.Span{StartLine: 3, EndLine: 9}, Role: model.RoleDecl},
			{SymbolID: iface.ID, FileID: fid, Span: model.Span{StartLine: 10, EndLine: 11}, Role: model.RoleDecl},
			{SymbolID: m.ID, FileID: fid, Span: model.Span{StartLine: 4, EndLine: 5}, Role: model.RoleDecl},
		},
		References: []model.Reference{{FileID: fid, TargetID: base.ID, TargetName: "Base", Kind: model.RefCall, Enclosing: m.ID, Confidence: model.ConfExact}},
		Relations: []model.Relation{
			{FileID: fid, From: der.ID, To: base.ID, Kind: model.RelInherits},
			{FileID: fid, From: der.ID, To: iface.ID, Kind: model.RelImplements},
			{FileID: fid, From: "csharp:N", To: der.ID, Kind: model.RelContains},
			{FileID: fid, From: der.ID, To: m.ID, Kind: model.RelContains},
		},
		Imports: []model.Import{{FileID: fid, Raw: "System", Kind: model.ImportUsing}},
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestJSONIsValidAndComplete(t *testing.T) {
	st := sampleStore(t)
	g, err := Load(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, g); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Meta      struct{ Format string } `json:"meta"`
		Files     []json.RawMessage       `json:"files"`
		Symbols   []json.RawMessage       `json:"symbols"`
		Locations []json.RawMessage       `json:"locations"`
		Refs      []json.RawMessage       `json:"references"`
		Relations []json.RawMessage       `json:"relations"`
		Imports   []json.RawMessage       `json:"imports"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON ungültig: %v\n%s", err, buf.String())
	}
	if doc.Meta.Format != "goreadmenator-graph" || len(doc.Symbols) != 4 || len(doc.Relations) != 4 ||
		len(doc.Refs) != 1 || len(doc.Imports) != 1 || len(doc.Files) != 1 || len(doc.Locations) != 4 {
		t.Fatalf("Inhalt unvollständig: %+v", doc)
	}
}

func TestJSONLinesOneRecordPerLine(t *testing.T) {
	g, _ := Load(context.Background(), sampleStore(t))
	var buf bytes.Buffer
	if err := WriteJSONL(&buf, g); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	want := len(g.Files) + len(g.Symbols) + len(g.Locations) + len(g.Refs) + len(g.Relations) + len(g.Imports)
	if len(lines) != want {
		t.Fatalf("Zeilen = %d, want %d", len(lines), want)
	}
	for _, l := range lines {
		var rec struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(l), &rec); err != nil || rec.Type == "" {
			t.Fatalf("ungültige Zeile %q: %v", l, err)
		}
	}
}

func TestMermaidShowsInheritanceAndScope(t *testing.T) {
	g, _ := Load(context.Background(), sampleStore(t))
	var buf bytes.Buffer
	if err := WriteMermaid(&buf, g, MermaidOptions{Members: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"classDiagram", `["Base"]`, "<|--", "<|..", "Run"} {
		if !strings.Contains(out, want) {
			t.Errorf("Mermaid fehlt %q:\n%s", want, out)
		}
	}
	// Basis zeigt auf die Ableitung mit dem Pfeil <|-- (Base <|-- Der)
	if !strings.Contains(out, "<|--") {
		t.Fatal("Vererbungspfeil fehlt")
	}
	var scoped bytes.Buffer
	if err := WriteMermaid(&scoped, g, MermaidOptions{Scope: "N.IFoo"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(scoped.String(), "Base") {
		t.Fatalf("Scope muss andere Typen ausblenden:\n%s", scoped.String())
	}
}

func TestMarkdownPerFile(t *testing.T) {
	g, _ := Load(context.Background(), sampleStore(t))
	dir := t.TempDir()
	written, err := WriteMarkdown(dir, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("Seiten = %d", len(written))
	}
	data, err := os.ReadFile(filepath.Join(dir, "src__a.cs.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"# src/a.cs", "N.Der.Run", "public void Run()", "Startet.", "Rückgabe", "eingehende Verwendung"} {
		if !strings.Contains(text, want) {
			t.Errorf("Markdown fehlt %q:\n%s", want, text)
		}
	}
}
