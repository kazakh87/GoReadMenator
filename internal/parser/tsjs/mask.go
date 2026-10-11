// Package tsjs parst TypeScript und JavaScript (inkl. JSX in .tsx/.jsx) in Symbole,
// Fundorte, Verwendungen, Imports und Beziehungen.
//
// Wie der C#-Parser arbeitet er in drei Stufen: Maskieren, Tokenisieren und
// Auswerten. Zusätzlich gibt es Besonderheiten: Template-Literale mit
// Interpolation, Regex-Literale (Unterscheidung von Division über den
// vorherigen Token) und String-Inhalte, die für Import-Pfade gebraucht werden.
package tsjs

import "sync"

// docBlock ist ein JSDoc-Kommentar (/** ... */) mit Start- und Endzeile.
type docBlock struct {
	startLine, endLine int
	lines              []string
}

// strSpan beschreibt ein String-Literal. start zeigt auf das öffnende Anführungszeichen,
// end ist exklusiv, value ist der Inhalt ohne Anführungszeichen (ohne Escape-Auflösung).
type strSpan struct {
	start, end int
	value      string
}

type maskResult struct {
	// templates enthält den Rohtext interpolierter Templates, Schlüssel ist die Position des Backticks.
	// Das Template bleibt als Code im Token-Strom, für URL-Muster wird der Rohtext hier gebraucht.
	templates map[int]tmplSpan
	masked    []byte
	docs      []docBlock
	strs      []strSpan
}

// regexAllowed entscheidet anhand des letzten Code-Zeichens, ob ein `/` ein Regex-Literal beginnt.
func regexAllowed(sig byte, word string) bool {
	switch sig {
	case 0:
		return true
	case '(', ',', '=', ':', '[', '!', '&', '|', '?', '{', '}', ';', '+', '-', '*', '%', '<', '>', '~', '^':
		return true
	case 'a':
		switch word {
		case "return", "typeof", "case", "in", "of", "delete", "void", "throw", "yield", "await", "else", "do":
			return true
		}
	}
	return false
}

