package resolve

import (
	"context"
	"path"
	"strconv"
	"strings"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

// Options steuern den Lauf.
type Options struct {
	// RepoDir ist das Repository-Root. Dort wird tsconfig.json gelesen. Leer = keine Pfad-Zuordnung.
	RepoDir string
	// OnlyNames begrenzt die Auflösung auf Verwendungen mit diesen Rohnamen (für inkrementelle Läufe).
	// nil bedeutet alle Verwendungen.
	OnlyNames map[string]bool
}

// Stats fasst den Lauf zusammen.
type Stats struct {
	References   int
	Relations    int // abgeleitete Beziehungen
	Exact        int
	Resolved     int
	Heuristic    int
	Unresolved   int
	Imports      int // TS/JS-Imports (ohne C#-using)
	ImportsFound int // davon auf eine Projektdatei aufgelöst
}

// binding ist die Auflösung einer lokalen Import-Bindung auf eine Projektdatei.
type binding struct {
	target   model.FileID
	imported string
}

// usingEntry ist eine C#-using-Direktive, vorbereitet für die Suche.
type usingEntry struct {
	prefix string // Namensraum mit Punkt, etwa "App.Models."
	alias  string // gesetzt bei `using A = B`
	target string // B bei Alias
	static bool
}

type resolver struct {
	// fieldType ordnet Felder, Properties und Variablen ihrem aufgelösten Typ zu (aus has_type-Verweisen
	// der ersten Auflösungsrunde). Damit lassen sich Aufrufe über Felder auflösen: _svc.Save() ist ein
	// Aufruf auf den Typ von _svc.
	fieldType map[model.SymbolID]model.SymbolID
	// exactName ist der Name, dessen Schreibweise exakt stimmen muss (nur für C#, die case-sensitiv ist).
	// Leer bedeutet: keine Prüfung.
	exactName string
	ix        *symIndex
	bindings  map[model.FileID]map[string]binding // lokale Import-Bindungen je Datei
	reexports map[model.FileID]map[string]binding // export { a as b } from "x"
	stars     map[model.FileID][]model.FileID     // export * from "x"
	usings    map[model.FileID][]usingEntry
}

// Run löst Imports und Verwendungen im Store auf und schreibt die Ergebnisse zurück.
func Run(ctx context.Context, st store.Store, opt Options) (Stats, error) {
	var stats Stats
	ix, err := buildIndex(ctx, st)
	if err != nil {
		return stats, err
	}
	tsc := loadTSConfig(opt.RepoDir)
	r := &resolver{
		ix:        ix,
		bindings:  map[model.FileID]map[string]binding{},
		reexports: map[model.FileID]map[string]binding{},
		stars:     map[model.FileID][]model.FileID{},
		usings:    map[model.FileID][]usingEntry{},
	}

	// 1. Imports: Modulpfade auf Dateien abbilden, Bindungen merken.
	imps, err := st.AllImports(ctx)
	if err != nil {
		return stats, err
	}
	var impUpdates []model.Import
	for i := range imps {
		im := &imps[i]
		file := ix.files[im.FileID]
		if im.Kind == model.ImportUsing {
			r.usings[im.FileID] = append(r.usings[im.FileID], newUsing(im.Raw))
			continue
		}
		if file.Lang != model.LangTSJS {
			continue
		}
		stats.Imports++
		before := im.ResolvedID
		im.ResolvedID = resolveModule(ix, file.Path, im.Raw, tsc)
		if im.ResolvedID != before {
			impUpdates = append(impUpdates, *im)
		}
		if im.ResolvedID != 0 {
			stats.ImportsFound++
			switch im.Kind {
			case model.ImportReexport:
				if len(im.Bindings) == 0 {
					r.stars[im.FileID] = append(r.stars[im.FileID], im.ResolvedID)
				}
				for _, b := range im.Bindings {
					setBinding(r.reexports, im.FileID, b.Local, binding{target: im.ResolvedID, imported: b.Imported})
				}
			default:
				for _, b := range im.Bindings {
					setBinding(r.bindings, im.FileID, b.Local, binding{target: im.ResolvedID, imported: b.Imported})
				}
			}
		}
	}
	if len(impUpdates) > 0 {
		if err := st.UpdateImports(ctx, impUpdates); err != nil {
			return stats, err
		}
	}

	// 2. Verwendungen auflösen.
	refs, err := st.AllReferences(ctx)
	if err != nil {
		return stats, err
	}
	// Zwei Runden: Die erste löst Typen auf, die zweite nutzt die Typen von Feldern für Aufrufe.
	r.fieldType = map[model.SymbolID]model.SymbolID{}
	for round := 0; round < 2; round++ {
		if round == 1 {
			for _, ref := range refs {
				if ref.Kind == model.RefTypeRef && ref.Role == model.RoleHasType && ref.TargetID != "" && ref.Enclosing != "" {
					r.fieldType[ref.Enclosing] = ref.TargetID
				}
			}
			// Nur die Verwendungszähler zurücksetzen, die Import-Zähler bleiben erhalten.
			stats.References, stats.Exact, stats.Resolved, stats.Heuristic, stats.Unresolved = 0, 0, 0, 0, 0
		}
		var refUpdates []model.Reference
		for i := range refs {
			ref := refs[i]
			if opt.OnlyNames != nil && !opt.OnlyNames[ref.TargetName] {
				continue
			}
			if ref.Kind == model.RefHTTPRoute || ref.Kind == model.RefHTTPCall || ref.Kind == model.RefHTTPConvention {
				continue // HTTP-Verweise haben keinen Namen zum Auflösen, sie werden in linkEndpoints verbunden
			}
			stats.References++
			target, conf, ok := r.resolveRef(ref)
			changed := false
			if ok {
				if ref.TargetID != target || ref.Confidence != conf {
					ref.TargetID = target
					ref.Confidence = conf
					changed = true
				}
				switch conf {
				case model.ConfExact:
					stats.Exact++
				case model.ConfResolved:
					stats.Resolved++
				case model.ConfHeuristic:
					stats.Heuristic++
				}
			} else {
				stats.Unresolved++
				if ref.TargetID != "" {
					// Vorher aufgelöst, jetzt mehrdeutig oder verschwunden: Ziel zurücknehmen.
					ref.TargetID = ""
					changed = true
				}
				// Unaufgelöst heißt: nur der Name ist bekannt. Das ist HEURISTIC, auch wenn der Parser
				// für explizite Angaben EXACT gesetzt hatte.
				if ref.Confidence != model.ConfHeuristic {
					ref.Confidence = model.ConfHeuristic
					changed = true
				}
			}
			if changed {
				refUpdates = append(refUpdates, ref)
				refs[i] = ref
			}
		}
		if len(refUpdates) > 0 {
			if err := st.UpdateReferences(ctx, refUpdates); err != nil {
				return stats, err
			}
		}
	}

	// 3. Beziehungen aus den aufgelösten Verweisen ableiten (ersetzt die vorherigen abgeleiteten).
	rels := deriveRelations(refs)
	rels = append(rels, linkEndpoints(refs)...)
	stats.Relations = len(rels)
	if err := st.SetDerivedRelations(ctx, rels); err != nil {
		return stats, err
	}
	return stats, nil
}

func newUsing(raw string) usingEntry {
	switch {
	case strings.HasPrefix(raw, "static "):
		ns := strings.TrimPrefix(raw, "static ")
		return usingEntry{prefix: ns + ".", static: true}
	case strings.Contains(raw, "="):
		i := strings.Index(raw, "=")
		return usingEntry{alias: raw[:i], target: raw[i+1:]}
	default:
		return usingEntry{prefix: raw + "."}
	}
}

// deriveRelations leitet Beziehungen aus aufgelösten Verweisen ab:
// inherits und implements aus Basisangaben, has_type, param_type und returns aus Typverweisen
// mit Rolle, attribute_of aus Attributen. Verweise mit HEURISTIC werden nicht übernommen, weil
// eine Beziehung sonst eine unsichere Zuordnung als Fakt darstellen würde.
func deriveRelations(refs []model.Reference) []model.Relation {
	var out []model.Relation
	seen := map[relKey]bool{}
	add := func(from, to model.SymbolID, kind model.RelationKind, file model.FileID) {
		if from == "" || to == "" {
			return
		}
		key := relKey{from, to, kind}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, model.Relation{FileID: file, From: from, To: to, Kind: kind})
	}
	for _, r := range refs {
		if r.TargetID == "" || r.Confidence == model.ConfHeuristic {
			continue
		}
		switch {
		case r.Kind == model.RefInherits:
			add(r.Enclosing, r.TargetID, model.RelInherits, r.FileID)
		case r.Kind == model.RefImplements:
			add(r.Enclosing, r.TargetID, model.RelImplements, r.FileID)
		case r.Kind == model.RefTypeRef && r.Role == model.RoleHasType:
			add(r.Enclosing, r.TargetID, model.RelHasType, r.FileID)
		case r.Kind == model.RefTypeRef && r.Role == model.RoleParamType:
			add(r.Enclosing, r.TargetID, model.RelParamType, r.FileID)
		case r.Kind == model.RefTypeRef && r.Role == model.RoleReturns:
			add(r.Enclosing, r.TargetID, model.RelReturns, r.FileID)
		case r.Kind == model.RefAttribute:
			add(r.TargetID, r.Enclosing, model.RelAttributeOf, r.FileID)
		}
	}
	return out
}

