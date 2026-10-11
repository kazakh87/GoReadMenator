package vbnet

import (
	"fmt"
	"regexp"
	"strings"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// owner ist der umgebende Namensraum oder Typ. prefix ist der FQN-Präfix für Kinder (mit Punkt).
type owner struct {
	id     model.SymbolID
	prefix string
	name   string
	kind   string // "namespace", "class", "module", "structure", "interface", "enum", "root"
	isRoot bool
}

type attrRef struct {
	name string
	tok  token
}

// fileState hält den Zustand einer Datei.
type fileState struct {
	stmts    []stmt
	docs     []docBlock
	docByEnd map[int]int
	res      *parser.Result
	pos      int
	// pendingAttrs sind Attribute auf eigenen Zeilen, die zur nächsten Deklaration gehören
	pendingAttrs []attrRef
	pendingLine  int
	seenSym      map[model.SymbolID]bool
	seenLoc      map[locKey]bool
	seenRef      map[refKey]bool
	seenRel      map[relKey]bool
}

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

func newFileState(l lexed) *fileState {
	f := &fileState{
		stmts:    l.stmts,
		docs:     l.docs,
		docByEnd: map[int]int{},
		res:      &parser.Result{},
		seenSym:  map[model.SymbolID]bool{},
		seenLoc:  map[locKey]bool{},
		seenRef:  map[refKey]bool{},
		seenRel:  map[relKey]bool{},
	}
	for i, d := range l.docs {
		f.docByEnd[d.endLine] = i
	}
	return f
}

func tokSpan(t token) model.Span {
	return model.Span{StartLine: t.line, StartCol: t.col, EndLine: t.endLine, EndCol: t.endCol}
}

func rangeSpan(a, b token) model.Span {
	return model.Span{StartLine: a.line, StartCol: a.col, EndLine: b.endLine, EndCol: b.endCol}
}

func (f *fileState) diag(t token, msg string) {
	f.res.Diagnostics = append(f.res.Diagnostics, parser.Diagnostic{Span: tokSpan(t), Message: msg})
}

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

// ---------- Anweisungen klassifizieren ----------

var modifierWords = map[string]bool{
	"public": true, "private": true, "protected": true, "friend": true, "shared": true,
	"overridable": true, "overrides": true, "mustoverride": true, "notoverridable": true,
	"overloads": true, "readonly": true, "withevents": true, "partial": true, "default": true,
	"static": true, "async": true, "iterator": true, "narrowing": true, "widening": true,
}

// stripAttrs entfernt Attributlisten <...> am Anfang einer Anweisung und liefert die Attributnamen.
func stripAttrs(toks []token) ([]attrRef, []token) {
	var out []attrRef
	for len(toks) > 0 && toks[0].text == "<" {
		depth := 0
		j := 0
		for ; j < len(toks); j++ {
			if toks[j].text == "<" {
				depth++
			} else if toks[j].text == ">" {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		// Namen: Bezeichner direkt nach "<" oder nach "," auf Tiefe 1, letzter Teil eines Punktnamens
		for k := 1; k < j && k < len(toks); k++ {
			if toks[k].kind == tokIdent && (toks[k-1].text == "<" || toks[k-1].text == ",") {
				name := toks[k]
				for k+2 < j && toks[k+1].text == "." && toks[k+2].kind == tokIdent {
					k += 2
					name = toks[k]
				}
				out = append(out, attrRef{name: name.text, tok: name})
			}
		}
		if j+1 > len(toks) {
			return out, nil
		}
		toks = toks[j+1:]
	}
	return out, toks
}

// stripMods entfernt Sichtbarkeits- und Besonderheitswörter am Anfang.
func stripMods(toks []token) ([]string, []token) {
	var mods []string
	for len(toks) > 0 && toks[0].kind == tokIdent && modifierWords[toks[0].lower] {
		if toks[0].lower == "default" && len(toks) > 1 && toks[1].lower != "property" {
			break
		}
		mods = append(mods, toks[0].lower)
		toks = toks[1:]
	}
	return mods, toks
}

// isEnd meldet, ob eine Anweisung "End <kw>" ist (oder "Next"/"Loop" für kw "next"/"loop").
func isEnd(s stmt, kw string) bool {
	if len(s.toks) == 0 {
		return false
	}
	if kw == "next" || kw == "loop" {
		return s.toks[0].lower == kw
	}
	return len(s.toks) >= 2 && s.toks[0].lower == "end" && s.toks[1].lower == kw
}

// blockOpener erkennt Anweisungen, nach denen ein Block mit End/Next/Loop beginnt.
func blockOpener(s stmt) bool {
	if len(s.toks) == 0 {
		return false
	}
	w := s.toks[0].lower
	last := s.toks[len(s.toks)-1].lower
	switch w {
	case "if":
		return last == "then"
	case "for", "do", "while", "select", "with", "try", "synclock", "using":
		return true
	case "get", "set":
		// Accessoren: "Get" allein oder "Set(value As T)"
		return len(s.toks) == 1 || s.toks[1].text == "("
	}
	return false
}

var reXMLTag = regexp.MustCompile(`<[^>]+>`)
var reSpaces = regexp.MustCompile(`\s+`)

// cleanDoc entfernt XML-Tags und normalisiert Leerraum.
func cleanDoc(s string) string {
	s = reXMLTag.ReplaceAllString(s, "")
	s = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`).Replace(s)
	return strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
}

// tagText liest den Inhalt eines XML-Elements aus dem Doku-Text.
func tagText(text, tag string) string {
	re := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
	if m := re.FindStringSubmatch(text); m != nil {
		return cleanDoc(m[1])
	}
	return ""
}

func (f *fileState) docFor(line int) string {
	idx, ok := f.docByEnd[line-1]
	if !ok {
		return ""
	}
	return strings.Join(f.docs[idx].lines, "\n")
}

func (f *fileState) applyDoc(sym *model.Symbol, line int) {
	raw := f.docFor(line)
	if raw == "" {
		return
	}
	sym.DocSource = "inline"
	sym.Summary = tagText(raw, "summary")
	sym.Remarks = tagText(raw, "remarks")
	sym.Returns = tagText(raw, "returns")
	re := regexp.MustCompile(`(?s)<param name="([^"]+)">(.*?)</param>`)
	for _, m := range re.FindAllStringSubmatch(raw, -1) {
		sym.ParamDocs = append(sym.ParamDocs, model.ParamDoc{Name: m[1], Text: cleanDoc(m[2])})
	}
}

func (f *fileState) flushAttrs(attrs []attrRef, id model.SymbolID) {
	for _, a := range attrs {
		f.emitRef(model.Reference{Kind: model.RefAttribute, TargetName: a.name, Span: tokSpan(a.tok),
			Enclosing: id, Confidence: model.ConfHeuristic})
	}
}

func joinTokens(ts []token) string {
	parts := make([]string, 0, len(ts))
	for _, t := range ts {
		parts = append(parts, t.text)
	}
	return fmt.Sprint(strings.Join(parts, " "))
}
