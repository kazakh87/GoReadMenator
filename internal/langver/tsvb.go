package langver

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"goreadmenator/internal/parser/csharp"
	"goreadmenator/internal/parser/tsjs"
	"goreadmenator/internal/parser/vbnet"
)

// Setting ist die aus einem Projekt gelesene Sprachversion einer Sprache, ohne C#-spezifische Felder.
type Setting struct {
	File      string // repo-relativer Pfad der Projektdatei
	Raw       string // Wert wie im Projekt geschrieben
	Effective string // vergleichbare Version, z. B. "2022" oder "16.9"; leer wenn unbekannt
	Note      string
}

// ES-Versionen werden als Jahr verglichen: ES2015 ist 2015. Die alten Nummern werden umgerechnet.
var esNumber = map[string]string{
	"3": "3", "5": "5", "6": "2015", "7": "2016", "8": "2017", "9": "2018", "10": "2019",
	"11": "2020", "12": "2021", "13": "2022", "14": "2023", "15": "2024",
}

// newestKnownES ist das neueste ES-Jahr, das diese Prüfung kennt. "esnext" zählt darauf.
const newestKnownES = "2024"

var (
	reTarget = regexp.MustCompile(`(?i)"target"\s*:\s*"([^"]+)"`)
	reVBLang = regexp.MustCompile(`<LangVersion>([^<]+)</LangVersion>`)
)

// ScanTypeScript sucht tsconfig.json-Dateien und liest den target-Wert. extends wird nicht aufgelöst,
// das ist als Hinweis vermerkt.
func ScanTypeScript(repo string) ([]Setting, error) {
	return scan(repo, func(name string) bool { return name == "tsconfig.json" }, parseTSConfig)
}

// ScanVisualBasic sucht .vbproj-Dateien und liest LangVersion. Ohne Angabe wird keine Version geraten.
func ScanVisualBasic(repo string) ([]Setting, error) {
	return scan(repo, func(name string) bool { return strings.EqualFold(filepath.Ext(name), ".vbproj") }, parseVBProj)
}

func scan(repo string, match func(string) bool, parse func(file, content string) Setting) ([]Setting, error) {
	var out []Setting
	err := filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", "obj", "node_modules":
				if p != repo {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !match(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repo, p)
		out = append(out, parse(filepath.ToSlash(rel), string(data)))
		return nil
	})
	return out, err
}

func parseTSConfig(file, content string) Setting {
	s := Setting{File: file}
	m := reTarget.FindStringSubmatch(content)
	if m == nil {
		s.Note = "kein target gesetzt (TypeScript-Standard)"
		return s
	}
	s.Raw = m[1]
	s.Effective = esEffective(m[1])
	if s.Effective == "" {
		s.Note = "target " + m[1] + " nicht erkannt"
	}
	return s
}

// esEffective bringt "es2020", "ES2020", "es6", "esnext" auf eine vergleichbare Jahreszahl.
func esEffective(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "esnext" {
		return newestKnownES
	}
	v = strings.TrimPrefix(v, "es")
	if y, ok := esNumber[v]; ok {
		return y
	}
	if len(v) == 4 && v[0] == '2' {
		return v
	}
	return ""
}

func parseVBProj(file, content string) Setting {
	s := Setting{File: file}
	m := reVBLang.FindStringSubmatch(content)
	if m == nil {
		s.Note = "LangVersion nicht gesetzt, Sprachversion nicht prüfbar"
		return s
	}
	s.Raw = strings.TrimSpace(m[1])
	switch strings.ToLower(s.Raw) {
	case "latest", "preview":
		s.Effective = maxVBKnown
		s.Note = fmt.Sprintf("LangVersion %s: wird als VB %s angenommen", s.Raw, maxVBKnown)
	default:
		s.Effective = normalizeVersion(s.Raw)
		if s.Effective == "" {
			s.Note = "LangVersion " + s.Raw + " nicht erkannt"
		}
	}
	return s
}

// maxVBKnown ist die neueste VB-Version, die diese Prüfung kennt.
const maxVBKnown = "16.9"

// CheckSettings vergleicht die gelesenen Versionen mit der Abdeckung eines Parsers und liefert Warnungen.
// Ein leeres maxVersion bedeutet: keine Prüfung. label nennt die Sprache in der Meldung.
func CheckSettings(label string, settings []Setting, maxVersion string) []string {
	if maxVersion == "" {
		return nil
	}
	var warnings []string
	for _, s := range settings {
		if s.Note != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", s.File, s.Note))
		}
		if s.Effective != "" && cmpVersion(s.Effective, maxVersion) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"%s: %s %s ist neuer als der Parser (bis %s). Konstrukte dieser Version werden möglicherweise nicht erfasst.",
				s.File, label, s.Effective, maxVersion))
		}
	}
	return warnings
}

// Warnings liefert alle Versionshinweise eines Repositories für C#, TypeScript und VB.NET. Sprachen ohne
// gefundene Projektdatei erzeugen nichts. Jede Sprache wird nur gegen die Abdeckung ihres Parsers geprüft.
func Warnings(repo string) []string {
	var out []string
	cs, _ := ScanCSharp(repo)
	out = append(out, Check(cs, csharp.MaxLanguageVersion)...)
	ts, _ := ScanTypeScript(repo)
	out = append(out, CheckSettings("ES", ts, tsjs.MaxLanguageVersion)...)
	vb, _ := ScanVisualBasic(repo)
	out = append(out, CheckSettings("VB", vb, vbnet.MaxLanguageVersion)...)
	return out
}
