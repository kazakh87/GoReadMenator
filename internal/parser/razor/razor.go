package razor

import (
	"bytes"
	"context"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
	"goreadmenator/internal/parser/csharp"
)

// Parser parst Razor-Dateien oder ASPX-Seiten. Er ist zustandslos und nebenläufig sicher.
type Parser struct {
	aspx bool
}

// NewRazor liefert den Parser für .cshtml und .razor.
func NewRazor() Parser { return Parser{} }

// NewASPX liefert den Parser für .aspx, .ascx und .master.
func NewASPX() Parser { return Parser{aspx: true} }

// Language meldet die Sprache.
func (p Parser) Language() model.Language {
	if p.aspx {
		return model.LangASPX
	}
	return model.LangRazor
}

// Parse erzeugt aus der Vorlage eine C#-Datei, parst sie mit dem C#-Parser und überträgt die Zeilen zurück.
func (p Parser) Parse(ctx context.Context, src parser.Source) (parser.Result, error) {
	if err := ctx.Err(); err != nil {
		return parser.Result{}, err
	}
	content := bytes.TrimPrefix(src.Content, []byte("\xef\xbb\xbf"))
	g := newGen(content)
	if p.aspx {
		g.aspxRegion()
	} else {
		g.razorRegion(0, len(content), false, &g.body)
	}
	synthetic, origLine := g.build(src.Path)
	res, err := csharp.New().Parse(ctx, parser.Source{Path: src.Path, Content: []byte(synthetic)})
	if err != nil {
		return parser.Result{}, err
	}
	mapLine := func(line int) int {
		if line >= 1 && line < len(origLine) && origLine[line] > 0 {
			return origLine[line]
		}
		return 1
	}
	for i := range res.Locations {
		res.Locations[i].Span.StartLine = mapLine(res.Locations[i].Span.StartLine)
		res.Locations[i].Span.EndLine = mapLine(res.Locations[i].Span.EndLine)
	}
	for i := range res.References {
		res.References[i].Span.StartLine = mapLine(res.References[i].Span.StartLine)
		res.References[i].Span.EndLine = mapLine(res.References[i].Span.EndLine)
	}
	for i := range res.Diagnostics {
		res.Diagnostics[i].Span.StartLine = mapLine(res.Diagnostics[i].Span.StartLine)
		res.Diagnostics[i].Span.EndLine = mapLine(res.Diagnostics[i].Span.EndLine)
	}
	// @page-Routen: die Seite ist unter ihrem Pfad erreichbar (alle Verben). Der Verweis hängt an der Seitenklasse.
	classID := model.NewSymbolID(model.LangCSharp, g.fqn, nil)
	for _, pg := range g.pages {
		res.References = append(res.References, model.Reference{Kind: model.RefHTTPRoute,
			TargetName: "ANY " + model.NormalizeRoute(pg.path), Enclosing: classID,
			Span: model.Span{StartLine: pg.line, StartCol: 1, EndLine: pg.line, EndCol: 1}, Confidence: model.ConfExact})
	}
	res.LOC = bytes.Count(content, []byte("\n")) + 1
	return res, nil
}

var _ parser.Parser = Parser{}
