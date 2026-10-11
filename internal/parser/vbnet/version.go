package vbnet

import (
	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// MaxLanguageVersion ist die neueste VB-Version, bis zu der die Golden-Fälle getestet sind (16.9).
const MaxLanguageVersion = "16.9"

// Coverage meldet die Sprachabdeckung des VB.NET-Parsers.
func (Parser) Coverage() parser.Coverage {
	return parser.Coverage{Language: model.LangVBNet, MaxVersion: MaxLanguageVersion}
}
