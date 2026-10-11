package csharp

import (
	"goreadmenator/internal/model"
)

// scanRefs durchsucht den Bereich [from, to) nach Aufrufen, Konstruktoraufrufen
// und Schreibzugriffen. enc ist das Symbol, in dem die Stelle liegt.
func (f *fileState) scanRefs(from, to int, enc model.SymbolID) {
	if from < 0 {
		return
	}
	for i := from; i < to && i < len(f.toks); i++ {
		t := f.toks[i]
		if t.kind != tokIdent {
			continue
		}
		if t.text == "new" {
			ty := readTypeAt(f.toks, i+1, to)
			if ty.text != "" {
				base := baseName(ty.text)
				if base != "" && !model.IsCSharpKeywordType(base) && ty.endIdx < to &&
					(isP(f.at(ty.endIdx), "(") || isP(f.at(ty.endIdx), "{")) {
					arity := -1
					if isP(f.at(ty.endIdx), "(") {
						arity = argCount(f.toks, ty.endIdx, to)
					}
					f.emitRef(model.Reference{
						Kind: model.RefConstruct, TargetName: base, Span: tokSpan(f.at(ty.startIdx)),
						Enclosing: enc, Confidence: model.ConfHeuristic, Arity: arity,
					})
				}
				i = ty.endIdx - 1
			}
			continue
		}
		name := unescape(t.text)
		if controlKeywords[name] || i+1 >= to {
			continue
		}
		nxt := f.at(i + 1)
		switch {
		case isP(nxt, "("):
			if verb, ok := mapVerbs[name]; ok {
				if lit := f.literalAt(i + 1); lit != "" {
					f.emitRef(model.Reference{Kind: model.RefHTTPRoute, TargetName: verb + " " + model.NormalizeRoute(lit),
						Span: tokSpan(t), Enclosing: f.enclosingOrTop(enc), Confidence: model.ConfExact})
				}
			}
			f.emitConvention(name, t, i+1, matchClose(f.toks, i+1, to, "(", ")"), enc)
			f.emitCall(name, t, enc, argCount(f.toks, i+1, to), qualifierAt(f.toks, i))
		case isP(nxt, "<"):
			if j := matchAngle(f.toks, i+1, to); j > 0 && j+1 < to && isP(f.at(j+1), "(") {
				f.emitCall(name, t, enc, argCount(f.toks, j+1, to), qualifierAt(f.toks, i))
			}
		case isP(nxt, "="):
			// `Typ name = ...` ist eine lokale Deklaration, keine Schreibzugriff auf ein Symbol.
			if prev := f.at(i - 1); prev.kind == tokIdent {
				continue
			}
			f.emitRef(model.Reference{
				Kind: model.RefWrite, TargetName: name, Span: tokSpan(t),
				Enclosing: enc, Confidence: model.ConfHeuristic,
			})
		}
	}
}

func (f *fileState) emitCall(name string, t token, enc model.SymbolID, arity int, qualifier string) {
	f.emitRef(model.Reference{
		Kind: model.RefCall, TargetName: name, Span: tokSpan(t),
		Enclosing: enc, Confidence: model.ConfHeuristic, Arity: arity, Qualifier: qualifier,
	})
}

// qualifierAt liefert den Namen vor einem Punkt (`a` in `a.b`), oder "".
func qualifierAt(toks []token, i int) string {
	if i < 2 || !isP(toks[i-1], ".") || toks[i-2].kind != tokIdent {
		return ""
	}
	return unescape(toks[i-2].text)
}

// argCount zählt die Argumente der Klammer bei open (Index von "("). Kommas in Klammern zählen nicht.
func argCount(toks []token, open, end int) int {
	close := matchClose(toks, open, end, "(", ")")
	if close < 0 {
		return -1
	}
	if close == open+1 {
		return 0
	}
	depth := 0
	n := 1
	for j := open + 1; j < close; j++ {
		t := toks[j]
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case ",":
			if depth == 0 {
				n++
			}
		}
	}
	return n
}
