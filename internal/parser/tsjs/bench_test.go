package tsjs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkParseCorpus parst alle TS/JS-Dateien eines Korpus (READMENATOR_CORPUS). Ohne Korpus wird übersprungen.
func BenchmarkParseCorpus(b *testing.B) {
	dir := os.Getenv("READMENATOR_CORPUS")
	if dir == "" {
		b.Skip("READMENATOR_CORPUS nicht gesetzt")
	}
	type file struct {
		path string
		data []byte
	}
	var files []file
	var total int64
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".ts") && !strings.Contains(p, "/.git/") && !strings.Contains(p, "node_modules") {
			if data, err := os.ReadFile(p); err == nil {
				rel, _ := filepath.Rel(dir, p)
				files = append(files, file{rel, data})
				total += int64(len(data))
			}
		}
		return nil
	})
	b.SetBytes(total)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, f := range files {
			parseFile(f.path, f.data)
		}
	}
}
