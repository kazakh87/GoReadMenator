package vbnet

import (
	"strings"

	"goreadmenator/internal/model"
)

// parseStatements verarbeitet Anweisungen bis zur Blockende-Anweisung "End <closer>" (exklusiv).
// closer ist leer auf oberster Ebene.
func (f *fileState) parseStatements(own owner, closer string) {
	for f.pos < len(f.stmts) {
		s := f.stmts[f.pos]
		if closer != "" && isEnd(s, closer) {
			return
		}
		before := f.pos
		attrs, toks := stripAttrs(s.toks)
		mods, rest := stripMods(toks)
		if len(rest) == 0 {
			// Attributzeile vor einer Deklaration: wird an die nächste Deklaration gehängt
			if len(attrs) > 0 {
				if len(f.pendingAttrs) == 0 {
					f.pendingLine = s.line
				}
				f.pendingAttrs = append(f.pendingAttrs, attrs...)
			}
			f.pos++
			continue
		}
		if len(f.pendingAttrs) > 0 {
			attrs = append(f.pendingAttrs, attrs...)
			s.line = f.pendingLine // Dokumentation steht über der ersten Attributzeile
			f.pendingAttrs = nil
		}
		switch rest[0].lower {
		case "imports":
			f.parseImports(rest)
			f.pos++
		case "namespace":
			f.parseNamespace(own, rest)
		case "class", "module", "structure", "interface":
			f.parseType(own, s, attrs, mods, rest)
		case "enum":
			f.parseEnum(own, s, attrs, mods, rest)
		case "sub", "function":
			f.parseMethod(own, s, attrs, mods, rest)
		case "property":
			f.parseProperty(own, s, attrs, mods, rest)
		case "event":
			f.parseEvent(own, s, attrs, mods, rest)
		case "dim", "const":
			f.parseFields(own, s, attrs, mods, rest[1:])
			f.pos++
		case "inherits":
			f.parseBaseRefs(own, rest[1:], "inherits")
			f.pos++
		case "implements":
			f.parseBaseRefs(own, rest[1:], "implements")
			f.pos++
		default:
			if own.kind == "class" || own.kind == "module" || own.kind == "structure" {
				if len(rest) > 1 && rest[0].kind == tokIdent && (rest[1].lower == "as" || rest[1].text == "=" || rest[1].text == "," || rest[1].text == "(") {
					f.parseFields(own, s, attrs, mods, rest)
					f.pos++
					break
				}
			}
			f.scanRefs(rest, own.id)
			f.pos++
		}
		if f.pos == before {
			f.pos++
		}
	}
}

func (f *fileState) parseImports(rest []token) {
	if len(rest) < 2 {
		return
	}
	body := rest[1:]
	raw := joinTokens(body)
	// Alias: Imports Alias = Namespace  ->  "Alias=Namespace" (wie bei using in C#)
	if len(body) >= 3 && body[1].text == "=" {
		raw = body[0].text + "=" + joinTokens(body[2:])
	}
	raw = strings.ReplaceAll(raw, " . ", ".")
	f.res.Imports = append(f.res.Imports, model.Import{Raw: raw, Kind: model.ImportUsing})
}

func (f *fileState) parseNamespace(own owner, rest []token) {
	if len(rest) < 2 {
		f.diag(rest[0], "Namespace ohne Namen")
		f.pos++
		return
	}
	name := joinQualified(rest[1:])
	fqn := name
	if !own.isRoot {
		fqn = own.prefix + name
	}
	id := model.NewSymbolID(model.LangVBNet, fqn, nil)
	nameTok := rest[1]
	endTok := rest[len(rest)-1]
	f.pos++
	inner := owner{id: id, prefix: fqn + ".", name: name, kind: "namespace"}
	f.parseStatements(inner, "namespace")
	if f.pos < len(f.stmts) {
		endTok = f.stmts[f.pos].toks[len(f.stmts[f.pos].toks)-1]
		f.pos++
	}
	f.emitSymbol(model.Symbol{ID: id, Kind: model.KindNamespace, Name: lastPart(name), FQN: fqn, Container: own.id})
	f.emitLoc(id, rangeSpan(nameTok, endTok), model.RoleDecl)
	if !own.isRoot {
		f.emitRel(own.id, id, model.RelContains)
	}
}

