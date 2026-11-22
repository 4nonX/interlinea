package term

import (
	"reflect"
	"testing"
)

func entry(id string, langs ...Language) Entry { return Entry{ID: id, Languages: langs} }

func lang(code string, terms ...Term) Language { return Language{Lang: code, Terms: terms} }

func ok(text string) Term  { return Term{Text: text, Status: StatusApproved} }
func bad(text string) Term { return Term{Text: text, Status: StatusForbidden} }

func testIndex() *Index {
	x := NewIndex()
	x.Add("project", []Entry{
		entry("p1", lang("de-DE", ok("Kaution")), lang("en-GB", ok("rental deposit"))),
	})
	x.Add("master", []Entry{
		entry("e1", lang("de-DE", ok("Mietsache")), lang("en-GB", ok("leased property"), Term{Text: "rented property", Status: StatusProposed})),
		entry("e2", lang("de-DE", ok("Kaution"), ok("Mietsicherheit")), lang("en-GB", ok("deposit"), ok("security deposit"))),
		entry("e3", lang("de-DE", ok("Betriebskosten")), lang("en-GB", ok("service charges"), bad("operating costs"))),
		entry("e4", lang("de-DE", ok("ordentliche Kündigung")), lang("en-GB", ok("termination with notice"))),
		entry("e5", lang("de-DE", ok("Kündigung")), lang("en-GB", ok("termination"))),
		entry("e6", lang("de-DE", ok("Miete")), lang("en-GB", ok("rent"))),
		entry("e7", lang("de-DE", ok("Vertrag")), lang("en-GB", ok("contract"))),
		entry("e8", lang("de-DE", ok("Kauf")), lang("en-GB", ok("purchase"))),
		entry("e9", lang("de-DE", bad("Vermieterin")), lang("en-GB", bad("landlady"))),
		entry("e10", lang("de", ok("Mieter")), lang("en", ok("tenant"))),
	})
	return x
}

type found struct {
	text, entry string
	exact       bool
}

func recognized(x *Index, text, src, tgt string) []found {
	var out []found
	for _, h := range x.Recognize(text, src, tgt) {
		out = append(out, found{text[h.Start:h.End], h.Source + "/" + h.EntryID, h.Exact})
	}
	return out
}

func TestRecognizeInflection(t *testing.T) {
	x := testIndex()
	cases := []struct {
		text string
		want []found
	}{
		{"Die Kautionen der Mietsachen.", []found{{"Kautionen", "project/p1", false}, {"Kautionen", "master/e2", false}, {"Mietsachen", "master/e1", false}}},
		{"Nach der ordentlichen Kündigung des Mieters", []found{{"ordentlichen Kündigung", "master/e4", false}, {"Mieters", "master/e10", false}}},
		{"Die Miete und der Mieter", []found{{"Miete", "master/e6", true}, {"Mieter", "master/e10", true}}},
		{"beide Verträge, alle Verträgen", []found{{"Verträge", "master/e7", false}, {"Verträgen", "master/e7", false}}},
		{"Der Käufer zahlt.", nil},
		{"Eine Kündigung; ordentliche, fristlose Kündigung", []found{{"Kündigung", "master/e5", true}, {"Kündigung", "master/e5", true}}},
		{"Die Betriebskosten-Abrechnung", []found{{"Betriebskosten", "master/e3", true}}},
		{"Untermietsache", nil},
	}
	for _, c := range cases {
		got := recognized(x, c.text, "de-DE", "en-GB")
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q:\n got  %v\n want %v", c.text, got, c.want)
		}
	}
}

func TestRecognizeTargetsAndLanguages(t *testing.T) {
	x := testIndex()
	hits := x.Recognize("Betriebskosten", "de-AT", "en-US")
	if len(hits) != 1 || hits[0].Targets[0].Text != "service charges" || hits[0].Targets[1].Status != StatusForbidden {
		t.Fatalf("primary-language fallback and target order: %+v", hits)
	}
	hits = x.Recognize("Mietsache", "de-DE", "en-GB")
	if hits[0].Targets[0].Text != "leased property" || hits[0].Targets[1].Status != StatusProposed {
		t.Fatalf("approved first: %+v", hits[0].Targets)
	}
	// Forbidden source terms are not offered.
	if hits := x.Recognize("Die Vermieterin", "de-DE", "en-GB"); len(hits) != 0 {
		t.Fatalf("forbidden source term recognized: %+v", hits)
	}
	// Offsets are bytes into the text.
	text := "Über die Kaution"
	hits = x.Recognize(text, "de-DE", "en-GB")
	if text[hits[0].Start:hits[0].End] != "Kaution" {
		t.Fatalf("offsets: %+v", hits[0])
	}
}

