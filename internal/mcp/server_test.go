package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"goreadmenator/internal/query"
)

func runSession(t *testing.T, eng *query.Engine, lines ...string) []map[string]any {
	t.Helper()
	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out bytes.Buffer
	srv := NewServer(in, &out, func(project string) (*query.Engine, error) {
		if eng == nil {
			return nil, fmt.Errorf("Index nicht geladen")
		}
		return eng, nil
	}, Tools())
	if err := srv.Serve(); err != nil {
		t.Fatal(err)
	}
	var resps []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("keine gültige JSON-Antwort: %q: %v", l, err)
		}
		resps = append(resps, m)
	}
	return resps
}

func TestHandshakeAndToolList(t *testing.T) {
	resps := runSession(t, nil,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
	)
	if len(resps) != 3 {
		t.Fatalf("Antworten = %d, want 3 (Benachrichtigung bekommt keine Antwort)", len(resps))
	}
	init := resps[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocolVersion = %v", init["protocolVersion"])
	}
	if _, ok := init["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatal("capabilities.tools fehlt")
	}
	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tl := range tools {
		m := tl.(map[string]any)
		names[m["name"].(string)] = true
		if m["inputSchema"] == nil || m["description"] == nil {
			t.Fatalf("Werkzeug ohne Schema oder Beschreibung: %v", m["name"])
		}
	}
	for _, want := range []string{"find_symbols", "get_symbol", "api", "neighborhood", "usages", "context", "source", "changes", "status"} {
		if !names[want] {
			t.Errorf("Werkzeug %s fehlt", want)
		}
	}
}

func TestUnknownMethodAndTool(t *testing.T) {
	resps := runSession(t, nil,
		`{"jsonrpc":"2.0","id":1,"method":"gibtesnicht"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`{kaputt`,
	)
	if resps[0]["error"] == nil || resps[1]["error"] == nil {
		t.Fatalf("Fehler müssen gemeldet werden: %+v", resps)
	}
	if resps[2]["error"] == nil {
		t.Fatal("Parse-Fehler muss gemeldet werden")
	}
}

func TestToolErrorIsReportedAsResultNotProtocolError(t *testing.T) {
	// ohne Engine ist die Ausführung nicht möglich: der Fehler muss als isError-Ergebnis kommen,
	// damit der Assistent die Meldung lesen kann.
	resps := runSession(t, nil,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"status","arguments":{}}}`,
	)
	if resps[0]["error"] != nil {
		t.Fatalf("Werkzeugfehler dürfen kein Protokollfehler sein: %+v", resps[0])
	}
}

func TestProjectsToolAndProjectErrorsAreResults(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"projects","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"status","arguments":{"project":"x"}}}`,
	}, "\n") + "\n")
	var out bytes.Buffer
	srv := NewServer(in, &out, func(project string) (*query.Engine, error) {
		return nil, fmt.Errorf("unbekanntes Projekt %q", project)
	}, Tools())
	srv.Projects = func() []string { return []string{"alpha", "beta"} }
	if err := srv.Serve(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if !strings.Contains(lines[0], "alpha") || !strings.Contains(lines[0], "beta") {
		t.Fatalf("projects-Antwort: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"isError":true`) || !strings.Contains(lines[1], "unbekanntes Projekt") {
		t.Fatalf("Projektfehler muss als Ergebnis kommen: %s", lines[1])
	}
}
