package csharp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

func parse(t *testing.T, src string) parser.Result {
	t.Helper()
	res, err := New().Parse(context.Background(), parser.Source{Path: "t.cs", Content: []byte(src)})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func findSym(t *testing.T, res parser.Result, id model.SymbolID) model.Symbol {
	t.Helper()
	for _, s := range res.Symbols {
		if s.ID == id {
			return s
		}
	}
	var ids []string
	for _, s := range res.Symbols {
		ids = append(ids, string(s.ID))
	}
	t.Fatalf("Symbol %q nicht gefunden. Vorhanden:\n%s", id, strings.Join(ids, "\n"))
	return model.Symbol{}
}

func locOf(t *testing.T, res parser.Result, id model.SymbolID) []model.Location {
	t.Helper()
	var out []model.Location
	for _, l := range res.Locations {
		if l.SymbolID == id {
			out = append(out, l)
		}
	}
	return out
}

func refsNamed(res parser.Result, kind model.RefKind, name string) []model.Reference {
	var out []model.Reference
	for _, r := range res.References {
		if r.Kind == kind && r.TargetName == name {
			out = append(out, r)
		}
	}
	return out
}

func TestMaskingIgnoresCommentsAndStrings(t *testing.T) {
	res := parse(t, `
namespace N
{
    // class Fake {}
    /* class AlsoFake { } */
    public class Real
    {
        string s = "class InString { void X() {} }";
        string v = @"class Verbatim ""quoted"" {}";
        string r = """
            class InRaw {}
            """;
        char c = '{';
    }
}
`)
	for _, s := range res.Symbols {
		if strings.Contains(s.Name, "Fake") || strings.Contains(s.Name, "InString") ||
			strings.Contains(s.Name, "Verbatim") || strings.Contains(s.Name, "InRaw") || s.Name == "AlsoFake" {
			t.Fatalf("Symbol aus Kommentar/String: %+v", s)
		}
	}
	findSym(t, res, "csharp:N.Real")
}

func TestPreprocessorDuplicatesAreMerged(t *testing.T) {
	res := parse(t, `
namespace N {
#if NET8
    public class A { public void Run() {} }
#else
    public class A { public void Run() {} }
#endif
}`)
	count := 0
	for _, s := range res.Symbols {
		if s.ID == "csharp:N.A" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Symbol A kommt %d-mal vor, want 1", count)
	}
}

func TestTypesNamespacesAndArity(t *testing.T) {
	res := parse(t, `
namespace MyApp.Services;

public interface IRepo<T> { }
public class Repo<T> : IRepo<T> { }
public record struct Point(int X, int Y);
public enum Color { Red, Green = 5, Blue }
`)
	findSym(t, res, "csharp:MyApp.Services")
	iface := findSym(t, res, "csharp:MyApp.Services.IRepo`1")
	if iface.Kind != model.KindInterface {
		t.Fatalf("IRepo Art = %s", iface.Kind)
	}
	cls := findSym(t, res, "csharp:MyApp.Services.Repo`1")
	if cls.Container != "csharp:MyApp.Services" {
		t.Fatalf("Container = %q", cls.Container)
	}
	findSym(t, res, "csharp:MyApp.Services.Point")
	color := findSym(t, res, "csharp:MyApp.Services.Color")
	if color.Kind != model.KindEnum {
		t.Fatalf("Color Art = %s", color.Kind)
	}
	findSym(t, res, "csharp:MyApp.Services.Color.Green")
	findSym(t, res, "csharp:MyApp.Services.Color.Blue")

	// Basisangaben: Klasse erster Eintrag ist Basis (HEURISTIC), IRepo ist Interface (RESOLVED)
	inh := refsNamed(res, model.RefInherits, "IRepo")
	impl := refsNamed(res, model.RefImplements, "IRepo")
	if len(inh)+len(impl) != 1 {
		t.Fatalf("Basisangabe IRepo: inherits=%d implements=%d", len(inh), len(impl))
	}
	if len(impl) == 1 && impl[0].Confidence != model.ConfResolved {
		t.Fatalf("I-Präfix sollte RESOLVED sein, ist %s", impl[0].Confidence)
	}
}

func TestMethodLinesAndOverloads(t *testing.T) {
	res := parse(t, `namespace N
{
    public class Svc
    {
        public void Run(int a)
        {
            Log(a);
        }

        public void Run(string s) => Log(s);

        private void Log(int x) { }
        private void Log(string x) { }
    }
}
`)
	run1 := findSym(t, res, "csharp:N.Svc.Run(System.Int32)")
	run2 := findSym(t, res, "csharp:N.Svc.Run(System.String)")
	if run1.Name != "Run" || run2.Name != "Run" {
		t.Fatal("Namen falsch")
	}
	l1 := locOf(t, res, run1.ID)
	if len(l1) != 1 || l1[0].Span.StartLine != 5 || l1[0].Span.EndLine != 8 {
		t.Fatalf("Run(int) Span = %+v, want Zeilen 5-8", l1)
	}
	l2 := locOf(t, res, run2.ID)
	if len(l2) != 1 || l2[0].Span.StartLine != 10 || l2[0].Span.EndLine != 10 {
		t.Fatalf("Run(string) Span = %+v, want Zeile 10", l2)
	}
	findSym(t, res, "csharp:N.Svc.Log(System.Int32)")
	findSym(t, res, "csharp:N.Svc.Log(System.String)")

	// Aufruf von Log(a) in Run(int) mit Konfidenz HEURISTIC und Enclosing = Run(int)
	calls := refsNamed(res, model.RefCall, "Log")
	found := false
	for _, c := range calls {
		if c.Enclosing == run1.ID && c.Span.StartLine == 7 {
			found = true
			if c.Confidence != model.ConfHeuristic {
				t.Fatalf("Konfidenz = %s", c.Confidence)
			}
		}
	}
	if !found {
		t.Fatalf("Aufruf Log in Run(int) fehlt: %+v", calls)
	}
}

func TestPropertiesFieldsEventsIndexer(t *testing.T) {
	res := parse(t, `namespace N {
    public class C {
        public int Count { get; set; } = 3;
        public string Name => _name;
        private int a = 1, b;
        public event EventHandler Changed;
        public int this[int i] { get { return i; } }
    }
}`)
	findSym(t, res, "csharp:N.C.Count")
	findSym(t, res, "csharp:N.C.Name")
	findSym(t, res, "csharp:N.C.a")
	findSym(t, res, "csharp:N.C.b")
	findSym(t, res, "csharp:N.C.Changed")
	idx := findSym(t, res, "csharp:N.C.this(System.Int32)")
	if idx.Kind != model.KindIndexer {
		t.Fatalf("Indexer Art = %s", idx.Kind)
	}
	if l := locOf(t, res, "csharp:N.C.b"); len(l) != 1 || l[0].Span.StartLine != 5 {
		t.Fatalf("Feld b Span = %+v", l)
	}
}

func TestConstructorAndNewAndWrites(t *testing.T) {
	res := parse(t, `namespace N {
    public class Order : Base {
        private int _n;
        public Order(int n) : base(n) {
            _n = n;
            this.Total = Compute();
        }
        public int Total;
        public int Compute() { return new Helper(_n).Value; }
    }
}`)
	ctor := findSym(t, res, "csharp:N.Order.Order(System.Int32)")
	if ctor.Kind != model.KindConstructor {
		t.Fatalf("Konstruktor Art = %s", ctor.Kind)
	}
	if len(refsNamed(res, model.RefWrite, "_n")) == 0 {
		t.Fatal("Schreibzugriff _n nicht erkannt")
	}
	if len(refsNamed(res, model.RefWrite, "Total")) == 0 {
		t.Fatal("Schreibzugriff this.Total nicht erkannt")
	}
	if len(refsNamed(res, model.RefCall, "Compute")) == 0 {
		t.Fatal("Aufruf Compute fehlt")
	}
	cons := refsNamed(res, model.RefConstruct, "Helper")
	if len(cons) != 1 || cons[0].Span.StartLine != 9 {
		t.Fatalf("new Helper: %+v", cons)
	}
}

func TestPartialRoleAndAttributes(t *testing.T) {
	res := parse(t, `namespace N {
    [Serializable]
    [Obsolete("x"), Route("a", Name = "b")]
    public partial class P
    {
        [Required]
        public int Id { get; set; }
    }
}`)
	p := findSym(t, res, "csharp:N.P")
	locs := locOf(t, res, p.ID)
	if len(locs) != 1 || locs[0].Role != model.RolePartial {
		t.Fatalf("partial-Rolle fehlt: %+v", locs)
	}
	if len(refsNamed(res, model.RefAttribute, "Serializable")) != 1 ||
		len(refsNamed(res, model.RefAttribute, "Obsolete")) != 1 ||
		len(refsNamed(res, model.RefAttribute, "Route")) != 1 {
		t.Fatalf("Attribute fehlen: %+v", res.References)
	}
	if len(refsNamed(res, model.RefAttribute, "Required")) != 1 {
		t.Fatal("Attribut am Property fehlt")
	}
}

func TestXMLDocumentation(t *testing.T) {
	res := parse(t, `namespace N {
    /// <summary>
    /// Verarbeitet <see cref="N.Order"/> Bestellungen.
    /// </summary>
    /// <param name="id">Die <c>ID</c>.</param>
    /// <returns>Ergebnis.</returns>
    /// <remarks>Hinweis.</remarks>
    /// <seealso cref="M:N.Other.Run(System.Int32)"/>
    public class Handler
    {
        /// <summary>Startet.</summary>
        public void Start(int id) { }
    }
}`)
	h := findSym(t, res, "csharp:N.Handler")
	if h.Summary != "Verarbeitet N.Order Bestellungen." {
		t.Fatalf("Summary = %q", h.Summary)
	}
	if h.Remarks != "Hinweis." {
		t.Fatalf("Remarks = %q", h.Remarks)
	}
	if len(refsNamed(res, model.RefCref, "N.Order")) != 1 {
		t.Fatal("cref N.Order fehlt")
	}
	if len(refsNamed(res, model.RefCref, "N.Other.Run(System.Int32)")) != 1 {
		t.Fatalf("seealso-cref ohne Präfix fehlt: %+v", res.References)
	}
	s := findSym(t, res, "csharp:N.Handler.Start(System.Int32)")
	if s.Summary != "Startet." {
		t.Fatalf("Methoden-Summary = %q", s.Summary)
	}
	h2 := findSym(t, res, "csharp:N.Handler")
	_ = h2
}

func TestParamAndReturnDocs(t *testing.T) {
	res := parse(t, `namespace N {
    public class H {
        /// <summary>S</summary>
        /// <param name="id">Die ID.</param>
        /// <returns>Der Wert.</returns>
        public int Get(int id) => id;
    }
}`)
	s := findSym(t, res, "csharp:N.H.Get(System.Int32)")
	if s.Returns != "Der Wert." {
		t.Fatalf("Returns = %q", s.Returns)
	}
	if len(s.ParamDocs) != 1 || s.ParamDocs[0].Name != "id" || s.ParamDocs[0].Text != "Die ID." {
		t.Fatalf("ParamDocs = %+v", s.ParamDocs)
	}
}

func TestUsingsAndTypeRefs(t *testing.T) {
	res := parse(t, `using System;
using static System.Math;
using Alias = System.Text.StringBuilder;
namespace N {
    public class S {
        public Dictionary<string, Order> Map(IList<Item>? items, int n) { return null; }
    }
}`)
	raws := map[string]bool{}
	for _, im := range res.Imports {
		raws[im.Raw] = true
	}
	for _, want := range []string{"System", "static System.Math", "Alias=System.Text.StringBuilder"} {
		if !raws[want] {
			t.Errorf("Import %q fehlt, vorhanden: %v", want, raws)
		}
	}
	for _, n := range []string{"Dictionary", "Order", "IList", "Item"} {
		if len(refsNamed(res, model.RefTypeRef, n)) == 0 {
			t.Errorf("Typverweis %s fehlt", n)
		}
	}
	if len(refsNamed(res, model.RefTypeRef, "string")) != 0 {
		t.Error("Schlüsselwort-Typ string darf kein Verweis sein")
	}
}

func TestNoPanicOnBrokenCode(t *testing.T) {
	inputs := []string{
		"class",
		"namespace",
		"public class A { public void M( { ",
		"class A { int = ; } }",
		"[",
		"class A : { }",
		"}}}}",
	}
	for _, in := range inputs {
		_ = parse(t, in)
	}
}

// TestCorpusSpotChecks prüft die Stichproben aus P0 gegen MediatR, falls READMENATOR_CORPUS gesetzt ist.
func TestCorpusSpotChecks(t *testing.T) {
	dir := os.Getenv("READMENATOR_CORPUS")
	if dir == "" {
		t.Skip("READMENATOR_CORPUS nicht gesetzt")
	}
	cases := []struct {
		file, name string
		line       int
	}{
		{"samples/MediatR.Examples/PingedHandler.cs", "ConstrainedPingedHandler", 43},
		{"samples/MediatR.Examples.Windsor/Program.cs", "BuildMediator", 26},
		{"test/MediatR.Tests/MicrosoftExtensionsDI/PipeLineMultiCallToConstructorTest.cs", "ConstructorTestHandler", 57},
		{"test/MediatR.Tests/PipelineTests.cs", "Handle", 45},
		{"samples/MediatR.Examples.PublishStrategies/Publisher.cs", "SyncContinueOnException", 131},
	}
	for _, c := range cases {
		data, err := os.ReadFile(filepath.Join(dir, c.file))
		if err != nil {
			t.Fatal(err)
		}
		res := parseFile(data)
		hit := false
		for _, s := range res.Symbols {
			if s.Name != c.name {
				continue
			}
			for _, l := range locOf(t, res, s.ID) {
				if l.Span.StartLine == c.line {
					hit = true
				}
			}
		}
		if !hit {
			t.Errorf("%s: %s nicht bei Zeile %d gefunden", c.file, c.name, c.line)
		}
	}
}