type relKey struct {
	from, to model.SymbolID
	kind     model.RelationKind
}

func setBinding(m map[model.FileID]map[string]binding, file model.FileID, local string, b binding) {
	inner := m[file]
	if inner == nil {
		inner = map[string]binding{}
		m[file] = inner
	}
	inner[local] = b
}

// resolveRef wählt je nach Art und Sprache die passende Regel.
func (r *resolver) resolveRef(ref model.Reference) (model.SymbolID, model.Confidence, bool) {
	file := r.ix.files[ref.FileID]
	enc := ref.Enclosing

	if ref.Kind == model.RefCref {
		// Doku-Verweise nennen oft den vollen Namen. Der exakte FQN ist eindeutig.
		if strings.Contains(ref.TargetName, "(") {
			if id, ok := r.crefWithParams(ref.TargetName, file.Lang); ok {
				return id, model.ConfExact, true
			}
		} else if id, ok := r.choose(r.ix.fqnLookup(file.Lang, ref.TargetName), ref, 0); ok {
			return id, model.ConfExact, true
		}
	}

	if ref.Qualifier != "" && ref.Kind == model.RefCall && ref.Qualifier != "this" && ref.Qualifier != "base" {
		if id, conf, ok := r.resolveQualified(ref, file, enc); ok {
			return id, conf, true
		}
	}

	names := [2]string{ref.TargetName, ""}
	count := 1
	if ref.Kind == model.RefAttribute {
		names = [2]string{ref.TargetName + "Attribute", ref.TargetName}
		count = 2
	}
	for _, name := range names[:count] {
		if name == "" {
			continue
		}
		var id model.SymbolID
		var conf model.Confidence
		var ok bool
		r.exactName = ""
		if caseSensitive(file.Lang) {
			r.exactName = name
		}
		if file.Lang != model.LangTSJS {
			id, conf, ok = r.resolveCSharp(name, ref, enc, file.ID)
		} else {
			id, conf, ok = r.resolveTSJS(name, ref, enc, file)
		}
		r.exactName = ""
		if ok {
			return id, conf, true
		}
	}
	return "", "", false
}

