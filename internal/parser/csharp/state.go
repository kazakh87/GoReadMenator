package csharp

import (
	"sort"
	"strconv"
	"strings"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// fileState hält den Zustand während des Parsens einer Datei.
type fileState struct {
	lang       model.Language
	toks       []token
	docs       []docBlock
	docByEnd   map[int]int // Endzeile -> Index in docs
	res        *parser.Result
	pos        int
	path       string // Pfad der Datei, für Top-Level-Anweisungen
	topLevel   model.SymbolID
	strs       []cstr // Stringliterale mit Position, siehe literalAt
	lineStarts []int
	seenSym    map[model.SymbolID]bool
	seenKey    map[string]bool
	seenLoc    map[locKey]bool
	seenRef    map[refKey]bool
	seenRel    map[relKey]bool
}

// owner beschreibt den umgebenden Namensraum oder Typ einer Deklaration.
type owner struct {
	id     model.SymbolID
	fqn    string
	name   string // einfacher Name ohne Arity-Suffix
	isRoot bool
	route  string // Route-Präfix des Controllers, für HTTP-Routen
}

type attrRef struct {
	name string
	tok  token
	arg  string // erstes Stringargument, z. B. der Routenpfad in [HttpGet("{id}")]
}

type baseRef struct {
	name   string
	tok    token
	endTok token
	arity  int // Anzahl generischer Argumente
}

func newFileState(toks []token, docs []docBlock) *fileState {
	f := &fileState{
		lang:     model.LangCSharp,
		toks:     toks,
		docs:     docs,
		docByEnd: map[int]int{},
		res:      &parser.Result{},
		seenSym:  map[model.SymbolID]bool{},
		seenKey:  map[string]bool{},
		seenLoc:  map[locKey]bool{},
		seenRef:  map[refKey]bool{},
		seenRel:  map[relKey]bool{},
	}
	for idx, d := range docs {
		f.docByEnd[d.endLine] = idx
	}
	return f
}

// at liefert das Token an Index i oder ein leeres Sentinel-Token.
func (f *fileState) at(i int) token {
	if i >= 0 && i < len(f.toks) {
		return f.toks[i]
	}
	return token{}
}

func tokSpan(t token) model.Span {
	return model.Span{StartLine: t.line, StartCol: t.col, EndLine: t.endLine, EndCol: t.endCol}
}

func rangeSpan(a, b token) model.Span {
	return model.Span{StartLine: a.line, StartCol: a.col, EndLine: b.endLine, EndCol: b.endCol}
}

func roleOf(mods []string) model.LocationRole {
	for _, m := range mods {
		if m == "partial" {
			return model.RolePartial
		}
	}
	return model.RoleDecl
}

func (f *fileState) diag(t token, msg string) {
	f.res.Diagnostics = append(f.res.Diagnostics, parser.Diagnostic{Span: tokSpan(t), Message: msg})
}

// --- Emission mit Duplikatschutz (wichtig bei #if-Zweigen mit gleichen Deklarationen) ---

func (f *fileState) emitSymbol(sym model.Symbol) {
	if f.seenSym[sym.ID] {
		return
	}
	f.seenSym[sym.ID] = true
	f.res.Symbols = append(f.res.Symbols, sym)
}

func (f *fileState) emitLoc(id model.SymbolID, span model.Span, role model.LocationRole) {
	key := locKey{id, span.StartLine, span.StartCol, span.EndLine, span.EndCol}
	if f.seenLoc[key] {
		return
	}
	f.seenLoc[key] = true
	f.res.Locations = append(f.res.Locations, model.Location{SymbolID: id, Span: span, Role: role})
}

func (f *fileState) emitRef(r model.Reference) {
	key := refKey{r.Kind, r.TargetName, r.Span, r.Enclosing}
	if f.seenRef[key] {
		return
	}
	f.seenRef[key] = true
	f.res.References = append(f.res.References, r)
}

func (f *fileState) emitRel(from, to model.SymbolID, kind model.RelationKind) {
	if from == "" || to == "" {
		return
	}
	key := relKey{from, to, kind}
	if f.seenRel[key] {
		return
	}
	f.seenRel[key] = true
	f.res.Relations = append(f.res.Relations, model.Relation{From: from, To: to, Kind: kind})
}

// --- Gliederung: Scope, using, namespace ---

// parseScope verarbeitet Deklarationen bis zum Index end (exklusiv).
// Garantiert Fortschritt, damit auch kaputter Code nicht endlos läuft.
func (f *fileState) parseScope(own owner, end int) {
	for f.pos < end {
		before := f.pos
		t := f.at(f.pos)
		switch {
		case isP(t, ";"):
			f.pos++
		case t.kind == tokIdent && t.text == "global" && isIdentAt(f, f.pos+1, "using"):
			f.pos++
		case t.kind == tokIdent && t.text == "using":
			f.parseUsing(end)
		case t.kind == tokIdent && t.text == "namespace":
			f.parseNamespace(own, end)
		case own.isRoot && t.kind == tokIdent && t.text == "var" && f.at(f.pos+1).kind == tokIdent:
			// Top-Level-Anweisung `var app = ...;`: eine lokale Variable, kein Feld. Verwendungen zählen
			// zum synthetischen Top-Level-Symbol.
			semi := f.statementEnd(end)
			f.scanRefs(f.pos, semi, f.enclosingOrTop(""))
			f.pos = semi + 1
		default:
			f.parseDeclaration(own, end)
		}
		if f.pos == before {
			f.pos++
		}
	}
}

func isIdentAt(f *fileState, i int, text string) bool {
	t := f.at(i)
	return t.kind == tokIdent && t.text == text
}

func (f *fileState) parseUsing(end int) {
	f.pos++ // using
	j := findTopLevel(f.toks, f.pos, end, ";")
	raw := joinTokens(f.toks[f.pos:j])
	if raw != "" {
		f.res.Imports = append(f.res.Imports, model.Import{Raw: raw, Kind: model.ImportUsing})
	}
	f.pos = j + 1
}

func (f *fileState) parseNamespace(parent owner, end int) {
	nsTok := f.at(f.pos)
	f.pos++
	if f.pos >= end || f.at(f.pos).kind != tokIdent {
		f.diag(nsTok, "Namespace ohne Name")
		return
	}
	nameStart := f.pos
	var parts []string
	for f.pos < end && f.at(f.pos).kind == tokIdent {
		parts = append(parts, unescape(f.at(f.pos).text))
		f.pos++
		if f.pos < end && isP(f.at(f.pos), ".") {
			f.pos++
			continue
		}
		break
	}
	name := strings.Join(parts, ".")
	fqn := name
	if !parent.isRoot {
		fqn = parent.fqn + "." + name
	}
	nsID := model.NewSymbolID(f.lang, fqn, nil)
	nameTok := f.at(nameStart)
	simple := parts[len(parts)-1]

	if f.pos < end && isP(f.at(f.pos), "{") {
		close := matchClose(f.toks, f.pos, end, "{", "}")
		if close < 0 {
			f.diag(nameTok, "Namespace-Block nicht geschlossen")
			close = end - 1
		}
		f.emitNamespace(nsID, simple, fqn, parent, rangeSpan(nameTok, f.at(close)))
		open := f.pos
		f.pos = open + 1
		f.parseScope(owner{id: nsID, fqn: fqn, name: simple}, close)
		f.pos = close + 1
		return
	}
	if f.pos < end && isP(f.at(f.pos), ";") {
		semi := f.pos
		f.pos++
		f.emitNamespace(nsID, simple, fqn, parent, rangeSpan(nameTok, f.at(semi)))
		// File-scoped Namespace: gilt bis zum Ende der Datei.
		f.parseScope(owner{id: nsID, fqn: fqn, name: simple}, end)
		return
	}
	f.diag(nameTok, "Namespace ohne Körper")
}

func (f *fileState) emitNamespace(id model.SymbolID, simple, fqn string, parent owner, span model.Span) {
	f.emitSymbol(model.Symbol{ID: id, Kind: model.KindNamespace, Name: simple, FQN: fqn, Container: parent.id})
	f.emitLoc(id, span, model.RoleDecl)
	f.emitRel(parent.id, id, model.RelContains)
}

// --- Deklarationen ---

func (f *fileState) parseDeclaration(own owner, end int) {
	start := f.pos
	attrs := f.parseAttributes(end)
	// Nach Attributen (etwa [assembly: ...]) kann ein Namensraum folgen.
	if t := f.at(f.pos); t.kind == tokIdent && t.text == "namespace" {
		f.parseNamespace(own, end)
		return
	}
	mods := f.parseModifiers(end)
	if f.pos >= end {
		return
	}
	t := f.at(f.pos)
	if t.kind == tokIdent {
		switch t.text {
		case "class", "struct", "interface", "enum":
			f.parseTypeDecl(own, mods, attrs, start, end)
			return
		case "record":
			if f.pos+1 < end && f.at(f.pos+1).kind == tokIdent {
				f.parseTypeDecl(own, mods, attrs, start, end)
				return
			}
		case "delegate":
			f.skipStatement(end)
			return
		case "event":
			f.parseEvent(own, mods, attrs, start, end)
			return
		}
	}
	if isP(t, "}") {
		f.pos++
		return
	}
	f.parseMember(own, mods, attrs, start, end)
}

func (f *fileState) parseTypeDecl(own owner, mods []string, attrs []attrRef, start, end int) {
	kwTok := f.at(f.pos)
	f.pos++
	kw := kwTok.text
	if kw == "record" && f.pos < end && (isIdentAt(f, f.pos, "class") || isIdentAt(f, f.pos, "struct")) {
		f.pos++
	}
	if f.pos >= end || f.at(f.pos).kind != tokIdent {
		f.diag(kwTok, "Typ ohne Name")
		f.skipStatement(end)
		return
	}
	nameIdx := f.pos
	nameTok := f.at(nameIdx)
	name := unescape(nameTok.text)
	f.pos++

	arity := 0
	if f.pos < end && isP(f.at(f.pos), "<") {
		if j := matchAngle(f.toks, f.pos, end); j > 0 {
			arity = countTopLevelParams(f.toks[f.pos+1 : j])
			f.pos = j + 1
		}
	}
	display := name
	if arity > 0 {
		display = name + "`" + strconv.Itoa(arity)
	}
	fqn := display
	if !own.isRoot {
		fqn = own.fqn + "." + display
	}
	id := model.NewSymbolID(f.lang, fqn, nil)
	kind := kindForKeyword(kw)

	// Primärkonstruktor bei record/class
	if f.pos < end && isP(f.at(f.pos), "(") {
		if j := matchClose(f.toks, f.pos, end, "(", ")"); j > 0 {
			f.pos = j + 1
		}
	}

	var bases []baseRef
	if f.pos < end && isP(f.at(f.pos), ":") {
		f.pos++
		for f.pos < end {
			ty := readTypeAt(f.toks, f.pos, end)
			if ty.text == "" {
				break
			}
			if model.IsCSharpKeywordType(baseName(ty.text)) || baseName(ty.text) == "object" {
				// Schlüsselwort-Typen wie object sind keine Projektsymbole
				f.pos = ty.endIdx
				if f.pos < end && isP(f.at(f.pos), ",") {
					f.pos++
					continue
				}
				break
			}
			bases = append(bases, baseRef{
				name:   baseName(ty.text),
				tok:    f.at(ty.startIdx),
				endTok: f.at(ty.endIdx - 1),
				arity:  genericArityIn(f.toks, ty.startIdx, ty.endIdx),
			})
			f.pos = ty.endIdx
			// Basiskonstruktor-Argumente: `: Base(a, b)` (z. B. bei Primärkonstruktoren)
			if f.pos < end && isP(f.at(f.pos), "(") {
				if j := matchClose(f.toks, f.pos, end, "(", ")"); j > 0 {
					f.scanRefs(f.pos+1, j, model.SymbolID(""))
					f.pos = j + 1
				}
			}
			if f.pos < end && isP(f.at(f.pos), ",") {
				f.pos++
				continue
			}
			break
		}
	}
	for f.pos < end && isIdentAt(f, f.pos, "where") {
		f.pos = findTopLevel(f.toks, f.pos, end, "{", ";")
	}

	openIdx, closeIdx := -1, nameIdx
	switch {
	case f.pos < end && isP(f.at(f.pos), "{"):
		openIdx = f.pos
		closeIdx = matchClose(f.toks, f.pos, end, "{", "}")
		if closeIdx < 0 {
			f.diag(nameTok, "Typ-Körper nicht geschlossen")
			closeIdx = end - 1
		}
		f.pos = closeIdx + 1
	case f.pos < end && isP(f.at(f.pos), ";"):
		closeIdx = f.pos
		f.pos++
	default:
		f.diag(nameTok, "Typ ohne Körper")
	}

	sig := strings.Join(append(append([]string{}, mods...), kwTok.text, display), " ")
	sym := model.Symbol{
		ID: id, Kind: kind, Name: name, FQN: fqn, Container: own.id,
		Modifiers: mods, Signature: sig,
	}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(closeIdx)), roleOf(mods))
	f.emitRel(own.id, id, model.RelContains)
	f.flushAttrs(attrs, id)

	if kw != "enum" {
		for idx, b := range bases {
			rk, conf := baseRefKind(kw, idx, b.name)
			f.emitRef(model.Reference{
				Kind: rk, TargetName: b.name, Span: rangeSpan(b.tok, b.endTok),
				Enclosing: id, Confidence: conf, Arity: b.arity,
			})
		}
	}

	if openIdx >= 0 {
		inner := owner{id: id, fqn: fqn, name: name, route: classRoute(attrs, name)}
		saved := f.pos
		f.pos = openIdx + 1
		if kw == "enum" {
			f.parseEnumBody(inner, closeIdx)
		} else {
			f.parseScope(inner, closeIdx)
		}
		f.pos = saved
	}
}

