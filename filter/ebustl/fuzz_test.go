package ebustl

import (
	"testing"

	"github.com/4nonX/interlinea/filter/formats/formatstest"
)

// FuzzSTL reads EBU STL files, translates what it reads, and reads the
// translation back: whatever a file holds, nothing panics, and what is
// written can be read.
func FuzzSTL(f *testing.F) {
	f.Add(formatstest.Subtitles())
	f.Add(makeSTL('1', cue{sn: 1, vp: 20, text: []byte("\x0D\x0B\x0BHallo\x0A\x0A\x8A\x8A\x0D\x0B\x0B\x03Welt\x0A\x0A")}))
	f.Add(makeSTL('0', cue{sn: 1, vp: 20, ebn: 0, text: []byte("Gr\xC8u\xFBe\x80x")}, cue{sn: 1, ebn: 0xFE}, cue{sn: 2, vp: 1, text: []byte("\x81\x85y")}))
	f.Fuzz(func(t *testing.T, data []byte) {
		ex, err := Extract(data, "de-DE")
		if err != nil {
			return
		}
		out, err := Merge(data, "de-DE", "fr-FR", upper(ex))
		if err != nil {
			return
		}
		back, err := Extract(out, "fr-FR")
		if err != nil {
			t.Fatalf("the translation does not read: %v", err)
		}
		if len(back.Units) != len(ex.Units) {
			t.Fatalf("%d subtitles written, %d read back", len(ex.Units), len(back.Units))
		}
	})
}
