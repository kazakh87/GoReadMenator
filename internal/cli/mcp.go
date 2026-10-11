package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goreadmenator/internal/mcp"
	"goreadmenator/internal/query"
	"goreadmenator/internal/registry"
	"goreadmenator/internal/store"
)

// closer ist ein Store, der sich ohne Zurückschreiben schließen lässt (Lesezugriff).
type closer interface {
	CloseWithoutSave() error
}

// cmdMCP startet den MCP-Server über stdin und stdout. Ausgaben auf stdout gehören dem Protokoll,
// Logs gehen nach stderr. Der Server liest nur. Mit -repo dient er genau diesem Projekt,
// sonst allen Projekten der Registry.
func (a *App) cmdMCP(ctx context.Context, in io.Reader, out io.Writer) int {
	log := slog.New(slog.NewTextHandler(a.Stderr, nil))
	mgr, err := a.projectManager(ctx, log)
	if err != nil {
		log.Error("Projekte laden", "err", err)
		return ExitError
	}
	defer mgr.closeAll()

	srv := mcp.NewServer(in, out, mgr.resolve, mcp.Tools())
	srv.Projects = mgr.names
	srv.Log = log
	if err := srv.Serve(); err != nil {
		log.Error("MCP-Server beendet", "err", err)
		return ExitError
	}
	return ExitOK
}

// projectManager baut die Projektliste: entweder das eine Repo aus -repo oder alle aus der Registry.
func (a *App) projectManager(ctx context.Context, log *slog.Logger) (*manager, error) {
	m := &manager{ctx: ctx, backend: a.Backend, log: log, holders: map[string]*engineHolder{}, projects: map[string]registry.Project{}}
	if a.RepoExplicit {
		abs, err := filepath.Abs(a.RepoDir)
		if err != nil {
			return nil, err
		}
		db := a.DBPath
		if !a.DBExplicit {
			db = registry.DefaultDB(abs)
		}
		name := strings.ToLower(filepath.Base(abs))
		m.projects[name] = registry.Project{Name: name, Repo: abs, DB: db}
		return m, nil
	}
	reg, err := registry.Load(a.registryPath())
	if err != nil {
		return nil, err
	}
	for _, p := range reg.Projects {
		m.projects[p.Name] = p
	}
	if len(reg.Projects) == 0 {
		log.Warn("Keine Projekte registriert. Mit \"goreadmenator project add <name> <repo>\" hinzufügen.")
	}
	return m, nil
}

func (a *App) registryPath() string {
	if a.Registry != "" {
		return a.Registry
	}
	p, err := registry.DefaultPath()
	if err != nil {
		return "projects.json"
	}
	return p
}

// manager verwaltet je Projekt einen Store und die Abfrageschicht, lädt sie lazy und erneuert sie bei Änderungen.
type manager struct {
	ctx      context.Context
	backend  string
	log      *slog.Logger
	projects map[string]registry.Project
	holders  map[string]*engineHolder
}

func (m *manager) names() []string {
	out := make([]string, 0, len(m.projects))
	for n := range m.projects {
		out = append(out, n)
	}
	return sortStrings(out)
}

// resolve liefert die Abfrageschicht eines Projekts. Ohne Namen gilt das einzige Projekt, sonst ist der Name nötig.
func (m *manager) resolve(name string) (*query.Engine, error) {
	if name == "" {
		switch len(m.projects) {
		case 0:
			return nil, fmt.Errorf("keine Projekte registriert")
		case 1:
			for n := range m.projects {
				name = n
			}
		default:
			return nil, fmt.Errorf("mehrere Projekte vorhanden, bitte \"project\" angeben: %s", strings.Join(m.names(), ", "))
		}
	}
	p, ok := m.projects[name]
	if !ok {
		return nil, fmt.Errorf("unbekanntes Projekt %q. Verfügbar: %s", name, strings.Join(m.names(), ", "))
	}
	h := m.holders[name]
	if h == nil {
		h = &engineHolder{ctx: m.ctx, backend: m.backend, path: p.DB, repo: p.Repo, log: m.log}
		if err := h.open(); err != nil {
			return nil, fmt.Errorf("Index von %s öffnen: %v", name, err)
		}
		m.holders[name] = h
	}
	h.refresh()
	if h.eng == nil {
		return nil, fmt.Errorf("Index von %s ist nicht geladen", name)
	}
	return h.eng, nil
}

func (m *manager) closeAll() {
	for _, h := range m.holders {
		h.close()
	}
}

