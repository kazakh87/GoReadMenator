package resolve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"goreadmenator/internal/index"
	"goreadmenator/internal/model"
	"goreadmenator/internal/parsers"
	"goreadmenator/internal/scanner"
	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
)

type mapReader map[string][]byte

func (m mapReader) Blob(sha string) ([]byte, error) { return m[sha], nil }
func (m mapReader) Close() error                    { return nil }

// indexFiles indiziert eine In-Memory-Quellsammlung (Pfad -> Inhalt) und löst auf.
func indexFiles(t *testing.T, files map[string]string, repoDir string) (store.Store, Stats) {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	blobs := mapReader{}
	var scanned []scanner.ScannedFile
	for p, content := range files {
		sha := "sha:" + p
		blobs[sha] = []byte(content)
		lang, ok := scanner.LangForPath(p)
		if !ok {
			t.Fatalf("keine Sprache für %s", p)
		}
		scanned = append(scanned, scanner.ScannedFile{Path: p, Lang: lang, BlobSHA: sha})
	}
	reg, err := parsers.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.Run(ctx, st, scanned, reg,
		func(context.Context) (index.ContentReader, error) { return blobs, nil },
		index.Options{Workers: 2, BatchSize: 50}); err != nil {
		t.Fatal(err)
	}
	stats, err := Run(ctx, st, Options{RepoDir: repoDir})
	if err != nil {
		t.Fatal(err)
	}
	return st, stats
}

// refTo sucht die Verwendung eines Namens in einer Datei an einer Zeile.
func refTo(t *testing.T, st store.Store, file string, name string, kind model.RefKind, line int) model.Reference {
	t.Helper()
	ctx := context.Background()
	refs, _ := st.AllReferences(ctx)
	fs, _ := st.Files(ctx)
	var fid model.FileID
	for _, f := range fs {
		if f.Path == file {
			fid = f.ID
		}
	}
	for _, r := range refs {
		if r.FileID == fid && r.TargetName == name && r.Kind == kind && r.Span.StartLine == line {
			return r
		}
	}
	t.Fatalf("Verwendung %s/%s (%s) in Zeile %d fehlt", file, name, kind, line)
	return model.Reference{}
}

func TestCSharpScopeUsingsAndOverloads(t *testing.T) {
	st, stats := indexFiles(t, map[string]string{
		"Models/Order.cs": `namespace App.Models;
public class Order {
    public int Total;
    public void Save(int n) { }
}
`,
		"Services/OrderService.cs": `namespace App.Services;
using App.Models;
public class OrderService {
    private Order _o;
    public void Run() {
        _o.Save(1);
        var x = new Order();
        Log(5);
        Log("a");
    }
    public void Log(int v) { }
    public void Log(string s) { }
}
`,
	}, "")

	ctx := context.Background()
	orderType := model.SymbolID("csharp:App.Models.Order")
	if r := refTo(t, st, "Services/OrderService.cs", "Order", model.RefTypeRef, 4); r.TargetID != orderType || r.Confidence != model.ConfResolved {
		t.Fatalf("Typverweis Order: %+v", r)
	}
	if r := refTo(t, st, "Services/OrderService.cs", "Order", model.RefConstruct, 7); r.TargetID != orderType || r.Confidence != model.ConfResolved {
		t.Fatalf("new Order: %+v", r)
	}
	if r := refTo(t, st, "Services/OrderService.cs", "Save", model.RefCall, 6); r.TargetID != "csharp:App.Models.Order.Save(System.Int32)" {
		t.Fatalf("Save-Aufruf: %+v", r)
	}
	// Zwei Überladungen mit gleicher Argumentzahl: ohne Typinferenz bleibt es mehrdeutig.
	// Ein falsches Ziel wäre schlimmer als keines.
	if r := refTo(t, st, "Services/OrderService.cs", "Log", model.RefCall, 8); r.TargetID != "" {
		t.Fatalf("mehrdeutige Überladung darf nicht geraten werden: %+v", r)
	}
	if r := refTo(t, st, "Services/OrderService.cs", "Log", model.RefCall, 9); r.TargetID != "" {
		t.Fatalf("mehrdeutige Überladung darf nicht geraten werden: %+v", r)
	}
	_ = ctx
	if stats.References == 0 || stats.Resolved == 0 || stats.Exact == 0 || stats.Unresolved == 0 {
		t.Fatalf("Stats: %+v", stats)
	}
}

func TestCSharpAmbiguousStaysUnresolved(t *testing.T) {
	st, _ := indexFiles(t, map[string]string{
		"A.cs": "namespace X { public class Dup { } }\n",
		"B.cs": "namespace Y { public class Dup { } }\nnamespace Z { public class User { Dup d; } }\n",
	}, "")
	r := refTo(t, st, "B.cs", "Dup", model.RefTypeRef, 2)
	if r.TargetID != "" {
		t.Fatalf("mehrdeutiger Name darf nicht geraten werden: %+v", r)
	}
}

