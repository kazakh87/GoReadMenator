package xmldoc

import (
	"strings"
	"testing"

	"goreadmenator/internal/model"
)

const sample = `<?xml version="1.0"?>
<doc>
  <assembly><name>Shop.Core</name></assembly>
  <members>
    <member name="T:Shop.Orders.Order">
      <summary>Eine <see cref="T:Shop.Orders.Line"/> Bestellung.</summary>
      <remarks>Nur &lt;gültig&gt;.</remarks>
    </member>
    <member name="T:Shop.Orders.Order.Line">
      <summary>Zeile.</summary>
    </member>
    <member name="T:Shop.Pair§2">
      <summary>Paar.</summary>
      <typeparam name="A">Erster.</typeparam>
    </member>
    <member name="M:Shop.Orders.Order.Submit(System.Int32,System.String)">
      <summary>Sendet ab.</summary>
      <param name="id">Die ID.</param>
      <returns>Ergebnis.</returns>
    </member>
    <member name="M:Shop.Orders.Order.Reset">
      <summary>Leert.</summary>
    </member>
    <member name="M:Shop.Orders.Order.#ctor(System.Collections.Generic.List{System.String})">
      <summary>Konstruktor.</summary>
    </member>
    <member name="P:Shop.Orders.Order.Total">
      <summary>Summe.</summary>
    </member>
    <member name="P:Shop.Orders.Order.Item(System.Int32)">
      <summary>Indexer.</summary>
    </member>
    <member name="F:Shop.Orders.Order.MaxLines">
      <summary>Grenze.</summary>
    </member>
    <member name="E:Shop.Orders.Order.Changed">
      <summary>Ereignis.</summary>
    </member>
    <member name="N:Shop.Orders">
      <summary>ignoriert</summary>
    </member>
  </members>
</doc>`

func parseSample(t *testing.T) Doc {
	t.Helper()
	doc, err := Parse(strings.NewReader(strings.ReplaceAll(sample, "§", "`")))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func byID(syms []model.Symbol) map[model.SymbolID]model.Symbol {
	m := map[model.SymbolID]model.Symbol{}
	for _, s := range syms {
		m[s.ID] = s
	}
	return m
}

func TestParseAssemblyAndMembers(t *testing.T) {
	doc := parseSample(t)
	if doc.Assembly != "Shop.Core" {
		t.Fatalf("Assembly = %q", doc.Assembly)
	}
	if len(doc.Members) != 11 {
		t.Fatalf("Members = %d", len(doc.Members))
	}
}

func TestSymbolIDsMatchSourceScheme(t *testing.T) {
	syms := byID(Symbols(parseSample(t)))
	want := map[model.SymbolID]model.SymbolKind{
		"csharp:Shop.Orders.Order":                                    model.KindTypeUnknown,
		"csharp:Shop.Orders.Order.Line":                               model.KindTypeUnknown,
		"csharp:Shop.Pair`2":                                          model.KindTypeUnknown,
		"csharp:Shop.Orders.Order.Submit(System.Int32,System.String)": model.KindMethod,
		"csharp:Shop.Orders.Order.Reset()":                            model.KindMethod,
		"csharp:Shop.Orders.Order.Order(System.Collections.Generic.List<System.String>)": model.KindConstructor,
		"csharp:Shop.Orders.Order.Total":              model.KindProperty,
		"csharp:Shop.Orders.Order.this(System.Int32)": model.KindIndexer,
		"csharp:Shop.Orders.Order.MaxLines":           model.KindField,
		"csharp:Shop.Orders.Order.Changed":            model.KindEvent,
		"csharp:Shop.Orders":                          model.KindNamespace,
	}
	for id, kind := range want {
		s, ok := syms[id]
		if !ok {
			var have []string
			for k := range syms {
				have = append(have, string(k))
			}
			t.Errorf("Symbol %s fehlt. Vorhanden: %v", id, have)
			continue
		}
		if s.Kind != kind {
			t.Errorf("%s: Art %s, want %s", id, s.Kind, kind)
		}
	}
	if _, ok := syms["csharp:Shop.Orders"]; !ok {
		t.Fatal("Namensraum muss aus den Typen entstehen")
	}
}

func TestContainersAndNestedTypes(t *testing.T) {
	syms := byID(Symbols(parseSample(t)))
	line := syms["csharp:Shop.Orders.Order.Line"]
	if line.Container != "csharp:Shop.Orders.Order" {
		t.Errorf("verschachtelter Typ: Container = %q", line.Container)
	}
	submit := syms["csharp:Shop.Orders.Order.Submit(System.Int32,System.String)"]
	if submit.Container != "csharp:Shop.Orders.Order" {
		t.Errorf("Member: Container = %q", submit.Container)
	}
	order := syms["csharp:Shop.Orders.Order"]
	if order.Container != "csharp:Shop.Orders" {
		t.Errorf("Top-Level-Typ: Container = %q", order.Container)
	}
}

func TestDocumentationAndCleaning(t *testing.T) {
	doc := parseSample(t)
	syms := byID(Symbols(doc))
	order := syms["csharp:Shop.Orders.Order"]
	if order.Summary != "Eine Shop.Orders.Line Bestellung." {
		t.Errorf("Summary = %q", order.Summary)
	}
	if order.Remarks != "Nur <gültig>." {
		t.Errorf("Remarks mit Entities = %q", order.Remarks)
	}
	if order.DocSource != "xml" {
		t.Errorf("DocSource = %q", order.DocSource)
	}
	submit := syms["csharp:Shop.Orders.Order.Submit(System.Int32,System.String)"]
	if submit.Returns != "Ergebnis." || len(submit.ParamDocs) != 1 || submit.ParamDocs[0].Name != "id" {
		t.Errorf("Parameter/Returns: %+v", submit)
	}
	var crefs []string
	for _, m := range doc.Members {
		if m.Name == "T:Shop.Orders.Order" {
			crefs = m.Crefs
		}
	}
	if len(crefs) != 1 || crefs[0] != "Shop.Orders.Line" {
		t.Errorf("Crefs = %v", crefs)
	}
}

func TestSplitParamsHandlesGenericsAndArrays(t *testing.T) {
	got := splitParams("System.Collections.Generic.Dictionary{System.String,System.Int32},System.Int32[0:,0:]")
	want := []string{"System.Collections.Generic.Dictionary<System.String,System.Int32>", "System.Int32[]"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("splitParams = %v", got)
	}
}
