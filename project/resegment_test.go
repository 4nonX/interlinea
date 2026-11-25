package project_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	. "github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/project"
)

// c reads content written as its String form: {1}bold{/1} and {2/}.
func c(s string) content.Content {
	out := content.Content{}
	re := regexp.MustCompile(`\{(/?)(\d+)(/?)\}`)
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		out = out.Append(s[last:m[0]])
		kind := content.TagOpen
		if s[m[2]:m[3]] == "/" {
			kind = content.TagClose
		} else if s[m[6]:m[7]] == "/" {
			kind = content.TagStandalone
		}
		out = out.AppendTag(content.Tag{Kind: kind, ID: s[m[4]:m[5]]})
		last = m[1]
	}
	return out.Append(s[last:])
}

func segmentsOf(ex *filter.Extraction) []project.Segment {
	out := make([]project.Segment, len(ex.Units))
	for i, u := range ex.Units {
		out[i] = project.Segment{ID: u.ID, Source: u.Source, Target: content.Content{}, Status: project.StatusNew, Block: u.Block}
	}
	return out
}

func sources(segs []project.Segment) string {
	var out []string
	for _, s := range segs {
		out = append(out, s.Source.String())
	}
	return strings.Join(out, "|")
}

func refused(t *testing.T, err error, want string) {
	t.Helper()
	var r *project.ErrResegment
	if !errors.As(err, &r) || !strings.Contains(r.Message, want) {
		t.Errorf("got %v, want a refusal with %q", err, want)
	}
}

func documentXML(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatal("no document.xml")
	return ""
}

var markup = regexp.MustCompile(`<[^>]+>`)