// resolveQualified löst `q.name(...)` auf, wenn q ein Namespace-Import, ein importierter Typ
// oder ein bekannter Typ im Scope ist. Sonst bleibt es bei der Namensauflösung.
func (r *resolver) resolveQualified(ref model.Reference, file model.File, enc model.SymbolID) (model.SymbolID, model.Confidence, bool) {
	kinds := kindsFor(ref.Kind)
	lang := file.Lang
	// Der Qualifier selbst wird ohne Schreibweisenprüfung gesucht, nur das Member muss exakt stimmen.
	r.exactName = ""
	q := ref.Qualifier
	if file.Lang == model.LangTSJS {
		b, ok := r.bindings[file.ID][q]
		if !ok {
			return "", "", false
		}
		if b.imported == "*" {
			if id, ok := r.resolveExport(b.target, ref.TargetName, kinds, 0); ok {
				return id, model.ConfExact, true
			}
			return "", "", false
		}
		// Der Qualifier darf jede Art sein (Namespace, Klasse, Objekt), nur das Member ist eingeschränkt.
		owner, ok := r.resolveBinding(b, 0)
		if !ok {
			return "", "", false
		}
		if id, ok := r.choose(r.ix.childLookup(lang, r.ix.childPrefix[owner], ref.TargetName), ref, kinds); ok {
			return id, model.ConfExact, true
		}
		return "", "", false
	}
	typeRef := model.Reference{Kind: model.RefTypeRef, TargetName: q, FileID: ref.FileID}
	owner, _, ok := r.resolveCSharp(q, typeRef, enc, file.ID)
	if !ok {
		// Kein Typ, aber vielleicht ein Feld oder eine Eigenschaft mit bekanntem Typ: svc.Save()
		field, found := r.fieldOwner(q, enc, lang)
		if !found {
			return "", "", false
		}
		t, known := r.fieldType[field]
		if !known {
			return "", "", false
		}
		owner = t
	}
	if caseSensitive(lang) {
		r.exactName = ref.TargetName
	}
	id, ok := r.choose(r.ix.childLookup(lang, r.ix.childPrefix[owner], ref.TargetName), ref, kinds)
	r.exactName = ""
	if ok {
		return id, model.ConfExact, true
	}
	return "", "", false
}

