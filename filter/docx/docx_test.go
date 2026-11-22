package docx_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	. "github.com/4nonX/interlinea/filter/docx/docxtest"
)

func units(t *testing.T, data []byte, lang string) map[string]filter.Unit {
	t.Helper()
	ex, err := docx.Extract(data, lang, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]filter.Unit{}
	for _, u := range ex.Units {
		if err := u.Source.Validate(); err != nil {
			t.Fatalf("%s: %v", u.ID, err)
		}
		out[u.ID] = u
	}
	return out
}

func readPart(t *testing.T, data []byte, name string) string {
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
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no part %s", name)
	return ""
}

func wellFormed(t *testing.T, data []byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xml") && !strings.HasSuffix(f.Name, ".rels") {
			continue
		}
		rc, _ := f.Open()
		d := xml.NewDecoder(rc)
		for {
			_, err := d.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("%s is not well-formed: %v", f.Name, err)
			}
		}
		rc.Close()
	}
}

func TestExtractSample(t *testing.T) {
	u := units(t, Hofmann(), "de-DE")
	cases := map[string]string{
		"document/p1/s1":  "Mietvertrag über Wohnraum",
		"document/p2/s1":  "Zwischen {1}Herrn Jonas Hofmann{/1} (Vermieter) und {2}Frau Lea Kraus{/2} (Mieterin) wird folgender Mietvertrag geschlossen:",
		"document/p4/s2":  "Die Mietsache besteht aus drei Zimmern, einer Küche, einem Bad und einem Kellerraum.",
		"document/p6/s2":  "Die ordentliche Kündigung richtet sich nach den gesetzlichen Vorschriften.",
		"document/p13/s1": "Gesamtmiete",
		"document/p20/s1": "Das Recht zur Mietminderung{1/} bleibt unberührt.",
		"document/p23/s1": "Die genannten Gesetze sind unter {1}Gesetze im Internet{/1} abrufbar.",
		"footnotes/p1/s1": "Siehe §§ 573 ff. BGB.",
		"footer1/p1/s1":   "Seite {1/} von",
	}
	for id, want := range cases {
		got, ok := u[id]
		if !ok {
			t.Errorf("missing unit %s", id)
			continue
		}
		if got.Source.String() != want {
			t.Errorf("%s:\n got %q\nwant %q", id, got.Source.String(), want)
		}
	}
	if len(u) != 38 {
		t.Errorf("got %d units, want 38", len(u))
	}
	// Tag types and native code.
	tags := u["document/p2/s1"].Source.Tags()
	if tags[0].Type != "bold" || !strings.Contains(tags[0].Native, "<w:b/>") {
		t.Errorf("bold tag = %+v", tags[0])
	}
	if tg := u["document/p20/s1"].Source.Tags()[0]; tg.Type != "footnote" || !strings.Contains(tg.Native, `w:footnoteReference w:id="2"`) {
		t.Errorf("footnote tag = %+v", tg)
	}
	if tg := u["document/p23/s1"].Source.Tags()[0]; tg.Type != "link" || !strings.HasPrefix(tg.Native, `<w:hyperlink r:id="rIdLink1"`) {
		t.Errorf("link tag = %+v", tg)
	}
	if tg := u["footer1/p1/s1"].Source.Tags()[0]; tg.Type != "field" || !strings.Contains(tg.Native, "PAGE") {
		t.Errorf("field tag = %+v", tg)
	}
}

func TestBlocks(t *testing.T) {
	ex, err := docx.Extract(Hofmann(), "de-DE", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]filter.Block{}
	for _, b := range ex.Blocks {
		byID[b.ID] = b
	}
	if b := byID["document/p3"]; b.Heading != 1 || b.Style != "Heading1" {
		t.Errorf("heading block = %+v", b)
	}
	if b := byID["document/p14"]; b.Cell == nil || b.Cell.Row != 2 || b.Cell.Col != 1 || b.Align != "right" {
		t.Errorf("table cell block = %+v cell %+v", b, b.Cell)
	}
	if b := byID["footnotes/p2"]; b.Part != filter.PartFootnote || b.Note != "2" {
		t.Errorf("footnote block = %+v", b)
	}
	if b := byID["document/p6"]; len(b.Trail) != 1 || b.Trail[0] != "footnote" {
		t.Errorf("footnote mark at the paragraph end = %+v", b.Trail)
	}
	if b := byID["footnotes/p1"]; len(b.Lead) != 1 || b.Lead[0] != "note-mark" {
		t.Errorf("footnote number at the start = %+v", b.Lead)
	}
	if b := byID["footer1/p1"]; b.Part != filter.PartFooter || len(b.Trail) != 1 || b.Trail[0] != "field" {
		t.Errorf("footer block = %+v", b)
	}
}