func TestRecognizeEnglishAndOthers(t *testing.T) {
	x := NewIndex()
	x.Add("tb", []Entry{
		entry("a", lang("en", ok("security deposit")), lang("de", ok("Kaution"))),
		entry("b", lang("en", ok("party")), lang("de", ok("Partei"))),
		entry("c", lang("en", ok("lease")), lang("de", ok("Mietvertrag"))),
		entry("d", lang("ko", ok("임대인")), lang("de", ok("Vermieter"))),
		entry("e", lang("ja", ok("賃貸借契約")), lang("de", ok("Mietvertrag"))),
		entry("f", lang("it", ok("contratto")), lang("de", ok("Vertrag"))),
	})
	if got := recognized(x, "The tenant's security deposits and both parties' leases.", "en-US", "de"); !reflect.DeepEqual(got, []found{{"security deposits", "tb/a", false}, {"parties", "tb/b", false}, {"leases", "tb/c", false}}) {
		t.Errorf("English: %v", got)
	}
	if got := recognized(x, "pleased", "en", "de"); got != nil {
		t.Errorf("no match inside words: %v", got)
	}
	if got := recognized(x, "임대인은 임차인에게", "ko", "de"); !reflect.DeepEqual(got, []found{{"임대인은", "tb/d", false}}) {
		t.Errorf("Korean particles: %v", got)
	}
	if got := recognized(x, "本賃貸借契約は", "ja", "de"); !reflect.DeepEqual(got, []found{{"賃貸借契約", "tb/e", true}}) {
		t.Errorf("Japanese substring: %v", got)
	}
	if got := recognized(x, "i contratti", "it", "de"); !reflect.DeepEqual(got, []found{{"contratti", "tb/f", false}}) {
		t.Errorf("Italian plural: %v", got)
	}
}

func TestCheck(t *testing.T) {
	x := testIndex()
	src := "Die Betriebskosten und die Mietsache."
	hits := x.Recognize(src, "de-DE", "en-GB")
	issues := x.Check(hits, "The operating costs and the leased properties.", "en-GB")
	want := []Issue{
		{Kind: "forbidden", Term: "Betriebskosten", Found: "operating costs", Start: 4, End: 19, EntryID: "e3"},
		{Kind: "missing", Term: "Betriebskosten", Expected: "service charges", EntryID: "e3"},
	}
	if !reflect.DeepEqual(issues, want) {
		t.Fatalf("issues:\n got  %+v\n want %+v", issues, want)
	}
	if issues := x.Check(hits, "The service charges and the leased property.", "en-GB"); len(issues) != 0 {
		t.Fatalf("clean translation: %+v", issues)
	}
	// A proposed translation counts as present.
	if issues := x.Check(hits, "The service charges and the rented property.", "en-GB"); len(issues) != 0 {
		t.Fatalf("proposed term: %+v", issues)
	}
	// A forbidden term no source term led to is still reported.
	issues = x.Check(nil, "The landlady signs.", "en-GB")
	if len(issues) != 1 || issues[0].Found != "landlady" || issues[0].Term != "" {
		t.Fatalf("global forbidden: %+v", issues)
	}
	if x.Check(hits, "  ", "en-GB") != nil {
		t.Fatal("empty target should give no issues")
	}
	// German target inflection: "Mietsachen" satisfies "Mietsache".
	y := NewIndex()
	y.Add("tb", []Entry{entry("a", lang("en", ok("leased property")), lang("de", ok("Mietsache")))})
	if issues := y.Check(y.Recognize("the leased property", "en", "de"), "der Mietsachen", "de"); len(issues) != 0 {
		t.Fatalf("inflected target: %+v", issues)
	}
}
