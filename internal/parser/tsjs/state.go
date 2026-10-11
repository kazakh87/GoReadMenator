package tsjs

import (
	"fmt"
	"strings"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// fileState hält den Zustand während des Parsens einer Datei.
type fileState struct {
	templates  map[int]tmplSpan // Rohtext interpolierter Templates nach Backtick-Position
	lineStarts []int
	path       string
	lang       model.Language
	toks       []token
	docs       []docBlock
	docByEnd   map[int]int
	res        *parser.Result
	pos        int
	seenSym    map[model.SymbolID]bool
	seenKey    map[string]bool
	seenLoc    map[locKey]bool
	seenRef    map[refKey]bool
	seenRel    map[relKey]bool
	tparams    map[string]bool // deklarierte Typparameter (werden nicht als Typverweise gezählt)
	jsx        bool
}

// owner ist der umgebende Namensraum, Typ oder das Modul einer Deklaration.
// prefix ist der vollqualifizierte Präfix für Kinder, etwa "src/a.ts#Klasse.".
type owner struct {
	id     model.SymbolID
	prefix string
	name   string
	isRoot bool
}

type attrRef struct {
	name string
	tok  token
}

type baseRef struct {
	name   string
	tok    token
	endTok token
}

func newFileState(path string, toks []token, docs []docBlock) *fileState {
	f := &fileState{
		path:     path,
		lang:     model.LangTSJS,
		toks:     toks,
		docs:     docs,
		docByEnd: map[int]int{},
		res:      &parser.Result{},
		seenSym:  map[model.SymbolID]bool{},
		seenKey:  map[string]bool{},
		seenLoc:  map[locKey]bool{},
		seenRef:  map[refKey]bool{},
		seenRel:  map[relKey]bool{},
		tparams:  map[string]bool{},
		jsx:      strings.HasSuffix(path, ".tsx") || strings.HasSuffix(path, ".jsx"),
	}
	for idx, d := range docs {
		f.docByEnd[d.endLine] = idx
	}
	return f
}

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

func roleFor(mods []string) model.LocationRole {
	return model.RoleDecl
}

// buildSignature baut die Signatur aus Modifikatoren, Name, Parametern und Rückgabetyp.
func buildSignature(mods []string, name string, params []tsParam, ret typeInfo) string {
	var parts []string
	parts = append(parts, mods...)
	parts = append(parts, name)
	var ps []string
	for _, p := range params {
		t := p.ty.text
		if t == "" {
			t = "any"
		}
		if p.name != "" {
			ps = append(ps, p.name+": "+t)
		} else {
			ps = append(ps, t)
		}
	}
	sig := strings.Join(parts, " ") + "(" + strings.Join(ps, ", ") + ")"
	if ret.text != "" {
		sig += ": " + ret.text
	}
	return sig
}

func hasMod(mods []string, m string) bool {
	for _, x := range mods {
		if x == m {
			return true
		}
	}
	return false
}

func (f *fileState) diag(t token, msg string) {
	f.res.Diagnostics = append(f.res.Diagnostics, parser.Diagnostic{Span: tokSpan(t), Message: msg})
}

// --- Emission mit Duplikatschutz ---

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

func (f *fileState) emitImport(raw string, kind model.ImportKind) {
	f.emitImportB(raw, kind, nil)
}

// emitImportB legt einen Import mit seinen lokalen Bindungen an.
func (f *fileState) emitImportB(raw string, kind model.ImportKind, bindings []model.ImportBinding) {
	// `export * from "x"` und `export { a } from "x"` sind verschiedene Re-Exporte, auch mit gleichem Pfad.
	// Ein Stern-Export darf deshalb nicht mit benannten Bindungen zusammenfallen.
	star := ""
	if kind == model.ImportReexport && len(bindings) == 0 {
		star = "*"
	}
	key := fmt.Sprintf("i|%s|%s|%s", raw, kind, star)
	if f.seenKey[key] {
		// Bindungen zum bereits vorhandenen Import ergänzen
		for k := range f.res.Imports {
			if f.res.Imports[k].Raw == raw && f.res.Imports[k].Kind == kind && (star == "") == (len(f.res.Imports[k].Bindings) > 0 || kind != model.ImportReexport) {
				f.res.Imports[k].Bindings = append(f.res.Imports[k].Bindings, bindings...)
			}
		}
		return
	}
	f.seenKey[key] = true
	f.res.Imports = append(f.res.Imports, model.Import{Raw: raw, Kind: kind, Bindings: bindings})
}

// --- Gliederung ---

// parseScope verarbeitet Anweisungen und Deklarationen bis end (exklusiv).
func (f *fileState) parseScope(own owner, end int) {
	for f.pos < end {
		before := f.pos
		t := f.at(f.pos)
		switch {
		case isP(t, ";"):
			f.pos++
		case isWord(t, "import") && !isP(f.at(f.pos+1), "(") && !isP(f.at(f.pos+1), "."):
			f.parseImport(own, end)
		case isWord(t, "export"):
			f.parseExport(own, end)
		case isP(t, "@"):
			attrs := f.parseDecorators(own, end)
			f.parseDeclaration(own, end, attrs, nil)
		default:
			f.parseDeclaration(own, end, nil, nil)
		}
		if f.pos == before {
			f.pos++
		}
	}
}

// parseDeclaration erkennt Deklarationen. Passt nichts, wird die Anweisung nur nach Verweisen durchsucht.
func (f *fileState) parseDeclaration(own owner, end int, attrs []attrRef, extraMods []string) {
	start := f.pos
	mods := append([]string{}, extraMods...)
	for f.pos < end {
		t := f.at(f.pos)
		if t.kind == tokIdent && (t.text == "export" || t.text == "default" || t.text == "declare" ||
			t.text == "abstract" || (t.text == "async" && (isWord(f.at(f.pos+1), "function") || isP(f.at(f.pos+1), "(")))) {
			if t.text == "export" && len(mods) > 0 {
				break
			}
			if !hasMod(mods, t.text) { // "export default" kommt von parseExport bereits mit
				mods = append(mods, t.text)
			}
			f.pos++
			continue
		}
		break
	}
	if f.pos >= end {
		return
	}
	t := f.at(f.pos)
	if t.kind == tokIdent {
		switch t.text {
		case "function":
			f.parseFunction(own, mods, attrs, start, end)
			return
		case "class":
			f.parseClass(own, mods, attrs, start, end)
			return
		case "interface":
			f.parseInterface(own, mods, attrs, start, end)
			return
		case "enum":
			f.parseEnum(own, mods, attrs, start, end)
			return
		case "const":
			if isWord(f.at(f.pos+1), "enum") {
				f.pos++
				f.parseEnum(own, mods, attrs, start, end)
				return
			}
			f.parseVariables(own, mods, attrs, start, end)
			return
		case "let", "var":
			f.parseVariables(own, mods, attrs, start, end)
			return
		case "type":
			if f.at(f.pos+1).kind == tokIdent {
				f.parseTypeAlias(own, mods, attrs, start, end)
				return
			}
		case "namespace", "module":
			n := f.at(f.pos + 1)
			if n.kind == tokIdent || n.kind == tokString {
				f.parseNamespace(own, mods, start, end)
				return
			}
		case "global":
			if isP(f.at(f.pos+1), "{") {
				f.parseNamespace(own, mods, start, end)
				return
			}
		}
	}
	// Keine Deklaration: Anweisung nach Verweisen durchsuchen.
	e := findStmtEnd(f.toks, f.pos, end)
	f.scanRefs(f.pos, e, own.id)
	f.pos = e
	if f.pos < end && isP(f.at(f.pos), ";") {
		f.pos++
	}
}

// parseImport verarbeitet alle Formen von `import ...`.
func (f *fileState) parseImport(own owner, end int) {
	kwTok := f.at(f.pos)
	f.pos++
	typeOnly := false
	if isWord(f.at(f.pos), "type") && (f.at(f.pos+1).kind == tokIdent || isP(f.at(f.pos+1), "{") || isP(f.at(f.pos+1), "*")) {
		typeOnly = true
		f.pos++
	}
	// Seiteneffekt-Import: import "x";
	if f.at(f.pos).kind == tokString {
		f.emitImport(f.at(f.pos).text, kindOr(typeOnly, model.ImportTypeOnly, model.ImportESM))
		f.pos++
		f.finishStmt(end)
		return
	}
	// import x = require("y");  (TypeScript-Form)
	if f.at(f.pos).kind == tokIdent && isP(f.at(f.pos+1), "=") && isWord(f.at(f.pos+2), "require") {
		name := f.at(f.pos).text
		j := f.pos + 3
		if isP(f.at(j), "(") && f.at(j+1).kind == tokString {
			f.emitImport(f.at(j+1).text, model.ImportRequire)
			f.emitRef(model.Reference{Kind: model.RefImport, TargetName: name, Span: tokSpan(kwTok), Confidence: model.ConfExact})
		}
		f.pos = findStmtEnd(f.toks, f.pos, end)
		f.finishStmt(end)
		return
	}

	kind := model.ImportDefault
	var bindings []model.ImportBinding
	from := findTopLevel(f.toks, f.pos, end, ";")
	// Bindungen bis zum Schlüsselwort "from" sammeln
	fromIdx := -1
	for j := f.pos; j < end && j < from; j++ {
		if isWord(f.at(j), "from") {
			fromIdx = j
			break
		}
	}
	if fromIdx < 0 {
		f.diag(kwTok, "import ohne from")
		f.pos = from
		f.finishStmt(end)
		return
	}
	for j := f.pos; j < fromIdx; j++ {
		t := f.at(j)
		switch {
		case isP(t, "*"):
			kind = model.ImportNamespace
		case isP(t, "{"):
			if kind != model.ImportNamespace {
				kind = model.ImportESM
			}
		case t.kind == tokIdent && t.text != "as" && t.text != "type":
			prev := f.at(j - 1)
			startOfBinding := j == f.pos || isP(prev, "{") || isP(prev, ",")
			afterTypeMarker := isWord(prev, "type") && (isP(f.at(j-2), "{") || isP(f.at(j-2), ","))
			if startOfBinding || afterTypeMarker {
				f.emitRef(model.Reference{Kind: model.RefImport, TargetName: t.text, Span: tokSpan(t), Confidence: model.ConfExact})
				// Ist der Name ein Alias-Ausgangspunkt (`a as b`), wird der lokale Name unten ergänzt.
				if !isWord(f.at(j+1), "as") {
					imported := t.text
					if j == f.pos {
						imported = "default" // Default-Import: `import X from ...`
					}
					bindings = append(bindings, model.ImportBinding{Local: t.text, Imported: imported})
				}
			}
		case isWord(t, "as") && f.at(j+1).kind == tokIdent:
			// lokaler Name: `* as ns` oder `a as b`
			local := f.at(j + 1)
			f.emitRef(model.Reference{Kind: model.RefImport, TargetName: local.text, Span: tokSpan(local), Confidence: model.ConfExact})
			if isWord(f.at(j-1), "*") || isP(f.at(j-1), "*") {
				bindings = append(bindings, model.ImportBinding{Local: local.text, Imported: "*"})
			} else {
				orig := f.at(j - 1)
				bindings = append(bindings, model.ImportBinding{Local: local.text, Imported: orig.text})
			}
		}
	}
	mod := f.at(fromIdx + 1)
	if mod.kind == tokString {
		k := kind
		if typeOnly {
			k = model.ImportTypeOnly
		}
		f.emitImportB(mod.text, k, bindings)
	}
	f.pos = fromIdx + 2
	f.finishStmt(end)
}

func kindOr(cond bool, a, b model.ImportKind) model.ImportKind {
	if cond {
		return a
	}
	return b
}

// finishStmt schließt eine Anweisung mit optionalem Semikolon ab.
func (f *fileState) finishStmt(end int) {
	if f.pos < end && isP(f.at(f.pos), ";") {
		f.pos++
	}
}

// parseExport behandelt die Export-Formen: Deklarationen, Listen, Re-Exporte und Default-Exporte.
func (f *fileState) parseExport(own owner, end int) {
	f.pos++ // export
	t := f.at(f.pos)
	switch {
	case isP(t, "*"):
		// export * from "x" / export * as ns from "x"
		j := findTopLevel(f.toks, f.pos, end, ";")
		for k := f.pos; k < j; k++ {
			if isWord(f.at(k), "from") && f.at(k+1).kind == tokString {
				f.emitImport(f.at(k+1).text, model.ImportReexport)
			}
		}
		f.pos = j
		f.finishStmt(end)
	case isP(t, "{"):
		close := matchClose(f.toks, f.pos, end, "{", "}")
		if close < 0 {
			f.pos = end
			return
		}
		j := close + 1
		if isWord(f.at(j), "from") && f.at(j+1).kind == tokString {
			// export { a as b } from "x": lokal b wird als a aus x weitergegeben
			var bindings []model.ImportBinding
			for k := f.pos + 1; k < close; k++ {
				if f.at(k).kind != tokIdent || f.at(k).text == "type" || f.at(k).text == "as" {
					continue
				}
				orig := f.at(k).text
				exported := orig
				if isWord(f.at(k+1), "as") && f.at(k+2).kind == tokIdent {
					exported = f.at(k + 2).text
					k += 2 // Alias-Teile überspringen, sonst würde der Alias als eigene Bindung gelten
				}
				bindings = append(bindings, model.ImportBinding{Local: exported, Imported: orig})
			}
			f.emitImportB(f.at(j+1).text, model.ImportReexport, bindings)
			j += 2
		}
		f.pos = j
		f.finishStmt(end)
	case isP(t, "="):
		f.pos = findStmtEnd(f.toks, f.pos, end)
		f.finishStmt(end)
	case isWord(t, "default"):
		n := f.at(f.pos + 1)
		if isWord(n, "function") || isWord(n, "class") || isWord(n, "async") || isWord(n, "abstract") || isWord(n, "interface") {
			f.parseDeclaration(own, end, nil, []string{"export", "default"})
			return
		}
		// export default Ausdruck;
		f.pos++
		e := findStmtEnd(f.toks, f.pos, end)
		f.scanRefs(f.pos, e, own.id)
		f.pos = e
		f.finishStmt(end)
	default:
		f.parseDeclaration(own, end, nil, []string{"export"})
	}
}

// parseNamespace verarbeitet namespace/module/global-Blöcke.
func (f *fileState) parseNamespace(own owner, mods []string, start, end int) {
	kwTok := f.at(f.pos)
	f.pos++
	var name string
	nameTok := kwTok
	if kwTok.text == "global" {
		name = "global"
	} else {
		nameTok = f.at(f.pos)
		if nameTok.kind == tokString {
			name = nameTok.text
			f.pos++
		} else {
			name, f.pos = readNameChain(f.toks, f.pos, end)
		}
	}
	if f.pos >= end || !isP(f.at(f.pos), "{") {
		f.finishStmt(end)
		return
	}
	open := f.pos
	close := matchClose(f.toks, open, end, "{", "}")
	if close < 0 {
		f.diag(nameTok, "Namespace-Block nicht geschlossen")
		f.pos = end
		return
	}
	fqn := own.prefix + name
	id := model.NewSymbolID(f.lang, fqn, nil)
	kind := model.KindNamespace
	if kwTok.text == "module" && nameTok.kind == tokString {
		kind = model.KindModule
	}
	sym := model.Symbol{ID: id, Kind: kind, Name: name, FQN: fqn, Container: own.id, Modifiers: mods, DocSource: "inline"}
	f.applyDoc(&sym, start)
	f.emitSymbol(sym)
	f.emitLoc(id, rangeSpan(nameTok, f.at(close)), roleFor(mods))
	f.emitRel(own.id, id, model.RelContains)

	saved := close + 1
	f.pos = open + 1
	f.parseScope(owner{id: id, prefix: fqn + ".", name: name}, close)
	f.pos = saved
}

// applyDoc übernimmt den JSDoc-Block oberhalb der Deklaration und legt `@see`/`{@link}` als Verwendungen an.
func (f *fileState) applyDoc(sym *model.Symbol, start int) {
	line := f.at(start).line
	idx, ok := f.docByEnd[line-1]
	if !ok {
		return
	}
	b := f.docs[idx]
	info := parseJSDoc(b.lines[0])
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

func (f *fileState) emitSymbolWithOwner(own owner, sym model.Symbol) {
	f.emitSymbol(sym)
	f.emitRel(own.id, sym.ID, model.RelContains)
}

// flushAttrs schreibt Decorator-Verweise am Symbol.
func (f *fileState) flushAttrs(attrs []attrRef, id model.SymbolID) {
	for _, a := range attrs {
		f.emitRef(model.Reference{Kind: model.RefAttribute, TargetName: a.name, Span: tokSpan(a.tok),
			Enclosing: id, Confidence: model.ConfHeuristic})
	}
}

// parseDecorators liest @Name oder @a.b(...) und liefert die Verweise.
func (f *fileState) parseDecorators(own owner, end int) []attrRef {
	var out []attrRef
	for f.pos < end && isP(f.at(f.pos), "@") {
		f.pos++
		name, next := readNameChain(f.toks, f.pos, end)
		if name == "" {
			break
		}
		last := f.at(next - 1)
		out = append(out, attrRef{name: lastSegment(name), tok: last})
		f.pos = next
		if f.pos < end && isP(f.at(f.pos), "(") {
			close := matchClose(f.toks, f.pos, end, "(", ")")
			if close > 0 {
				f.scanRefs(f.pos+1, close, own.id)
				f.pos = close + 1
			}
		}
	}
	return out
}

func lastSegment(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
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

// setTemplates übernimmt die Rohtexte der Templates und die Zeilenstarts, um Token auf Positionen abzubilden.
func (f *fileState) setTemplates(templates map[int]tmplSpan, content []byte) {
	f.templates = templates
	f.lineStarts = []int{0}
	for i, c := range content {
		if c == '\n' {
			f.lineStarts = append(f.lineStarts, i+1)
		}
	}
}

// templateAt liefert den Rohtext des Templates, in dem das Token first liegt. Das Backtick ist maskiert und
// der Text davor ist leer, deshalb wird über die Spanne jedes Templates gesucht.
func (f *fileState) templateAt(first token) (string, bool) {
	if len(f.templates) == 0 || first.line < 1 || first.line > len(f.lineStarts) {
		return "", false
	}
	off := f.lineStarts[first.line-1] + first.col - 1
	for start, sp := range f.templates {
		if start < off && off < sp.end {
			return sp.raw, true
		}
	}
	return "", false
}
