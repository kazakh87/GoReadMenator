package tsjs

import (
	"goreadmenator/internal/model"
)

// tsParam ist ein Parameter. name ist leer bei Destrukturierung.
type tsParam struct {
	name string
	ty   typeInfo
}

// parseFunction verarbeitet `function name(...)`. Ohne Körper ist es eine Überladungssignatur.
func (f *fileState) parseFunction(own owner, mods []string, attrs []attrRef, start, end int) {
	f.pos++ // function
	if isP(f.at(f.pos), "*") {
		f.pos++
	}
	nameTok := f.at(f.pos)
	name := "default"
	if nameTok.kind == tokIdent {
		name = nameTok.text
		f.pos++
	} else {
		nameTok = f.at(f.pos - 1)
	}
	f.parseFunctionAs(own, mods, attrs, start, end, name, nameTok)
}

// parseFunctionAs verarbeitet Parameter, Rückgabetyp und Körper ab der aktuellen Position.
func (f *fileState) parseFunctionAs(own owner, mods []string, attrs []attrRef, start, end int, name string, nameTok token) {
	f.parseTypeParams()
	if !isP(f.at(f.pos), "(") {
		f.diag(nameTok, "Funktion ohne Parameterliste")
		f.pos = findStmtEnd(f.toks, f.pos, end)
		f.finishStmt(end)
		return
	}
	params, after := f.parseParams(f.pos, end)
	f.pos = after
	var ret typeInfo
	if isP(f.at(f.pos), ":") {
		ret = readType(f.toks, f.pos+1, end, "{", ";")
		f.pos = ret.endIdx
	}
	open, close := f.parseBlockOrNone(f.pos, end)
	f.pos = close + 1
	f.emitCallable(own, mods, attrs, start, nameTok, name, model.KindFunction, params, ret, open, close)
}

