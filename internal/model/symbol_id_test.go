package model

import "testing"

func TestTypeIDHasNoParentheses(t *testing.T) {
	got := NewSymbolID(LangCSharp, "MyApp.Services.OrderService", nil)
	want := SymbolID("csharp:MyApp.Services.OrderService")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMethodIDIncludesParameterTypes(t *testing.T) {
	got := NewSymbolID(LangCSharp, "MyApp.OrderService.Submit", []string{"int", "string"})
	want := SymbolID("csharp:MyApp.OrderService.Submit(System.Int32,System.String)")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEmptyParameterListDiffersFromNil(t *testing.T) {
	none := NewSymbolID(LangCSharp, "A.B", nil)
	empty := NewSymbolID(LangCSharp, "A.B", []string{})
	if none == empty {
		t.Fatalf("nil and empty parameter list must differ, both %q", none)
	}
	if empty != "csharp:A.B()" {
		t.Fatalf("empty list ID = %q", empty)
	}
}

func TestOverloadsGetDistinctIDs(t *testing.T) {
	a := NewSymbolID(LangCSharp, "A.Run", []string{"int"})
	b := NewSymbolID(LangCSharp, "A.Run", []string{"string"})
	c := NewSymbolID(LangCSharp, "A.Run", []string{"int", "int"})
	if a == b || a == c || b == c {
		t.Fatalf("overloads collide: %q %q %q", a, b, c)
	}
}

func TestKeywordAndSystemTypeGiveSameID(t *testing.T) {
	kw := NewSymbolID(LangCSharp, "A.Run", []string{"int"})
	sys := NewSymbolID(LangCSharp, "A.Run", []string{"System.Int32"})
	if kw != sys {
		t.Fatalf("alias mismatch: %q vs %q", kw, sys)
	}
}

func TestNormalizeTypeKeepsGenericsArraysAndNullable(t *testing.T) {
	cases := map[string]string{
		"List< int >":             "List<int>",       // Generics bleiben, Leerzeichen weg
		"int[]":                   "System.Int32[]",  // Array-Suffix bleibt
		"string?":                 "System.String?",  // Nullable-Suffix bleibt
		"int[]?":                  "System.Int32[]?", // kombiniert
		"Dictionary<string, int>": "Dictionary<string,int>",
	}
	for in, want := range cases {
		if got := NormalizeType(LangCSharp, in); got != want {
			t.Errorf("NormalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTSJSNormalizationOnlyStripsWhitespace(t *testing.T) {
	if got := NormalizeType(LangTSJS, "number []"); got != "number[]" {
		t.Fatalf("got %q", got)
	}
}

func TestSameTypeIDIndependentOfFile(t *testing.T) {
	// Partial-Klassen: die ID hängt nur am FQN, nicht am Fundort.
	a := NewSymbolID(LangCSharp, "MyApp.Order", nil)
	b := NewSymbolID(LangCSharp, "MyApp.Order", nil)
	if a != b {
		t.Fatal("same FQN must yield same ID")
	}
}

func TestLang(t *testing.T) {
	if got := NewSymbolID(LangTSJS, "src/a.ts#f", []string{}).Lang(); got != LangTSJS {
		t.Fatalf("Lang() = %q", got)
	}
}
