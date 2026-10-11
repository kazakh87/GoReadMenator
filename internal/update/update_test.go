package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parsers"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
)

type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q")
	return r
}

func (r *repo) git(args ...string) {
	r.t.Helper()
	full := append([]string{"-C", r.dir, "-c", "user.name=t", "-c", "user.email=t@e.x", "-c", "commit.gpgsign=false"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *repo) write(rel, content string) {
	r.t.Helper()
	p := filepath.Join(r.dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) remove(rel string) {
	r.t.Helper()
	if err := os.Remove(filepath.Join(r.dir, rel)); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) commit(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
}

func (r *repo) cur(st store.Store, opt Options) Result {
	r.t.Helper()
	res, err := Run(context.Background(), st, opt)
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

func opts(dir string) Options {
	reg, _ := parsers.Registry()
	return Options{RepoDir: dir, Registry: reg, Workers: 3, BatchSize: 20, MaxFileSizeBytes: 5 << 20}
}

// snapshot beschreibt den Store ohne interne IDs, damit zwei Stores verglichen werden können.
func snapshot(t *testing.T, st store.Store) []string {
	t.Helper()
	ctx := context.Background()
	files, _ := st.Files(ctx)
	path := map[model.FileID]string{}
	for _, f := range files {
		path[f.ID] = f.Path
	}
	syms, _ := st.AllSymbols(ctx)
	locs, _ := st.AllLocations(ctx)
	refs, _ := st.AllReferences(ctx)
	rels, _ := st.AllRelations(ctx)
	imps, _ := st.AllImports(ctx)
	symName := map[model.SymbolID]string{}
	var out []string
	for _, s := range syms {
		symName[s.ID] = string(s.ID)
		out = append(out, "sym "+string(s.ID)+" "+string(s.Kind))
	}
	for _, l := range locs {
		out = append(out, fmt.Sprintf("loc %s %s %d:%d-%d:%d %s", l.SymbolID, path[l.FileID],
			l.Span.StartLine, l.Span.StartCol, l.Span.EndLine, l.Span.EndCol, l.Role))
	}
	for _, r := range refs {
		out = append(out, fmt.Sprintf("ref %s %s %s %d:%d %s -> %s %s", path[r.FileID], r.Kind, r.TargetName,
			r.Span.StartLine, r.Span.StartCol, r.Enclosing, r.TargetID, r.Confidence))
	}
	for _, x := range rels {
		out = append(out, fmt.Sprintf("rel %s %s %s %v", x.Kind, x.From, x.To, x.Derived))
	}
	for _, im := range imps {
		out = append(out, fmt.Sprintf("imp %s %s %s -> %s", path[im.FileID], im.Kind, im.Raw, path[im.ResolvedID]))
	}
	sort.Strings(out)
	return out
}

func equalSnap(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") == strings.Join(want, "\n") {
		return
	}
	gs, ws := map[string]bool{}, map[string]bool{}
	for _, g := range got {
		gs[g] = true
	}
	for _, w := range want {
		ws[w] = true
	}
	for _, g := range got {
		if !ws[g] {
			t.Errorf("nur im Inkrement: %s", g)
		}
	}
	for _, w := range want {
		if !gs[w] {
			t.Errorf("nur im Vollaufbau: %s", w)
		}
	}
	t.FailNow()
}

// fresh baut einen Index aus Grund auf (Vergleichsbasis).
func fresh(t *testing.T, dir string) []string {
	t.Helper()
	st := memory.New()
	if _, err := Run(context.Background(), st, withForce(opts(dir))); err != nil {
		t.Fatal(err)
	}
	return snapshot(t, st)
}

func withForce(o Options) Options { o.Force = true; return o }

func TestIncrementalMatchesFullBuild(t *testing.T) {
	r := newRepo(t)
	r.write("Models/Order.cs", "namespace App.Models;\npublic class Order {\n  public void Save(int n) { Log(n); }\n  void Log(int v) { }\n}\n")
	r.write("Services/Svc.cs", "namespace App.Services;\nusing App.Models;\npublic class Svc {\n  Order o;\n  public void Run() { o.Save(1); }\n}\n")
	r.write("web/lib.ts", "export class Repo { find(id: string) { return id; } }\nexport function helper(a: number) { return a; }\n")
	r.write("web/gone.ts", "export const old = 1;\n")
	r.commit("A")

	st := memory.New()
	o := opts(r.dir)
	first := r.cur(st, o)
	if first.Mode != "full" {
		t.Fatalf("erster Lauf muss Vollaufbau sein: %+v", first)
	}
	equalSnap(t, snapshot(t, st), fresh(t, r.dir))

	// Änderungen: Methode umbenennen, Datei löschen, neue Datei, Umbenennung, Änderung mit neuer Partial-Klasse
	r.write("Models/Order.cs", "namespace App.Models;\npublic class Order {\n  public void Persist(int n) { Log(n); }\n  void Log(int v) { }\n}\n")
	r.remove("web/gone.ts")
	r.write("Services/Extra.cs", "namespace App.Services;\npublic partial class Svc { public void Extra() { } }\n")
	r.write("web/lib2.ts", "import { helper } from './lib';\nexport const v = helper(2);\n")
	r.git("mv", "web/lib.ts", "web/core.ts")
	r.write("web/core.ts", "export class Repo { find(id: string) { return id; } }\nexport function helper(a: number) { return a + 1; }\n")
	r.commit("B")

	inc := r.cur(st, o)
	if inc.Mode != "incremental" {
		t.Fatalf("zweiter Lauf muss Inkrement sein: %+v", inc)
	}
	if inc.Parsed >= 6 {
		t.Fatalf("Inkrement hat %d Dateien geparst, erwartet nur die geänderten", inc.Parsed)
	}
	equalSnap(t, snapshot(t, st), fresh(t, r.dir))

	// Keine Änderung: nichts tun
	if again := r.cur(st, o); again.Mode != "noop" {
		t.Fatalf("ohne Änderung muss noop sein: %+v", again)
	}
}

func TestMissingCommitFallsBackToFullBuild(t *testing.T) {
	r := newRepo(t)
	r.write("a.ts", "export function f() { }\n")
	r.commit("A")
	st := memory.New()
	o := opts(r.dir)
	r.cur(st, o)
	if err := st.SetMeta(context.Background(), MetaLastCommit, "0123456789012345678901234567890123456789"); err != nil {
		t.Fatal(err)
	}
	res := r.cur(st, o)
	if res.Mode != "full" {
		t.Fatalf("unbekannter Commit muss Vollaufbau auslösen: %+v", res)
	}
}

func TestParserVersionChangeForcesFullBuild(t *testing.T) {
	r := newRepo(t)
	r.write("a.ts", "export function f() { }\n")
	r.commit("A")
	st := memory.New()
	o := opts(r.dir)
	r.cur(st, o)
	if err := st.SetMeta(context.Background(), MetaParserVersion, "veraltet"); err != nil {
		t.Fatal(err)
	}
	if res := r.cur(st, o); res.Mode != "full" {
		t.Fatalf("Parser-Version muss Vollaufbau auslösen: %+v", res)
	}
}

func TestDeletedPartialFileKeepsSymbol(t *testing.T) {
	r := newRepo(t)
	r.write("P1.cs", "namespace N { public partial class P { void A() { } } }\n")
	r.write("P2.cs", "namespace N { public partial class P { void B() { } } }\n")
	r.commit("A")
	st := memory.New()
	o := opts(r.dir)
	r.cur(st, o)
	r.remove("P2.cs")
	r.commit("B")
	r.cur(st, o)
	equalSnap(t, snapshot(t, st), fresh(t, r.dir))
	ctx := context.Background()
	p, ok, _ := st.Symbol(ctx, "csharp:N.P")
	if !ok {
		t.Fatal("partial-Klasse muss erhalten bleiben, solange eine Datei sie noch deklariert")
	}
	locs, _ := st.Locations(ctx, p.ID)
	if len(locs) != 1 {
		t.Fatalf("Fundorte = %d, want 1", len(locs))
	}
}

var _ = sort.Strings
