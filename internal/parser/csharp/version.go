package csharp

import (
	"fmt"

	"goreadmenator/internal/model"
	"goreadmenator/internal/parser"
)

// MaxLanguageVersion ist die neueste C#-Version, die der Parser vollständig erfassen soll.
// Neuere Versionen führen zu einer Warnung, siehe internal/langver.
const MaxLanguageVersion = "13"

// Coverage meldet die Sprachabdeckung des C#-Parsers.
func (Parser) Coverage() parser.Coverage {
	return parser.Coverage{Language: model.LangCSharp, MaxVersion: MaxLanguageVersion}
}

// partialFeature beschreibt ein Konstrukt, das der Parser nur teilweise erfasst. Erkannt wird es am
// Token-Muster, nicht am Text, damit Kommentare und Strings keine Warnung auslösen.
type partialFeature struct {
	name    string // Bezeichnung für die Meldung
	since   string // erste C#-Version mit dem Konstrukt
	message string // was fehlt
	detect  func(toks []token, i int) bool
}

var partialFeatures = []partialFeature{
	{
		name: "primärer Konstruktor", since: "12",
		message: "die Parameter werden nicht als Symbole erfasst",
		detect: func(toks []token, i int) bool {
			// class Name(  |  struct Name(
			return (toks[i].text == "class" || toks[i].text == "struct") && i+3 < len(toks) &&
				toks[i+1].kind == tokIdent && toks[i+2].text == "("
		},
	},
	{
		name: "Positionsparameter eines Records", since: "9",
		message: "die Parameter werden nicht als Symbole erfasst",
		detect: func(toks []token, i int) bool {
			// record Name(  |  record struct Name(
			if toks[i].text != "record" || i+2 >= len(toks) {
				return false
			}
			j := i + 1
			if toks[j].text == "struct" || toks[j].text == "class" {
				j++
			}
			return j+2 < len(toks) && toks[j].kind == tokIdent && toks[j+1].text == "("
		},
	},
	{
		name: "file-lokaler Typ", since: "11",
		message: "der Typ wird erfasst und mit dem Modifikator file gespeichert, die Begrenzung auf die Datei wird aber nicht geprüft",
		detect: func(toks []token, i int) bool {
			if toks[i].text != "file" || i+1 >= len(toks) {
				return false
			}
			switch toks[i+1].text {
			case "class", "struct", "interface", "enum", "record":
				return true
			}
			return false
		},
	},
}

// versionWarnings sucht Konstrukte, die der Parser nur teilweise erfasst, und liefert Warnungen dafür.
func versionWarnings(toks []token) []parser.Diagnostic {
	var out []parser.Diagnostic
	for i := range toks {
		if toks[i].kind != tokIdent {
			continue
		}
		for _, pf := range partialFeatures {
			if pf.detect(toks, i) {
				out = append(out, parser.Diagnostic{
					Span:     model.Span{StartLine: toks[i].line, StartCol: toks[i].col, EndLine: toks[i].endLine, EndCol: toks[i].endCol},
					Message:  fmt.Sprintf("%s (C# %s): %s", pf.name, pf.since, pf.message),
					Severity: parser.SeverityWarning,
				})
				break
			}
		}
	}
	return out
}
