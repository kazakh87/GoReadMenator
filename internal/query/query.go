// Package query ist die Abfrageschicht für Werkzeuge (CLI, MCP). Sie liefert nur die angefragten
// Ausschnitte des Graphen und begrenzt jede Antwort auf ein Token-Budget. Der Quellcode wird nur
// auf ausdrückliche Anfrage und in kurzen Ausschnitten gelesen.
//
// Die Schicht spricht ausschließlich über store.Store. Ein SQLite-Backend ersetzt also nur die
// Implementierung, die Abfragen bleiben gleich. Die Namenssuche würde dort als LIKE laufen.
package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"goreadmenator/internal/gitx"
	"goreadmenator/internal/impact"
	"goreadmenator/internal/langver"
	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

const (
	// CharsPerToken ist eine konservative Schätzung: ein Token entspricht etwa vier Zeichen.
	CharsPerToken = 4
	// DefaultBudgetTokens ist das Standardbudget einer Antwort.
	DefaultBudgetTokens = 2500
	minBudgetTokens     = 200
	maxBudgetTokens     = 8000
	maxSourceLines      = 80
	summaryLimit        = 120
)

// ErrNotFound meldet, dass kein Symbol zur Anfrage passt.
var ErrNotFound = errors.New("nicht gefunden")

// ErrAmbiguous meldet, dass ein Name auf mehrere Symbole passt. Die Fehlermeldung nennt die Kandidaten.
var ErrAmbiguous = errors.New("mehrdeutig")

// Engine führt Abfragen auf einem Store aus. Symbolbestände werden einmal geladen.
type Engine struct {
	st     store.Store
	repo   string
	loaded bool
	syms   []model.Symbol
	byID   map[model.SymbolID]model.Symbol
	files  map[model.FileID]string
	locs   map[model.SymbolID][]model.Location
}

// New erstellt die Abfrageschicht.
func New(st store.Store, repo string) *Engine {
	return &Engine{st: st, repo: repo}
}

// Invalidate verwirft die geladenen Bestände (nach einem Neuaufbau des Index).
func (e *Engine) Invalidate() { e.loaded = false }

func (e *Engine) load(ctx context.Context) error {
	if e.loaded {
		return nil
	}
	syms, err := e.st.AllSymbols(ctx)
	if err != nil {
		return err
	}
	locs, err := e.st.AllLocations(ctx)
	if err != nil {
		return err
	}
	files, err := e.st.Files(ctx)
	if err != nil {
		return err
	}
	e.syms = syms
	e.byID = make(map[model.SymbolID]model.Symbol, len(syms))
	for _, s := range syms {
		e.byID[s.ID] = s
	}
	e.files = make(map[model.FileID]string, len(files))
	for _, f := range files {
		e.files[f.ID] = f.Path
	}
	e.locs = map[model.SymbolID][]model.Location{}
	for _, l := range locs {
		e.locs[l.SymbolID] = append(e.locs[l.SymbolID], l)
	}
	e.loaded = true
	return nil
}

// ---------- Budget und Paginierung ----------

// ClampBudget bringt ein Token-Budget in den erlaubten Bereich.
func ClampBudget(tokens int) int {
	if tokens <= 0 {
		tokens = DefaultBudgetTokens
	}
	if tokens < minBudgetTokens {
		tokens = minBudgetTokens
	}
	if tokens > maxBudgetTokens {
		tokens = maxBudgetTokens
	}
	return tokens * CharsPerToken
}

