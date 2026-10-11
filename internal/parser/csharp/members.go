package csharp

import (
	"strings"

	"goreadmenator/internal/model"
)

type paramInfo struct {
	ty   typeInfo
	name string
}

// parseMember verarbeitet Methoden, Konstruktoren, Properties, Indexer, Operatoren und Felder.
func (f *fileState) parseMember(own owner, mods []string, attrs []attrRef, start, end int) {
	t := f.at(f.pos)

	// Destruktor
	if isP(t, "~") {
		f.skipStatement(end)
		return
	}

	// Konvertierungsoperator: implicit operator T(...)
	if (t.text == "implicit" || t.text == "explicit") && isIdentAt(f, f.pos+1, "operator") {
		f.pos += 2
		ty := readTypeAt(f.toks, f.pos, end)
		if ty.text == "" {
			f.diag(t, "Konvertierungsoperator ohne Zieltyp")
			f.skipStatement(end)
			return
		}
		nameTok := f.at(f.pos - 1)
		f.pos = ty.endIdx
		f.finishCallable(own, mods, attrs, start, nameTok, t.text+" operator "+ty.text, model.KindMethod, &ty, f.pos, end)
		return
	}

	// Konstruktor: Name entspricht dem Typnamen und ist gefolgt von "("
	if t.kind == tokIdent && !own.isRoot && unescape(t.text) == own.name && isP(f.at(f.pos+1), "(") {
		f.pos++
		f.finishCallable(own, mods, attrs, start, t, own.name, model.KindConstructor, nil, f.pos, end)
		return
	}

	ret := readTypeAt(f.toks, f.pos, end)
	if ret.text == "" {
		f.diag(t, "Member ohne erkennbaren Typ")
		f.skipStatement(end)
		return
	}
	i := ret.endIdx
	if i >= end {
		f.pos = end
		return
	}

	nameTok := f.at(i)
	var name string
	kind := model.KindMethod
	switch {
	case nameTok.kind == tokIdent && nameTok.text == "this":
		name = "this"
		kind = model.KindIndexer
		i++
	case nameTok.kind == tokIdent && nameTok.text == "operator":
		name = "operator" + f.at(i+1).text
		i += 2
	case nameTok.kind == tokIdent:
		name = unescape(nameTok.text)
		i++
		// explizite Interface-Implementierung: IFoo<T>.Bar, IFoo.Bar
		for i < end {
			if isP(f.at(i), "<") {
				if j := matchAngle(f.toks, i, end); j > 0 {
					i = j + 1
					continue
				}
			}
			if i+1 < end && isP(f.at(i), ".") && f.at(i+1).kind == tokIdent {
				name += "." + f.at(i+1).text
				i += 2
				continue
			}
			break
		}
		if i < end && isP(f.at(i), "<") {
			if j := matchAngle(f.toks, i, end); j > 0 {
				i = j + 1
			}
		}
	default:
		f.diag(nameTok, "Member-Name erwartet")
		f.skipStatement(end)
		return
	}

	if i >= end {
		f.pos = end
		return
	}
	switch at := f.at(i); {
	case isP(at, "(") || (kind == model.KindIndexer && isP(at, "[")):
		f.finishCallable(own, mods, attrs, start, nameTok, name, kind, &ret, i, end)
	case isP(at, "{") || isP(at, "=>"):
		f.finishProperty(own, mods, attrs, start, nameTok, name, ret, i, end)
	case isP(at, "=") || isP(at, ";") || isP(at, ","):
		if kind != model.KindMethod {
			f.diag(nameTok, "Feld mit ungültigem Namen")
			f.skipStatement(end)
			return
		}
		f.finishFields(own, mods, attrs, start, ret, nameTok, i, end)
	default:
		f.diag(nameTok, "Member nicht erkannt")
		f.skipStatement(end)
	}
}

// parseEvent verarbeitet `event Typ Name;` und die Form mit Accessor-Block.
func (f *fileState) parseEvent(own owner, mods []string, attrs []attrRef, start, end int) {
	f.pos++ // event
	ty := readTypeAt(f.toks, f.pos, end)
	if ty.text == "" || ty.endIdx >= end || f.at(ty.endIdx).kind != tokIdent {
		f.diag(f.at(f.pos), "Event ohne Name")
		f.skipStatement(end)
		return
	}
	nameTok := f.at(ty.endIdx)
	name := unescape(nameTok.text)
	i := ty.endIdx + 1
	var last int
	var openIdx = -1
	if i < end && isP(f.at(i), "{") {
		openIdx = i
		last = matchClose(f.toks, i, end, "{", "}")
		if last < 0 {
			last = end - 1
		}
	} else {
		last = findTopLevel(f.toks, i, end, ";")
	}
	f.pos = last + 1
	id := model.NewSymbolID(f.lang, own.fqn+"."+name, nil)
	sym := model.Symbol{ID: id, Kind: model.KindEvent, Name: name, FQN: own.fqn + "." + name, Container: own.id, Modifiers: mods}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(last)), roleOf(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	f.emitRoutes(own, attrs, id)
	f.emitTypeRefsRole(ty, id, model.RoleHasType)
	if openIdx >= 0 {
		f.scanRefs(openIdx+1, last, id)
	}
}

