package term

import (
	"strings"
	"testing"
)

func mkEntry(fields []Value, langs ...Language) Entry {
	return Entry{ID: "c1", Fields: fields, Languages: langs}
}

func de(terms ...Term) Language { return Language{Lang: "de-DE", Terms: terms} }
func en(terms ...Term) Language { return Language{Lang: "en-GB", Terms: terms} }

func paths(cs []Conflict) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Path)
	}
	return strings.Join(out, " | ")
}

func TestMerge3(t *testing.T) {
	base := mkEntry([]Value{{"Subject", "Law"}}, de(Term{Text: "Kaution"}), en(Term{Text: "deposit"}))

	// One side changes, the other does not: take the change.
	ours := mkEntry([]Value{{"Subject", "Law"}}, de(Term{Text: "Kaution", Status: StatusApproved}), en(Term{Text: "deposit"}, Term{Text: "security deposit"}))
	theirs := mkEntry([]Value{{"Subject", "Tenancy law"}}, de(Term{Text: "Kaution"}), en(Term{Text: "deposit"}))
	got, cs := Merge3(base, ours, theirs)
	if len(cs) != 0 {
		t.Fatalf("conflicts: %v", cs)
	}
	want := mkEntry([]Value{{"Subject", "Tenancy law"}}, de(Term{Text: "Kaution", Status: StatusApproved}), en(Term{Text: "deposit"}, Term{Text: "security deposit"}))
	if !sameEntry(got, want) {
		t.Fatalf("merged:\n got %+v\nwant %+v", got, want)
	}

	// The same change on both sides is no conflict.
	got, cs = Merge3(base, theirs, theirs)
	if len(cs) != 0 || !sameEntry(got, theirs) {
		t.Fatalf("same change: %v %+v", cs, got)
	}

	// Different changes to one place: conflict, theirs kept there.
	ours2 := mkEntry([]Value{{"Subject", "Contract law"}}, de(Term{Text: "Kaution", Status: StatusForbidden}), en(Term{Text: "deposit"}))
	theirs2 := mkEntry([]Value{{"Subject", "Tenancy law"}}, de(Term{Text: "Kaution", Status: StatusApproved}), en(Term{Text: "deposit"}))
	got, cs = Merge3(base, ours2, theirs2)
	if paths(cs) != "entry field Subject | de-DE term Kaution status" {
		t.Fatalf("conflicts: %v", paths(cs))
	}
	if cs[0].Base != "Law" || cs[0].Ours != "Contract law" || cs[0].Theirs != "Tenancy law" {
		t.Fatalf("conflict values: %+v", cs[0])
	}
	if !sameEntry(got, theirs2) {
		t.Fatalf("theirs not kept: %+v", got)
	}

	// Removal on one side, no change on the other: removed.
	ours3 := mkEntry([]Value{{"Subject", "Law"}}, de(Term{Text: "Kaution"}))
	got, cs = Merge3(base, ours3, base)
	if len(cs) != 0 || len(got.Languages) != 1 {
		t.Fatalf("language removal: %v %+v", cs, got)
	}
	// Removal on one side, change on the other: conflict, the change kept.
	theirs4 := mkEntry([]Value{{"Subject", "Law"}}, de(Term{Text: "Kaution"}), en(Term{Text: "deposit", Status: StatusApproved}))
	got, cs = Merge3(base, ours3, theirs4)
	if paths(cs) != "en-GB language" || len(got.Languages) != 2 {
		t.Fatalf("remove against change: %v %+v", paths(cs), got)
	}
	// Term removed on one side.
	ours5 := mkEntry([]Value{{"Subject", "Law"}}, de(), en(Term{Text: "deposit"}))
	got, _ = Merge3(base, ours5, base)
	if len(got.Lang("de-DE").Terms) != 0 {
		t.Fatalf("term removal: %+v", got)
	}
	// The same term added on both sides with different statuses.
	o6 := mkEntry(nil, de(Term{Text: "Kaution"}, Term{Text: "Mietsicherheit", Status: StatusApproved}), en(Term{Text: "deposit"}))
	t6 := mkEntry(nil, de(Term{Text: "Kaution"}, Term{Text: "Mietsicherheit", Status: StatusProposed}), en(Term{Text: "deposit"}))
	_, cs = Merge3(mkEntry(nil, de(Term{Text: "Kaution"}), en(Term{Text: "deposit"})), o6, t6)
	if paths(cs) != "de-DE term Mietsicherheit status" {
		t.Fatalf("added on both: %v", paths(cs))
	}
}

func sameEntry(a, b Entry) bool {
	if !sameValues(a.Fields, b.Fields) || len(a.Languages) != len(b.Languages) {
		return false
	}
	for i := range a.Languages {
		if !sameLanguage(a.Languages[i], b.Languages[i]) {
			return false
		}
	}
	return true
}

func TestDiff(t *testing.T) {
	a := mkEntry([]Value{{"Subject", "Law"}}, de(Term{Text: "Kaution"}), en(Term{Text: "deposit"}))
	b := mkEntry([]Value{{"Subject", "Tenancy law"}}, de(Term{Text: "Kaution", Status: StatusApproved}, Term{Text: "Mietsicherheit"}), Language{Lang: "fr-FR", Terms: []Term{{Text: "caution"}}})
	var got []string
	for _, c := range Diff(a, b) {
		got = append(got, c.Path+": "+c.Old+" > "+c.New)
	}
	want := "entry field Subject: Law > Tenancy law | de-DE term Kaution status:  > approved | de-DE term Mietsicherheit:  > Mietsicherheit | fr-FR language:  > added | fr-FR term caution:  > caution | en-GB language: present > removed"
	if strings.Join(got, " | ") != want {
		t.Fatalf("diff:\n got %s\nwant %s", strings.Join(got, " | "), want)
	}
	if len(Diff(a, a)) != 0 {
		t.Fatal("diff of equal entries")
	}
}
