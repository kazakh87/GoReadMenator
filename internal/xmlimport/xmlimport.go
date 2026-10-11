// Package xmlimport schreibt die Symbole einer XML-Dokumentation in den Index. Der Import
// ist unabhängig von Git und ersetzt bei einer Wiederholung seinen eigenen Stand.
//
// Die Symbole bekommen als Fundort ein virtuelles Assembly ("bin/Name.dll"), weil es keine
// Quelldatei gibt. Stimmt eine ID mit einem Symbol aus dem Quelltext überein, werden beide
// zusammengeführt: der Store behält vorhandene Felder und ergänzt leere.
package xmlimport

import (
	"context"
	"os"
	"path/filepath"

	"goreadmenator/internal/model"
	"goreadmenator/internal/store"
	"goreadmenator/internal/xmldoc"
)

// Prefix kennzeichnet virtuelle Dateien eines Imports. update.fullBuild und das Inkrement
// lassen sie unberührt, weil sie nicht aus Git stammen.
const Prefix = "xml:"

// Result beschreibt den Import.
type Result struct {
	Assembly string
	Path     string // virtueller Pfad des Assemblys
	Members  int
	Symbols  int
	Crefs    int
	Replaced bool // ein früherer Import desselben Assemblys wurde ersetzt
}

// Import liest die XML-Datei und schreibt ihre Symbole in den Store.
func Import(ctx context.Context, st store.Store, xmlPath string) (Result, error) {
	var res Result
	f, err := os.Open(xmlPath)
	if err != nil {
		return res, err
	}
	defer f.Close()
	doc, err := xmldoc.Parse(f)
	if err != nil {
		return res, err
	}
	if doc.Assembly == "" {
		doc.Assembly = trimExt(filepath.Base(xmlPath))
	}
	res.Assembly = doc.Assembly
	res.Path = "bin/" + doc.Assembly + ".dll"
	res.Members = len(doc.Members)

	if old, ok, err := st.FileByPath(ctx, res.Path); err != nil {
		return res, err
	} else if ok {
		if err := st.DeleteFile(ctx, old.ID); err != nil {
			return res, err
		}
		res.Replaced = true
	}
	fid, err := st.PutFile(ctx, model.File{Path: res.Path, Lang: model.LangCSharp, BlobSHA: Prefix + doc.Assembly})
	if err != nil {
		return res, err
	}

	syms := xmldoc.Symbols(doc)
	batch := store.Batch{}
	for _, s := range syms {
		batch.Symbols = append(batch.Symbols, s)
		batch.Locations = append(batch.Locations, model.Location{SymbolID: s.ID, FileID: fid, Role: model.RoleDecl})
		if s.Kind != model.KindNamespace && s.Container != "" {
			batch.Relations = append(batch.Relations, model.Relation{FileID: fid, From: s.Container, To: s.ID, Kind: model.RelContains})
		}
	}
	// Crefs: explizite Verweise aus der Dokumentation, Fundort ist das Member.
	for _, m := range doc.Members {
		id, ok := xmldoc.IDOf(m)
		if !ok {
			continue
		}
		for _, c := range m.Crefs {
			batch.References = append(batch.References, model.Reference{
				FileID: fid, TargetName: c, Kind: model.RefCref, Enclosing: id, Confidence: model.ConfExact,
			})
			res.Crefs++
		}
	}
	res.Symbols = len(syms)
	if err := st.WriteBatch(ctx, batch); err != nil {
		return res, err
	}
	return res, nil
}

func trimExt(name string) string {
	if e := filepath.Ext(name); e != "" {
		return name[:len(name)-len(e)]
	}
	return name
}
