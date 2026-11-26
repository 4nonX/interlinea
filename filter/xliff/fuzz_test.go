package xliff

import (
	"testing"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/fuzzseed"
)

// FuzzExtract reads bilingual files of every kind. A file it reads is
// written back with each unit's source as its translation, and what is
// written reads again, with as many units.
func FuzzExtract(f *testing.F) {
	fuzzseed.Add(f, "filter/xliff/testdata/*", "filter/xliff/testdata/tables/*",
		"internal/realworld/testdata/*/*.sdlxliff", "internal/realworld/testdata/*/*.mqxliff",
		"internal/realworld/testdata/*/*.mxliff", "internal/realworld/testdata/*/*.txml",
		"internal/realworld/testdata/*/*.ttx")
	f.Fuzz(func(t *testing.T, data []byte) {
		Detect(data)
		ex, err := Extract(data)
		if err != nil {
			return
		}
		tr := filter.Translations{}
		for _, u := range ex.Units {
			tr[u.ID] = u.Source
		}
		out, err := Translations(data, tr)
		if err != nil {
			return
		}
		again, err := Extract(out)
		if err != nil {
			t.Fatalf("the file written back is not read: %v", err)
		}
		if len(again.Units) != len(ex.Units) {
			t.Fatalf("%d units read back of %d", len(again.Units), len(ex.Units))
		}
	})
}