func TestSplitAndJoinBack(t *testing.T) {
	segs := []project.Segment{
		{ID: "p1/s1", Block: "p1", Source: c("Die Miete, {1}die Kaution und die Nebenkosten{/1} sind fällig."), Target: c("The rent is due."), Status: project.StatusConfirmed,
			Origin: &project.Origin{Kind: "tm"}, Comments: []project.Comment{{ID: "c1", Text: "Why?"}}},
		{ID: "p1/s2", Block: "p1", Source: c("Sie beträgt 900 Euro."), Target: content.Content{}, Status: project.StatusNew},
	}
	got, err := project.Split(segs, 0, c("Die Miete, {1}die Kaution "))
	if err != nil {
		t.Fatal(err)
	}
	// The pair open at the cut is closed and opened again; the space at the cut is kept aside.
	if s := sources(got); s != "Die Miete, {1}die Kaution{/1}|{1}und die Nebenkosten{/1} sind fällig.|Sie beträgt 900 Euro." {
		t.Fatalf("sources: %s", s)
	}
	a, b := got[0], got[1]
	if a.ID != "p1/s1#1" || b.ID != "p1/s1#2" || a.Split.Unit != "p1/s1" || b.Split.Space != " " || a.Split.Space != "" {
		t.Errorf("parts: %+v %+v / %+v %+v", a.ID, a.Split, b.ID, b.Split)
	}
	if a.Target.Plain() != "The rent is due." || a.Status != project.StatusDraft || a.Origin != nil || len(a.Comments) != 1 {
		t.Errorf("the first part keeps the translation as a draft: %+v", a)
	}
	if len(b.Target) != 0 || b.Status != project.StatusNew || len(b.Comments) != 0 || b.Block != "p1" {
		t.Errorf("the second part starts empty: %+v", b)
	}
	// The file's unit is still what its parts stand for.
	us := project.Units(got)
	if len(us) != 2 || us[0].ID != "p1/s1" || !us[0].Source.Equal(segs[0].Source) || len(us[0].Segments) != 2 || us[1].ID != "p1/s2" {
		t.Errorf("units: %+v", us)
	}
	// A part splits again, and its new part gets a name of its own.
	three, err := project.Split(got, 0, c("Die Miete,"))
	if err != nil {
		t.Fatal(err)
	}
	if three[0].ID != "p1/s1#1" || three[1].ID != "p1/s1#3" || three[2].ID != "p1/s1#2" || sources(three[:3]) != "Die Miete,|{1}die Kaution{/1}|{1}und die Nebenkosten{/1} sind fällig." {
		t.Errorf("three parts: %s %s %s: %s", three[0].ID, three[1].ID, three[2].ID, sources(three))
	}
	if u := project.Units(three); !u[0].Source.Equal(segs[0].Source) {
		t.Errorf("three parts make the unit: %s", u[0].Source)
	}
	// Parts are translated on their own; a part without a translation keeps its source.
	three[0].Target, three[2].Target = c("The rent,"), c("{1}and the charges{/1} are due.")
	tr := project.Translations(three)
	if got := tr["p1/s1"].String(); got != "The rent, {1}die Kaution{/1} {1}and the charges{/1} are due." || len(tr) != 1 {
		t.Errorf("translations: %q %d", got, len(tr))
	}
	// Joined in any order, the parts are the unit again, healed.
	back, err := project.Join(three, 1)
	if err != nil {
		t.Fatal(err)
	}
	if back[1].Source.String() != "{1}die Kaution und die Nebenkosten{/1} sind fällig." || back[1].ID != "p1/s1#3" || back[1].Split == nil {
		t.Errorf("two parts joined: %s %s", back[1].ID, back[1].Source)
	}
	if back[1].Target.String() != "{1}and the charges{/1} are due." {
		t.Errorf("an empty part adds nothing to the translation: %s", back[1].Target)
	}
	twice := append([]project.Segment{}, got...)
	twice[0].Target, twice[1].Target = c("The rent, {1}the deposit{/1}"), c("{1}and the charges{/1} are due.")
	if one, err := project.Join(twice, 0); err != nil || one[0].Target.String() != "The rent, {1}the deposit and the charges{/1} are due." {
		t.Errorf("a pair that stands in both translations is one pair again: %v %v", one[0].Target, err)
	} else if js, _ := one[0].Target.MarshalJSON(); new(content.Content).UnmarshalJSON(js) != nil {
		t.Error("the joined translation is not valid content")
	}
	whole, err := project.Join(back, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(whole) != 2 || whole[0].ID != "p1/s1" || whole[0].Split != nil || !whole[0].Source.Equal(segs[0].Source) || whole[0].Target.String() != "The rent, {1}and the charges{/1} are due." || whole[0].Status != project.StatusDraft {
		t.Errorf("whole again: %+v", whole[0])
	}

	// What is refused.
	_, err = project.Split(segs, 0, c("Die Miete, {1}die Kaution und die Nebenkosten{/1} sind fällig."))
	refused(t, err, "Both parts need text")
	_, err = project.Split(segs, 0, c("Etwas anderes"))
	refused(t, err, "not in the segment's source")
	_, err = project.Split(segs, 0, c("Die Miete, {2}die"))
	refused(t, err, "not in the segment's source")
	locked := append([]project.Segment{}, segs...)
	locked[0].Locked = true
	_, err = project.Split(locked, 0, c("Die Miete,"))
	refused(t, err, "locked")
	_, err = project.Join(locked, 0)
	refused(t, err, "locked")
	_, err = project.Join(segs, 1)
	refused(t, err, "no segment after")
	other := append([]project.Segment{}, segs...)
	other[1].Block = "p2"
	_, err = project.Join(other, 0)
	refused(t, err, "different paragraphs")
	held := append([]project.Segment{}, segs...)
	held[0].File = &project.FileState{State: "new", Fixed: true}
	_, err = project.Split(held, 0, c("Die Miete,"))
	refused(t, err, "keeps its own record")
	_, err = project.Join(held, 0)
	refused(t, err, "keeps its own record")
	// A file that keeps no such record is cut like any other.
	held[0].File.Fixed = false
	if cutUp, err := project.Split(held, 0, c("Die Miete,")); err != nil || cutUp[0].File != nil {
		t.Errorf("a plain bilingual file: %v", err)
	}
}

func ids(segs []project.Segment) string {
	var out []string
	for _, s := range segs {
		out = append(out, s.ID)
	}
	return strings.Join(out, " ")
}

func TestJoinAndSplitFreely(t *testing.T) {
	segs := []project.Segment{
		{ID: "p1/s1", Block: "p1", Source: c("Erster {1}Satz{/1}."), Target: c("First {1}sentence{/1}."), Status: project.StatusConfirmed},
		{ID: "p1/s2", Block: "p1", Source: c("Zweiter {1}Satz{/1}{2/}."), Target: c("Second {1}sentence{/1}{2/}."), Status: project.StatusDraft, MaxLength: 40},
		{ID: "p1/s3", Block: "p1", Source: c("{1}Dritter{/1} Satz."), Target: content.Content{}, Status: project.StatusNew},
	}
	two, err := project.Join(segs, 0)
	if err != nil {
		t.Fatal(err)
	}
	z := two[0]
	if len(two) != 2 || z.ID != "p1/s1" || z.Source.String() != "Erster {1}Satz{/1}. Zweiter {2}Satz{/2}{3/}." || z.Target.String() != "First {1}sentence{/1}. Second {2}sentence{/2}{3/}." {
		t.Fatalf("joined: %s / %s", z.Source, z.Target)
	}
	if z.Status != project.StatusDraft || len(z.Joined) != 2 || z.Joined[1].ID != "p1/s2" || z.MaxLength != 0 || z.Split != nil {
		t.Errorf("joined: %+v", z)
	}
	// A third is joined on: the same as joining the last two first.
	all, err := project.Join(two, 0)
	if err != nil {
		t.Fatal(err)
	}
	const whole = "Erster {1}Satz{/1}. Zweiter {2}Satz{/2}{3/}. {4}Dritter{/4} Satz."
	if len(all) != 1 || all[0].Source.String() != whole || len(all[0].Joined) != 3 {
		t.Fatalf("three joined: %s", all[0].Source)
	}
	last, err := project.Join(segs, 1)
	if err != nil {
		t.Fatal(err)
	}
	last[1].Target = c("Second {1}sentence{/1}{2/} and {3}third{/3}.")
	other, err := project.Join(last, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !other[0].Source.Equal(all[0].Source) || other[0].Target.String() != "First {1}sentence{/1}. Second {2}sentence{/2}{3/} and {4}third{/4}." {
		t.Errorf("joined the other way round: %s / %s", other[0].Source, other[0].Target)
	}
	us := project.Units(all)
	if len(us) != 3 || us[1].ID != "p1/s2" || !us[1].Source.Equal(segs[1].Source) || us[2].Segments[0] != 0 {
		t.Errorf("units: %+v", us)
	}
	tr := project.Translations(other)
	if len(tr) != 3 || !tr["p1/s1"].Equal(other[0].Target) || !filter.IsJoined(tr["p1/s2"]) || !filter.IsJoined(tr["p1/s3"]) {
		t.Errorf("translations: %+v", tr)
	}
	blankOne := append([]project.Segment{}, other...)
	blankOne[0].Target = content.Content{}
	if tr := project.Translations(blankOne); len(tr) != 0 {
		t.Errorf("an empty joined segment translates nothing: %+v", tr)
	}

	// What was joined is split anywhere: here inside the second sentence.
	cutUp, err := project.Split(other, 0, c("Erster {1}Satz{/1}. Zweiter {2}Satz{/2}"))
	if err != nil {
		t.Fatal(err)
	}
	if ids(cutUp) != "p1/s1#1 p1/s1#2" || sources(cutUp) != "Erster {1}Satz{/1}. Zweiter {2}Satz{/2}|{3/}. {4}Dritter{/4} Satz." || len(cutUp[1].Joined) != 3 || cutUp[1].Split.Unit != "p1/s1" {
		t.Fatalf("a joined segment split: %s: %s", ids(cutUp), sources(cutUp))
	}
	if project.CheckRuns(cutUp) != -1 || len(project.Units(cutUp)) != 3 {
		t.Errorf("the parts stand for the three units: %d", project.CheckRuns(cutUp))
	}
	cutUp[1].Target = c("{3/}, and a {4}third{/4}.")
	tr = project.Translations(cutUp)
	if got := tr["p1/s1"].String(); got != "First {1}sentence{/1}. Second {2}sentence{/2}{3/} and {4}third{/4}.{3/}, and a {4}third{/4}." || !filter.IsJoined(tr["p1/s3"]) {
		t.Errorf("the parts of a joined stretch are written as one: %q", got)
	}
	// Split where it was joined, the sides stand apart again, with the tags they had.
	apart, err := project.Split(all, 0, c("Erster {1}Satz{/1}. Zweiter {2}Satz{/2}{3/}."))
	if err != nil {
		t.Fatal(err)
	}
	if ids(apart) != "p1/s1 p1/s3" || sources(apart) != "Erster {1}Satz{/1}. Zweiter {2}Satz{/2}{3/}.|{1}Dritter{/1} Satz." || len(apart[0].Joined) != 2 || apart[1].Joined != nil || apart[1].Split != nil {
		t.Fatalf("split at the join: %s: %s %+v", ids(apart), sources(apart), apart[1])
	}
	back, err := project.Split(apart, 0, c("Erster {1}Satz{/1}."))
	if err != nil {
		t.Fatal(err)
	}
	if ids(back) != "p1/s1 p1/s2 p1/s3" || sources(back) != sources(segs) || back[0].Joined != nil || back[1].Status != project.StatusNew || back[0].Target.Plain() != all[0].Target.Plain() {
		t.Fatalf("back to the file's segments: %s: %s", ids(back), sources(back))
	}

	// A part is joined with the sentence after it, and the stretch grows.
	parts, err := project.Split(segs, 0, c("Erster"))
	if err != nil {
		t.Fatal(err)
	}
	parts[1].Target = c("{1}sentence{/1}.")
	grown, err := project.Join(parts, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ids(grown) != "p1/s1#1 p1/s1#2 p1/s3" || sources(grown) != "Erster|{1}Satz{/1}. Zweiter {2}Satz{/2}{3/}.|{1}Dritter{/1} Satz." || len(grown[0].Joined) != 2 || len(grown[1].Joined) != 2 {
		t.Fatalf("a part joined with the next sentence: %s: %s", ids(grown), sources(grown))
	}
	if grown[1].Target.String() != "{1}sentence{/1}. Second {2}sentence{/2}{3/}." || project.CheckRuns(grown) != -1 {
		t.Errorf("its translation: %s", grown[1].Target)
	}
	// And a sentence with the part after it: both stretches' parts are one stretch's.
	tail, err := project.Split(grown, 2, c("{1}Dritter{/1}"))
	if err != nil {
		t.Fatal(err)
	}
	if ids(tail) != "p1/s1#1 p1/s1#2 p1/s3#1 p1/s3#2" {
		t.Fatalf("two stretches in parts: %s", ids(tail))
	}
	one, err := project.Join(tail, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ids(one) != "p1/s1#1 p1/s1#2 p1/s1#3" || sources(one) != "Erster|{1}Satz{/1}. Zweiter {2}Satz{/2}{3/}. {4}Dritter{/4}|Satz." || len(one[2].Joined) != 3 || one[2].Split.Unit != "p1/s1" {
		t.Fatalf("joined across two stretches: %s: %s", ids(one), sources(one))
	}
	if project.CheckRuns(one) != -1 {
		t.Error("the parts no longer stand for the units")
	}
	us = project.Units(one)
	if len(us) != 3 || !us[2].Source.Equal(segs[2].Source) || len(us[2].Segments) != 3 {
		t.Errorf("units: %+v", us)
	}
	// Joined all the way, it is the three sentences as one segment.
	for len(one) > 1 {
		if one, err = project.Join(one, 0); err != nil {
			t.Fatal(err)
		}
	}
	if one[0].ID != "p1/s1" || one[0].Source.String() != whole || one[0].Split != nil || len(one[0].Joined) != 3 {
		t.Errorf("all joined: %+v", one[0])
	}
	// Parts that do not read as their units are found.
	bad := append([]project.Segment{}, tail...)
	bad[1].Source = c("etwas anderes")
	if project.CheckRuns(bad) != 0 {
		t.Errorf("wrong parts pass: %d", project.CheckRuns(bad))
	}
	// Chinese sentences are joined without a space.
	zh := []project.Segment{{ID: "a", Block: "p", Source: c("第一句。"), Target: content.Content{}}, {ID: "b", Block: "p", Source: c("第二句。"), Target: content.Content{}}}
	j, err := project.Join(zh, 0)
	if err != nil || j[0].Source.Plain() != "第一句。第二句。" {
		t.Errorf("zh: %v %v", j, err)
	}
}

// A document takes joined and split segments as it takes its own: in
// Word, with the formatting of both joined sentences.
func TestResegmentedWordDocument(t *testing.T) {
	data := Document(Parts{Body: P("", R("Erster "), B("Satz"), R(". Zweiter "), I("Satz"), R(" hier. Dritter Satz.")) + P("", R("Anderer Absatz.")), Lang: "de-DE"})
	ex, err := docx.Extract(data, "de-DE", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	segs := segmentsOf(ex)
	if sources(segs) != "Erster {1}Satz{/1}.|Zweiter {1}Satz{/1} hier.|Dritter Satz.|Anderer Absatz." {
		t.Fatalf("units: %s", sources(segs))
	}
	if _, err := project.Join(segs, 2); err == nil {
		t.Error("segments of two paragraphs were joined")
	}
	segs, err = project.Join(segs, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The translation turns the two sentences round.
	segs[0].Target = c("A second {2}sentence{/2} here, after a first {1}one{/1}.")
	segs, err = project.Split(segs, 1, c("Dritter"))
	if err != nil {
		t.Fatal(err)
	}
	segs[1].Target, segs[2].Target = c("A third"), c("sentence.")
	out, err := docx.Merge(data, "de-DE", "en-GB", project.Translations(segs), docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	doc := documentXML(t, out)
	text := markup.ReplaceAllString(doc, "")
	if !strings.Contains(text, "A second sentence here, after a first one. A third sentence.Anderer Absatz.") {
		t.Errorf("text: %s", text)
	}
	italic := regexp.MustCompile(`<w:r><w:rPr>[^/]*<w:i/>.*?</w:rPr><w:t[^>]*>sentence</w:t>`)
	bold := regexp.MustCompile(`<w:r><w:rPr>[^/]*<w:b/>.*?</w:rPr><w:t[^>]*>one</w:t>`)
	if !italic.MatchString(doc) || !bold.MatchString(doc) {
		t.Errorf("the second sentence's italics and the first one's bold are kept: %s", doc)
	}
	// Separated again and without translations, the document is as it was.
	segs, err = project.Split(segs, 0, c("Erster {1}Satz{/1}."))
	if err != nil || len(segs) != 5 {
		t.Fatalf("split at the join: %v %s", err, sources(segs))
	}
	for i := range segs {
		segs[i].Target = content.Content{}
	}
	plain, err := docx.Merge(data, "de-DE", "en-GB", project.Translations(segs), docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := markup.ReplaceAllString(documentXML(t, plain), ""); !strings.Contains(got, "Erster Satz. Zweiter Satz hier. Dritter Satz.Anderer Absatz.") {
		t.Errorf("untranslated: %s", got)
	}
}

// The same in a web page, whose filter builds paragraphs another way.
func TestResegmentedWebPage(t *testing.T) {
	page := []byte("<html><body><p>Erster <b>Satz</b>. Zweiter <i>Satz</i> hier. Dritter Satz.</p><p>Anderer Absatz.</p></body></html>")
	ex, err := formats.Extract("html", page, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	segs := segmentsOf(ex)
	if len(segs) != 4 {
		t.Fatalf("units: %s", sources(segs))
	}
	segs, err = project.Join(segs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if segs[0].Source.String() != "Erster {1}Satz{/1}. Zweiter {2}Satz{/2} hier." {
		t.Fatalf("joined: %s", segs[0].Source)
	}
	segs[0].Target = c("A second {2}sentence{/2} here, after a first {1}one{/1}.")
	segs, err = project.Split(segs, 1, c("Dritter"))
	if err != nil {
		t.Fatal(err)
	}
	segs[2].Target = c("sentence.")
	out, err := formats.Merge("html", page, "de-DE", "en-GB", project.Translations(segs))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<p>A second <i>sentence</i> here, after a first <b>one</b>. Dritter sentence.</p><p>Anderer Absatz.</p>") {
		t.Errorf("page: %s", out)
	}
}

func TestProjectSegmentationRules(t *testing.T) {
	page := []byte("<html><body><p>Die Miete nach Abs. 2 ist fällig; die Kaution auch. Der <b>Mieter zahlt. Er</b> zahlt gern.</p><p>z. B. so.</p></body></html>")
	ex, err := formats.Extract("html", page, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	segs := segmentsOf(ex)
	usual := sources(segs)
	rules := []project.SegmentationRule{
		{Break: false, Before: `\bz\.`, After: `\s+B\.`},
		{Break: true, Before: `;`, After: `\s`},
	}
	for i, r := range rules {
		if err := r.Check(); err != nil {
			t.Fatalf("rule %d: %v", i, err)
		}
	}
	got := project.ApplySegmentation(segs, rules, "de-DE")
	var parts []string
	for _, s := range got {
		parts = append(parts, s.Source.Plain())
	}
	t.Logf("usual: %s", usual)
	want := "Die Miete nach Abs. 2 ist fällig;|die Kaution auch.|Der Mieter zahlt. Er zahlt gern.|z. B. so."
	if strings.Join(parts, "|") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(parts, "|"), want)
	}
	// The file takes them as its own, and is written with their translations.
	us := project.Units(got)
	if len(us) != len(ex.Units) {
		t.Fatalf("%d units for %d of the file", len(us), len(ex.Units))
	}
	for i := range got {
		got[i].Target = content.Text("T" + string(rune('1'+i)) + ".")
	}
	out, err := formats.Merge("html", page, "de-DE", "en-GB", project.Translations(got))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<p>T1. T2. T3.</p><p>T4.</p>") || len(got) != 4 || len(got[3].Joined) != 2 || got[0].Split == nil {
		t.Errorf("page: %s", out)
	}
	// Without rules, and with rules that match nothing, nothing changes.
	if same := project.ApplySegmentation(segs, nil, "de-DE"); sources(same) != usual {
		t.Errorf("no rules: %s", sources(same))
	}
	if same := project.ApplySegmentation(segs, []project.SegmentationRule{{Break: true, Before: `§§§`}}, "de-DE"); sources(same) != usual {
		t.Errorf("a rule that matches nothing: %s", sources(same))
	}
	// A rule that is no regular expression, or says nothing, is refused.
	if (project.SegmentationRule{Break: true, Before: `(`}).Check() == nil || (project.SegmentationRule{Break: true}).Check() == nil {
		t.Error("a bad rule passes")
	}
}

// A plain XLIFF file takes segments cut differently too: joined within
// one of its units, and split anywhere.
func TestResegmentedXLIFF(t *testing.T) {
	data, err := os.ReadFile("../filter/xliff/testdata/xliff2.xlf")
	if err != nil {
		t.Fatal(err)
	}
	ex, err := xliff.Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	var segs []project.Segment
	for _, u := range ex.Units {
		s := project.Segment{ID: u.ID, Source: u.Source, Target: u.Target, Block: u.Block, Locked: u.Locked, Status: project.StatusNew}
		if s.Target == nil {
			s.Target = content.Content{}
		}
		segs = append(segs, s)
	}
	if len(segs) != 3 || segs[0].Block != segs[1].Block || segs[1].Block == segs[2].Block {
		t.Fatalf("units: %s", sources(segs))
	}
	if _, err := project.Join(segs, 1); err == nil {
		t.Error("segments of two units of the file were joined")
	}
	segs, err = project.Join(segs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if segs[0].Source.String() != "Hallo {1}Welt{/1} {2/}. Zweiter {3}Satz{/3}." {
		t.Fatalf("joined: %s", segs[0].Source)
	}
	segs[0].Target = c("A second {3}sentence{/3}, hello {1}world{/1} {2/}.")
	segs[0].Status = project.StatusConfirmed
	last := len(segs) - 1
	segs, err = project.Split(segs, last, segs[last].Source.Slice(0, len("Entwurf")))
	if err != nil {
		t.Fatal(err)
	}
	segs[last].Target, segs[last].Status = c("Draft"), project.StatusConfirmed
	segs[last+1].Target, segs[last+1].Status = segs[last+1].Source.Clone(), project.StatusDraft
	out, err := xliff.Merge(data, project.XLIFFUpdates(segs))
	if err != nil {
		t.Fatal(err)
	}
	back, err := xliff.Extract(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := back.Units[0].Target.String(); got != "A second {3}sentence{/3}, hello {1}world{/1} {2/}." || back.Units[0].State != filter.StateTranslated {
		t.Errorf("the joined translation stands in the first segment: %q %s\n%s", got, back.Units[0].State, out)
	}
	if len(back.Units[1].Target) != 0 || back.Units[1].State != filter.StateTranslated {
		t.Errorf("the segment joined to it is empty, in the same state: %q %s", back.Units[1].Target, back.Units[1].State)
	}
	if got := back.Units[2].Target.Plain(); got != "Draft Kaution" || back.Units[2].State != filter.StateDraft {
		t.Errorf("the parts stand one after the other, in the least advanced state: %q %s", got, back.Units[3].State)
	}
	if !strings.Contains(string(out), `<sc id="3"/>sentence<ec startRef="3"/>`) {
		t.Errorf("the second segment's tags are written as the file has them: %s", out)
	}
}

// XLIFF 1.2 with segments marked inside a unit: joined, the translation
// stands in the first marker and the second is empty.
func TestResegmentedXLIFF12(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2"><file original="a.txt" source-language="de-DE" target-language="en-GB" datatype="plaintext"><body>
<trans-unit id="1"><source>Erster Satz. Zweiter <g id="7">Satz</g>.</source><seg-source><mrk mtype="seg" mid="1">Erster Satz.</mrk> <mrk mtype="seg" mid="2">Zweiter <g id="7">Satz</g>.</mrk></seg-source></trans-unit>
<trans-unit id="2"><source>Dritter Satz.</source></trans-unit>
</body></file></xliff>`)
	ex, err := xliff.Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	var segs []project.Segment
	for _, u := range ex.Units {
		segs = append(segs, project.Segment{ID: u.ID, Source: u.Source, Target: content.Content{}, Block: u.Block, Status: project.StatusNew})
	}
	if len(segs) != 3 {
		t.Fatalf("units: %s", sources(segs))
	}
	segs, err = project.Join(segs, 0)
	if err != nil {
		t.Fatal(err)
	}
	tagged := segs[0].Source.Tags()
	if len(tagged) != 2 {
		t.Fatalf("joined: %s", segs[0].Source)
	}
	segs[0].Target = content.Text("A second ").AppendTag(content.Tag{Kind: content.TagOpen, ID: tagged[0].ID}).Append("sentence").AppendTag(content.Tag{Kind: content.TagClose, ID: tagged[0].ID}).Append(" after the first.")
	segs[0].Status = project.StatusConfirmed
	segs, err = project.Split(segs, 1, c("Dritter"))
	if err != nil {
		t.Fatal(err)
	}
	segs[1].Target, segs[1].Status = c("A third"), project.StatusConfirmed
	segs[2].Target, segs[2].Status = c("sentence."), project.StatusConfirmed
	out, err := xliff.Merge(data, project.XLIFFUpdates(segs))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<mrk mtype="seg" mid="1">A second <g id="7">sentence</g> after the first.</mrk> <mrk mtype="seg" mid="2"></mrk>`,
		`<target state="translated">A third sentence.</target>`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	if _, err := xliff.Extract(out); err != nil {
		t.Errorf("the file is no longer read: %v", err)
	}
}

// A rule that joins a sentence and one that cuts the same sentence both apply.
func TestSegmentationRulesOnOneSentence(t *testing.T) {
	text := []byte("Siehe z. B. Abs. 2; dort steht mehr. Ende.\n")
	ex, err := formats.Extract("txt", text, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	segs := segmentsOf(ex)
	if sources(segs) != "Siehe z.|B. Abs. 2; dort steht mehr.|Ende." {
		t.Fatalf("usual: %s", sources(segs))
	}
	got := project.ApplySegmentation(segs, []project.SegmentationRule{
		{Break: false, Before: `\bz\.`, After: `\s+B\.`},
		{Break: true, Before: `;`, After: `\s`},
	}, "de-DE")
	if sources(got) != "Siehe z. B. Abs. 2;|dort steht mehr.|Ende." || len(got[0].Joined) != 2 || got[1].Split == nil || project.CheckRuns(got) != -1 {
		t.Fatalf("ruled: %s", sources(got))
	}
	got[0].Target, got[1].Target = c("See e.g. para. 2;"), c("there is more.")
	out, err := formats.Merge("txt", text, "de-DE", "en-GB", project.Translations(got))
	if err != nil || string(out) != "See e.g. para. 2; there is more. Ende.\n" {
		t.Errorf("text: %q %v", out, err)
	}
}
