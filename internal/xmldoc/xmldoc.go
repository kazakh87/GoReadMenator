// Package xmldoc importiert die XML-Dokumentationsdatei eines .NET-Assemblys
// (erzeugt beim Build mit GenerateDocumentationFile). Damit lässt sich ein Index
// aufbauen, wenn nur DLL und XML vorliegen und der Quelltext fehlt.
//
// Grenzen: Die XML-Datei kennt nur die öffentlich und geschützt sichtbaren Member
// mit Dokumentation. Die Art eines Typs (Klasse, Interface, Struktur, Enum) steht
// nicht darin. Dafür braucht es die Metadaten der DLL, das ist ein eigener Schritt.
// Beziehungen wie Vererbung oder Feldtypen fehlen deshalb in diesem Modus.
package xmldoc

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	"goreadmenator/internal/model"
)

// Member ist ein Eintrag aus der Dokumentationsdatei.
type Member struct {
	Name      string // Dokumentations-ID, etwa "M:Ns.Typ.Methode(System.Int32)"
	Summary   string
	Remarks   string
	Returns   string
	Params    []model.ParamDoc
	TypeParam []model.ParamDoc
	Crefs     []string
}

// Doc ist die geparste Datei.
type Doc struct {
	Assembly string
	Members  []Member
}

type rawMember struct {
	Name    string `xml:"name,attr"`
	Summary struct {
		Inner string `xml:",innerxml"`
	} `xml:"summary"`
	Remarks struct {
		Inner string `xml:",innerxml"`
	} `xml:"remarks"`
	Returns struct {
		Inner string `xml:",innerxml"`
	} `xml:"returns"`
	Params []struct {
		Name  string `xml:"name,attr"`
		Inner string `xml:",innerxml"`
	} `xml:"param"`
	TypeParams []struct {
		Name  string `xml:"name,attr"`
		Inner string `xml:",innerxml"`
	} `xml:"typeparam"`
}

// Parse liest eine XML-Dokumentationsdatei. Die Verarbeitung ist streamend.
func Parse(r io.Reader) (Doc, error) {
	var doc Doc
	dec := xml.NewDecoder(r)
	inAssembly := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return doc, fmt.Errorf("XML-Dokumentation: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "assembly":
			inAssembly = true
		case "name":
			if inAssembly && doc.Assembly == "" {
				var name string
				if err := dec.DecodeElement(&name, &se); err != nil {
					return doc, err
				}
				doc.Assembly = strings.TrimSpace(name)
			}
		case "member":
			var rm rawMember
			if err := dec.DecodeElement(&rm, &se); err != nil {
				return doc, fmt.Errorf("member %q: %w", rm.Name, err)
			}
			doc.Members = append(doc.Members, toMember(rm))
		}
	}
	return doc, nil
}

