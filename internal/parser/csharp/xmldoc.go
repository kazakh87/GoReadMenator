package csharp

import (
	"html"
	"regexp"
	"strings"

	"goreadmenator/internal/model"
)

var (
	reCref    = regexp.MustCompile(`cref="([^"]+)"`)
	reSeeCref = regexp.MustCompile(`<see\s+cref="(?:[A-Z]:)?([^"]+)"\s*/?>`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reSpace   = regexp.MustCompile(`\s+`)
	reParam   = regexp.MustCompile(`(?s)<param\s+name="([^"]+)"\s*>(.*?)</param>`)
	reSummary = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)
	reRemarks = regexp.MustCompile(`(?s)<remarks>(.*?)</remarks>`)
	reReturns = regexp.MustCompile(`(?s)<returns>(.*?)</returns>`)
)

type docInfo struct {
	summary, remarks, returns string
	params                    []model.ParamDoc
	crefs                     []string
}

// parseDocBlock liest die XML-Dokumentation eines `///`-Blocks.
// Die Auswertung ist bewusst tolerant. Unvollständiges XML führt nicht zu Fehlern.
func parseDocBlock(b docBlock) docInfo {
	text := strings.Join(b.lines, "\n")
	var info docInfo

	seen := map[string]bool{}
	for _, m := range reCref.FindAllStringSubmatch(text, -1) {
		name := crefName(m[1])
		if !seen[name] {
			seen[name] = true
			info.crefs = append(info.crefs, name)
		}
	}

	info.summary = cleanDocText(firstGroup(reSummary, text))
	info.remarks = cleanDocText(firstGroup(reRemarks, text))
	info.returns = cleanDocText(firstGroup(reReturns, text))
	for _, m := range reParam.FindAllStringSubmatch(text, -1) {
		info.params = append(info.params, model.ParamDoc{Name: m[1], Text: cleanDocText(m[2])})
	}
	return info
}

func firstGroup(re *regexp.Regexp, text string) string {
	if m := re.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// crefName entfernt Präfixe wie `T:` oder `M:` aus einem cref-Wert.
func crefName(s string) string {
	if len(s) > 2 && s[1] == ':' {
		s = s[2:]
	}
	// Doku-IDs schreiben Generics mit geschweiften Klammern: Processor{T} -> Processor<T>
	return strings.NewReplacer("{", "<", "}", ">").Replace(s)
}

// cleanDocText ersetzt <see cref> und <seealso cref> durch den Namen, entfernt übrige Tags,
// löst XML-Entities auf (&amp; wird zu &) und normalisiert Leerraum.
func cleanDocText(s string) string {
	s = reSeeCref.ReplaceAllString(s, "$1")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
}