// fieldOwner sucht ein Feld, eine Eigenschaft oder eine Variable mit diesem Namen im Scope des Verweises.
func (r *resolver) fieldOwner(name string, enc model.SymbolID, lang model.Language) (model.SymbolID, bool) {
	var found model.SymbolID
	r.scopeChain(enc, func(c model.SymbolID) bool {
		prefix, ok := r.ix.childPrefix[c]
		if !ok {
			return true
		}
		for _, id := range r.ix.childLookup(lang, prefix, name) {
			if kindBit(r.ix.kindOf[id])&(bitField|bitProperty|bitVariable) != 0 {
				found = id
				return false
			}
		}
		return true
	})
	return found, found != ""
}

// resolveExport findet ein exportiertes Symbol einer Datei, auch über Re-Exporte.
func (r *resolver) resolveExport(file model.FileID, name string, kinds kindSet, depth int) (model.SymbolID, bool) {
	if depth > 8 || name == "*" {
		return "", false
	}
	lang := r.ix.files[file].Lang
	if name == "default" {
		fp := r.ix.files[file].Path + "#"
		var cands []model.SymbolID
		for _, id := range r.ix.fileSyms[file] {
			fqn := r.ix.fqnOf[id]
			if strings.HasPrefix(fqn, fp) && hasDefaultModifier(r.ix, id) {
				cands = append(cands, id)
			}
		}
		if id, ok := r.choose(cands, model.Reference{}, kinds); ok {
			return id, true
		}
	} else if id, ok := r.choose(r.ix.childLookup(lang, r.ix.fileScope[file], name), model.Reference{}, kinds); ok {
		return id, true
	}
	if b, ok := r.reexports[file][name]; ok {
		return r.resolveExport(b.target, b.imported, kinds, depth+1)
	}
	for _, t := range r.stars[file] {
		if id, ok := r.resolveExport(t, name, kinds, depth+1); ok {
			return id, true
		}
	}
	return "", false
}

// crefWithParams löst `Namespace.Typ.Member(Typ1,Typ2)` über die exakte ID auf.
func (r *resolver) crefWithParams(name string, lang model.Language) (model.SymbolID, bool) {
	open := strings.IndexByte(name, '(')
	base := name[:open]
	inner := name[open+1 : len(name)-1]
	want := 0
	if strings.TrimSpace(inner) != "" {
		want = paramCount(model.SymbolID(base + "(" + inner + ")")) // Anzahl über dieselbe Zählung wie IDs
	}
	for _, id := range r.ix.fqnLookup(lang, base) {
		if r.ix.paramsOf[id] == want {
			return id, true
		}
	}
	return "", false
}

