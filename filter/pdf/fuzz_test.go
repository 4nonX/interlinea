package pdf

import (
	"testing"

	"github.com/4nonX/interlinea/internal/fuzzseed"
)

// FuzzToWord reads PDF files into Word documents.
func FuzzToWord(f *testing.F) {
	fuzzseed.Add(f, "filter/pdf/testdata/*.pdf")
	f.Fuzz(func(t *testing.T, data []byte) {
		ToWord(data)
	})
}
