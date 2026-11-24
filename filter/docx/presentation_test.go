package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
)

func partOf(t *testing.T, pkg []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no part %s", name)
	return ""
}

func TestPresentationExtract(t *testing.T) {
	ex, err := ExtractPresentation(docxtest.Deck(), "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Format != "pptx" {
		t.Errorf("format %q", ex.Format)
	}
	var got []string
	for _, u := range ex.Units {
		got = append(got, u.ID+" "+u.Source.String())
	}
	// The first slide of the show is slide2.xml: the order is the
	// presentation's, not the file names'. Notes follow their slide, the
	// chart its slide; the master's prompt and the slide number are not read.
	want := []string{
		"slide2/p1/s1 Mietrecht 2027",
		"slide2/p2/s1 Was sich für Vermieter ändert.",
		"slide2/p2/s2 {1}Stand: November 2026{/1}.",
		"notesSlide2/p1/s1 Begrüßung.",
		"notesSlide2/p1/s2 Kurz die Agenda nennen.",
		"slide1/p1/s1 Fristen",
		"slide1/p2/s1 Kündigung: drei Monate",
		"slide1/p3/s1 Erste Zeile{1/}zweite Zeile",
		"slide1/p4/s1 Frist",
		"slide1/p5/s1 Dauer",
		"slide1/p6/s1 Kündigung",
		"slide1/p7/s1 3 Monate",
		"slide1/p8/s1 Quelle: BGB",
		"chart1/p1/s1 Miete und Nebenkosten 2027",
		"chart1/v1/s1 Nettokaltmiete",
		"chart1/v2/s1 Januar",
		"chart1/v3/s1 Februar",
		"notesSlide1/p1/s1 Auf die Tabelle hinweisen.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("units:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	blocks := map[string]filter.Block{}
	for _, b := range ex.Blocks {
		blocks[b.ID] = b
	}
	if b := blocks["notesSlide2/p1"]; b.Part != filter.PartNotes {
		t.Errorf("notes block: %+v", b)
	}
	if b := blocks["slide1/p7"]; b.Cell == nil || *b.Cell != (filter.Cell{Table: 0, Row: 1, Col: 1}) {
		t.Errorf("table cell block: %+v", b.Cell)
	}
	if b := blocks["slide1/p8"]; b.Cell != nil || b.Part != filter.PartBody {
		t.Errorf("text box after the table: %+v", b)
	}
}

func TestPresentationMerge(t *testing.T) {
	deck := docxtest.Deck()
	tr := filter.Translations{
		"slide2/p1/s1":      content.Text("Tenancy law 2027"),
		"slide2/p2/s1":      content.Text("What changes for landlords."),
		"slide2/p2/s2":      content.Content{}.AppendTag(content.Tag{Kind: content.TagOpen, ID: "1"}).Append("As of November 2026").AppendTag(content.Tag{Kind: content.TagClose, ID: "1"}).Append("."),
		"notesSlide2/p1/s1": content.Text("Welcome."),
		"slide1/p3/s1":      content.Text("First line").AppendTag(content.Tag{Kind: content.TagStandalone, ID: "1"}).Append("second line"),
		"slide1/p7/s1":      content.Text("3 months"),
		"chart1/v2/s1":      content.Text("January"),
	}
	out, err := MergePresentation(deck, "de-DE", "en-GB", tr)
	if err != nil {
		t.Fatal(err)
	}
	s2 := partOf(t, out, "ppt/slides/slide2.xml")
	for _, want := range []string{
		`<a:t>Tenancy law 2027</a:t>`,
		`<a:r><a:rPr lang="en-GB"/><a:t>What changes for landlords.</a:t></a:r>`,
		`<a:r><a:rPr lang="en-GB" b="1"/><a:t>As of November 2026</a:t></a:r>`,
	} {
		if !strings.Contains(s2, want) {
			t.Errorf("slide2 lacks %s:\n%s", want, s2)
		}
	}
	s1 := partOf(t, out, "ppt/slides/slide1.xml")
	for _, want := range []string{
		`<a:t>First line</a:t></a:r><a:br><a:rPr lang="de-DE"/></a:br><a:r><a:rPr lang="en-GB"/><a:t>second line</a:t>`,
		`<a:t>3 months</a:t>`,
		`<a:t>Fristen</a:t>`, // not translated: the source stays
		`<a:t>Quelle: BGB</a:t>`,
	} {
		if !strings.Contains(s1, want) {
			t.Errorf("slide1 lacks %s:\n%s", want, s1)
		}
	}
	notes := partOf(t, out, "ppt/notesSlides/notesSlide2.xml")
	if !strings.Contains(notes, `<a:t>Welcome.</a:t>`) || !strings.Contains(notes, `Kurz die Agenda nennen.`) || !strings.Contains(notes, `type="slidenum"`) {
		t.Errorf("notes: %s", notes)
	}
	chart := partOf(t, out, "ppt/charts/chart1.xml")
	if !strings.Contains(chart, `<c:v>January</c:v>`) || !strings.Contains(chart, `<c:v>Februar</c:v>`) {
		t.Errorf("chart: %s", chart)
	}
	// The chart's own data follows, so "Edit data" shows the translation.
	wb := partOf(t, out, "ppt/embeddings/chart1.xlsx")
	if ss := partOf(t, []byte(wb), "xl/sharedStrings.xml"); !strings.Contains(ss, "<t>January</t>") {
		t.Errorf("chart data: %s", ss)
	}
	// The master is untouched, and so is everything without a translation.
	if got, want := partOf(t, out, "ppt/slideMasters/slideMaster1.xml"), partOf(t, deck, "ppt/slideMasters/slideMaster1.xml"); got != want {
		t.Error("the slide master changed")
	}
	// Reading the result gives the translations as the source.
	back, err := ExtractPresentation(out, "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	if back.Units[0].Source.Plain() != "Tenancy law 2027" || len(back.Units) != 18 {
		t.Errorf("read back: %d units, first %q", len(back.Units), back.Units[0].Source.Plain())
	}
}

func TestPresentationWithoutTranslationsKeepsText(t *testing.T) {
	deck := docxtest.Deck()
	out, err := MergePresentation(deck, "de-DE", "en-GB", nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ExtractPresentation(deck, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ExtractPresentation(out, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Units) != len(b.Units) {
		t.Fatalf("%d units, then %d", len(a.Units), len(b.Units))
	}
	for i := range a.Units {
		if a.Units[i].Source.String() != b.Units[i].Source.String() {
			t.Errorf("unit %d: %q became %q", i, a.Units[i].Source.String(), b.Units[i].Source.String())
		}
	}
}

func TestPresentationRightToLeft(t *testing.T) {
	deck := docxtest.Presentation(docxtest.Slide{Shapes: docxtest.Shape(2, "title", docxtest.AP(docxtest.AR("Fristen"))) +
		docxtest.Shape(3, "body", docxtest.APR(`algn="r"`, docxtest.AR("Rechts")), docxtest.APR(`algn="ctr"`, docxtest.AR("Mitte")), docxtest.AP(docxtest.AR("Bleibt")))})
	out, err := MergePresentation(deck, "de-DE", "ar-SA", filter.Translations{
		"slide1/p1/s1": content.Text("المواعيد"),
		"slide1/p2/s1": content.Text("يمين"),
		"slide1/p3/s1": content.Text("وسط"),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := partOf(t, out, "ppt/slides/slide1.xml")
	for _, want := range []string{
		`<a:p><a:pPr algn="r" rtl="1"/><a:r><a:rPr lang="ar-SA"/><a:t>المواعيد</a:t>`,
		`<a:pPr algn="l" rtl="1"/><a:r><a:rPr lang="ar-SA"/><a:t>يمين</a:t>`,
		`<a:pPr algn="ctr" rtl="1"/><a:r><a:rPr lang="ar-SA"/><a:t>وسط</a:t>`,
		`<a:p><a:r><a:rPr lang="ar-SA"/><a:t>Bleibt</a:t>`, // not translated: not turned
	} {
		if !strings.Contains(s, want) {
			t.Errorf("lacks %s:\n%s", want, s)
		}
	}
	// And back: an Arabic paragraph translated into German reads from the left.
	back, err := MergePresentation(out, "ar-SA", "de-DE", filter.Translations{"slide1/p2/s1": content.Text("Rechts")})
	if err != nil {
		t.Fatal(err)
	}
	if s := partOf(t, back, "ppt/slides/slide1.xml"); !strings.Contains(s, `<a:pPr algn="r" rtl="0"/><a:r><a:rPr lang="de-DE"/><a:t>Rechts</a:t>`) {
		t.Errorf("turned back: %s", s)
	}
}

func TestNotAPresentation(t *testing.T) {
	if _, err := ExtractPresentation([]byte("not a zip"), "de-DE"); !errors.Is(err, ErrNotPresentation) {
		t.Errorf("plain bytes: %v", err)
	}
	if _, err := ExtractPresentation(docxtest.Hofmann(), "de-DE"); !errors.Is(err, ErrNotPresentation) {
		t.Errorf("a Word document: %v", err)
	}
	if _, err := Extract(docxtest.Deck(), "de-DE", Options{}); !errors.Is(err, ErrNotDocx) {
		t.Errorf("a presentation read as Word: %v", err)
	}
}