func kindForKeyword(kw string) model.SymbolKind {
	switch kw {
	case "struct":
		return model.KindStruct
	case "interface":
		return model.KindInterface
	case "enum":
		return model.KindEnum
	case "record":
		return model.KindRecord
	default:
		return model.KindClass
	}
}

// baseRefKind entscheidet, ob eine Basisangabe Vererbung oder Interface-Implementierung ist.
// Ohne Typauflösung gilt: Interfaces beginnen mit I plus Großbuchstabe. Die erste Basis einer
// Klasse ist sonst meist die Basisklasse. Das bleibt eine Vermutung (HEURISTIC).
func baseRefKind(kw string, idx int, name string) (model.RefKind, model.Confidence) {
	isIface := len(name) >= 2 && name[0] == 'I' && name[1] >= 'A' && name[1] <= 'Z'
	switch {
	case kw == "interface":
		return model.RefInherits, model.ConfHeuristic
	case isIface:
		return model.RefImplements, model.ConfResolved
	case idx == 0:
		return model.RefInherits, model.ConfHeuristic
	default:
		return model.RefImplements, model.ConfHeuristic
	}
}

func (f *fileState) parseEnumBody(own owner, close int) {
	for f.pos < close {
		before := f.pos
		attrs := f.parseAttributes(close)
		if f.pos >= close {
			break
		}
		t := f.at(f.pos)
		if t.kind != tokIdent {
			f.pos++
			continue
		}
		stop := findTopLevel(f.toks, f.pos, close, ",")
		name := unescape(t.text)
		id := model.NewSymbolID(f.lang, own.fqn+"."+name, nil)
		f.emitSymbol(model.Symbol{ID: id, Kind: model.KindField, Name: name, FQN: own.fqn + "." + name, Container: own.id, DocSource: "inline"})
		f.emitLoc(id, rangeSpan(t, f.at(stop-1)), model.RoleDecl)
		f.emitRel(own.id, id, model.RelContains)
		f.flushAttrs(attrs, id)
		f.pos = stop
		if f.pos < close && isP(f.at(f.pos), ",") {
			f.pos++
		}
		if f.pos == before {
			f.pos++
		}
	}
}

