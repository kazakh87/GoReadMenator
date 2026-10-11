// Package parsers ist die einzige Stelle, an der alle Parser eingetragen werden.
//
// Ein neuer Parser wird hier ergänzt. CLI und alle Tests verwenden Registry(), damit kein Test
// versehentlich einen unvollständigen Stand benutzt.
package parsers

import (
	"goreadmenator/internal/parser"
	"goreadmenator/internal/parser/csharp"
	"goreadmenator/internal/parser/razor"
	"goreadmenator/internal/parser/tsjs"
	"goreadmenator/internal/parser/vbnet"
)

// Registry liefert die Registry mit allen unterstützten Sprachen.
func Registry() (*parser.Registry, error) {
	return parser.NewRegistry(csharp.New(), tsjs.New(), vbnet.New(), razor.NewRazor(), razor.NewASPX())
}
