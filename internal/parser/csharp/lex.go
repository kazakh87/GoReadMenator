package csharp

import "sync"

type tokKind uint8

const (
	tokIdent tokKind = iota + 1
	tokPunct
	tokNumber
)

// token ist ein Lexem mit Position. Spalten sind 1-basierte Byte-Offsets,
// endCol ist inklusiv.
type token struct {
	kind    tokKind
	text    string
	line    int
	col     int
	endLine int
	endCol  int
}

// multiPunct sind Operatoren mit mehr als einem Zeichen. `>>` fehlt absichtlich,
// damit verschachtelte Generics (`List<List<int>>`) einzeln geschlossen werden.
var multiPunct = []string{
	"??=", "<<=", "=>", "==", "!=", "<=", ">=", "&&", "||", "++", "--",
	"??", "?.", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=",
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '@' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lex zerlegt maskierten Quelltext in Tokens.
func lex(src []byte) []token {
	toks := getTokBuf()
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
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
			lineStart = i
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, token{kind: tokIdent, text: text(src[i:j]),
				line: line, col: i - lineStart + 1, endLine: line, endCol: j - lineStart})
			i = j
		case isDigit(c):
			j := i + 1
			for j < n && (isIdentPart(src[j]) || src[j] == '.') {
				j++
			}
			toks = append(toks, token{kind: tokNumber, text: text(src[i:j]),
				line: line, col: i - lineStart + 1, endLine: line, endCol: j - lineStart})
			i = j
		default:
			txt := punctText[c]
			for _, p := range multiPunct {
				if hasPrefixAt(src, i, p) {
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

// punctText hält Einzelzeichen-Operatoren als Strings (keine Allokation je Zeichen).
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