func TestMergeWithoutTranslationsKeepsText(t *testing.T) {
	src := Hofmann()
	out, err := docx.Merge(src, "de-DE", "de-DE", nil, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, out)
	before := units(t, src, "de-DE")
	after := units(t, out, "de-DE")
	if len(before) != len(after) {
		t.Fatalf("unit count changed: %d -> %d", len(before), len(after))
	}
	for id, b := range before {
		if a := after[id]; !a.Source.Equal(b.Source) {
			t.Errorf("%s changed:\n%s\n%s", id, b.Source, a.Source)
		}
	}
	for _, name := range []string{"word/styles.xml", "word/settings.xml", "[Content_Types].xml", "word/_rels/document.xml.rels"} {
		if readPart(t, src, name) != readPart(t, out, name) {
			t.Errorf("%s changed", name)
		}
	}
	// Bytes outside translated paragraphs stay identical: the section
	// properties and the empty paragraph.
	doc := readPart(t, out, "word/document.xml")
	if !strings.Contains(doc, `<w:sectPr><w:headerReference w:type="default" r:id="rIdHeader1"/>`) || !strings.Contains(doc, `<w:p><w:r><w:t xml:space="preserve"></w:t></w:r></w:p>`) {
		t.Error("untouched markup was rewritten")
	}
}

