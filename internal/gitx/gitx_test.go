package gitx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLsTree(t *testing.T) {
	data := []byte("100644 blob 1111111111111111111111111111111111111111       12\tsrc/A.cs\x00" +
		"120000 blob 2222222222222222222222222222222222222222        7\tlink.cs\x00" +
		"160000 commit 3333333333333333333333333333333333333333       -\tvendor/sub\x00" +
		"100644 blob 4444444444444444444444444444444444444444      100\tName with space.cs\x00")
	got, err := ParseLsTree(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("entries = %d", len(got))
	}
	if got[0].Path != "src/A.cs" || got[0].Size != 12 || got[0].Mode != ModeRegular {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].Mode != ModeSymlink {
		t.Errorf("entry 1 mode = %s", got[1].Mode)
	}
	if got[2].Size != -1 || got[2].Type != "commit" {
		t.Errorf("submodule entry = %+v", got[2])
	}
	if got[3].Path != "Name with space.cs" {
		t.Errorf("path with spaces = %q", got[3].Path)
	}
}

func TestParseLsTreeRejectsTruncated(t *testing.T) {
	if _, err := ParseLsTree([]byte("100644 blob abc 1\tfile")); err == nil {
		t.Fatal("erwartet Fehler bei fehlendem NUL-Abschluss")
	}
}

func TestHeadFailsOnEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	r := Runner{Dir: dir}
	if err := runGit(dir, "init", "-q"); err != nil {
		t.Skip("git nicht verfügbar:", err)
	}
	if _, err := r.Head(context.Background()); err == nil {
		t.Fatal("erwartet Fehler für Repo ohne Commit")
	}
}

func TestCatFileReadsBlobs(t *testing.T) {
	dir := t.TempDir()
	if err := runGit(dir, "init", "-q"); err != nil {
		t.Skip("git nicht verfügbar")
	}
	writeFile(t, dir, "a.txt", "hallo\nwelt\n")
	writeFile(t, dir, "b.txt", "")
	if err := runGit(dir, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(dir, "commit", "-q", "-m", "x"); err != nil {
		t.Fatal(err)
	}
	r := Runner{Dir: dir}
	entries, err := r.ListTree(context.Background(), "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := r.OpenCatFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cf.Close()
	for _, e := range entries {
		got, err := cf.Blob(e.BlobSHA)
		if err != nil {
			t.Fatalf("%s: %v", e.Path, err)
		}
		want := map[string]string{"a.txt": "hallo\nwelt\n", "b.txt": ""}[e.Path]
		if string(got) != want {
			t.Errorf("%s: inhalt %q, want %q", e.Path, got, want)
		}
	}
	// Mehrere Abfragen hintereinander über denselben Prozess.
	for i := 0; i < 1000; i++ {
		if _, err := cf.Blob(entries[0].BlobSHA); err != nil {
			t.Fatalf("wiederholte Abfrage %d: %v", i, err)
		}
	}
	if _, err := cf.Blob("0000000000000000000000000000000000000000"); err == nil {
		t.Fatal("fehlender Blob muss Fehler liefern")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseDiffRaw(t *testing.T) {
	sha := "1111111111111111111111111111111111111111"
	data := []byte(":100644 100644 0000000000000000000000000000000000000000 " + sha + " A\x00new.cs\x00" +
		":100644 100644 " + sha + " 0000000000000000000000000000000000000000 D\x00gone.cs\x00" +
		":100644 100644 " + sha + " " + sha + " R087\x00old name.cs\x00renamed.cs\x00" +
		":100644 100644 " + sha + " " + sha + " M\x00mod.ts\x00")
	got, err := ParseDiffRaw(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("Einträge = %d", len(got))
	}
	if got[0].Status != 'A' || got[0].Path != "new.cs" || got[0].NewSHA != sha {
		t.Errorf("A: %+v", got[0])
	}
	if got[1].Status != 'D' || got[1].NewSHA != "" || got[1].Path != "gone.cs" {
		t.Errorf("D: %+v", got[1])
	}
	if got[2].Status != 'R' || got[2].OldPath != "old name.cs" || got[2].Path != "renamed.cs" {
		t.Errorf("R: %+v", got[2])
	}
	if got[3].Path != "mod.ts" || got[3].Status != 'M' {
		t.Errorf("M: %+v", got[3])
	}
}

func TestParseHunks(t *testing.T) {
	data := []byte(`diff --git a/a.cs b/a.cs
--- a/a.cs
+++ b/a.cs
@@ -10 +10,2 @@ class A
-old
+new1
+new2
@@ -20,2 +21,0 @@
-gone1
-gone2
diff --git a/gone.cs b/gone.cs
--- a/gone.cs
+++ /dev/null
@@ -1,3 +0,0 @@
`)
	got := ParseHunks(data)
	if r := got["a.cs"]; len(r) != 2 || r[0] != (LineRange{10, 11}) || r[1] != (LineRange{21, 21}) {
		t.Fatalf("Hunks a.cs = %+v", r)
	}
	if _, ok := got["gone.cs"]; ok {
		t.Fatal("gelöschte Datei darf keine Bereiche liefern")
	}
}
