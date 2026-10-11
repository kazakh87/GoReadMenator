package cli

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectAddListRemoveAndMultiProjectResolve(t *testing.T) {
	dir := t.TempDir()
	repoA := filepath.Join(dir, "alpha-repo")
	repoB := filepath.Join(dir, "beta-repo")
	os.MkdirAll(repoA, 0o755)
	os.MkdirAll(repoB, 0o755)
	regPath := filepath.Join(dir, "projects.json")

	run := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		app := &App{Stdout: &out, Stderr: &errb}
		// Optionen über die Befehlszeile: Flag-Standardwerte überschreiben vorab gesetzte Felder.
		code := app.Run(context.Background(), append([]string{"-registry", regPath}, args...))
		return out.String(), errb.String(), code
	}

	if _, errs, code := run("project", "add", "alpha", repoA); code != ExitOK {
		t.Fatalf("add alpha: %d %s", code, errs)
	}
	if _, errs, code := run("project", "add", "beta", repoB); code != ExitOK {
		t.Fatalf("add beta: %d %s", code, errs)
	}
	if _, _, code := run("project", "add", "Ungültig Name", repoA); code == ExitOK {
		t.Fatal("ungültiger Name muss abgelehnt werden")
	}
	out, _, _ := run("project", "list")
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("list: %q", out)
	}

	// Mehrere Projekte ohne Namen: Fehler mit Auswahl
	var log bytes.Buffer
	app := &App{Stderr: &log, Registry: regPath, Backend: "snapshot"}
	mgr, err := app.projectManager(context.Background(), slog.New(slog.NewTextHandler(&log, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.closeAll()
	if _, err := mgr.resolve(""); err == nil || !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("ohne Projektname muss die Auswahl genannt werden: %v", err)
	}
	if _, err := mgr.resolve("gibtsnicht"); err == nil || !strings.Contains(err.Error(), "Verfügbar") {
		t.Fatalf("unbekanntes Projekt: %v", err)
	}
	if eng, err := mgr.resolve("alpha"); err != nil || eng == nil {
		t.Fatalf("alpha muss auflösbar sein: %v", err)
	}
	if got := mgr.names(); len(got) != 2 || got[0] != "alpha" {
		t.Fatalf("Namen = %v", got)
	}

	if _, _, code := run("project", "remove", "beta"); code != ExitOK {
		t.Fatal("remove beta")
	}
	out, _, _ = run("project", "list")
	if strings.Contains(out, "beta") {
		t.Fatalf("beta muss entfernt sein: %q", out)
	}
}
