package index

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"goreadmenator/internal/gitx"
	"goreadmenator/internal/scanner"
	"goreadmenator/internal/store/memory"
)

// Integrationstest: echte Pipeline mit echtem git cat-file gegen ein Fixture-Repo.
func TestPipelineWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@e.x"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	for i := 0; i < 200; i++ {
		p := filepath.Join(dir, fmt.Sprintf("packages/m%d/F%d.cs", i%5, i))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(fmt.Sprintf("class F%d {}\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-q", "-m", "fixtures")

	ctx := context.Background()
	res, err := scanner.Scan(ctx, dir, scanner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 200 {
		t.Fatalf("Scanner: %d Dateien, want 200", len(res.Files))
	}

	r := gitx.Runner{Dir: dir}
	factory := func(ctx context.Context) (ContentReader, error) {
		return r.OpenCatFile(ctx)
	}
	st := memory.New()
	rep, err := Run(ctx, st, res.Files, newReg(t, &fakeParser{}), factory, Options{Workers: 4, BatchSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 200 || len(rep.Errors) != 0 {
		t.Fatalf("Report: files=%d errors=%v", rep.Files, rep.Errors)
	}
}
