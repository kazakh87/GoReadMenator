//go:build modernc

package sqlite

// Bindet den reinen Go-Treiber modernc.org/sqlite ein. Gebaut wird mit: go build -tags modernc
// Voraussetzung: go get modernc.org/sqlite@v1.60.1 und ein Go, das dessen go-Direktive erfüllt.
import _ "modernc.org/sqlite"
