package ttml

import "testing"

// FuzzTTML reads Timed Text, translates what it reads, and reads the
// translation back.
func FuzzTTML(f *testing.F) {
	f.Add([]byte(sample))
	f.Add([]byte(`<tt:tt xmlns:tt="http://www.w3.org/ns/ttml"><tt:body><tt:div><tt:p begin="1s" xml:space="preserve">a<tt:br/>b</tt:p></tt:div></tt:body></tt:tt>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		ex, err := Extract(data, "de-DE")
		if err != nil {
			return
		}
		out, err := Merge(data, "de-DE", "fr-FR", upper(ex))
		if err != nil {
			t.Fatalf("a file that reads does not merge: %v", err)
		}
		back, err := Extract(out, "fr-FR")
		if err != nil {
			t.Fatalf("the translation does not read: %v\n%s", err, out)
		}
		if len(back.Units) != len(ex.Units) {
			t.Fatalf("%d subtitles written, %d read back:\n%s", len(ex.Units), len(back.Units), out)
		}
	})
}
