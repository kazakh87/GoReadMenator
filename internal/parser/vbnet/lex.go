// Package vbnet parst VB.NET in Symbole, Fundorte, Verwendungen, Imports und Beziehungen.
//
// VB.NET ist zeilenorientiert: Anweisungen enden am Zeilenende oder an ":", Blöcke enden mit
// "End <Art>". Der Parser arbeitet deshalb nicht mit Klammern, sondern mit Anweisungen und
// einem Blockstapel. Groß- und Kleinschreibung spielt in VB keine Rolle, Namen bleiben aber
// im Originalformat erhalten.
package vbnet

import "strings"

type tokKind uint8

const (
	tokIdent tokKind = iota + 1
	tokPunct
	tokNumber
	tokString
)

// token ist ein Lexem. lower ist die Kleinschreibung für Schlüsselwörter. Spalten sind 1-basiert,
// endCol inklusiv.
type token struct {
	kind    tokKind
	text    string
	lower   string
	line    int
	col     int
	endLine int
	endCol  int
}

// stmt ist eine Anweisung: alle Tokens bis zum Zeilenende oder bis ":".
type stmt struct {
	toks []token
	line int
}

// docBlock ist ein Block aus ”' Zeilen (XML-Dokumentation). endLine ist die letzte Zeile.
type docBlock struct {
	startLine, endLine int
	lines              []string
}

// lexed ist das Ergebnis der Zerlegung einer Datei.
type lexed struct {
	stmts []stmt
	docs  []docBlock
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// lex zerlegt den Text in Anweisungen. Kommentare fallen weg, ”' Kommentare werden als Doku gesammelt.
func lex(src []byte) lexed {
	var out lexed
	var cur []token
	line := 1
	n := len(src)
	lineStart := 0
	continuing := false // Zeile endet mit " _": die Anweisung geht in der nächsten Zeile weiter
	flush := func(l int) {
		if len(cur) > 0 {
			out.stmts = append(out.stmts, stmt{toks: cur, line: l})
			cur = nil
		}
	}
	// addDoc hängt eine ''' Zeile an den letzten Block an, wenn sie direkt folgt.
	addDoc := func(l int, text string) {
		if k := len(out.docs) - 1; k >= 0 && out.docs[k].endLine == l-1 {
			out.docs[k].endLine = l
			out.docs[k].lines = append(out.docs[k].lines, text)
			return
		}
		out.docs = append(out.docs, docBlock{startLine: l, endLine: l, lines: []string{text}})
	}

	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '\n':
			if !continuing {
				flush(line)
			}
			continuing = false
			line++
			i++
			lineStart = i
		case c == '\r' || c == ' ' || c == '\t':
			i++
		case c == '\'':
			// Kommentar bis zum Zeilenende. ''' ist XML-Dokumentation.
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			if i+2 < n && src[i+1] == '\'' && src[i+2] == '\'' {
				addDoc(line, strings.TrimSpace(strings.TrimRight(string(src[i+3:j]), "\r")))
			}
			i = j
		case c == ':':
			flush(line)
			i++
		case c == '"':
			j := i + 1
			for j < n && src[j] != '\n' {
				if src[j] == '"' {
					if j+1 < n && src[j+1] == '"' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			end := j + 1
			if j >= n || src[j] != '"' {
				end = j // unterminiert: am Zeilenende beenden
			}
			value := strings.ReplaceAll(string(src[i+1:j]), `""`, `"`)
			cur = append(cur, token{kind: tokString, text: value, lower: value, line: line,
				col: i - lineStart + 1, endLine: line, endCol: end - lineStart})
			i = end
		case c == '[':
			// [Bezeichner] mit Klammern (Schlüsselwörter als Namen)
			j := i + 1
			for j < n && src[j] != ']' && src[j] != '\n' {
				j++
			}
			name := string(src[i+1 : j])
			cur = append(cur, token{kind: tokIdent, text: name, lower: strings.ToLower(name), line: line,
				col: i - lineStart + 1, endLine: line, endCol: j + 1 - lineStart})
			i = j + 1
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentPart(src[j]) {
				j++
			}
			word := string(src[i:j])
			// Typsuffixe wie & ! # @ gehören zum Namen, werden hier übersprungen
			if j < n && (src[j] == '&' || src[j] == '!' || src[j] == '#' || src[j] == '@' || src[j] == '%') {
				j++
			}
			// Zeilenfortsetzung: " _" am Zeilenende
			if strings.EqualFold(word, "rem") && (len(cur) == 0) {
				for j < n && src[j] != '\n' {
					j++
				}
				i = j
				continue
			}
			if word == "_" {
				continuing = true
				i = j
				continue
			}
			continuing = false
			cur = append(cur, token{kind: tokIdent, text: word, lower: strings.ToLower(word), line: line,
				col: i - lineStart + 1, endLine: line, endCol: j - lineStart})
			i = j
		case c >= '0' && c <= '9':
			j := i + 1
			for j < n && (isIdentPart(src[j]) || src[j] == '.') {
				j++
			}
			cur = append(cur, token{kind: tokNumber, text: string(src[i:j]), line: line,
				col: i - lineStart + 1, endLine: line, endCol: j - lineStart})
			i = j
		default:
			// Zeilenfortsetzung: Unterstrich als eigenes Zeichen gefolgt von Zeilenende
			if c == '_' {
				i++
				continue
			}
			txt := string(c)
			if i+1 < n && ((c == '<' && (src[i+1] == '>' || src[i+1] == '=')) || (c == '>' && src[i+1] == '=')) {
				txt = string(src[i : i+2])
			}
			cur = append(cur, token{kind: tokPunct, text: txt, lower: txt, line: line,
				col: i - lineStart + 1, endLine: line, endCol: i + len(txt) - lineStart})
			i += len(txt)
		}
	}
	flush(line)
	return out
}
