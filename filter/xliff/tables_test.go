package xliff

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
)

func tableFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/tables/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func zipPart(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatalf("no %s", name)
	return ""
}

// checkMemoQ checks the units of the memoQ fixtures, which hold the same
// segments as RTF and as DOCX.
func checkMemoQ(t *testing.T, format string, data []byte) {
	t.Helper()
	if f, ok := Detect(data); !ok || f != format {
		t.Fatalf("detected %q %v", f, ok)
	}
	ex, m := units(t, data)
	if ex.Format != format || ex.SourceLang != "de" || ex.TargetLang != "en-GB" || len(ex.Warnings) != 0 {
		t.Fatalf("extraction %s %s>%s %v", ex.Format, ex.SourceLang, ex.TargetLang, ex.Warnings)
	}
	if len(ex.Blocks) != 1 || ex.Blocks[0].File != "Mietvertrag.docx" {
		t.Errorf("blocks %+v", ex.Blocks)
	}
	u := m["a1b2c3d4-0001-4000-8000-000000000001"]
	if show(u.Source) != "Der {1}Mieter{/1} zahlt die Miete." || u.Key != "1" || u.State != filter.StateNew || u.Target != nil {
		t.Errorf("1: %q %+v", show(u.Source), u)
	}
	u = m["a1b2c3d4-0002-4000-8000-000000000002"]
	if show(u.Source) != "Miete: 500 €" || show(u.Target) != "Rent: €500" || u.State != filter.StateDraft || u.Match != 99 || u.Origin != "tm" ||
		len(u.Notes) != 1 || u.Notes[0] != "Betrag prüfen" {
		t.Errorf("2: %q %q %+v", show(u.Source), show(u.Target), u)
	}
	// memoQ writes the same code alike more than once: [4] [4].
	u = m["a1b2c3d4-0003-4000-8000-000000000003"]
	if show(u.Source) != "{3/} Kaution {4/}{4.3/}" || show(u.Target) != "{3/} Deposit {4/}{4.3/}" || u.State != filter.StateTranslated {
		t.Errorf("3: %q %q %+v", show(u.Source), show(u.Target), u)
	}
	if u.Source[0].Tag.Native != "[3]" {
		t.Errorf("tag %+v", u.Source[0].Tag)
	}
}

func TestMemoQRTF(t *testing.T) {
	data := tableFile(t, "memoq-two-column.rtf")
	checkMemoQ(t, FormatMQRTF, data)
	_, m := units(t, data)
	if u := m["a1b2c3d4-0004-4000-8000-000000000004"]; show(u.Source) != "Ölheizung" {
		t.Errorf("4: %q", show(u.Source))
	}
	if !Fixed(FormatMQRTF) {
		t.Error("memoQ keeps its segments")
	}
	out, err := Merge(data, map[string]Update{
		"a1b2c3d4-0001-4000-8000-000000000001": {Target: parse("The {1}tenant{/1} pays the rent."), State: filter.StateTranslated},
		"a1b2c3d4-0002-4000-8000-000000000002": {State: filter.StateTranslated},
		"a1b2c3d4-0004-4000-8000-000000000004": {Target: content.Text("Oil heating {ü} 😀"), State: filter.StateDraft},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`{\noproof\cs99\f0\fs20\cf13 [1\}}tenant{\noproof\cs99\f0\fs20\cf13 \{2]} pays the rent.}\cell }`,
		`Translator confirmed}\cell }`,
		`Translator confirmed (99%)}\cell }`,
		`Oil heating \{\u252?\} \u-10179?\u-8704?}\cell }`,
		`Edited}\cell }`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s", want)
		}
	}
	ex2, m2 := units(t, out)
	if len(ex2.Units) != 4 {
		t.Fatalf("%d units", len(ex2.Units))
	}
	if u := m2["a1b2c3d4-0001-4000-8000-000000000001"]; show(u.Target) != "The {1}tenant{/1} pays the rent." || u.State != filter.StateTranslated {
		t.Errorf("1 back: %q %s", show(u.Target), u.State)
	}
	if u := m2["a1b2c3d4-0002-4000-8000-000000000002"]; show(u.Target) != "Rent: €500" || u.State != filter.StateTranslated || u.Match != 99 {
		t.Errorf("2 back: %q %+v", show(u.Target), u)
	}
	if u := m2["a1b2c3d4-0004-4000-8000-000000000004"]; u.Target.Plain() != "Oil heating {ü} 😀" || u.Target.HasTags() || u.State != filter.StateDraft {
		t.Errorf("4 back: %q %s", show(u.Target), u.State)
	}
	// What did not change is as it was.
	if !strings.HasPrefix(s, string(data[:bytes.Index(data, []byte("a1b2c3d4-0001"))])) {
		t.Error("the head of the file changed")
	}
	if _, err := Merge(data, map[string]Update{"a1b2c3d4-0001-4000-8000-000000000001": {Target: parse("x {9/}")}}); !errors.Is(err, ErrTags) {
		t.Errorf("unknown tag: %v", err)
	}
	if _, err := Merge(data, map[string]Update{"nope": {Target: parse("x")}}); !errors.Is(err, ErrNoSegment) {
		t.Errorf("unknown segment: %v", err)
	}
	if _, err := Merge(data, map[string]Update{"a1b2c3d4-0001-4000-8000-000000000001": {Target: parse("x"), Joined: []string{"a1b2c3d4-0002-4000-8000-000000000002"}}}); err == nil {
		t.Error("segments were joined")
	}
}

