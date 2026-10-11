package csharp

import "strings"

// typeInfo beschreibt einen gelesenen Typ im Token-Strom.
// endIdx ist exklusiv. Ist text leer, wurde kein Typ erkannt.
type typeInfo struct {
	text     string
	startIdx int
	endIdx   int
}

var modifierSet = map[string]bool{
	"public": true, "private": true, "protected": true, "internal": true,
	"static": true, "readonly": true, "abstract": true, "virtual": true,
	"override": true, "sealed": true, "partial": true, "async": true, "file": true,
	"extern": true, "unsafe": true, "new": true, "const": true,
	"volatile": true, "required": true, "ref": true,
}

var paramModifierSet = map[string]bool{
	"ref": true, "out": true, "in": true, "params": true, "this": true, "scoped": true, "readonly": true,
}

// controlKeywords sind Schlüsselwörter, die wie Aufrufe aussehen, aber keine sind.
var controlKeywords = map[string]bool{
	"if": true, "else": true, "for": true, "foreach": true, "while": true, "do": true,
	"switch": true, "case": true, "catch": true, "using": true, "lock": true,
	"return": true, "typeof": true, "nameof": true, "sizeof": true, "default": true,
	"await": true, "when": true, "fixed": true, "checked": true, "unchecked": true,
	"base": true, "this": true, "throw": true, "yield": true, "is": true, "as": true,
	"in": true, "out": true, "ref": true, "stackalloc": true, "where": true,
	"new": true, "var": true,
}

func isP(t token, s string) bool { return t.kind == tokPunct && t.text == s }

func unescape(s string) string { return strings.TrimPrefix(s, "@") }

// matchClose liefert den Index der schließenden Klammer zu toks[i]. -1, wenn keiner da ist.
func matchClose(toks []token, i, end int, open, close string) int {
	depth := 0
	for j := i; j < end && j < len(toks); j++ {
		switch {
		case isP(toks[j], open):
			depth++
		case isP(toks[j], close):
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// matchAngle liefert den Index des schließenden `>` zu einem `<` bei i. -1, wenn keiner da ist.
// Bricht bei Anweisungsgrenzen ab, damit ein einzelnes `<` (Vergleich) nicht den Rest auffrisst.
func matchAngle(toks []token, i, end int) int {
	if i >= end || i >= len(toks) || !isP(toks[i], "<") {
		return -1
	}
	depth := 0
	for j := i; j < end && j < len(toks); j++ {
		t := toks[j]
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case "<":
			depth++
		case ">":
			depth--
			if depth == 0 {
				return j
			}
		case ";", "{", "}", "=>", "==":
			return -1
		}
	}
	return -1
}

// findTopLevel sucht ab i das erste der Stopp-Tokens, das nicht in Klammern steht.
// Liefert end, wenn keines gefunden wird.
func findTopLevel(toks []token, i, end int, stops ...string) int {
	depth := 0
	for j := i; j < end && j < len(toks); j++ {
		t := toks[j]
		if t.kind == tokPunct {
			if depth == 0 {
				for _, s := range stops {
					if t.text == s {
						return j
					}
				}
			}
			switch t.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			}
		}
	}
	return end
}

// readTypeAt liest einen Typ ab Index i: Namen mit Punkten, Generics, Tupel,
// Array-Ränge und Nullable-Suffixe.
func readTypeAt(toks []token, i, end int) typeInfo {
	start := i
	if i >= end || i >= len(toks) {
		return typeInfo{startIdx: i, endIdx: i}
	}
	switch {
	case toks[i].kind == tokIdent:
		i++
	case isP(toks[i], "("):
		j := matchClose(toks, i, end, "(", ")")
		if j < 0 {
			return typeInfo{startIdx: i, endIdx: i}
		}
		i = j + 1
	default:
		return typeInfo{startIdx: i, endIdx: i}
	}
	for {
		if i < end && isP(toks[i], "<") {
			if j := matchAngle(toks, i, end); j > 0 {
				i = j + 1
				continue
			}
		}
		if i+1 < end && isP(toks[i], ".") && toks[i+1].kind == tokIdent {
			i += 2
			continue
		}
		break
	}
	for i < end {
		if isP(toks[i], "[") {
			if j := matchClose(toks, i, end, "[", "]"); j > 0 {
				i = j + 1
				continue
			}
		}
		if isP(toks[i], "?") || isP(toks[i], "*") {
			i++
			continue
		}
		break
	}
	return typeInfo{text: joinTokens(toks[start:i]), startIdx: start, endIdx: i}
}

// joinTokens setzt Token-Texte zusammen. Zwischen zwei Bezeichnern steht ein Leerzeichen.
func joinTokens(ts []token) string {
	var b strings.Builder
	for i, t := range ts {
		if i > 0 && ts[i-1].kind == tokIdent && t.kind == tokIdent {
			b.WriteByte(' ')
		}
		b.WriteString(t.text)
	}
	return b.String()
}

// baseName liefert den letzten Namenssegment ohne Generics und Array-Suffix.
func baseName(text string) string {
	if i := strings.IndexByte(text, '<'); i >= 0 {
		text = text[:i]
	}
	text = strings.TrimRight(text, "[]?*")
	if i := strings.LastIndexByte(text, '.'); i >= 0 {
		text = text[i+1:]
	}
	return unescape(text)
}

// countTopLevelParams zählt die Typparameter in einem Generic-Ausdruck (Kommas auf Tiefe 0 + 1).
func countTopLevelParams(ts []token) int {
	if len(ts) == 0 {
		return 0
	}
	depth := 0
	n := 1
	for _, t := range ts {
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case "<", "(", "[":
			depth++
		case ">", ")", "]":
			depth--
		case ",":
			if depth == 0 {
				n++
			}
		}
	}
	return n
}

// findExprEnd sucht das Ende eines Ausdrucks (z. B. einer Feld-Initialisierung).
// Anders als findTopLevel werden Generic-Argumente übersprungen, sodass ein Komma
// in `new Dictionary<A, B>()` nicht als Trenner zählt.
func findExprEnd(toks []token, i, end int, stops ...string) int {
	depth := 0
	for j := i; j < end && j < len(toks); j++ {
		t := toks[j]
		if t.kind == tokIdent && j+1 < end && isP(toks[j+1], "<") {
			if k := matchAngle(toks, j+1, end); k > 0 {
				j = k
				continue
			}
		}
		if t.kind != tokPunct {
			continue
		}
		if depth == 0 {
			for _, s := range stops {
				if t.text == s {
					return j
				}
			}
		}
		switch t.text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		}
	}
	return end
}

// genericArityIn liefert die Anzahl generischer Argumente eines Typausdrucks im Bereich [from, to).
// 0, wenn keine Argumente angegeben sind.
func genericArityIn(toks []token, from, to int) int {
	for k := from; k < to; k++ {
		if isP(toks[k], "<") {
			if j := matchAngle(toks, k, to); j > 0 {
				return countTopLevelParams(toks[k+1 : j])
			}
			return 0
		}
	}
	return 0
}
