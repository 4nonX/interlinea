package rtf

import (
	"testing"

	"github.com/4nonX/interlinea/internal/fuzzseed"
)

// FuzzTokenize reads RTF files (the bilingual tables for review).
func FuzzTokenize(f *testing.F) {
	fuzzseed.Add(f, "filter/xliff/testdata/tables/*.rtf")
	f.Add([]byte(`{\rtf1\ansi\ansicpg1252{\fonttbl{\f0 Arial;}}\f0 Der \b Mieter\b0  zahlt 荤? 5.\par}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		Tokenize(data)
	})
}
