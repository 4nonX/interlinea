package mqpackage

import "testing"

// FuzzOpen reads handoff packages (.mqout) and the documents in them.
func FuzzOpen(f *testing.F) {
	data, _ := handoff(f)
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := Open(data)
		if err != nil {
			return
		}
		for _, file := range p.Files {
			if b, err := Read(data, file.Path); err == nil {
				ReadArchive(b)
			}
		}
	})
}
