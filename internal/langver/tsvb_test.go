package langver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestESTargets(t *testing.T) {
	cases := map[string]string{
		"ES5": "5", "es6": "2015", "ES2020": "2020", "es2022": "2022",
		"ESNext": newestKnownES, "es3": "3", "bogus": "",
	}
	for raw, want := range cases {
		if got := esEffective(raw); got != want {
			t.Errorf("esEffective(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestTSConfigWithCommentsAndTrailingComma(t *testing.T) {
	// tsconfig.json erlaubt Kommentare und nachgestellte Kommas. Die Suche nach "target" funktioniert trotzdem.
	s := parseTSConfig("tsconfig.json", `{
  // Ziel
  "compilerOptions": { "target": "ES2022", },
}`)
	if s.Effective != "2022" || s.Raw != "ES2022" {
		t.Fatalf("Setting = %+v", s)
	}
}

func TestVBLangVersion(t *testing.T) {
	if s := parseVBProj("a.vbproj", `<Project><LangVersion>16.9</LangVersion></Project>`); s.Effective != "16.9" {
		t.Fatalf("16.9: %+v", s)
	}
	if s := parseVBProj("a.vbproj", `<Project><LangVersion>latest</LangVersion></Project>`); s.Effective != maxVBKnown || s.Note == "" {
		t.Fatalf("latest: %+v", s)
	}
	if s := parseVBProj("a.vbproj", `<Project></Project>`); s.Effective != "" || s.Note == "" {
		t.Fatalf("ohne Angabe darf nichts geraten werden: %+v", s)
	}
}

func TestWarningsForAllLanguages(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("web/tsconfig.json", `{"compilerOptions": {"target": "ESNext"}}`)
	write("web/tsconfig.app.json", `{"compilerOptions": {"target": "ES2020"}}`)
	write("svc/Svc.vbproj", `<Project><LangVersion>17.0</LangVersion></Project>`)
	write("api/Api.csproj", `<Project><TargetFramework>net10.0</TargetFramework></Project>`)

	w := strings.Join(Warnings(dir), "\n")
	for _, want := range []string{"web/tsconfig.json: ES 2024 ist neuer", "svc/Svc.vbproj: VB 17 ist neuer", "api/Api.csproj: C# 14 ist neuer"} {
		if !strings.Contains(w, want) {
			t.Errorf("Warnung fehlt: %q\nalle:\n%s", want, w)
		}
	}
	if strings.Contains(w, "tsconfig.app.json") {
		t.Errorf("ES2020 darf nicht warnen:\n%s", w)
	}
}
