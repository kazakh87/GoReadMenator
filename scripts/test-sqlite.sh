#!/usr/bin/env bash
# Testet das SQLite-Backend mit dem echten Treiber modernc.org/sqlite.
# Voraussetzung: Netzzugriff auf das Go-Modul-Proxy und ein Go, das die go-Direktive des Treibers erfüllt.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "== Treiber einbinden"
go get modernc.org/sqlite@v1.60.1
go mod tidy

echo "== Vertragstests des SQLite-Backends"
go test -tags modernc -count=1 -v ./internal/store/sqlite/

echo "== Gesamte Suite mit Treiber"
go test -tags modernc -count=1 ./...

echo "== Ende-zu-Ende: Index mit SQLite aufbauen und abfragen"
go build -tags modernc -o bin/goreadmenator ./cmd/goreadmenator
rm -f /tmp/rm-sqlite-test.db
bin/goreadmenator -repo internal/resolve/testdata/11_mixed_languages_web -store sqlite -db /tmp/rm-sqlite-test.db index
bin/goreadmenator -repo internal/resolve/testdata/11_mixed_languages_web -store sqlite -db /tmp/rm-sqlite-test.db symbol Load | head -5
echo "OK"
