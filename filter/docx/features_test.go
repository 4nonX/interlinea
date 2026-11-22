package docx_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	. "github.com/4nonX/interlinea/filter/docx/docxtest"
)

func featureTranslations(t *testing.T) filter.Translations {
	return filter.Translations{
		"document/p4/s1": content.Text("Note: interest is paid on the deposit."),
		"document/p4/s2": content.Text("The interest belongs to the tenant."),
		"document/p5/s1": mustParse(t, `[{"text":"Start of the lease: "},{"tag":{"kind":"open","id":"1"}},{"text":"1 January 2027"},{"tag":{"kind":"close","id":"1"}},{"text":" (binding)."}]`),
		"document/p7/s1": mustParse(t, `[{"text":"Service charges are paid "},{"tag":{"kind":"open","id":"1"}},{"text":"monthly"},{"tag":{"kind":"close","id":"1"}},{"text":" in advance."}]`),
		"comments/p1/s1": content.Text("Please clarify with the client."),
		"chart1/p1/s1":   content.Text("Rent and service charges 2027"),
		"chart1/v1/s1":   content.Text("Basic rent"),
		"chart1/v2/s1":   content.Text("January"),
		"data1/p3/s1":    mustParse(t, `[{"tag":{"kind":"open","id":"1"}},{"text":"Check"},{"tag":{"kind":"close","id":"1"}},{"text":" documents"}]`),
		"data1/p2/s1":    content.Text("Submit application"),
		"document/p6/s1": content.Text("The notice period is three months."),
		"document/p8/s1": mustParse(t, `[{"text":"What matters are "},{"tag":{"kind":"open","id":"1"}},{"text":"deadlines"},{"tag":{"kind":"close","id":"1"}},{"text":", "},{"tag":{"kind":"open","id":"2"}},{"text":"amounts"},{"tag":{"kind":"close","id":"2"}},{"text":" and "},{"tag":{"kind":"open","id":"3"}},{"text":"signatures"},{"tag":{"kind":"close","id":"3"}},{"text":"."}]`),
		"document/p1/s1": content.Text("Further content"),
		"document/p2/s1": content.Text("See the note in the box:"),
		"chart1/v3/s1":   content.Text("February"),
	}
}

func TestFeaturesMerge(t *testing.T) {
	src := Features()
	out, err := docx.Merge(src, "de-DE", "en-GB", featureTranslations(t), docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, out)
	doc := readPart(t, out, "word/document.xml")
	// The text box text is in the shape and in its fallback copy.
	if n := strings.Count(doc, "Note: interest is paid on the deposit."); n != 2 {
		t.Errorf("text box translated %d times, want 2 (shape and fallback)", n)
	}
	if strings.Contains(doc, "Kaution") {
		t.Error("German text left in the text box")
	}
	for _, want := range []string{
		`<w:sdtPr><w:alias w:val="Datum"/><w:tag w:val="start"/>`,
		`<w:sdtContent><w:r><w:rPr><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">1 January 2027</w:t></w:r></w:sdtContent></w:sdt>`,
		`<w:rPr><w:color w:val="C00000"/><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">monthly</w:t>`,
		`<w:rPr><w:rStyle w:val="Strong"/><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">signatures</w:t>`,
		`<w:commentRangeStart w:id="0"/>`, `<w:commentReference w:id="0"/>`,
		`The notice period is three months.`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document.xml lacks %s", want)
		}
	}
	if strings.Contains(doc, "<w:del ") || strings.Contains(doc, "<w:ins ") {
		t.Error("tracked changes should be accepted in the rewritten paragraph")
	}
	if c := readPart(t, out, "word/comments.xml"); !strings.Contains(c, "Please clarify with the client.") || !strings.Contains(c, "<w:annotationRef/>") {
		t.Errorf("comment not translated: %s", c)
	}
	chart := readPart(t, out, "word/charts/chart1.xml")
	for _, want := range []string{
		`<a:r><a:rPr lang="en-GB" sz="1400" b="1"/><a:t>Rent and service charges 2027</a:t></a:r>`,
		`<c:v>Basic rent</c:v>`, `<c:v>January</c:v>`, `<c:v>February</c:v>`, `<c:v>1250</c:v>`,
	} {
		if !strings.Contains(chart, want) {
			t.Errorf("chart lacks %s", want)
		}
	}
	// The embedded workbook has the translated labels, so "Edit data" keeps them.
	wb := readPart(t, out, "word/embeddings/Microsoft_Excel_Worksheet.xlsx")
	zr, err := zip.NewReader(strings.NewReader(wb), int64(len(wb)))
	if err != nil {
		t.Fatal(err)
	}
	var ss string
	for _, f := range zr.File {
		if f.Name == "xl/sharedStrings.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			ss = string(b)
		}
	}
	if !strings.Contains(ss, "<si><t>Basic rent</t></si><si><t>January</t></si><si><t>February</t></si>") {
		t.Errorf("embedded workbook not updated: %s", ss)
	}
	// SmartArt: data and cached drawing agree, bold kept.
	for _, name := range []string{"word/diagrams/data1.xml", "word/diagrams/drawing1.xml"} {
		x := readPart(t, out, name)
		for _, want := range []string{`<a:r><a:rPr lang="en-GB" b="1"/><a:t>Check</a:t></a:r><a:r><a:rPr lang="en-GB"/><a:t> documents</a:t></a:r>`, `<a:t>Submit application</a:t>`} {
			if !strings.Contains(x, want) {
				t.Errorf("%s lacks %s\n%s", name, want, x)
			}
		}
	}
	// Units come back from the translated file.
	got := units(t, out, "en-GB")
	if g := got["document/p5/s1"].Source.String(); g != "Start of the lease: {1}1 January 2027{/1} (binding)." {
		t.Errorf("content control unit: %q", g)
	}
}