// finishFields legt ein oder mehrere Felder an (`int a = 1, b;`).
func (f *fileState) finishFields(own owner, mods []string, attrs []attrRef, start int, ret typeInfo, nameTok token, i, end int) {
	first := true
	for {
		name := unescape(nameTok.text)
		stop := i
		if i < end && isP(f.at(i), "=") {
			stop = findExprEnd(f.toks, i+1, end, ";", ",")
		}
		id := model.NewSymbolID(f.lang, own.fqn+"."+name, nil)
		sym := model.Symbol{ID: id, Kind: model.KindField, Name: name, FQN: own.fqn + "." + name, Container: own.id, Modifiers: mods}
		if first {
			f.applyDoc(&sym, start)
		}
		f.emitSymbol(sym)
		f.emitLoc(id, rangeSpan(nameTok, f.at(stop)), roleOf(mods))
		f.emitRel(own.id, id, model.RelContains)
		if first {
			f.flushAttrs(attrs, id)
			f.emitRoutes(own, attrs, id)
			first = false
		}
		f.emitTypeRefsRole(ret, id, model.RoleHasType)
		if i < end && isP(f.at(i), "=") {
			f.scanRefs(i+1, stop, id)
		}
		i = stop
		if i < end && isP(f.at(i), ",") && f.at(i+1).kind == tokIdent {
			nameTok = f.at(i + 1)
			i += 2
			continue
		}
		if i < end && isP(f.at(i), ";") {
			f.pos = i + 1
		} else {
			f.pos = i
			f.diag(nameTok, "Feld nicht mit ; abgeschlossen")
		}
		return
	}
}

// finishProperty legt eine Property an. Accessoren oder Ausdruckskörper werden nach Verwendungen durchsucht.
func (f *fileState) finishProperty(own owner, mods []string, attrs []attrRef, start int, nameTok token, name string, ret typeInfo, i, end int) {
	var open, close int
	if isP(f.at(i), "{") {
		open = i
		close = matchClose(f.toks, i, end, "{", "}")
	} else {
		open = i
		close = findExprEnd(f.toks, i+1, end, ";")
	}
	if close < 0 || close >= end {
		f.diag(nameTok, "Property nicht geschlossen")
		f.pos = end
		return
	}
	f.pos = close + 1
	id := model.NewSymbolID(f.lang, own.fqn+"."+name, nil)
	sym := model.Symbol{ID: id, Kind: model.KindProperty, Name: name, FQN: own.fqn + "." + name, Container: own.id, Modifiers: mods}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(close)), roleOf(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	f.emitRoutes(own, attrs, id)
	f.emitTypeRefsRole(ret, id, model.RoleHasType)
	f.scanRefs(open+1, close, id)
	// Initialisierer nach dem Accessor-Block: `int X { get; } = 5;`
	if f.pos < end && isP(f.at(f.pos), "=") {
		j := findExprEnd(f.toks, f.pos+1, end, ";")
		f.scanRefs(f.pos+1, j, id)
		f.pos = j + 1
	}
}

// finishCallable verarbeitet Methoden, Konstruktoren, Operatoren und Indexer.
// i zeigt auf "(" oder "[". ret ist nil für Konstruktoren.
func (f *fileState) finishCallable(own owner, mods []string, attrs []attrRef, start int, nameTok token,
	name string, kind model.SymbolKind, ret *typeInfo, i, end int) {

	params, after, ok := f.parseParams(i, end)
	if !ok {
		f.diag(nameTok, "Parameterliste nicht geschlossen")
		f.pos = end
		return
	}
	bodyIdx, iniFrom, iniTo := f.skipToBody(after, end)
	open, close, ok := f.parseBodyAt(bodyIdx, end)
	if !ok {
		f.diag(nameTok, "Kein Körper erkannt")
		f.pos = end
		return
	}
	f.pos = close + 1

	fqn := own.fqn + "." + name
	types := make([]string, len(params))
	for k, p := range params {
		types[k] = p.ty.text
	}
	id := model.NewSymbolID(f.lang, fqn, types)
	sym := model.Symbol{
		ID: id, Kind: kind, Name: name, FQN: fqn, Container: own.id,
		Modifiers: mods, Signature: buildSignature(mods, ret, name, params),
	}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(close)), roleOf(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	f.emitRoutes(own, attrs, id)
	f.emitConventionAction(own, attrs, mods, id, name)
	if ret != nil {
		f.emitTypeRefsRole(*ret, id, model.RoleReturns)
	}
	for _, p := range params {
		f.emitTypeRefsRole(p.ty, id, model.RoleParamType)
	}
	if iniFrom >= 0 {
		f.scanRefs(iniFrom, iniTo, id)
	}
	if open >= 0 {
		f.scanRefs(open+1, close, id)
	}
}