// emitCallable legt ein Funktions- oder Methodensymbol an und durchsucht Typen und Körper.
func (f *fileState) emitCallable(own owner, mods []string, attrs []attrRef, start int, nameTok token,
	name string, kind model.SymbolKind, params []tsParam, ret typeInfo, open, close int) {

	fqn := own.prefix + name
	var id model.SymbolID
	if kind == model.KindProperty {
		// get/set-Accessoren teilen sich die ID des Props, ohne Parameterliste
		id = model.NewSymbolID(f.lang, fqn, nil)
	} else {
		types := make([]string, len(params))
		for k, p := range params {
			types[k] = p.ty.text
			if types[k] == "" {
				types[k] = "any"
			}
		}
		id = model.NewSymbolID(f.lang, fqn, types)
	}
	sym := model.Symbol{
		ID: id, Kind: kind, Name: name, FQN: fqn, Container: own.id,
		Modifiers: mods, Signature: buildSignature(mods, name, params, ret),
	}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	endTok := f.at(close)
	if close < 0 {
		endTok = nameTok
	}
	f.emitLoc(id, rangeSpan(nameTok, endTok), roleFor(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	for _, p := range params {
		if p.ty.text != "" {
			f.emitTypeRefsRole(p.ty.startIdx, p.ty.endIdx, id, model.RoleParamType)
		}
	}
	if ret.text != "" {
		f.emitTypeRefsRole(ret.startIdx, ret.endIdx, id, model.RoleReturns)
	}
	if open >= 0 {
		f.scanRefs(open+1, close, id)
	}
}

// parseBlockOrNone liest einen Funktionskörper. Liefert open (-1 ohne Körper) und close.
// Ohne Körper ist close das Semikolon oder das Token vor der nächsten Anweisung.
func (f *fileState) parseBlockOrNone(i, end int) (int, int) {
	t := f.at(i)
	switch {
	case isP(t, "{"):
		close := matchClose(f.toks, i, end, "{", "}")
		if close < 0 {
			f.diag(t, "Körper nicht geschlossen")
			return i, recoverBodyEnd(f.toks, i, end)
		}
		return i, close
	case isP(t, ";"):
		return -1, i
	}
	return -1, i - 1
}

// parseParams liest eine Parameterliste ab i ("(") und liefert die Parameter und den Index danach.
func (f *fileState) parseParams(i, end int) ([]tsParam, int) {
	close := matchClose(f.toks, i, end, "(", ")")
	if close < 0 {
		f.diag(f.at(i), "Parameterliste nicht geschlossen")
		return nil, end
	}
	var params []tsParam
	p := i + 1
	for p < close {
		before := p
		// Decorator und Sichtbarkeitsmodifikatoren (Konstruktor-Parameter)
		for isP(f.at(p), "@") {
			p++
			_, p = readNameChain(f.toks, p, close)
			if isP(f.at(p), "(") {
				if j := matchClose(f.toks, p, close, "(", ")"); j > 0 {
					p = j + 1
				}
			}
		}
		for p < close && f.at(p).kind == tokIdent && (modifierWords[f.at(p).text] || f.at(p).text == "readonly") &&
			f.at(p+1).kind == tokIdent {
			p++
		}
		if isP(f.at(p), "...") {
			p++
		}
		name := ""
		switch t := f.at(p); {
		case t.kind == tokIdent:
			name = t.text
			p++
		case isP(t, "{") || isP(t, "["):
			if j := matchClose(f.toks, p, close, t.text, map[string]string{"{": "}", "[": "]"}[t.text]); j > 0 {
				p = j + 1
			} else {
				p = close
			}
		}
		if isP(f.at(p), "?") {
			p++
		}
		var ty typeInfo
		if isP(f.at(p), ":") {
			ty = readType(f.toks, p+1, close, ",", "=")
			p = ty.endIdx
		}
		if isP(f.at(p), "=") {
			p = skipType(f.toks, p+1, close, ",")
		}
		if name != "" || ty.text != "" {
			params = append(params, tsParam{name: name, ty: ty})
		}
		if isP(f.at(p), ",") {
			p++
		} else if p == before {
			p++
		}
	}
	return params, close + 1
}

// parseTypeParams überspringt `<T extends X, U = Y>` und merkt sich die Namen.
func (f *fileState) parseTypeParams() []string {
	if !isP(f.at(f.pos), "<") {
		return nil
	}
	j := matchAngle(f.toks, f.pos, len(f.toks))
	if j < 0 {
		return nil
	}
	var names []string
	expectName := true
	depth := 0
	for k := f.pos + 1; k < j; k++ {
		t := f.at(k)
		if t.kind == tokPunct {
			switch t.text {
			case "<", "(", "[", "{":
				depth++
			case ">", ")", "]", "}":
				depth--
			case ",":
				if depth == 0 {
					expectName = true
				}
			}
			continue
		}
		if expectName && depth == 0 && t.kind == tokIdent {
			names = append(names, t.text)
			f.tparams[t.text] = true
			expectName = false
		}
	}
	f.pos = j + 1
	return names
}

// parseClass verarbeitet Klassen mit Basisangaben und Mitgliedern.
func (f *fileState) parseClass(own owner, mods []string, attrs []attrRef, start, end int) {
	f.pos++ // class
	nameTok := f.at(f.pos)
	name := "default"
	if nameTok.kind == tokIdent {
		name = nameTok.text
		f.pos++
	} else {
		nameTok = f.at(f.pos - 1)
	}
	f.parseTypeParams()
	var bases []baseRef
	var impls []baseRef
	if isWord(f.at(f.pos), "extends") {
		f.pos++
		bases = f.parseBaseList(end)
	}
	if isWord(f.at(f.pos), "implements") {
		f.pos++
		impls = f.parseBaseList(end)
	}
	if !isP(f.at(f.pos), "{") {
		f.diag(nameTok, "Klasse ohne Körper")
		f.finishStmt(end)
		return
	}
	open := f.pos
	close := matchClose(f.toks, open, end, "{", "}")
	if close < 0 {
		f.diag(nameTok, "Klassen-Körper nicht geschlossen")
		close = recoverBodyEnd(f.toks, open, end)
	}
	fqn := own.prefix + name
	id := model.NewSymbolID(f.lang, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindClass, Name: name, FQN: fqn, Container: own.id, Modifiers: mods,
		Signature: "class " + name, DocSource: "inline"}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(close)), roleFor(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	for _, b := range bases {
		f.emitRef(model.Reference{Kind: model.RefInherits, TargetName: b.name, Span: rangeSpan(b.tok, b.endTok),
			Enclosing: id, Confidence: model.ConfHeuristic})
	}
	for _, b := range impls {
		f.emitRef(model.Reference{Kind: model.RefImplements, TargetName: b.name, Span: rangeSpan(b.tok, b.endTok),
			Enclosing: id, Confidence: model.ConfHeuristic})
	}
	f.pos = open + 1
	f.parseMembers(owner{id: id, prefix: fqn + ".", name: name}, close, false)
	f.pos = close + 1
}

// parseBaseList liest `A, B.C<T>, D` für extends/implements.
func (f *fileState) parseBaseList(end int) []baseRef {
	var out []baseRef
	for f.pos < end {
		name, next := readNameChain(f.toks, f.pos, end)
		if name == "" {
			break
		}
		endTok := f.at(next - 1)
		if isP(f.at(next), "<") {
			if j := matchAngle(f.toks, next, end); j > 0 {
				next = j + 1
			}
		}
		out = append(out, baseRef{name: lastSegment(name), tok: f.at(f.pos), endTok: endTok})
		f.pos = next
		if isP(f.at(f.pos), ",") {
			f.pos++
			continue
		}
		break
	}
	return out
}

// parseInterface verarbeitet Interface-Deklarationen mit Mitgliedern.
func (f *fileState) parseInterface(own owner, mods []string, attrs []attrRef, start, end int) {
	f.pos++ // interface
	nameTok := f.at(f.pos)
	if nameTok.kind != tokIdent {
		f.diag(nameTok, "Interface ohne Name")
		f.finishStmt(end)
		return
	}
	name := nameTok.text
	f.pos++
	f.parseTypeParams()
	var bases []baseRef
	if isWord(f.at(f.pos), "extends") {
		f.pos++
		bases = f.parseBaseList(end)
	}
	if !isP(f.at(f.pos), "{") {
		f.diag(nameTok, "Interface ohne Körper")
		f.pos = findStmtEnd(f.toks, f.pos, end)
		f.finishStmt(end)
		return
	}
	open := f.pos
	close := matchClose(f.toks, open, end, "{", "}")
	if close < 0 {
		f.diag(nameTok, "Interface-Körper nicht geschlossen")
		close = recoverBodyEnd(f.toks, open, end)
	}
	fqn := own.prefix + name
	id := model.NewSymbolID(f.lang, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindInterface, Name: name, FQN: fqn, Container: own.id, Modifiers: mods,
		Signature: "interface " + name, DocSource: "inline"}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(close)), roleFor(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	for _, b := range bases {
		f.emitRef(model.Reference{Kind: model.RefInherits, TargetName: b.name, Span: rangeSpan(b.tok, b.endTok),
			Enclosing: id, Confidence: model.ConfHeuristic})
	}
	f.pos = open + 1
	f.parseMembers(owner{id: id, prefix: fqn + ".", name: name}, close, true)
	f.pos = close + 1
}

// parseEnum verarbeitet enum-Deklarationen. Mitglieder werden als Felder geführt.
func (f *fileState) parseEnum(own owner, mods []string, attrs []attrRef, start, end int) {
	f.pos++ // enum
	nameTok := f.at(f.pos)
	if nameTok.kind != tokIdent {
		f.diag(nameTok, "Enum ohne Name")
		f.finishStmt(end)
		return
	}
	name := nameTok.text
	f.pos++
	if !isP(f.at(f.pos), "{") {
		f.finishStmt(end)
		return
	}
	open := f.pos
	close := matchClose(f.toks, open, end, "{", "}")
	if close < 0 {
		f.pos = end
		return
	}
	fqn := own.prefix + name
	id := model.NewSymbolID(f.lang, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindEnum, Name: name, FQN: fqn, Container: own.id, Modifiers: mods, DocSource: "inline"}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(close)), roleFor(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)

	member := owner{id: id, prefix: fqn + ".", name: name}
	for k := open + 1; k < close; {
		t := f.at(k)
		if t.kind != tokIdent && t.kind != tokString {
			k++
			continue
		}
		stop := findTopLevel(f.toks, k+1, close, ",")
		if isP(f.at(k+1), "=") {
			f.scanRefs(k+2, stop, id)
		}
		mname := t.text
		mid := model.NewSymbolID(f.lang, member.prefix+mname, nil)
		f.emitSymbol(model.Symbol{ID: mid, Kind: model.KindField, Name: mname, FQN: member.prefix + mname, Container: id})
		f.emitLoc(mid, rangeSpan(t, f.at(stop-1)), model.RoleDecl)
		f.emitRel(id, mid, model.RelContains)
		k = stop + 1
	}
	f.pos = close + 1
}

// parseTypeAlias verarbeitet `type Name<T> = ...`.
func (f *fileState) parseTypeAlias(own owner, mods []string, attrs []attrRef, start, end int) {
	f.pos++ // type
	nameTok := f.at(f.pos)
	name := nameTok.text
	f.pos++
	f.parseTypeParams()
	if !isP(f.at(f.pos), "=") {
		f.finishStmt(end)
		return
	}
	valStart := f.pos + 1
	e := findStmtEnd(f.toks, valStart, end)
	fqn := own.prefix + name
	id := model.NewSymbolID(f.lang, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindTypeAlias, Name: name, FQN: fqn, Container: own.id, Modifiers: mods,
		Signature: "type " + name, DocSource: "inline"}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(e-1)), roleFor(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	f.emitTypeRefs(valStart, e, id)
	f.pos = e
	f.finishStmt(end)
}

// parseVariables verarbeitet const/let/var mit einem oder mehreren Deklaratoren.
// Funktionswerte (Pfeilfunktion, function-Ausdruck) werden als Funktion geführt.
func (f *fileState) parseVariables(own owner, mods []string, attrs []attrRef, start, end int) {
	f.pos++ // const/let/var
	for {
		nameTok := f.at(f.pos)
		if nameTok.kind != tokIdent {
			// Destrukturierung: nur Verweise übernehmen
			e := findStmtEnd(f.toks, f.pos, end)
			f.scanRefs(f.pos, e, own.id)
			f.pos = e
			f.finishStmt(end)
			return
		}
		name := nameTok.text
		f.pos++
		var ty typeInfo
		if isP(f.at(f.pos), ":") {
			ty = readType(f.toks, f.pos+1, end, "=", ";", ",")
			f.pos = ty.endIdx
		}
		fqn := own.prefix + name
		id := model.NewSymbolID(f.lang, fqn, nil)

		if isP(f.at(f.pos), "=") {
			initStart := f.pos + 1
			if f.isFunctionValue(initStart, end) {
				f.parseFunctionValue(own, mods, attrs, start, nameTok, name, initStart, end)
			} else {
				e := f.exprEnd(initStart, end)
				sym := model.Symbol{ID: id, Kind: model.KindVariable, Name: name, FQN: fqn, Container: own.id, Modifiers: mods, DocSource: "inline"}
				if ty.text != "" {
					sym.Signature = name + ": " + ty.text
				}
				f.applyDoc(&sym, start)
				f.emitSymbol(sym)
				f.emitLoc(id, rangeSpan(nameTok, f.at(e-1)), roleFor(mods))
				f.emitRel(own.id, id, model.RelContains)
				f.flushAttrs(attrs, id)
				if ty.text != "" {
					f.emitTypeRefsRole(ty.startIdx, ty.endIdx, id, model.RoleHasType)
				}
				f.scanRefs(initStart, e, id)
				f.pos = e
			}
		} else {
			sym := model.Symbol{ID: id, Kind: model.KindVariable, Name: name, FQN: fqn, Container: own.id, Modifiers: mods, DocSource: "inline"}
			f.applyDoc(&sym, start)
			f.emitSymbol(sym)
			f.emitLoc(id, tokSpan(nameTok), roleFor(mods))
			f.emitRel(own.id, id, model.RelContains)
			f.flushAttrs(attrs, id)
			if ty.text != "" {
				f.emitTypeRefsRole(ty.startIdx, ty.endIdx, id, model.RoleHasType)
			}
		}

		if isP(f.at(f.pos), ",") {
			f.pos++
			continue
		}
		f.finishStmt(end)
		return
	}
}

// exprEnd sucht das Ende eines Ausdrucks: Komma oder Semikolon auf Tiefe 0, schließende Klammer
// oder Zeilenumbruch vor einer neuen Anweisung.
func (f *fileState) exprEnd(i, end int) int {
	depth := 0
	for j := i; j < end && j < len(f.toks); j++ {
		t := f.toks[j]
		if j > i && depth == 0 && t.line > f.toks[j-1].endLine && startsStatement(t) {
			return j
		}
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth == 0 {
				return j
			}
			depth--
		case ",", ";":
			if depth == 0 {
				return j
			}
		}
	}
	return end
}