func TestMemoQDOCX(t *testing.T) {
	data := tableFile(t, "memoq-two-column.docx")
	checkMemoQ(t, FormatMQDOCX, data)
	if !IsReviewDOCX(data) {
		t.Fatal("not a table for review")
	}
	out, err := Merge(data, map[string]Update{
		"a1b2c3d4-0001-4000-8000-000000000001": {Target: parse("The {1}tenant{/1} pays."), State: filter.StateTranslated},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := zipPart(t, out, "word/document.xml")
	if !strings.Contains(doc, `<w:r><w:rPr><w:rStyle w:val="mqInternal"/><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">[1}</w:t></w:r>`) {
		t.Errorf("tag not in memoQ's style: %s", doc)
	}
	_, m := units(t, out)
	if u := m["a1b2c3d4-0001-4000-8000-000000000001"]; show(u.Target) != "The {1}tenant{/1} pays." || u.State != filter.StateTranslated {
		t.Errorf("back: %q %s", show(u.Target), u.State)
	}
}

func TestDejaVuRTF(t *testing.T) {
	data := tableFile(t, "dejavu-external-view.rtf")
	if f, ok := Detect(data); !ok || f != FormatDVRTF {
		t.Fatalf("detected %q %v", f, ok)
	}
	ex, m := units(t, data)
	if ex.SourceLang != "de-DE" || ex.TargetLang != "en-GB" || len(ex.Units) != 3 {
		t.Fatalf("%s>%s %d", ex.SourceLang, ex.TargetLang, len(ex.Units))
	}
	if u := m["0000001"]; show(u.Source) != "Bitte {00108/}nicht{00109/} knicken." || u.State != filter.StateNew || u.Target != nil {
		t.Errorf("1: %q %+v", show(u.Source), u)
	}
	if u := m["0000002"]; show(u.Source) != "Größe: 30 cm" || show(u.Target) != "Size: 30 cm" || u.State != filter.StateTranslated || len(u.Notes) != 1 || u.Notes[0] != "Maß prüfen" {
		t.Errorf("2: %+v", u)
	}
	out, err := Merge(data, map[string]Update{"0000001": {Target: parse("Please do {00108/}not{00109/} fold."), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `\insrsid9000367 {\uc1 Please do \{00108\}not\{00109\} fold.}\cell }`) {
		t.Errorf("target cell: %s", out)
	}
	_, m2 := units(t, out)
	if u := m2["0000001"]; show(u.Target) != "Please do {00108/}not{00109/} fold." || u.State != filter.StateTranslated {
		t.Errorf("back: %q", show(u.Target))
	}
	if u := m2["0000002"]; show(u.Target) != "Size: 30 cm" {
		t.Errorf("2 changed: %q", show(u.Target))
	}
}

func TestTradosReview(t *testing.T) {
	data := tableFile(t, "lieferschein.docx.review.docx")
	if f, ok := Detect(data); !ok || f != FormatSDLReview {
		t.Fatalf("detected %q %v", f, ok)
	}
	ex, m := units(t, data)
	if ex.SourceLang != "de-DE" || ex.TargetLang != "en-GB" || len(ex.Units) != 3 {
		t.Fatalf("%s>%s %d", ex.SourceLang, ex.TargetLang, len(ex.Units))
	}
	u := m["8c9f2b9e-0000-4000-8000-000000000001/1"]
	if show(u.Source) != "{1}Achtung:{/1} Nicht {2/} knicken." || u.Key != "1" || u.State != filter.StateNew || u.Target != nil {
		t.Errorf("1: %q %+v", show(u.Source), u)
	}
	u = m["8c9f2b9e-0000-4000-8000-000000000002/2"]
	if show(u.Target) != "Size: 30 cm" || u.State != filter.StateDraft || u.Match != 77 || len(u.Notes) != 1 || u.Notes[0] != "Anna: Maß bitte prüfen" {
		t.Errorf("2: %+v", u)
	}
	u = m["8c9f2b9e-0000-4000-8000-000000000003/3"]
	if show(u.Source) != "Modell {locked1/} prüfen" || show(u.Target) != "Check model {locked1/}" || u.State != filter.StateTranslated || u.Match != 100 {
		t.Errorf("3: %q %q %+v", show(u.Source), show(u.Target), u)
	}
	out, err := Merge(data, map[string]Update{
		"8c9f2b9e-0000-4000-8000-000000000001/1": {Target: parse("{1}Caution:{/1} Do not {2/} fold."), State: filter.StateTranslated},
		"8c9f2b9e-0000-4000-8000-000000000002/2": {State: filter.StateTranslated},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := zipPart(t, out, "word/document.xml")
	for _, want := range []string{
		`<w:p><w:pPr><w:rPr><w:lang w:val="en-GB" /></w:rPr></w:pPr><w:r><w:rPr><w:rStyle w:val="Tag"/><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">&lt;1&gt;</w:t></w:r><w:r><w:rPr><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">Caution:</w:t></w:r>`,
		`<w:t xml:space="preserve">&lt;2/&gt;</w:t>`,
		`<w:t xml:space="preserve">Translated</w:t>`,
		`<w:t xml:space="preserve">Translated (77%)</w:t>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s", want)
		}
	}
	// Studio's record of the segments stays as it was.
	if zipPart(t, out, "customXML/item.xml") != zipPart(t, data, "customXML/item.xml") {
		t.Error("the custom XML part changed")
	}
	_, m2 := units(t, out)
	if u := m2["8c9f2b9e-0000-4000-8000-000000000001/1"]; show(u.Target) != "{1}Caution:{/1} Do not {2/} fold." || u.State != filter.StateTranslated {
		t.Errorf("1 back: %q %s", show(u.Target), u.State)
	}
	if u := m2["8c9f2b9e-0000-4000-8000-000000000003/3"]; show(u.Target) != "Check model {locked1/}" {
		t.Errorf("3 changed: %q", show(u.Target))
	}
	// A tag the reviewer typed without the Tag style is the source's tag;
	// one the source does not have is text.
	typed := strings.Replace(zipPart(t, data, "word/document.xml"), `<w:t xml:space="preserve">Size: 30 cm</w:t>`, `<w:t xml:space="preserve">Size: &lt;9&gt;30 cm</w:t>`, 1)
	typed = strings.Replace(typed, `<w:lang w:val="en-GB" /></w:rPr></w:pPr></w:p></w:tc></w:tr>`, `<w:lang w:val="en-GB" /></w:rPr></w:pPr><w:r><w:t>&lt;1&gt;Note:&lt;/1&gt; &lt;2/&gt;</w:t></w:r></w:p></w:tc></w:tr>`, 1)
	zr, err := docx.OpenPackage(data)
	if err != nil {
		t.Fatal(err)
	}
	edited, err := docx.RewritePackage(zr, map[string][]byte{"word/document.xml": []byte(typed)})
	if err != nil {
		t.Fatal(err)
	}
	_, m3 := units(t, edited)
	if u := m3["8c9f2b9e-0000-4000-8000-000000000001/1"]; show(u.Target) != "{1}Note:{/1} {2/}" || u.State != filter.StateDraft {
		t.Errorf("typed tags: %q %s", show(u.Target), u.State)
	}
	if u := m3["8c9f2b9e-0000-4000-8000-000000000002/2"]; show(u.Target) != "Size: <9>30 cm" {
		t.Errorf("tag the source lacks: %q", show(u.Target))
	}
	if _, err := Merge(data, map[string]Update{"8c9f2b9e-0000-4000-8000-000000000003/3": {Target: parse("Check {locked2/}")}}); !errors.Is(err, ErrTags) {
		t.Errorf("unknown locked content: %v", err)
	}
	// A Word document that is no table for review is not one.
	if IsReviewDOCX(tableFile(t, "memoq-two-column.rtf")) {
		t.Error("RTF taken for a Word document")
	}
}
