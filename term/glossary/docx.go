package glossary

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/4nonX/interlinea/langtag"
)

func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			if r < 0x20 && r != '\t' && r != '\n' {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wordRun(r Run, style string) string {
	var p strings.Builder
	if style != "" {
		fmt.Fprintf(&p, `<w:rStyle w:val="%s"/>`, style)
	}
	if r.Bold {
		p.WriteString("<w:b/><w:bCs/>")
	}
	if r.Italic {
		p.WriteString("<w:i/><w:iCs/>")
	}
	if r.Strike {
		p.WriteString("<w:strike/>")
	}
	if r.Grey {
		p.WriteString(`<w:color w:val="6B6B6B"/>`)
	}
	if r.Small {
		p.WriteString(`<w:sz w:val="16"/><w:szCs w:val="16"/>`)
	}
	if r.Lang != "" {
		if langtag.RTL(r.Lang) {
			p.WriteString("<w:rtl/>")
			fmt.Fprintf(&p, `<w:lang w:bidi="%s"/>`, xmlEscape(r.Lang))
		} else if s := langtag.Script(r.Lang); s == "Hani" || s == "Hans" || s == "Hant" || s == "Jpan" || s == "Kore" {
			fmt.Fprintf(&p, `<w:lang w:eastAsia="%s"/>`, xmlEscape(r.Lang))
		} else {
			fmt.Fprintf(&p, `<w:lang w:val="%s"/>`, xmlEscape(r.Lang))
		}
	}
	rpr := ""
	if p.Len() > 0 {
		rpr = "<w:rPr>" + p.String() + "</w:rPr>"
	}
	return fmt.Sprintf(`<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, rpr, xmlEscape(r.Text))
}

func wordPara(b Block, style string, headword bool) string {
	var out strings.Builder
	out.WriteString("<w:p><w:pPr>")
	fmt.Fprintf(&out, `<w:pStyle w:val="%s"/>`, style)
	if b.RTL {
		out.WriteString("<w:bidi/>")
	}
	out.WriteString("</w:pPr>")
	for i, r := range b.Runs {
		st := ""
		// The headword carries the style the running head looks up.
		if headword && i == 0 && r.Bold {
			st = "Headword"
		}
		out.WriteString(wordRun(r, st))
	}
	out.WriteString("</w:p>")
	return out.String()
}

// Word writes the glossary as a Word document: a dictionary in two
// columns, or a table whose header row repeats on every page, with the
// first and last headword of each page in its running head.
func Word(g *Glossary, made time.Time) []byte {
	w, h := 11906, 16838
	if g.Options.Page == "letter" {
		w, h = 12240, 15840
	}
	margin := 1134
	rtl := langtag.RTL(g.Sort)
	var body strings.Builder
	body.WriteString(wordPara(Block{Runs: []Run{{Text: g.Title}}}, "Title", false))
	body.WriteString(wordPara(Block{Runs: []Run{{Text: g.subtitle(made)}}}, "Subtitle", false))
	if l := g.Legend(); l != "" {
		body.WriteString(wordPara(Block{Runs: []Run{{Text: l, Italic: true}}}, "Subtitle", false))
	}
	cols := 2
	if g.Table() {
		cols = 1
	}
	// The title spans the page; the glossary below it takes its columns.
	fmt.Fprintf(&body, `<w:p><w:pPr><w:sectPr><w:type w:val="continuous"/><w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="567" w:footer="567" w:gutter="0"/><w:cols w:space="425"/></w:sectPr></w:pPr></w:p>`,
		w, h, margin+284, margin, margin, margin)
	for _, s := range g.Sections {
		if s.Letter != "" {
			lb := Block{Runs: []Run{{Text: s.Letter}}, RTL: rtl}
			body.WriteString(wordPara(lb, "Letter", false))
		}
		if g.Table() {
			body.WriteString(g.wordTable(s, w-2*margin))
			continue
		}
		for _, a := range s.Articles {
			for _, b := range g.Blocks(a) {
				style := "Entry"
				if b.Indent > 0 {
					style = "EntryMore"
				}
				body.WriteString(wordPara(b, style, b.Head))
			}
		}
	}
	bidi := ""
	if rtl {
		bidi = "<w:bidi/>"
	}
	fmt.Fprintf(&body, `<w:sectPr><w:headerReference w:type="default" r:id="rIdHeader"/><w:footerReference w:type="default" r:id="rIdFooter"/><w:type w:val="continuous"/><w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="567" w:footer="567" w:gutter="0"/><w:cols w:num="%d" w:space="425"/>%s</w:sectPr>`,
		w, h, margin+284, margin, margin, margin, cols, bidi)

	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>` +
		body.String() + `</w:body></w:document>`

	field := func(instr string) string {
		return `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> ` + instr + ` </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t></w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`
	}
	header := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:pStyle w:val="Header"/>` + bidi + `</w:pPr>` +
		wordRun(Run{Text: g.Title}, "") + `<w:r><w:tab/></w:r>` + field(`STYLEREF "Headword"`) + wordRun(Run{Text: " – "}, "") + field(`STYLEREF "Headword" \l`) +
		`</w:p></w:hdr>`
	footer := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:pStyle w:val="Footer"/><w:jc w:val="center"/></w:pPr>` + field("PAGE") + `</w:p></w:ftr>`
	right := w - 2*margin
	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:sz w:val="19"/><w:szCs w:val="19"/><w:lang w:val="` + xmlEscape(g.Sort) + `"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="0" w:line="264" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="40"/></w:pPr><w:rPr><w:b/><w:sz w:val="36"/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="280"/></w:pPr><w:rPr><w:color w:val="6B6B6B"/><w:sz w:val="17"/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Letter"><w:name w:val="Letter"/><w:basedOn w:val="Normal"/><w:next w:val="Entry"/><w:pPr><w:keepNext/><w:spacing w:before="200" w:after="80"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="30"/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Entry"><w:name w:val="Entry"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="80"/><w:ind w:left="240" w:hanging="240"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="EntryMore"><w:name w:val="Entry continued"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="480" w:hanging="240"/></w:pPr></w:style>` +
		`<w:style w:type="character" w:styleId="Headword"><w:name w:val="Headword"/><w:rPr><w:b/><w:bCs/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Header"><w:name w:val="header"/><w:basedOn w:val="Normal"/><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="4" w:space="4" w:color="BFBFBF"/></w:pBdr><w:tabs><w:tab w:val="right" w:pos="` + fmt.Sprint(right) + `"/></w:tabs></w:pPr><w:rPr><w:color w:val="6B6B6B"/><w:sz w:val="17"/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Footer"><w:name w:val="footer"/><w:basedOn w:val="Normal"/><w:rPr><w:color w:val="6B6B6B"/><w:sz w:val="17"/></w:rPr></w:style>` +
		`<w:style w:type="table" w:styleId="Glossary"><w:name w:val="Glossary table"/><w:tblPr><w:tblBorders><w:insideH w:val="single" w:sz="4" w:color="D0D3DA"/><w:bottom w:val="single" w:sz="4" w:color="D0D3DA"/></w:tblBorders><w:tblCellMar><w:top w:w="60" w:type="dxa"/><w:left w:w="80" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>` +
		`</w:styles>`

	parts := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
			`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>` +
			`<Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>` +
			`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>` +
			`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
			`</Relationships>`},
		{"docProps/core.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
			`<dc:title>` + xmlEscape(g.Title) + `</dc:title><dc:creator>Interlinea</dc:creator><dc:language>` + xmlEscape(g.Sort) + `</dc:language>` +
			`<dcterms:created xsi:type="dcterms:W3CDTF">` + made.UTC().Format(time.RFC3339) + `</dcterms:created></cp:coreProperties>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`<Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>` +
			`<Relationship Id="rIdHeader" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>` +
			`<Relationship Id="rIdFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>` +
			`</Relationships>`},
		// Fields such as the running head are worked out when it opens.
		{"word/settings.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:updateFields w:val="true"/></w:settings>`},
		{"word/document.xml", doc},
		{"word/styles.xml", styles},
		{"word/header1.xml", header},
		{"word/footer1.xml", footer},
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, p := range parts {
		f, _ := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: made})
		f.Write([]byte(p.body))
	}
	zw.Close()
	return out.Bytes()
}

