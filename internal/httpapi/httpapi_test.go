package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/query"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
)

// fixtureStore enthält zwei Symbole mit gleichem Namen (für 409) und eine Klasse mit Methode.
func fixtureStore(t *testing.T) *query.Engine {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	fid, _ := st.PutFile(ctx, model.File{Path: "A.cs", Lang: model.LangCSharp})
	syms := []model.Symbol{
		{ID: "csharp:N.Repo", Kind: model.KindClass, Name: "Repo", FQN: "N.Repo", Modifiers: []string{"public"}, Container: "csharp:N"},
		{ID: "csharp:N.Repo.Save(System.Int32)", Kind: model.KindMethod, Name: "Save", FQN: "N.Repo.Save", Modifiers: []string{"public"},
			Container: "csharp:N.Repo", Signature: "public void Save(int id)", Summary: "Speichert."},
		{ID: "csharp:N.A.Save()", Kind: model.KindMethod, Name: "Save", FQN: "N.A.Save", Modifiers: []string{"public"}},
		{ID: "csharp:N.A", Kind: model.KindClass, Name: "A", FQN: "N.A", Modifiers: []string{"public"}},
	}
	var locs []model.Location
	for _, s := range syms {
		locs = append(locs, model.Location{SymbolID: s.ID, FileID: fid, Role: model.RoleDecl, Span: model.Span{StartLine: 1, EndLine: 2}})
	}
	if err := st.WriteBatch(ctx, store.Batch{Symbols: syms, Locations: locs}); err != nil {
		t.Fatal(err)
	}
	return query.New(st, "")
}

func newTestServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	eng := fixtureStore(t)
	srv := New(func(project string) (*query.Engine, error) {
		if project != "demo" {
			return nil, fmt.Errorf("unbekanntes Projekt %q", project)
		}
		return eng, nil
	}, func() []string { return []string{"demo"} }, token)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, url string, header ...string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("keine JSON-Antwort (%d): %s", resp.StatusCode, body)
	}
	return resp.StatusCode, m
}

func TestProjectsAndStatus(t *testing.T) {
	ts := newTestServer(t, "")
	code, m := get(t, ts.URL+"/v1/projects")
	if code != 200 || len(m["projects"].([]any)) != 1 {
		t.Fatalf("projects: %d %v", code, m)
	}
	code, m = get(t, ts.URL+"/v1/demo/status")
	if code != 200 || m["symbols"].(float64) != 4 {
		t.Fatalf("status: %d %v", code, m)
	}
}

func TestSymbolSearchAndDetails(t *testing.T) {
	ts := newTestServer(t, "")
	code, m := get(t, ts.URL+"/v1/demo/symbols?q=Repo&kind=class")
	if code != 200 || len(m["items"].([]any)) != 1 {
		t.Fatalf("symbols: %d %v", code, m)
	}
	code, m = get(t, ts.URL+"/v1/demo/symbol?ref="+strings.ReplaceAll("csharp:N.Repo.Save(System.Int32)", " ", "%20"))
	if code != 200 || m["summary"] != "Speichert." {
		t.Fatalf("symbol: %d %v", code, m)
	}
}

func TestErrorStatusCodes(t *testing.T) {
	ts := newTestServer(t, "")
	if code, m := get(t, ts.URL+"/v1/unbekannt/status"); code != 404 || !strings.Contains(m["error"].(string), "unbekanntes Projekt") {
		t.Fatalf("unbekanntes Projekt: %d %v", code, m)
	}
	if code, _ := get(t, ts.URL+"/v1/demo/symbol?ref=Gibtsnicht"); code != 404 {
		t.Fatalf("unbekanntes Symbol: %d", code)
	}
	if code, m := get(t, ts.URL+"/v1/demo/symbol?ref=Save"); code != 409 || !strings.Contains(m["error"].(string), "mehrdeutig") {
		t.Fatalf("mehrdeutiger Name: %d %v", code, m)
	}
	if code, _ := get(t, ts.URL+"/v1/demo/gibtsnicht"); code != 404 {
		t.Fatalf("unbekannter Endpunkt: %d", code)
	}
	if code, _ := get(t, ts.URL+"/v1/demo"); code != 404 {
		t.Fatalf("Pfad ohne Werkzeug: %d", code)
	}
}

func TestReadOnlyAndToken(t *testing.T) {
	ts := newTestServer(t, "geheim")
	if code, _ := get(t, ts.URL+"/v1/projects"); code != 401 {
		t.Fatalf("ohne Token: %d", code)
	}
	if code, _ := get(t, ts.URL+"/v1/projects", "Authorization", "Bearer falsch"); code != 401 {
		t.Fatalf("falsches Token: %d", code)
	}
	if code, _ := get(t, ts.URL+"/v1/projects", "Authorization", "Bearer geheim"); code != 200 {
		t.Fatalf("richtiges Token: %d", code)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/demo/status", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer geheim")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST muss abgelehnt werden: %d", resp.StatusCode)
	}
}

func TestOpenAPIDescribesEndpoints(t *testing.T) {
	ts := newTestServer(t, "")
	code, m := get(t, ts.URL+"/v1/openapi.json")
	if code != 200 {
		t.Fatalf("openapi: %d", code)
	}
	paths := m["paths"].(map[string]any)
	for _, p := range []string{"/v1/{project}/symbols", "/v1/{project}/context", "/v1/{project}/source", "/v1/{project}/changes"} {
		if _, ok := paths[p]; !ok {
			t.Errorf("Pfad %s fehlt in der Beschreibung", p)
		}
	}
}
