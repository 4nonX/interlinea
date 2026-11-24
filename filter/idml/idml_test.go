package idml

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
)

const head = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
const pkgNS = `xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging" DOMVersion="8.0"`
const none = `AppliedCharacterStyle="CharacterStyle/$ID/[No character style]"`

func csr(attrs string, kids ...string) string {
	if attrs != "" {
		attrs = " " + attrs
	}
	return `<CharacterStyleRange ` + none + attrs + `>` + strings.Join(kids, "") + `</CharacterStyleRange>`
}

func psr(style string, kids ...string) string {
	return `<ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/` + style + `">` + strings.Join(kids, "") + `</ParagraphStyleRange>`
}

func text(s string) string { return `<Content>` + s + `</Content>` }

func storyFile(id string, body string) string {
	return head + `<idPkg:Story ` + pkgNS + `><Story Self="` + id + `" AppliedTOCStyle="n" TrackChanges="false" StoryTitle="$ID/" AppliedNamedGrid="n"><StoryPreference OpticalMarginAlignment="false" OpticalMarginSize="12" FrameType="TextFrameType" StoryOrientation="Horizontal" StoryDirection="LeftToRightDirection"/>` + body + `</Story></idPkg:Story>`
}

// lease is an IDML file with three stories. The frame of the second
// stands first on the page, the third is on no page, and the first has
// everything: two paragraphs in one style range, another format, a link,
// a forced line break, a page number, a table, a footnote and a note.
func lease() []byte {
	main := psr("Titel", csr("", text("Mietvertrag"), `<Br/>`)) +
		psr("Text",
			csr("", text("Die Kündigung muss ")),
			csr(`FontStyle="Bold"`, text("schriftlich")),
			csr("", text(" erfolgen. Sie ist an den "), `<HyperlinkTextSource Self="Hyperlink_1" Name="Vermieter" Hidden="false" AppliedCharacterStyle="n">`+text("Vermieter")+`</HyperlinkTextSource>`, text(" zu richten."), `<Br/>`,
				text("Zeile eins Zeile zwei, Seite <?ACE 18?> von 3"), `<Br/>`),
		) +
		psr("Text",
			csr("", text("Fristen"),
				`<Footnote><ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Fussnote"><CharacterStyleRange `+none+`><Content><?ACE 4?></Content></CharacterStyleRange><CharacterStyleRange `+none+`><Content> Siehe § 573c BGB.</Content></CharacterStyleRange></ParagraphStyleRange></Footnote>`,
				text(" im Überblick:"),
				`<Note Collapsed="false" CreationDate="2026-11-01T09:00:00" UserName="Eva"><ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle"><CharacterStyleRange `+none+`><Content>Noch prüfen</Content></CharacterStyleRange></ParagraphStyleRange></Note>`,
				`<Br/>`,
				`<Table Self="t1" HeaderRowCount="1" BodyRowCount="1" ColumnCount="2"><Row Self="t1r0" Name="0"/><Row Self="t1r1" Name="1"/><Column Self="t1c0" Name="0"/><Column Self="t1c1" Name="1"/>`+
					`<Cell Self="t1i0" Name="0:0">`+psr("Zelle", csr("", text("Frist")))+`</Cell>`+
					`<Cell Self="t1i1" Name="1:0">`+psr("Zelle", csr("", text("Dauer")))+`</Cell>`+
					`<Cell Self="t1i2" Name="0:1">`+psr("Zelle", csr("", text("Kündigung")))+`</Cell>`+
					`<Cell Self="t1i3" Name="1:1">`+psr("Zelle", csr("", text("3 Monate")))+`</Cell>`+
					`</Table>`,
				`<Br/>`),
			csr("", text("Schluss &amp; Ende")),
		)
	return pack(map[string]string{
		"mimetype":                          "application/vnd.adobe.indesign-idml-package",
		"designmap.xml":                     head + `<?aid style="50" type="document" readerVersion="6.0" featureSet="257" product="8.0(370)" ?>` + "\n" + `<Document ` + pkgNS + ` Self="d" StoryList="u1 u2 u3" Name="Mietvertrag.indd"><idPkg:MasterSpread src="MasterSpreads/MasterSpread_um.xml"/><idPkg:Spread src="Spreads/Spread_ua.xml"/><idPkg:Story src="Stories/Story_u1.xml"/><idPkg:Story src="Stories/Story_u2.xml"/><idPkg:Story src="Stories/Story_u3.xml"/></Document>`,
		"Spreads/Spread_ua.xml":             head + `<idPkg:Spread ` + pkgNS + `><Spread Self="ua" PageCount="1"><Page Self="p1" Name="1"/><TextFrame Self="f2" ParentStory="u2" ContentType="TextType"/><Group Self="g1"><TextFrame Self="f1" ParentStory="u1" ContentType="TextType"/></Group><TextFrame Self="f1b" ParentStory="u1" PreviousTextFrame="f1" ContentType="TextType"/></Spread></idPkg:Spread>`,
		"MasterSpreads/MasterSpread_um.xml": head + `<idPkg:MasterSpread ` + pkgNS + `><MasterSpread Self="um" Name="A-Master"/></idPkg:MasterSpread>`,
		"Stories/Story_u1.xml":              storyFile("u1", main),
		"Stories/Story_u2.xml":              storyFile("u2", psr("Kopf", csr("", text("Kanzlei Weber")))),
		"Stories/Story_u3.xml":              storyFile("u3", psr("Text", csr("", text("Auf keiner Seite")))),
	})
}

