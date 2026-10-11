package tsjs

import (
	"strings"

	"goreadmenator/internal/model"
)

// scanRefs durchsucht [from, to) nach Aufrufen, Konstruktoraufrufen, Schreibzugriffen,
// Imports über require/import(), JSX-Komponenten und Typangaben nach `as`.
func (f *fileState) scanRefs(from, to int, enc model.SymbolID) {
	if from < 0 {
		return
	}
	for i := from; i < to && i < len(f.toks); i++ {
		t := f.toks[i]

		if t.kind == tokPunct && t.text == "<" && f.jsx && f.isJSXStart(i, to) {
			name, _ := readNameChain(f.toks, i+1, to)
			if name != "" {
				f.emitRef(model.Reference{Kind: model.RefJSXComponent, TargetName: lastSegment(name),
					Span: tokSpan(f.at(i + 1)), Enclosing: enc, Confidence: model.ConfHeuristic})
			}
			continue
		}
		if t.kind != tokIdent {
			continue
		}

		switch {
		case t.text == "new":
			name, next := readNameChain(f.toks, i+1, to)
			if name != "" {
				if isP(f.at(next), "<") {
					if j := matchAngle(f.toks, next, to); j > 0 {
						next = j + 1
					}
				}
				if isP(f.at(next), "(") && !builtinTypes[name] {
					f.emitRef(model.Reference{Kind: model.RefConstruct, TargetName: lastSegment(name),
						Span: tokSpan(f.at(i + 1)), Enclosing: enc, Confidence: model.ConfHeuristic,
						Arity: argCount(f.toks, next, to)})
				}
				i = next - 1
			}
			continue
		case t.text == "require" && isP(f.at(i+1), "(") && f.at(i+2).kind == tokString && isP(f.at(i+3), ")"):
			f.emitImport(f.at(i+2).text, model.ImportRequire)
			i += 3
			continue
		case t.text == "import" && isP(f.at(i+1), "(") && f.at(i+2).kind == tokString:
			f.emitImport(f.at(i+2).text, model.ImportDynamic)
			i += 2
			continue
		case t.text == "as" || t.text == "satisfies":
			if n := f.at(i + 1); n.kind == tokIdent && !builtinTypes[n.text] {
				_, next := readNameChain(f.toks, i+1, to)
				f.emitTypeRefs(i+1, next, enc)
			}
			continue
		}

		if controlWords[t.text] || builtinTypes[t.text] {
			continue
		}
		nxt := f.at(i + 1)
		prev := f.at(i - 1)
		switch {
		case isP(nxt, "("):
			if !isWord(prev, "function") && !isWord(prev, "class") {
				f.emitCall(t, enc, argCount(f.toks, i+1, to), qualifierAt(f.toks, i))
				f.emitHTTPCall(i, to, enc)
			}
		case isP(nxt, "<"):
			if j := matchAngle(f.toks, i+1, to); j > 0 && isP(f.at(j+1), "(") {
				f.emitCall(t, enc, argCount(f.toks, j+1, to), qualifierAt(f.toks, i))
			}
		case isP(nxt, "="):
			// Zuweisung nur am Anweisungsanfang oder nach Punkt; `name=` in JSX-Attributen zählt nicht.
			assignPos := prev.kind == 0 || isP(prev, ".") || isP(prev, ";") || isP(prev, "{") || isP(prev, "}") || isP(prev, ")")
			if assignPos && !(isWord(prev, "const") || isWord(prev, "let") || isWord(prev, "var")) {
				f.emitRef(model.Reference{Kind: model.RefWrite, TargetName: t.text, Span: tokSpan(t),
					Enclosing: enc, Confidence: model.ConfHeuristic})
			}
		}
	}
}

// isJSXStart erkennt `<Komponente` in Ausdrucksposition (nicht als Vergleich).
func (f *fileState) isJSXStart(i, to int) bool {
	n := f.at(i + 1)
	if n.kind != tokIdent || n.text == "" || !(n.text[0] >= 'A' && n.text[0] <= 'Z') {
		return false
	}
	prev := f.at(i - 1)
	if prev.kind == tokIdent {
		return prev.text == "return" || prev.text == "default"
	}
	if prev.kind != tokPunct {
		return false
	}
	switch prev.text {
	case "(", "=", ":", ",", "?", "&&", "||", "=>", "{", "[", "??", ">":
		return true
	}
	return false
}

func (f *fileState) emitCall(t token, enc model.SymbolID, arity int, qualifier string) {
	f.emitRef(model.Reference{Kind: model.RefCall, TargetName: t.text, Span: tokSpan(t),
		Enclosing: enc, Confidence: model.ConfHeuristic, Arity: arity, Qualifier: qualifier})
}

// qualifierAt liefert den Namen vor einem Punkt (`a` in `a.b`), oder "".
func qualifierAt(toks []token, i int) string {
	if i < 2 || !isP(toks[i-1], ".") || toks[i-2].kind != tokIdent {
		return ""
	}
	return toks[i-2].text
}

// argCount zählt die Argumente der Klammer bei open (Index von "("). -1, wenn sie nicht geschlossen ist.
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

