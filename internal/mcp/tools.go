package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"goreadmenator/internal/query"
)

// Tools liefert die Werkzeugliste. Das Werkzeug "projects" behandelt der Server selbst. Die Beschreibungen sind kurz gehalten, weil sie bei jeder
// Sitzung in den Kontext des Assistenten gelangen.
func Tools() []Tool {
	budget := map[string]any{"type": "integer", "description": "Token-Budget der Antwort, Standard 2500"}
	cursor := map[string]any{"type": "string", "description": "nextCursor aus der vorherigen Antwort"}
	ref := map[string]any{"type": "string", "description": "Symbol-ID, Name oder FQN"}
	return []Tool{
		{
			Name:        "find_symbols",
			Description: "Symbole nach Namensteil suchen. Liefert ID, Art, FQN, Datei und Signatur, ohne Quelltext.",
			Schema: schema(map[string]any{
				"q":             map[string]any{"type": "string"},
				"kind":          map[string]any{"type": "string", "description": "z. B. class, method, interface"},
				"limit":         map[string]any{"type": "integer"},
				"cursor":        cursor,
				"budget_tokens": budget,
			}, "q"),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					Q      string `json:"q"`
					Kind   string `json:"kind"`
					Cursor string `json:"cursor"`
					Limit  int    `json:"limit"`
					Budget int    `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.FindSymbols(context.Background(), a.Q, a.Kind, a.Cursor, a.Limit, a.Budget)
			},
		},
		{
			Name:        "get_symbol",
			Description: "Details eines Symbols: Signatur, Doku, Parameter, Rückgabe, Fundorte, Anzahl der Beziehungen. Kein Quelltext.",
			Schema:      schema(map[string]any{"ref": ref, "budget_tokens": budget}, "ref"),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					Ref    string `json:"ref"`
					Budget int    `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.GetSymbol(context.Background(), a.Ref, a.Budget)
			},
		},
		{
			Name:        "api",
			Description: "Öffentliche API unter einem Namensbereich: Art, FQN, Signatur und kurze Doku je Zeile. Paginiert.",
			Schema: schema(map[string]any{
				"scope":         map[string]any{"type": "string", "description": "Präfix der FQN, z. B. MyApp.Services."},
				"cursor":        cursor,
				"budget_tokens": budget,
			}),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					Scope  string `json:"scope"`
					Cursor string `json:"cursor"`
					Budget int    `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.API(context.Background(), a.Scope, a.Cursor, a.Budget)
			},
		},
		{
			Name:        "neighborhood",
			Description: "Beziehungen eines Symbols bis zur Tiefe 1 bis 3 (inherits, implements, has_type, param_type, returns, contains, attribute_of).",
			Schema: schema(map[string]any{
				"ref":            ref,
				"depth":          map[string]any{"type": "integer", "description": "1 bis 3, Standard 1"},
				"relation_kinds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"cursor":         cursor,
				"budget_tokens":  budget,
			}, "ref"),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					Ref   string   `json:"ref"`
					Depth int      `json:"depth"`
					Kinds []string `json:"relation_kinds"`
					Cur   string   `json:"cursor"`
					Bud   int      `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.Neighborhood(context.Background(), a.Ref, a.Depth, a.Kinds, a.Cur, a.Bud)
			},
		},
		{
			Name:        "usages",
			Description: "Verwendungen eines Symbols bis zur Tiefe 1 bis 3, mit Datei, Zeile und umgebendem Symbol. Wirkungskette für Änderungen.",
			Schema: schema(map[string]any{
				"ref":           ref,
				"depth":         map[string]any{"type": "integer", "description": "1 bis 3, Standard 1"},
				"cursor":        cursor,
				"budget_tokens": budget,
			}, "ref"),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					Ref    string `json:"ref"`
					Depth  int    `json:"depth"`
					Cursor string `json:"cursor"`
					Budget int    `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.Usages(context.Background(), a.Ref, a.Depth, a.Cursor, a.Budget)
			},
		},
		{
			Name:        "context",
			Description: "Kompaktes Paket für eine Aufgabe: Symbol, direkte Beziehungen und Verwendungen, bis das Budget erreicht ist. Ideal für Spezifikation und Plan.",
			Schema:      schema(map[string]any{"ref": ref, "budget_tokens": budget}, "ref"),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					Ref    string `json:"ref"`
					Budget int    `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.Context(context.Background(), a.Ref, a.Budget)
			},
		},
		{
			Name:        "source",
			Description: "Quelltext-Ausschnitt aus dem HEAD, höchstens 80 Zeilen. Nur gezielt verwenden, Graphdaten zuerst abfragen.",
			Schema: schema(map[string]any{
				"file":       map[string]any{"type": "string", "description": "Repo-relativer Pfad"},
				"start_line": map[string]any{"type": "integer"},
				"end_line":   map[string]any{"type": "integer"},
			}, "file", "start_line", "end_line"),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					File  string `json:"file"`
					Start int    `json:"start_line"`
					End   int    `json:"end_line"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.Source(context.Background(), a.File, a.Start, a.End)
			},
		},
		{
			Name:        "changes",
			Description: "Geänderte Symbole seit einem Commit bis HEAD, mit Kennzeichen direkt betroffen und Anzahl der Verwendungen.",
			Schema: schema(map[string]any{
				"from":          map[string]any{"type": "string", "description": "Commit, ab dem verglichen wird"},
				"cursor":        cursor,
				"budget_tokens": budget,
			}, "from"),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					From   string `json:"from"`
					Cursor string `json:"cursor"`
					Budget int    `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.Changes(context.Background(), a.From, a.Cursor, a.Budget)
			},
		},
		{
			Name:        "endpoints",
			Description: "HTTP-Routen des Backends mit Aktion, Datei, Zeile und den Frontend-Funktionen, die sie aufrufen.",
			Schema:      schema(map[string]any{"cursor": cursor, "budget_tokens": budget}),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				var a struct {
					Cursor string `json:"cursor"`
					Budget int    `json:"budget_tokens"`
				}
				if err := decode(raw, &a); err != nil {
					return nil, err
				}
				return e.Endpoints(context.Background(), a.Cursor, a.Budget)
			},
		},
		{
			Name:        "status",
			Description: "Zustand des Index: indizierter Commit, HEAD, Anzahl Symbole und Verwendungen.",
			Schema:      schema(map[string]any{}),
			Run: func(e *query.Engine, raw json.RawMessage) (any, error) {
				return e.Status(context.Background())
			},
		},
	}
}

// projectProp ist der optionale Projektname, den jedes Werkzeug akzeptiert.
var projectProp = map[string]any{"type": "string", "description": "Projektname (siehe Werkzeug projects). Leer = Standardprojekt"}

func schema(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	props["project"] = projectProp
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("ungültige Argumente: %w", err)
	}
	return nil
}