func (g *Glossary) wordTable(s Section, width int) string {
	hs := g.Headers()
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="Glossary"/><w:tblW w:w="5000" w:type="pct"/><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	colW := width / len(hs)
	for range hs {
		fmt.Fprintf(&b, `<w:gridCol w:w="%d"/>`, colW)
	}
	b.WriteString(`</w:tblGrid><w:tr><w:trPr><w:tblHeader/><w:cantSplit/></w:trPr>`)
	for i, h := range hs {
		rtl := i < len(g.Languages) && langtag.RTL(g.Languages[i])
		fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="EDEFF4"/></w:tcPr>`, colW)
		b.WriteString(wordPara(Block{Runs: []Run{{Text: h, Bold: true}}, RTL: rtl}, "Normal", false))
		b.WriteString(`</w:tc>`)
	}
	b.WriteString(`</w:tr>`)
	for _, a := range s.Articles {
		for k, e := range a.Entries {
			b.WriteString(`<w:tr>`)
			for c, cell := range g.Cells(e) {
				fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>`, colW)
				if len(cell) == 0 {
					b.WriteString(`<w:p/>`)
				}
				for i, bl := range cell {
					// The headword of the row, for the running head.
					b.WriteString(wordPara(bl, "Normal", c == 0 && i == 0 && k == 0))
				}
				b.WriteString(`</w:tc>`)
			}
			b.WriteString(`</w:tr>`)
		}
	}
	b.WriteString(`</w:tbl><w:p/>`)
	return b.String()
}
