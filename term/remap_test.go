package term

import "testing"

func TestRemap(t *testing.T) {
	entries := []Entry{
		{ID: "1", Languages: []Language{
			{Lang: "de", Terms: []Term{{Text: "Apfel"}}},
			{Lang: "en-GB", Terms: []Term{{Text: "apple", Status: StatusApproved}}},
			{Lang: "en-US", Terms: []Term{{Text: "eating apple"}}, Fields: []Value{{Field: "Note", Value: "US"}}},
			{Lang: "fr", Terms: []Term{{Text: "pomme"}}},
		}},
		{ID: "2", Languages: []Language{{Lang: "fr", Terms: []Term{{Text: "poire"}}}}},
	}
	out, dropped := Remap(entries, map[string]string{"de": "de-DE", "en-US": "en-GB", "fr": ""})
	if dropped != 1 || len(out) != 1 {
		t.Fatalf("%d entries, %d dropped", len(out), dropped)
	}
	e := out[0]
	if len(e.Languages) != 2 || e.Languages[0].Lang != "de-DE" || e.Languages[1].Lang != "en-GB" {
		t.Fatalf("languages %+v", e.Languages)
	}
	if en := e.Languages[1]; len(en.Terms) != 2 || en.Terms[1].Text != "eating apple" || len(en.Fields) != 1 {
		t.Fatalf("joined languages %+v", en)
	}
	if len(entries[0].Languages) != 4 || len(entries[0].Languages[1].Terms) != 1 {
		t.Fatal("the entries given were changed")
	}
	SetDefaultStatus(out, StatusProposed)
	if out[0].Languages[0].Terms[0].Status != StatusProposed || out[0].Languages[1].Terms[0].Status != StatusApproved {
		t.Fatalf("statuses %+v", out[0].Languages)
	}
	if got := entries[0].Label(); got != "Apfel, apple, eating apple, pomme" {
		t.Fatalf("label %q", got)
	}
	tb := &Termbase{Definition: Definition{Languages: []string{"en-GB"}}, Entries: entries}
	if got := tb.FileLanguages(); len(got) != 4 || got[0] != "en-GB" || got[1] != "de" {
		t.Fatalf("file languages %v", got)
	}
}
