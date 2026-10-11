package httpapi

import "net/http"

// openAPI liefert die Beschreibung der Schnittstelle. Sie ist bewusst knapp gehalten, weil sie
// von einem Sprachmodell gelesen wird und dabei Kontext verbraucht.
func (s *Server) openAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, spec)
}

var budgetParam = map[string]any{"name": "budget", "in": "query", "required": false,
	"description": "Token-Budget der Antwort (Standard 2500, 200 bis 8000)", "schema": map[string]any{"type": "integer"}}
var cursorParam = map[string]any{"name": "cursor", "in": "query", "required": false,
	"description": "nextCursor aus der vorherigen Antwort", "schema": map[string]any{"type": "string"}}
var projectParam = map[string]any{"name": "project", "in": "path", "required": true,
	"description": "Projektname, siehe /v1/projects", "schema": map[string]any{"type": "string"}}

func op(summary string, params ...map[string]any) map[string]any {
	return map[string]any{"get": map[string]any{"summary": summary, "parameters": append([]map[string]any{projectParam}, params...),
		"responses": map[string]any{"200": map[string]any{"description": "JSON"}, "404": map[string]any{"description": "Projekt oder Symbol nicht gefunden"},
			"409": map[string]any{"description": "Name ist mehrdeutig, Kandidaten in der Meldung"}}}}
}

var refParam = map[string]any{"name": "ref", "in": "query", "required": true,
	"description": "Symbol-ID, Name oder FQN", "schema": map[string]any{"type": "string"}}

var spec = map[string]any{
	"openapi": "3.0.3",
	"info":    map[string]any{"title": "goreadmenator", "version": "0.1.0", "description": "Lesender Zugriff auf den Symbolgraphen. Antworten sind auf ein Token-Budget begrenzt. Ergebnisse mit nextCursor lassen sich seitenweise abrufen."},
	"paths": map[string]any{
		"/v1/projects": map[string]any{"get": map[string]any{"summary": "Projekte auflisten"}},
		"/v1/{project}/symbols": op("Symbole suchen", map[string]any{"name": "q", "in": "query", "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "kind", "in": "query", "schema": map[string]any{"type": "string"}}, cursorParam, budgetParam),
		"/v1/{project}/symbol":       op("Details eines Symbols ohne Quelltext", refParam, budgetParam),
		"/v1/{project}/api":          op("Öffentliche API unter einem Namensbereich", map[string]any{"name": "scope", "in": "query", "schema": map[string]any{"type": "string"}}, cursorParam, budgetParam),
		"/v1/{project}/neighborhood": op("Beziehungen bis Tiefe 3", refParam, map[string]any{"name": "depth", "in": "query", "schema": map[string]any{"type": "integer"}}, map[string]any{"name": "kinds", "in": "query", "description": "Kommagetrennte Beziehungsarten", "schema": map[string]any{"type": "string"}}, cursorParam, budgetParam),
		"/v1/{project}/usages":       op("Verwendungen bis Tiefe 3", refParam, map[string]any{"name": "depth", "in": "query", "schema": map[string]any{"type": "integer"}}, cursorParam, budgetParam),
		"/v1/{project}/context":      op("Kontextpaket: Symbol, Beziehungen, Verwendungen im Budget", refParam, budgetParam),
		"/v1/{project}/source": op("Quelltext-Ausschnitt aus dem HEAD, höchstens 80 Zeilen",
			map[string]any{"name": "file", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "start", "in": "query", "required": true, "schema": map[string]any{"type": "integer"}},
			map[string]any{"name": "end", "in": "query", "required": true, "schema": map[string]any{"type": "integer"}}),
		"/v1/{project}/changes":   op("Geänderte Symbole seit einem Commit", map[string]any{"name": "from", "in": "query", "required": true, "schema": map[string]any{"type": "string"}}, cursorParam, budgetParam),
		"/v1/{project}/status":    op("Zustand des Index"),
		"/v1/{project}/endpoints": op("HTTP-Routen des Backends mit Aufrufern aus dem Frontend", cursorParam, budgetParam),
	},
}
