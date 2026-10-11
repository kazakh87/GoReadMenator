package vbnet

import (
	"context"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// Parser ist der VB.NET-Parser. Er ist zustandslos und nebenläufig sicher.
type Parser struct{}

// New liefert den Parser.
func New() Parser { return Parser{} }

// Language meldet die Sprache.
func (Parser) Language() model.Language { return model.LangVBNet }

// Parse analysiert eine VB.NET-Datei.
func (Parser) Parse(ctx context.Context, src parser.Source) (parser.Result, error) {
	if err := ctx.Err(); err != nil {
		return parser.Result{}, err
	}
	return parseFile(src.Content), nil
}

func parseFile(content []byte) parser.Result {
	content = trimBOM(content)
	l := lex(content)
	f := newFileState(l)
	f.parseStatements(owner{isRoot: true, kind: "root"}, "")
	res := *f.res
	res.LOC = countLines(l)
	return res
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func countLines(l lexed) int {
	lines := map[int]bool{}
	for _, s := range l.stmts {
		lines[s.line] = true
	}
	return len(lines)
}

var _ parser.Parser = Parser{}
