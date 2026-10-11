// Package mcp stellt die Abfrageschicht als MCP-Server bereit (Model Context Protocol, stdio-Transport).
// Der Server ist eine dünne Hülle über internal/query: Jedes Werkzeug liefert nur den angefragten
// Ausschnitt, begrenzt auf ein Token-Budget. Die Implementierung braucht nur die Standardbibliothek.
package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"

	"goreadmenator/internal/query"
)

const protocolVersion = "2024-11-05"

// Tool beschreibt ein Werkzeug für tools/list und seine Ausführung.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Run         func(e *query.Engine, args json.RawMessage) (any, error)
}

// Server verarbeitet JSON-RPC-Nachrichten zeilenweise.
type Server struct {
	in     io.Reader
	out    io.Writer
	tools  []Tool
	byName map[string]Tool
	// Resolve liefert die Abfrageschicht für ein Projekt. Leerer Name = Standardprojekt.
	// Die Funktion lädt den Index bei Bedarf neu.
	Resolve func(project string) (*query.Engine, error)
	// Projects liefert die bekannten Projektnamen für das Werkzeug "projects".
	Projects func() []string
	Log      *slog.Logger
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewServer erstellt den Server mit den gegebenen Werkzeugen.
func NewServer(in io.Reader, out io.Writer, resolve func(string) (*query.Engine, error), tools []Tool) *Server {
	s := &Server{in: in, out: out, Resolve: resolve, tools: tools, byName: map[string]Tool{}, Log: slog.Default(),
		Projects: func() []string { return nil }}
	for _, t := range tools {
		s.byName[t.Name] = t
	}
	return s
}

// Serve liest Nachrichten, bis die Eingabe endet.
func (s *Server) Serve() error {
	sc := bufio.NewScanner(s.in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	w := bufio.NewWriter(s.out)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := s.handle(line)
		if resp == nil {
			continue
		}
		b, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		w.Write(b)
		w.WriteByte('\n')
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return sc.Err()
}

// handle verarbeitet eine Nachricht. nil bedeutet: keine Antwort (Benachrichtigung).
func (s *Server) handle(line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "Parse-Fehler"}}
	}
	isNotification := len(req.ID) == 0
	if isNotification {
		return nil
	}
	switch req.Method {
	case "initialize":
		return ok(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "goreadmenator", "version": "0.1.0"},
		})
	case "ping":
		return ok(req.ID, map[string]any{})
	case "tools/list":
		var list []map[string]any
		for _, t := range s.tools {
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.Schema})
		}
		return ok(req.ID, map[string]any{"tools": list})
	case "tools/call":
		return s.call(req)
	default:
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "unbekannte Methode: " + req.Method}}
	}
}

func (s *Server) call(req request) *response {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "ungültige Parameter"}}
	}
	if p.Name == "projects" {
		return ok(req.ID, textResult(map[string]any{"projects": s.Projects()}, false))
	}
	t, found := s.byName[p.Name]
	if !found {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "unbekanntes Werkzeug: " + p.Name}}
	}
	var pa struct {
		Project string `json:"project"`
	}
	if len(p.Arguments) > 0 {
		_ = json.Unmarshal(p.Arguments, &pa)
	}
	eng, err := s.Resolve(pa.Project)
	if err != nil {
		return ok(req.ID, textResult(err.Error(), true))
	}
	result, err := t.Run(eng, p.Arguments)
	if err != nil {
		return ok(req.ID, textResult(err.Error(), true))
	}
	return ok(req.ID, textResult(result, false))
}

// textResult verpackt ein Ergebnis als Textinhalt im MCP-Format. Strings bleiben unverändert.
func textResult(v any, isError bool) map[string]any {
	text, isStr := v.(string)
	if !isStr {
		b, err := json.Marshal(v)
		if err != nil {
			text = "Ergebnis nicht kodierbar: " + err.Error()
			isError = true
		} else {
			text = string(b)
		}
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
}

func ok(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}
