package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"goreadmenator/internal/model"
)

func TestLangForPath(t *testing.T) {
	cases := map[string]model.Language{
		"A.cs":      model.LangCSharp,
		"x/B.CS":    model.LangCSharp,
		"index.ts":  model.LangTSJS,
		"App.tsx":   model.LangTSJS,
		"lib.mjs":   model.LangTSJS,
		"README.md": "",
		"Makefile":  "",
	}
	for p, want := range cases {
		got, ok := LangForPath(p)
		if want == "" && ok {
			t.Errorf("%s: unerwartet unterstützt (%s)", p, got)
		}
		if want != "" && (!ok || got != want) {
			t.Errorf("%s: got %q,%v want %q", p, got, ok, want)
		}
	}
}

func TestSafePath(t *testing.T) {
	ok := []string{"src/A.cs", "packages/zod/src/x.ts", "Name with space.cs"}
	bad := []string{"", "/etc/passwd", "../x.cs", "a/../../b.cs", "a\\b.cs", "a//b.cs"}
	for _, p := range ok {
		if !SafePath(p) {
			t.Errorf("SafePath(%q) = false, want true", p)
		}
	}
	for _, p := range bad {
		if SafePath(p) {
			t.Errorf("SafePath(%q) = true, want false", p)
		}
	}
}

// makeRepo erzeugt ein Git-Repo mit Fixtures und liefert das Verzeichnis.
func makeRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@e.x", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init", "-q")
	write(".gitignore", "bin/\n")
	write("src/A.cs", "class A {}\n")
	write("Ümlaut Datei.cs", "class U {}\n")
	// Monorepo-Layout: muss enthalten sein (Regression aus P0, Python ignoriert "packages").
	write("packages/zod/src/x.ts", "export const x = 1;\n")
	write("README.md", "# doku\n")
	write("big.ts", string(make([]byte, 2048)))
	write("bin/Debug/ignored.cs", "class I {}\n") // per .gitignore nicht getrackt
	if err := os.Symlink("../outside.cs", filepath.Join(dir, "link.cs")); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "fixtures")
	// Untracked Datei nach dem Commit darf nicht auftauchen.
	write("untracked.cs", "class N {}\n")
	return dir
}

func TestScanFixtureRepo(t *testing.T) {
	dir := makeRepo(t)
	res, err := Scan(context.Background(), dir, Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if res.Commit == "" {
		t.Fatal("Commit leer")
	}

	got := map[string]model.Language{}
	for _, f := range res.Files {
		got[f.Path] = f.Lang
		if f.BlobSHA == "" {
			t.Errorf("%s: BlobSHA leer", f.Path)
		}
	}
	want := map[string]model.Language{
		"src/A.cs":              model.LangCSharp,
		"Ümlaut Datei.cs":       model.LangCSharp,
		"packages/zod/src/x.ts": model.LangTSJS,
	}
	for p, lang := range want {
		if got[p] != lang {
			t.Errorf("Datei %q fehlt oder falsche Sprache: %q", p, got[p])
		}
	}
	if len(got) != len(want) {
		t.Errorf("unerwartete Dateien: %v", got)
	}

	counts := res.SkipCounts()
	if counts[SkipTooLarge] != 1 {
		t.Errorf("too_large = %d, want 1", counts[SkipTooLarge])
	}
	if counts[SkipSymlink] != 1 {
		t.Errorf("symlink = %d, want 1", counts[SkipSymlink])
	}
	if counts[SkipUnsupported] != 2 {
		t.Errorf("unsupported_ext = %d, want 2 (README.md, .gitignore)", counts[SkipUnsupported])
	}
	for _, s := range res.Skipped {
		if s.Path == "untracked.cs" || s.Path == "bin/Debug/ignored.cs" {
			t.Errorf("nicht getrackte Datei taucht auf: %s", s.Path)
		}
	}
}

func TestScanEmptyRepoFails(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skip("git nicht verfügbar")
	}
	if _, err := Scan(context.Background(), dir, Options{}); err == nil {
		t.Fatal("erwartet Fehler bei Repo ohne Commit")
	}
}

// TestScanMatchesGitLsFiles prüft gegen ein echtes Repository, falls
// READMENATOR_CORPUS gesetzt ist. Die Dateiliste muss der von `git ls-files`
// für die indizierbaren Endungen entsprechen.
func TestScanMatchesGitLsFiles(t *testing.T) {
	dir := os.Getenv("READMENATOR_CORPUS")
	if dir == "" {
		t.Skip("READMENATOR_CORPUS nicht gesetzt")
	}
	res, err := Scan(context.Background(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", dir, "ls-files").Output()
	if err != nil {
		t.Fatal(err)
	}
	var expected int
	for _, line := range splitLines(out) {
		if _, ok := LangForPath(line); ok {
			expected++
		}
	}
	if len(res.Files) != expected {
		t.Fatalf("Scanner: %d Dateien, git ls-files (unterstützte Endungen): %d", len(res.Files), expected)
	}
}

func splitLines(b []byte) []string {
	var out []string
	start := 0
	for i, c := range b {
		if c == '\n' {
			out = append(out, string(b[start:i]))
			start = i + 1
		}
	}
	return out
}