// choose wählt aus Kandidaten nach Art und Argumentzahl. Gibt ok=false zurück, wenn es keinen
// oder mehrere Kandidaten gibt. Mehrdeutige Treffer werden nie geraten.
func (r *resolver) choose(ids []model.SymbolID, ref model.Reference, kinds kindSet) (model.SymbolID, bool) {
	var pass []model.SymbolID
	typeCheck := isTypeRef(ref.Kind)
	for _, id := range ids {
		if kinds != 0 && kindBit(r.ix.kindOf[id])&kinds == 0 {
			continue
		}
		if r.exactName != "" && lastSegment(r.ix.fqnOf[id]) != r.exactName {
			continue // C#: andere Schreibweise ist ein anderer Name
		}
		if typeCheck && r.ix.typeArityOf[id] != ref.Arity {
			continue // generische Typen nur mit passender Anzahl Typargumente
		}
		pass = append(pass, id)
	}
	if len(pass) > 1 && (ref.Kind == model.RefCall || ref.Kind == model.RefConstruct) && ref.Arity >= 0 {
		var byArity []model.SymbolID
		for _, id := range pass {
			pc := r.ix.paramsOf[id]
			if pc == -1 || pc == ref.Arity {
				byArity = append(byArity, id)
			}
		}
		if len(byArity) == 0 {
			for _, id := range pass {
				if r.ix.paramsOf[id] >= ref.Arity {
					byArity = append(byArity, id)
				}
			}
		}
		pass = byArity
	}
	if len(pass) == 1 {
		return pass[0], true
	}
	return "", false
}

// lastSegment liefert den letzten Namensteil eines FQN ohne Arity-Suffix.
func lastSegment(fqn string) string {
	if i := strings.LastIndexAny(fqn, ".#"); i >= 0 {
		fqn = fqn[i+1:]
	}
	return stripArity(fqn)
}

func isTypeRef(k model.RefKind) bool {
	switch k {
	case model.RefTypeRef, model.RefInherits, model.RefImplements:
		return true
	}
	return false
}

