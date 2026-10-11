// Package snapshot ist ein persistentes Backend als Übergang: Der Bestand liegt im Speicher
// (Backend memory) und wird beim Schließen in eine Datei geschrieben. Es ersetzt SQLite, bis
// dessen Treiber verfügbar ist. Die Schnittstelle store.Store bleibt dieselbe, der Wechsel
// ist eine neue Implementierung ohne Umbau des Anwendungscodes.
package snapshot

import (
	"context"
	"errors"
	"os"

	"goreadmenator/internal/store"
	"goreadmenator/internal/store/memory"
)

func init() {
	store.Register("snapshot", func(_ context.Context, path string) (store.Store, error) {
		m, err := memory.LoadFile(path)
		if errors.Is(err, memory.ErrIncompatible) {
			// Altes Format: die Datei wird beiseitegelegt, der Index wird beim nächsten Lauf neu aufgebaut.
			os.Rename(path, path+".alt")
			m = memory.New()
		} else if err != nil {
			return nil, err
		}
		return &backend{Store: m, path: path}, nil
	})
}

type backend struct {
	*memory.Store
	path string
}

// Close schreibt den Bestand und gibt danach Ressourcen frei.
func (b *backend) Close() error {
	if err := b.Store.SaveFile(b.path); err != nil {
		return err
	}
	return b.Store.Close()
}

// CloseWithoutSave gibt Ressourcen frei, ohne den Bestand zurückzuschreiben. Für reine Lesezugriffe
// wie den MCP-Server, damit keine Datei geändert wird.
func (b *backend) CloseWithoutSave() error {
	return b.Store.Close()
}
