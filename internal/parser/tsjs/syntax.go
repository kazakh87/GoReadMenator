package tsjs

import "strings"

// typeInfo ist ein gelesener Typausdruck. endIdx ist exklusiv.
type typeInfo struct {
	text     string
	startIdx int
	endIdx   int
}

var modifierWords = map[string]bool{
	"public": true, "private": true, "protected": true, "static": true, "readonly": true,
	"abstract": true, "override": true, "async": true, "declare": true, "accessor": true,
}

// builtinTypes sind Schlüsselwörter und eingebaute Typen, die keine Verweise auf Symbole sind.
var builtinTypes = map[string]bool{
	"string": true, "number": true, "boolean": true, "any": true, "unknown": true, "never": true,
	"void": true, "null": true, "undefined": true, "object": true, "symbol": true, "bigint": true,
	"this": true, "true": true, "false": true, "typeof": true, "keyof": true, "readonly": true,
	"infer": true, "is": true, "asserts": true, "unique": true, "extends": true, "in": true,
	"out": true, "new": true, "import": true, "as": true, "const": true, "satisfies": true,
	"var": true, "let": true, "function": true, "class": true, "return": true, "if": true,
	"else": true, "for": true, "while": true, "do": true, "switch": true, "case": true,
	"default": true, "break": true, "continue": true, "try": true, "catch": true, "finally": true,
	"throw": true, "delete": true, "yield": true, "await": true, "super": true,
	"of": true, "with": true, "debugger": true,
}

// controlWords sind Schlüsselwörter, die wie Aufrufe aussehen.
var controlWords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true, "function": true,
	"return": true, "typeof": true, "void": true, "delete": true, "await": true, "yield": true,
	"super": true, "this": true, "new": true, "import": true, "require": true, "with": true,
	"do": true, "else": true, "throw": true, "in": true, "of": true, "case": true,
}

func isP(t token, s string) bool { return t.kind == tokPunct && t.text == s }

func isWord(t token, s string) bool { return t.kind == tokIdent && t.text == s }

// matchClose liefert den Index der schließenden Klammer. -1, wenn keine gefunden wird.
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

// matchAngle liefert den Index des schließenden `>` zu `<` bei i. Ignoriert Inhalte
// in Klammern und Klammer-Blöcken. Bricht an `;` ab.
func matchAngle(toks []token, i, end int) int {
	if i >= end || i >= len(toks) || !isP(toks[i], "<") {
		return -1
	}
	angle, paren := 0, 0
	for j := i; j < end && j < len(toks); j++ {
		t := toks[j]
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case ";":
			return -1
		case "(", "[", "{":
			paren++
		case ")", "]", "}":
			paren--
			if paren < 0 {
				return -1
			}
		case "<":
			if paren == 0 {
				angle++
			}
		case ">":
			if paren == 0 {
				angle--
				if angle == 0 {
					return j
				}
			}
		}
	}
	return -1
}

// findTopLevel sucht das erste Stopp-Token auf Klammertiefe 0. Liefert end, wenn keines gefunden wird.
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

// skipType überspringt einen Typausdruck bis zu einem Stopp-Token auf Tiefe 0.
// Generics (<...>) und Klammern werden mitgezählt. Liefert den Index des Stopps.
func skipType(toks []token, i, end int, stops ...string) int {
	paren, angle := 0, 0
	for j := i; j < end && j < len(toks); j++ {
		t := toks[j]
		if t.kind != tokPunct {
			continue
		}
		if isTypeBraceStart(toks, j, i) {
			paren++
			continue
		}
		if paren == 0 && angle == 0 {
			for _, s := range stops {
				if t.text == s {
					return j
				}
			}
		}
		switch t.text {
		case "(", "[", "{":
			paren++
		case ")", "]", "}":
			if paren == 0 {
				return j
			}
			paren--
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		case ";":
			if paren == 0 {
				return j
			}
		}
	}
	return end
}

// isTypeBraceStart erkennt `{` als Beginn eines Objekttyps (nicht als Körper):
// am Typanfang oder nach einem Operator wie `:`, `|`, `&`, `<`, `,`, `(`, `[`, `=>`.
func isTypeBraceStart(toks []token, j, typeStart int) bool {
	if !isP(toks[j], "{") {
		return false
	}
	if j == typeStart {
		return true
	}
	prev := toks[j-1]
	if prev.kind != tokPunct {
		return false
	}
	switch prev.text {
	case ":", "|", "&", "<", ",", "(", "[", "=>":
		return true
	}
	return false
}

// readTypeAfterColon liest den Typ ab i (ohne Doppelpunkt) bis zu einem der Stopps.
func readType(toks []token, i, end int, stops ...string) typeInfo {
	e := skipType(toks, i, end, stops...)
	return typeInfo{text: joinTokens(toks[i:e]), startIdx: i, endIdx: e}
}

// readNameChain liest einen qualifizierten Namen (A.B.C) und gibt Namen und Index danach zurück.
func readNameChain(toks []token, i, end int) (string, int) {
	if i >= end || i >= len(toks) || toks[i].kind != tokIdent {
		return "", i
	}
	parts := []string{toks[i].text}
	i++
	for i+1 < end && isP(toks[i], ".") && toks[i+1].kind == tokIdent {
		parts = append(parts, toks[i+1].text)
		i += 2
	}
	return strings.Join(parts, "."), i
}

// joinTokens setzt Texte zusammen. Zwischen zwei Bezeichnern steht ein Leerzeichen.
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

// startsStatement meldet, ob ein Token typischerweise eine neue Anweisung oder Deklaration beginnt.
func startsStatement(t token) bool {
	if isP(t, "@") {
		return true
	}
	if t.kind != tokIdent {
		return false
	}
	switch t.text {
	case "function", "class", "interface", "enum", "const", "let", "var", "export", "import",
		"declare", "namespace", "module", "abstract", "if", "for", "while", "do", "return",
		"try", "switch", "throw":
		return true
	}
	return false
}

// findStmtEnd liefert den Index des Semikolons, das die Anweisung beendet, oder den
// Index des ersten Tokens der nächsten Anweisung (Zeilenumbruch, ASI). Liefert end, wenn nichts passt.
func findStmtEnd(toks []token, i, end int) int {
	depth := 0
	for j := i; j < end && j < len(toks); j++ {
		t := toks[j]
		if j > i && depth == 0 && t.line > toks[j-1].endLine && startsStatement(t) {
			return j
		}
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case "(", "[", "{":
			depth++
		case ")", "]":
			depth--
		case "}":
			if depth == 0 {
				return j
			}
			depth--
		case ";":
			if depth == 0 {
				return j
			}
		}
	}
	return end
}

// recoverBodyEnd sucht bei einem nicht geschlossenen Körper das Ende vor der nächsten
// Deklaration, die in Spalte 1 beginnt. So bleiben die folgenden Deklarationen erhalten.
// Liefert end-1, wenn keine solche Stelle existiert.
func recoverBodyEnd(toks []token, open, end int) int {
	for j := open + 1; j < end && j < len(toks); j++ {
		t := toks[j]
		if t.col != 1 || t.kind != tokIdent {
			continue
		}
		switch t.text {
		case "export", "function", "class", "interface", "const", "let", "var", "type", "enum", "import":
			return j - 1
		}
	}
	return end - 1
}
