package sqlite

// schema legt alle Tabellen an. Listen (Modifikatoren, Parameter, Bindings) werden als JSON gespeichert,
// weil sie nie einzeln abgefragt werden. Die Reihenfolge der Einfügung bleibt über rowid und
// AUTOINCREMENT erhalten, denn der Speicher-Backend-Vertrag verlangt diese Reihenfolge.
const schema = `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
	id       INTEGER PRIMARY KEY,
	path     TEXT NOT NULL UNIQUE,
	lang     TEXT NOT NULL DEFAULT '',
	blob_sha TEXT NOT NULL DEFAULT '',
	loc      INTEGER NOT NULL DEFAULT 0,
	file_doc TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS symbols (
	id         TEXT PRIMARY KEY,
	kind       TEXT NOT NULL DEFAULT '',
	name       TEXT NOT NULL DEFAULT '',
	fqn        TEXT NOT NULL DEFAULT '',
	container  TEXT NOT NULL DEFAULT '',
	signature  TEXT NOT NULL DEFAULT '',
	modifiers  TEXT,
	summary    TEXT NOT NULL DEFAULT '',
	remarks    TEXT NOT NULL DEFAULT '',
	returns    TEXT NOT NULL DEFAULT '',
	param_docs TEXT,
	doc_source TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS symbols_name ON symbols(name);

CREATE TABLE IF NOT EXISTS locations (
	seq        INTEGER PRIMARY KEY AUTOINCREMENT,
	symbol_id  TEXT NOT NULL,
	file_id    INTEGER NOT NULL,
	start_line INTEGER, start_col INTEGER, end_line INTEGER, end_col INTEGER,
	role       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS locations_symbol ON locations(symbol_id);
CREATE INDEX IF NOT EXISTS locations_file ON locations(file_id);

CREATE TABLE IF NOT EXISTS refs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	file_id     INTEGER NOT NULL,
	target_id   TEXT NOT NULL DEFAULT '',
	target_name TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT '',
	start_line  INTEGER, start_col INTEGER, end_line INTEGER, end_col INTEGER,
	enclosing   TEXT NOT NULL DEFAULT '',
	confidence  TEXT NOT NULL DEFAULT '',
	arity       INTEGER NOT NULL DEFAULT 0,
	role        TEXT NOT NULL DEFAULT '',
	qualifier   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS refs_target_id ON refs(target_id);
CREATE INDEX IF NOT EXISTS refs_target_name ON refs(target_name);
CREATE INDEX IF NOT EXISTS refs_file ON refs(file_id);

CREATE TABLE IF NOT EXISTS relations (
	seq     INTEGER PRIMARY KEY AUTOINCREMENT,
	file_id INTEGER NOT NULL DEFAULT 0,
	from_id TEXT NOT NULL,
	to_id   TEXT NOT NULL,
	kind    TEXT NOT NULL,
	derived INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS relations_from ON relations(from_id);
CREATE INDEX IF NOT EXISTS relations_to ON relations(to_id);
CREATE INDEX IF NOT EXISTS relations_file ON relations(file_id);

CREATE TABLE IF NOT EXISTS imports (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	file_id     INTEGER NOT NULL,
	raw         TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT '',
	resolved_id INTEGER NOT NULL DEFAULT 0,
	bindings    TEXT
);
CREATE INDEX IF NOT EXISTS imports_file ON imports(file_id);
`
