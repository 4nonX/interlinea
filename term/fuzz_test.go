package term

import (
	"testing"

	"github.com/4nonX/interlinea/internal/fuzzseed"
)

// fuzzNames are the file names a termbase file is read under; the first
// input of the fuzzer picks one, and with it how the file is taken.
var fuzzNames = []string{"x.tbx", "x.xml", "x.csv", "x.tsv", "x.xlsx", "x.sdltb", "x.xdt", "x.txt"}

// FuzzRead reads termbase files: TBX, termbase XML and definitions,
// .sdltb files, CSV, TSV and Excel.
func FuzzRead(f *testing.F) {
	for _, data := range fuzzseed.Files(f, "term/testdata/*") {
		f.Add(uint8(5), data)
	}
	f.Add(uint8(0), []byte(tbx3))
	f.Add(uint8(2), []byte("Deutsch;Englisch;Definition Deutsch;Status Englisch\nMiete;rent;Entgelt für die Nutzung;approved\nKaution;deposit;;\n"))
	f.Add(uint8(3), []byte("de-DE\ten-GB\tNote\nMiete\trent\tmonatlich\n"))
	f.Add(uint8(4), writeXLSX("Terms", [][]string{{"German", "English"}, {"Miete", "rent"}}))
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		name := fuzzNames[int(which)%len(fuzzNames)]
		if format, err := Detect(name, data); err == nil {
			Read(format, data, nil)
		}
	})
}
