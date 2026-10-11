// Package gitx kapselt den Zugriff auf das Git-Repository über das git-Binary.
// Es wird kein Shell verwendet. Argumente gehen direkt an exec, damit keine
// Pfade oder Refs interpretiert werden.
package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Mode-Werte aus `git ls-tree` für Einträge, die der Scanner unterscheiden muss.
const (
	ModeRegular    = "100644"
	ModeExecutable = "100755"
	ModeSymlink    = "120000"
	ModeGitlink    = "160000" // Submodul
)

// TreeEntry ist ein Eintrag im Tree eines Commits.
type TreeEntry struct {
	Path    string // relativ zur Repo-Wurzel, mit /-Trennern
	Mode    string
	Type    string // "blob", "tree", "commit"
	BlobSHA string
	Size    int64 // -1, wenn unbekannt (kein Blob)
}

// Runner führt git-Kommandos in einem Repository aus.
type Runner struct {
	Dir string // Repo-Wurzel, per `git -C` übergeben
}

func (r Runner) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"-C", r.Dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Head liefert den vollständigen Hash des aktuellen HEAD-Commits.
func (r Runner) Head(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ListTree liefert alle Einträge des Trees eines Commits, rekursiv.
// Es werden nur getrackte Dateien gelistet. Dadurch greifen .gitignore und
// ähnliche Regeln automatisch, ohne dass Verzeichnisnamen fest verdrahtet werden.
func (r Runner) ListTree(ctx context.Context, commit string) ([]TreeEntry, error) {
	out, err := r.run(ctx, "ls-tree", "-r", "-l", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	return ParseLsTree(out)
}

// ParseLsTree zerlegt die Ausgabe von `git ls-tree -r -l -z`.
// Format pro Eintrag: "<mode> SP <type> SP <sha> SP <size>\t<path>\0"
func ParseLsTree(data []byte) ([]TreeEntry, error) {
	var entries []TreeEntry
	for len(data) > 0 {
		i := bytes.IndexByte(data, 0)
		if i < 0 {
			return nil, fmt.Errorf("ls-tree: Eintrag ohne Abschluss")
		}
		rec := data[:i]
		data = data[i+1:]

		tab := bytes.IndexByte(rec, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("ls-tree: fehlendes Tab in %q", rec)
		}
		meta := strings.Fields(string(rec[:tab]))
		if len(meta) != 4 {
			return nil, fmt.Errorf("ls-tree: unerwartete Metadaten %q", rec[:tab])
		}
		size := int64(-1)
		if meta[3] != "-" {
			v, err := strconv.ParseInt(meta[3], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("ls-tree: ungültige Größe %q", meta[3])
			}
			size = v
		}
		entries = append(entries, TreeEntry{
			Path:    string(rec[tab+1:]),
			Mode:    meta[0],
			Type:    meta[1],
			BlobSHA: meta[2],
			Size:    size,
		})
	}
	return entries, nil
}

// Change ist ein Eintrag aus `git diff --raw`.
type Change struct {
	Status  byte   // 'A', 'M', 'D', 'R', 'C', 'T'
	Path    string // neuer Pfad (bei Umbenennung der neue)
	OldPath string // alter Pfad bei R und C, sonst leer
	NewSHA  string // Blob-SHA im neuen Stand, leer bei D
}

// CommitExists prüft, ob ein Commit im Repository vorhanden ist.
func (r Runner) CommitExists(ctx context.Context, commit string) bool {
	if commit == "" {
		return false
	}
	_, err := r.run(ctx, "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// DiffRaw liefert die Änderungen zwischen zwei Commits mit Umbenennungserkennung.
func (r Runner) DiffRaw(ctx context.Context, from, to string) ([]Change, error) {
	out, err := r.run(ctx, "diff", "--raw", "-z", "--no-abbrev", "--find-renames", from, to)
	if err != nil {
		return nil, err
	}
	return ParseDiffRaw(out)
}

// ParseDiffRaw zerlegt die Ausgabe von `git diff --raw -z`.
// Format: ":<alt-mode> <neu-mode> <alt-sha> <neu-sha> <status>\0<pfad>\0" und bei R/C zusätzlich <pfad>\0.
func ParseDiffRaw(data []byte) ([]Change, error) {
	var out []Change
	parts := strings.Split(string(data), "\x00")
	for i := 0; i < len(parts); i++ {
		meta := parts[i]
		if meta == "" {
			continue
		}
		if !strings.HasPrefix(meta, ":") {
			return nil, fmt.Errorf("diff: unerwarteter Eintrag %q", meta)
		}
		fields := strings.Fields(meta[1:])
		if len(fields) != 5 {
			return nil, fmt.Errorf("diff: ungültige Metadaten %q", meta)
		}
		status := fields[4][0]
		ch := Change{Status: status, NewSHA: fields[3]}
		if strings.Trim(ch.NewSHA, "0") == "" {
			ch.NewSHA = ""
		}
		if status == 'R' || status == 'C' {
			if i+2 >= len(parts) {
				return nil, fmt.Errorf("diff: Umbenennung ohne Pfade")
			}
			ch.OldPath = parts[i+1]
			ch.Path = parts[i+2]
			i += 2
		} else {
			if i+1 >= len(parts) {
				return nil, fmt.Errorf("diff: Eintrag ohne Pfad")
			}
			ch.Path = parts[i+1]
			i++
		}
		out = append(out, ch)
	}
	return out, nil
}

// LineRange ist ein geänderter Bereich im neuen Stand (1-basiert, inklusiv).
// Bei reinen Löschungen ist Start == End die Zeile direkt vor der Lösch-Stelle.
type LineRange struct {
	Start, End int
}

// DiffHunks liefert die geänderten Zeilenbereiche des neuen Stands je Datei (Pfad mit Umbenennung).
func (r Runner) DiffHunks(ctx context.Context, from, to string) (map[string][]LineRange, error) {
	out, err := r.run(ctx, "diff", "-U0", "--no-color", "--find-renames", from, to)
	if err != nil {
		return nil, err
	}
	return ParseHunks(out), nil
}

// ParseHunks wertet die Ausgabe von `git diff -U0` aus.
func ParseHunks(data []byte) map[string][]LineRange {
	res := map[string][]LineRange{}
	var cur string
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			if p == "/dev/null" {
				cur = ""
			} else {
				cur = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@") && cur != "":
			// @@ -a[,b] +c[,d] @@
			fields := strings.Fields(line)
			if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
				continue
			}
			c, d := parseRange(strings.TrimPrefix(fields[2], "+"))
			if d == 0 {
				res[cur] = append(res[cur], LineRange{Start: c, End: c})
			} else {
				res[cur] = append(res[cur], LineRange{Start: c, End: c + d - 1})
			}
		}
	}
	return res
}

func parseRange(s string) (int, int) {
	start, count := s, "1"
	if i := strings.IndexByte(s, ','); i >= 0 {
		start, count = s[:i], s[i+1:]
	}
	c, _ := strconv.Atoi(start)
	d, _ := strconv.Atoi(count)
	return c, d
}

// ShowFile liefert den Inhalt einer Datei in einem Commit.
func (r Runner) ShowFile(ctx context.Context, commit, path string) ([]byte, error) {
	return r.run(ctx, "show", commit+":"+path)
}
