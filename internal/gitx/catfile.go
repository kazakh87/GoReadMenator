package gitx

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// CatFile ist ein langlebiger `git cat-file --batch`-Prozess. Er liest viele
// Blobs über eine einzige Pipe, statt für jede Datei einen neuen Prozess zu
// starten. Das ist der entscheidende Performance-Punkt beim Einlesen.
//
// Ein CatFile ist nicht nebenläufig nutzbar. Jeder Worker erhält seine eigene Instanz.
type CatFile struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	closed bool
}

// OpenCatFile startet den Prozess für das Repository.
func (r Runner) OpenCatFile(ctx context.Context) (*CatFile, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", r.Dir, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git cat-file starten: %w", err)
	}
	return &CatFile{cmd: cmd, stdin: stdin, stdout: bufio.NewReaderSize(stdout, 64<<10)}, nil
}

// Blob liefert den Inhalt eines Blobs anhand seiner SHA.
func (c *CatFile) Blob(sha string) ([]byte, error) {
	if c.closed {
		return nil, fmt.Errorf("cat-file: bereits geschlossen")
	}
	if _, err := io.WriteString(c.stdin, sha+"\n"); err != nil {
		return nil, fmt.Errorf("cat-file schreiben: %w", err)
	}
	header, err := c.stdout.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("cat-file Header: %w", err)
	}
	fields := strings.Fields(header)
	if len(fields) == 2 && fields[1] == "missing" {
		return nil, fmt.Errorf("blob %s fehlt", sha)
	}
	if len(fields) != 3 || fields[1] != "blob" {
		return nil, fmt.Errorf("cat-file: unerwarteter Header %q", strings.TrimSpace(header))
	}
	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cat-file: ungültige Größe %q", fields[2])
	}
	buf := make([]byte, size+1) // Inhalt plus abschließendes LF
	if _, err := io.ReadFull(c.stdout, buf); err != nil {
		return nil, fmt.Errorf("cat-file Inhalt: %w", err)
	}
	return buf[:size], nil
}

// Close beendet den Prozess.
func (c *CatFile) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	_ = c.stdin.Close()
	// Wait liefert bei Abbruch durch Kontext einen Fehler; für uns zählt nur das Ende.
	_ = c.cmd.Wait()
	return nil
}
