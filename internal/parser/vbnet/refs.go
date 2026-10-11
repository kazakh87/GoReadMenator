package vbnet

import (
	"strings"

	"goreadmenator/internal/model"
)

// typeInfo ist ein Typausdruck aus einer As-Klausel.
type typeInfo struct {
	text string
	toks []token
}

// param ist ein Parameter einer Sub, Function oder Property.
type param struct {
	name string
	typ  typeInfo
}

// builtinTypes sind eingebaute VB-Typen und Schlüsselwörter, die keine Verweise sind.
var builtinTypes = map[string]bool{
	"integer": true, "long": true, "short": true, "byte": true, "boolean": true, "string": true,
	"object": true, "double": true, "single": true, "decimal": true, "date": true, "char": true,
	"sbyte": true, "uinteger": true, "ulong": true, "ushort": true, "of": true, "nothing": true,
	"true": true, "false": true, "me": true, "mybase": true, "myclass": true, "as": true, "new": true,
}

// controlWords sehen aus wie Aufrufe, sind aber Schlüsselwörter.
var controlWords = map[string]bool{
	"if": true, "elseif": true, "while": true, "until": true, "for": true, "select": true, "case": true,
	"return": true, "function": true, "sub": true, "typeof": true, "gettype": true, "directcast": true,
	"ctype": true, "cint": true, "cstr": true, "cdbl": true, "cbool": true, "cdate": true, "call": true,
	"addressof": true, "redim": true, "throw": true, "using": true, "lock": true, "synclock": true,
	"nameof": true, "raiseevent": false, "and": true, "or": true, "not": true, "andalso": true, "orelse": true,
}

// parseParams liest eine Parameterliste ab der Position i (erwartet "(" oder nichts). Liefert die
// Parameter und die Position nach der Liste.
func parseParams(toks []token, i int) ([]param, int) {
	if i >= len(toks) || toks[i].text != "(" {
		return nil, i
	}
	depth := 0
	end := len(toks)
	for j := i; j < len(toks); j++ {
		if toks[j].text == "(" {
			depth++
		} else if toks[j].text == ")" {
			depth--
			if depth == 0 {
				end = j
				break
			}
		}
	}
	var out []param
	for _, seg := range splitTop(toks[i+1 : end]) {
		// Modifikatoren des Parameters entfernen (ByVal, ByRef, Optional, ParamArray)
		for len(seg) > 0 && (seg[0].lower == "byval" || seg[0].lower == "byref" || seg[0].lower == "optional" || seg[0].lower == "paramarray") {
			seg = seg[1:]
		}
		if len(seg) == 0 || seg[0].kind != tokIdent {
			continue
		}
		p := param{name: seg[0].text}
		for k := 1; k < len(seg); k++ {
			if seg[k].lower == "as" {
				p.typ = readTypeTokens(seg, k+1)
				break
			}
		}
		out = append(out, p)
	}
	return out, end + 1
}

// readTypeTokens liest einen Typ ab Position i bis zum Ende der Liste oder bis "=" auf Klammertiefe 0.
func readTypeTokens(toks []token, i int) typeInfo {
	if i >= len(toks) {
		return typeInfo{}
	}
	depth := 0
	j := i
	for ; j < len(toks); j++ {
		t := toks[j]
		if t.text == "(" {
			depth++
		} else if t.text == ")" {
			if depth == 0 {
				break
			}
			depth--
		} else if depth == 0 && (t.text == "=" || t.text == ",") {
			break
		}
	}
	part := toks[i:j]
	return typeInfo{text: joinTokens(part), toks: part}
}

// buildSig baut eine lesbare Signatur.
func buildSig(mods []string, kw, name string, params []param, ret typeInfo) string {
	var ps []string
	for _, p := range params {
		if p.typ.text != "" {
			ps = append(ps, p.name+" As "+p.typ.text)
		} else {
			ps = append(ps, p.name)
		}
	}
	sig := strings.Join(append(append([]string{}, mods...), kw, name), " ") + "(" + strings.Join(ps, ", ") + ")"
	if ret.text != "" {
		sig += " As " + ret.text
	}
	return sig
}

