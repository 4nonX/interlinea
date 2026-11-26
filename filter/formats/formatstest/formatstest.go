// Package formatstest builds one small document of every format, for
// tests of whatever takes files in: the server, the apps, the command
// line. The documents say the same things, in German.
package formatstest

import (
	"archive/zip"
	"bytes"

	"github.com/4nonX/interlinea/filter/docx/docxtest"
)

const head = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

func pack(files [][2]string, storeFirst bool) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, f := range files {
		method := zip.Deflate
		if storeFirst && i == 0 {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f[0], Method: method})
		if err != nil {
			panic(err)
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// Deck is a presentation of two slides with notes, a table and a chart.
func Deck() []byte { return docxtest.Deck() }

// Workbook is a workbook with one sheet: a heading row and a row of text
// and a number.
func Workbook() []byte {
	const main = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	const rels = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
	const typ = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`
	return pack([][2]string{
		{"[Content_Types].xml", head + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/></Types>`},
		{"_rels/.rels", head + `<Relationships ` + rels + `><Relationship Id="rId1" Type="` + typ + `officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", head + `<workbook ` + main + `><sheets><sheet name="Fristen" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", head + `<Relationships ` + rels + `><Relationship Id="rId1" Type="` + typ + `worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="` + typ + `sharedStrings" Target="sharedStrings.xml"/></Relationships>`},
		{"xl/sharedStrings.xml", head + `<sst ` + main + ` count="4" uniqueCount="4"><si><t>Frist</t></si><si><t>Dauer in Monaten</t></si><si><t>Kündigung durch den Mieter</t></si><si><t>Die Kaution ist fällig.</t></si></sst>`},
		{"xl/worksheets/sheet1.xml", head + `<worksheet ` + main + `><sheetViews><sheetView workbookViewId="0"/></sheetViews><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row><row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2"><v>3</v></c></row><row r="3"><c r="A3" t="s"><v>3</v></c></row></sheetData></worksheet>`},
	}, false)
}

// Page is a web page with a title, a heading, a paragraph with a link and
// a picture with a description.
func Page() []byte {
	return []byte(`<!DOCTYPE html>
<html lang="de">
<head>
<meta charset="utf-8">
<title>Fristen im Mietrecht</title>
</head>
<body>
<h1>Fristen im Mietrecht</h1>
<p>Die Kaution ist <b>fällig</b>. Näheres steht im <a href="vertrag.html">Mietvertrag</a>.</p>
<p><img src="haus.png" alt="Das Haus in der Lindenstraße"></p>
</body>
</html>
`)
}

// Layout is an InDesign document (IDML) with one story of a title and two
// paragraphs, one with a word in another format.
func Layout() []byte {
	const ns = `xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging" DOMVersion="8.0"`
	const plain = `AppliedCharacterStyle="CharacterStyle/$ID/[No character style]"`
	return pack([][2]string{
		{"mimetype", "application/vnd.adobe.indesign-idml-package"},
		{"designmap.xml", head + `<Document ` + ns + ` Self="d" StoryList="u1"><idPkg:Spread src="Spreads/Spread_ua.xml"/><idPkg:Story src="Stories/Story_u1.xml"/></Document>`},
		{"Spreads/Spread_ua.xml", head + `<idPkg:Spread ` + ns + `><Spread Self="ua" PageCount="1"><TextFrame Self="f1" ParentStory="u1" ContentType="TextType"/></Spread></idPkg:Spread>`},
		{"Stories/Story_u1.xml", head + `<idPkg:Story ` + ns + `><Story Self="u1" AppliedTOCStyle="n" TrackChanges="false" StoryTitle="$ID/" AppliedNamedGrid="n">` +
			`<ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Titel"><CharacterStyleRange ` + plain + `><Content>Fristen im Mietrecht</Content><Br/></CharacterStyleRange></ParagraphStyleRange>` +
			`<ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Text"><CharacterStyleRange ` + plain + `><Content>Die Kaution ist </Content></CharacterStyleRange><CharacterStyleRange ` + plain + ` FontStyle="Bold"><Content>fällig</Content></CharacterStyleRange><CharacterStyleRange ` + plain + `><Content>.</Content><Br/><Content>Näheres steht im Mietvertrag.</Content></CharacterStyleRange></ParagraphStyleRange>` +
			`</Story></idPkg:Story>`},
	}, true)
}

