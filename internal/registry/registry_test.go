package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddListRemoveRoundtrip(t *testing.T) {
	dir := t.TempDir()
	repoA := filepath.Join(dir, "a")
	repoB := filepath.Join(dir, "b")
	os.MkdirAll(filepath.Join(repoA, ".git"), 0o755)
	os.MkdirAll(repoB, 0o755)
	path := filepath.Join(dir, "projects.json")

	r, err := Load(path)
	if err != nil || len(r.Projects) != 0 {
		t.Fatalf("leere Registry: %+v %v", r, err)
	}
	if err := r.Add("alpha", repoA, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("beta", repoB, filepath.Join(dir, "beta.snap")); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	r2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := r2.Names(); len(got) != 2 || got[0] != "alpha" {
		t.Fatalf("Namen = %v", got)
	}
	a, _ := r2.Get("alpha")
	if a.DB != filepath.Join(repoA, ".git", "goreadmenator", "index.snap") {
		t.Fatalf("Standard-DB für Git-Repo: %s", a.DB)
	}
	if !r2.Remove("beta") || r2.Remove("beta") {
		t.Fatal("Remove muss genau einmal wirken")
	}
}

func TestInvalidNamesAndPaths(t *testing.T) {
	var r Registry
	if err := r.Add("Mit Leerzeichen", t.TempDir(), ""); err == nil {
		t.Fatal("ungültiger Name muss abgelehnt werden")
	}
	if err := r.Add("ok", filepath.Join(t.TempDir(), "gibtsnicht"), ""); err == nil {
		t.Fatal("fehlendes Verzeichnis muss abgelehnt werden")
	}
}