// Page ist das Ergebnis einer begrenzten Liste.
type Page[T any] struct {
	Items      []T    `json:"items"`
	Total      int    `json:"total"`
	Truncated  bool   `json:"truncated,omitempty"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// paginate nimmt Elemente ab dem Cursor auf, solange das Zeichenbudget reicht und limit nicht überschritten ist.
func paginate[T any](all []T, cursor string, limit, budgetChars int) Page[T] {
	offset := 0
	if cursor != "" {
		if v, err := strconv.Atoi(cursor); err == nil && v > 0 {
			offset = v
		}
	}
	page := Page[T]{Total: len(all)}
	used := 0
	i := offset
	for ; i < len(all); i++ {
		if limit > 0 && len(page.Items) >= limit {
			break
		}
		b, _ := json.Marshal(all[i])
		if used+len(b) > budgetChars && len(page.Items) > 0 {
			break
		}
		used += len(b)
		page.Items = append(page.Items, all[i])
	}
	if i < len(all) {
		page.Truncated = true
		page.NextCursor = strconv.Itoa(i)
	}
	if page.Items == nil {
		page.Items = []T{}
	}
	return page
}

func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut] != ' ' {
		cut--
	}
	if cut == 0 {
		cut = n
	}
	return s[:cut] + "…"
}

// ---------- Symbolauswahl ----------

// Ref ist ein kompakter Verweis auf ein Symbol.
type Ref struct {
	ID   model.SymbolID   `json:"id"`
	Kind model.SymbolKind `json:"kind"`
	FQN  string           `json:"fqn"`
	File string           `json:"file,omitempty"`
	Line int              `json:"line,omitempty"`
	Sig  string           `json:"sig,omitempty"`
}

func (e *Engine) ref(s model.Symbol) Ref {
	r := Ref{ID: s.ID, Kind: s.Kind, FQN: s.FQN, Sig: s.Signature}
	if locs := e.locs[s.ID]; len(locs) > 0 {
		l := pickLocation(locs)
		r.File = e.files[l.FileID]
		r.Line = l.Span.StartLine
	}
	return r
}

// pickLocation bevorzugt eine Quelldatei vor dem virtuellen Assembly eines XML-Imports.
func pickLocation(locs []model.Location) model.Location {
	for _, l := range locs {
		if l.Role != "" {
			return l
		}
	}
	return locs[0]
}

// resolveAll liefert alle Symbole zu einer ID, einem Namen oder einer FQN-Endung.
func (e *Engine) resolveAll(ref string) []model.Symbol {
	if s, ok := e.byID[model.SymbolID(ref)]; ok {
		return []model.Symbol{s}
	}
	// Eine Parameterliste im Namen (Start(System.Int32)) zählt nicht für die Suche nach dem FQN.
	if i := strings.IndexByte(ref, '('); i > 0 && strings.HasSuffix(ref, ")") {
		ref = ref[:i]
	}
	var exact, suffix []model.Symbol
	for _, s := range e.syms {
		switch {
		case s.Name == ref || s.FQN == ref:
			exact = append(exact, s)
		case strings.HasSuffix(s.FQN, "."+ref) || strings.HasSuffix(s.FQN, "#"+ref):
			suffix = append(suffix, s)
		}
	}
	out := exact
	if len(out) == 0 {
		out = suffix
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// one liefert genau ein Symbol. Bei keinem oder mehreren Treffern entsteht ein erklärender Fehler.
func (e *Engine) one(ctx context.Context, ref string) (model.Symbol, error) {
	if err := e.load(ctx); err != nil {
		return model.Symbol{}, err
	}
	cands := e.resolveAll(ref)
	switch len(cands) {
	case 0:
		return model.Symbol{}, fmt.Errorf("%w: kein Symbol %q", ErrNotFound, ref)
	case 1:
		return cands[0], nil
	}
	var ids []string
	for i, c := range cands {
		if i >= 10 {
			ids = append(ids, fmt.Sprintf("… (%d weitere)", len(cands)-10))
			break
		}
		ids = append(ids, string(c.ID))
	}
	return model.Symbol{}, fmt.Errorf("%w: %q, bitte per ID wählen: %s", ErrAmbiguous, ref, strings.Join(ids, ", "))
}

// ---------- Werkzeuge ----------

// FindSymbols sucht Symbole nach Teil des Namens oder der FQN. Genaue Treffer kommen zuerst.
func (e *Engine) FindSymbols(ctx context.Context, q, kind, cursor string, limit, budgetTokens int) (Page[Ref], error) {
	if err := e.load(ctx); err != nil {
		return Page[Ref]{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(q))
	var exact, partial []Ref
	for _, s := range e.syms {
		if kind != "" && string(s.Kind) != kind {
			continue
		}
		if s.Kind == model.KindNamespace && kind == "" {
			continue
		}
		name := strings.ToLower(s.Name)
		fqn := strings.ToLower(s.FQN)
		switch {
		case needle == "":
			partial = append(partial, e.ref(s))
		case name == needle || fqn == needle:
			exact = append(exact, e.ref(s))
		case strings.Contains(name, needle) || strings.Contains(fqn, needle):
			partial = append(partial, e.ref(s))
		}
	}
	all := append(exact, partial...)
	if limit <= 0 {
		limit = 30
	}
	return paginate(all, cursor, limit, ClampBudget(budgetTokens)), nil
}

// Detail ist die vollständige Beschreibung eines Symbols ohne Quelltext.
type Detail struct {
	ID        model.SymbolID   `json:"id"`
	Kind      model.SymbolKind `json:"kind"`
	FQN       string           `json:"fqn"`
	Container string           `json:"container,omitempty"`
	Sig       string           `json:"sig,omitempty"`
	Summary   string           `json:"summary,omitempty"`
	Remarks   string           `json:"remarks,omitempty"`
	Returns   string           `json:"returns,omitempty"`
	Params    []model.ParamDoc `json:"params,omitempty"`
	Locations []LocJSON        `json:"locations,omitempty"`
	Relations map[string]int   `json:"relations,omitempty"`
	DocSource string           `json:"docSource,omitempty"`
}

// LocJSON ist ein Fundort.
type LocJSON struct {
	File  string `json:"file"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Role  string `json:"role,omitempty"`
}

