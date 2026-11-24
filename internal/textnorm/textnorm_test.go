package textnorm

import "testing"

func TestNFC(t *testing.T) {
	decomposed := "Café Müller"
	if got := NFC(decomposed); got != "Café Müller" {
		t.Fatalf("NFC = %q", got)
	}
	// Korean syllables stored as their letters compose too.
	if got := NFC("한"); got != "한" {
		t.Fatalf("NFC of jamo = %q", got)
	}
	if got := NFC("plain"); got != "plain" {
		t.Fatalf("NFC = %q", got)
	}
}

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"İSTANBUL":   "istanbul",
		"ΟΔΟΣ":       "οδοσ",
		"οδός":       "οδόσ",
		"STRASSE":    "strasse",
		"Ünïcode":    "ünïcode",
		"ДОГОВОР":    "договор",
		"Café":      "café",
		"ՊԱՅՄԱՆԱԳԻՐ": "պայմանագիր",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripMarks(t *testing.T) {
	for in, want := range map[string]string{
		"العَقْدُ": "العقد",
		"عقـــد":   "عقد",
		"שָׁלוֹם":  "שלום",
		"σύμβαση":  "συμβαση",
		"contract": "contract",
	} {
		if got := StripMarks(in); got != want {
			t.Errorf("StripMarks(%q) = %q, want %q", in, got, want)
		}
	}
}
