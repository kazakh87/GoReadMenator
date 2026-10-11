// Package langver liest die Sprachversion eines Projekts aus den Projektdateien und prüft sie gegen
// die Abdeckung der Parser. Die Prüfung warnt nur. Sie bricht nichts ab.
package langver

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// CSharpProject beschreibt eine .csproj-Datei und die daraus abgeleitete C#-Version.
type CSharpProject struct {
	File       string // repo-relativer Pfad
	Frameworks []string
	LangRaw    string // Inhalt von LangVersion, leer wenn nicht gesetzt
	Effective  string // angenommene C#-Version, z. B. "12" oder "7.3"; leer wenn unbekannt
	Note       string // Hinweis, z. B. bei "latest"
}

var (
	reTFM        = regexp.MustCompile(`<TargetFrameworks?>([^<]+)</TargetFrameworks?>`)
	reLangVer    = regexp.MustCompile(`<LangVersion>([^<]+)</LangVersion>`)
	reNetCore    = regexp.MustCompile(`^net(\d+)\.(\d+)`)
	reNetFW      = regexp.MustCompile(`^net(\d)(\d*)$`)
	reNetCoreOld = regexp.MustCompile(`^netcoreapp(\d+)\.(\d+)$`)
	reNetStd     = regexp.MustCompile(`^netstandard(\d+)\.(\d+)$`)
)

// newestKnown ist die neueste C#-Version, die diese Prüfung kennt. "latest" und "preview" zählen darauf.
const newestKnown = "14"

// ScanCSharp durchsucht das Repository nach .csproj-Dateien. Verzeichnisse wie .git oder bin werden übersprungen.
func ScanCSharp(repo string) ([]CSharpProject, error) {
	var out []CSharpProject
	err := filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unlesbare Verzeichnisse überspringen
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
		if !strings.EqualFold(filepath.Ext(p), ".csproj") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repo, p)
		out = append(out, parseCSharpProject(filepath.ToSlash(rel), string(data)))
		return nil
	})
	return out, err
}

// parseCSharpProject leitet die C#-Version aus Inhalt einer .csproj-Datei ab.
func parseCSharpProject(file, content string) CSharpProject {
	p := CSharpProject{File: file}
	if m := reTFM.FindStringSubmatch(content); m != nil {
		for _, f := range strings.Split(m[1], ";") {
			if f = strings.TrimSpace(f); f != "" {
				p.Frameworks = append(p.Frameworks, f)
			}
		}
	}
	if m := reLangVer.FindStringSubmatch(content); m != nil {
		p.LangRaw = strings.TrimSpace(m[1])
	}
	switch strings.ToLower(p.LangRaw) {
	case "latest", "preview":
		p.Effective = newestKnown
		p.Note = fmt.Sprintf("LangVersion %s: wird als C# %s angenommen", p.LangRaw, newestKnown)
		return p
	case "", "default":
		p.Effective = ""
		for _, tfm := range p.Frameworks {
			if v := defaultForTFM(tfm); v != "" && (p.Effective == "" || cmpVersion(v, p.Effective) > 0) {
				p.Effective = v
			}
		}
		if p.Effective == "" {
			p.Note = "Sprachversion nicht ableitbar (TargetFramework unbekannt)"
		}
		return p
	}
	p.Effective = normalizeVersion(p.LangRaw)
	if p.Effective == "" {
		p.Note = "LangVersion " + p.LangRaw + " nicht erkannt"
	}
	return p
}

// defaultForTFM liefert die Standard-C#-Version eines Ziel-Frameworks.
func defaultForTFM(tfm string) string {
	tfm = strings.ToLower(strings.TrimSpace(tfm))
	if m := reNetCore.FindStringSubmatch(tfm); m != nil {
		major, _ := strconv.Atoi(m[1])
		return netMajorToCSharp(major)
	}
	if m := reNetCoreOld.FindStringSubmatch(tfm); m != nil {
		if m[1] == "2" {
			return "7.3"
		}
		return "8"
	}
	if m := reNetStd.FindStringSubmatch(tfm); m != nil {
		if m[1] == "2" && m[2] == "1" {
			return "8"
		}
		return "7.3"
	}
	if m := reNetFW.FindStringSubmatch(tfm); m != nil {
		return "7.3" // .NET Framework: Standard C# 7.3
	}
	return ""
}

func netMajorToCSharp(major int) string {
	switch major {
	case 5:
		return "9"
	case 6:
		return "10"
	case 7:
		return "11"
	case 8:
		return "12"
	case 9:
		return "13"
	case 10:
		return "14"
	}
	if major > 10 {
		return "" // unbekannt: nicht raten
	}
	return ""
}

// normalizeVersion bringt "12.0" auf "12", lässt "7.3" stehen.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasSuffix(v, ".0") {
		v = strings.TrimSuffix(v, ".0")
	}
	if _, err := strconv.ParseFloat(v, 64); err != nil {
		return ""
	}
	return v
}

// cmpVersion vergleicht zwei Versionen wie "7.3" und "12".
func cmpVersion(a, b string) int {
	fa, _ := strconv.ParseFloat(a, 64)
	fb, _ := strconv.ParseFloat(b, 64)
	switch {
	case fa < fb:
		return -1
	case fa > fb:
		return 1
	}
	return 0
}

// Check vergleicht die Projekte mit der Abdeckung des Parsers (maxVersion, z. B. "13") und liefert Warnungen.
// Ein leeres maxVersion bedeutet: der Parser macht keine Angabe, es gibt nichts zu prüfen.
func Check(projects []CSharpProject, maxVersion string) []string {
	if maxVersion == "" {
		return nil
	}
	var warnings []string
	for _, p := range projects {
		if p.Note != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", p.File, p.Note))
		}
		if p.Effective != "" && cmpVersion(p.Effective, maxVersion) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"%s: C# %s ist neuer als der Parser (bis C# %s). Konstrukte dieser Version werden möglicherweise nicht erfasst.",
				p.File, p.Effective, maxVersion))
		}
	}
	return warnings
}
