package tsjs

import (
	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// MaxLanguageVersion ist das neueste ES-Jahr, bis zu dem der Parser Syntax erfasst (ES2022).
// Neuere Ziele führen zu einer Projektwarnung, siehe internal/langver.
const MaxLanguageVersion = "2022"

// Coverage meldet die Sprachabdeckung des TypeScript/JavaScript-Parsers.
func (Parser) Coverage() parser.Coverage {
	return parser.Coverage{Language: model.LangTSJS, MaxVersion: MaxLanguageVersion}
}
