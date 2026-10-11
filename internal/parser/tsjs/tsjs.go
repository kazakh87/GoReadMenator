package tsjs

import (
	"bytes"
	"context"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// Parser ist der TypeScript/JavaScript-Parser. Er ist zustandslos und nebenläufig sicher.
type Parser struct{}

// New liefert den Parser.
func New() Parser { return Parser{} }

// Language meldet die Sprache.
func (Parser) Language() model.Language { return model.LangTSJS }

// Parse analysiert eine TS/JS-Datei. Der Pfad ist Teil der Symbol-IDs.
func (Parser) Parse(ctx context.Context, src parser.Source) (parser.Result, error) {
	if err := ctx.Err(); err != nil {
		return parser.Result{}, err
	}
	return parseFile(src.Path, src.Content), nil
}

func parseFile(path string, content []byte) parser.Result {
	content = bytes.TrimPrefix(content, []byte("\xEF\xBB\xBF"))
	m := mask(content)
	toks := lex(m.masked, m.strs)
	f := newFileState(path, toks, m.docs)
	f.setTemplates(m.templates, content)
	f.parseScope(owner{isRoot: true, prefix: path + "#"}, len(toks))
	res := *f.res
	res.LOC = countCodeLines(m.masked)
	putTokBuf(toks)
	putMaskBuf(m.masked)
	return res
}

var _ parser.Parser = Parser{}
