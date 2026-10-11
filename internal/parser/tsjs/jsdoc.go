package tsjs

import (
	"regexp"
	"strings"

	"goreadmenator/internal/model"
)

var (
	reJSDocLink = regexp.MustCompile(`\{@(?:link|linkcode|linkplain)\s+([^}\s|]+)[^}]*\}`)
	reJSDocTag  = regexp.MustCompile(`(?m)^\s*@(\w+)\s*`)
)

type jsdocInfo struct {
	summary, remarks, returns string
	params                    []model.ParamDoc
	crefs                     []string
}

// parseJSDoc liest einen JSDoc-Block. Beschreibung vor dem ersten Tag ist die Zusammenfassung,
// `@param`, `@returns`/`@return`, `@remarks` und `@see`/`{@link}` werden ausgewertet.
func parseJSDoc(raw string) jsdocInfo {
	body := strings.TrimSuffix(strings.TrimPrefix(raw, "/**"), "*/")
	var lines []string
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(l, "*")
		lines = append(lines, strings.TrimSpace(l))
	}
	text := strings.Join(lines, "\n")

	var info jsdocInfo
	locs := reJSDocTag.FindAllStringSubmatchIndex(text, -1)
	descEnd := len(text)
	if len(locs) > 0 {
		descEnd = locs[0][0]
	}
	info.summary = collapse(text[:descEnd])

	for k, loc := range locs {
		tag := text[loc[2]:loc[3]]
		start := loc[1]
		end := len(text)
		if k+1 < len(locs) {
			end = locs[k+1][0]
		}
		content := strings.TrimSpace(text[start:end])
		switch tag {
		case "param", "arg", "argument":
			name, desc := splitParamName(content)
			if name != "" {
				info.params = append(info.params, model.ParamDoc{Name: name, Text: collapse(desc)})
			}
		case "returns", "return":
			info.returns = collapse(content)
		case "remarks":
			info.remarks = collapse(content)
		case "see":
			if name := strings.Fields(content); len(name) > 0 {
				info.crefs = append(info.crefs, name[0])
			}
		}
	}
	for _, m := range reJSDocLink.FindAllStringSubmatch(text, -1) {
		info.crefs = append(info.crefs, m[1])
	}
	info.crefs = dedupe(info.crefs)
	return info
}

// splitParamName trennt `{Typ} name Beschreibung` in Name und Beschreibung.
// Eckige Klammern um optionale Namen (`[name=default]`) werden entfernt.
func splitParamName(s string) (string, string) {
	if strings.HasPrefix(s, "{") {
		if i := strings.Index(s, "}"); i >= 0 {
			s = strings.TrimSpace(s[i+1:])
		}
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", ""
	}
	name := strings.Trim(fields[0], "[]")
	if i := strings.Index(name, "="); i >= 0 {
		name = name[:i]
	}
	return name, strings.Join(fields[1:], " ")
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
