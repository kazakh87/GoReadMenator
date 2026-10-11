package memory

import (
	"bufio"
	"encoding/gob"
	"errors"
	"io"
	"os"
	"path/filepath"

	"goreadmenator/internal/model"
)

// snapshotHeader enthält alles außer den Datensätzen.
type snapshotHeader struct {
	Meta     map[string]string
	NextRef  int64
	NextImp  int64
	NextFile model.FileID
}

// snapshotMagic kennzeichnet das Format. Ändert sich das Layout, ändert sich die Kennung,
// und ältere Dateien werden als nicht kompatibel erkannt statt falsch gelesen.
const snapshotMagic = "RMSNAP02"

// ErrIncompatible meldet eine Snapshot-Datei in einem anderen Format.
var ErrIncompatible = errors.New("snapshot: Format nicht kompatibel")

// chunkSize begrenzt, wie viele Datensätze pro gob-Nachricht kodiert werden. Eine einzige
// große Nachricht würde einen Puffer in Größe des gesamten Bestands erzeugen.
const chunkSize = 4096

// SaveFile schreibt den Bestand atomar in eine Datei. Die Datensätze werden in Blöcken
// kodiert, damit der Schreibpuffer klein bleibt. Ein Übergang bis SQLite verfügbar ist.
func (s *Store) SaveFile(path string) error {
	s.mu.RLock()
	files := make([]model.File, 0, len(s.files))
	for _, f := range s.files {
		files = append(files, f)
	}
	syms := make([]model.Symbol, 0, len(s.symbols))
	for _, sym := range s.symbols {
		syms = append(syms, sym)
	}
	hdr := snapshotHeader{Meta: s.meta, NextRef: s.nextRef, NextImp: s.nextImp, NextFile: s.nextFile}
	locs, refs, rels, derived, imps := s.locations, s.refs, s.relations, s.derived, s.imports
	s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if _, err := bw.WriteString(snapshotMagic); err != nil {
		return fail(err)
	}
	enc := gob.NewEncoder(bw)
	if err := enc.Encode(hdr); err != nil {
		return fail(err)
	}
	if err := encodeChunks(enc, files); err != nil {
		return fail(err)
	}
	if err := encodeChunks(enc, syms); err != nil {
		return fail(err)
	}
	if err := encodeChunks(enc, locs); err != nil {
		return fail(err)
	}
	if err := encodeChunks(enc, refs); err != nil {
		return fail(err)
	}
	if err := encodeChunks(enc, rels); err != nil {
		return fail(err)
	}
	if err := encodeChunks(enc, derived); err != nil {
		return fail(err)
	}
	if err := encodeChunks(enc, imps); err != nil {
		return fail(err)
	}
	if err := bw.Flush(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// encodeChunks schreibt Anzahl und dann die Datensätze in Blöcken.
func encodeChunks[T any](enc *gob.Encoder, items []T) error {
	if err := enc.Encode(len(items)); err != nil {
		return err
	}
	for i := 0; i < len(items); i += chunkSize {
		end := i + chunkSize
		if end > len(items) {
			end = len(items)
		}
		if err := enc.Encode(items[i:end]); err != nil {
			return err
		}
	}
	return nil
}

// decodeChunks liest, was encodeChunks geschrieben hat.
func decodeChunks[T any](dec *gob.Decoder) ([]T, error) {
	var n int
	if err := dec.Decode(&n); err != nil {
		return nil, err
	}
	out := make([]T, 0, n)
	for len(out) < n {
		var part []T
		if err := dec.Decode(&part); err != nil {
			return nil, err
		}
		if len(part) == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		out = append(out, part...)
	}
	return out, nil
}

// LoadFile liest einen mit SaveFile geschriebenen Bestand. Fehlt die Datei, ist der Store leer.
func LoadFile(path string) (*Store, error) {
	s := New()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() == 0 {
		return s, nil
	}
	br := bufio.NewReaderSize(f, 1<<20)
	magic := make([]byte, len(snapshotMagic))
	if _, err := io.ReadFull(br, magic); err != nil || string(magic) != snapshotMagic {
		return nil, ErrIncompatible
	}
	dec := gob.NewDecoder(br)
	var hdr snapshotHeader
	if err := dec.Decode(&hdr); err != nil {
		return nil, err
	}
	if hdr.Meta != nil {
		s.meta = hdr.Meta
	}
	s.nextRef, s.nextImp, s.nextFile = hdr.NextRef, hdr.NextImp, hdr.NextFile

	files, err := decodeChunks[model.File](dec)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		s.files[file.ID] = file
		s.byPath[file.Path] = file.ID
	}
	syms, err := decodeChunks[model.Symbol](dec)
	if err != nil {
		return nil, err
	}
	for _, sym := range syms {
		s.symbols[sym.ID] = sym
	}
	if s.locations, err = decodeChunks[model.Location](dec); err != nil {
		return nil, err
	}
	if s.refs, err = decodeChunks[model.Reference](dec); err != nil {
		return nil, err
	}
	if s.relations, err = decodeChunks[model.Relation](dec); err != nil {
		return nil, err
	}
	if s.derived, err = decodeChunks[model.Relation](dec); err != nil {
		return nil, err
	}
	if s.imports, err = decodeChunks[model.Import](dec); err != nil {
		return nil, err
	}
	return s, nil
}