func TestMergeTranslation(t *testing.T) {
	src := Hofmann()
	tr := filter.Translations{
		"document/p1/s1": content.Text("Residential lease agreement"),
		// Tags moved and the bold names kept.
		"document/p2/s1":  mustParse(t, `[{"text":"The following lease is concluded between "},{"tag":{"kind":"open","id":"1"}},{"text":"Mr Jonas Hofmann"},{"tag":{"kind":"close","id":"1"}},{"text":" (landlord) and "},{"tag":{"kind":"open","id":"2"}},{"text":"Ms Lea Kraus"},{"tag":{"kind":"close","id":"2"}},{"text":" (tenant):"}]`),
		"document/p8/s1":  mustParse(t, `[{"text":"The monthly rent is "},{"tag":{"kind":"open","id":"1"}},{"text":"EUR 1,250.00"},{"tag":{"kind":"close","id":"1"}},{"text":" plus an advance payment of "},{"tag":{"kind":"open","id":"2"}},{"text":"EUR 220.00"},{"tag":{"kind":"close","id":"2"}},{"text":" for service charges."}]`),
		"document/p20/s1": mustParse(t, `[{"text":"The right to reduce the rent"},{"tag":{"kind":"standalone","id":"1"}},{"text":" remains unaffected."}]`),
		"document/p23/s1": mustParse(t, `[{"text":"The statutes cited are available at "},{"tag":{"kind":"open","id":"1"}},{"text":"Gesetze im Internet"},{"tag":{"kind":"close","id":"1"}},{"text":"."}]`),
		"footnotes/p1/s1": content.Text("See sections 573 et seq. BGB."),
		"footer1/p1/s1":   mustParse(t, `[{"text":"Page "},{"tag":{"kind":"standalone","id":"1"}},{"text":" of"}]`),
		"document/p25/s1": content.Text("Place, date"),
	}
	out, err := docx.Merge(src, "de-DE", "en-GB", tr, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, out)
	got := units(t, out, "en-GB")
	for id, want := range map[string]string{
		"document/p1/s1":  "Residential lease agreement",
		"document/p2/s1":  "The following lease is concluded between {1}Mr Jonas Hofmann{/1} (landlord) and {2}Ms Lea Kraus{/2} (tenant):",
		"document/p8/s1":  "The monthly rent is {1}EUR 1,250.00{/1} plus an advance payment of {2}EUR 220.00{/2} for service charges.",
		"document/p20/s1": "The right to reduce the rent{1/} remains unaffected.",
		"document/p23/s1": "The statutes cited are available at {1}Gesetze im Internet{/1}.",
		"footer1/p1/s1":   "Page {1/} of",
		"document/p25/s1": "Place, date",
		"document/p26/s1": "Ort, Datum", // not translated: source kept
	} {
		if g := got[id].Source.String(); g != want {
			t.Errorf("%s:\n got %q\nwant %q", id, g, want)
		}
	}
	doc := readPart(t, out, "word/document.xml")
	for _, want := range []string{
		`<w:r><w:rPr><w:b/><w:bCs/><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">EUR 1,250.00</w:t></w:r>`,
		`<w:hyperlink r:id="rIdLink1" w:history="1"><w:r><w:rPr><w:rStyle w:val="Hyperlink"/><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">Gesetze im Internet</w:t></w:r></w:hyperlink>`,
		`<w:footnoteReference w:id="2"/>`,
		`<w:footnoteReference w:id="1"/>`, // at the paragraph edge, kept outside the unit
		`<w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:rPr><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">Residential lease agreement</w:t></w:r>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document.xml lacks %s", want)
		}
	}
	footer := readPart(t, out, "word/footer1.xml")
	if !strings.Contains(footer, `<w:instrText xml:space="preserve"> PAGE </w:instrText>`) || !strings.Contains(footer, `<w:instrText xml:space="preserve"> NUMPAGES </w:instrText>`) {
		t.Errorf("fields lost in footer: %s", footer)
	}
	notes := readPart(t, out, "word/footnotes.xml")
	if !strings.Contains(notes, `<w:footnoteRef/></w:r>`) || !strings.Contains(notes, "See sections 573 et seq. BGB.") || !strings.Contains(notes, `w:type="separator"`) {
		t.Errorf("footnotes not rebuilt correctly: %s", notes)
	}
}

func TestMergeDamagedTags(t *testing.T) {
	// A translation that drops a tag, leaves a span open and adds an
	// unknown tag still produces a valid document.
	tr := filter.Translations{
		"document/p2/s1":  mustParse(t, `[{"tag":{"kind":"open","id":"1"}},{"text":"Mr Hofmann"},{"text":" and Ms Kraus"},{"tag":{"kind":"standalone","id":"9"}}]`),
		"document/p20/s1": content.Text("The right to reduce the rent remains unaffected."),
		"document/p23/s1": mustParse(t, `[{"text":"The statutes cited can be found at "},{"tag":{"kind":"open","id":"1"}},{"text":"the site"}]`),
	}
	out, err := docx.Merge(Hofmann(), "de-DE", "en-GB", tr, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, out)
	got := units(t, out, "en-GB")
	if g := got["document/p2/s1"].Source.String(); g != "Mr Hofmann and Ms Kraus" {
		t.Errorf("open span is closed at the unit end and becomes paragraph formatting: %q", g)
	}
	if g := got["document/p23/s1"].Source.String(); g != "The statutes cited can be found at {1}the site{/1}" {
		t.Errorf("unclosed link: %q", g)
	}
	if strings.Contains(readPart(t, out, "word/document.xml"), `w:footnoteReference w:id="2"`) {
		t.Error("a footnote reference left out of the translation should be gone")
	}
}

func TestSentenceBreakInsideTagsIsNotUsed(t *testing.T) {
	data := Document(Parts{Body: P("", R("Erster Satz. "), B("Zweiter Satz. Dritter"), R(" Satz. Vierter Satz.")), Lang: "de-DE"})
	ex, err := docx.Extract(data, "de-DE", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range ex.Units {
		got = append(got, u.Source.String())
	}
	want := []string{"Erster Satz.", "{1}Zweiter Satz. Dritter{/1} Satz.", "Vierter Satz."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
	// Merging back keeps the spaces between sentences.
	out, err := docx.Merge(data, "de-DE", "en-GB", filter.Translations{ex.Units[0].ID: content.Text("First sentence.")}, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	doc := readPart(t, out, "word/document.xml")
	if !strings.Contains(doc, `First sentence.</w:t></w:r><w:r><w:rPr><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve"> </w:t></w:r><w:r><w:rPr><w:b/><w:bCs/>`) {
		t.Errorf("gap lost: %s", doc)
	}
}