// isFunctionValue erkennt Pfeilfunktionen und function-Ausdrücke ab i.
func (f *fileState) isFunctionValue(i, end int) bool {
	if isWord(f.at(i), "async") {
		i++
	}
	t := f.at(i)
	if isWord(t, "function") {
		return true
	}
	if t.kind == tokIdent && isP(f.at(i+1), "=>") {
		return true
	}
	if isP(t, "(") {
		c := matchClose(f.toks, i, end, "(", ")")
		if c < 0 {
			return false
		}
		if isP(f.at(c+1), "=>") {
			return true
		}
		if isP(f.at(c+1), ":") {
			j := skipType(f.toks, c+2, end, "=>", ";", ",")
			return isP(f.at(j), "=>")
		}
	}
	return false
}

// parseFunctionValue legt eine Variable mit Funktionswert als Funktion an.
func (f *fileState) parseFunctionValue(own owner, mods []string, attrs []attrRef, start int, nameTok token,
	name string, i, end int) {

	f.pos = i
	if isWord(f.at(f.pos), "async") {
		f.pos++
		mods = append(mods, "async")
	}
	if isWord(f.at(f.pos), "function") {
		f.pos++
		if isP(f.at(f.pos), "*") {
			f.pos++
		}
		if f.at(f.pos).kind == tokIdent {
			f.pos++ // Name eines Funktionsausdrucks wird ignoriert
		}
		f.parseTypeParams()
		params, after := f.parseParams(f.pos, end)
		f.pos = after
		var ret typeInfo
		if isP(f.at(f.pos), ":") {
			ret = readType(f.toks, f.pos+1, end, "{", ";")
			f.pos = ret.endIdx
		}
		open, close := f.parseBlockOrNone(f.pos, end)
		f.pos = close + 1
		f.emitCallable(own, mods, attrs, start, nameTok, name, model.KindFunction, params, ret, open, close)
		return
	}

	f.parseTypeParams()
	var params []tsParam
	if f.at(f.pos).kind == tokIdent {
		params = []tsParam{{name: f.at(f.pos).text}}
		f.pos++
	} else {
		var after int
		params, after = f.parseParams(f.pos, end)
		f.pos = after
	}
	var ret typeInfo
	if isP(f.at(f.pos), ":") {
		ret = readType(f.toks, f.pos+1, end, "=>")
		f.pos = ret.endIdx
	}
	if !isP(f.at(f.pos), "=>") {
		f.diag(nameTok, "Pfeilfunktion ohne =>")
		f.pos = f.exprEnd(f.pos, end)
		return
	}
	arrow := f.pos
	var open, close int
	if isP(f.at(arrow+1), "{") {
		open = arrow + 1
		close = matchClose(f.toks, open, end, "{", "}")
		if close < 0 {
			close = end - 1
		}
		f.pos = close + 1
	} else {
		open = arrow
		e := f.exprEnd(arrow+1, end)
		close = e - 1
		f.pos = e
	}
	f.emitCallable(own, mods, attrs, start, nameTok, name, model.KindFunction, params, ret, open, close)
}
