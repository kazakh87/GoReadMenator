// Package sqlite ist das persistente Backend auf SQLite. Es spricht ausschließlich über database/sql.
// Der Treiber wird über das Build-Tag "modernc" eingebunden (siehe driver_modernc.go), damit der Standardbau
// keine Fremdabhängigkeit braucht.
//
// Alle SQL-Statements gehören in dieses Paket. Das Verhalten folgt dem Speicher-Backend und wird durch die
// gemeinsame Vertragssuite internal/store/storetest geprüft.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
)

// ErrNotAvailable zeigt an, dass der Treiber nicht eingebunden ist.
var ErrNotAvailable = errors.New("sqlite-backend: Treiber fehlt, mit -tags modernc bauen (siehe docs/SQLITE.md)")

const driverName = "sqlite"

func init() {
	store.Register("sqlite", func(ctx context.Context, path string) (store.Store, error) {
		s, err := Open(ctx, path)
		if err != nil {
			return nil, err // nil als Interface, nicht als typisierter nil-Zeiger
		}
		return s, nil
	})
}

// Store ist das SQLite-Backend.
type Store struct {
	db *sql.DB
}

var _ store.Store = (*Store)(nil)

// Open öffnet oder erstellt die Datenbank unter path.
func Open(ctx context.Context, path string) (*Store, error) {
	if !driverRegistered() {
		return nil, ErrNotAvailable
	}
	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, err
	}
	// Ein Verbindung genügt: Schreibzugriffe sind serialisiert, SQLite sperrt die Datei ohnehin.
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode = WAL", "PRAGMA synchronous = NORMAL", "PRAGMA foreign_keys = OFF"} {
		if _, err := db.ExecContext(ctx, p); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite: %s: %w", p, err)
		}
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: schema: %w", err)
	}
	return &Store{db: db}, nil
}

func driverRegistered() bool {
	return slices.Contains(sql.Drivers(), driverName)
}

// Close schließt die Datenbank.
func (s *Store) Close() error { return s.db.Close() }

// ---------- Meta und Dateien ----------

func (s *Store) Meta(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// PutFile legt eine Datei an oder aktualisiert sie. Der Pfad ist der Schlüssel, wie im Speicher-Backend.
func (s *Store) PutFile(ctx context.Context, f model.File) (model.FileID, error) {
	var existing int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM files WHERE path = ?`, f.Path).Scan(&existing)
	switch {
	case err == nil:
		f.ID = model.FileID(existing)
	case errors.Is(err, sql.ErrNoRows):
		if f.ID == 0 {
			res, err := s.db.ExecContext(ctx, `INSERT INTO files (path, lang, blob_sha, loc, file_doc) VALUES (?, ?, ?, ?, ?)`,
				f.Path, string(f.Lang), f.BlobSHA, f.LOC, f.FileDoc)
			if err != nil {
				return 0, err
			}
			id, err := res.LastInsertId()
			return model.FileID(id), err
		}
	default:
		return 0, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO files (id, path, lang, blob_sha, loc, file_doc) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET path = excluded.path, lang = excluded.lang, blob_sha = excluded.blob_sha,
		loc = excluded.loc, file_doc = excluded.file_doc`,
		int64(f.ID), f.Path, string(f.Lang), f.BlobSHA, f.LOC, f.FileDoc)
	return f.ID, err
}