// typeArity liefert die Anzahl der Typparameter aus dem FQN (`Name`2` -> 2), sonst 0.
func typeArity(id model.SymbolID) int {
	s := string(id)
	i := strings.LastIndexByte(s, '`')
	if i < 0 {
		return 0
	}
	n := 0
	for _, c := range s[i+1:] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// kindsFor beschränkt die zulässigen Symbolarten je nach Verwendungsart. 0 = keine Einschränkung.
func kindsFor(k model.RefKind) kindSet {
	switch k {
	case model.RefCall:
		return bitMethod | bitFunction | bitConstructor | bitProperty | bitVariable
	case model.RefConstruct:
		return bitClass | bitStruct | bitRecord | bitConstructor | bitFunction
	case model.RefTypeRef, model.RefInherits, model.RefImplements:
		return bitClass | bitStruct | bitInterface | bitRecord | bitEnum | bitTypeAlias
	case model.RefWrite:
		return bitField | bitProperty | bitVariable
	case model.RefAttribute:
		return bitClass | bitFunction
	case model.RefJSXComponent:
		return bitFunction | bitClass | bitVariable
	}
	return 0
}

// scopeChain liefert das umgebende Symbol und seine Container nach außen (ohne Allokation pro Schritt).
func (r *resolver) scopeChain(enc model.SymbolID, visit func(model.SymbolID) bool) {
	for cur := enc; cur != ""; {
		if !visit(cur) {
			return
		}
		cur = r.ix.containerOf[cur]
	}
}

// uniqueByName sucht global einen eindeutigen Treffer über den Namen. Ergebnis ist HEURISTIC.
// Das Ergebnis hängt nur von Name und Art der Verwendung ab und wird deshalb gemerkt.
func (r *resolver) uniqueByName(lang model.Language, name string, ref model.Reference, kinds kindSet) (model.SymbolID, model.Confidence, bool) {
	key := uniqueKey{name: name, kind: ref.Kind, arity: ref.Arity, lang: lang}
	res, ok := r.ix.uniqueMemo[key]
	if !ok {
		var cands []model.SymbolID
		if lang == model.LangTSJS {
			for _, id := range r.ix.nameLookup(lang, name) {
				if id.Lang() == model.LangTSJS {
					cands = append(cands, id)
				}
			}
		} else {
			cands = r.ix.nameLookup(lang, name)
		}
		id, found := r.choose(cands, ref, kinds)
		res = uniqueResult{id: id, ok: found}
		r.ix.uniqueMemo[key] = res
	}
	if res.ok {
		return res.id, model.ConfHeuristic, true
	}
	return "", "", false
}

// resolveCSharp: Scope-Kette, dann using-Direktiven, dann global.
func (r *resolver) resolveCSharp(name string, ref model.Reference, enc model.SymbolID, file model.FileID) (model.SymbolID, model.Confidence, bool) {
	kinds := kindsFor(ref.Kind)
	lang := r.ix.files[file].Lang

	// Qualifizierter Name (Shop.Models.Order): als vollständiger Name suchen, vom innersten Namensraum
	// nach außen. Relative Namen (Models.Order innerhalb von Shop) werden so ebenfalls gefunden. Ein Treffer
	// ist exakt. Sonst gilt der letzte Teil wie bisher.
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		last := name[i+1:]
		saved := r.exactName
		r.exactName = last
		defer func() { r.exactName = saved }()
		if hit, conf, ok := r.qualifiedLookup(name, ref, kinds, lang, enc, file); ok {
			return hit, conf, true
		}
		name = last
	}

	// 1. Scope-Kette: Member, verschachtelter Typ, Namespace.
	var found model.SymbolID
	var stop bool
	r.scopeChain(enc, func(c model.SymbolID) bool {
		prefix, ok := r.ix.childPrefix[c]
		if !ok {
			return true
		}
		ids := r.ix.childLookup(lang, prefix, name)
		if len(ids) == 0 {
			return true
		}
		if id, ok := r.choose(ids, ref, kinds); ok {
			found = id
		} else {
			stop = true // mehrdeutig oder falsche Art: nicht weitersuchen
		}
		return false
	})
	if found != "" {
		return found, model.ConfExact, true
	}
	if stop {
		return "", "", false
	}

	// 2. using-Direktiven der Datei.
	for _, u := range r.usings[file] {
		switch {
		case u.static:
			if id, ok := r.choose(r.ix.childLookup(lang, u.prefix, name), ref, kinds); ok {
				return id, model.ConfResolved, true
			}
		case u.alias != "":
			if fold(lang, u.alias) == fold(lang, name) {
				// Der Alias-Name ist lokal, das Ziel hat einen anderen Namen: keine Schreibweisenprüfung
				saved := r.exactName
				r.exactName = ""
				id, ok := r.choose(r.ix.fqnLookup(lang, u.target), ref, kinds)
				r.exactName = saved
				if ok {
					return id, model.ConfResolved, true
				}
			}
		default:
			if id, ok := r.choose(r.ix.childLookup(lang, u.prefix, name), ref, kinds); ok {
				return id, model.ConfResolved, true
			}
		}
	}

	// 3. Global im Wurzel-Namensraum.
	if id, ok := r.choose(r.ix.childLookup(lang, "", name), ref, kinds); ok {
		return id, model.ConfResolved, true
	}

	// Ein Aufruf auf einen Typnamen, den das Projekt nicht kennt (WebApplication.Create()), ist ein Fremdaufruf.
	// Der eindeutige Name im Projekt würde hier falsch treffen. Kleingeschriebene Qualifier (Variablen) bleiben erlaubt.
	if ref.Qualifier != "" && ref.Qualifier[0] >= 'A' && ref.Qualifier[0] <= 'Z' {
		return "", "", false
	}
	// 4. Eindeutiger Name im Projekt.
	return r.uniqueByName(lang, name, ref, kinds)
}