// emitTypeRefs legt Verweise für die Namen eines Typs an. Der erste Verweis trägt die Rolle.
// Qualifizierte Namen liefern nur das letzte Segment. Eingebaute Typen zählen nicht.
func (f *fileState) emitTypeRefs(toks []token, enc model.SymbolID, role string) {
	for k := 0; k < len(toks); k++ {
		t := toks[k]
		if t.kind != tokIdent || builtinTypes[t.lower] {
			continue
		}
		if k+1 < len(toks) && toks[k+1].text == "." {
			continue
		}
		if k+1 < len(toks) && toks[k+1].lower == "as" {
			continue // benannte Tupel-Elemente: (Left As Integer) ist kein Typ "Left"
		}
		f.emitRef(model.Reference{Kind: model.RefTypeRef, TargetName: t.text, Span: tokSpan(t),
			Enclosing: enc, Confidence: model.ConfHeuristic, Role: role})
		role = ""
	}
}

// scanRefs durchsucht eine Anweisung nach Aufrufen, Erzeugungen und Zuweisungen.
func (f *fileState) scanRefs(toks []token, enc model.SymbolID) {
	// Zuweisung am Anfang: "x = ..." oder "Me.x = ..." oder "obj.x = ..."
	if len(toks) >= 2 && toks[0].kind == tokIdent && toks[1].text == "=" && !builtinTypes[toks[0].lower] {
		f.emitRef(model.Reference{Kind: model.RefWrite, TargetName: toks[0].text, Span: tokSpan(toks[0]),
			Enclosing: enc, Confidence: model.ConfHeuristic})
	}
	if len(toks) >= 4 && toks[0].kind == tokIdent && toks[1].text == "." && toks[2].kind == tokIdent && toks[3].text == "=" {
		f.emitRef(model.Reference{Kind: model.RefWrite, TargetName: toks[2].text, Span: tokSpan(toks[2]),
			Enclosing: enc, Confidence: model.ConfHeuristic})
	}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tokIdent {
			continue
		}
		if t.lower == "new" {
			j := i + 1
			name := ""
			for j < len(toks) && toks[j].kind == tokIdent {
				name = toks[j].text
				j++
				if j < len(toks) && toks[j].text == "." {
					j++
					continue
				}
				break
			}
			if name != "" && j < len(toks) && toks[j].text == "(" && !builtinTypes[strings.ToLower(name)] {
				f.emitRef(model.Reference{Kind: model.RefConstruct, TargetName: name, Span: tokSpan(toks[j-1]),
					Enclosing: enc, Confidence: model.ConfHeuristic, Arity: argCount(toks, j)})
			}
			continue
		}
		if controlWords[t.lower] || builtinTypes[t.lower] || i+1 >= len(toks) || toks[i+1].text != "(" {
			continue
		}
		if i > 0 && toks[i-1].lower == "new" {
			continue // "New Typ(" ist eine Erzeugung und schon erfasst
		}
		qualifier := ""
		if i >= 2 && toks[i-1].text == "." && toks[i-2].kind == tokIdent {
			qualifier = toks[i-2].text
		}
		f.emitRef(model.Reference{Kind: model.RefCall, TargetName: t.text, Span: tokSpan(t),
			Enclosing: enc, Confidence: model.ConfHeuristic, Arity: argCount(toks, i+1), Qualifier: qualifier})
	}
}

// argCount zählt die Argumente der Klammer bei open. -1, wenn sie nicht geschlossen ist.
func argCount(toks []token, open int) int {
	depth := 0
	close := -1
	for j := open; j < len(toks); j++ {
		if toks[j].text == "(" {
			depth++
		} else if toks[j].text == ")" {
			depth--
			if depth == 0 {
				close = j
				break
			}
		}
	}
	if close < 0 {
		return -1
	}
	if close == open+1 {
		return 0
	}
	n := 1
	depth = 0
	for j := open + 1; j < close; j++ {
		switch toks[j].text {
		case "(":
			depth++
		case ")":
			depth--
		case ",":
			if depth == 0 {
				n++
			}
		}
	}
	return n
}
