// Package resolve bildet die Namen der Verwendungen auf vollqualifizierte Symbol-IDs ab.
//
// Ohne Compiler gibt es keine vollständige Namensbindung. Der Resolver arbeitet
// deshalb mit festen Regeln und markiert jedes Ergebnis mit einer Konfidenz:
//
//	EXACT      lexikalisch eindeutig (Scope-Kette, Datei, expliziter Import)
//	RESOLVED   über Namensraum-Direktiven (C# using) oder Kontext ermittelt
//	HEURISTIC  nur über einen eindeutigen Namen im ganzen Projekt vermutet
//
// Mehrdeutige Fälle bleiben unaufgelöst. Ein falsches Ziel ist schlimmer als keines.
//
// Performance: Alle Nachschlagewerke sind Hash-Maps mit Schlüsseln, die ohne
// Stringverkettung auskommen (Präfix plus Name als Struktur). Symbolarten sind
// Bitmasken, Parameterzahlen und Typparameter werden einmal berechnet.
package resolve

import (
	"context"
	"strings"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

// kindSet ist eine Bitmaske über Symbolarten. 0 bedeutet: jede Art ist zulässig.
type kindSet uint32

const (
	bitNamespace kindSet = 1 << iota
	bitModule
	bitClass
	bitStruct
	bitInterface
	bitRecord
	bitEnum
	bitTypeAlias
	bitMethod
	bitConstructor
	bitFunction
	bitProperty
	bitField
	bitEvent
	bitIndexer
	bitVariable
)

// kindBit liefert das Bit einer Symbolart (0 für unbekannte Arten).
func kindBit(k model.SymbolKind) kindSet {
	switch k {
	case model.KindNamespace:
		return bitNamespace
	case model.KindModule:
		return bitModule
	case model.KindClass:
		return bitClass
	case model.KindStruct:
		return bitStruct
	case model.KindInterface:
		return bitInterface
	case model.KindRecord:
		return bitRecord
	case model.KindEnum:
		return bitEnum
	case model.KindTypeAlias:
		return bitTypeAlias
	case model.KindMethod:
		return bitMethod
	case model.KindConstructor:
		return bitConstructor
	case model.KindFunction:
		return bitFunction
	case model.KindProperty:
		return bitProperty
	case model.KindField:
		return bitField
	case model.KindEvent:
		return bitEvent
	case model.KindIndexer:
		return bitIndexer
	case model.KindVariable:
		return bitVariable
	}
	return 0
}

// childKey ist der Schlüssel für "Name in Scope": Präfix ist der FQN des Containers mit Trenner.
// Die Sprache ist Teil des Schlüssels, damit Namen verschiedener Sprachen sich nie treffen.
type childKey struct {
	lang   model.Language
	prefix string
	name   string
}

// nameKey ist der Schlüssel für die Suche nach einem Namen im ganzen Projekt.
type nameKey struct {
	lang model.Language
	name string
}

// fqnKey ist der Schlüssel für den vollqualifizierten Namen.
type fqnKey struct {
	lang model.Language
	fqn  string
}

// family fasst Sprachen zusammen, die dieselben Namen teilen. C# und VB.NET kompilieren auf dieselben
// .NET-Typen: ein Typ aus C# ist aus VB sichtbar und umgekehrt. TypeScript und JavaScript bilden eine
// eigene Familie mit Modulpfaden.
// caseSensitive meldet, ob Namen in dieser Sprache exakt geschrieben werden müssen (C# und die Sprachen mit C#-Code).
func caseSensitive(lang model.Language) bool {
	return lang == model.LangCSharp || lang == model.LangRazor || lang == model.LangASPX
}

func family(lang model.Language) model.Language {
	if lang == model.LangCSharp || lang == model.LangVBNet || lang == model.LangRazor || lang == model.LangASPX {
		return "dotnet"
	}
	return lang
}

// fold bringt einen Namen in die Form, in der Schlüssel abgelegt werden. .NET-Namen werden
// kleingeschrieben, weil VB nicht case-sensitiv ist. Für C# prüft der Resolver die Schreibweise
// danach exakt (siehe exactName). TypeScript und JavaScript bleiben unverändert.
func fold(lang model.Language, s string) string {
	if family(lang) == "dotnet" {
		return strings.ToLower(s)
	}
	return s
}

// symIndex hält den Symbolbestand als Nachschlagewerke. Das wächst mit der Projektgröße,
// nicht mit der Anzahl der Verwendungen.
type symIndex struct {
	kindOf      map[model.SymbolID]model.SymbolKind
	paramsOf    map[model.SymbolID]int // Parameterzahl (-1 ohne Liste)
	typeArityOf map[model.SymbolID]int // Typparameter aus dem FQN
	containerOf map[model.SymbolID]model.SymbolID
	childPrefix map[model.SymbolID]string // FQN + "." für Container
	fqnOf       map[model.SymbolID]string
	byFQN       map[fqnKey][]model.SymbolID // FQN (für cref und Alias-Ziele)
	byChild     map[childKey][]model.SymbolID
	byName      map[nameKey][]model.SymbolID
	fileSyms    map[model.FileID][]model.SymbolID
	fileScope   map[model.FileID]string // Pfad + "#": Präfix der Datei-Ebene
	files       map[model.FileID]model.File
	filesByPath map[string]model.FileID
	uniqueMemo  map[uniqueKey]uniqueResult
	defaults    map[model.SymbolID]bool // Symbole mit Modifikator "default"
}

type uniqueKey struct {
	name  string
	kind  model.RefKind
	arity int
	lang  model.Language
}

type uniqueResult struct {
	id model.SymbolID
	ok bool
}

// splitFQN trennt einen FQN in Präfix (bis einschließlich des Trenners) und Namen.
// Trenner ist "#" bei Dateipfaden (TS/JS) und "." sonst. Ein Punkt im Dateipfad zählt nicht.
// Generic-Suffixe (`Name`2`) werden aus dem Namen entfernt, damit Verwendungen ohne Arity treffen.
func splitFQN(fqn string) (string, string) {
	hash := strings.IndexByte(fqn, '#')
	dot := strings.LastIndexByte(fqn, '.')
	var cut int
	switch {
	case dot > hash:
		cut = dot
	case hash >= 0:
		cut = hash
	default:
		return "", stripArity(fqn)
	}
	return fqn[:cut+1], stripArity(fqn[cut+1:])
}

func stripArity(name string) string {
	if i := strings.IndexByte(name, '`'); i >= 0 {
		return name[:i]
	}
	return name
}

func buildIndex(ctx context.Context, st store.Store) (*symIndex, error) {
	syms, err := st.AllSymbols(ctx)
	if err != nil {
		return nil, err
	}
	locs, err := st.AllLocations(ctx)
	if err != nil {
		return nil, err
	}
	files, err := st.Files(ctx)
	if err != nil {
		return nil, err
	}
	n := len(syms)
	ix := &symIndex{
		kindOf:      make(map[model.SymbolID]model.SymbolKind, n),
		paramsOf:    make(map[model.SymbolID]int, n),
		typeArityOf: make(map[model.SymbolID]int, n),
		containerOf: make(map[model.SymbolID]model.SymbolID, n),
		childPrefix: make(map[model.SymbolID]string, n/4),
		fqnOf:       make(map[model.SymbolID]string, n),
		byFQN:       make(map[fqnKey][]model.SymbolID, n),
		byChild:     make(map[childKey][]model.SymbolID, n),
		byName:      make(map[nameKey][]model.SymbolID, n),
		fileSyms:    map[model.FileID][]model.SymbolID{},
		fileScope:   map[model.FileID]string{},
		files:       make(map[model.FileID]model.File, len(files)),
		filesByPath: make(map[string]model.FileID, len(files)),
		uniqueMemo:  map[uniqueKey]uniqueResult{},
		defaults:    map[model.SymbolID]bool{},
	}
	for _, s := range syms {
		ix.kindOf[s.ID] = s.Kind
		ix.paramsOf[s.ID] = paramCount(s.ID)
		ix.typeArityOf[s.ID] = typeArity(s.ID)
		ix.fqnOf[s.ID] = s.FQN
		for _, m := range s.Modifiers {
			if m == "default" {
				ix.defaults[s.ID] = true
			}
		}
		if s.Container != "" {
			ix.containerOf[s.ID] = s.Container
		}
		lang := family(s.ID.Lang())
		if s.FQN != "" {
			fk := fqnKey{lang, fold(lang, s.FQN)}
			ix.byFQN[fk] = append(ix.byFQN[fk], s.ID)
			prefix, name := splitFQN(s.FQN)
			ck := childKey{lang, fold(lang, prefix), fold(lang, name)}
			ix.byChild[ck] = append(ix.byChild[ck], s.ID)
			ix.childPrefix[s.ID] = fold(lang, s.FQN+".")
		}
		if s.Name != "" {
			nk := nameKey{lang, fold(lang, s.Name)}
			ix.byName[nk] = append(ix.byName[nk], s.ID)
		}
	}
	type symFile struct {
		sym  model.SymbolID
		file model.FileID
	}
	seen := make(map[symFile]bool, len(locs))
	for _, l := range locs {
		k := symFile{l.SymbolID, l.FileID}
		if seen[k] {
			continue
		}
		seen[k] = true
		ix.fileSyms[l.FileID] = append(ix.fileSyms[l.FileID], l.SymbolID)
	}
	for _, f := range files {
		ix.files[f.ID] = f
		ix.filesByPath[f.Path] = f.ID
		ix.fileScope[f.ID] = f.Path + "#"
	}
	return ix, nil
}

// childLookup liefert alle Symbole mit diesem Namen im Scope prefix (in der Sprache lang).
func (ix *symIndex) childLookup(lang model.Language, prefix, name string) []model.SymbolID {
	fam := family(lang)
	return ix.byChild[childKey{fam, fold(fam, prefix), fold(fam, name)}]
}

// fqnLookup liefert die Symbole mit genau diesem vollqualifizierten Namen.
func (ix *symIndex) fqnLookup(lang model.Language, fqn string) []model.SymbolID {
	fam := family(lang)
	return ix.byFQN[fqnKey{fam, fold(fam, fqn)}]
}

// nameLookup liefert alle Symbole mit diesem Namen im Projekt.
func (ix *symIndex) nameLookup(lang model.Language, name string) []model.SymbolID {
	fam := family(lang)
	return ix.byName[nameKey{fam, fold(fam, name)}]
}

// paramCount liefert die Anzahl der Parameter aus einer Symbol-ID. -1, wenn die ID keine
// Parameterliste hat (Typen, Felder, Properties).
func paramCount(id model.SymbolID) int {
	s := string(id)
	open := strings.IndexByte(s, '(')
	if open < 0 || !strings.HasSuffix(s, ")") {
		return -1
	}
	inner := s[open+1 : len(s)-1]
	if inner == "" {
		return 0
	}
	depth := 0
	n := 1
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '<', '(', '[':
			depth++
		case '>', ')', ']':
			depth--
		case ',':
			if depth == 0 {
				n++
			}
		}
	}
	return n
}
