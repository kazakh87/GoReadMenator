package razor

import (
	"bytes"
	"path"
	"regexp"
	"strings"
)

// razorDirectives sind Razor-Direktiven, die über ihre Zeile ausgewertet werden.
var razorDirectives = map[string]bool{"using": true, "model": true, "inject": true, "inherits": true,
	"page": true, "layout": true, "namespace": true, "attribute": true, "implements": true,
	"rendermode": true, "preservewhitespace": true, "typeparam": true}

// razorRegion durchsucht den Bereich [start, end) der Vorlage. codeMode bedeutet: der Text ist C#,
// außer Zeilen, die mit einem Tag beginnen (Markup). Im HTML-Modus wird nur Code hinter @ erfasst.
// Erzeugter Code geht in bucket.
func (g *gen) razorRegion(start, end int, codeMode bool, bucket *[]sline) {
	src := g.src
	runStart := -1
	flush := func(upto int) {
		if runStart >= 0 && upto > runStart {
			line, col := g.pos(runStart)
			g.emit(bucket, string(src[runStart:upto]), line, col)
		}
		runStart = -1
	}
	at := func(i int) byte {
		if i < len(src) {
			return src[i]
		}
		return 0
	}
	i := start
	for i < end {
		c := src[i]
		if c == '@' {
			next := at(i + 1)
			if codeMode && (next == '"' || isIdentStart(next) || next == '@') {
				// C#-eigene @-Syntax (@"...", @keyword): gehört zum Code
				if runStart < 0 {
					runStart = i
				}
				i += 2
				continue
			}
			if next == '@' { // @@ ist ein Literal-@ im HTML
				i += 2
				continue
			}
			flush(i)
			switch {
			case next == '*':
				j := bytes.Index(src[i+2:], []byte("*@"))
				if j < 0 {
					i = end
				} else {
					i = i + 2 + j + 2
				}
			case next == '{':
				e := g.readBalanced(i+1, '{', '}')
				g.razorRegion(i+2, e-1, true, bucketBody(bucket, g))
				i = e
			case next == '(':
				e := g.readBalanced(i+1, '(', ')')
				line, col := g.pos(i + 2)
				g.emit(bucket, string(src[i+2:e-1])+";", line, col)
				i = e
			case isIdentStart(next):
				j := g.identEnd(i + 1)
				word := string(src[i+1 : j])
				switch {
				case word == "code" || word == "functions":
					k := skipWhite(src, j)
					if at(k) == '{' {
						e := g.readBalanced(k, '{', '}')
						g.razorRegion(k+1, e-1, true, &g.members)
						i = e
					} else {
						i = j
					}
				case razorDirectives[word]:
					i = g.directive(word, g.skipSpaces(j), end, &g.members)
				case controlWords[word]:
					i = g.controlBlock(word, i, j, end, codeMode, bucket)
				case htmlWords[word]:
					i = g.controlBlock(word, i, j, end, false, bucket)
				default:
					k := j
					for k < len(src) {
						if src[k] == '.' && isIdentStart(at(k+1)) {
							k = g.identEnd(k + 1)
						} else if src[k] == '(' {
							k = g.readBalanced(k, '(', ')')
						} else if src[k] == '[' {
							k = g.readBalanced(k, '[', ']')
						} else {
							break
						}
					}
					line, col := g.pos(i + 1)
					g.emit(bucket, string(src[i+1:k])+";", line, col)
					i = k
				}
			default:
				i++
			}
			continue
		}
		if codeMode {
			// Markup: eine Zeile, die mit einem Tag beginnt, ist HTML, auch im Code-Block
			if (c == '<') && onlySpacesBefore(src, i) && (isIdentStart(at(i+1)) || at(i+1) == '/' || at(i+1) == '!') {
				flush(i)
				e := g.lineEnd(i)
				g.razorRegion(i, e, false, bucket)
				i = e
				continue
			}
			if c == '"' || c == '\'' {
				if runStart < 0 {
					runStart = i
				}
				i = skipQuoted(src, i) + 1
				continue
			}
			if runStart < 0 {
				runStart = i
			}
		}
		i++
	}
	flush(end)
}

// bucketBody liefert den Körper-Bereich, in den Code aus @{...}-Blöcken geschrieben wird.
func bucketBody(bucket *[]sline, g *gen) *[]sline {
	return &g.body
}

// controlBlock verarbeitet @if (...) { ... } und ähnliche. Kopf und schließende Klammer werden
// als Code ausgegeben, der Körper wird im Modus codeMode bzw. HTML durchsucht.
func (g *gen) controlBlock(word string, at, afterWord, end int, codeMode bool, bucket *[]sline) int {
	src := g.src
	k := afterWord
	if skipWhite(src, k) < len(src) && src[skipWhite(src, k)] == '(' {
		k = g.readBalanced(skipWhite(src, k), '(', ')')
	}
	open := skipWhite(src, k)
	if open >= end || open >= len(src) || src[open] != '{' {
		return afterWord
	}
	closeAt := g.readBalanced(open, '{', '}')
	headLine, headCol := g.pos(at + 1)
	head := strings.TrimSpace(string(src[at+1 : open]))
	g.emit(bucket, head+" {", headLine, headCol)
	g.razorRegion(open+1, closeAt-1, codeMode, bucket)
	endLine, endCol := g.pos(closeAt - 1)
	g.emit(bucket, "}", endLine, endCol)
	return closeAt
}

