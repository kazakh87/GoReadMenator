# SQLite-Backend testen

Das Backend liegt in `internal/store/sqlite`. Es spricht über `database/sql`.
Der Treiber `modernc.org/sqlite` wird nur mit dem Build-Tag `modernc`
eingebunden, damit der Standardbau ohne Fremdabhängigkeit läuft.

## Ablauf auf einem Rechner mit Netz

```
cd GoReadMenator
./scripts/test-sqlite.sh
```

Das Skript

1. bindet den Treiber in `go.mod` ein (`go get modernc.org/sqlite@v1.60.1`,
   `go mod tidy`),
2. führt die gemeinsamen Vertragstests des Store-Interface gegen SQLite aus,
3. führt die gesamte Testsuite mit Treiber aus,
4. baut das Programm mit Treiber und indiziert ein Beispielprojekt in eine
   SQLite-Datei.

Ohne Treiber (Standard) überspringt `TestContract` den Lauf,
`TestWithoutDriverReportsUnavailable` prüft die klare Fehlermeldung.

## Was dabei geprüft wird

Die Vertragssuite `internal/store/storetest` ist für alle Backends gleich. Das
SQLite-Backend muss dasselbe Verhalten wie das Speicher-Backend zeigen:
Merge-Regel für Symbole, Reihenfolge der Fundorte und Verweise, Löschen mit
Aufräumen verwaister Symbole, Trennung geparster und abgeleiteter Beziehungen,
Upsert von Dateien.

## Bekannte Punkte

- Der Index liegt im WAL-Modus. Der MCP-Server beobachtet deshalb auch die Datei
  `-wal` für Neuladen.
- Das Schema ist in `schema.go`. Änderungen am Schema brauchen eine neue Parser-
  oder Index-Version, sonst bleibt eine alte Datei unverändert und wird nicht
  neu aufgebaut.
- Die SQL-Anweisungen wurden gegen SQLite 3.45 geprüft, die Treiberversion
  `v1.60.1` nutzt eine neuere SQLite-Version. Der Vertragstest ist die
  verbindliche Prüfung.
