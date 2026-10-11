// Package parser definiert die Schnittstelle für Sprach-Parser und eine
// Registry, die Sprachen auf Parser abbildet.
//
// Ein Parser ist zustandslos oder hält nur unveränderliche Daten (etwa
// kompilierte Regexe). Er wird von mehreren Goroutinen gleichzeitig benutzt
// und muss deshalb nebenläufig sicher sein.
package parser

import (
	"context"
	"fmt"

	"goreadmenator/internal/model"
)

// Source ist die Eingabe eines Parsers: eine Datei mit ihrem Inhalt.
type Source struct {
	Path    string
	BlobSHA string
	Content []byte
}

// Severity gibt an, wie ernst eine Diagnose ist. Der Nullwert ist ein Fehler, damit bestehende Diagnosen gleich bleiben.
type Severity uint8

const (
	// SeverityError meldet Stellen, die nicht verstanden wurden.
	SeverityError Severity = iota
	// SeverityWarning meldet Konstrukte, die der Parser nur teilweise oder gar nicht erfasst.
	SeverityWarning
)

// Diagnostic ist eine nicht fatale Auffälligkeit beim Parsen (z. B. unbekannte Syntax).
type Diagnostic struct {
	Span     model.Span
	Message  string
	Severity Severity
}

// Coverage beschreibt, welche Sprachversion ein Parser erfasst. MaxVersion ist leer, wenn der Parser keine
// Versionsangabe macht. Dann wird die Sprachversion eines Projekts nicht gegen ihn geprüft.
type Coverage struct {
	Language   model.Language
	MaxVersion string // z. B. "13" für C# 13
}

// Versioned wird von Parsern implementiert, die ihre Sprachabdeckung melden.
type Versioned interface {
	Coverage() Coverage
}

// Result ist das Ergebnis eines Parse-Laufs über eine Datei.
// Datei-IDs fehlen hier absichtlich. Sie vergibt die Pipeline beim Schreiben.
type Result struct {
	FileDoc     string
	LOC         int
	Symbols     []model.Symbol
	Locations   []model.Location
	References  []model.Reference
	Relations   []model.Relation
	Imports     []model.Import
	Diagnostics []Diagnostic
}

// Parser analysiert eine Datei einer bestimmten Sprache.
type Parser interface {
	// Language liefert die Sprache, für die der Parser zuständig ist.
	Language() model.Language
	// Parse analysiert eine Datei. Der Kontext erlaubt den Abbruch bei langen Dateien.
	Parse(ctx context.Context, src Source) (Result, error)
}

// Registry ordnet Sprachen ihren Parsern zu.
type Registry struct {
	parsers map[model.Language]Parser
}

// NewRegistry erstellt eine Registry. Doppelte Sprachen sind ein Fehler.
func NewRegistry(ps ...Parser) (*Registry, error) {
	r := &Registry{parsers: map[model.Language]Parser{}}
	for _, p := range ps {
		if err := r.Add(p); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Add registriert einen Parser.
func (r *Registry) Add(p Parser) error {
	if _, dup := r.parsers[p.Language()]; dup {
		return fmt.Errorf("parser: Sprache %q doppelt registriert", p.Language())
	}
	r.parsers[p.Language()] = p
	return nil
}

// For liefert den Parser für eine Sprache.
func (r *Registry) For(lang model.Language) (Parser, bool) {
	p, ok := r.parsers[lang]
	return p, ok
}