// resolveTSJS: Scope-Kette, Datei-Ebene, Import-Bindungen, dann eindeutiger Name.
func (r *resolver) resolveTSJS(name string, ref model.Reference, enc model.SymbolID, file model.File) (model.SymbolID, model.Confidence, bool) {
	kinds := kindsFor(ref.Kind)
	lang := file.Lang

	// 1. Scope-Kette (Klassenmitglieder über this/Klasse).
	var found model.SymbolID
	var stop bool
	r.scopeChain(enc, func(c model.SymbolID) bool {
		prefix, ok := r.ix.childPrefix[c]
		if !ok {
			return true
		}
		ids := r.ix.childLookup(lang, prefix, name)
		if len(ids) == 0 {
			return true
		}
		if id, ok := r.choose(ids, ref, kinds); ok {
			found = id
		} else {
			stop = true
		}
		return false
	})
	if found != "" {
		return found, model.ConfExact, true
	}
	if stop {
		return "", "", false
	}

	// 2. Datei-Ebene.
	if id, ok := r.choose(r.ix.childLookup(lang, r.ix.fileScope[file.ID], name), ref, kinds); ok {
		return id, model.ConfExact, true
	}

	// 3. Import-Bindung (folgt auch Re-Exporten).
	if b, ok := r.bindings[file.ID][name]; ok {
		if id, ok := r.resolveExport(b.target, b.imported, kinds, 0); ok {
			return id, model.ConfExact, true
		}
		return "", "", false
	}

	// 4. Eindeutiger Name im Projekt (nur Projektsymbole, keine Fremdpakete).
	return r.uniqueByName(lang, name, ref, kinds)
}

// resolveBinding löst eine Bindung auf das exportierte Symbol auf.
func (r *resolver) resolveBinding(b binding, kinds kindSet) (model.SymbolID, bool) {
	return r.resolveExport(b.target, b.imported, kinds, 0)
}

func hasDefaultModifier(ix *symIndex, id model.SymbolID) bool {
	// Modifikatoren liegen nur im Symbol selbst; die Auflösung über Export-Listen nutzt sie
	// deshalb über den Index. Ohne Eintrag gilt das Symbol nicht als Default-Export.
	return ix.defaults[id]
}

func resolveModule(ix *symIndex, fromPath, spec string, tsc *tsConfig) model.FileID {
	var cands []string
	switch {
	case strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || spec == "." || spec == "..":
		cands = []string{path.Clean(path.Join(path.Dir(fromPath), spec))}
	case tsc != nil:
		cands = tsc.candidates(spec)
	default:
		return 0
	}
	for _, c := range cands {
		if id := tryFile(ix, c); id != 0 {
			return id
		}
	}
	return 0
}

var moduleExts = []string{".ts", ".tsx", ".d.ts", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}

// tryFile prüft Pfad, Pfad mit Endungen und Verzeichnis mit index-Datei.
func tryFile(ix *symIndex, cand string) model.FileID {
	if id, ok := ix.filesByPath[cand]; ok {
		return id
	}
	for _, js := range []string{".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(cand, js) {
			base := strings.TrimSuffix(cand, js)
			for _, ext := range []string{".ts", ".tsx", ".mts", ".cts"} {
				if id, ok := ix.filesByPath[base+ext]; ok {
					return id
				}
			}
		}
	}
	for _, ext := range moduleExts {
		if id, ok := ix.filesByPath[cand+ext]; ok {
			return id
		}
	}
	for _, ext := range moduleExts {
		if id, ok := ix.filesByPath[path.Join(cand, "index"+ext)]; ok {
			return id
		}
	}
	return 0
}

