package qa

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/spell"
	"github.com/4nonX/interlinea/term"
)

func seg(id, src, tgt string) Segment {
	return Segment{ID: id, Source: content.Text(src), Target: content.Text(tgt)}
}

func kinds(issues []Issue) []Kind {
	var out []Kind
	for _, i := range issues {
		out = append(out, i.Kind)
	}
	return out
}

func has(issues []Issue, k Kind) bool {
	for _, i := range issues {
		if i.Kind == k {
			return true
		}
	}
	return false
}

func checkOne(t *testing.T, s Segment, o Options) []Issue {
	t.Helper()
	is, err := CheckSegment(s, o)
	if err != nil {
		t.Fatal(err)
	}
	return is
}

var enDe = Options{SourceLang: "en-US", TargetLang: "de-DE"}

func TestCleanSegment(t *testing.T) {
	is := checkOne(t, seg("1", "The deposit is EUR 1,250.50.", "Die Kaution beträgt 1.250,50 EUR."), enDe)
	if len(is) != 0 {
		t.Fatalf("issues on a clean segment: %+v", is)
	}
}

func TestEmptyAndUntranslated(t *testing.T) {
	is := checkOne(t, seg("1", "Hello world.", ""), enDe)
	if len(is) != 1 || is[0].Kind != Empty || is[0].Severity != Error {
		t.Fatalf("empty: %+v", is)
	}
	is = checkOne(t, seg("1", "Hello world.", "Hello  world."), enDe)
	if !has(is, Untranslated) {
		t.Fatalf("untranslated not found: %v", kinds(is))
	}
	// Same language pair (en-US to en-GB) is not untranslated.
	is = checkOne(t, seg("1", "Hello world.", "Hello world."), Options{SourceLang: "en-US", TargetLang: "en-GB"})
	if has(is, Untranslated) {
		t.Fatal("untranslated within one language")
	}
	// Only numbers or codes: not untranslated.
	is = checkOne(t, seg("1", "4.2", "4,2"), enDe)
	if has(is, Untranslated) || len(is) != 0 {
		t.Fatalf("number-only segment: %+v", is)
	}
}

func TestNumbers(t *testing.T) {
	cases := []struct {
		name     string
		src, tgt string
		o        Options
		missing  bool
		extra    bool
	}{
		{"locale separators", "Pay 1,250.50 by 3 May.", "Zahlen Sie 1.250,50 bis zum 3. Mai.", enDe, false, false},
		{"wrong separator", "It is 1.5 m long.", "Es ist 1.5 m lang.", enDe, true, true},
		{"missing", "Rooms 3 and 4.", "Zimmer 3.", enDe, true, false},
		{"extra", "Room 3.", "Zimmer 3 und 4.", enDe, false, true},
		{"repeated value counts", "3 and 3", "3 und 3", enDe, false, false},
		{"repeated value missing", "3 and 3", "3 und drei", enDe, true, false},
		{"French space group", "It costs 1,250.", "Cela coûte 1 250.", Options{SourceLang: "en", TargetLang: "fr-FR"}, false, false},
		{"no-break space group", "10,000 users", "10 000 Nutzer", enDe, false, false},
		{"English space is two numbers", "Pages 5 100", "Seiten 5 100", Options{SourceLang: "en", TargetLang: "en-GB"}, false, false},
		{"Swiss apostrophe", "CHF 1,250.50", "CHF 1'250.50", Options{SourceLang: "en", TargetLang: "de-CH"}, false, false},
		{"trailing zeros", "EUR 1,250.00", "1.250 EUR", enDe, false, false},
		{"leading zeros differ", "Item 007", "Artikel 7", enDe, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			is := checkOne(t, seg("1", c.src, c.tgt), c.o)
			if has(is, NumbersMissing) != c.missing || has(is, NumbersExtra) != c.extra {
				t.Fatalf("got %v, want missing=%v extra=%v", is, c.missing, c.extra)
			}
		})
	}
	is := checkOne(t, seg("1", "Rooms 3, 4 and 5.", "Zimmer."), enDe)
	for _, i := range is {
		if i.Kind == NumbersMissing && i.Message != "The numbers 3, 4 and 5 from the source are not in the translation." {
			t.Fatalf("message: %q", i.Message)
		}
	}
}