// --- Attribute und Modifikatoren ---

// parseAttributes liest alle Attributlisten `[...]` ab der aktuellen Position.
func (f *fileState) parseAttributes(end int) []attrRef {
	var out []attrRef
	for f.pos < end && isP(f.at(f.pos), "[") {
		close := matchClose(f.toks, f.pos, end, "[", "]")
		if close < 0 {
			f.diag(f.at(f.pos), "Attributliste nicht geschlossen")
			f.pos = end
			return out
		}
		i := f.pos + 1
		for i < close {
			// Ziel-Präfix wie `assembly:` oder `return:` überspringen
			if i+1 < close && isP(f.at(i+1), ":") && f.at(i).kind == tokIdent {
				i += 2
			}
			var last token
			have := false
			for i < close && f.at(i).kind == tokIdent {
				last = f.at(i)
				have = true
				i++
				if i+1 < close && isP(f.at(i), ".") {
					i++
					continue
				}
				break
			}
			arg := ""
			if have && i < close && isP(f.at(i), "(") {
				arg = f.literalAt(i)
			}
			if have {
				out = append(out, attrRef{name: unescape(last.text), tok: last, arg: arg})
			}
			depth := 0
			for i < close {
				t := f.at(i)
				if isP(t, "(") {
					depth++
				} else if isP(t, ")") {
					depth--
				} else if depth == 0 && isP(t, ",") {
					break
				}
				i++
			}
			if i < close && isP(f.at(i), ",") {
				i++
			}
		}
		f.pos = close + 1
	}
	return out
}

