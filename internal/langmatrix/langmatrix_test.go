// Package langmatrix runs every language the apps list through every part
// of the core that depends on the language: sentence splitting, word
// counts, memory lookup and search, TMX, term recognition, the quality
// checks, language detection, and Word and XLIFF round trips. A language
// added to the list without a sample here fails the test.
package langmatrix

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/langdetect"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/qa"
	"github.com/4nonX/interlinea/segment"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmdb"
)

const english = "The contract begins today. The tenant pays the rent to the landlord every month, and the deposit is not due this month."

var ctx = context.Background()

// sampleFor finds the sample of a listed language: its own, or the one of
// its language in the same script.
func sampleFor(tag string) (sample, bool) {
	switch tag {
	case "es-419":
		tag = "es-MX"
	case "zh-HK":
		tag = "zh-TW"
	}
	for _, s := range samples {
		if s.lang == tag {
			return s, true
		}
	}
	for _, s := range samples {
		if !strings.Contains(s.lang, "-") && s.lang == langtag.Primary(tag) {
			return s, true
		}
	}
	return sample{}, false
}

// otherLang is the language the samples' translations are kept under: the
// English text, labelled German where the sample itself is English.
func otherLang(tag string) string {
	if langtag.Primary(tag) == "en" {
		return "de-DE"
	}
	return "en-GB"
}

// numberIn is 1250.5 as the tag's region writes it.
func numberIn(tag string, s sample) string {
	if strings.HasSuffix(tag, "-CH") {
		return "1’250.5"
	}
	return s.number
}

// perCharacter says the script is written without spaces between words.
func perCharacter(tag string) bool {
	switch langtag.Primary(tag) {
	case "zh", "ja", "th", "lo", "km", "my":
		return true
	}
	return false
}