func TestFeaturesKeepRevisions(t *testing.T) {
	src := Features()
	opts := docx.Options{KeepRevisions: true}
	ex, err := docx.Extract(src, "de-DE", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Warnings) != 1 || !strings.Contains(ex.Warnings[0], "kept as tags") {
		t.Errorf("warnings = %q", ex.Warnings)
	}
	tr := filter.Translations{"document/p6/s1": mustParse(t, `[{"text":"The notice period is "},{"tag":{"kind":"standalone","id":"1"}},{"tag":{"kind":"open","id":"2"}},{"text":"three"},{"tag":{"kind":"close","id":"2"}},{"text":" months."}]`)}
	out, err := docx.Merge(src, "de-DE", "en-GB", tr, opts)
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, out)
	doc := readPart(t, out, "word/document.xml")
	want := `<w:t xml:space="preserve">The notice period is </w:t></w:r><w:del w:id="10" w:author="Eva" w:date="2026-11-01T09:00:00Z"><w:r><w:delText>zwei</w:delText></w:r></w:del><w:ins w:id="11" w:author="Eva" w:date="2026-11-01T09:00:00Z"><w:r><w:rPr><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">three</w:t></w:r></w:ins>`
	if !strings.Contains(doc, want) {
		t.Errorf("tracked changes not kept:\n%s", doc)
	}
	acc, _ := docx.Extract(src, "de-DE", docx.Options{})
	if len(acc.Warnings) != 1 || !strings.Contains(acc.Warnings[0], "accepted") {
		t.Errorf("accept warning = %q", acc.Warnings)
	}
}

func TestBlocksForFeatures(t *testing.T) {
	ex, err := docx.Extract(Features(), "de-DE", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]filter.Block{}
	for _, b := range ex.Blocks {
		by[b.ID] = b
	}
	if b := by["document/p4"]; b.Frame != "textbox" {
		t.Errorf("text box block = %+v", b)
	}
	if b := by["comments/p1"]; b.Part != filter.PartComment || b.Note != "0" {
		t.Errorf("comment block = %+v", b)
	}
	if b := by["chart1/v2"]; b.Part != filter.PartChart || b.Style != "label" {
		t.Errorf("chart label block = %+v", b)
	}
	if b := by["data1/p3"]; b.Part != filter.PartDiagram {
		t.Errorf("diagram block = %+v", b)
	}
	if b := by["document/p6"]; !b.Revisions {
		t.Error("revisions not reported")
	}
	for _, b := range ex.Blocks {
		if strings.HasPrefix(b.ID, "drawing1") {
			t.Errorf("the SmartArt drawing cache must not add blocks: %s", b.ID)
		}
	}
}

func TestUntranslatedMirrorKeepsSource(t *testing.T) {
	// A fallback copy whose text differs from the shape is left alone.
	body := P("", `<w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><w:txbxContent>`+P("", R("Eins."))+`</w:txbxContent></w:drawing></mc:Choice><mc:Fallback><w:pict><w:txbxContent>`+P("", R("Anders."))+`</w:txbxContent></w:pict></mc:Fallback></mc:AlternateContent></w:r>`)
	data := Document(Parts{Body: body, Lang: "de-DE"})
	out, err := docx.Merge(data, "de-DE", "en-GB", filter.Translations{"document/p2/s1": content.Text("One.")}, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	doc := readPart(t, out, "word/document.xml")
	if !strings.Contains(doc, "One.") || !strings.Contains(doc, "Anders.") {
		t.Errorf("unexpected output: %s", doc)
	}
	_ = bytes.MinRead
}