func (f *fileState) parseModifiers(end int) []string {
	var mods []string
	for f.pos < end {
		t := f.at(f.pos)
		if t.kind != tokIdent || !modifierSet[t.text] {
			break
		}
		mods = append(mods, t.text)
		f.pos++
	}
	return mods
}

// setLiterals übernimmt die Stringliterale und die Zeilenstarts, damit Positionen auf Stringwerte abgebildet werden.
func (f *fileState) setLiterals(strs []cstr, content []byte) {
	f.strs = strs
	f.lineStarts = []int{0}
	for i, c := range content {
		if c == '\n' {
			f.lineStarts = append(f.lineStarts, i+1)
		}
	}
}

// offset liefert die Byteposition eines Tokens im Quelltext.
func (f *fileState) offset(t token) int {
	if t.line < 1 || t.line > len(f.lineStarts) {
		return 0
	}
	return f.lineStarts[t.line-1] + t.col - 1
}

// literalAt liefert den Stringwert, der zwischen dem Token bei i und dem nächsten Token steht (z. B. das
// Argument nach einer öffnenden Klammer). Leer, wenn dort kein Stringliteral steht.
func (f *fileState) literalAt(i int) string {
	if i < 0 || i >= len(f.toks) || len(f.lineStarts) == 0 {
		return ""
	}
	from := f.offset(f.toks[i])
	to := int(^uint(0) >> 1)
	if i+1 < len(f.toks) {
		to = f.offset(f.toks[i+1])
	}
	k := sort.Search(len(f.strs), func(k int) bool { return f.strs[k].off > from })
	if k < len(f.strs) && f.strs[k].off < to {
		return f.strs[k].value
	}
	return ""
}

