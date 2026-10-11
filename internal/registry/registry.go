// Package registry verwaltet die Projekte, die der MCP-Server bedienen kann. Jedes Projekt hat
// seinen eigenen Index. Die Trennung verhindert Kollisionen von Symbol-IDs zwischen Projekten
// und erlaubt, einzelne Projekte neu aufzubauen oder zu entfernen, ohne die anderen anzufassen.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Project beschreibt ein registriertes Repository und den Ort seines Index.
type Project struct {
	Name string `json:"name"`
	Repo string `json:"repo"`
	DB   string `json:"db"`
}

// Registry ist die Liste der Projekte.
type Registry struct {
	Projects []Project `json:"projects"`
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// DefaultPath liefert den Standardort der Registry im Benutzerverzeichnis.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".goreadmenator", "projects.json"), nil
}

// Load liest die Registry. Fehlt die Datei, ist die Registry leer.
func Load(path string) (Registry, error) {
	var r Registry
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("registry %s: %w", path, err)
	}
	return r, nil
}

// Save schreibt die Registry atomar.
func (r Registry) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	sort.Slice(r.Projects, func(i, j int) bool { return r.Projects[i].Name < r.Projects[j].Name })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Add legt ein Projekt an oder ersetzt es. Ohne DB-Pfad liegt der Index im Git-Verzeichnis des Repos.
func (r *Registry) Add(name, repo, db string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("ungültiger Projektname %q (erlaubt: Kleinbuchstaben, Ziffern, . _ -)", name)
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return fmt.Errorf("Repository-Pfad %q ist kein Verzeichnis", repo)
	}
	if db == "" {
		db = DefaultDB(abs)
	}
	p := Project{Name: name, Repo: abs, DB: db}
	for i := range r.Projects {
		if r.Projects[i].Name == name {
			r.Projects[i] = p
			return nil
		}
	}
	r.Projects = append(r.Projects, p)
	return nil
}

// Remove entfernt ein Projekt aus der Registry. Der Index bleibt auf der Platte.
func (r *Registry) Remove(name string) bool {
	for i := range r.Projects {
		if r.Projects[i].Name == name {
			r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
			return true
		}
	}
	return false
}

// Get liefert ein Projekt über seinen Namen.
func (r Registry) Get(name string) (Project, bool) {
	for _, p := range r.Projects {
		if p.Name == name {
			return p, true
		}
	}
	return Project{}, false
}

// Names liefert die Projektnamen, sortiert.
func (r Registry) Names() []string {
	out := make([]string, 0, len(r.Projects))
	for _, p := range r.Projects {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

// DefaultDB liefert den Indexpfad für ein Repository: im Git-Verzeichnis, wenn vorhanden.
func DefaultDB(repo string) string {
	gitDir := filepath.Join(repo, ".git")
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		return filepath.Join(gitDir, "goreadmenator", "index.snap")
	}
	return filepath.Join(repo, ".goreadmenator", "index.snap")
}
