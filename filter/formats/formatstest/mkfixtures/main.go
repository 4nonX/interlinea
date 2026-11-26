// Command mkfixtures writes one document of every format into a folder,
// for the tests of the apps:
//
//	go run ./filter/formats/formatstest/mkfixtures web/e2e/fixtures/documents
//
// The memoQ handoff package and archive are made from the memoQ XLIFF
// file the XLIFF filter is tested with.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/4nonX/interlinea/filter/formats/formatstest"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mkfixtures folder")
		os.Exit(2)
	}
	dir := os.Args[1]
	doc, err := os.ReadFile(filepath.Join("filter", "xliff", "testdata", "memoq.mqxliff"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "run from the repository's root:", err)
		os.Exit(1)
	}
	for name, data := range map[string][]byte{
		"Folien.pptx":       formatstest.Deck(),
		"Tabelle.xlsx":      formatstest.Workbook(),
		"Seite.html":        formatstest.Page(),
		"Layout.idml":       formatstest.Layout(),
		"Hofmann.mqout":     formatstest.Handoff(doc),
		"Mietvertrag.mqxlz": formatstest.Archive(doc),
		"Untertitel.stl":    formatstest.Subtitles(),
		"Untertitel.scc":    []byte(formatstest.Captions),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