// parseType verarbeitet Class, Module, Structure und Interface mit ihrem Körper.
func (f *fileState) parseType(own owner, s stmt, attrs []attrRef, mods []string, rest []token) {
	kw := rest[0].lower
	if len(rest) < 2 {
		f.diag(rest[0], "Typ ohne Namen")
		f.pos++
		return
	}
	nameTok := rest[1]
	name := nameTok.text
	fqn := own.prefix + name
	if own.isRoot {
		fqn = name
	}
	id := model.NewSymbolID(model.LangVBNet, fqn, nil)
	kind := map[string]model.SymbolKind{"class": model.KindClass, "module": model.KindClass,
		"structure": model.KindStruct, "interface": model.KindInterface}[kw]
	sym := model.Symbol{ID: id, Kind: kind, Name: name, FQN: fqn, Container: own.id, Modifiers: mods,
		Signature: strings.Join(append(append([]string{}, mods...), kw, name), " ")}
	f.applyDoc(&sym, s.line)
	f.emitSymbol(sym)
	f.flushAttrs(attrs, id)
	f.pos++
	inner := owner{id: id, prefix: fqn + ".", name: name, kind: kw}
	f.parseStatements(inner, kw)
	endTok := nameTok
	if f.pos < len(f.stmts) {
		endTok = f.stmts[f.pos].toks[len(f.stmts[f.pos].toks)-1]
		f.pos++
	}
	f.emitLoc(id, rangeSpan(nameTok, endTok), roleOf(mods))
	f.emitRel(own.id, id, model.RelContains)
}

func (f *fileState) parseEnum(own owner, s stmt, attrs []attrRef, mods []string, rest []token) {
	if len(rest) < 2 {
		f.pos++
		return
	}
	nameTok := rest[1]
	fqn := own.prefix + nameTok.text
	if own.isRoot {
		fqn = nameTok.text
	}
	id := model.NewSymbolID(model.LangVBNet, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindEnum, Name: nameTok.text, FQN: fqn, Container: own.id, Modifiers: mods}
	f.applyDoc(&sym, s.line)
	f.emitSymbol(sym)
	f.flushAttrs(attrs, id)
	f.pos++
	endTok := nameTok
	for f.pos < len(f.stmts) && !isEnd(f.stmts[f.pos], "enum") {
		m := f.stmts[f.pos]
		if len(m.toks) > 0 && m.toks[0].kind == tokIdent {
			mt := m.toks[0]
			mid := model.NewSymbolID(model.LangVBNet, fqn+"."+mt.text, nil)
			f.emitSymbol(model.Symbol{ID: mid, Kind: model.KindField, Name: mt.text, FQN: fqn + "." + mt.text, Container: id})
			f.emitLoc(mid, rangeSpan(mt, m.toks[len(m.toks)-1]), model.RoleDecl)
			f.emitRel(id, mid, model.RelContains)
			if len(m.toks) > 2 && m.toks[1].text == "=" {
				f.scanRefs(m.toks[2:], mid)
			}
		}
		f.pos++
	}
	if f.pos < len(f.stmts) {
		endTok = f.stmts[f.pos].toks[len(f.stmts[f.pos].toks)-1]
		f.pos++
	}
	f.emitLoc(id, rangeSpan(nameTok, endTok), roleOf(mods))
	f.emitRel(own.id, id, model.RelContains)
}

// parseMethod verarbeitet Sub und Function. Ohne Körper bei Interfaces und MustOverride/Declare.
func (f *fileState) parseMethod(own owner, s stmt, attrs []attrRef, mods []string, rest []token) {
	kw := rest[0].lower
	if len(rest) < 2 {
		f.pos++
		return
	}
	nameTok := rest[1]
	name := nameTok.text
	kind := model.KindMethod
	if strings.EqualFold(name, "new") {
		kind = model.KindConstructor
	}
	params, after := parseParams(rest, 2)
	var ret typeInfo
	if after < len(rest) && rest[after].lower == "as" {
		ret = readTypeTokens(rest, after+1)
	}
	types := make([]string, len(params))
	for i, p := range params {
		types[i] = p.typ.text
		if types[i] == "" {
			types[i] = "Object"
		}
	}
	fqn := own.prefix + name
	if own.isRoot {
		fqn = name
	}
	id := model.NewSymbolID(model.LangVBNet, fqn, types)
	sym := model.Symbol{ID: id, Kind: kind, Name: name, FQN: fqn, Container: own.id, Modifiers: mods,
		Signature: buildSig(mods, kw, name, params, ret)}
	f.applyDoc(&sym, s.line)
	f.emitSymbol(sym)
	f.flushAttrs(attrs, id)
	f.emitRel(own.id, id, model.RelContains)
	if ret.text != "" {
		f.emitTypeRefs(ret.toks, id, model.RoleReturns)
	}
	for _, p := range params {
		f.emitTypeRefs(p.typ.toks, id, model.RoleParamType)
	}
	f.pos++
	hasBody := own.kind != "interface" && !hasWord(mods, "mustoverride") && !hasWord(rest, "declare")
	endTok := s.toks[len(s.toks)-1]
	if hasBody {
		endTok = f.parseMemberBody(id, kw)
	}
	f.emitLoc(id, rangeSpan(nameTok, endTok), roleOf(mods))
}

