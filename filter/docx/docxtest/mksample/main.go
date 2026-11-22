// Command mksample writes the sample lease used by the prototype.
//
//	go run ./filter/docx/docxtest/mksample prototype/public/samples/Mietvertrag_Hofmann_2026.docx
package main

import (
	"fmt"
	"os"

	"github.com/4nonX/interlinea/filter/docx/docxtest"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mksample out.docx")
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[1], docxtest.Hofmann(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
