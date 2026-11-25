package project_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	. "github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/formats"
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
	_, err = project.Join(got, 1)
	refused(t, err, "its own parts first")
	_, err = project.Join(segs, 1)
	refused(t, err, "no segment after")
	other := append([]project.Segment{}, segs...)
	other[1].Block = "p2"
	_, err = project.Join(other, 0)
	refused(t, err, "different paragraphs")
	_, err = project.Separate(segs, 0)
	refused(t, err, "was not joined")
}

func TestJoinAndSeparate(t *testing.T) {
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
	if z.Status != project.StatusDraft || len(z.Joined) != 2 || z.Joined[1].ID != "p1/s2" || z.MaxLength != 0 {
		t.Errorf("joined: %+v", z)
	}
	// A third is joined on: the same as joining the last two first.
	all, err := project.Join(two, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Source.String() != "Erster {1}Satz{/1}. Zweiter {2}Satz{/2}{3/}. {4}Dritter{/4} Satz." || len(all[0].Joined) != 3 {
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
	// The file's units are still there.
	us := project.Units(all)
	if len(us) != 3 || us[1].ID != "p1/s2" || !us[1].Source.Equal(segs[1].Source) || us[2].Segments[0] != 0 {
		t.Errorf("units: %+v", us)
	}
	tr := project.Translations(other)
	if len(tr) != 3 || !tr["p1/s1"].Equal(other[0].Target) || !filter.IsJoined(tr["p1/s2"]) || !filter.IsJoined(tr["p1/s3"]) {
		t.Errorf("translations: %+v", tr)
	}
	// Without a translation the document keeps all of its source.
	other[0].Target = content.Content{}
	if tr := project.Translations(other); len(tr) != 0 {
		t.Errorf("an empty joined segment translates nothing: %+v", tr)
	}
	// A joined segment is not split, it is separated: the units come back, the translation stays with the first.
	_, err = project.Split(all, 0, c("Erster"))
	refused(t, err, "separate it instead")
	apart, err := project.Separate(all, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sources(apart) != sources(segs) || apart[0].Target.Plain() != all[0].Target.Plain() || apart[1].Status != project.StatusNew || apart[2].ID != "p1/s3" || apart[1].Block != "p1" || apart[1].Joined != nil {
		t.Errorf("apart: %+v", apart)
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
	segs, _ = project.Separate(segs, 0)
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