func tagged(parts ...any) content.Content {
	var c content.Content
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			c = c.Append(v)
		case content.Tag:
			c = c.AppendTag(v)
		}
	}
	return c
}

func open(id string) content.Tag  { return content.Tag{Kind: content.TagOpen, ID: id} }
func close(id string) content.Tag { return content.Tag{Kind: content.TagClose, ID: id} }
func ph(id string) content.Tag    { return content.Tag{Kind: content.TagStandalone, ID: id} }

func TestTags(t *testing.T) {
	src := tagged("A ", open("1"), "bold", close("1"), " and ", open("2"), "it", ph("3"), close("2"), ".")
	ok := Segment{ID: "1", Source: src, Target: tagged("Ein ", open("2"), "it", ph("3"), close("2"), " und ", open("1"), "fett", close("1"), ".")}
	if is := checkOne(t, ok, enDe); len(is) != 0 {
		t.Fatalf("reordered spans: %+v", is)
	}
	missing := Segment{ID: "1", Source: src, Target: tagged("Ein ", open("1"), "fett", close("1"), " und kursiv.")}
	is := checkOne(t, missing, enDe)
	if !has(is, TagsMissing) || is[0].Message != "3 tags of the source are missing." {
		t.Fatalf("missing: %+v", is)
	}
	extra := Segment{ID: "1", Source: tagged("Plain."), Target: tagged("Ein", ph("9"), ".")}
	if is := checkOne(t, extra, enDe); !has(is, TagsExtra) {
		t.Fatalf("extra: %+v", is)
	}
	nested := Segment{ID: "1", Source: tagged(open("1"), "a ", open("2"), "b", close("2"), close("1"), "."),
		Target: tagged(open("2"), "a ", open("1"), "b", close("1"), close("2"), ".")}
	if is := checkOne(t, nested, Options{SourceLang: "en", TargetLang: "de"}); !has(is, TagOrder) {
		t.Fatalf("nesting: %+v", is)
	}
	crossed := Segment{ID: "1", Source: nested.Source, Target: tagged(open("1"), "a ", open("2"), "b", close("1"), close("2"), ".")}
	if is := checkOne(t, crossed, Options{SourceLang: "en", TargetLang: "de"}); !has(is, TagOrder) {
		t.Fatalf("crossed: %+v", is)
	}
	// A target with only tags is not empty.
	onlyTags := Segment{ID: "1", Source: tagged(ph("1")), Target: tagged(ph("1"))}
	if is := checkOne(t, onlyTags, enDe); len(is) != 0 {
		t.Fatalf("tag-only: %+v", is)
	}
}