// httpVerbs ordnet Attribute der Route dem HTTP-Verb zu.
var httpVerbs = map[string]string{"HttpGet": "GET", "HttpPost": "POST", "HttpPut": "PUT",
	"HttpDelete": "DELETE", "HttpPatch": "PATCH"}

// classRoute liest den Route-Präfix einer Controller-Klasse. [controller] wird durch den Namen ohne
// Suffix "Controller" in Kleinbuchstaben ersetzt, wie ASP.NET es tut.
func classRoute(attrs []attrRef, name string) string {
	for _, a := range attrs {
		if a.name == "Route" || a.name == "RoutePrefix" {
			short := strings.ToLower(strings.TrimSuffix(name, "Controller"))
			return strings.ReplaceAll(a.arg, "[controller]", short)
		}
	}
	return ""
}

// combineRoute setzt Präfix des Controllers und Pfad der Aktion zusammen. Ein Pfad mit führendem "/" ist absolut.
func combineRoute(prefix, template string) string {
	switch {
	case strings.HasPrefix(template, "/"):
		return template
	case prefix == "":
		return template
	case template == "":
		return prefix
	}
	return prefix + "/" + template
}

// emitRoutes legt für eine Aktion mit HTTP-Attribut die Route an. Sie hängt an der Aktion, damit der
// Resolver sie mit Aufrufen im Frontend verbinden kann.
func (f *fileState) emitRoutes(own owner, attrs []attrRef, id model.SymbolID) {
	for _, a := range attrs {
		verb, ok := httpVerbs[a.name]
		if !ok {
			continue
		}
		f.emitRef(model.Reference{
			Kind: model.RefHTTPRoute, TargetName: verb + " " + model.NormalizeRoute(combineRoute(own.route, a.arg)),
			Span: tokSpan(a.tok), Enclosing: id, Confidence: model.ConfExact,
		})
	}
}

// mapVerbs sind Minimal-API-Aufrufe wie app.MapGet("/api/x", ...).
var mapVerbs = map[string]string{"MapGet": "GET", "MapPost": "POST", "MapPut": "PUT", "MapDelete": "DELETE", "MapPatch": "PATCH"}