// GetSymbol liefert Signatur, Dokumentation, Fundorte und Anzahl der Beziehungen.
func (e *Engine) GetSymbol(ctx context.Context, ref string, budgetTokens int) (Detail, error) {
	s, err := e.one(ctx, ref)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{
		ID: s.ID, Kind: s.Kind, FQN: s.FQN, Container: string(s.Container), Sig: s.Signature,
		Summary: shorten(s.Summary, 600), Remarks: shorten(s.Remarks, 400), Returns: s.Returns,
		Params: s.ParamDocs, DocSource: s.DocSource,
	}
	for i, l := range e.locs[s.ID] {
		if i >= 5 {
			break
		}
		d.Locations = append(d.Locations, LocJSON{File: e.files[l.FileID], Start: l.Span.StartLine, End: l.Span.EndLine, Role: string(l.Role)})
	}
	rels, err := e.st.Relations(ctx, s.ID)
	if err != nil {
		return d, err
	}
	d.Relations = map[string]int{}
	for _, r := range rels {
		d.Relations[string(r.Kind)]++
	}
	if len(d.Relations) == 0 {
		d.Relations = nil
	}
	return d, nil
}

// APIItem ist eine Zeile der öffentlichen Oberfläche.
type APIItem struct {
	Kind model.SymbolKind `json:"kind"`
	FQN  string           `json:"fqn"`
	Sig  string           `json:"sig,omitempty"`
	Doc  string           `json:"doc,omitempty"`
}

// API listet die öffentlich sichtbaren Symbole unter einem Namensbereich (Präfix der FQN).
// Sichtbar sind Symbole mit Modifikator public (C#) oder export (TS) und alle aus XML-Dokumentation.
func (e *Engine) API(ctx context.Context, scope, cursor string, budgetTokens int) (Page[APIItem], error) {
	if err := e.load(ctx); err != nil {
		return Page[APIItem]{}, err
	}
	var items []APIItem
	for _, s := range e.syms {
		if s.Kind == model.KindNamespace || s.Kind == model.KindModule {
			continue
		}
		if scope != "" && !strings.HasPrefix(s.FQN, scope) {
			continue
		}
		if !e.visible(s) {
			continue
		}
		items = append(items, APIItem{Kind: s.Kind, FQN: s.FQN, Sig: s.Signature, Doc: shorten(s.Summary, summaryLimit)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].FQN < items[j].FQN })
	return paginate(items, cursor, 0, ClampBudget(budgetTokens)), nil
}

