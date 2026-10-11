package model

import "strings"

// SymbolID identifiziert ein Symbol eindeutig über alle Dateien und Commits hinweg.
//
// Format:
//
//	<sprache>:<vollqualifizierter-name>[(<parametertypen>)]
//
// Beispiele:
//
//	csharp:MyApp.Services.OrderService                       (Typ, keine Klammern)
//	csharp:MyApp.Services.OrderService.Submit(System.Int32)  (Methode)
//	tsjs:src/orders.ts#OrderService.submit(number)            (Methode)
//
// Entscheidung: Überladungen werden über die Parametertypen unterschieden,
// nicht über einen Laufindex. Ein Index würde sich bei jeder Umsortierung
// im Quelltext ändern und damit das inkrementelle Update zerstören.
type SymbolID string

// Sprache für die ID-Bildung. Eigene Typen statt freier Strings verhindern Tippfehler.
type Language string

const (
	LangCSharp Language = "csharp"
	LangTSJS   Language = "tsjs"
	LangVBNet  Language = "vbnet"
	LangRazor  Language = "razor" // .cshtml, .razor: C# mit HTML
	LangASPX   Language = "aspx"  // .aspx, .ascx, .master: Web Forms mit C#
)

// csharpAliases bildet C#-Schlüsselwörter auf ihre System-Typen ab, damit
// `int` und `System.Int32` in Parameterlisten dieselbe ID ergeben.
var csharpAliases = map[string]string{
	"bool":    "System.Boolean",
	"byte":    "System.Byte",
	"sbyte":   "System.SByte",
	"char":    "System.Char",
	"decimal": "System.Decimal",
	"double":  "System.Double",
	"float":   "System.Single",
	"int":     "System.Int32",
	"uint":    "System.UInt32",
	"long":    "System.Int64",
	"ulong":   "System.UInt64",
	"object":  "System.Object",
	"short":   "System.Int16",
	"ushort":  "System.UInt16",
	"string":  "System.String",
}

// vbAliases bildet VB-Schlüsselworttypen auf System-Typen ab (Dim x As Integer == System.Int32).
var vbAliases = map[string]string{
	"boolean": "System.Boolean", "byte": "System.Byte", "sbyte": "System.SByte", "char": "System.Char",
	"date": "System.DateTime", "decimal": "System.Decimal", "double": "System.Double", "single": "System.Single",
	"integer": "System.Int32", "uinteger": "System.UInt32", "long": "System.Int64", "ulong": "System.UInt64",
	"object": "System.Object", "short": "System.Int16", "ushort": "System.UInt16", "string": "System.String",
}

// NewSymbolID bildet die ID aus Sprache, vollqualifiziertem Namen und
// Parametertypen. paramTypes ist nil für Symbole ohne Parameterliste
// (Typen, Properties, Felder). Eine leere, aber nicht nil Liste ergibt `()`.
func NewSymbolID(lang Language, fqn string, paramTypes []string) SymbolID {
	var b strings.Builder
	b.WriteString(string(lang))
	b.WriteByte(':')
	b.WriteString(fqn)
	if paramTypes != nil {
		b.WriteByte('(')
		for i, p := range paramTypes {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(NormalizeType(lang, p))
		}
		b.WriteByte(')')
	}
	return SymbolID(b.String())
}

// NormalizeType bringt einen Typnamen in die kanonische Schreibweise:
// Leerzeichen entfernen, Schlüsselwort-Aliase auflösen, Arrays und
// Nullable-Suffixe beibehalten.
func NormalizeType(lang Language, t string) string {
	t = strings.Join(strings.Fields(t), "")
	if lang == LangVBNet {
		// VB ist nicht case-sensitiv: Schlüsselworttypen auf System-Typen abbilden
		if canon, ok := vbAliases[strings.ToLower(t)]; ok {
			return canon
		}
		return t
	}
	if lang != LangCSharp {
		return t
	}
	// Suffixe (Array, Nullable) abtrennen, Basis auflösen, Suffix wieder anhängen.
	base, suffix := t, ""
	for {
		switch {
		case strings.HasSuffix(base, "[]"):
			suffix = "[]" + suffix
			base = base[:len(base)-2]
			continue
		case strings.HasSuffix(base, "?"):
			suffix = "?" + suffix
			base = base[:len(base)-1]
			continue
		}
		break
	}
	if canon, ok := csharpAliases[base]; ok {
		base = canon
	}
	return base + suffix
}

// Lang liefert die Sprache, die in der ID kodiert ist.
func (id SymbolID) Lang() Language {
	if i := strings.IndexByte(string(id), ':'); i >= 0 {
		return Language(id[:i])
	}
	return ""
}

// IsCSharpKeywordType meldet, ob ein Name ein C#-Schlüsselwort-Typ ist (int, string, ...).
func IsCSharpKeywordType(name string) bool {
	_, ok := csharpAliases[name]
	return ok
}
