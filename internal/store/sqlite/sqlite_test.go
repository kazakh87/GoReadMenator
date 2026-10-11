package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"goreadmenator/internal/store"
	"goreadmenator/internal/store/storetest"
)

// Ohne Treiber meldet das Backend klar, was fehlt, und baut kein leeres Backend.
func TestWithoutDriverReportsUnavailable(t *testing.T) {
	if driverRegistered() {
		t.Skip("Treiber eingebunden, dieser Test prüft den Fehlerfall")
	}
	_, err := store.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "x.db"))
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("err = %v, want ErrNotAvailable", err)
	}
}

// Mit Treiber läuft die gemeinsame Vertragssuite gegen eine frische Datei je Test.
func TestContract(t *testing.T) {
	if !driverRegistered() {
		t.Skip("Treiber fehlt, mit -tags modernc bauen")
	}
	storetest.Run(t, func(t *testing.T) store.Store {
		s, err := Open(context.Background(), filepath.Join(t.TempDir(), "contract.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	})
}