// visible prüft, ob ein Symbol und alle umgebenden Typen öffentlich sind. Ein öffentliches Member
// eines internen Typs gehört nicht zur öffentlichen API.
func (e *Engine) visible(s model.Symbol) bool {
	for depth := 0; depth < 32; depth++ {
		if !isPublic(s) {
			return false
		}
		if s.Container == "" {
			return true
		}
		parent, ok := e.byID[s.Container]
		if !ok {
			return true // Container unbekannt (etwa virtueller Namensraum): nicht ausschließen
		}
		if parent.Kind == model.KindNamespace {
			return true
		}
		s = parent
	}
	return true
}

func isPublic(s model.Symbol) bool {
	if s.DocSource == "xml" {
		return true
	}
	for _, m := range s.Modifiers {
		if m == "public" || m == "export" {
			return true
		}
	}
	return false
}

// Edge ist eine Beziehung zwischen zwei Symbolen (als FQN, damit die Antwort kurz bleibt).
type Edge struct {
	From string `json:"from"`
	Kind string `json:"kind"`
	To   string `json:"to"`
}

// Neighborhood liefert die Beziehungen eines Symbols bis zur Tiefe depth (1 bis 3).
// relKinds begrenzt die Beziehungsarten (leer = alle).
func (e *Engine) Neighborhood(ctx context.Context, ref string, depth int, relKinds []string, cursor string, budgetTokens int) (Page[Edge], error) {
	s, err := e.one(ctx, ref)
	if err != nil {
		return Page[Edge]{}, err
	}
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	allowed := map[string]bool{}
	for _, k := range relKinds {
		allowed[k] = true
	}
	seen := map[model.SymbolID]bool{s.ID: true}
	seenEdge := map[string]bool{}
	frontier := []model.SymbolID{s.ID}
	var edges []Edge
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []model.SymbolID
		for _, id := range frontier {
			rels, err := e.st.Relations(ctx, id)
			if err != nil {
				return Page[Edge]{}, err
			}
			for _, r := range rels {
				if len(allowed) > 0 && !allowed[string(r.Kind)] {
					continue
				}
				key := string(r.From) + "|" + string(r.Kind) + "|" + string(r.To)
				if seenEdge[key] {
					continue
				}
				seenEdge[key] = true
				edges = append(edges, Edge{From: e.fqnOf(r.From), Kind: string(r.Kind), To: e.fqnOf(r.To)})
				other := r.To
				if r.To == id {
					other = r.From
				}
				if !seen[other] {
					seen[other] = true
					next = append(next, other)
				}
			}
		}
		frontier = next
	}
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Kind != edges[j].Kind {
			return edges[i].Kind < edges[j].Kind
		}
		return edges[i].From+edges[i].To < edges[j].From+edges[j].To
	})
	return paginate(edges, cursor, 0, ClampBudget(budgetTokens)), nil
}

func (e *Engine) fqnOf(id model.SymbolID) string {
	if s, ok := e.byID[id]; ok {
		return s.FQN
	}
	return string(id)
}