func (f *fileState) flushAttrs(attrs []attrRef, id model.SymbolID) {
	for _, a := range attrs {
		f.emitRef(model.Reference{
			Kind: model.RefAttribute, TargetName: a.name, Span: tokSpan(a.tok),
			Enclosing: id, Confidence: model.ConfHeuristic,
		})
	}
}

// skipStatement überspringt eine Anweisung bis zum Semikolon oder bis zum Ende eines Blocks.
func (f *fileState) skipStatement(end int) {
	depth := 0
	for f.pos < end {
		t := f.at(f.pos)
		switch {
		case isP(t, "(") || isP(t, "["):
			depth++
		case isP(t, ")") || isP(t, "]"):
			depth--
		case isP(t, "{"):
			close := matchClose(f.toks, f.pos, end, "{", "}")
			if close < 0 {
				f.pos = end
				return
			}
			f.pos = close + 1
			if depth == 0 && !(f.pos < end && isP(f.at(f.pos), ";")) {
				return
			}
			continue
		case isP(t, ";") && depth <= 0:
			f.pos++
			return
		}
		f.pos++
	}
}

// applyDoc übernimmt die XML-Dokumentation direkt über der Deklaration und
// legt `<see cref>`-Verwendungen als Referenzen an.
func (f *fileState) applyDoc(sym *model.Symbol, start int) {
	line := f.at(start).line
	idx, ok := f.docByEnd[line-1]
	if !ok {
		return
	}
	b := f.docs[idx]
	info := parseDocBlock(b)
	sym.Summary = info.summary
	sym.Remarks = info.remarks
	sym.Returns = info.returns
	sym.ParamDocs = info.params
	sym.DocSource = "inline"
	for _, c := range info.crefs {
		f.emitRef(model.Reference{
			Kind: model.RefCref, TargetName: c,
			Span:       model.Span{StartLine: b.startLine, StartCol: 1, EndLine: b.endLine, EndCol: 1},
			Enclosing:  sym.ID,
			Confidence: model.ConfExact,
		})
	}
}

// Schlüssel für die Duplikaterkennung. Typisierte Schlüssel vermeiden Strings pro Datensatz.
type locKey struct {
	id                                   model.SymbolID
	startLine, startCol, endLine, endCol int
}

type refKey struct {
	kind model.RefKind
	name string
	span model.Span
	enc  model.SymbolID
}

type relKey struct {
	from, to model.SymbolID
	kind     model.RelationKind
}

// enclosingOrTop liefert den umgebenden Member. Top-Level-Anweisungen (etwa app.MapGet in Program.cs) gehören
// zu einem synthetischen Symbol der Datei, damit ihre Routen verbunden werden können.
func (f *fileState) enclosingOrTop(enc model.SymbolID) model.SymbolID {
	if enc != "" {
		return enc
	}
	if f.topLevel == "" {
		f.topLevel = model.NewSymbolID(model.LangCSharp, "<top-level>@"+f.path, nil)
		f.emitSymbol(model.Symbol{ID: f.topLevel, Kind: model.KindMethod, Name: "<top-level>", FQN: "<top-level>@" + f.path,
			Modifiers: []string{"private"}, Signature: "<top-level statements>"})
		f.emitLoc(f.topLevel, model.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}, model.RoleDecl)
	}
	return f.topLevel
}

// scanTopLevelRoutes sucht Minimal-API-Aufrufe außerhalb aller Blöcke (Top-Level-Anweisungen). Innerhalb von
// Methoden und Typen erfasst scanRefs sie bereits.
func (f *fileState) scanTopLevelRoutes(toks []token) {
	depth := 0
	for i := 0; i+1 < len(toks); i++ {
		switch {
		case isP(toks[i], "{"):
			depth++
		case isP(toks[i], "}"):
			depth--
		case depth == 0 && toks[i].kind == tokIdent && isP(toks[i+1], "("):
			if toks[i].text == "MapControllerRoute" {
				f.emitConvention(toks[i].text, toks[i], i+1, matchClose(toks, i+1, len(toks), "(", ")"), "")
				continue
			}
			verb, ok := mapVerbs[toks[i].text]
			if !ok {
				continue
			}
			if lit := f.literalAt(i + 1); lit != "" {
				f.emitRef(model.Reference{Kind: model.RefHTTPRoute, TargetName: verb + " " + model.NormalizeRoute(lit),
					Span: tokSpan(toks[i]), Enclosing: f.enclosingOrTop(""), Confidence: model.ConfExact})
			}
		}
	}
}