// mask ersetzt Kommentare, Strings, Template-Text und Regex-Literale durch Leerzeichen.
// Ausdrücke in `${...}` bleiben sichtbar. Zeilenumbrüche bleiben erhalten.
func mask(src []byte) maskResult {
	n := len(src)
	out := getMaskBuf(n)
	copy(out, src)
	var docs []docBlock
	var strs []strSpan
	var templates map[int]tmplSpan
	line := 1

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

	// scanTemplate maskiert Template-Text ab start. Liefert den Index nach dem Ende
	// und ob bei "${" angehalten wurde. Das "${" selbst bleibt sichtbar.
	scanTemplate := func(start int) (int, bool) {
		j := start
		for j < n {
			c := src[j]
			if c == '\n' {
				line++
			}
			if c == '\\' {
				j += 2
				continue
			}
			if c == '`' {
				blank(start, j+1)
				return j + 1, false
			}
			if c == '$' && j+1 < n && src[j+1] == '{' {
				blank(start, j)
				return j + 2, true
			}
			j++
		}
		blank(start, n)
		return n, false
	}

	var tmpl []int // Klammertiefe je offenem ${...}
	var lastSig byte
	lastWord := ""

	i := 0
	if n >= 2 && src[0] == '#' && src[1] == '!' {
		j := 0
		for j < n && src[j] != '\n' {
			j++
		}
		blank(0, j)
		i = j
	}

	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++

		case c == ' ' || c == '\t' || c == '\r':
			i++

		case c == '/' && i+1 < n && src[i+1] == '/':
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j

		case c == '/' && i+1 < n && src[i+1] == '*':
			j := i + 2
			startLine := line
			for j < n && !(src[j] == '*' && j+1 < n && src[j+1] == '/') {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			end := j + 2
			if end > n {
				end = n
			}
			if i+2 < n && src[i+2] == '*' && !(i+3 < n && src[i+3] == '/') {
				docs = append(docs, docBlock{startLine: startLine, endLine: line, lines: []string{string(src[i:end])}})
			}
			blank(i, end)
			i = end

		case (c == '"' || c == '\'') && !(i > 0 && isIdentPart(src[i-1])):
			// Ein Anführungszeichen direkt hinter einem Bezeichner ist im Code ungültig, also ein
			// Apostroph im JSX-Text ("Don't"). Es darf keinen String öffnen.
			j := i + 1
			for j < n && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j > n {
				j = n
			}
			value := string(src[i+1 : j])
			end := j
			if end < n && src[end] == c {
				end++
			}
			strs = append(strs, strSpan{start: i, end: end, value: value})
			blank(i, end)
			i = end
			lastSig = 'v'

		case c == '`':
			blank(i, i+1)
			j, interp := scanTemplate(i + 1)
			// Der Rohtext des Templates (mit ${...}) bleibt für URL-Muster erhalten.
			if interp {
				if raw, end, ok := rawTemplate(src, i); ok {
					if templates == nil {
						templates = map[int]tmplSpan{}
					}
					templates[i] = tmplSpan{end: end, raw: raw}
				}
			}
			if j < n && src[j] == '`' {
				// j zeigt auf das schließende Zeichen: die Span endet dahinter
				strs = append(strs, strSpan{start: i, end: j + 1, value: string(src[i+1 : j])})
				j++
			} else if j-1 > i && src[j-1] == '`' {
				strs = append(strs, strSpan{start: i, end: j, value: string(src[i+1 : j-1])})
			}
			if interp {
				tmpl = append(tmpl, 0)
				lastSig = '{'
			} else {
				lastSig = 'v'
			}
			i = j

		case c == '{':
			if len(tmpl) > 0 {
				tmpl[len(tmpl)-1]++
			}
			lastSig = '{'
			i++

		case c == '}':
			if len(tmpl) > 0 {
				top := len(tmpl) - 1
				if tmpl[top] == 0 {
					tmpl = tmpl[:top]
					j, interp := scanTemplate(i + 1)
					if interp {
						tmpl = append(tmpl, 0)
						lastSig = '{'
					} else {
						lastSig = 'v'
					}
					i = j
					continue
				}
				tmpl[top]--
			}
			lastSig = '}'
			i++

		case c == '/' && regexAllowed(lastSig, lastWord) && regexEnd(src, i) > 0:
			// Regex-Literal nur, wenn es auf derselben Zeile abgeschlossen wird. Sonst
			// ist es Division oder JSX-Schließtag und darf keine Klammern verschlucken.
			j := regexEnd(src, i)
			for j < n && isIdentPart(src[j]) {
				j++
			}
			blank(i, j)
			i = j
			lastSig = 'v'

		case isIdentStart(c) || (c == '#' && i+1 < n && isIdentStart(src[i+1])):
			j := i + 1
			for j < n && isIdentPart(src[j]) {
				j++
			}
			lastWord = string(src[i:j])
			lastSig = 'a'
			i = j

		case isDigit(c):
			j := i + 1
			for j < n && (isIdentPart(src[j]) || src[j] == '.') {
				j++
			}
			lastSig = 'v'
			i = j

		default:
			lastSig = c
			lastWord = ""
			i++
		}
	}
	return maskResult{masked: out, docs: docs, strs: strs, templates: templates}
}

// regexEnd liefert den Index nach dem abschließenden `/` eines Regex-Literals ab i, oder -1.
func regexEnd(src []byte, i int) int {
	n := len(src)
	j := i + 1
	inClass := false
	for j < n && src[j] != '\n' {
		ch := src[j]
		switch {
		case ch == '\\':
			j += 2
			continue
		case ch == '[':
			inClass = true
		case ch == ']':
			inClass = false
		case ch == '/' && !inClass:
			if j == i+1 {
				return -1 // "//" ist ein Kommentar, kein Regex
			}
			return j + 1
		}
		j++
	}
	return -1
}

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

// rawTemplate liest ein Template ab dem Backtick bei i bis zum schließenden Backtick und liefert den Text dazwischen,
// mit den Ausdrücken ${...} unverändert. Verschachtelte Backticks in Ausdrücken werden übersprungen.
func rawTemplate(src []byte, i int) (string, int, bool) {
	depth := 0
	for j := i + 1; j < len(src); j++ {
		switch {
		case src[j] == '\\':
			j++
		case src[j] == '$' && j+1 < len(src) && src[j+1] == '{':
			depth++
			j++
		case src[j] == '}' && depth > 0:
			depth--
		case src[j] == '`' && depth == 0:
			return string(src[i+1 : j]), j + 1, true
		case src[j] == '`' && depth > 0:
			// verschachteltes Template im Ausdruck: bis zu seinem Ende überspringen
			if k, ok := rawTemplateEnd(src, j); ok {
				j = k
			}
		}
	}
	return "", 0, false
}

// tmplSpan ist ein interpoliertes Template: Rohtext und Position hinter dem schließenden Backtick.
type tmplSpan struct {
	end int
	raw string
}

func rawTemplateEnd(src []byte, i int) (int, bool) {
	for j := i + 1; j < len(src); j++ {
		if src[j] == '\\' {
			j++
			continue
		}
		if src[j] == '`' {
			return j, true
		}
	}
	return 0, false
}
