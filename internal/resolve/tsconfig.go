package resolve

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// tsConfig enthält die für Modulauflösung nötigen Angaben aus tsconfig.json.
type tsConfig struct {
	baseURL string // relativ zum Repository-Root, bereits bereinigt
	paths   []pathMapping
}

type pathMapping struct {
	prefix, suffix string // Muster "prefix*suffix" oder exakter Name (star=false)
	star           bool
	targets        []string
}

var reTrailingComma = regexp.MustCompile(`,(\s*[}\]])`)

// loadTSConfig liest tsconfig.json im Repository-Root. Fehlt die Datei oder ist sie
// unlesbar, liefert die Funktion nil. Modulauflösung läuft dann ohne Pfad-Zuordnung.
func loadTSConfig(repoDir string) *tsConfig {
	if repoDir == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(repoDir, "tsconfig.json"))
	if err != nil {
		return nil
	}
	var doc struct {
		CompilerOptions struct {
			BaseURL string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	clean := reTrailingComma.ReplaceAllString(stripJSONComments(string(raw)), "$1")
	if err := json.Unmarshal([]byte(clean), &doc); err != nil {
		return nil
	}
	cfg := &tsConfig{baseURL: path.Clean(strings.TrimPrefix(doc.CompilerOptions.BaseURL, "./"))}
	if cfg.baseURL == "." {
		cfg.baseURL = ""
	}
	for pat, targets := range doc.CompilerOptions.Paths {
		m := pathMapping{targets: targets}
		if i := strings.Index(pat, "*"); i >= 0 {
			m.star = true
			m.prefix = pat[:i]
			m.suffix = pat[i+1:]
		} else {
			m.prefix = pat
		}
		cfg.paths = append(cfg.paths, m)
	}
	return cfg
}

// candidates liefert Dateipfade, die ein Modulname laut paths/baseUrl meinen kann.
func (c *tsConfig) candidates(spec string) []string {
	var out []string
	for _, m := range c.paths {
		var rest string
		switch {
		case m.star && strings.HasPrefix(spec, m.prefix) && strings.HasSuffix(spec, m.suffix) &&
			len(spec) >= len(m.prefix)+len(m.suffix):
			rest = spec[len(m.prefix) : len(spec)-len(m.suffix)]
		case !m.star && spec == m.prefix:
			rest = ""
		default:
			continue
		}
		for _, t := range m.targets {
			sub := strings.Replace(t, "*", rest, 1)
			out = append(out, path.Clean(path.Join(c.baseURL, strings.TrimPrefix(sub, "./"))))
		}
	}
	if len(out) == 0 && c.baseURL != "" {
		out = append(out, path.Clean(path.Join(c.baseURL, spec)))
	}
	return out
}

// stripJSONComments entfernt // und /* */-Kommentare außerhalb von Strings.
// tsconfig.json erlaubt Kommentare, encoding/json nicht.
func stripJSONComments(s string) string {
	var b strings.Builder
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			b.WriteByte(c)
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