// pack zips the files with the package's type first and not compressed,
// as InDesign writes and expects it.
func pack(files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	w.Write([]byte(files["mimetype"]))
	names := make([]string, 0, len(files))
	for n := range files {
		if n != "mimetype" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate})
		w.Write([]byte(files[n]))
	}
	zw.Close()
	return buf.Bytes()
}

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
			b, _ := io.ReadAll(rc)
			return string(b)
		}
	}
	t.Fatalf("no part %s", name)
	return ""
}

func TestExtract(t *testing.T) {
	ex, err := Extract(lease(), "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range ex.Units {
		got = append(got, u.ID+" "+u.Source.String())
	}
	want := []string{
		"Story_u2/p1/s1 Kanzlei Weber",
		"Story_u1/p1/s1 Mietvertrag",
		"Story_u1/p2/s1 Die Kündigung muss {1}schriftlich{/1} erfolgen.",
		"Story_u1/p2/s2 Sie ist an den {1}Vermieter{/1} zu richten.",
		"Story_u1/p3/s1 Zeile eins{1/}Zeile zwei, Seite {2/} von 3",
		"Story_u1/p4/s1 Fristen{1/} im Überblick:",
		"Story_u1/p5/s1 Schluss & Ende",
		"Story_u1/p6/s1 Siehe § 573c BGB.",
		"Story_u1/p7/s1 Frist",
		"Story_u1/p8/s1 Dauer",
		"Story_u1/p9/s1 Kündigung",
		"Story_u1/p10/s1 3 Monate",
		"Story_u3/p1/s1 Auf keiner Seite",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("units:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	blocks := map[string]filter.Block{}
	for _, b := range ex.Blocks {
		blocks[b.ID] = b
	}
	if b := blocks["Story_u1/p1"]; b.Style != "Titel" || b.Part != filter.PartBody {
		t.Errorf("title: %+v", b)
	}
	if b := blocks["Story_u1/p6"]; b.Part != filter.PartFootnote || len(b.Lead) != 1 {
		t.Errorf("footnote: %+v", b)
	}
	if b := blocks["Story_u1/p10"]; b.Cell == nil || *b.Cell != (filter.Cell{Table: 0, Row: 1, Col: 1}) {
		t.Errorf("cell: %+v", b.Cell)
	}
	tags := ex.Units[2].Source.Tags()
	if tags[0].Type != "bold" || tags[0].Desc != "Bold" {
		t.Errorf("bold tag: %+v", tags[0])
	}
	if tags := ex.Units[4].Source.Tags(); tags[0].Type != "lb" || tags[1].Type != "special" {
		t.Errorf("special characters: %+v", tags)
	}
	if tags := ex.Units[5].Source.Tags(); tags[0].Type != "footnote" {
		t.Errorf("footnote reference: %+v", tags)
	}
}

func tag(kind content.TagKind, id string) content.Tag { return content.Tag{Kind: kind, ID: id} }

func TestMerge(t *testing.T) {
	src := lease()
	pair := func(before, inside, after string) content.Content {
		return content.Text(before).AppendTag(tag(content.TagOpen, "1")).Append(inside).AppendTag(tag(content.TagClose, "1")).Append(after)
	}
	out, err := Merge(src, "de-DE", "en-GB", filter.Translations{
		"Story_u1/p2/s1":  pair("Notice must be given ", "in writing", "."),
		"Story_u1/p2/s2":  pair("It is to be addressed to the ", "landlord", "."),
		"Story_u1/p3/s1":  content.Text("Line one").AppendTag(tag(content.TagStandalone, "1")).Append("line two, page ").AppendTag(tag(content.TagStandalone, "2")).Append(" of 3"),
		"Story_u1/p4/s1":  content.Text("Notice periods").AppendTag(tag(content.TagStandalone, "1")).Append(" at a glance:"),
		"Story_u1/p6/s1":  content.Text("See section 573c BGB."),
		"Story_u1/p10/s1": content.Text("3 months"),
		"Story_u1/p5/s1":  content.Text("The <end> & more"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := partOf(t, out, "Stories/Story_u1.xml")
	for _, want := range []string{
		// Untouched: the title's range, byte for byte.
		psr("Titel", csr("", text("Mietvertrag"), `<Br/>`)),
		// Two paragraphs of one range, the bold range and the link in place.
		`<Content>Notice must be given </Content></CharacterStyleRange><CharacterStyleRange ` + none + ` FontStyle="Bold"><Content>in writing</Content></CharacterStyleRange><CharacterStyleRange ` + none + `><Content>. It is to be addressed to the </Content><HyperlinkTextSource Self="Hyperlink_1" Name="Vermieter" Hidden="false" AppliedCharacterStyle="n"><Content>landlord</Content></HyperlinkTextSource><Content>.</Content><Br/>`,
		"<Content>Line one</Content><Content> </Content><Content>line two, page </Content><Content><?ACE 18?></Content><Content> of 3</Content><Br/></CharacterStyleRange></ParagraphStyleRange>",
		// The footnote is translated inside the paragraph that holds it,
		// the note stays, and so does the table but for the one cell.
		`<Content>Notice periods</Content><Footnote><ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Fussnote"><CharacterStyleRange ` + none + `><Content><?ACE 4?></Content><Content> See section 573c BGB.</Content></CharacterStyleRange></ParagraphStyleRange></Footnote><Content> at a glance:</Content><Note Collapsed="false"`,
		`<Content>Noch prüfen</Content>`,
		`<Cell Self="t1i2" Name="0:1">` + psr("Zelle", csr("", text("Kündigung"))) + `</Cell>`,
		`<Cell Self="t1i3" Name="1:1">` + psr("Zelle", csr("", text("3 months"))) + `</Cell>`,
		`<Content>The &lt;end&gt; &amp; more</Content></CharacterStyleRange></ParagraphStyleRange></Story>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the story lacks\n%s", want)
		}
	}
	if t.Failed() {
		t.Log(got)
	}
	for _, name := range []string{"Stories/Story_u2.xml", "Stories/Story_u3.xml", "designmap.xml", "mimetype", "Spreads/Spread_ua.xml"} {
		if partOf(t, out, name) != partOf(t, src, name) {
			t.Errorf("%s changed", name)
		}
	}
	// The package still starts with its type, as InDesign expects.
	zr, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if zr.File[0].Name != "mimetype" || zr.File[0].Method != zip.Store {
		t.Errorf("first entry is %s, method %d", zr.File[0].Name, zr.File[0].Method)
	}
	back, err := Extract(out, "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Units) != 13 || back.Units[2].Source.String() != "Notice must be given {1}in writing{/1}." {
		t.Errorf("read back: %d units, %q", len(back.Units), back.Units[2].Source.String())
	}
}

func TestMergeNothingKeepsEveryStory(t *testing.T) {
	src := lease()
	out, err := Merge(src, "de-DE", "en-GB", nil)
	if err != nil {
		t.Fatal(err)
	}
	if partOf(t, out, "Stories/Story_u1.xml") != partOf(t, src, "Stories/Story_u1.xml") {
		t.Error("a story changed without a translation")
	}
}

func TestOnlyNestedTextTranslated(t *testing.T) {
	// A cell is translated and the paragraph that holds the table is not:
	// that paragraph stays as it was around the new cell.
	src := lease()
	out, err := Merge(src, "de-DE", "en-GB", filter.Translations{"Story_u1/p7/s1": content.Text("Period")})
	if err != nil {
		t.Fatal(err)
	}
	got, was := partOf(t, out, "Stories/Story_u1.xml"), partOf(t, src, "Stories/Story_u1.xml")
	if want := strings.Replace(was, text("Frist"), text("Period"), 1); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRightToLeft(t *testing.T) {
	out, err := Merge(lease(), "de-DE", "ar-SA", filter.Translations{"Story_u2/p1/s1": content.Text("مكتب ويبر")})
	if err != nil {
		t.Fatal(err)
	}
	if got := partOf(t, out, "Stories/Story_u2.xml"); !strings.Contains(got, `<ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Kopf" ParagraphDirection="RightToLeftDirection"><CharacterStyleRange `+none+`><Content>مكتب ويبر</Content>`) {
		t.Errorf("story: %s", got)
	}
}

func TestStructureTagsAreKept(t *testing.T) {
	body := psr("Text", `<XMLElement Self="x1" MarkupTag="XMLTag/Absatz">`+csr("", text("Im Strukturbaum"))+`</XMLElement>`) + psr("Text", csr("", text("Frei")))
	doc := pack(map[string]string{
		"mimetype":             "application/vnd.adobe.indesign-idml-package",
		"designmap.xml":        head + `<Document ` + pkgNS + ` Self="d"><idPkg:Story src="Stories/Story_u1.xml"/></Document>`,
		"Stories/Story_u1.xml": storyFile("u1", body),
	})
	ex, err := Extract(doc, "de")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 1 || ex.Units[0].Source.Plain() != "Frei" || len(ex.Warnings) != 1 {
		t.Errorf("units %d, warnings %q", len(ex.Units), ex.Warnings)
	}
}

func TestNotIDML(t *testing.T) {
	if _, err := Extract([]byte("no"), "de"); !errors.Is(err, ErrNotIDML) {
		t.Errorf("plain bytes: %v", err)
	}
	if _, err := Extract(docxtest.Hofmann(), "de"); !errors.Is(err, ErrNotIDML) {
		t.Errorf("a Word document: %v", err)
	}
}
