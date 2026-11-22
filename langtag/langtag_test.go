package langtag

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"de-de":      "de-DE",
		"EN_gb":      "en-GB",
		"zh-hant-tw": "zh-Hant-TW",
		"es-419":     "es-419",
		"ko":         "ko",
		" fr-CA ":    "fr-CA",
		"sr-latn":    "sr-Latn",
		"de-CH-1996": "de-CH-1996",
	}
	for in, want := range cases {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "-", "e", "de--DE", "1de", "de-DE!", "toolongsubtag"} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q): expected error", bad)
		}
	}
}

func TestMatch(t *testing.T) {
	if Match("de-DE", "de_de") != ExactMatch {
		t.Error("exact")
	}
	if Match("de-AT", "de") != PrimaryMatch {
		t.Error("primary")
	}
	if Match("de", "en") != NoMatch {
		t.Error("none")
	}
	if Match("", "de") != NoMatch {
		t.Error("invalid")
	}
	if Primary("EN_us") != "en" {
		t.Error("Primary")
	}
}

func TestMustNormalizePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	MustNormalize("")
}
