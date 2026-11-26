package term

import (
	"strings"
	"testing"
	"time"
)

// A header cell of many words is not tried word by word against every
// language name: a file with one would keep a server busy for minutes.
func TestLongHeaderIsQuick(t *testing.T) {
	data := []byte("Deutsch;" + strings.Repeat("Wort ", 50000) + "\nHaus;x\n")
	start := time.Now()
	f, err := Detect("long.csv", data)
	if err != nil {
		t.Fatal(err)
	}
	Read(f, data, nil)
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("reading took %v", took)
	}
}

// Headers that name a language before or after a field still do.
func TestHeaderLanguageAndField(t *testing.T) {
	for h, want := range map[string][2]string{
		"German Definition":                     {"de", "Definition"},
		"Status Englisch":                       {"en", "Status"},
		"Note Arabic (United Arab Emirates)":    {"ar-AE", "Note"},
		"Arabic (United Arab Emirates) Synonym": {"ar-AE", "Synonym"},
	} {
		l, f := splitLangField(h)
		if l != want[0] || f != want[1] {
			t.Errorf("%q: %q, %q; want %q, %q", h, l, f, want[0], want[1])
		}
	}
}