func TestPunctuation(t *testing.T) {
	cases := []struct {
		name     string
		src, tgt string
		o        Options
		want     Kind
		none     bool
	}{
		{"end differs", "Is it ready?", "Ist es fertig.", enDe, EndPunctuation, false},
		{"end missing", "It is ready.", "Es ist fertig", enDe, EndPunctuation, false},
		{"end extra", "Ready", "Fertig.", enDe, EndPunctuation, false},
		{"end inside brackets", "(See below.)", "(Siehe unten.)", enDe, "", true},
		{"Chinese stop", "It is ready.", "准备好了。", Options{SourceLang: "en", TargetLang: "zh-CN"}, "", true},
		{"Greek question", "Is it ready?", "Είναι έτοιμο;", Options{SourceLang: "en", TargetLang: "el"}, "", true},
		{"ellipsis", "Wait...", "Warten …", enDe, "", true},
		{"double space", "One two.", "Eins  zwei.", enDe, DoubleSpace, false},
		{"leading space", "One.", " Eins.", enDe, EdgeSpace, false},
		{"trailing space kept", "One. ", "Eins. ", enDe, "", true},
		{"space before comma", "One, two.", "Eins , zwei.", enDe, SpaceBeforePunct, false},
		{"French missing space", "Really?", "Vraiment?", Options{SourceLang: "en", TargetLang: "fr"}, SpaceBeforePunct, false},
		{"French narrow space", "Really?", "Vraiment ?", Options{SourceLang: "en", TargetLang: "fr"}, "", true},
		{"French guillemets", "He said \"yes\".", "Il a dit «oui».", Options{SourceLang: "en", TargetLang: "fr"}, SpaceBeforePunct, false},
		{"French time", "At 10:30.", "À 10:30.", Options{SourceLang: "en", TargetLang: "fr"}, "", true},
		{"brackets", "See (a) and (b).", "Siehe (a und (b).", enDe, Brackets, false},
		{"repeated word", "The lease ends.", "Der der Vertrag endet.", enDe, RepeatedWord, false},
		{"repeated in source too", "Bye bye.", "Tschüss tschüss.", enDe, "", true},
		{"capital lost", "The lease ends.", "der Vertrag endet.", enDe, Capitalization, false},
		{"German noun capital", "lease ends", "Vertrag endet", enDe, "", true},
		{"Spanish inverted mark", "Is it ready?", "¿Está listo?", Options{SourceLang: "en", TargetLang: "es"}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			is := checkOne(t, seg("1", c.src, c.tgt), c.o)
			if c.none {
				if len(is) != 0 {
					t.Fatalf("unexpected issues: %+v", is)
				}
				return
			}
			if !has(is, c.want) {
				t.Fatalf("want %s, got %+v", c.want, is)
			}
		})
	}
}

func TestLength(t *testing.T) {
	s := seg("1", "Save", "Speichern unter")
	s.MaxLength = 10
	is := checkOne(t, s, enDe)
	if !has(is, Length) || is[0].Message != "The translation has 15 characters; at most 10 are allowed." {
		t.Fatalf("length: %+v", is)
	}
	long := seg("1", "The tenant pays the rent.", "Der Mieter zahlt die Miete pünktlich jeden Monat am ersten Werktag.")
	if has(checkOne(t, long, enDe), LengthRatio) {
		t.Fatal("ratio check is off by default")
	}
	o := enDe
	o.Enabled = []Kind{LengthRatio}
	if !has(checkOne(t, long, o), LengthRatio) {
		t.Fatal("ratio check enabled but silent")
	}
	o.Disabled = []Kind{LengthRatio}
	if has(checkOne(t, long, o), LengthRatio) {
		t.Fatal("disabled wins over enabled")
	}
}

func TestConsistency(t *testing.T) {
	segs := []Segment{
		seg("a", "Terminate the lease.", "Kündigen Sie den Vertrag."),
		seg("b", "Terminate the lease.", "Beenden Sie den Vertrag."),
		seg("c", "terminate  the lease.", "Kündigen Sie den Vertrag."),
		seg("d", "Pay the rent.", "Zahlen Sie."),
		seg("e", "Pay the deposit.", "Zahlen Sie."),
	}
	is, err := Check(segs, enDe)
	if err != nil {
		t.Fatal(err)
	}
	var inc []string
	for _, i := range is {
		if i.Kind == Inconsistent {
			inc = append(inc, i.Segment)
			if !strings.Contains(i.Message, "in 2 ways") {
				t.Fatalf("message: %q", i.Message)
			}
		}
		if i.Kind == InconsistentSrc {
			t.Fatal("reverse check is off by default")
		}
	}
	if strings.Join(inc, ",") != "a,b,c" {
		t.Fatalf("inconsistent: %v", inc)
	}
	o := enDe
	o.Enabled = []Kind{InconsistentSrc}
	is, _ = Check(segs, o)
	var rev []string
	for _, i := range is {
		if i.Kind == InconsistentSrc {
			rev = append(rev, i.Segment)
		}
	}
	if strings.Join(rev, ",") != "d,e" {
		t.Fatalf("reverse: %v", rev)
	}
	// Issues come in segment order, and skipped segments are left out.
	segs[1].Skip = true
	is, _ = Check(segs, enDe)
	for _, i := range is {
		if i.Segment == "b" || i.Kind == Inconsistent {
			t.Fatalf("skipped segment counted: %+v", i)
		}
	}
}