func TestList(t *testing.T) {
	seen := map[string]bool{}
	prev := ""
	for _, l := range langtag.Languages() {
		if n, err := langtag.Normalize(l.Tag); err != nil || n != l.Tag {
			t.Errorf("%s: not a canonical tag (%q, %v)", l.Tag, n, err)
		}
		if !langtag.WellFormed(l.Tag) {
			t.Errorf("%s: not well formed", l.Tag)
		}
		if seen[l.Tag] {
			t.Errorf("%s: listed twice", l.Tag)
		}
		seen[l.Tag] = true
		if l.Name == "" || l.Native == "" || !strings.Contains(l.Name, "(") {
			t.Errorf("%s: name %q, native %q", l.Tag, l.Name, l.Native)
		}
		if l.Name <= prev {
			t.Errorf("%s: %q is out of order after %q", l.Tag, l.Name, prev)
		}
		prev = l.Name
		if l.RTL != langtag.RTL(l.Tag) {
			t.Errorf("%s: listed right-to-left %v, RTL says %v", l.Tag, l.RTL, langtag.RTL(l.Tag))
		}
		if _, ok := sampleFor(l.Tag); !ok {
			t.Errorf("%s: no sample text, so nothing checks it", l.Tag)
		}
		// Its names are read back as the language, in a spreadsheet heading.
		if got, ok := term.LangFromName(l.Name); !ok || got != l.Tag {
			t.Errorf("%s: LangFromName(%q) = %q, %v", l.Tag, l.Name, got, ok)
		}
		if got, ok := term.LangFromName(l.Tag); !ok || got != l.Tag {
			t.Errorf("%s: LangFromName of the tag = %q, %v", l.Tag, got, ok)
		}
		if term.LangName(l.Tag) != l.Name {
			t.Errorf("%s: LangName = %q", l.Tag, term.LangName(l.Tag))
		}
	}
	// Scripts that are not interchangeable are different languages to a memory.
	for _, pair := range [][2]string{{"zh-CN", "zh-TW"}, {"zh-CN", "zh-HK"}, {"sr-Latn-RS", "sr-Cyrl-RS"}} {
		if langtag.Match(pair[0], pair[1]) != langtag.NoMatch {
			t.Errorf("%s and %s match", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{{"zh-TW", "zh-HK"}, {"de-DE", "de-AT"}, {"pt-BR", "pt-PT"}, {"sr-Cyrl-RS", "sr-RS"}} {
		if langtag.Match(pair[0], pair[1]) != langtag.PrimaryMatch {
			t.Errorf("%s and %s do not match as variants", pair[0], pair[1])
		}
	}
}

func TestEveryLanguage(t *testing.T) {
	db, err := tmdb.Open(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rules := segment.Default()
	for _, l := range langtag.Languages() {
		s, ok := sampleFor(l.Tag)
		if !ok {
			continue
		}
		tag := l.Tag
		t.Run(tag, func(t *testing.T) {
			t.Run("sentences", func(t *testing.T) { sentences(t, rules, tag, s) })
			t.Run("words", func(t *testing.T) { words(t, tag, s) })
			t.Run("memory", func(t *testing.T) {
				m, err := tm.NewInMemory(tm.Info{Name: tag, SourceLang: tag, TargetLang: otherLang(tag)})
				if err != nil {
					t.Fatal(err)
				}
				memory(t, m, tag, s)
				tmxRoundTrip(t, m, tag, s)
			})
			t.Run("stored memory", func(t *testing.T) {
				m, err := db.Create(ctx, tm.Info{Name: tag, SourceLang: tag, TargetLang: otherLang(tag)})
				if err != nil {
					t.Fatal(err)
				}
				memory(t, m, tag, s)
			})
			t.Run("terms", func(t *testing.T) { terms(t, tag, s) })
			t.Run("checks", func(t *testing.T) { checks(t, tag, s) })
			t.Run("detection", func(t *testing.T) { detection(t, tag, s) })
			t.Run("word file", func(t *testing.T) { wordFile(t, tag, s) })
			t.Run("xliff file", func(t *testing.T) { xliffFile(t, tag, s) })
		})
	}
}

func sentences(t *testing.T, rules *segment.Rules, tag string, s sample) {
	want := s.sentences
	if want == 0 {
		want = 2
	}
	got := segment.New(rules, tag).Strings(s.text)
	if len(got) != want {
		t.Fatalf("%d sentences, want %d: %q", len(got), want, got)
	}
	if strings.Join(got, " ") != s.text && strings.Join(got, "") != s.text {
		t.Errorf("the sentences do not add up to the text: %q", got)
	}
}

func words(t *testing.T, tag string, s sample) {
	n := analysis.Count(content.Text(s.text))
	want := len(strings.Fields(s.text))
	if perCharacter(tag) {
		// A word for each character; the signs written on one are part of it.
		want = 0
		for _, r := range s.text {
			if (unicode.IsLetter(r) || unicode.IsDigit(r)) && !unicode.IsMark(r) {
				want++
			}
		}
	}
	if n.Words != want {
		t.Errorf("%d words, want %d: %q", n.Words, want, analysis.Words(s.text))
	}
	if n.Segments != 1 || n.Characters == 0 {
		t.Errorf("counts %+v", n)
	}
}

func memory(t *testing.T, m tm.Memory, tag string, s sample) {
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text(s.text), Target: content.Text(english)}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	best := func(text string) int {
		ms, err := m.Lookup(ctx, content.Text(text), tm.LookupOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) == 0 {
			return 0
		}
		if ms[0].Unit.Target.Plain() != english {
			t.Errorf("match gives %q", ms[0].Unit.Target.Plain())
		}
		return ms[0].Score
	}
	if got := best(s.text); got != 100 {
		t.Errorf("the same text scores %d", got)
	}
	// The same text as a macOS tool would store it, accents decomposed.
	if got := best(norm.NFD.String(s.text)); got != 100 {
		t.Errorf("the text with decomposed accents scores %d", got)
	}
	// A little more text is a close match, not an exact one and not none.
	if got := best(s.text + " " + s.term); got < 60 || got > 99 {
		t.Errorf("the text with a word added scores %d", got)
	}
	// Search finds its first word in either case, and its term's letters.
	first := strings.TrimFunc(strings.Fields(s.text)[0], func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsMark(r) })
	if perCharacter(tag) {
		first = s.term
	}
	for _, q := range []string{first, strings.ToUpper(first), strings.ToLower(first), norm.NFD.String(first)} {
		us, err := m.Concordance(ctx, q, tm.ConcordanceOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(us) != 1 {
			t.Errorf("search for %q finds %d units", q, len(us))
		}
	}
	us, err := m.Concordance(ctx, "tenant", tm.ConcordanceOptions{InTarget: true})
	if err != nil || len(us) != 1 {
		t.Errorf("search in the translations finds %d units (%v)", len(us), err)
	}
}

func tmxRoundTrip(t *testing.T, m tm.Memory, tag string, s sample) {
	var buf bytes.Buffer
	if err := tm.ExportTMX(ctx, m, &buf, "test"); err != nil {
		t.Fatal(err)
	}
	back, err := tm.NewInMemory(tm.Info{Name: "back", SourceLang: tag, TargetLang: otherLang(tag)})
	if err != nil {
		t.Fatal(err)
	}
	st, err := tm.ImportTMX(ctx, back, bytes.NewReader(buf.Bytes()), tm.ImportOptions{})
	if err != nil || st.Stored != 1 {
		t.Fatalf("TMX read back: %+v, %v\n%s", st, err, buf.String())
	}
	ms, err := back.Lookup(ctx, content.Text(s.text), tm.LookupOptions{})
	if err != nil || len(ms) != 1 || ms[0].Score != 100 || ms[0].Unit.Source.Plain() != s.text {
		t.Errorf("after TMX: %+v, %v", ms, err)
	}
}

func terms(t *testing.T, tag string, s sample) {
	entry := term.Entry{ID: "c1", Languages: []term.Language{
		{Lang: tag, Terms: []term.Term{{Text: s.term, Status: term.StatusApproved}}},
		{Lang: "en-GB", Terms: []term.Term{{Text: "contract", Status: term.StatusApproved}}},
	}}
	x := term.NewIndex()
	x.Add("test", []term.Entry{entry})

	// The term is found in running text, in the form it takes there.
	hits := x.Recognize(s.form, tag, "en-GB")
	if len(hits) != 1 {
		t.Fatalf("%q in %q: %d hits", s.term, s.form, len(hits))
	}
	if found := s.form[hits[0].Start:hits[0].End]; !strings.Contains(s.form, found) || found == "" {
		t.Errorf("hit %q", found)
	}
	if issues := x.Check(hits, "The agreement begins today.", "en-GB"); len(issues) != 1 || issues[0].Kind != "missing" {
		t.Errorf("a translation without the term: %+v", issues)
	}
	if issues := x.Check(hits, "The contract begins today.", "en-GB"); len(issues) != 0 {
		t.Errorf("a translation with the term: %+v", issues)
	}
	// And the other way: translating into the language, the form counts as
	// the term being used.
	back := x.Recognize("The contract begins today.", "en-GB", tag)
	if len(back) != 1 {
		t.Fatalf("English source: %d hits", len(back))
	}
	if issues := x.Check(back, s.form, tag); len(issues) != 0 {
		t.Errorf("%q does not count as using %q: %+v", s.form, s.term, issues)
	}
	if issues := x.Check(back, "0000", tag); len(issues) != 1 {
		t.Errorf("a translation without the term: %+v", issues)
	}
	// It is not found inside an unrelated run of letters.
	if hits := x.Recognize("xqzv "+strings.Repeat("q", 12)+" 12345", tag, "en-GB"); len(hits) != 0 {
		t.Errorf("found in nonsense: %+v", hits)
	}
}

// The checks whose result depends on the language's conventions.
var conventionChecks = map[qa.Kind]bool{
	qa.NumbersMissing: true, qa.NumbersExtra: true, qa.EndPunctuation: true, qa.SpaceBeforePunct: true, qa.DoubleSpace: true,
	qa.EdgeSpace: true, qa.Brackets: true, qa.RepeatedWord: true, qa.Capitalization: true,
}

func checks(t *testing.T, tag string, s sample) {
	run := func(src, tgt string) []qa.Issue {
		issues, err := qa.Check([]qa.Segment{{ID: "1", Source: content.Text(src), Target: content.Text(tgt)}}, qa.Options{SourceLang: "en-GB", TargetLang: tag})
		if err != nil {
			t.Fatal(err)
		}
		var out []qa.Issue
		for _, i := range issues {
			if conventionChecks[i.Kind] {
				out = append(out, i)
			}
		}
		return out
	}
	// A correct translation raises nothing.
	if issues := run(english, s.text); len(issues) != 0 {
		t.Errorf("a correct translation: %+v", issues)
	}
	// A number written the language's way is the same number.
	number := numberIn(tag, s)
	if issues := run("1,250.5 (see § 3)", number+" (§ 3)"); len(issues) != 0 {
		t.Errorf("%s for 1,250.5: %+v", number, issues)
	}
	// A different number is noticed, in whatever digits it is written.
	wrong := strings.NewReplacer("5", "7", "٥", "٧", "۵", "۷", "५", "७", "๕", "๗").Replace(number)
	found := false
	for _, i := range run("1,250.5 (see § 3)", wrong+" (§ 3)") {
		found = found || i.Kind == qa.NumbersMissing
	}
	if !found {
		t.Errorf("%s for 1,250.5 passes as the same number", wrong)
	}
	// Translating out of the language works the same.
	issues, err := qa.Check([]qa.Segment{{ID: "1", Source: content.Text(s.text), Target: content.Text(english)}}, qa.Options{SourceLang: tag, TargetLang: "en-GB"})
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range issues {
		if conventionChecks[i.Kind] {
			t.Errorf("into English: %+v", i)
		}
	}
}

func detection(t *testing.T, tag string, s sample) {
	d := langdetect.Detect(s.text)
	ok := langtag.Primary(d.Lang) == langtag.Primary(tag) && (langtag.Script(d.Lang) == "" || langtag.Script(tag) == "" || langtag.Script(d.Lang) == langtag.Script(tag))
	for _, a := range s.detect {
		ok = ok || d.Lang == a
	}
	if !ok {
		t.Errorf("detected %q (%.2f, %s), alternatives %+v", d.Lang, d.Confidence, d.Reason, d.Alternatives)
	}
}

func documentXML(t *testing.T, data []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			return string(b)
		}
	}
	t.Fatal("no word/document.xml")
	return ""
}

func wordFile(t *testing.T, tag string, s sample) {
	doc := docxtest.Document(docxtest.Parts{Body: docxtest.P("", docxtest.R("The contract begins today.")) + docxtest.P("", docxtest.R("Untranslated."))})
	ext, err := docx.Extract(doc, "en-GB", docx.Options{})
	if err != nil || len(ext.Units) != 2 {
		t.Fatalf("extract: %v, %d units", err, len(ext.Units))
	}
	out, err := docx.Merge(doc, "en-GB", tag, filter.Translations{ext.Units[0].ID: content.Text(s.text)}, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	back, err := docx.Extract(out, tag, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range back.Units {
		got = append(got, u.Source.Plain())
	}
	// The translation may come back as several sentences; their text is the same.
	squeeze := func(x string) string { return strings.Join(strings.Fields(x), "") }
	if squeeze(strings.Join(got, "")) != squeeze(s.text+"Untranslated.") {
		t.Errorf("the translated document reads %q", got)
	}
	x := documentXML(t, out)
	if !strings.Contains(x, `="`+tag+`"`) {
		t.Errorf("the translated text is not marked as %s", tag)
	}
	// A right-to-left translation is laid out from the right, in its own
	// paragraph only; the others are not.
	rtl := langtag.RTL(tag)
	if got := strings.Count(x, "<w:bidi/>"); (rtl && got != 1) || (!rtl && got != 0) {
		t.Errorf("%d paragraphs laid out right to left", got)
	}
	if got := strings.Contains(x, "<w:rtl/>"); got != rtl {
		t.Errorf("runs marked right to left: %v", got)
	}
	if !rtl {
		return
	}
	// Translating it back into English puts the paragraph left to right again.
	ext2, err := docx.Extract(out, tag, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := docx.Merge(out, tag, "en-GB", filter.Translations{ext2.Units[0].ID: content.Text(english)}, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if x := documentXML(t, again); strings.Contains(x, "<w:bidi/>") || strings.Contains(x, "<w:rtl/>") {
		t.Errorf("the English translation is still laid out right to left")
	}
}

func xliffFile(t *testing.T, tag string, s sample) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
<file source-language="en-GB" target-language="` + tag + `" datatype="plaintext" original="a.txt"><body>
<trans-unit id="1"><source>The contract begins today.</source></trans-unit>
<trans-unit id="2"><source>` + english + `</source><target>` + s.text + `</target></trans-unit>
</body></file></xliff>`
	ext, err := xliff.Extract([]byte(doc))
	if err != nil || len(ext.Units) != 2 {
		t.Fatalf("extract: %v", err)
	}
	if ext.TargetLang != tag {
		t.Errorf("target language read as %q", ext.TargetLang)
	}
	if got := ext.Units[1].Target.Plain(); got != s.text {
		t.Errorf("the translation in the file reads %q", got)
	}
	out, err := xliff.Merge([]byte(doc), map[string]xliff.Update{ext.Units[0].ID: {Target: content.Text(s.form), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	back, err := xliff.Extract(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := back.Units[0].Target.Plain(); got != s.form {
		t.Errorf("the saved translation reads %q", got)
	}
	if got := back.Units[1].Target.Plain(); got != s.text {
		t.Errorf("the other translation reads %q after saving", got)
	}
}