func (s *Store) FileByPath(ctx context.Context, path string) (model.File, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, path, lang, blob_sha, loc, file_doc FROM files WHERE path = ?`, path)
	f, err := scanFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.File{}, false, nil
	}
	return f, err == nil, err
}

func (s *Store) Files(ctx context.Context) ([]model.File, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, path, lang, blob_sha, loc, file_doc FROM files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func scanFile(sc interface{ Scan(...any) error }) (model.File, error) {
	var f model.File
	var id int64
	var lang string
	if err := sc.Scan(&id, &f.Path, &lang, &f.BlobSHA, &f.LOC, &f.FileDoc); err != nil {
		return model.File{}, err
	}
	f.ID = model.FileID(id)
	f.Lang = model.Language(lang)
	return f, nil
}

// DeleteFile entfernt alle Daten einer Datei. Symbole ohne verbleibende Fundorte fallen weg.
func (s *Store) DeleteFile(ctx context.Context, id model.FileID) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM locations WHERE file_id = ?`,
			`DELETE FROM refs WHERE file_id = ?`,
			`DELETE FROM relations WHERE file_id = ?`,
			`DELETE FROM imports WHERE file_id = ?`,
			`DELETE FROM files WHERE id = ?`,
		} {
			if _, err := tx.ExecContext(ctx, q, int64(id)); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM symbols WHERE id NOT IN (SELECT DISTINCT symbol_id FROM locations)`)
		return err
	})
}

// ---------- Schreiben ----------

// WriteBatch schreibt einen Batch atomar. Vorhandene Symbole werden nach der Merge-Regel ergänzt.
func (s *Store) WriteBatch(ctx context.Context, b store.Batch) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, sym := range b.Symbols {
			if err := upsertSymbol(ctx, tx, sym); err != nil {
				return err
			}
		}
		for _, l := range b.Locations {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM symbols WHERE id = ?`, string(l.SymbolID)).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("location für unbekanntes Symbol %q", l.SymbolID)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO locations (symbol_id, file_id, start_line, start_col, end_line, end_col, role)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, string(l.SymbolID), int64(l.FileID), l.Span.StartLine, l.Span.StartCol,
				l.Span.EndLine, l.Span.EndCol, string(l.Role)); err != nil {
				return err
			}
		}
		for _, r := range b.References {
			if _, err := tx.ExecContext(ctx, `INSERT INTO refs (file_id, target_id, target_name, kind, start_line, start_col,
				end_line, end_col, enclosing, confidence, arity, role, qualifier) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				int64(r.FileID), string(r.TargetID), r.TargetName, string(r.Kind), r.Span.StartLine, r.Span.StartCol,
				r.Span.EndLine, r.Span.EndCol, string(r.Enclosing), string(r.Confidence), r.Arity, r.Role, r.Qualifier); err != nil {
				return err
			}
		}
		for _, rel := range b.Relations {
			if err := insertRelation(ctx, tx, rel); err != nil {
				return err
			}
		}
		for _, im := range b.Imports {
			bind, err := json.Marshal(im.Bindings)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO imports (file_id, raw, kind, resolved_id, bindings) VALUES (?, ?, ?, ?, ?)`,
				int64(im.FileID), im.Raw, string(im.Kind), int64(im.ResolvedID), string(bind)); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertSymbol(ctx context.Context, tx *sql.Tx, sym model.Symbol) error {
	old, ok, err := readSymbol(ctx, tx, sym.ID)
	if err != nil {
		return err
	}
	if ok {
		sym = mergeSymbol(old, sym)
	}
	mods, err := json.Marshal(sym.Modifiers)
	if err != nil {
		return err
	}
	params, err := json.Marshal(sym.ParamDocs)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO symbols (id, kind, name, fqn, container, signature, modifiers, summary, remarks, returns, param_docs, doc_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET kind = excluded.kind, name = excluded.name, fqn = excluded.fqn, container = excluded.container,
		signature = excluded.signature, modifiers = excluded.modifiers, summary = excluded.summary, remarks = excluded.remarks,
		returns = excluded.returns, param_docs = excluded.param_docs, doc_source = excluded.doc_source`,
		string(sym.ID), string(sym.Kind), sym.Name, sym.FQN, string(sym.Container), sym.Signature, string(mods),
		sym.Summary, sym.Remarks, sym.Returns, string(params), sym.DocSource)
	return err
}

func insertRelation(ctx context.Context, tx *sql.Tx, rel model.Relation) error {
	derived := 0
	if rel.Derived {
		derived = 1
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO relations (file_id, from_id, to_id, kind, derived) VALUES (?, ?, ?, ?, ?)`,
		int64(rel.FileID), string(rel.From), string(rel.To), string(rel.Kind), derived)
	return err
}

// ---------- Lesen ----------

func (s *Store) Symbol(ctx context.Context, id model.SymbolID) (model.Symbol, bool, error) {
	return readSymbol(ctx, s.db, id)
}

// symbolQueryer ist DB oder Transaktion.
type symbolQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readSymbol(ctx context.Context, q symbolQueryer, id model.SymbolID) (model.Symbol, bool, error) {
	row := q.QueryRowContext(ctx, symbolColumns+` WHERE id = ?`, string(id))
	sym, err := scanSymbol(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Symbol{}, false, nil
	}
	return sym, err == nil, err
}

const symbolColumns = `SELECT id, kind, name, fqn, container, signature, modifiers, summary, remarks, returns, param_docs, doc_source FROM symbols`

func scanSymbol(sc interface{ Scan(...any) error }) (model.Symbol, error) {
	var sym model.Symbol
	var id, kind, container string
	var mods, params sql.NullString
	if err := sc.Scan(&id, &kind, &sym.Name, &sym.FQN, &container, &sym.Signature, &mods, &sym.Summary,
		&sym.Remarks, &sym.Returns, &params, &sym.DocSource); err != nil {
		return model.Symbol{}, err
	}
	sym.ID = model.SymbolID(id)
	sym.Kind = model.SymbolKind(kind)
	sym.Container = model.SymbolID(container)
	if err := decodeJSON(mods, &sym.Modifiers); err != nil {
		return model.Symbol{}, err
	}
	if err := decodeJSON(params, &sym.ParamDocs); err != nil {
		return model.Symbol{}, err
	}
	return sym, nil
}

// decodeJSON liest eine JSON-Spalte. NULL bleibt nil, so wie der Speicher es liefert.
func decodeJSON(v sql.NullString, dst any) error {
	if !v.Valid || v.String == "" || v.String == "null" {
		return nil
	}
	return json.Unmarshal([]byte(v.String), dst)
}

func (s *Store) SymbolsByName(ctx context.Context, name string) ([]model.Symbol, error) {
	return s.querySymbols(ctx, symbolColumns+` WHERE name = ? ORDER BY id`, name)
}

func (s *Store) AllSymbols(ctx context.Context) ([]model.Symbol, error) {
	return s.querySymbols(ctx, symbolColumns+` ORDER BY id`)
}

func (s *Store) querySymbols(ctx context.Context, q string, args ...any) ([]model.Symbol, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Symbol
	for rows.Next() {
		sym, err := scanSymbol(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

const locationColumns = `SELECT symbol_id, file_id, start_line, start_col, end_line, end_col, role FROM locations`

func scanLocation(sc interface{ Scan(...any) error }) (model.Location, error) {
	var l model.Location
	var sid string
	var fid int64
	var role string
	if err := sc.Scan(&sid, &fid, &l.Span.StartLine, &l.Span.StartCol, &l.Span.EndLine, &l.Span.EndCol, &role); err != nil {
		return model.Location{}, err
	}
	l.SymbolID = model.SymbolID(sid)
	l.FileID = model.FileID(fid)
	l.Role = model.LocationRole(role)
	return l, nil
}

func (s *Store) Locations(ctx context.Context, id model.SymbolID) ([]model.Location, error) {
	return s.queryLocations(ctx, locationColumns+` WHERE symbol_id = ? ORDER BY seq`, string(id))
}

func (s *Store) AllLocations(ctx context.Context) ([]model.Location, error) {
	return s.queryLocations(ctx, locationColumns+` ORDER BY seq`)
}

func (s *Store) queryLocations(ctx context.Context, q string, args ...any) ([]model.Location, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Location
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

const refColumns = `SELECT id, file_id, target_id, target_name, kind, start_line, start_col, end_line, end_col,
	enclosing, confidence, arity, role, qualifier FROM refs`

func scanRef(sc interface{ Scan(...any) error }) (model.Reference, error) {
	var r model.Reference
	var fid int64
	var target, kind, enc, conf string
	if err := sc.Scan(&r.ID, &fid, &target, &r.TargetName, &kind, &r.Span.StartLine, &r.Span.StartCol,
		&r.Span.EndLine, &r.Span.EndCol, &enc, &conf, &r.Arity, &r.Role, &r.Qualifier); err != nil {
		return model.Reference{}, err
	}
	r.FileID = model.FileID(fid)
	r.TargetID = model.SymbolID(target)
	r.Kind = model.RefKind(kind)
	r.Enclosing = model.SymbolID(enc)
	r.Confidence = model.Confidence(conf)
	return r, nil
}

func (s *Store) queryRefs(ctx context.Context, q string, args ...any) ([]model.Reference, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Reference
	for rows.Next() {
		r, err := scanRef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ReferencesTo(ctx context.Context, id model.SymbolID) ([]model.Reference, error) {
	return s.queryRefs(ctx, refColumns+` WHERE target_id = ? ORDER BY id`, string(id))
}

func (s *Store) ReferencesByName(ctx context.Context, name string) ([]model.Reference, error) {
	return s.queryRefs(ctx, refColumns+` WHERE target_name = ? ORDER BY id`, name)
}

func (s *Store) AllReferences(ctx context.Context) ([]model.Reference, error) {
	return s.queryRefs(ctx, refColumns+` ORDER BY id`)
}

const relationColumns = `SELECT file_id, from_id, to_id, kind, derived FROM relations`

func scanRelation(sc interface{ Scan(...any) error }) (model.Relation, error) {
	var r model.Relation
	var fid int64
	var from, to, kind string
	var derived int
	if err := sc.Scan(&fid, &from, &to, &kind, &derived); err != nil {
		return model.Relation{}, err
	}
	r.FileID = model.FileID(fid)
	r.From = model.SymbolID(from)
	r.To = model.SymbolID(to)
	r.Kind = model.RelationKind(kind)
	r.Derived = derived != 0
	return r, nil
}

func (s *Store) queryRelations(ctx context.Context, q string, args ...any) ([]model.Relation, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Relation
	for rows.Next() {
		r, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Relations: zuerst die geparsten, dann die abgeleiteten, so wie der Speicher es liefert.
func (s *Store) Relations(ctx context.Context, id model.SymbolID) ([]model.Relation, error) {
	return s.queryRelations(ctx, relationColumns+` WHERE from_id = ? OR to_id = ? ORDER BY derived, seq`, string(id), string(id))
}

func (s *Store) AllRelations(ctx context.Context) ([]model.Relation, error) {
	return s.queryRelations(ctx, relationColumns+` ORDER BY derived, seq`)
}

func (s *Store) AllImports(ctx context.Context) ([]model.Import, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, file_id, raw, kind, resolved_id, bindings FROM imports ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Import
	for rows.Next() {
		var im model.Import
		var fid, resolved int64
		var kind string
		var bindings sql.NullString
		if err := rows.Scan(&im.ID, &fid, &im.Raw, &kind, &resolved, &bindings); err != nil {
			return nil, err
		}
		im.FileID = model.FileID(fid)
		im.Kind = model.ImportKind(kind)
		im.ResolvedID = model.FileID(resolved)
		if err := decodeJSON(bindings, &im.Bindings); err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

// ---------- Aktualisieren ----------

func (s *Store) UpdateReferences(ctx context.Context, refs []model.Reference) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, r := range refs {
			if _, err := tx.ExecContext(ctx, `UPDATE refs SET target_id = ?, confidence = ? WHERE id = ?`,
				string(r.TargetID), string(r.Confidence), r.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) UpdateImports(ctx context.Context, imps []model.Import) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, im := range imps {
			if _, err := tx.ExecContext(ctx, `UPDATE imports SET resolved_id = ? WHERE id = ?`, int64(im.ResolvedID), im.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetDerivedRelations ersetzt alle abgeleiteten Beziehungen. Geparste bleiben unberührt.
func (s *Store) SetDerivedRelations(ctx context.Context, rels []model.Relation) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM relations WHERE derived = 1`); err != nil {
			return err
		}
		for _, r := range rels {
			r.Derived = true
			if err := insertRelation(ctx, tx, r); err != nil {
				return err
			}
		}
		return nil
	})
}

// tx führt fn in einer Transaktion aus und setzt bei Fehlern zurück.
func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// mergeSymbol übernimmt Felder aus neu nur dort, wo old leer ist (Merge-Regel aus store.Store).
func mergeSymbol(old, neu model.Symbol) model.Symbol {
	if old.Kind == "" {
		old.Kind = neu.Kind
	}
	if old.Name == "" {
		old.Name = neu.Name
	}
	if old.FQN == "" {
		old.FQN = neu.FQN
	}
	if old.Container == "" {
		old.Container = neu.Container
	}
	if old.Signature == "" {
		old.Signature = neu.Signature
	}
	if len(old.Modifiers) == 0 {
		old.Modifiers = neu.Modifiers
	}
	if old.Summary == "" {
		old.Summary = neu.Summary
	}
	if old.Remarks == "" {
		old.Remarks = neu.Remarks
	}
	if old.Returns == "" {
		old.Returns = neu.Returns
	}
	if len(old.ParamDocs) == 0 {
		old.ParamDocs = neu.ParamDocs
	}
	if old.DocSource == "" {
		old.DocSource = neu.DocSource
	}
	return old
}