// Usage ist eine Verwendung eines Symbols, auch aus höheren Ebenen.
type Usage struct {
	Depth int    `json:"depth"`
	User  string `json:"user,omitempty"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Kind  string `json:"kind"`
}

// Usages liefert die Verwendungen eines Symbols bis zur Tiefe depth (1 bis 3).
func (e *Engine) Usages(ctx context.Context, ref string, depth int, cursor string, budgetTokens int) (Page[Usage], error) {
	s, err := e.one(ctx, ref)
	if err != nil {
		return Page[Usage]{}, err
	}
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	hits, err := impact.Analyze(ctx, e.st, s.ID, depth)
	if err != nil {
		return Page[Usage]{}, err
	}
	items := make([]Usage, 0, len(hits))
	for _, h := range hits {
		items = append(items, Usage{Depth: h.Depth, User: e.fqnOf(h.User), File: h.File, Line: h.Span.StartLine, Kind: string(h.Kind)})
	}
	return paginate(items, cursor, 0, ClampBudget(budgetTokens)), nil
}

// ContextPack ist ein kompaktes Paket für eine Aufgabe: Symbol, Beziehungen und Verwendungen,
// in dieser Reihenfolge, bis das Budget erreicht ist.
type ContextPack struct {
	Symbol    Detail  `json:"symbol"`
	Edges     []Edge  `json:"edges"`
	Usages    []Usage `json:"usages"`
	Truncated bool    `json:"truncated,omitempty"`
}

func (e *Engine) Context(ctx context.Context, ref string, budgetTokens int) (ContextPack, error) {
	budget := ClampBudget(budgetTokens)
	d, err := e.GetSymbol(ctx, ref, budgetTokens)
	if err != nil {
		return ContextPack{}, err
	}
	pack := ContextPack{Symbol: d}
	used := jsonLen(d)
	edges, err := e.Neighborhood(ctx, ref, 1, nil, "", budgetTokens)
	if err != nil {
		return pack, err
	}
	for _, ed := range edges.Items {
		if used+jsonLen(ed) > budget {
			pack.Truncated = true
			break
		}
		used += jsonLen(ed)
		pack.Edges = append(pack.Edges, ed)
	}
	if !pack.Truncated {
		us, err := e.Usages(ctx, ref, 1, "", budgetTokens)
		if err != nil {
			return pack, err
		}
		for _, u := range us.Items {
			if used+jsonLen(u) > budget {
				pack.Truncated = true
				break
			}
			used += jsonLen(u)
			pack.Usages = append(pack.Usages, u)
		}
	}
	if pack.Edges == nil {
		pack.Edges = []Edge{}
	}
	if pack.Usages == nil {
		pack.Usages = []Usage{}
	}
	return pack, nil
}

func jsonLen(v any) int {
	b, _ := json.Marshal(v)
	return len(b)
}

// SourceLines ist ein Ausschnitt aus dem Quelltext (aus dem HEAD-Commit).
type SourceLines struct {
	File  string   `json:"file"`
	Start int      `json:"start"`
	End   int      `json:"end"`
	Lines []string `json:"lines"`
}

// Source liefert höchstens 80 Zeilen. Der Assistent soll den Code nur gezielt lesen.
func (e *Engine) Source(ctx context.Context, file string, start, end int) (SourceLines, error) {
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}
	if end-start+1 > maxSourceLines {
		end = start + maxSourceLines - 1
	}
	head, err := gitx.Runner{Dir: e.repo}.Head(ctx)
	if err != nil {
		return SourceLines{}, err
	}
	data, err := gitx.Runner{Dir: e.repo}.ShowFile(ctx, head, file)
	if err != nil {
		return SourceLines{}, fmt.Errorf("Datei %q im HEAD nicht gefunden", file)
	}
	lines := strings.Split(string(data), "\n")
	if start > len(lines) {
		return SourceLines{}, fmt.Errorf("Datei hat nur %d Zeilen", len(lines))
	}
	if end > len(lines) {
		end = len(lines)
	}
	return SourceLines{File: file, Start: start, End: end, Lines: lines[start-1 : end]}, nil
}

// Status beschreibt den Zustand des Index gegenüber dem Repository.
type Status struct {
	Head         string   `json:"head,omitempty"`
	Indexed      string   `json:"indexedCommit,omitempty"`
	UpToDate     bool     `json:"upToDate"`
	Symbols      int      `json:"symbols"`
	References   int      `json:"references"`
	LangWarnings []string `json:"langWarnings,omitempty"`
}

func (e *Engine) Status(ctx context.Context) (Status, error) {
	var s Status
	if err := e.load(ctx); err != nil {
		return s, err
	}
	s.Symbols = len(e.syms)
	refs, err := e.st.AllReferences(ctx)
	if err != nil {
		return s, err
	}
	s.References = len(refs)
	if v, ok, _ := e.st.Meta(ctx, "last_commit"); ok {
		s.Indexed = v
	}
	if e.repo != "" {
		if head, err := (gitx.Runner{Dir: e.repo}).Head(ctx); err == nil {
			s.Head = head
			s.UpToDate = head == s.Indexed
		}
		s.LangWarnings = langver.Warnings(e.repo)
	}
	return s, nil
}

// ChangeItem ist ein geänderter Bereich mit seiner Wirkung.
type ChangeItem struct {
	Symbol string `json:"symbol"`
	Kind   string `json:"kind"`
	File   string `json:"file"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Direct bool   `json:"direct"`
	Usages int    `json:"usages"`
}