// parseProperty verarbeitet Property mit Get/Set-Körper oder ohne Körper (Interface, Auto-Property).
func (f *fileState) parseProperty(own owner, s stmt, attrs []attrRef, mods []string, rest []token) {
	if len(rest) < 2 {
		f.pos++
		return
	}
	nameTok := rest[1]
	name := nameTok.text
	params, after := parseParams(rest, 2)
	var ty typeInfo
	if after < len(rest) && rest[after].lower == "as" {
		ty = readTypeTokens(rest, after+1)
	}
	var types []string
	for _, p := range params {
		t := p.typ.text
		if t == "" {
			t = "Object"
		}
		types = append(types, t)
	}
	fqn := own.prefix + name
	if own.isRoot {
		fqn = name
	}
	kind := model.KindProperty
	var id model.SymbolID
	if len(params) > 0 {
		id = model.NewSymbolID(model.LangVBNet, fqn, types)
	} else {
		id = model.NewSymbolID(model.LangVBNet, fqn, nil)
	}
	sym := model.Symbol{ID: id, Kind: kind, Name: name, FQN: fqn, Container: own.id, Modifiers: mods,
		Signature: buildSig(mods, "property", name, params, ty)}
	f.applyDoc(&sym, s.line)
	f.emitSymbol(sym)
	f.flushAttrs(attrs, id)
	f.emitRel(own.id, id, model.RelContains)
	if ty.text != "" {
		f.emitTypeRefs(ty.toks, id, model.RoleHasType)
	}
	f.pos++
	endTok := s.toks[len(s.toks)-1]
	// Körper nur, wenn Get/Set oder End Property folgt. Auto-Properties haben keinen Körper.
	if own.kind != "interface" && f.pos < len(f.stmts) && (f.stmts[f.pos].toks[0].lower == "get" ||
		f.stmts[f.pos].toks[0].lower == "set" || isEnd(f.stmts[f.pos], "property")) {
		endTok = f.parseMemberBody(id, "property")
	}
	f.emitLoc(id, rangeSpan(nameTok, endTok), roleOf(mods))
}

func (f *fileState) parseEvent(own owner, s stmt, attrs []attrRef, mods []string, rest []token) {
	if len(rest) < 2 {
		f.pos++
		return
	}
	nameTok := rest[1]
	fqn := own.prefix + nameTok.text
	if own.isRoot {
		fqn = nameTok.text
	}
	id := model.NewSymbolID(model.LangVBNet, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindEvent, Name: nameTok.text, FQN: fqn, Container: own.id, Modifiers: mods}
	f.applyDoc(&sym, s.line)
	f.emitSymbol(sym)
	f.flushAttrs(attrs, id)
	f.emitRel(own.id, id, model.RelContains)
	if _, after := parseParams(rest, 2); after < len(rest) && rest[after].lower == "as" {
		f.emitTypeRefs(readTypeTokens(rest, after+1).toks, id, model.RoleHasType)
	}
	f.emitLoc(id, rangeSpan(nameTok, s.toks[len(s.toks)-1]), roleOf(mods))
	f.pos++
}

// parseFields verarbeitet Dim/Const und Felder mit Sichtbarkeit: "a As Integer, b = 2".
func (f *fileState) parseFields(own owner, s stmt, attrs []attrRef, mods []string, toks []token) {
	// Mehrere Deklaratoren, getrennt durch Komma auf Klammertiefe 0
	start := 0
	depth := 0
	for i := 0; i <= len(toks); i++ {
		if i == len(toks) || (depth == 0 && toks[i].text == ",") {
			f.fieldDeclarator(own, s, attrs, mods, toks[start:i], start == 0)
			start = i + 1
			continue
		}
		switch toks[i].text {
		case "(":
			depth++
		case ")":
			depth--
		}
	}
}

func (f *fileState) fieldDeclarator(own owner, s stmt, attrs []attrRef, mods []string, d []token, first bool) {
	if len(d) == 0 || d[0].kind != tokIdent {
		return
	}
	nameTok := d[0]
	fqn := own.prefix + nameTok.text
	if own.isRoot {
		fqn = nameTok.text
	}
	id := model.NewSymbolID(model.LangVBNet, fqn, nil)
	sym := model.Symbol{ID: id, Kind: model.KindField, Name: nameTok.text, FQN: fqn, Container: own.id, Modifiers: mods}
	if first {
		f.applyDoc(&sym, s.line)
		f.flushAttrs(attrs, id)
	}
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, d[len(d)-1]), roleOf(mods))
	f.emitRel(own.id, id, model.RelContains)
	for i := 1; i < len(d); i++ {
		if d[i].lower == "as" {
			f.emitTypeRefs(readTypeTokens(d, i+1).toks, id, model.RoleHasType)
			break
		}
		if d[i].text == "=" {
			f.scanRefs(d[i+1:], id)
			break
		}
	}
}

