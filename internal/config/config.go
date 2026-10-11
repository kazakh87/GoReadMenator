// Package config bündelt die Laufzeitkonfiguration mit sinnvollen Defaults.
package config

import (
	"runtime"
	"time"
)

// Config steuert Verhalten und Ressourcen eines Laufs.
type Config struct {
	// RepoPath ist das Wurzelverzeichnis des Git-Repositories.
	RepoPath string
	// StoreBackend wählt das Persistenz-Backend (siehe store.Backends).
	StoreBackend string
	// DBPath ist der Pfad zur Datenbankdatei (bei memory ohne Bedeutung).
	DBPath string
	// Workers ist die Anzahl paralleler Parser-Goroutinen.
	Workers int
	// BatchSize ist die Anzahl Datensätze pro Schreibtransaktion.
	BatchSize int
	// MaxFileSizeBytes überspringt Dateien, die größer sind.
	MaxFileSizeBytes int64
	// ImpactDepth begrenzt die Tiefe der Impact-Analyse.
	ImpactDepth int
	// Timeout begrenzt die Laufzeit eines Befehls (0 = unbegrenzt).
	Timeout time.Duration
	// Verbose aktiviert Debug-Logging.
	Verbose bool
}

// Default liefert die Standardkonfiguration.
func Default() Config {
	return Config{
		RepoPath:         ".",
		StoreBackend:     "memory",
		DBPath:           ".goreadmenator/index.db",
		Workers:          runtime.NumCPU(),
		BatchSize:        1000,
		MaxFileSizeBytes: 5 << 20, // 5 MiB
		ImpactDepth:      3,
	}
}

// Validate prüft die Konfiguration und korrigiert unsinnige Werte.
func (c *Config) Validate() error {
	if c.Workers < 1 {
		c.Workers = 1
	}
	if c.BatchSize < 1 {
		c.BatchSize = 1
	}
	if c.MaxFileSizeBytes <= 0 {
		c.MaxFileSizeBytes = Default().MaxFileSizeBytes
	}
	if c.ImpactDepth < 1 {
		c.ImpactDepth = 1
	}
	return nil
}