func TestEdgesAndTrackedChanges(t *testing.T) {
	body := `<w:p><w:bookmarkStart w:id="0" w:name="_Ref1"/>` + Link("rIdL", "Whole paragraph link.") + `<w:bookmarkEnd w:id="0"/></w:p>` +
		`<w:p>` + R("Kept ") + `<w:ins w:id="1" w:author="A"><w:r><w:t>inserted</w:t></w:r></w:ins><w:del w:id="2" w:author="A"><w:r><w:delText>deleted</w:delText></w:r></w:del>` + R(" text.") + `</w:p>` +
		`<w:p>` + R("Page ") + Field("PAGE", "3") + R(" of the file.") + `</w:p>` +
		`<w:p>` + R("Tab") + Tab() + R("stop and ") + `<w:r><w:t>soft</w:t><w:softHyphen/><w:t>hyphen</w:t></w:r>` + R(".") + `</w:p>`
	data := Document(Parts{Body: body, Links: map[string]string{"rIdL": "https://example.org"}})
	ex, err := docx.Extract(data, "en-GB", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range ex.Units {
		got = append(got, u.Source.String())
	}
	want := []string{"Whole paragraph link.", "Kept inserted text.", "Page {1/} of the file.", "Tab{1/}stop and soft\u00adhyphen."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q\nwant %q", got, want)
	}
	tr := filter.Translations{}
	for _, u := range ex.Units {
		tr[u.ID] = u.Source
	}
	out, err := docx.Merge(data, "en-GB", "en-GB", tr, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, out)
	doc := readPart(t, out, "word/document.xml")
	for _, want := range []string{`<w:bookmarkStart w:id="0" w:name="_Ref1"/><w:hyperlink r:id="rIdL" w:history="1">`, `</w:hyperlink><w:bookmarkEnd w:id="0"/></w:p>`, `<w:softHyphen/>`, `<w:tab/>`, ` PAGE `} {
		if !strings.Contains(doc, want) {
			t.Errorf("output lacks %s", want)
		}
	}
	if strings.Contains(doc, "deleted") {
		t.Error("tracked deletion should be removed when the paragraph is rewritten")
	}
}

func TestSetLanguageAttribute(t *testing.T) {
	body := P("", RP(`<w:b/><w:lang w:val="de-DE" w:eastAsia="zh-CN"/><w:rPrChange w:id="5" w:author="A"><w:rPr/></w:rPrChange>`, "Fett.")) +
		P("", `<w:r><w:rPr/><w:t>Leer.</w:t></w:r>`) +
		P("", RP(`<w:i/><w:rPrChange w:id="6" w:author="A"><w:rPr/></w:rPrChange>`, "Kursiv."))
	data := Document(Parts{Body: body})
	ex, err := docx.Extract(data, "de-DE", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tr := filter.Translations{}
	for _, u := range ex.Units {
		tr[u.ID] = u.Source
	}
	for lang, want := range map[string][]string{
		"en-GB": {`<w:lang w:val="en-GB" w:eastAsia="zh-CN"/><w:rPrChange`, `<w:rPr><w:lang w:val="en-GB"/></w:rPr><w:t xml:space="preserve">Leer.`, `<w:i/><w:lang w:val="en-GB"/><w:rPrChange`},
		"ja-JP": {`<w:lang w:val="de-DE" w:eastAsia="ja-JP"/>`, `<w:lang w:eastAsia="ja-JP"/>`},
		"ar-SA": {`<w:lang w:val="de-DE" w:eastAsia="zh-CN" w:bidi="ar-SA"/>`},
	} {
		out, err := docx.Merge(data, "de-DE", lang, tr, docx.Options{})
		if err != nil {
			t.Fatal(err)
		}
		wellFormed(t, out)
		doc := readPart(t, out, "word/document.xml")
		for _, w := range want {
			if !strings.Contains(doc, w) {
				t.Errorf("%s: output lacks %s\n%s", lang, w, doc)
			}
		}
	}
}

func TestNotADocx(t *testing.T) {
	if _, err := docx.Extract([]byte("plain text"), "en", docx.Options{}); !errors.Is(err, docx.ErrNotDocx) {
		t.Errorf("err = %v, want ErrNotDocx", err)
	}
	empty := Build(map[string]string{"hello.txt": "hi"})
	if _, err := docx.Extract(empty, "en", docx.Options{}); !errors.Is(err, docx.ErrNotDocx) {
		t.Errorf("zip without document: err = %v", err)
	}
}

func TestMergeIsDeterministic(t *testing.T) {
	tr := filter.Translations{"document/p1/s1": content.Text("Lease")}
	a, err := docx.Merge(Hofmann(), "de-DE", "en-GB", tr, docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := docx.Merge(Hofmann(), "de-DE", "en-GB", tr, docx.Options{})
	if !bytes.Equal(a, b) {
		t.Error("same input gave different output")
	}
}

func mustParse(t *testing.T, js string) content.Content {
	t.Helper()
	var c content.Content
	if err := c.UnmarshalJSON([]byte(js)); err != nil {
		t.Fatal(err)
	}
	return c
}
