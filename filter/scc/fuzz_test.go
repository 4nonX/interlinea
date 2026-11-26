package scc

import (
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/fuzzseed"
)

// FuzzSCC reads caption files, translates what it reads into text closed
// captions can show, and reads the translation back.
func FuzzSCC(f *testing.F) {
	f.Add([]byte(sample))
	f.Add([]byte("Scenarist_SCC V1.0\n\n00:00:00;29\t9420 9420 94ae 94ae 1370 1370 " + hexOf("Hi there") + " 942c 942c 942f 942f\n"))
	f.Add([]byte("Scenarist_SCC V1.0\n\n00:00:01:00\t1c20 1c20 1cae 1cae 1c70 1c70 " + hexOf("Two") + " 1c2f 1c2f\n"))
	f.Add([]byte(rollSample))
	fuzzseed.Add(f, "filter/scc/testdata/*/*.scc")
	f.Fuzz(func(t *testing.T, data []byte) {
		ex, err := Extract(data, "en")
		if err != nil {
			return
		}
		tr := filter.Translations{}
		for _, u := range ex.Units {
			var c content.Content
			for _, r := range u.Source {
				if r.Tag != nil {
					c = c.AppendTag(*r.Tag)
				} else {
					c = c.Append(strings.ToUpper(strings.Map(func(r rune) rune {
						if _, err := encode(r); err != nil {
							return 'x'
						}
						return r
					}, r.Text)))
				}
			}
			tr[u.ID] = c
		}
		out, err := Merge(data, "en", "de", tr)
		if err != nil {
			if strings.Contains(err.Error(), "rows") || strings.Contains(err.Error(), "empty") || strings.Contains(err.Error(), "can't show") {
				return
			}
			t.Fatalf("a file that reads does not merge: %v", err)
		}
		back, err := Extract(out, "de")
		if err != nil {
			t.Fatalf("the translation does not read: %v\n%s", err, out)
		}
		roll := false
		for _, u := range ex.Units {
			roll = roll || strings.Contains(u.Notes[len(u.Notes)-1], "roll-up")
		}
		// A roll-up line longer than a row rolls on to a next one.
		if len(back.Units) < len(ex.Units) || len(back.Units) > len(ex.Units) && !roll {
			t.Fatalf("%d captions written, %d read back:\n%s\n%s", len(ex.Units), len(back.Units), data, out)
		}
	})
}
