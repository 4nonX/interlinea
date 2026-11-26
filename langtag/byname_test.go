package langtag

import "testing"

func TestByName(t *testing.T) {
	for name, want := range map[string]string{
		"German":                   "de",
		"German (Germany)":         "de-DE",
		"english (united states)":  "en-US",
		"English (United Kingdom)": "en-GB",
		"Hmong":                    "hmn",
		"Serbian (Latin, Serbia)":  "sr-Latn-RS",
		"Chinese (Simplified)":     "zh-Hans",
		"Norwegian Bokmal":         "nb",
		"Norwegian (Bokmal)":       "nb",
		"Portuguese (Brazil)":      "pt-BR",
		"French (Belgium)":         "fr-BE",
	} {
		got, ok := ByName(name)
		if !ok || got != want {
			t.Errorf("ByName(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"", "Klingon", "ID", "Status"} {
		if got, ok := ByName(name); ok {
			t.Errorf("ByName(%q) = %q", name, got)
		}
	}
}

func TestFromLCID(t *testing.T) {
	for id, want := range map[int]string{1031: "de-DE", 1033: "en-US", 2057: "en-GB", 2074: "sr-Latn-RS", 3082: "es-ES", 2052: "zh-CN"} {
		if got, ok := FromLCID(id); !ok || got != want {
			t.Errorf("FromLCID(%d) = %q, %v; want %q", id, got, ok, want)
		}
	}
	if _, ok := FromLCID(1024); ok {
		t.Error("1024 is no language")
	}
}
