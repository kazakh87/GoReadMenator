package csharp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkParseCorpus parst alle C#-Dateien eines Korpus (READMENATOR_CORPUS). Ohne Korpus wird übersprungen.
func BenchmarkParseCorpus(b *testing.B) {
	dir := os.Getenv("READMENATOR_CORPUS")
	if dir == "" {
		b.Skip("READMENATOR_CORPUS nicht gesetzt")
	}
	var files [][]byte
	var total int64
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".cs") && !strings.Contains(p, "/.git/") {
			if data, err := os.ReadFile(p); err == nil {
				files = append(files, data)
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
			parseFile(f)
		}
	}
}
