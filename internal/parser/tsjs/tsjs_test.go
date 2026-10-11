package tsjs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

func parse(t *testing.T, path, src string) parser.Result {
	t.Helper()
	res, err := New().Parse(context.Background(), parser.Source{Path: path, Content: []byte(src)})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func sym(t *testing.T, res parser.Result, id model.SymbolID) model.Symbol {
	t.Helper()
	for _, s := range res.Symbols {
		if s.ID == id {
			return s
		}
	}
	var ids []string
	for _, s := range res.Symbols {
		ids = append(ids, string(s.ID))
	}
	t.Fatalf("Symbol %q fehlt. Vorhanden:\n%s", id, strings.Join(ids, "\n"))
	return model.Symbol{}
}

func locs(res parser.Result, id model.SymbolID) []model.Location {
	var out []model.Location
	for _, l := range res.Locations {
		if l.SymbolID == id {
			out = append(out, l)
		}
	}
	return out
}

func refs(res parser.Result, kind model.RefKind, name string) []model.Reference {
	var out []model.Reference
	for _, r := range res.References {
		if r.Kind == kind && r.TargetName == name {
			out = append(out, r)
		}
	}
	return out
}

func importsOf(res parser.Result) map[string]model.ImportKind {
	m := map[string]model.ImportKind{}
	for _, im := range res.Imports {
		m[im.Raw] = im.Kind
	}
	return m
}

func TestImportForms(t *testing.T) {
	res := parse(t, "src/a.ts", `
import React from 'react';
import { useState, type Dispatch, Foo as Bar } from "react/hooks";
import * as utils from './utils';
import './side-effect';
import type { Config } from "./config";
export * from './barrel';
export * as ns from './ns';
export { a, b } from './reexp';
const fs = require('fs');
const lazy = import('./lazy');
import legacy = require('legacy');
`)
	got := importsOf(res)
	want := map[string]model.ImportKind{
		"react":         model.ImportDefault,
		"react/hooks":   model.ImportESM,
		"./utils":       model.ImportNamespace,
		"./side-effect": model.ImportESM,
		"./config":      model.ImportTypeOnly,
		"./barrel":      model.ImportReexport,
		"./ns":          model.ImportReexport,
		"./reexp":       model.ImportReexport,
		"fs":            model.ImportRequire,
		"./lazy":        model.ImportDynamic,
		"legacy":        model.ImportRequire,
	}
	for raw, kind := range want {
		if got[raw] != kind {
			t.Errorf("Import %q: Art %q, want %q (alle: %v)", raw, got[raw], kind, got)
		}
	}
	for _, n := range []string{"useState", "Foo", "utils"} {
		if len(refs(res, model.RefImport, n)) == 0 {
			t.Errorf("Import-Verweis %q fehlt", n)
		}
	}
}

func TestFunctionsOverloadsAndArrows(t *testing.T) {
	res := parse(t, "src/fn.ts", `
export function add(a: number, b: number): number {
  return a + b;
}

export function pick(x: string): string;
export function pick(x: number): number;
export function pick(x: any): any { return x; }

export const double = (n: number): number => n * 2;
const greet = async name => {
  return helper(name);
};
const handler = function (req: Request) { return req.url; };
export const answer = 42;
`)
	add := sym(t, res, "tsjs:src/fn.ts#add(number,number)")
	if add.Kind != model.KindFunction || add.Signature == "" {
		t.Fatalf("add: %+v", add)
	}
	if l := locs(res, add.ID); len(l) != 1 || l[0].Span.StartLine != 2 || l[0].Span.EndLine != 4 {
		t.Fatalf("add Span = %+v, want 2-4", l)
	}
	sym(t, res, "tsjs:src/fn.ts#pick(string)")
	sym(t, res, "tsjs:src/fn.ts#pick(number)")
	pickAny := sym(t, res, "tsjs:src/fn.ts#pick(any)")
	if len(locs(res, pickAny.ID)) != 1 {
		t.Fatal("Implementierung pick(any) fehlt")
	}
	dbl := sym(t, res, "tsjs:src/fn.ts#double(number)")
	if dbl.Kind != model.KindFunction {
		t.Fatalf("Pfeilfunktion als function erwartet, ist %s", dbl.Kind)
	}
	sym(t, res, "tsjs:src/fn.ts#greet(any)")
	sym(t, res, "tsjs:src/fn.ts#handler(Request)")
	ans := sym(t, res, "tsjs:src/fn.ts#answer")
	if ans.Kind != model.KindVariable {
		t.Fatalf("answer Art = %s", ans.Kind)
	}
	// Verwendung innerhalb des Pfeilfunktions-Körpers
	h := refs(res, model.RefCall, "helper")
	if len(h) != 1 || h[0].Enclosing != "tsjs:src/fn.ts#greet(any)" {
		t.Fatalf("helper-Aufruf: %+v", h)
	}
}

func TestClassesInterfacesEnumsTypes(t *testing.T) {
	res := parse(t, "src/model.ts", `
import { Base } from './base';

@Injectable()
export class OrderService extends Base implements Repo<Order>, Disposable {
  private readonly items: Order[] = [];
  static count = 0;
  #secret: string;

  constructor(private http: HttpClient, name = 'x') {
    super();
    this.items = load();
  }

  get size(): number { return this.items.length; }

  async submit(order: Order, retries?: number): Promise<Receipt> {
    const r = await this.http.post('/x', order);
    return new Receipt(r);
  }
}

export interface Repo<T> extends Reader {
  save(item: T): void
  readonly id: string
  find?(id: string): T | undefined
}

export enum Status { Open = 1, Closed, "Other" = 9 }

export type Pair<A, B> = [A, B] | { first: A; second: B };
`)
	cls := sym(t, res, "tsjs:src/model.ts#OrderService")
	if cls.Kind != model.KindClass {
		t.Fatalf("Klasse Art = %s", cls.Kind)
	}
	if len(refs(res, model.RefAttribute, "Injectable")) != 1 {
		t.Fatal("Decorator Injectable fehlt")
	}
	if len(refs(res, model.RefInherits, "Base")) != 1 {
		t.Fatal("extends Base fehlt")
	}
	if len(refs(res, model.RefImplements, "Repo")) != 1 || len(refs(res, model.RefImplements, "Disposable")) != 1 {
		t.Fatal("implements fehlt")
	}
	ctor := sym(t, res, "tsjs:src/model.ts#OrderService.constructor(HttpClient,any)")
	if ctor.Kind != model.KindConstructor {
		t.Fatalf("Konstruktor Art = %s", ctor.Kind)
	}
	sym(t, res, "tsjs:src/model.ts#OrderService.items")
	sym(t, res, "tsjs:src/model.ts#OrderService.count")
	sym(t, res, "tsjs:src/model.ts#OrderService.size")
	submit := sym(t, res, "tsjs:src/model.ts#OrderService.submit(Order,number)")
	if submit.Kind != model.KindMethod {
		t.Fatalf("submit Art = %s", submit.Kind)
	}
	if len(refs(res, model.RefConstruct, "Receipt")) != 1 {
		t.Fatal("new Receipt fehlt")
	}
	if len(refs(res, model.RefCall, "load")) != 1 {
		t.Fatal("Aufruf load fehlt")
	}
	if len(refs(res, model.RefWrite, "items")) != 1 {
		t.Fatalf("Schreibzugriff this.items fehlt: %+v", res.References)
	}

	iface := sym(t, res, "tsjs:src/model.ts#Repo")
	if iface.Kind != model.KindInterface {
		t.Fatalf("Interface Art = %s", iface.Kind)
	}
	sym(t, res, "tsjs:src/model.ts#Repo.save(T)")
	sym(t, res, "tsjs:src/model.ts#Repo.id")
	sym(t, res, "tsjs:src/model.ts#Repo.find(string)")
	if len(refs(res, model.RefInherits, "Reader")) != 1 {
		t.Fatal("Interface extends Reader fehlt")
	}

	en := sym(t, res, "tsjs:src/model.ts#Status")
	if en.Kind != model.KindEnum {
		t.Fatalf("Enum Art = %s", en.Kind)
	}
	sym(t, res, "tsjs:src/model.ts#Status.Open")
	sym(t, res, "tsjs:src/model.ts#Status.Other")

	ta := sym(t, res, "tsjs:src/model.ts#Pair")
	if ta.Kind != model.KindTypeAlias {
		t.Fatalf("type-Alias Art = %s", ta.Kind)
	}
	if len(refs(res, model.RefTypeRef, "Pair")) != 0 {
		// Typparameter A und B dürfen keine Verweise sein
	}
	for _, n := range []string{"A", "B"} {
		if len(refs(res, model.RefTypeRef, n)) != 0 {
			t.Errorf("Typparameter %s darf kein Verweis sein", n)
		}
	}
}

func TestJSDoc(t *testing.T) {
	res := parse(t, "src/doc.ts", `
/**
 * Summiert Werte.
 * @param a Erster Wert
 * @param {number} [b=2] Zweiter Wert
 * @returns Die Summe.
 * @remarks Nur ganze Zahlen.
 * @see {@link other.run}
 */
export function sum(a: number, b?: number) { return a + (b ?? 0); }
`)
	s := sym(t, res, "tsjs:src/doc.ts#sum(number,number)")
	if s.Summary != "Summiert Werte." {
		t.Fatalf("Summary = %q", s.Summary)
	}
	if s.Returns != "Die Summe." || s.Remarks != "Nur ganze Zahlen." {
		t.Fatalf("Returns/Remarks = %q / %q", s.Returns, s.Remarks)
	}
	if len(s.ParamDocs) != 2 || s.ParamDocs[0].Name != "a" || s.ParamDocs[1].Name != "b" {
		t.Fatalf("ParamDocs = %+v", s.ParamDocs)
	}
	if len(refs(res, model.RefCref, "other.run")) != 1 {
		t.Fatal("@see/@link fehlt")
	}
}

func TestMaskingTemplatesRegexAndStrings(t *testing.T) {
	res := parse(t, "src/m.ts", `
// function Fake() {}
const s1 = "class InString {}";
const s2 = 'function Also() {}';
const t1 = `+"`"+`template class Tpl {} ${ realCall(1) } tail`+"`"+`;
const re = /class Re\/ex {}/g;
const div = 10 / 2 / 5;
export function real() {}
`)
	for _, s := range res.Symbols {
		if strings.Contains(s.Name, "Fake") || strings.Contains(s.Name, "InString") || strings.Contains(s.Name, "Also") ||
			strings.Contains(s.Name, "Tpl") || strings.Contains(s.Name, "Re") {
			t.Fatalf("Symbol aus Text/Literal: %+v", s)
		}
	}
	sym(t, res, "tsjs:src/m.ts#real()")
	if len(refs(res, model.RefCall, "realCall")) != 1 {
		t.Fatal("Aufruf in Template-Interpolation fehlt")
	}
	if len(refs(res, model.RefCall, "Re")) != 0 {
		t.Fatal("Regex-Inhalt darf keine Aufrufe erzeugen")
	}
}

func TestJSXComponentsInTSX(t *testing.T) {
	res := parse(t, "src/App.tsx", `
import Button from './Button';
export function App() {
  return (
    <div>
      <Button label="ok" />
      <span>Don't stop</span>
    </div>
  );
}
`)
	if len(refs(res, model.RefJSXComponent, "Button")) != 1 {
		t.Fatalf("JSX-Komponente Button fehlt: %+v", res.References)
	}
	sym(t, res, "tsjs:src/App.tsx#App()")
}

func TestDefaultAndAnonymousExports(t *testing.T) {
	res := parse(t, "src/d.js", `
export default function () { return 1; }
export default class Named {}
`)
	sym(t, res, "tsjs:src/d.js#default()")
	sym(t, res, "tsjs:src/d.js#Named")
}

func TestUnicodeBOMAndShebang(t *testing.T) {
	res := parse(t, "src/bom.ts", "\xEF\xBB\xBFexport const x = 1;\n")
	sym(t, res, "tsjs:src/bom.ts#x")
	res = parse(t, "bin/cli.js", "#!/usr/bin/env node\nfunction main() {}\n")
	sym(t, res, "tsjs:bin/cli.js#main()")
}

func TestPartialDeclarationsMergeViaSameID(t *testing.T) {
	res := parse(t, "src/merge.ts", `
interface Box { a: string }
interface Box { b: number }
`)
	b := sym(t, res, "tsjs:src/merge.ts#Box")
	if len(locs(res, b.ID)) != 2 {
		t.Fatalf("Interface-Merging: %d Fundorte, want 2", len(locs(res, b.ID)))
	}
}

func TestBrokenInputDoesNotPanic(t *testing.T) {
	inputs := []string{
		"", "class", "function (", "import", "export {", "interface X {", "const a = `${",
		"@", "enum E {", "type T =", "namespace N {", "import x from", "let [a, b",
	}
	for _, in := range inputs {
		_ = parse(t, "src/x.ts", in)
	}
}

// TestCorpusZod prüft die Stichproben aus P0 gegen zod, falls READMENATOR_CORPUS gesetzt ist.
func TestCorpusZod(t *testing.T) {
	dir := os.Getenv("READMENATOR_CORPUS")
	if dir == "" {
		t.Skip("READMENATOR_CORPUS nicht gesetzt")
	}
	rel := "packages/zod/src/v4/classic/schemas.ts"
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	res := parseFile(rel, data)
	for _, want := range []struct {
		name string
		line int
	}{{"ZodStandardSchemaWithJSON", 33}, {"ZodType", 34}, {"_ZodType", 170}, {"_ZodString", 361}, {"ZodString", 452}, {"string", 617}} {
		hit := false
		for _, s := range res.Symbols {
			if s.Name != want.name {
				continue
			}
			for _, l := range locs(res, s.ID) {
				if l.Span.StartLine == want.line {
					hit = true
				}
			}
		}
		if !hit {
			t.Errorf("%s nicht bei Zeile %d", want.name, want.line)
		}
	}
	t.Logf("zod schemas.ts: %d Symbole, %d Verwendungen, %d Diagnosen", len(res.Symbols), len(res.References), len(res.Diagnostics))
}
