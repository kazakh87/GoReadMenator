package resolve

import (
	"context"
	"testing"

	"goreadmenator/internal/model"
)

func hasRel(t *testing.T, rels []model.Relation, from, to model.SymbolID, kind model.RelationKind) bool {
	t.Helper()
	for _, r := range rels {
		if r.From == from && r.To == to && r.Kind == kind {
			return true
		}
	}
	return false
}

func TestDerivedRelationsCSharp(t *testing.T) {
	ctx := context.Background()
	st, stats := indexFiles(t, map[string]string{
		"Base.cs": "namespace App { public class Base { } public class Money { } public class Marker : System.Attribute { } }\n",
		"Svc.cs": `namespace App.Services;
using App;
[Marker]
public class Svc : Base {
    private Money _total;
    public Money Get(Money delta) { return delta; }
}
`,
	}, "")
	svc := model.SymbolID("csharp:App.Services.Svc")
	base := model.SymbolID("csharp:App.Base")
	money := model.SymbolID("csharp:App.Money")
	marker := model.SymbolID("csharp:App.Marker")
	rels, _ := st.Relations(ctx, svc)

	if !hasRel(t, rels, svc, base, model.RelInherits) {
		t.Fatalf("inherits fehlt: %+v", rels)
	}
	if !hasRel(t, rels, svc, "csharp:App.Services.Svc._total", model.RelContains) && !hasRel(t, rels, svc, "csharp:App.Services.Svc._total", model.RelContains) {
		t.Fatalf("contains fehlt")
	}
	field := model.SymbolID("csharp:App.Services.Svc._total")
	if fr, _ := st.Relations(ctx, field); !hasRel(t, fr, field, money, model.RelHasType) {
		t.Fatalf("has_type fehlt: %+v", fr)
	}
	method := model.SymbolID("csharp:App.Services.Svc.Get(Money)")
	mr, _ := st.Relations(ctx, method)
	if !hasRel(t, mr, method, money, model.RelParamType) || !hasRel(t, mr, method, money, model.RelReturns) {
		t.Fatalf("param_type/returns fehlen: %+v", mr)
	}
	if !hasRel(t, rels, marker, svc, model.RelAttributeOf) {
		t.Fatalf("attribute_of fehlt: %+v", rels)
	}
	if stats.Relations == 0 {
		t.Fatal("Stats.Relations = 0")
	}
}

func TestDerivedRelationsTSJS(t *testing.T) {
	ctx := context.Background()
	st, _ := indexFiles(t, map[string]string{
		"src/base.ts": "export class Base {}\nexport interface Shape {}\nexport class Cfg {}\n",
		"src/app.ts": `import { Base, Shape, Cfg } from './base';
export class App extends Base implements Shape {
  cfg: Cfg;
}
export function make(c: Cfg): Cfg { return c; }
`,
	}, "")
	app := model.SymbolID("tsjs:src/app.ts#App")
	base := model.SymbolID("tsjs:src/base.ts#Base")
	shape := model.SymbolID("tsjs:src/base.ts#Shape")
	cfg := model.SymbolID("tsjs:src/base.ts#Cfg")
	rels, _ := st.Relations(ctx, app)
	if !hasRel(t, rels, app, base, model.RelInherits) {
		t.Fatalf("extends fehlt: %+v", rels)
	}
	if !hasRel(t, rels, app, shape, model.RelImplements) {
		t.Fatalf("implements fehlt: %+v", rels)
	}
	prop := model.SymbolID("tsjs:src/app.ts#App.cfg")
	if pr, _ := st.Relations(ctx, prop); !hasRel(t, pr, prop, cfg, model.RelHasType) {
		t.Fatalf("has_type fehlt: %+v", pr)
	}
	fn := model.SymbolID("tsjs:src/app.ts#make(Cfg)")
	fr, _ := st.Relations(ctx, fn)
	if !hasRel(t, fr, fn, cfg, model.RelParamType) || !hasRel(t, fr, fn, cfg, model.RelReturns) {
		t.Fatalf("param_type/returns fehlen: %+v", fr)
	}
}
