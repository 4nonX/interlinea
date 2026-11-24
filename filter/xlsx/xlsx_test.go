package xlsx

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

const head = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
const main = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
const relNS = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
const relType = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`

// book is a workbook with two sheets. The second comes first in the
// workbook's list, shared strings are used by several cells and in an
// order of their own, and there are a number, a formula, a cell with its
// own text, rich text with a line break, a comment and a text box.
func book() []byte {
	return docxtest.Build(map[string]string{
		"[Content_Types].xml":        head + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"_rels/.rels":                head + `<Relationships ` + relNS + `><Relationship Id="rId1" Type="` + relType + `officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            head + `<workbook ` + main + `><sheets><sheet name="Übersicht" sheetId="2" r:id="rId2"/><sheet name="Daten" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": head + `<Relationships ` + relNS + `><Relationship Id="rId1" Type="` + relType + `worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="` + relType + `worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="rId3" Type="` + relType + `sharedStrings" Target="sharedStrings.xml"/></Relationships>`,
		"xl/sharedStrings.xml": head + `<sst ` + main + ` count="7" uniqueCount="6">` +
			`<si><t>Miete</t></si>` +
			`<si><t>Nicht verwendet</t></si>` +
			`<si><r><t xml:space="preserve">Zahlbar bis zum </t></r><r><rPr><b/><sz val="11"/></rPr><t>dritten Werktag</t></r><r><t>. Danach fallen Zinsen an.</t></r></si>` +
			`<si><t>Erste Zeile` + "\n" + `zweite Zeile</t><phoneticPr fontId="1"/></si>` +
			`<si><t>Jahr</t></si>` +
			`<si><t xml:space="preserve">  </t></si>` +
			`</sst>`,
		"xl/worksheets/sheet1.xml": head + `<worksheet ` + main + `><sheetViews><sheetView workbookViewId="0"/></sheetViews><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>4</v></c><c r="B1" t="s"><v>0</v></c></row>` +
			`<row r="2"><c r="A2"><v>2027</v></c><c r="B2"><v>1250</v></c><c r="C2" t="str"><f>A1&amp;"!"</f><v>Jahr!</v></c><c r="D2" t="s"><v>5</v></c></row>` +
			`<row r="3"><c r="A3" t="inlineStr"><is><t>Nur hier</t></is></c><c r="B3" t="s"><v>0</v></c></row>` +
			`</sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": head + `<worksheet ` + main + `><sheetViews><sheetView tabSelected="1" workbookViewId="0"/></sheetViews><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>2</v></c></row>` +
			`<row r="2"><c r="B2" t="s"><v>3</v></c><c r="C2" t="s"><v>0</v></c></row>` +
			`</sheetData><drawing r:id="rId1"/><legacyDrawing r:id="rId3"/></worksheet>`,
		"xl/worksheets/_rels/sheet2.xml.rels": head + `<Relationships ` + relNS + `><Relationship Id="rId1" Type="` + relType + `drawing" Target="../drawings/drawing1.xml"/><Relationship Id="rId2" Type="` + relType + `comments" Target="../comments1.xml"/></Relationships>`,
		"xl/comments1.xml":                    head + `<comments ` + main + `><authors><author>Eva</author></authors><commentList><comment ref="B2" authorId="0"><text><r><rPr><b/></rPr><t>Eva:</t></r><r><t xml:space="preserve"> Bitte prüfen.</t></r></text></comment></commentList></comments>`,
		"xl/drawings/drawing1.xml":            head + `<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><xdr:twoCellAnchor><xdr:sp><xdr:nvSpPr><xdr:cNvPr id="2" name="Textfeld 1"/><xdr:cNvSpPr txBox="1"/></xdr:nvSpPr><xdr:spPr/><xdr:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="de-DE"/><a:t>Stand: November</a:t></a:r></a:p></xdr:txBody></xdr:sp><xdr:clientData/></xdr:twoCellAnchor></xdr:wsDr>`,
	})
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
	ex, err := Extract(book(), "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range ex.Units {
		got = append(got, u.ID+" "+u.Source.String())
	}
	// The sheets in the workbook's order, the cells along the rows; a
	// shared text where it first appears; the comment and the text box
	// after the cells of their sheet. No number, formula, unused or
	// blank string.
	want := []string{
		"sheet2/A1/s1 Zahlbar bis zum {1}dritten Werktag{/1}.",
		"sheet2/A1/s2 Danach fallen Zinsen an.",
		"sheet2/B2/s1 Erste Zeile{1/}zweite Zeile",
		"sheet2/C2/s1 Miete",
		"comments1/B2/s1 {1}Eva:{/1} Bitte prüfen.",
		"drawing1/p1/s1 Stand: November",
		"sheet1/A1/s1 Jahr",
		"sheet1/A3/s1 Nur hier",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("units:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(ex.Blocks) != 7 {
		t.Errorf("%d blocks", len(ex.Blocks))
	}
	for _, b := range ex.Blocks {
		switch b.ID {
		case "sheet2/B2":
			if b.Cell == nil || *b.Cell != (filter.Cell{Table: 0, Row: 1, Col: 1}) {
				t.Errorf("B2 on the first sheet of the workbook: %+v", b.Cell)
			}
		case "sheet1/A3":
			if b.Cell == nil || *b.Cell != (filter.Cell{Table: 1, Row: 2, Col: 0}) {
				t.Errorf("A3: %+v", b.Cell)
			}
		case "comments1/B2":
			if b.Part != filter.PartComment || b.Note != "B2" {
				t.Errorf("comment: %+v", b)
			}
		}
	}
	if tag := ex.Units[0].Source.Tags()[0]; tag.Type != "format" || tag.Desc != "Bold, Size 11 pt" {
		t.Errorf("tag %+v", tag)
	}
}

func bold(s string) content.Content {
	return content.Content{}.AppendTag(content.Tag{Kind: content.TagOpen, ID: "1"}).Append(s).AppendTag(content.Tag{Kind: content.TagClose, ID: "1"})
}

func TestMerge(t *testing.T) {
	src := book()
	out, err := Merge(src, "de-DE", "en-GB", filter.Translations{
		"sheet2/A1/s1":    append(content.Text("Payable by the "), append(bold("third working day"), content.Run{Text: "."})...),
		"sheet2/B2/s1":    content.Text("First line").AppendTag(content.Tag{Kind: content.TagStandalone, ID: "1"}).Append("second line"),
		"sheet2/C2/s1":    content.Text("Rent & more"),
		"comments1/B2/s1": append(bold("Eva:"), content.Run{Text: " Please check."}),
		"drawing1/p1/s1":  content.Text("As of November"),
		"sheet1/A3/s1":    content.Text("Only here"),
	})
	if err != nil {
		t.Fatal(err)
	}
	sst := partOf(t, out, "xl/sharedStrings.xml")
	for _, want := range []string{
		`<si><t>Rent &amp; more</t></si>`,
		`<si><t>Nicht verwendet</t></si>`,
		// The second sentence has no translation and keeps its source.
		`<si><r><t xml:space="preserve">Payable by the </t></r><r><rPr><b/><sz val="11"/></rPr><t>third working day</t></r><r><t>. Danach fallen Zinsen an.</t></r></si>`,
		`<si><t xml:space="preserve">First line` + "\n" + `second line</t></si>`,
		`<si><t>Jahr</t></si>`, // untouched
		`count="7" uniqueCount="6"`,
	} {
		if !strings.Contains(sst, want) {
			t.Errorf("shared strings lack %s:\n%s", want, sst)
		}
	}
	if s := partOf(t, out, "xl/worksheets/sheet1.xml"); !strings.Contains(s, `<c r="A3" t="inlineStr"><is><t>Only here</t></is></c>`) || !strings.Contains(s, `<f>A1&amp;"!"</f>`) {
		t.Errorf("sheet1: %s", s)
	}
	if s := partOf(t, out, "xl/comments1.xml"); !strings.Contains(s, `<text><r><rPr><b/></rPr><t>Eva:</t></r><r><t xml:space="preserve"> Please check.</t></r></text>`) {
		t.Errorf("comments: %s", s)
	}
	if s := partOf(t, out, "xl/drawings/drawing1.xml"); !strings.Contains(s, `<a:rPr lang="en-GB"/><a:t>As of November</a:t>`) {
		t.Errorf("drawing: %s", s)
	}
	if partOf(t, out, "xl/workbook.xml") != partOf(t, src, "xl/workbook.xml") || partOf(t, out, "xl/worksheets/sheet2.xml") != partOf(t, src, "xl/worksheets/sheet2.xml") {
		t.Error("the workbook or a sheet without text of its own changed")
	}
	back, err := Extract(out, "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Units) != 8 || back.Units[3].Source.Plain() != "Rent & more" {
		t.Errorf("read back: %d units, %q", len(back.Units), back.Units[3].Source.Plain())
	}
}

func TestMergeNothingKeepsEveryPart(t *testing.T) {
	src := book()
	out, err := Merge(src, "de-DE", "en-GB", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xl/sharedStrings.xml", "xl/worksheets/sheet1.xml", "xl/comments1.xml"} {
		if partOf(t, out, name) != partOf(t, src, name) {
			t.Errorf("%s changed without a translation", name)
		}
	}
}

func TestRightToLeft(t *testing.T) {
	out, err := Merge(book(), "de-DE", "ar-SA", filter.Translations{"sheet1/A1/s1": content.Text("سنة")})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml"} {
		if s := partOf(t, out, name); !strings.Contains(s, `rightToLeft="1"`) {
			t.Errorf("%s is not turned: %s", name, s)
		}
	}
	back, err := Merge(out, "ar-SA", "de-DE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := partOf(t, back, "xl/worksheets/sheet2.xml"); !strings.Contains(s, `<sheetView tabSelected="1" workbookViewId="0" rightToLeft="0"/>`) {
		t.Errorf("turned back: %s", s)
	}
}

func TestNotAWorkbook(t *testing.T) {
	if _, err := Extract([]byte("no"), "de-DE"); !errors.Is(err, ErrNotWorkbook) {
		t.Errorf("plain bytes: %v", err)
	}
	if _, err := Extract(docxtest.Hofmann(), "de-DE"); !errors.Is(err, ErrNotWorkbook) {
		t.Errorf("a Word document: %v", err)
	}
}