// statementEnd liefert die Position des Semikolons, das die Anweisung ab f.pos beendet (auf Klammertiefe 0).
func (f *fileState) statementEnd(end int) int {
	depth := 0
	for i := f.pos; i < end; i++ {
		switch {
		case isP(f.at(i), "(") || isP(f.at(i), "{") || isP(f.at(i), "["):
			depth++
		case isP(f.at(i), ")") || isP(f.at(i), "}") || isP(f.at(i), "]"):
			depth--
		case isP(f.at(i), ";") && depth == 0:
			return i
		}
	}
	return end
}

// conventionPattern liest das Muster aus MapControllerRoute(...). Das Muster ist entweder als Argument
// `pattern:` benannt oder das zweite Stringargument. Die Argumentliste reicht von open bis close.
func (f *fileState) conventionPattern(open, close int) string {
	depth := 0
	comma := 0
	for k := open + 1; k < close; k++ {
		switch {
		case isP(f.at(k), "(") || isP(f.at(k), "[") || isP(f.at(k), "{"):
			depth++
		case isP(f.at(k), ")") || isP(f.at(k), "]") || isP(f.at(k), "}"):
			depth--
		case depth == 0 && isP(f.at(k), ","):
			comma++
		case depth == 0 && isIdentAt(f, k, "pattern") && isP(f.at(k+1), ":"):
			return f.literalAt(k + 1)
		}
	}
	// Positional: MapControllerRoute(name, pattern, ...) -> das Literal nach dem ersten Komma auf Tiefe 0
	depth = 0
	seen := 0
	for k := open + 1; k < close; k++ {
		switch {
		case isP(f.at(k), "(") || isP(f.at(k), "[") || isP(f.at(k), "{"):
			depth++
		case isP(f.at(k), ")") || isP(f.at(k), "]") || isP(f.at(k), "}"):
			depth--
		case depth == 0 && isP(f.at(k), ","):
			seen++
			if seen == 1 {
				if lit := f.literalAt(k); lit != "" {
					return lit
				}
			}
		}
	}
	return ""
}

// emitConvention legt für einen Aufruf von MapControllerRoute den Muster-Verweis an.
func (f *fileState) emitConvention(name string, t token, open, close int, enc model.SymbolID) {
	if name != "MapControllerRoute" {
		return
	}
	if pat := f.conventionPattern(open, close); pat != "" {
		f.emitRef(model.Reference{Kind: model.RefHTTPConvention, TargetName: model.ConventionPattern + pat,
			Span: tokSpan(t), Enclosing: f.enclosingOrTop(enc), Confidence: model.ConfExact})
	}
}

// emitConventionAction legt für eine öffentliche Aktion eines Controllers ohne Attributroute den Aktionsverweis an.
// Der Resolver setzt daraus die Routen aus den Mustern von MapControllerRoute zusammen.
func (f *fileState) emitConventionAction(own owner, attrs []attrRef, mods []string, id model.SymbolID, name string) {
	if !strings.HasSuffix(own.name, "Controller") || !hasMod(mods, "public") {
		return
	}
	for _, a := range attrs {
		if _, isVerb := httpVerbs[a.name]; isVerb || a.name == "Route" {
			return
		}
	}
	if own.route != "" {
		// Controller mit Klassenroute, Aktion ohne Verb: die Aktion ist unter dem Klassenpfad erreichbar (jedes Verb)
		f.emitRef(model.Reference{Kind: model.RefHTTPRoute, TargetName: "ANY " + model.NormalizeRoute(own.route),
			Span: tokSpan(f.toks[0]), Enclosing: id, Confidence: model.ConfExact})
		return
	}
	ctrl := strings.ToLower(strings.TrimSuffix(own.name, "Controller"))
	f.emitRef(model.Reference{Kind: model.RefHTTPConvention,
		TargetName: model.ConventionAction + ctrl + "/" + strings.ToLower(name), Enclosing: id, Confidence: model.ConfExact})
}

func hasMod(mods []string, m string) bool {
	for _, x := range mods {
		if x == m {
			return true
		}
	}
	return false
}
