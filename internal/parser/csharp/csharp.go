package csharp

import (
	"bytes"
	"context"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// Parser ist der C#-Parser. Er ist zustandslos und damit nebenläufig sicher.
type Parser struct{}

// New liefert einen C#-Parser.
func New() Parser { return Parser{} }

// Language meldet die Sprache.
func (Parser) Language() model.Language { return model.LangCSharp }

// Parse analysiert eine C#-Datei.
func (Parser) Parse(ctx context.Context, src parser.Source) (parser.Result, error) {
	if err := ctx.Err(); err != nil {
		return parser.Result{}, err
	}
	return parseFileAt(src.Path, src.Content), nil
}

// parseFile führt Maskierung, Tokenisierung und Auswertung aus.
func parseFile(content []byte) parser.Result { return parseFileAt("", content) }

// parseFileAt wie parseFile, mit dem Pfad der Datei. Der Pfad macht die Symbole für Top-Level-Anweisungen eindeutig.
func parseFileAt(path string, content []byte) parser.Result {
	// UTF-8-BOM würde sonst mit dem ersten Schlüsselwort zu einem Bezeichner verschmelzen.
	content = bytes.TrimPrefix(content, []byte("\xEF\xBB\xBF"))
	m := mask(content)
	toks := lex(m.masked)
	f := newFileState(toks, m.docs)
	f.setLiterals(m.strs, content)
	f.path = path
	f.parseScope(owner{isRoot: true}, len(toks))
	f.res.Diagnostics = append(f.res.Diagnostics, versionWarnings(toks)...)
	f.scanTopLevelRoutes(toks)
	res := *f.res
	res.LOC = countCodeLines(m.masked)
	putTokBuf(toks)
	putMaskBuf(m.masked)
	return res
}

var _ parser.Parser = Parser{}
