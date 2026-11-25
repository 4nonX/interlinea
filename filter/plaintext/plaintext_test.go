package plaintext

import (
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

func TestTextRoundTrip(t *testing.T) {
	text := "Der Schlüssel liegt beim Hausmeister. Die Heizung läuft.  Bitte lüften.\r\n\r\n  Zweite Zeile.\n"
	ex, err := Extract([]byte(text), "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 4 || ex.Units[0].ID != "s1" || ex.Units[3].Block != "p2" || ex.Units[2].Block != "p1" || ex.Units[3].Source.Plain() != "Zweite Zeile." {
		t.Fatalf("units: %+v", ex.Units)
	}
	// Nothing translated: the file as it was.
	out, err := Merge([]byte(text), "de-DE", nil)
	if err != nil || string(out) != text {
		t.Fatalf("untranslated: %q %v", out, err)
	}
	// One sentence translated, two joined into one, line ends and blank lines kept.
	out, err = Merge([]byte(text), "de-DE", filter.Translations{
		"s1": content.Text("The heating is on, and the key is with the caretaker."),
		"s2": filter.Joined(),
		"s4": content.Text("Second line."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "The heating is on, and the key is with the caretaker.  Bitte lüften.\r\n\r\n  Second line.\n"; string(out) != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
	// UTF-16 with its mark goes back as it came.
	wide := Encode(text, []byte{0xFF, 0xFE})
	ex2, err := Extract(wide, "de-DE")
	if err != nil || len(ex2.Units) != 4 {
		t.Fatalf("utf-16: %v", err)
	}
	out, err = Merge(wide, "de-DE", filter.Translations{"s4": content.Text("Second line.")})
	if err != nil {
		t.Fatal(err)
	}
	if back, bom, _ := Decode(out); len(bom) != 2 || back != "Der Schlüssel liegt beim Hausmeister. Die Heizung läuft.  Bitte lüften.\r\n\r\n  Second line.\n" {
		t.Errorf("utf-16 back: %q", back)
	}
	if _, err := Extract([]byte{0xC3, 0x28}, "de-DE"); err == nil {
		t.Error("text that is not UTF-8 is read")
	}
}
