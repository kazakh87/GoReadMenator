// Package razor parst Razor-Dateien (.cshtml, .razor) und ASPX-Seiten (.aspx, .ascx, .master).
//
// Die Vorlage ist HTML mit eingebettetem C#. Der Parser baut daraus eine C#-Datei, die nur den Code
// enthält, und lässt den C#-Parser darauf laufen. Der HTML-Teil wird weggelassen. Jede Codezeile
// behält ihre Zeilennummer und ihre Spalte aus der Originaldatei, damit Fundorte stimmen.
// Zeilen, die nicht aus der Vorlage stammen (Klassenkopf, Methodenkopf), zeigen auf Zeile 1.
package razor

import (
	"regexp"
	"sort"
	"strings"
)

// sline ist eine Zeile der erzeugten C#-Datei. orig ist die Zeile in der Vorlage.
type sline struct {
	text string
	orig int
}

// gen sammelt die erzeugten Zeilen in drei Bereichen.
type gen struct {
	src        []byte
	lineStarts []int // Byteposition des Zeilenanfangs, 0-basiert
	usings     []sline
	members    []sline
	body       []sline
	base       string
	classFQN   string // ASPX: voller Name der Seitenklasse (aus Inherits)
	pages      []pageRoute
	fqn        string // voller Name der erzeugten Klasse, gesetzt von build
}

// pageRoute ist eine @page-Direktive mit ihrem Pfad und ihrer Zeile.
type pageRoute struct {
	path string
	line int
}

func newGen(src []byte) *gen {
	g := &gen{src: src, lineStarts: []int{0}}
	for i, c := range src {
		if c == '\n' {
			g.lineStarts = append(g.lineStarts, i+1)
		}
	}
	return g
}

// pos liefert Zeile und Spalte (1-basiert, Bytes) einer Position in der Vorlage.
func (g *gen) pos(i int) (int, int) {
	k := sort.Search(len(g.lineStarts), func(k int) bool { return g.lineStarts[k] > i }) - 1
	return k + 1, i - g.lineStarts[k] + 1
}

// emit schreibt Code, der bei Zeile line und Spalte col der Vorlage beginnt, in den Bereich.
// Die erste Zeile wird auf die Spalte eingerückt, Folgezeilen beginnen in Spalte 1 der Vorlage.
func (g *gen) emit(bucket *[]sline, code string, line, col int) {
	code = strings.ReplaceAll(code, "\r", "")
	if strings.TrimSpace(code) == "" {
		return
	}
	for i, part := range strings.Split(code, "\n") {
		if i == 0 {
			part = strings.Repeat(" ", col-1) + part
		}
		*bucket = append(*bucket, sline{text: part, orig: line + i})
	}
}

// emitLine fügt eine Zeile ohne Vorlagenbezug hinzu, sie zeigt auf die angegebene Zeile.
func emitLine(bucket *[]sline, text string, orig int) {
	*bucket = append(*bucket, sline{text: text, orig: orig})
}

// readBalanced liest ab i (dort steht das öffnende Zeichen) bis zum passenden schließenden Zeichen.
// Strings und Zeichenliterale werden übersprungen. Liefert die Position nach dem Schließen.
func (g *gen) readBalanced(i int, open, close byte) int {
	depth := 0
	for j := i; j < len(g.src); j++ {
		c := g.src[j]
		switch {
		case c == '"' || c == '\'':
			j = skipQuoted(g.src, j)
		case c == open:
			depth++
		case c == close:
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return len(g.src)
}

// skipQuoted überspringt einen String oder Zeichenliteral ab der Anführungszeichenposition und liefert
// die Position des letzten Zeichens davon.
func skipQuoted(src []byte, i int) int {
	q := src[i]
	for j := i + 1; j < len(src); j++ {
		if src[j] == '\\' {
			j++
			continue
		}
		if src[j] == q {
			return j
		}
		if src[j] == '\n' && q == '\'' {
			return j
		}
	}
	return len(src) - 1
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// identAt liefert das Ende des Bezeichners ab i (i zeigt auf den ersten Buchstaben).
func (g *gen) identEnd(i int) int {
	j := i
	for j < len(g.src) && isIdentPart(g.src[j]) {
		j++
	}
	return j
}

func (g *gen) skipSpaces(i int) int {
	for i < len(g.src) && (g.src[i] == ' ' || g.src[i] == '\t') {
		i++
	}
	return i
}

// lineEnd liefert die Position des Zeilenendes ab i.
func (g *gen) lineEnd(i int) int {
	for i < len(g.src) && g.src[i] != '\n' {
		i++
	}
	return i
}

// controlWords sind Razor-Blöcke mit C#-Code im Körper.
var controlWords = map[string]bool{"if": true, "for": true, "foreach": true, "while": true, "switch": true,
	"using": true, "try": true, "lock": true}

// htmlWords sind Razor-Blöcke mit HTML im Körper.
var htmlWords = map[string]bool{"section": true}

// directive behandelt Razor-Direktiven ab dem Wort. Liefert die Position nach der Direktive.
func (g *gen) directive(word string, i, end int, members *[]sline) int {
	line, _ := g.pos(i)
	val := strings.TrimSpace(string(g.src[i:g.lineEnd(i)]))
	switch word {
	case "using":
		emitLine(&g.usings, "using "+strings.TrimSpace(val)+";", line)
		return g.lineEnd(i)
	case "model":
		emitLine(members, "public "+strings.TrimSpace(val)+" Model;", line)
		return g.lineEnd(i)
	case "inject":
		parts := strings.Fields(val)
		if len(parts) >= 2 {
			emitLine(members, "public "+parts[0]+" "+parts[1]+";", line)
		}
		return g.lineEnd(i)
	case "inherits":
		if f := strings.Fields(val); len(f) > 0 {
			g.base = f[0]
		}
		return g.lineEnd(i)
	case "page":
		// @page "/pfad" macht die Seite über diesen Pfad erreichbar
		if m := rePageRoute.FindStringSubmatch(val); m != nil {
			g.pages = append(g.pages, pageRoute{path: m[1], line: line})
		}
		return g.lineEnd(i)
	}
	return g.lineEnd(i) // übrige Direktiven (page, layout, namespace, attribute, ...) werden nicht ausgewertet
}

var rePageRoute = regexp.MustCompile(`^"([^"]*)"`)

var reAspImport = regexp.MustCompile(`Namespace\s*=\s*"([^"]+)"`)
var reAspInherits = regexp.MustCompile(`Inherits\s*=\s*"([^"]+)"`)
