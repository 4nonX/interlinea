package sdlpackage

import "testing"

// FuzzOpen reads project packages (.sdlppx) and the files in them.
func FuzzOpen(f *testing.F) {
	x := sdlxliff(f)
	f.Add(makePackage(f, manifest, map[string][]byte{"de-DE/Mietvertrag.docx.sdlxliff": x, "en-GB/Mietvertrag.docx.sdlxliff": x, "Tm/Weber.sdltm": []byte("SQLite format 3")}))
	f.Add(makePackage(f, "", map[string][]byte{"en-GB/Anlage.docx.sdlxliff": x}))
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := Open(data)
		if err != nil {
			return
		}
		for _, file := range p.Files {
			Read(data, file.Path)
		}
	})
}