// emitTypeRefs legt Typverweise für Namen im Bereich [from, to) an.
// Eingebaute Typen, Typparameter und Schlüssel in Objekttypen zählen nicht.
// Bei qualifizierten Namen (A.B) gilt nur das letzte Segment.
func (f *fileState) emitTypeRefs(from, to int, enc model.SymbolID) {
	f.emitTypeRefsRole(from, to, enc, "")
}

// emitTypeRefsRole wie emitTypeRefs. Der erste Verweis (Kopf des Typs) trägt die Rolle.
func (f *fileState) emitTypeRefsRole(from, to int, enc model.SymbolID, role string) {
	for k := from; k < to && k < len(f.toks); k++ {
		t := f.toks[k]
		if t.kind != tokIdent {
			continue
		}
		name := t.text
		if builtinTypes[name] || f.tparams[name] {
			continue
		}
		if k+1 < to && isP(f.at(k+1), ".") {
			continue
		}
		if isP(f.at(k+1), ":") || (isP(f.at(k+1), "?") && isP(f.at(k+2), ":")) {
			// Eigenschaftsname in Objekttyp oder Parameter
			continue
		}
		f.emitRef(model.Reference{Kind: model.RefTypeRef, TargetName: strings.TrimSpace(name), Span: tokSpan(t),
			Enclosing: enc, Confidence: model.ConfHeuristic, Role: role})
		role = ""
	}
}

// httpVerbOf liefert das HTTP-Verb für einen Aufruf, der eine URL entgegennimmt, oder "" wenn es keiner ist.
// fetch ist GET, außer das Optionsobjekt setzt ein anderes Verb. axios.get und http.get geben das Verb vor.
func httpVerbOf(toks []token, i int) string {
	name := toks[i].text
	qualifier := qualifierAt(toks, i)
	switch {
	case name == "fetch" && qualifier == "":
		return "GET"
	case (qualifier == "axios" || qualifier == "http") && isHTTPMethodName(name):
		return strings.ToUpper(name)
	}
	return ""
}

func isHTTPMethodName(n string) bool {
	switch n {
	case "get", "post", "put", "delete", "patch":
		return true
	}
	return false
}

// emitHTTPCall legt einen HTTP-Aufruf an, wenn die URL als Literal, Template oder Verkettung erkennbar ist.
// Teile, die zur Laufzeit entstehen (id in "api/" + id), werden zu {} und passen auf jeden Routen-Parameter.
func (f *fileState) emitHTTPCall(i, to int, enc model.SymbolID) {
	verb := httpVerbOf(f.toks, i)
	if verb == "" || i+1 >= to || !isP(f.at(i+1), "(") {
		return
	}
	end := matchClose(f.toks, i+1, to, "(", ")")
	if end < 0 {
		return
	}
	args := splitArgs(f.toks, i+2, end)
	if len(args) == 0 {
		return
	}
	urlTemplate, ok := urlTemplateOf(args[0])
	if len(args[0]) == 0 {
		return
	}
	if raw, isTmpl := f.templateAt(args[0][0]); isTmpl {
		urlTemplate, ok = raw, true
	}
	if !ok {
		return
	}
	if verb == "GET" {
		for _, a := range args[1:] {
			if v, ok := methodOption(a); ok {
				verb = v
			}
		}
	}
	f.emitRef(model.Reference{Kind: model.RefHTTPCall, TargetName: verb + " " + model.NormalizeRoute(urlTemplate),
		Span: tokSpan(f.toks[i]), Enclosing: enc, Confidence: model.ConfHeuristic})
}

// splitArgs teilt die Argumente zwischen from und to an Kommas auf Klammertiefe 0.
func splitArgs(toks []token, from, to int) [][]token {
	var out [][]token
	depth, start := 0, from
	for k := from; k < to; k++ {
		switch {
		case isP(toks[k], "(") || isP(toks[k], "[") || isP(toks[k], "{"):
			depth++
		case isP(toks[k], ")") || isP(toks[k], "]") || isP(toks[k], "}"):
			depth--
		case isP(toks[k], ",") && depth == 0:
			out = append(out, toks[start:k])
			start = k + 1
		}
	}
	if start < to {
		out = append(out, toks[start:to])
	}
	return out
}

// urlTemplateOf baut aus einem URL-Ausdruck das Muster. Nur Ausdrücke, die mit einem Literal beginnen, zählen.
func urlTemplateOf(arg []token) (string, bool) {
	if len(arg) == 0 || arg[0].kind != tokString {
		return "", false
	}
	var b strings.Builder
	prevDynamic := false
	for k, t := range arg {
		switch {
		case t.kind == tokString:
			b.WriteString(t.text)
			prevDynamic = false
		case isP(t, "+"):
			// Verkettung: Literal und dynamischer Teil wechseln sich ab
		default:
			if !prevDynamic && k > 0 {
				b.WriteString("{}")
			}
			prevDynamic = true
		}
	}
	return b.String(), true
}

// methodOption liest method: 'POST' aus einem Optionsobjekt von fetch.
func methodOption(arg []token) (string, bool) {
	for k := 0; k+2 < len(arg); k++ {
		if arg[k].text == "method" && isP(arg[k+1], ":") && arg[k+2].kind == tokString {
			return strings.ToUpper(arg[k+2].text), true
		}
	}
	return "", false
}