func TestTerms(t *testing.T) {
	x := term.NewIndex()
	x.Add("tb", []term.Entry{{ID: "c1", Languages: []term.Language{
		{Lang: "en", Terms: []term.Term{{Text: "deposit"}}},
		{Lang: "de", Terms: []term.Term{{Text: "Kaution", Status: "preferred"}, {Text: "Pfand", Status: "forbidden"}}},
	}}})
	o := enDe
	o.Terms = x
	is := checkOne(t, seg("1", "The deposit is due.", "Das Pfand ist fällig."), o)
	if !has(is, TermMissing) || !has(is, TermForbidden) {
		t.Fatalf("terms: %+v", is)
	}
	if is := checkOne(t, seg("1", "The deposits are due.", "Die Kautionen sind fällig."), o); len(is) != 0 {
		t.Fatalf("inflected term: %+v", is)
	}
}

type fakeSpeller map[string]bool

func (f fakeSpeller) Spell(w string) bool { return f[strings.ToLower(w)] }
func (f fakeSpeller) Suggest(w string, max int) []string {
	if w == "Vertarg" {
		return []string{"Vertrag"}
	}
	return nil
}

func TestSpelling(t *testing.T) {
	o := enDe
	o.Spell = fakeSpeller{"der": true, "endet": true, "am": true, "mai": true, "mit": true}
	o.SpellIgnore = []string{"Hofmann"}
	is := checkOne(t, seg("1", "The Acme lease with Hofmann ends in May.", "Der Vertarg mit Acme und Hofmann endet am 3. Mai, siehe www.acme.example und info@acme.example, Nr. A12 GmbH."), o)
	var found []string
	var first Issue
	for _, i := range is {
		if i.Kind == Spelling {
			if found == nil {
				first = i
			}
			found = append(found, i.Found)
		}
	}
	// Vertarg is wrong; und, siehe, Nr are unknown to the tiny
	// dictionary; Acme is in the source, Hofmann ignored, A12 has a digit,
	// GmbH is not all capitals so it counts, addresses are skipped.
	if strings.Join(found, ",") != "Vertarg,und,siehe,Nr,GmbH" {
		t.Fatalf("spelling: %v", found)
	}
	if len(first.Suggestions) != 1 || first.Suggestions[0] != "Vertrag" {
		t.Fatalf("suggestions: %+v", first)
	}
}

func TestWords(t *testing.T) {
	var got []string
	for _, w := range words("l'été, don’t well-known -x 3rd") {
		got = append(got, w.text)
	}
	if strings.Join(got, "|") != "l'été|don’t|well-known|x|3rd" {
		t.Fatalf("words: %v", got)
	}
}

func TestRules(t *testing.T) {
	o := enDe
	o.Rules = []UserRule{
		{Name: "No Sie", Target: `\bSie\b`, Mode: "must-not", Message: "Use du."},
		{Name: "Euro sign", Source: `EUR`, Target: `€`, Mode: "must"},
		{Name: "Same URL count", Source: `https?://\S+`, Target: `https?://\S+`, Mode: "count", Severity: Error},
	}
	is := checkOne(t, seg("1", "Pay EUR 5 at http://a.example.", "Zahlen Sie 5 EUR."), o)
	var names []string
	for _, i := range is {
		if i.Kind == Rule {
			names = append(names, i.Rule)
			if i.Rule == "No Sie" && (i.Found != "Sie" || i.Message != "Use du.") {
				t.Fatalf("must-not: %+v", i)
			}
			if i.Rule == "Same URL count" && i.Severity != Error {
				t.Fatalf("severity: %+v", i)
			}
		}
	}
	if strings.Join(names, ",") != "No Sie,Euro sign,Same URL count" {
		t.Fatalf("rules: %v", names)
	}
	for _, bad := range []UserRule{{Name: "x", Target: "("}, {Name: "x", Target: "a", Mode: "maybe"}, {Name: "x", Target: "a", Mode: "count"}, {Name: "x", Mode: "must"}} {
		if bad.Mode == "" {
			bad.Mode = "must"
		}
		if _, err := CheckSegment(seg("1", "a", "b"), Options{Rules: []UserRule{bad}}); err == nil {
			t.Fatalf("bad rule accepted: %+v", bad)
		}
	}
}

