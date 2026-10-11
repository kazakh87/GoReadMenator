package tsjs

import "sync"

type tokKind uint8

const (
	tokIdent tokKind = iota + 1
	tokPunct
	tokNumber
	tokString
)

// token ist ein Lexem mit Position. Spalten sind 1-basierte Byte-Offsets, endCol inklusiv.
// Bei tokString ist text der Inhalt des Strings ohne Anführungszeichen.
type token struct {
	kind    tokKind
	text    string
	line    int
	col     int
	endLine int
	endCol  int
}

var multiPunct = []string{
	"...", "===", "!==", "**=", "??=", "&&=", "||=", "=>", "==", "!=", "<=", ">=",
	"&&", "||", "??", "?.", "++", "--", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "**",
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lex zerlegt den maskierten Quelltext. Strings werden anhand der Spans aus dem
// Original-Inhalt eingesetzt, weil das Maskieren ihren Inhalt entfernt hat.
func lex(masked []byte, strs []strSpan) []token {
	toks := getTokBuf()
	// Bezeichner und Operatoren wiederholen sich stark. Gleiche Texte teilen sich einen String,
	// das spart den größten Teil der Allokationen beim Tokenisieren.
	intern := map[string]string{}
	text := func(b []byte) string {
		if s, ok := intern[string(b)]; ok {
			return s
		}
		s := string(b)
		intern[s] = s
		return s
	}
	line, lineStart := 1, 0
	n := len(masked)
	si := 0
	for i := 0; i < n; {
		if si < len(strs) && i == strs[si].start {
			s := strs[si]
			toks = append(toks, token{kind: tokString, text: s.value,
				line: line, col: i - lineStart + 1, endLine: line, endCol: s.end - lineStart})
			i = s.end
			si++
			continue
		}
		c := masked[i]
		switch {
		case c == '\n':
			line++
			i++
			lineStart = i
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
		case isIdentStart(c) || (c == '#' && i+1 < n && isIdentStart(masked[i+1])):
			j := i + 1
			for j < n && isIdentPart(masked[j]) {
				j++
			}
			toks = append(toks, token{kind: tokIdent, text: text(masked[i:j]),
				line: line, col: i - lineStart + 1, endLine: line, endCol: j - lineStart})
			i = j
		case isDigit(c):
			j := i + 1
			for j < n && (isIdentPart(masked[j]) || masked[j] == '.') {
				j++
			}
			toks = append(toks, token{kind: tokNumber, text: text(masked[i:j]),
				line: line, col: i - lineStart + 1, endLine: line, endCol: j - lineStart})
			i = j
		default:
			txt := punctText[c]
			for _, p := range multiPunct {
				if hasPrefixAt(masked, i, p) {
					txt = p
					break
				}
			}
			j := i + len(txt)
			toks = append(toks, token{kind: tokPunct, text: txt,
				line: line, col: i - lineStart + 1, endLine: line, endCol: j - lineStart})
			i = j
		}
	}
	return toks
}

// punctText hält die Einzelzeichen-Operatoren als Strings, damit das Tokenisieren keinen
// String pro Zeichen erzeugt.
var punctText = func() [256]string {
	var t [256]string
	for i := range t {
		t[i] = string([]byte{byte(i)})
	}
	return t
}()

func hasPrefixAt(src []byte, i int, p string) bool {
	if i+len(p) > len(src) {
		return false
	}
	return string(src[i:i+len(p)]) == p
}

// tokPool wiederverwendet Token-Slices zwischen Dateien. Parser-Ergebnisse enthalten keine
// Verweise auf dieses Slice (nur Strings und Werte), es kann also nach dem Parsen zurück.
var tokPool = sync.Pool{New: func() any { s := make([]token, 0, 4096); return &s }}

func getTokBuf() []token {
	return (*tokPool.Get().(*[]token))[:0]
}

// putTokBuf gibt ein Slice zurück. Große Slices werden verworfen, damit der Pool nicht wächst.
func putTokBuf(t []token) {
	if cap(t) > 1<<20 {
		return
	}
	t = t[:0]
	tokPool.Put(&t)
}