// parseBaseRefs verarbeitet "Inherits X" und "Implements A, B" im Körper eines Typs.
func (f *fileState) parseBaseRefs(own owner, toks []token, which string) {
	kind := model.RefInherits
	if which == "implements" {
		kind = model.RefImplements
	}
	for _, t := range splitTop(toks) {
		if len(t) == 0 || t[0].kind != tokIdent {
			continue
		}
		name := t[len(t)-1].text
		f.emitRef(model.Reference{Kind: kind, TargetName: name, Span: rangeSpan(t[0], t[len(t)-1]),
			Enclosing: own.id, Confidence: model.ConfHeuristic})
	}
}

// parseMemberBody läuft durch den Körper eines Members bis zur passenden Endanweisung und liefert das Ende.
// kw ist "sub", "function" oder "property".
func (f *fileState) parseMemberBody(enc model.SymbolID, kw string) token {
	depth := 0
	var last token
	for f.pos < len(f.stmts) {
		s := f.stmts[f.pos]
		last = s.toks[len(s.toks)-1]
		if depth == 0 && isEnd(s, kw) {
			f.pos++
			return last
		}
		// Wiedererkennung nach Fehlern: beginnt auf Ebene 0 eine Deklaration oder endet der Typ,
		// fehlt das End des Körpers. Die Deklaration bleibt erhalten.
		if depth == 0 && (startsDeclaration(s) || isTypeEnd(s)) {
			f.diag(s.toks[0], "Körper nicht mit End "+kw+" geschlossen")
			return last
		}
		if blockOpener(s) {
			depth++
		} else if len(s.toks) > 0 && (s.toks[0].lower == "end" || s.toks[0].lower == "next" || s.toks[0].lower == "loop") {
			if depth > 0 {
				depth--
			}
		}
		_, body := stripAttrs(s.toks)
		// Kopfzeilen der Accessoren (Get, Set(value)) sind keine Verwendungen
		if len(body) > 0 && (body[0].lower == "get" || body[0].lower == "set") {
			body = body[:0]
		}
		f.scanRefs(body, enc)
		f.pos++
	}
	return last
}

// splitTop teilt Tokens an Kommas auf Klammertiefe 0.
func splitTop(toks []token) [][]token {
	var out [][]token
	depth := 0
	start := 0
	for i := 0; i <= len(toks); i++ {
		if i == len(toks) || (depth == 0 && toks[i].text == ",") {
			if i > start {
				out = append(out, toks[start:i])
			}
			start = i + 1
			continue
		}
		switch toks[i].text {
		case "(":
			depth++
		case ")":
			depth--
		}
	}
	return out
}

func joinQualified(toks []token) string {
	var b strings.Builder
	for _, t := range toks {
		if t.text == "." || t.kind == tokIdent {
			b.WriteString(t.text)
		}
	}
	return b.String()
}

func lastPart(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func hasWord(ts any, w string) bool {
	switch v := ts.(type) {
	case []string:
		for _, x := range v {
			if x == w {
				return true
			}
		}
	case []token:
		for _, t := range v {
			if t.lower == w {
				return true
			}
		}
	}
	return false
}

func roleOf(mods []string) model.LocationRole {
	if hasWord(mods, "partial") {
		return model.RolePartial
	}
	return model.RoleDecl
}

// startsDeclaration erkennt Deklarationsanfänge. Eine Function mit "(" direkt danach ist ein Lambda
// im Ausdruck und zählt nicht.
func startsDeclaration(s stmt) bool {
	_, toks := stripAttrs(s.toks)
	_, rest := stripMods(toks)
	if len(rest) == 0 {
		return false
	}
	switch rest[0].lower {
	case "sub", "function":
		return len(rest) > 1 && rest[1].text != "("
	case "class", "namespace", "module", "structure", "interface", "enum":
		return true
	}
	return false
}

// isTypeEnd erkennt "End Class" und ähnliche Blockenden.
func isTypeEnd(s stmt) bool {
	for _, k := range []string{"class", "namespace", "module", "structure", "interface", "enum"} {
		if isEnd(s, k) {
			return true
		}
	}
	return false
}