// Handoff is a memoQ handoff package of one document, kept in an archive
// of its own, with a memory and a term base beside it. document is the
// memoQ XLIFF to put in it.
func Handoff(document []byte) []byte {
	return pack([][2]string{
		{"HandoffPackageInfo.xml", `<?xml version="1.0" encoding="utf-8"?>` + "\n" + `<HandoffPackageInfo xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><ProjectName>Mietvertrag Hofmann</ProjectName><UserName>Anna Lindner</UserName></HandoffPackageInfo>` + "\n"},
		{"Documents/Mietvertrag.docx.mqxlz", string(Archive(document))},
		{"TM/Kanzlei Weber.tmx", `<?xml version="1.0" encoding="utf-8"?><tmx version="1.4"><header creationtool="memoQ" creationtoolversion="9" segtype="sentence" o-tmf="memoQ" adminlang="en" srclang="de-DE" datatype="plaintext"/><body/></tmx>`},
		{"TB/Begriffe.csv", "German;English\nKaution;deposit\n"},
	}, false)
}

// Archive is a memoQ XLIFF document in its archive (.mqxlz).
func Archive(document []byte) []byte {
	return pack([][2]string{{"document.mqxliff", string(document)}, {"skeleton.xml", `<?xml version="1.0" encoding="utf-8"?><skeleton/>`}}, false)
}

// Subtitles is a small EBU STL file of open subtitles: two subtitles, the
// first with two rows and a word in italics, the second with an umlaut
// written as EBU STL writes accents.
func Subtitles() []byte {
	gsi := bytes.Repeat([]byte(" "), 1024)
	copy(gsi, "850STL25.01")
	copy(gsi[11:], "00008") // open subtitles, Latin, German
	copy(gsi[16:], "Mietvertrag")
	copy(gsi[224:], "261126261126")
	copy(gsi[238:], "0000200002001")
	copy(gsi[251:], "3823")
	copy(gsi[255:], "10000000000000000")
	copy(gsi[272:], "11DEU")
	out := gsi
	for i, c := range []struct {
		in, out [4]byte
		text    string
	}{
		{[4]byte{0, 0, 1, 0}, [4]byte{0, 0, 4, 0}, "Der \x80Mieter\x81 zahlt\x8Adie Miete."},
		{[4]byte{0, 0, 5, 0}, [4]byte{0, 0, 7, 0}, "Die Kaution betr\xC8agt drei Monatsmieten."},
	} {
		b := make([]byte, 128)
		b[1] = byte(i + 1)
		b[3] = 0xFF
		copy(b[5:], c.in[:])
		copy(b[9:], c.out[:])
		b[13], b[14] = 20, 2
		field := bytes.Repeat([]byte{0x8F}, 112)
		copy(field, c.text)
		copy(b[16:], field)
		out = append(out, b...)
	}
	return out
}

// Captions is a small Scenarist closed caption file (CEA-608): two pop-on
// captions, the first with two rows, the second with an extended
// character (ä after its fallback a), each erased later.
const Captions = "Scenarist_SCC V1.0\n\n" +
	"00:00:01:00\t9420 9420 94ae 94ae 9452 9452 97a2 97a2 c465 f220 cd69 e574 e5f2 207a 61e8 ec74 94f2 94f2 97a2 97a2 6469 e520 cd69 e574 e5ae 942f 942f\n\n" +
	"00:00:04:00\t942c 942c\n\n" +
	"00:00:05:00\t9420 9420 94ae 94ae 94f2 94f2 97a2 97a2 c4e9 e520 cb61 75f4 e9ef 6e20 62e5 f4f2 6180 1331 1331 67f4 ae80 942f 942f\n\n" +
	"00:00:08:00\t942c 942c\n"

// RollUpCaptions is a Scenarist closed caption file with two roll-up lines
// in a window of two rows, as live captions are written, erased later.
const RollUpCaptions = "Scenarist_SCC V1.0\n\n" +
	"00:00:01:00\t9425 9425 94ad 94ad 9470 9470 c775 f4e5 6e20 c162 e56e 642c\n\n" +
	"00:00:03:00\t94ad 94ad 9470 9470 68e9 e5f2 2073 e96e 6420 64e9 e520 ce61 e368 f2e9 e368 f4e5 6eae\n\n" +
	"00:00:06:00\t942c 942c\n"