// skipWhite überspringt Leerraum einschließlich Zeilenumbrüchen.
func skipWhite(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\r' || src[i] == '\n') {
		i++
	}
	return i
}

// onlySpacesBefore meldet, ob vor i in derselben Zeile nur Leerraum steht.
func onlySpacesBefore(src []byte, i int) bool {
	for j := i - 1; j >= 0 && src[j] != '\n'; j-- {
		if src[j] != ' ' && src[j] != '\t' {
			return false
		}
	}
	return true
}

var reAspScript = regexp.MustCompile(`(?i)<script[^>]*runat\s*=\s*"server"[^>]*>`)

// aspxRegion durchsucht eine ASPX-Seite: <% %>, <%= %>, <%@ %>, <%-- --%> und <script runat="server">.
func (g *gen) aspxRegion() {
	src := g.src
	n := len(src)
	i := 0
	for i < n {
		switch {
		case bytes.HasPrefix(src[i:], []byte("<%--")):
			j := bytes.Index(src[i:], []byte("--%>"))
			if j < 0 {
				i = n
			} else {
				i += j + 4
			}
		case bytes.HasPrefix(src[i:], []byte("<%@")):
			e := bytes.Index(src[i:], []byte("%>"))
			if e < 0 {
				i = n
				break
			}
			text := string(src[i+3 : i+e])
			line, _ := g.pos(i)
			if m := reAspImport.FindStringSubmatch(text); m != nil {
				emitLine(&g.usings, "using "+m[1]+";", line)
			}
			if m := reAspInherits.FindStringSubmatch(text); m != nil {
				// Bei ASPX ist Inherits die Klasse der Seite selbst, also die partielle Klasse aus dem
				// Code-behind. Die Basisklasse ist System.Web.UI.Page und wird nicht angegeben.
				g.classFQN = m[1]
			}
			i += e + 2
		case bytes.HasPrefix(src[i:], []byte("<%=")):
			e := bytes.Index(src[i:], []byte("%>"))
			if e < 0 {
				i = n
				break
			}
			line, col := g.pos(i + 3)
			g.emit(&g.body, strings.TrimSpace(string(src[i+3:i+e]))+";", line, col)
			i += e + 2
		case bytes.HasPrefix(src[i:], []byte("<%")):
			e := bytes.Index(src[i:], []byte("%>"))
			if e < 0 {
				i = n
				break
			}
			line, col := g.pos(i + 2)
			g.emit(&g.body, string(src[i+2:i+e]), line, col)
			i += e + 2
		case src[i] == '<' && bytes.HasPrefix(bytes.ToLower(src[i:min(i+30, n)]), []byte("<script")):
			loc := reAspScript.FindIndex(src[i:])
			if loc == nil || loc[0] != 0 {
				i++
				break
			}
			contentStart := i + loc[1]
			e := bytes.Index(bytes.ToLower(src[contentStart:]), []byte("</script>"))
			if e < 0 {
				i = n
				break
			}
			line, col := g.pos(contentStart)
			g.emit(&g.members, string(src[contentStart:contentStart+e]), line, col)
			i = contentStart + e + len("</script>")
		default:
			i++
		}
	}
}

// build setzt die C#-Datei zusammen und liefert sie mit der Zuordnung Zeile → Vorlagenzeile.
func (g *gen) build(p string) (string, []int) {
	var lines []sline
	add := func(text string, orig int) { lines = append(lines, sline{text, orig}) }
	for _, u := range g.usings {
		add(u.text, u.orig)
	}
	ns, cls := namespaceAndClass(p)
	if g.classFQN != "" {
		ns, cls = splitFQN(g.classFQN)
	}
	g.fqn = cls
	if ns != "" {
		g.fqn = ns + "." + cls
	}
	if ns != "" {
		add("namespace "+ns+" {", 1)
	}
	head := "partial class " + cls
	if g.base != "" {
		head += " : " + g.base
	}
	add(head+" {", 1)
	lines = append(lines, g.members...)
	add("private void __Render() {", 1)
	lines = append(lines, g.body...)
	add("}", 1)
	add("}", 1)
	if ns != "" {
		add("}", 1)
	}
	var b strings.Builder
	orig := make([]int, len(lines)+1)
	for i, l := range lines {
		b.WriteString(l.text)
		b.WriteByte('\n')
		orig[i+1] = l.orig
	}
	return b.String(), orig
}

// namespaceAndClass leitet aus dem Pfad den Namensraum (Ordner) und den Klassennamen (Datei) ab.
func namespaceAndClass(p string) (string, string) {
	p = strings.ReplaceAll(p, "\\", "/")
	dir, file := path.Split(p)
	base := strings.TrimSuffix(file, path.Ext(file))
	var parts []string
	for _, d := range strings.Split(strings.Trim(dir, "/"), "/") {
		if d != "" && d != "." {
			parts = append(parts, identifier(d))
		}
	}
	return strings.Join(parts, "."), identifier(base)
}

// identifier macht aus einem Namen einen gültigen C#-Bezeichner.
func identifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || (out[0] >= '0' && out[0] <= '9') {
		out = "_" + out // Bezeichner dürfen nicht mit Ziffer beginnen
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// splitFQN trennt einen vollen Typnamen in Namensraum und Klassennamen.
func splitFQN(fqn string) (string, string) {
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		return fqn[:i], fqn[i+1:]
	}
	return "", fqn
}
