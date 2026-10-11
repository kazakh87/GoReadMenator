package tsjs

import "goreadmenator/internal/model"

// parseMembers verarbeitet die Mitglieder einer Klasse oder eines Interfaces bis close (exklusiv).
func (f *fileState) parseMembers(own owner, close int, isIface bool) {
	for f.pos < close {
		before := f.pos
		attrs := f.parseDecorators(own, close)
		start := f.pos
		mods := f.parseMemberMods(close)
		t := f.at(f.pos)
		switch {
		case f.pos >= close:
		case isP(t, ";"):
			f.pos++
		case isWord(t, "static") && isP(f.at(f.pos+1), "{"):
			// static-Block
			if c := matchClose(f.toks, f.pos+1, close, "{", "}"); c > 0 {
				f.scanRefs(f.pos+2, c, own.id)
				f.pos = c + 1
			} else {
				f.pos = close
			}
		case isP(t, "["), isP(t, "("), isP(t, "<"), isWord(t, "new") && isP(f.at(f.pos+1), "("):
			// Index-Signatur, Aufrufsignatur, Konstruktor-Signatur: nicht indizieren
			f.pos = findStmtEnd(f.toks, f.pos, close)
			f.finishStmt(close)
		default:
			f.parseMemberDecl(own, mods, attrs, start, close, isIface)
		}
		if f.pos == before {
			f.pos++
		}
	}
}

// parseMemberMods liest Modifikatoren, wenn ihnen ein Name folgt (`static foo`, nicht `static(`).
func (f *fileState) parseMemberMods(end int) []string {
	var mods []string
	for f.pos < end {
		t := f.at(f.pos)
		if t.kind != tokIdent || !modifierWords[t.text] {
			break
		}
		n := f.at(f.pos + 1)
		if !(n.kind == tokIdent || n.kind == tokString || isP(n, "[") || isP(n, "*")) {
			break
		}
		mods = append(mods, t.text)
		f.pos++
	}
	return mods
}

// parseMemberDecl verarbeitet ein Mitglied: Methode, Konstruktor, Accessor oder Feld/Property.
func (f *fileState) parseMemberDecl(own owner, mods []string, attrs []attrRef, start, end int, isIface bool) {
	t := f.at(f.pos)
	kind := model.KindMethod
	if (isWord(t, "get") || isWord(t, "set")) && (f.at(f.pos+1).kind == tokIdent || f.at(f.pos+1).kind == tokString) {
		kind = model.KindProperty
		f.pos++
		t = f.at(f.pos)
	}
	if isP(t, "*") {
		f.pos++
		t = f.at(f.pos)
	}
	nameTok := t
	var name string
	switch {
	case t.kind == tokIdent || t.kind == tokString:
		name = t.text
		f.pos++
	case isP(t, "["):
		// berechneter Name: überspringen
		f.pos = findStmtEnd(f.toks, f.pos, end)
		f.finishStmt(end)
		return
	default:
		f.diag(t, "Mitgliedsname erwartet")
		f.pos = findStmtEnd(f.toks, f.pos, end)
		f.finishStmt(end)
		return
	}
	if isP(f.at(f.pos), "?") || isP(f.at(f.pos), "!") {
		f.pos++
	}

	if isP(f.at(f.pos), "(") || isP(f.at(f.pos), "<") {
		if kind == model.KindMethod && name == "constructor" {
			kind = model.KindConstructor
		}
		f.parseMethodRest(own, mods, attrs, start, nameTok, name, kind, end, isIface)
		return
	}

	// Feld oder Property
	var ty typeInfo
	if isP(f.at(f.pos), ":") {
		ty = readMemberType(f.toks, f.pos+1, end)
		f.pos = ty.endIdx
	}
	fqn := own.prefix + name
	id := model.NewSymbolID(f.lang, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindProperty, Name: name, FQN: fqn, Container: own.id, Modifiers: mods,
		DocSource: "inline"}
	if ty.text != "" {
		sym.Signature = name + ": " + ty.text
	}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(f.pos-1)), roleFor(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)
	if ty.text != "" {
		f.emitTypeRefsRole(ty.startIdx, ty.endIdx, id, model.RoleHasType)
	}
	if isP(f.at(f.pos), "=") {
		// Klassenfelder enden mit ; oder Zeilenumbruch, nie mit Komma (Generics im Typ würden sonst trennen)
		e := findStmtEnd(f.toks, f.pos+1, end)
		f.scanRefs(f.pos+1, e, id)
		f.pos = e
	}
	f.finishStmt(end)
}

// parseMethodRest verarbeitet Parameter, Rückgabetyp und optionalen Körper einer Methode.
func (f *fileState) parseMethodRest(own owner, mods []string, attrs []attrRef, start int, nameTok token,
	name string, kind model.SymbolKind, end int, isIface bool) {

	f.parseTypeParams()
	if !isP(f.at(f.pos), "(") {
		f.diag(nameTok, "Methode ohne Parameterliste")
		f.pos = findStmtEnd(f.toks, f.pos, end)
		f.finishStmt(end)
		return
	}
	params, after := f.parseParams(f.pos, end)
	f.pos = after
	var ret typeInfo
	if isP(f.at(f.pos), ":") {
		ret = readMemberType(f.toks, f.pos+1, end)
		f.pos = ret.endIdx
	}
	open, close := f.parseBlockOrNone(f.pos, end)
	if open < 0 && close >= 0 && isP(f.at(close), ";") {
		f.pos = close + 1
	} else {
		f.pos = close + 1
	}
	f.emitCallable(own, mods, attrs, start, nameTok, name, kind, params, ret, open, close)
}

// readMemberType liest einen Typ in einem Klassen- oder Interface-Körper. Endet bei `;`, `{`, `}`,
// `=`, Komma auf Tiefe 0 oder einem Zeilenumbruch vor einem neuen Mitglied.
func readMemberType(toks []token, i, end int) typeInfo {
	paren, angle := 0, 0
	j := i
	for ; j < end && j < len(toks); j++ {
		t := toks[j]
		if j > i && paren == 0 && angle == 0 && t.line > toks[j-1].endLine && (t.kind == tokIdent || t.kind == tokString || isP(t, "[") || isP(t, "@")) {
			prev := toks[j-1]
			if !(prev.kind == tokPunct && prev.text != ")" && prev.text != "]" && prev.text != ">" && prev.text != "}") {
				break
			}
			if prev.kind == tokPunct && (prev.text == ")" || prev.text == "]" || prev.text == ">" || prev.text == "}") {
				break
			}
		}
		if t.kind != tokPunct {
			continue
		}
		if isTypeBraceStart(toks, j, i) {
			paren++
			continue
		}
		if paren == 0 && angle == 0 {
			switch t.text {
			case ";", "{", "}", "=", ",":
				return typeInfo{text: joinTokens(toks[i:j]), startIdx: i, endIdx: j}
			}
		}
		switch t.text {
		case "(", "[":
			paren++
		case ")", "]":
			if paren == 0 {
				return typeInfo{text: joinTokens(toks[i:j]), startIdx: i, endIdx: j}
			}
			paren--
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		}
	}
	return typeInfo{text: joinTokens(toks[i:j]), startIdx: i, endIdx: j}
}