// Changes listet die Symbole, die zwischen from und HEAD geändert wurden, mit Anzahl der Verwendungen.
func (e *Engine) Changes(ctx context.Context, from, cursor string, budgetTokens int) (Page[ChangeItem], error) {
	head, err := gitx.Runner{Dir: e.repo}.Head(ctx)
	if err != nil {
		return Page[ChangeItem]{}, err
	}
	changed, err := impact.Changed(ctx, e.st, e.repo, from, head)
	if err != nil {
		return Page[ChangeItem]{}, err
	}
	var items []ChangeItem
	for _, c := range changed {
		n := 0
		if c.Direct {
			hits, err := impact.Analyze(ctx, e.st, c.Symbol.ID, 1)
			if err != nil {
				return Page[ChangeItem]{}, err
			}
			n = len(hits)
		}
		items = append(items, ChangeItem{Symbol: c.Symbol.FQN, Kind: string(c.Symbol.Kind), File: c.File,
			Start: c.Span.StartLine, End: c.Span.EndLine, Direct: c.Direct, Usages: n})
	}
	return paginate(items, cursor, 0, ClampBudget(budgetTokens)), nil
}

// Endpoint ist eine Route im Backend mit den Aufrufern aus dem Frontend, die sie erreichen.
type Endpoint struct {
	Route   string   `json:"route"` // "VERB pfad", normalisiert
	Action  string   `json:"action"`
	File    string   `json:"file,omitempty"`
	Line    int      `json:"line,omitempty"`
	Callers []string `json:"callers,omitempty"`
}

// Endpoints listet alle Routen des Backends. Mit calls_endpoint-Beziehungen sieht man, welche Frontend-Funktionen sie aufrufen.
func (e *Engine) Endpoints(ctx context.Context, cursor string, budgetTokens int) (Page[Endpoint], error) {
	if err := e.load(ctx); err != nil {
		return Page[Endpoint]{}, err
	}
	refs, err := e.st.AllReferences(ctx)
	if err != nil {
		return Page[Endpoint]{}, err
	}
	var items []Endpoint
	routes := append(filterKind(refs, model.RefHTTPRoute), model.ExpandConventionRoutes(refs)...)
	for _, r := range routes {
		ep := Endpoint{Route: r.TargetName, Action: e.fqnOf(r.Enclosing), File: e.files[r.FileID], Line: r.Span.StartLine}
		rels, err := e.st.Relations(ctx, r.Enclosing)
		if err != nil {
			return Page[Endpoint]{}, err
		}
		for _, rel := range rels {
			if rel.Kind == model.RelCallsEndpoint && rel.To == r.Enclosing {
				ep.Callers = append(ep.Callers, e.fqnOf(rel.From))
			}
		}
		sort.Strings(ep.Callers)
		items = append(items, ep)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Route != items[j].Route {
			return items[i].Route < items[j].Route
		}
		return items[i].Action < items[j].Action
	})
	return paginate(items, cursor, 0, ClampBudget(budgetTokens)), nil
}

// filterKind liefert die Verweise einer Art.
func filterKind(refs []model.Reference, kind model.RefKind) []model.Reference {
	var out []model.Reference
	for _, r := range refs {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}
