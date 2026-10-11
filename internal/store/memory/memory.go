// Package memory ist ein reines In-Memory-Backend. Es dient als Referenz-
// implementierung der store.Store-API, für Tests und als Übergangslösung,
// solange kein persistentes Backend verfügbar ist. Es ist nicht für große
// Repositories gedacht, da alles im RAM liegt.
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

func init() {
	store.Register("memory", func(ctx context.Context, _ string) (store.Store, error) {
		return New(), nil
	})
}

// Store ist die In-Memory-Implementierung.
type Store struct {
	mu        sync.RWMutex
	meta      map[string]string
	files     map[model.FileID]model.File
	byPath    map[string]model.FileID
	nextFile  model.FileID
	symbols   map[model.SymbolID]model.Symbol
	locations []model.Location
	refs      []model.Reference
	relations []model.Relation
	imports   []model.Import
	nextRef   int64
	nextImp   int64
	derived   []model.Relation
}

// New erstellt einen leeren Store.
func New() *Store {
	return &Store{
		meta:    map[string]string{},
		files:   map[model.FileID]model.File{},
		byPath:  map[string]model.FileID{},
		symbols: map[model.SymbolID]model.Symbol{},
	}
}

var _ store.Store = (*Store)(nil)

func (s *Store) Meta(_ context.Context, key string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.meta[key]
	return v, ok, nil
}

func (s *Store) SetMeta(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta[key] = value
	return nil
}

func (s *Store) PutFile(_ context.Context, f model.File) (model.FileID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byPath[f.Path]; ok {
		f.ID = id
	} else if f.ID == 0 {
		s.nextFile++
		f.ID = s.nextFile
	}
	if f.ID > s.nextFile {
		s.nextFile = f.ID
	}
	s.files[f.ID] = f
	s.byPath[f.Path] = f.ID
	return f.ID, nil
}

func (s *Store) FileByPath(_ context.Context, path string) (model.File, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byPath[path]
	if !ok {
		return model.File{}, false, nil
	}
	return s.files[id], true, nil
}

func (s *Store) Files(_ context.Context) ([]model.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.File, 0, len(s.files))
	for _, f := range s.files {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *Store) DeleteFile(_ context.Context, id model.FileID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	if !ok {
		return nil
	}
	delete(s.files, id)
	delete(s.byPath, f.Path)

	s.locations = filterLoc(s.locations, id)
	s.refs = filterRef(s.refs, id)
	s.relations = filterRel(s.relations, id)
	s.derived = filterRel(s.derived, id)
	s.imports = filterImp(s.imports, id)

	// Symbole ohne verbleibende Fundorte entfernen.
	alive := map[model.SymbolID]bool{}
	for _, l := range s.locations {
		alive[l.SymbolID] = true
	}
	for sid := range s.symbols {
		if !alive[sid] {
			delete(s.symbols, sid)
		}
	}
	return nil
}

func (s *Store) WriteBatch(_ context.Context, b store.Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sym := range b.Symbols {
		if old, ok := s.symbols[sym.ID]; ok {
			s.symbols[sym.ID] = mergeSymbol(old, sym)
		} else {
			s.symbols[sym.ID] = sym
		}
	}
	for _, l := range b.Locations {
		if _, ok := s.symbols[l.SymbolID]; !ok {
			return fmt.Errorf("location für unbekanntes Symbol %q", l.SymbolID)
		}
	}
	s.locations = append(s.locations, b.Locations...)
	for _, r := range b.References {
		s.nextRef++
		r.ID = s.nextRef
		s.refs = append(s.refs, r)
	}
	s.relations = append(s.relations, b.Relations...)
	for _, im := range b.Imports {
		s.nextImp++
		im.ID = s.nextImp
		s.imports = append(s.imports, im)
	}
	return nil
}

func (s *Store) Symbol(_ context.Context, id model.SymbolID) (model.Symbol, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sym, ok := s.symbols[id]
	return sym, ok, nil
}