// engineHolder hält Store und Abfrageschicht eines Projekts und lädt sie neu, wenn sich der Index geändert hat.
type engineHolder struct {
	ctx     context.Context
	backend string
	path    string
	repo    string
	log     *slog.Logger
	st      store.Store
	eng     *query.Engine
	mtime   time.Time
}

func (h *engineHolder) open() error {
	st, err := store.Open(h.ctx, h.backend, h.path)
	if err != nil {
		return err
	}
	h.st = st
	h.eng = query.New(st, h.repo)
	if t, ok := indexMtime(h.path); ok {
		h.mtime = t
	} else {
		h.log.Warn("Index-Datei fehlt, bitte zuerst \"goreadmenator index\" ausführen", "path", h.path)
	}
	return nil
}

func (h *engineHolder) close() {
	if h.st == nil {
		return
	}
	if c, ok := h.st.(closer); ok {
		c.CloseWithoutSave()
		return
	}
	h.st.Close()
}

// refresh lädt den Index neu, wenn die Datei seit dem letzten Stand geändert wurde.
func (h *engineHolder) refresh() {
	t, ok := indexMtime(h.path)
	if !ok || t.Equal(h.mtime) {
		return
	}
	h.close()
	if err := h.open(); err != nil {
		h.log.Error("Index neu laden", "err", err)
		return
	}
	h.log.Info("Index neu geladen", "path", h.path)
}

// cmdProject verwaltet die Registry: add, list, remove.
func (a *App) cmdProject(args []string) int {
	path := a.registryPath()
	reg, err := registry.Load(path)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return ExitError
	}
	if len(args) == 0 {
		fmt.Fprintln(a.Stderr, "Verwendung: project add <name> <repo> [-db pfad] | project list | project remove <name>")
		return ExitUsage
	}
	switch args[0] {
	case "list":
		if a.JSON {
			return a.writeJSONCode(reg)
		}
		if len(reg.Projects) == 0 {
			fmt.Fprintln(a.Stdout, "Keine Projekte registriert.")
		}
		for _, p := range reg.Projects {
			fmt.Fprintf(a.Stdout, "%-20s %s\n", p.Name, p.Repo)
		}
		return ExitOK
	case "add":
		if len(args) != 3 {
			fmt.Fprintln(a.Stderr, "Verwendung: project add <name> <repo> [-db pfad]")
			return ExitUsage
		}
		db := ""
		if a.DBExplicit {
			db = a.DBPath
		}
		if err := reg.Add(args[1], args[2], db); err != nil {
			fmt.Fprintln(a.Stderr, err)
			return ExitError
		}
		if err := reg.Save(path); err != nil {
			fmt.Fprintln(a.Stderr, err)
			return ExitError
		}
		p, _ := reg.Get(args[1])
		fmt.Fprintf(a.Stdout, "Projekt %s hinzugefügt (Index: %s). Erst \"goreadmenator -repo %s index\" ausführen.\n", p.Name, p.DB, p.Repo)
		return ExitOK
	case "remove":
		if len(args) != 2 {
			fmt.Fprintln(a.Stderr, "Verwendung: project remove <name>")
			return ExitUsage
		}
		if !reg.Remove(args[1]) {
			fmt.Fprintf(a.Stderr, "Projekt %q nicht gefunden\n", args[1])
			return ExitError
		}
		if err := reg.Save(path); err != nil {
			fmt.Fprintln(a.Stderr, err)
			return ExitError
		}
		fmt.Fprintf(a.Stdout, "Projekt %s entfernt. Der Index bleibt auf der Platte.\n", args[1])
		return ExitOK
	default:
		fmt.Fprintf(a.Stderr, "unbekannter Unterbefehl %q\n", args[0])
		return ExitUsage
	}
}

func (a *App) writeJSONCode(v any) int {
	if err := a.writeJSON(v); err != nil {
		return ExitError
	}
	return ExitOK
}

func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

// indexMtime liefert den neuesten Zeitstempel eines Index. Bei SQLite im WAL-Modus landen Änderungen zuerst in
// der Datei -wal, die Hauptdatei ändert sich erst beim Checkpoint. Deshalb zählen beide.
func indexMtime(path string) (time.Time, bool) {
	var newest time.Time
	found := false
	for _, p := range []string{path, path + "-wal"} {
		if fi, err := os.Stat(p); err == nil {
			found = true
			if fi.ModTime().After(newest) {
				newest = fi.ModTime()
			}
		}
	}
	return newest, found
}
