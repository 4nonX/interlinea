package textfmt

import "testing"

var subtitleFormats = []string{SRT, VTT, SBV, ASS, MicroDVD}

// FuzzSubtitles reads subtitles in each format, translates what it reads,
// and reads the translation back.
func FuzzSubtitles(f *testing.F) {
	f.Add(uint8(0), []byte("1\n00:00:01,000 --> 00:00:02,000\n<i>Hi</i>\nthere\n"))
	f.Add(uint8(1), []byte("WEBVTT\n\n00:01.000 --> 00:02.000\n<v A>Hi\n"))
	f.Add(uint8(2), []byte("0:00:01.000,0:00:02.000\nHi\nthere\n"))
	f.Add(uint8(3), []byte("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\i1}Hi{\\i0}\\Nthere\\hyou\n"))
	f.Add(uint8(4), []byte("{1}{1}25\n{25}{50}{y:i}Hi|there\n"))
	f.Add(uint8(4), []byte("00:00:01.00,00:00:03.50\nHi[br]there\n"))
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		format := subtitleFormats[int(which)%len(subtitleFormats)]
		ex, err := Extract(format, data, "de-DE")
		if err != nil {
			return
		}
		out, err := Merge(format, data, "de-DE", "fr-FR", translate(ex))
		if err != nil {
			t.Fatalf("a file that reads does not merge: %v", err)
		}
		back, err := Extract(format, out, "fr-FR")
		if err != nil {
			t.Fatalf("the translation does not read: %v\n%q", err, out)
		}
		if len(back.Units) != len(ex.Units) {
			t.Fatalf("%d subtitles written, %d read back:\n%q\n%q", len(ex.Units), len(back.Units), data, out)
		}
	})
}
