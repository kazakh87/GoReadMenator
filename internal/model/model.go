// Package model enthält die Datentypen des Symbolindex. Sie sind frei von
// Speicher- und Parser-Details und werden von allen anderen Paketen geteilt.
package model

import (
	"regexp"
	"strings"
)

// FileID ist der stabile Schlüssel einer Datei im Index.
type FileID int64

// SymbolKind beschreibt die Art eines Symbols.
type SymbolKind string

const (
	KindNamespace   SymbolKind = "namespace"
	KindModule      SymbolKind = "module"
	KindClass       SymbolKind = "class"
	KindStruct      SymbolKind = "struct"
	KindInterface   SymbolKind = "interface"
	KindRecord      SymbolKind = "record"
	KindEnum        SymbolKind = "enum"
	KindTypeAlias   SymbolKind = "type"
	KindMethod      SymbolKind = "method"
	KindConstructor SymbolKind = "constructor"
	KindFunction    SymbolKind = "function"
	KindProperty    SymbolKind = "property"
	KindField       SymbolKind = "field"
	KindEvent       SymbolKind = "event"
	KindIndexer     SymbolKind = "indexer"
	KindVariable    SymbolKind = "variable"
	// KindTypeUnknown ist ein Typ aus einer XML-Dokumentation, dessen Art (Klasse, Interface,
	// Struktur, Enum) ohne Metadaten der DLL nicht bestimmbar ist.
	KindTypeUnknown SymbolKind = "type-unknown"
)

// LocationRole unterscheidet die Deklaration von weiteren Teildeklarationen (partial).
type LocationRole string

const (
	RoleDecl    LocationRole = "decl"
	RolePartial LocationRole = "partial"
)

// RefKind beschreibt, wie ein Symbol verwendet wird.
type RefKind string

const (
	RefCall       RefKind = "call"
	RefConstruct  RefKind = "construct"
	RefRead       RefKind = "read"
	RefWrite      RefKind = "write"
	RefTypeRef    RefKind = "type-ref"
	RefInherits   RefKind = "inherits"
	RefImplements RefKind = "implements"
	RefAttribute  RefKind = "attribute"
	// RefHTTPRoute ist eine Route im Backend: TargetName ist "VERB pfad", Enclosing die Aktion.
	RefHTTPRoute RefKind = "http-route"
	// RefHTTPCall ist ein HTTP-Aufruf im Frontend: TargetName ist "VERB pfad", Enclosing die aufrufende Funktion.
	RefHTTPCall RefKind = "http-call"
	// RefHTTPConvention ist konventionsbasiertes Routing: ein Muster oder eine Aktion, siehe convention.go.
	RefHTTPConvention RefKind = "http-convention"
	RefCref           RefKind = "cref"
	RefImport         RefKind = "import"
	RefJSXComponent   RefKind = "jsx"
)

// Confidence gibt an, wie sicher eine Verwendung aufgelöst wurde.
type Confidence string

const (
	// ConfExact: über Namensraum, Imports oder explizite Angabe eindeutig.
	ConfExact Confidence = "EXACT"
	// ConfResolved: über Kontext aufgelöst, aber nicht ganz eindeutig belegt.
	ConfResolved Confidence = "RESOLVED"
	// ConfHeuristic: nur über den Namen vermutet.
	ConfHeuristic Confidence = "HEURISTIC"
)

// RelationKind beschreibt eine strukturelle Beziehung zwischen zwei Symbolen.
type RelationKind string

const (
	RelContains    RelationKind = "contains"
	RelInherits    RelationKind = "inherits"
	RelImplements  RelationKind = "implements"
	RelHasType     RelationKind = "has_type"
	RelParamType   RelationKind = "param_type"
	RelReturns     RelationKind = "returns"
	RelAttributeOf RelationKind = "attribute_of"
	// RelCallsEndpoint verbindet eine Methode im Frontend mit der Controller-Aktion, die sie aufruft.
	RelCallsEndpoint RelationKind = "calls_endpoint"
)

// ImportKind beschreibt die Form eines Imports.
type ImportKind string

