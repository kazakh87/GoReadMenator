// Package httpapi stellt die Abfrageschicht als HTTP-JSON-Schnittstelle bereit. Die Schnittstelle
// ist nur lesend. Jeder Endpunkt entspricht einem Werkzeug des MCP-Servers und liefert dieselben
// begrenzten Antworten. Die Beschreibung im OpenAPI-Format liegt unter /v1/openapi.json, damit sich
// auch andere Sprachmodelle und Clients ohne Vorwissen anbinden können.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"goreadmenator/internal/query"
)

// Resolver liefert die Abfrageschicht für ein Projekt (wie beim MCP-Server).
type Resolver func(project string) (*query.Engine, error)

// Server ist der HTTP-Handler. Anfragen werden nacheinander bearbeitet, weil die Abfrageschicht
// Zwischenstände hält und nicht nebenläufig ausgelegt ist. Für ein lokales Werkzeug ist das ausreichend.
type Server struct {
	resolve  Resolver
	projects func() []string
	token    string
	mu       sync.Mutex
	mux      *http.ServeMux
}

// New erstellt den Server. Ist token nicht leer, verlangt jede Anfrage den Header
// "Authorization: Bearer <token>".
func New(resolve Resolver, projects func() []string, token string) *Server {
	s := &Server{resolve: resolve, projects: projects, token: token, mux: http.NewServeMux()}
	s.mux.HandleFunc("/v1/openapi.json", s.openAPI)
	s.mux.HandleFunc("/v1/projects", s.listProjects)
	s.mux.HandleFunc("/v1/", s.project)
	return s
}

// ServeHTTP prüft Authentisierung und Methode und leitet an den Endpunkt weiter.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
		writeError(w, http.StatusUnauthorized, "Token fehlt oder ist falsch")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "nur GET ist erlaubt, die Schnittstelle ist schreibgeschützt")
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"projects": s.projects()})
}

// project behandelt /v1/{projekt}/{werkzeug}.
func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/"), "/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "Pfad ist /v1/{projekt}/{werkzeug}")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	eng, err := s.resolve(parts[0])
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	q := r.URL.Query()
	ctx := r.Context()
	budget := intParam(q.Get("budget"))
	var result any
	switch parts[1] {
	case "symbols":
		result, err = eng.FindSymbols(ctx, q.Get("q"), q.Get("kind"), q.Get("cursor"), intParam(q.Get("limit")), budget)
	case "symbol":
		result, err = eng.GetSymbol(ctx, q.Get("ref"), budget)
	case "api":
		result, err = eng.API(ctx, q.Get("scope"), q.Get("cursor"), budget)
	case "neighborhood":
		depth := intParam(q.Get("depth"))
		result, err = eng.Neighborhood(ctx, q.Get("ref"), depth, splitList(q.Get("kinds")), q.Get("cursor"), budget)
	case "usages":
		result, err = eng.Usages(ctx, q.Get("ref"), intParam(q.Get("depth")), q.Get("cursor"), budget)
	case "context":
		result, err = eng.Context(ctx, q.Get("ref"), budget)
	case "source":
		result, err = eng.Source(ctx, q.Get("file"), intParam(q.Get("start")), intParam(q.Get("end")))
	case "changes":
		result, err = eng.Changes(ctx, q.Get("from"), q.Get("cursor"), budget)
	case "endpoints":
		result, err = eng.Endpoints(ctx, q.Get("cursor"), budget)
	case "status":
		result, err = eng.Status(ctx)
	default:
		writeError(w, http.StatusNotFound, "unbekannter Endpunkt "+parts[1])
		return
	}
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// statusFor ordnet Fehler der Abfrageschicht HTTP-Statuscodes zu.
func statusFor(err error) int {
	switch {
	case errors.Is(err, query.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, query.ErrAmbiguous):
		return http.StatusConflict
	}
	if strings.Contains(err.Error(), "ist nicht geladen") {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

func intParam(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

var _ = context.Background