func TestReport(t *testing.T) {
	segs := []Segment{seg("a", "One.", "Eins."), seg("b", "Two 2.", ""), seg("c", "Three 3.", "Drei.")}
	is, err := Check(segs, enDe)
	if err != nil {
		t.Fatal(err)
	}
	r := NewReport("Mietvertrag.docx", segs, is, enDe, time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC))
	if r.Errors != 2 || r.Warnings != 0 || r.Checked != 3 || len(r.ByKind) != 2 {
		t.Fatalf("report: %+v", r)
	}
	if r.Issues[0].Number != 2 || r.Issues[1].Number != 3 || r.Issues[1].Source != "Three 3." {
		t.Fatalf("issues: %+v", r.Issues)
	}
	data, err := r.CSV()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		t.Fatal("no BOM")
	}
	rows, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil || len(rows) != 3 || rows[2][2] != "Missing numbers" || rows[2][7] != "Drei." {
		t.Fatalf("csv: %v %v", rows, err)
	}
	h := string(r.HTML())
	if !strings.Contains(h, "QA report: Mietvertrag.docx") || !strings.Contains(h, "2 errors, 0 warnings") || !strings.Contains(h, "Empty translation") {
		t.Fatalf("html: %s", h)
	}
	if strings.Contains(h, "—") {
		t.Fatal("em dash in report")
	}
	j, _ := r.JSON()
	var back Report
	if err := json.Unmarshal(j, &back); err != nil || len(back.Issues) != 2 {
		t.Fatalf("json: %v", err)
	}
	empty := NewReport("", nil, nil, enDe, time.Now())
	if !strings.Contains(string(empty.HTML()), "No issues found.") {
		t.Fatal("empty report")
	}
}

// The Hunspell dictionaries of package spell are what Options.Spell
// takes.
var _ Speller = (*spell.Dictionary)(nil)

func TestSpellingWithHunspell(t *testing.T) {
	d, err := spell.Load([]byte("SET UTF-8\nTRY ae\nSFX N Y 1\nSFX N 0 en .\n"), []byte("4\nDie/N\nKaution/N\nist\nfällig\n"))
	if err != nil {
		t.Fatal(err)
	}
	o := enDe
	o.Spell = d
	is := checkOne(t, seg("1", "The deposits are due.", "Die Kautionen ist fälig."), o)
	if len(is) != 1 || is[0].Kind != Spelling || is[0].Found != "fälig" || len(is[0].Suggestions) == 0 || is[0].Suggestions[0] != "fällig" {
		t.Fatalf("issues: %+v", is)
	}
}

func TestRuleCheck(t *testing.T) {
	ok := []UserRule{
		{Name: "no Sie", Target: `\bSie\b`, Mode: "must-not"},
		{Name: "euro", Source: `EUR`, Target: `€`, Mode: "count", Severity: Error},
		{Name: "date", Target: `\d{2}\.\d{2}`, Mode: "must", Severity: Warning},
	}
	for _, r := range ok {
		if err := r.Check(); err != nil {
			t.Errorf("%s: %v", r.Name, err)
		}
	}
	bad := []UserRule{
		{Name: "no target", Mode: "must"},
		{Name: "bad target", Target: `(`, Mode: "must"},
		{Name: "bad source", Source: `[`, Target: `x`, Mode: "must"},
		{Name: "bad mode", Target: `x`, Mode: "should"},
		{Name: "count without source", Target: `x`, Mode: "count"},
		{Name: "bad severity", Target: `x`, Mode: "must", Severity: "fatal"},
	}
	for _, r := range bad {
		if err := r.Check(); err == nil {
			t.Errorf("%s accepted", r.Name)
		}
		if _, err := Check(nil, Options{Rules: []UserRule{r}}); err == nil {
			t.Errorf("%s accepted by Check", r.Name)
		}
	}
}