const (
	ImportESM       ImportKind = "esm"
	ImportDefault   ImportKind = "default"
	ImportNamespace ImportKind = "namespace"
	ImportReexport  ImportKind = "reexport"
	ImportDynamic   ImportKind = "dynamic"
	ImportRequire   ImportKind = "require"
	ImportUsing     ImportKind = "using"
	ImportTypeOnly  ImportKind = "type-only"
)

// Span ist ein Bereich im Quelltext. Zeilen und Spalten sind 1-basiert.
type Span struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// File ist eine indizierte Quelldatei.
type File struct {
	ID      FileID
	Path    string
	Lang    Language
	BlobSHA string
	LOC     int
	FileDoc string
}

// Symbol ist die zentrale Einheit des Index. Es hat eine stabile ID und kann
// über mehrere Fundorte (Locations) verteilt sein, etwa bei C# partial.
type Symbol struct {
	ID        SymbolID
	Kind      SymbolKind
	Name      string
	FQN       string
	Container SymbolID // übergeordnetes Symbol (Namespace oder Typ), leer nur beim Wurzel-Namespace
	Signature string
	Modifiers []string
	Summary   string
	Remarks   string
	Returns   string
	ParamDocs []ParamDoc
	DocSource string // "inline" | "xml"
}

// ParamDoc ist die Beschreibung eines Parameters aus der Dokumentation.
type ParamDoc struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// Location ist ein Fundort eines Symbols in einer Datei.
type Location struct {
	SymbolID SymbolID
	FileID   FileID
	Span     Span
	Role     LocationRole
}

// Reference ist eine Verwendung eines Symbols an einer Quellstelle (Compiler-Sicht).
type Reference struct {
	ID         int64 // vom Store vergeben, eindeutig über alle Verwendungen
	FileID     FileID
	TargetID   SymbolID // leer, wenn nicht aufgelöst
	TargetName string   // Rohname, bleibt für spätere Neuauflösung erhalten
	Kind       RefKind
	Span       Span
	Enclosing  SymbolID // umgebendes Symbol, das die Verwendung enthält
	Confidence Confidence
	// Arity ist die Anzahl der Argumente bei Aufrufen und Konstruktoraufrufen (sonst ungenutzt).
	Arity int
	// Role markiert den Kopf eines Typverweises, aus dem eine Beziehung entsteht:
	// RoleHasType (Typ eines Members), RoleParamType (Parametertyp), RoleReturns (Rückgabetyp).
	Role string
	// Qualifier ist der Name links vom Punkt bei Member-Zugriffen (`util` in `util.keys()`), sonst leer.
	Qualifier string
}

// Rollen eines Typverweises (siehe Reference.Role).
const (
	RoleHasType   = "has-type"
	RoleParamType = "param-type"
	RoleReturns   = "returns"
)

// Relation ist eine strukturelle Beziehung zwischen zwei Symbolen.
// Derived markiert Beziehungen, die der Resolver aus aufgelösten Verweisen ableitet.
type Relation struct {
	FileID  FileID
	From    SymbolID
	To      SymbolID
	Kind    RelationKind
	Derived bool
}

// ImportBinding ist eine lokale Bindung eines Imports. Imported ist "default" für den
// Default-Export, "*" für einen Namespace-Import und sonst der Name im Modul.
type ImportBinding struct {
	Local    string
	Imported string
}

// Import ist ein Import-Statement in einer Datei.
type Import struct {
	ID         int64 // vom Store vergeben
	FileID     FileID
	Raw        string
	Kind       ImportKind
	ResolvedID FileID // 0, wenn nicht aufgelöst (etwa externe Pakete oder C#-Namespaces)
	Bindings   []ImportBinding
}

// NormalizeRoute bringt einen HTTP-Pfad in eine vergleichbare Form. Parameter aller Schreibweisen
// ({id}, {id:int}, ${id}) werden zu {}, Schema und Host entfallen, Groß- und Kleinschreibung spielt keine Rolle.
func NormalizeRoute(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.Index(p, "://"); i >= 0 {
		rest := p[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			p = rest[j:]
		} else {
			p = ""
		}
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = routeParam.ReplaceAllString(p, "{}")
	p = strings.ToLower(strings.Trim(p, "/"))
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

var routeParam = regexp.MustCompile(`\$?\{[^}]*\}`)
