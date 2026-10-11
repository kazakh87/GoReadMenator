// Package store definiert die Schnittstelle zur Persistenz des Symbolindex.
//
// Der restliche Code spricht ausschließlich über das Interface Store.
// SQL-Statements stehen nur im Paket store/sqlite. Ein Backend wird über
// Register angemeldet und über Open ausgewählt. Dadurch lässt sich die
// Datenbank später austauschen, ohne Parser, Resolver oder CLI anzufassen.
package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"goreadmenator/internal/model"
)

// ErrNotFound wird geliefert, wenn ein angefragter Datensatz nicht existiert.
var ErrNotFound = errors.New("store: nicht gefunden")

// Batch bündelt alle Ergebnisse einer Datei oder eines Parser-Laufs.
// Er wird in einer einzigen Transaktion geschrieben. Ein Store muss die Datensätze
// beim Schreiben kopieren: der Aufrufer verwendet die Slices danach wieder.
type Batch struct {
	Symbols    []model.Symbol
	Locations  []model.Location
	References []model.Reference
	Relations  []model.Relation
	Imports    []model.Import
}

// Store ist die API zur Persistenz. Implementierungen müssen für parallele
// Lesezugriffe sicher sein. Schreibzugriffe laufen über einen einzigen Writer.
//
// Merge-Regel für Symbole: Ein bereits vorhandenes Symbol (gleiche ID, etwa
// bei partial) wird nicht überschrieben. Leere Felder werden aus dem neuen
// Datensatz ergänzt.
type Store interface {
	// Meta liest einen Metaschlüssel. ok ist false, wenn der Schlüssel fehlt.
	Meta(ctx context.Context, key string) (value string, ok bool, err error)
	// SetMeta setzt einen Metaschlüssel.
	SetMeta(ctx context.Context, key, value string) error

	// PutFile legt eine Datei an oder aktualisiert sie (Schlüssel: Pfad).
	// Die Datei-ID wird vergeben, wenn f.ID == 0 ist.
	PutFile(ctx context.Context, f model.File) (model.FileID, error)
	// FileByPath liefert eine Datei über ihren Pfad.
	FileByPath(ctx context.Context, path string) (model.File, bool, error)
	// Files liefert alle indizierten Dateien.
	Files(ctx context.Context) ([]model.File, error)
	// DeleteFile entfernt alle Daten einer Datei. Symbole ohne verbleibende
	// Fundorte werden mit ihren Beziehungen entfernt.
	DeleteFile(ctx context.Context, id model.FileID) error

	// WriteBatch schreibt einen Batch atomar.
	WriteBatch(ctx context.Context, b Batch) error

	// Symbol liefert ein Symbol über seine ID.
	Symbol(ctx context.Context, id model.SymbolID) (model.Symbol, bool, error)
	// SymbolsByName liefert alle Symbole mit diesem Namen (ohne Namensraum).
	SymbolsByName(ctx context.Context, name string) ([]model.Symbol, error)
	// Locations liefert alle Fundorte eines Symbols.
	Locations(ctx context.Context, id model.SymbolID) ([]model.Location, error)
	// ReferencesTo liefert alle aufgelösten Verwendungen eines Symbols.
	ReferencesTo(ctx context.Context, id model.SymbolID) ([]model.Reference, error)
	// ReferencesByName liefert Verwendungen über den Rohnamen. Wird für die
	// Neuauflösung nach inkrementellen Updates benötigt.
	ReferencesByName(ctx context.Context, name string) ([]model.Reference, error)
	// Relations liefert alle Beziehungen, die von oder zu einem Symbol führen.
	Relations(ctx context.Context, id model.SymbolID) ([]model.Relation, error)

	// AllSymbols, AllLocations, AllReferences, AllImports liefern den gesamten Bestand.
	// Sie dienen dem Resolver, der nicht pro Symbol abfragt.
	AllSymbols(ctx context.Context) ([]model.Symbol, error)
	AllLocations(ctx context.Context) ([]model.Location, error)
	AllReferences(ctx context.Context) ([]model.Reference, error)
	AllImports(ctx context.Context) ([]model.Import, error)

	// UpdateReferences setzt Ziel und Konfidenz der Verwendungen (Schlüssel: ID).
	UpdateReferences(ctx context.Context, refs []model.Reference) error
	// UpdateImports setzt die aufgelöste Datei der Imports (Schlüssel: ID).
	UpdateImports(ctx context.Context, imps []model.Import) error
	// SetDerivedRelations ersetzt alle vom Resolver abgeleiteten Beziehungen.
	// Geparste Beziehungen (Derived == false) bleiben unberührt.
	SetDerivedRelations(ctx context.Context, rels []model.Relation) error
	// AllRelations liefert alle Beziehungen, geparste und abgeleitete.
	AllRelations(ctx context.Context) ([]model.Relation, error)

	// Close gibt Ressourcen frei.
	Close() error
}

// Opener erstellt eine Store-Implementierung für einen Pfad.
type Opener func(ctx context.Context, path string) (Store, error)

var (
	regMu    sync.RWMutex
	registry = map[string]Opener{}
)

// Register meldet ein Backend unter einem Namen an. Wird von den Backend-Paketen
// in init() aufgerufen. Doppelte Namen sind ein Programmierfehler und führen zu panic.
func Register(name string, open Opener) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("store: backend %q doppelt registriert", name))
	}
	registry[name] = open
}

// Open öffnet das Backend mit dem gegebenen Namen.
func Open(ctx context.Context, backend, path string) (Store, error) {
	regMu.RLock()
	open, ok := registry[backend]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("store: unbekanntes Backend %q (verfügbar: %v)", backend, Backends())
	}
	return open(ctx, path)
}

// Backends liefert die Namen aller registrierten Backends, sortiert.
func Backends() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