// parseParams liest die Parameterliste ab i ("(" oder "[").
func (f *fileState) parseParams(i, end int) ([]paramInfo, int, bool) {
	openText := f.at(i).text
	closeText := ")"
	if openText == "[" {
		closeText = "]"
	}
	close := matchClose(f.toks, i, end, openText, closeText)
	if close < 0 {
		return nil, end, false
	}
	var params []paramInfo
	segStart := i + 1
	depth := 0
	for j := i + 1; j <= close; j++ {
		t := f.at(j)
		if j == close || (depth == 0 && isP(t, ",")) {
			if p, ok := f.parseOneParam(segStart, j); ok {
				params = append(params, p)
			}
			segStart = j + 1
			continue
		}
		switch {
		case isP(t, "(") || isP(t, "[") || isP(t, "{") || isP(t, "<"):
			depth++
		case isP(t, ")") || isP(t, "]") || isP(t, "}") || isP(t, ">"):
			depth--
		}
	}
	return params, close + 1, true
}

func (f *fileState) parseOneParam(s, e int) (paramInfo, bool) {
	for s < e && isP(f.at(s), "[") {
		m := matchClose(f.toks, s, e, "[", "]")
		if m < 0 {
			return paramInfo{}, false
		}
		s = m + 1
	}
	for s < e && f.at(s).kind == tokIdent && paramModifierSet[f.at(s).text] {
		s++
	}
	ty := readTypeAt(f.toks, s, e)
	if ty.text == "" {
		return paramInfo{}, false
	}
	name := ""
	if ty.endIdx < e && f.at(ty.endIdx).kind == tokIdent {
		name = unescape(f.at(ty.endIdx).text)
	}
	return paramInfo{ty: ty, name: name}, true
}

// skipToBody überspringt where-Klauseln und Konstruktor-Initialisierer (`: base(x)`).
// Liefert den Index des Körpers und den Bereich des Initialisierers (-1, wenn keiner).
func (f *fileState) skipToBody(i, end int) (int, int, int) {
	iniFrom, iniTo := -1, -1
	for i < end {
		t := f.at(i)
		switch {
		case isIdentAt(f, i, "where"):
			i = findTopLevel(f.toks, i, end, "{", "=>", ";")
		case isP(t, ":"):
			iniFrom = i + 1
			i = findTopLevel(f.toks, i+1, end, "{", "=>", ";")
			iniTo = i
		default:
			return i, iniFrom, iniTo
		}
	}
	return i, iniFrom, iniTo
}

// parseBodyAt erkennt den Körper: Block, Ausdruckskörper oder Semikolon.
// Liefert open (-1 bei Semikolon) und close (Index des letzten Tokens).
func (f *fileState) parseBodyAt(i, end int) (int, int, bool) {
	if i >= end {
		return -1, -1, false
	}
	t := f.at(i)
	switch {
	case isP(t, "{"):
		close := matchClose(f.toks, i, end, "{", "}")
		if close < 0 {
			return -1, -1, false
		}
		return i, close, true
	case isP(t, "=>"):
		close := findTopLevel(f.toks, i+1, end, ";")
		if close >= end {
			return -1, -1, false
		}
		return i, close, true
	case isP(t, ";"):
		return -1, i, true
	}
	return -1, -1, false
}

func buildSignature(mods []string, ret *typeInfo, name string, params []paramInfo) string {
	var parts []string
	parts = append(parts, mods...)
	if ret != nil && ret.text != "" {
		parts = append(parts, ret.text)
	}
	parts = append(parts, name)
	var ps []string
	for _, p := range params {
		if p.name != "" {
			ps = append(ps, p.ty.text+" "+p.name)
		} else {
			ps = append(ps, p.ty.text)
		}
	}
	return strings.Join(parts, " ") + "(" + strings.Join(ps, ", ") + ")"
}

// emitTypeRefs legt Typverweise für alle Namen im Typausdruck an, inklusive Generic-Argumente.
// Qualifizierte Namen liefern nur das letzte Segment.
func (f *fileState) emitTypeRefs(ty typeInfo, enc model.SymbolID) {
	f.emitTypeRefsRole(ty, enc, "")
}

// emitTypeRefsRole wie emitTypeRefs. Der erste Verweis (Kopf des Typs) trägt die Rolle.
func (f *fileState) emitTypeRefsRole(ty typeInfo, enc model.SymbolID, role string) {
	for k := ty.startIdx; k < ty.endIdx; k++ {
		t := f.at(k)
		if t.kind != tokIdent {
			continue
		}
		name := unescape(t.text)
		if model.IsCSharpKeywordType(name) || name == "void" || name == "dynamic" {
			continue
		}
		// Punktkette Shop.Models.Order: ein Verweis mit dem vollen Namen, bis zu einem Generic-Argument.
		last := k
		for last+2 < ty.endIdx && isP(f.at(last+1), ".") && f.at(last+2).kind == tokIdent {
			last += 2
			name += "." + unescape(f.at(last).text)
		}
		f.emitRef(model.Reference{
			Kind: model.RefTypeRef, TargetName: name, Span: rangeSpan(t, f.at(last)),
			Enclosing: enc, Confidence: model.ConfHeuristic,
			Arity: genericArityIn(f.toks, last, ty.endIdx), Role: role,
		})
		role = ""
		k = last
	}
}
