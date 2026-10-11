package langver

import (
	"os"
	"path/filepath"
	"testing"
)

// Jede Zeile: Ziel-Frameworks und LangVersion, und die erwartete C#-Version.
func TestEffectiveVersion(t *testing.T) {
	cases := []struct {
		name, tfm, lang, want string
	}{
		{"net8 ohne LangVersion", "<TargetFramework>net8.0</TargetFramework>", "", "12"},
		{"net9 ohne LangVersion", "<TargetFramework>net9.0</TargetFramework>", "", "13"},
		{"net10 ohne LangVersion", "<TargetFramework>net10.0</TargetFramework>", "", "14"},
		{"net48 Framework", "<TargetFramework>net48</TargetFramework>", "", "7.3"},
		{"netstandard2.0", "<TargetFramework>netstandard2.0</TargetFramework>", "", "7.3"},
		{"netstandard2.1", "<TargetFramework>netstandard2.1</TargetFramework>", "", "8"},
		{"mehrere Frameworks: höchster gewinnt", "<TargetFrameworks>net6.0;net8.0</TargetFrameworks>", "", "12"},
		{"LangVersion überschreibt", "<TargetFramework>net8.0</TargetFramework><LangVersion>11</LangVersion>", "", "11"},
		{"LangVersion 12.0 wird 12", "<TargetFramework>net8.0</TargetFramework><LangVersion>12.0</LangVersion>", "", "12"},
		{"latest zählt als neueste bekannte Version", "<TargetFramework>net8.0</TargetFramework><LangVersion>latest</LangVersion>", "", "14"},
		{"unbekanntes Framework", "<TargetFramework>net99.0</TargetFramework>", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := parseCSharpProject("x.csproj", "<Project>"+c.tfm+"</Project>")
			if p.Effective != c.want {
				t.Fatalf("Effective = %q, want %q (Note %q)", p.Effective, c.want, p.Note)
			}
		})
	}
}

func TestCheckWarnsOnlyWhenProjectIsNewer(t *testing.T) {
	newer := parseCSharpProject("neu.csproj", "<Project><TargetFramework>net10.0</TargetFramework></Project>")
	older := parseCSharpProject("alt.csproj", "<Project><TargetFramework>net8.0</TargetFramework></Project>")
	w := Check([]CSharpProject{newer, older}, "13")
	if len(w) != 1 || w[0][:7] != "neu.csp" {
		t.Fatalf("Warnungen = %v, erwartet genau eine für neu.csproj", w)
	}
	if got := Check([]CSharpProject{newer}, ""); got != nil {
		t.Fatalf("ohne Parser-Angabe darf nichts geprüft werden: %v", got)
	}
}

func TestScanSkipsBuildDirectories(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("App/App.csproj", "<Project><TargetFramework>net8.0</TargetFramework></Project>")
	write("App/bin/Debug/Old.csproj", "<Project><TargetFramework>net5.0</TargetFramework></Project>")
	projects, err := ScanCSharp(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].File != "App/App.csproj" {
		t.Fatalf("Projekte = %+v, erwartet nur App/App.csproj", projects)
	}
}
