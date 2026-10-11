// Package csharp parst C#-Quelltext in Symbole, Fundorte, Verwendungen und Beziehungen.
//
// Der Parser arbeitet in drei Stufen: Maskieren (Kommentare, Strings und
// Präprozessor-Zeilen werden durch Leerzeichen ersetzt, Positionen bleiben
// erhalten), Tokenisieren und rekursives Auswerten der Deklarationen. Eine
// Typauflösung über Namensräume gibt es hier noch nicht. Das folgt in P8.
package csharp

import "sync"

import "strings"

// docBlock ist ein zusammenhängender Block aus `///`-Zeilen.
type docBlock struct {
	startLine, endLine int
	lines              []string
}

type maskResult struct {
	masked []byte
	docs   []docBlock
	strs   []cstr
}

// cstr ist ein Stringliteral mit Startposition und Inhalt. Der Inhalt wird für Routen gebraucht.
type cstr struct {
	off   int
	value string
}

// mask ersetzt Kommentare, Strings, Zeichenliterale und Präprozessor-Zeilen
// durch Leerzeichen. Zeilenumbrüche bleiben, damit Zeilen- und Spaltenangaben
// im Quelltext stimmen. `///`-Kommentare werden zusätzlich gesammelt.
func mask(src []byte) maskResult {
	out := getMaskBuf(len(src))
	copy(out, src)
	n := len(src)
	var docs []docBlock
	var strs []cstr
	line := 1
	atLineStart := true

	blank := func(a, b int) {
		if b > n {
			b = n
		}
		for k := a; k < b; k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}

	i := 0
	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			line++
			atLineStart = true
			i++
			continue
		case c == ' ' || c == '\t' || c == '\r':
			i++
			continue
		case c == '#' && atLineStart:
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j
			continue
		}
		atLineStart = false

		switch {
		case c == '/' && i+1 < n && src[i+1] == '/':
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			// `///`, aber nicht `////`
			if i+2 < n && src[i+2] == '/' && !(i+3 < n && src[i+3] == '/') {
				docs = addDoc(docs, line, strings.TrimSpace(string(src[i+3:j])))
			}
			blank(i, j)
			i = j

		case c == '/' && i+1 < n && src[i+1] == '*':
			j := i + 2
			for j < n && !(src[j] == '*' && j+1 < n && src[j+1] == '/') {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			blank(i, j+2)
			i = j + 2

		case c == '"' && i+2 < n && src[i+1] == '"' && src[i+2] == '"':
			// Rohstring: """ ... """ (Anzahl der Anführungszeichen muss passen)
			start := i
			q := 0
			for i+q < n && src[i+q] == '"' {
				q++
			}
			j := i + q
			for j < n {
				if src[j] == '\n' {
					line++
					j++
					continue
				}
				if src[j] == '"' {
					k := 0
					for j+k < n && src[j+k] == '"' {
						k++
					}
					j += k
					if k >= q {
						break
					}
					continue
				}
				j++
			}
			blank(start, j)
			i = j

		case c == '"':
			start := i
			verbatim := start > 0 && src[start-1] == '@'
			j := i + 1
			for j < n {
				ch := src[j]
				if verbatim {
					if ch == '\n' {
						line++
					}
					if ch == '"' {
						if j+1 < n && src[j+1] == '"' {
							j += 2
							continue
						}
						j++
						break
					}
					j++
					continue
				}
				if ch == '\\' {
					j += 2
					continue
				}
				if ch == '\n' { // unterminierter String: nicht über die Zeile hinaus maskieren
					break
				}
				if ch == '"' {
					j++
					break
				}
				j++
			}
			if j-1 > start && src[j-1] == '"' && !verbatim {
				strs = append(strs, cstr{off: start, value: string(src[start+1 : j-1])})
			}
			blank(start, j)
			i = j

		case c == '\'':
			start := i
			j := i + 1
			if j < n && src[j] == '\\' {
				j += 2
			} else {
				j++
			}
			if j < n && src[j] == '\'' {
				j++
			}
			blank(start, j)
			i = j

		default:
			i++
		}
	}
	return maskResult{masked: out, docs: docs, strs: strs}
}

// addDoc hängt eine `///`-Zeile an den letzten Block an, wenn sie direkt darauf folgt.
func addDoc(docs []docBlock, line int, text string) []docBlock {
	if k := len(docs) - 1; k >= 0 && docs[k].endLine == line-1 {
		docs[k].endLine = line
		docs[k].lines = append(docs[k].lines, text)
		return docs
	}
	return append(docs, docBlock{startLine: line, endLine: line, lines: []string{text}})
}

// countCodeLines zählt Zeilen mit mindestens einem Nicht-Leerzeichen im maskierten Text.
func countCodeLines(masked []byte) int {
	count := 0
	has := false
	for _, c := range masked {
		switch c {
		case '\n':
			if has {
				count++
			}
			has = false
		case ' ', '\t', '\r':
		default:
			has = true
		}
	}
	if has {
		count++
	}
	return count
}

// maskPool wiederverwendet die maskierte Kopie des Quelltexts. Sie wird nach dem Parsen nicht mehr gebraucht.
var maskPool = sync.Pool{New: func() any { b := make([]byte, 0, 64<<10); return &b }}

func getMaskBuf(n int) []byte {
	b := *maskPool.Get().(*[]byte)
	if cap(b) < n {
		return make([]byte, n)
	}
	return b[:n]
}

func putMaskBuf(b []byte) {
	if cap(b) > 8<<20 {
		return
	}
	b = b[:0]
	maskPool.Put(&b)
}