func (s *Store) SymbolsByName(_ context.Context, name string) ([]model.Symbol, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Symbol
	for _, sym := range s.symbols {
		if sym.Name == name {
			out = append(out, sym)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) Locations(_ context.Context, id model.SymbolID) ([]model.Location, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Location
	for _, l := range s.locations {
		if l.SymbolID == id {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *Store) ReferencesTo(_ context.Context, id model.SymbolID) ([]model.Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Reference
	for _, r := range s.refs {
		if r.TargetID == id {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ReferencesByName(_ context.Context, name string) ([]model.Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Reference
	for _, r := range s.refs {
		if r.TargetName == name {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) Relations(_ context.Context, id model.SymbolID) ([]model.Relation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Relation
	for _, r := range append(append([]model.Relation(nil), s.relations...), s.derived...) {
		if r.From == id || r.To == id {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) SetDerivedRelations(_ context.Context, rels []model.Relation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.derived = make([]model.Relation, 0, len(rels))
	for _, r := range rels {
		r.Derived = true
		s.derived = append(s.derived, r)
	}
	return nil
}

func (s *Store) AllRelations(_ context.Context) ([]model.Relation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]model.Relation(nil), s.relations...)
	return append(out, s.derived...), nil
}

func (s *Store) AllSymbols(_ context.Context) ([]model.Symbol, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Symbol, 0, len(s.symbols))
	for _, sym := range s.symbols {
		out = append(out, sym)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) AllLocations(_ context.Context) ([]model.Location, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Location(nil), s.locations...), nil
}

func (s *Store) AllReferences(_ context.Context) ([]model.Reference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Reference(nil), s.refs...), nil
}

func (s *Store) AllImports(_ context.Context) ([]model.Import, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Import(nil), s.imports...), nil
}

func (s *Store) UpdateReferences(_ context.Context, refs []model.Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := make(map[int64]model.Reference, len(refs))
	for _, r := range refs {
		byID[r.ID] = r
	}
	for i := range s.refs {
		if u, ok := byID[s.refs[i].ID]; ok {
			s.refs[i].TargetID = u.TargetID
			s.refs[i].Confidence = u.Confidence
		}
	}
	return nil
}

func (s *Store) UpdateImports(_ context.Context, imps []model.Import) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := make(map[int64]model.Import, len(imps))
	for _, im := range imps {
		byID[im.ID] = im
	}
	for i := range s.imports {
		if u, ok := byID[s.imports[i].ID]; ok {
			s.imports[i].ResolvedID = u.ResolvedID
		}
	}
	return nil
}

func (s *Store) Close() error { return nil }

// mergeSymbol übernimmt Felder aus neu nur dort, wo old leer ist.
func mergeSymbol(old, neu model.Symbol) model.Symbol {
	if old.Kind == "" {
		old.Kind = neu.Kind
	}
	if old.Name == "" {
		old.Name = neu.Name
	}
	if old.FQN == "" {
		old.FQN = neu.FQN
	}
	if old.Container == "" {
		old.Container = neu.Container
	}
	if old.Signature == "" {
		old.Signature = neu.Signature
	}
	if len(old.Modifiers) == 0 {
		old.Modifiers = neu.Modifiers
	}
	if old.Summary == "" {
		old.Summary = neu.Summary
	}
	if old.Remarks == "" {
		old.Remarks = neu.Remarks
	}
	if old.Returns == "" {
		old.Returns = neu.Returns
	}
	if len(old.ParamDocs) == 0 {
		old.ParamDocs = neu.ParamDocs
	}
	if old.DocSource == "" {
		old.DocSource = neu.DocSource
	}
	return old
}

func filterLoc(in []model.Location, id model.FileID) []model.Location {
	out := in[:0]
	for _, x := range in {
		if x.FileID != id {
			out = append(out, x)
		}
	}
	return out
}

func filterRef(in []model.Reference, id model.FileID) []model.Reference {
	out := in[:0]
	for _, x := range in {
		if x.FileID != id {
			out = append(out, x)
		}
	}
	return out
}

func filterRel(in []model.Relation, id model.FileID) []model.Relation {
	out := in[:0]
	for _, x := range in {
		if x.FileID != id {
			out = append(out, x)
		}
	}
	return out
}

func filterImp(in []model.Import, id model.FileID) []model.Import {
	out := in[:0]
	for _, x := range in {
		if x.FileID != id {
			out = append(out, x)
		}
	}
	return out
}