func TestCSharpCrefWithParams(t *testing.T) {
	st, _ := indexFiles(t, map[string]string{
		"A.cs": `namespace N {
public class Other { public void Run(int a) { } }
/// <see cref="N.Other.Run(System.Int32)"/>
public class Doc { }
}`,
	}, "")
	r := refTo(t, st, "A.cs", "N.Other.Run(System.Int32)", model.RefCref, 3)
	if r.TargetID != "csharp:N.Other.Run(System.Int32)" || r.Confidence != model.ConfExact {
		t.Fatalf("cref: %+v", r)
	}
}

func TestTSJSImportsBindingsAndDefault(t *testing.T) {
	st, stats := indexFiles(t, map[string]string{
		"src/lib.ts": `export class Repo {
  find(id: string) { return id; }
}
export function helper(a: number) { return a; }
export default function main() { return 1; }
`,
		"src/app.ts": `import { Repo, helper as h } from './lib';
import main from './lib';
import React from 'react';
class App {
  run() {
    const r = new Repo();
    r.find('x');
    h(1);
    main();
  }
}
`,
	}, "")
	if r := refTo(t, st, "src/app.ts", "Repo", model.RefConstruct, 6); r.TargetID != "tsjs:src/lib.ts#Repo" || r.Confidence != model.ConfExact {
		t.Fatalf("new Repo: %+v", r)
	}
	if r := refTo(t, st, "src/app.ts", "find", model.RefCall, 7); r.TargetID != "tsjs:src/lib.ts#Repo.find(string)" {
		t.Fatalf("r.find (Heuristik über eindeutigen Namen): %+v", r)
	}
	if r := refTo(t, st, "src/app.ts", "h", model.RefCall, 8); r.TargetID != "tsjs:src/lib.ts#helper(number)" || r.Confidence != model.ConfExact {
		t.Fatalf("Alias h: %+v", r)
	}
	if r := refTo(t, st, "src/app.ts", "main", model.RefCall, 9); r.TargetID != "tsjs:src/lib.ts#main()" {
		t.Fatalf("Default-Import main: %+v", r)
	}
	if stats.Imports != 3 || stats.ImportsFound != 2 {
		t.Fatalf("Imports=%d found=%d, want 3/2 (react extern)", stats.Imports, stats.ImportsFound)
	}
}

func TestTSConfigPathsAndTypes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{
  // Pfad-Alias
  "compilerOptions": {
    "baseUrl": ".",
    "paths": { "@/*": ["src/*"], }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := indexFiles(t, map[string]string{
		"src/util/x.ts": "export interface Opts { a: number }\nexport function fn(o: Opts) { return o; }\n",
		"src/main.ts":   "import { fn, Opts } from '@/util/x';\nconst o: Opts = { a: 1 };\nfn(o);\n",
	}, dir)
	if r := refTo(t, st, "src/main.ts", "Opts", model.RefTypeRef, 2); r.TargetID != "tsjs:src/util/x.ts#Opts" {
		t.Fatalf("Typ über Pfad-Alias: %+v", r)
	}
	if r := refTo(t, st, "src/main.ts", "fn", model.RefCall, 3); r.TargetID != "tsjs:src/util/x.ts#fn(Opts)" || r.Confidence != model.ConfExact {
		t.Fatalf("Funktion über Pfad-Alias: %+v", r)
	}
}

func TestRerunOnlyNamesClearsStaleTarget(t *testing.T) {
	ctx := context.Background()
	st, _ := indexFiles(t, map[string]string{
		"A.cs": "namespace N { public class Gone { } }\n",
		"B.cs": "namespace N { public class User { Gone g; } }\n",
	}, "")
	r := refTo(t, st, "B.cs", "Gone", model.RefTypeRef, 1)
	if r.TargetID == "" {
		t.Fatal("Ausgangslage: Gone muss aufgelöst sein")
	}
	// Symbol entfernen (Datei A löschen) und gezielt neu auflösen
	fs, _ := st.Files(ctx)
	for _, f := range fs {
		if f.Path == "A.cs" {
			if err := st.DeleteFile(ctx, f.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := Run(ctx, st, Options{OnlyNames: map[string]bool{"Gone": true}}); err != nil {
		t.Fatal(err)
	}
	r2 := refTo(t, st, "B.cs", "Gone", model.RefTypeRef, 1)
	if r2.TargetID != "" {
		t.Fatalf("veraltetes Ziel bleibt bestehen: %+v", r2)
	}
	_ = fmt.Sprint
}

func TestParamCount(t *testing.T) {
	cases := map[model.SymbolID]int{
		"csharp:A.B":                         -1,
		"csharp:A.B()":                       0,
		"csharp:A.B(System.Int32)":           1,
		"csharp:A.B(Dictionary<K,V>,int)":    2,
		"tsjs:src/a.ts#f(number[],Map<A,B>)": 2,
	}
	for id, want := range cases {
		if got := paramCount(id); got != want {
			t.Errorf("paramCount(%q) = %d, want %d", id, got, want)
		}
	}
}