// qualifiedLookup sucht einen qualifizierten Namen. Kandidaten entstehen aus den Präfixen der FQN jedes
// umgebenden Symbols, vom längsten zum kürzesten. Bei Generics wird die Arity als `N angehängt.
func (r *resolver) qualifiedLookup(name string, ref model.Reference, kinds kindSet, lang model.Language, enc model.SymbolID, file model.FileID) (model.SymbolID, model.Confidence, bool) {
	candidates := func(fqn string) (model.SymbolID, bool) {
		if id, ok := r.choose(r.ix.fqnLookup(lang, fqn), ref, kinds); ok {
			return id, true
		}
		if ref.Arity > 0 {
			if id, ok := r.choose(r.ix.fqnLookup(lang, fqn+"`"+strconv.Itoa(ref.Arity)), ref, kinds); ok {
				return id, true
			}
		}
		return "", false
	}
	var hit model.SymbolID
	found := false
	r.scopeChain(enc, func(c model.SymbolID) bool {
		base := r.ix.fqnOf[c]
		for {
			if id, ok := candidates(joinFQN(base, name)); ok {
				hit, found = id, true
				return false
			}
			i := strings.LastIndexByte(base, '.')
			if i < 0 {
				break
			}
			base = base[:i]
		}
		return true
	})
	if found {
		return hit, model.ConfExact, true
	}
	// Über die using-Direktiven der Datei: Models.Order mit "using Shop;" ergibt Shop.Models.Order.
	// Trifft genau ein Namensraum, ist das Ergebnis eindeutig. Treffen mehrere zu, meldet der Compiler
	// CS0104. Dann wird nichts geraten.
	var viaUsing []model.SymbolID
	seen := map[model.SymbolID]bool{}
	first, rest, _ := strings.Cut(name, ".")
	for _, u := range r.usings[file] {
		if u.static {
			continue
		}
		if u.alias != "" {
			// Alias-Teilname: using X = Shop; und dann X.Models.Order
			if fold(lang, u.alias) == fold(lang, first) && rest != "" {
				if id, ok := candidates(joinFQN(u.target, rest)); ok && !seen[id] {
					seen[id] = true
					viaUsing = append(viaUsing, id)
				}
			}
			continue
		}
		id, ok := candidates(joinFQN(strings.TrimSuffix(u.prefix, "."), name))
		if ok && !seen[id] {
			seen[id] = true
			viaUsing = append(viaUsing, id)
		}
	}
	if len(viaUsing) == 1 {
		return viaUsing[0], model.ConfResolved, true
	}
	if len(viaUsing) > 1 {
		return "", "", false
	}
	if id, ok := candidates(name); ok {
		return id, model.ConfExact, true
	}
	return "", "", false
}

func joinFQN(base, name string) string {
	if base == "" {
		return name
	}
	return base + "." + name
}

// linkEndpoints verbindet HTTP-Aufrufe im Frontend mit den Routen im Backend. Ein Aufruf zählt nur, wenn
// genau eine Route passt: gleiches Verb (oder ANY), gleiche Anzahl Segmente, und jedes Segment gleich oder
// ein Parameter auf einer der beiden Seiten. Mehrere Treffer werden nicht geraten.
func linkEndpoints(refs []model.Reference) []model.Relation {
	var routes, calls []model.Reference
	for _, ref := range refs {
		switch ref.Kind {
		case model.RefHTTPRoute:
			routes = append(routes, ref)
		case model.RefHTTPCall:
			calls = append(calls, ref)
		}
	}
	// Konventionsrouten (MapControllerRoute) werden aus Muster und Aktionen zusammengesetzt.
	routes = append(routes, model.ExpandConventionRoutes(refs)...)
	var out []model.Relation
	seen := map[relKey]bool{}
	for _, c := range calls {
		if c.Enclosing == "" {
			continue
		}
		var matches []model.Reference
		for _, rt := range routes {
			if routeMatches(rt.TargetName, c.TargetName) && rt.Enclosing != "" {
				matches = append(matches, rt)
			}
		}
		if len(matches) != 1 {
			continue
		}
		k := relKey{c.Enclosing, matches[0].Enclosing, model.RelCallsEndpoint}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, model.Relation{FileID: c.FileID, From: c.Enclosing, To: matches[0].Enclosing,
			Kind: model.RelCallsEndpoint, Derived: true})
	}
	return out
}

// routeMatches vergleicht "VERB pfad" aus Route und Aufruf segmentweise.
func routeMatches(route, call string) bool {
	rv, rp, _ := strings.Cut(route, " ")
	cv, cp, _ := strings.Cut(call, " ")
	if rv != "ANY" && cv != "ANY" && rv != cv {
		return false
	}
	rs := strings.Split(rp, "/")
	cs := strings.Split(cp, "/")
	if rp == "" || cp == "" {
		return rp == cp
	}
	if len(rs) != len(cs) {
		return false
	}
	for i := range rs {
		if rs[i] == cs[i] || rs[i] == "{}" || cs[i] == "{}" {
			continue
		}
		return false
	}
	return true
}
