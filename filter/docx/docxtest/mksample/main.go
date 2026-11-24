// Command mksample writes the sample documents: the lease the browser client's sample project
// opens and, with -features, the document that exercises text boxes,
// content controls, tracked changes, comments, charts and SmartArt.
//
//	go run ./filter/docx/docxtest/mksample web/samples/Mietvertrag_Hofmann_2026.docx
//	go run ./filter/docx/docxtest/mksample -features features.docx
package main

import (
	"fmt"
	"os"

	"github.com/4nonX/interlinea/filter/docx/docxtest"
)

func main() {
	args := os.Args[1:]
	data := docxtest.Hofmann
	if len(args) == 2 && args[0] == "-features" {
		data, args = docxtest.Features, args[1:]
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: mksample [-features] out.docx")
		os.Exit(2)
	}
	if err := os.WriteFile(args[0], data(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