var (
	reSeeCref = regexp.MustCompile(`<see\s+(?:cref|langword)="(?:[A-Za-z]:)?([^"]+)"\s*/?>`)
	reSeeAlso = regexp.MustCompile(`<seealso\s+cref="(?:[A-Za-z]:)?([^"]+)"\s*/?>`)
	reTags    = regexp.MustCompile(`<[^>]+>`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

func toMember(rm rawMember) Member {
	m := Member{Name: rm.Name}
	m.Summary = cleanText(rm.Summary.Inner)
	m.Remarks = cleanText(rm.Remarks.Inner)
	m.Returns = cleanText(rm.Returns.Inner)
	seen := map[string]bool{}
	for _, raw := range []string{rm.Summary.Inner, rm.Remarks.Inner, rm.Returns.Inner} {
		for _, c := range reSeeCref.FindAllStringSubmatch(raw, -1) {
			addOnce(&m.Crefs, seen, c[1])
		}
		for _, c := range reSeeAlso.FindAllStringSubmatch(raw, -1) {
			addOnce(&m.Crefs, seen, c[1])
		}
	}
	for _, p := range rm.Params {
		m.Params = append(m.Params, model.ParamDoc{Name: p.Name, Text: cleanText(p.Inner)})
	}
	for _, p := range rm.TypeParams {
		m.TypeParam = append(m.TypeParam, model.ParamDoc{Name: p.Name, Text: cleanText(p.Inner)})
	}
	return m
}

func addOnce(list *[]string, seen map[string]bool, v string) {
	if !seen[v] {
		seen[v] = true
		*list = append(*list, v)
	}
}

// cleanText ersetzt <see cref> durch den Namen, entfernt übrige Tags, löst Entities und Leerraum auf.
func cleanText(inner string) string {
	s := reSeeCref.ReplaceAllString(inner, "$1")
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
}

// Symbol-ID-Präfixe der Dokumentations-IDs.
const (
	prefixType     = "T:"
	prefixMethod   = "M:"
	prefixProperty = "P:"
	prefixField    = "F:"
	prefixEvent    = "E:"
	prefixNS       = "N:"
)

// Symbols wandelt die Members in Symbole des Index um. Jede Doku-ID entspricht einer
// C#-Symbol-ID: Typen über den FQN (Generics mit `N`), Member über FQN plus Parametertypen.
// Namensräume entstehen aus den Typen, weil die Datei sie nicht gesondert auflistet.
// Die Dokumentation wird als Quelle "xml" markiert.
func Symbols(doc Doc) []model.Symbol {
	typeHeads := map[string]bool{}
	for _, m := range doc.Members {
		if strings.HasPrefix(m.Name, prefixType) {
			typeHeads[m.Name[2:]] = true
		}
	}
	var out []model.Symbol
	nsSeen := map[string]bool{}
	for _, m := range doc.Members {
		s, ok := toSymbol(m)
		if !ok {
			continue
		}
		out = append(out, s)
		// Übergeordneten Namensraum anlegen, wenn der Container kein Typ der Datei ist.
		if strings.HasPrefix(m.Name, prefixType) {
			parent := parentType(m.Name[2:])
			if parent != "" && !typeHeads[parent] && !nsSeen[parent] {
				nsSeen[parent] = true
				out = append(out, model.Symbol{
					ID: model.NewSymbolID(model.LangCSharp, parent, nil), Kind: model.KindNamespace,
					Name: lastSegmentOf(parent), FQN: parent, DocSource: "xml",
				})
			}
		}
	}
	return out
}

// toSymbol bildet eine Dokumentations-ID auf ein Symbol ab.
func toSymbol(m Member) (model.Symbol, bool) {
	if len(m.Name) < 3 || m.Name[1] != ':' {
		return model.Symbol{}, false
	}
	prefix, rest := m.Name[:2], m.Name[2:]

	// Parameterliste abtrennen. Der Kopf enthält Namen mit Punkten, die Liste steht in Klammern.
	head := rest
	var params []string
	hasParams := false
	if i := strings.IndexByte(rest, '('); i >= 0 && strings.HasSuffix(rest, ")") {
		head = rest[:i]
		params = splitParams(rest[i+1 : len(rest)-1])
		hasParams = true
	}

	var kind model.SymbolKind
	var container, name string
	switch prefix {
	case prefixType:
		kind = model.KindTypeUnknown
		container = parentType(head)
		name = head[len(container)+boolToInt(container != ""):]
	case prefixMethod:
		kind = model.KindMethod
		container, name = splitMember(head)
		if name == "#ctor" || name == "#cctor" {
			kind = model.KindConstructor
			name = lastTypeSegment(container)
		}
	case prefixProperty:
		kind = model.KindProperty
		container, name = splitMember(head)
		if name == "Item" && hasParams {
			kind = model.KindIndexer
			name = "this"
		}
	case prefixField:
		kind = model.KindField
		container, name = splitMember(head)
	case prefixEvent:
		kind = model.KindEvent
		container, name = splitMember(head)
	default:
		return model.Symbol{}, false
	}

	fqn := head
	if prefix != prefixType {
		fqn = joinFQN(container, name)
	}
	idParams := params
	if prefix == prefixType || prefix == prefixField || prefix == prefixEvent {
		idParams = nil
	}
	if (prefix == prefixMethod || prefix == prefixProperty) && !hasParams {
		idParams = nil
		if prefix == prefixMethod {
			idParams = []string{} // Methode ohne Parameter: leere Liste, ID mit "()"
		}
	}
	id := model.NewSymbolID(model.LangCSharp, fqn, idParams)
	if kind == model.KindIndexer {
		fqn = joinFQN(container, "this")
		id = model.NewSymbolID(model.LangCSharp, fqn, idParams)
	}
	s := model.Symbol{
		ID: id, Kind: kind, Name: name, FQN: fqn,
		Summary: m.Summary, Remarks: m.Remarks, Returns: m.Returns,
		ParamDocs: m.Params, DocSource: "xml",
	}
	if prefix == prefixType {
		s.Name = name
	}
	if container != "" {
		s.Container = model.NewSymbolID(model.LangCSharp, container, nil)
	}
	return s, true
}

// splitMember trennt "Ns.Typ.Member" in Container und Member. Das ist der letzte Punkt.
func splitMember(head string) (string, string) {
	if i := strings.LastIndexByte(head, '.'); i >= 0 {
		return head[:i], head[i+1:]
	}
	return "", head
}

func joinFQN(container, name string) string {
	if container == "" {
		return name
	}
	return container + "." + name
}

// parentType liefert den übergeordneten Typ bei verschachtelten Typen. Namensräume zählen nicht,
// daher bleibt das Ergebnis bei Typen ohne Verschachtelung leer, außer der Namensraum selbst.
func parentType(head string) string {
	if i := strings.LastIndexByte(head, '.'); i >= 0 {
		return head[:i]
	}
	return ""
}

func lastTypeSegment(fqn string) string {
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		fqn = fqn[i+1:]
	}
	if i := strings.IndexByte(fqn, '`'); i >= 0 {
		fqn = fqn[:i]
	}
	return fqn
}

// Hinweis: Generic-Suffixe wie `2 bleiben im FQN erhalten, wie in der Quelle.
func lastSegmentOf(s string) string {
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// splitParams trennt die Parameterliste an Kommas auf Klammertiefe 0 (Generics wie `Dictionary{A,B}` nutzen geschweifte Klammern in Doku-IDs).
func splitParams(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '<', '(', '[':
			depth++
		case '}', '>', ')', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, normalizeDocType(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, normalizeDocType(s[start:]))
	return out
}

// normalizeDocType übersetzt die Schreibweise der Dokumentations-IDs in die der Symbol-IDs:
// geschweifte Klammern werden zu spitzen, Array-Ränge `[0:,0:]` zu `[]`.
func normalizeDocType(t string) string {
	t = strings.ReplaceAll(t, "{", "<")
	t = strings.ReplaceAll(t, "}", ">")
	var b strings.Builder
	for {
		i := strings.IndexByte(t, '[')
		if i < 0 {
			break
		}
		j := strings.IndexByte(t[i:], ']')
		if j < 0 {
			break
		}
		b.WriteString(t[:i])
		b.WriteString("[]")
		t = t[i+j+1:]
	}
	b.WriteString(t)
	return strings.TrimSpace(b.String())
}

// IDOf liefert die Symbol-ID eines Members, dessen Dokumentations-ID gültig ist.
func IDOf(m Member) (model.SymbolID, bool) {
	s, ok := toSymbol(m)
	return s.ID, ok
}
