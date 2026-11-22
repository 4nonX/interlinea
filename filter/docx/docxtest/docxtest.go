// Package docxtest builds small, valid Word documents for tests and samples.
package docxtest

import (
	"archive/zip"
	"bytes"
	"sort"
	"strings"
	"time"
)

const ns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

// Build zips the given files in a fixed order with a fixed time, so the
// same input always gives the same bytes. [Content_Types].xml comes first.
func Build(files map[string]string) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "[Content_Types].xml" {
			return true
		}
		if names[j] == "[Content_Types].xml" {
			return false
		}
		return names[i] < names[j]
	})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mod := time.Date(2026, 11, 16, 8, 30, 0, 0, time.UTC)
	for _, n := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: mod})
		if err != nil {
			panic(err)
		}
		if _, err := w.Write([]byte(files[n])); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// Parts describes a document to build. Body is the content of w:body
// without the section properties.
type Parts struct {
	Body      string
	Footnotes []string // paragraphs of each footnote, numbered from 1
	Header    string   // paragraphs; empty for none
	Footer    string
	Links     map[string]string // relationship ID to external URL
	Lang      string            // document default language
}

// Document builds a .docx from parts with a small style sheet: Normal,
// Title, Heading1, Heading2, FootnoteText, FootnoteReference, Hyperlink,
// Header, Footer and TableGrid.
func Document(p Parts) []byte {
	lang := p.Lang
	if lang == "" {
		lang = "en-GB"
	}
	files := map[string]string{
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/styles.xml": styles(lang),
		"word/settings.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings ` + ns + `><w:defaultTabStop w:val="708"/><w:footnotePr><w:footnote w:id="-1"/><w:footnote w:id="0"/></w:footnotePr><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat></w:settings>`,
	}
	ct := []string{
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`,
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>`,
		`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>`,
	}
	rels := []string{
		`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`,
		`<Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>`,
	}
	sect := `<w:sectPr>`
	if p.Header != "" {
		files["word/header1.xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr ` + ns + `>` + p.Header + `</w:hdr>`
		ct = append(ct, `<Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>`)
		rels = append(rels, `<Relationship Id="rIdHeader1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>`)
		sect += `<w:headerReference w:type="default" r:id="rIdHeader1"/>`
	}
	if p.Footer != "" {
		files["word/footer1.xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr ` + ns + `>` + p.Footer + `</w:ftr>`
		ct = append(ct, `<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>`)
		rels = append(rels, `<Relationship Id="rIdFooter1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>`)
		sect += `<w:footerReference w:type="default" r:id="rIdFooter1"/>`
	}
	sect += `<w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1417" w:right="1417" w:bottom="1134" w:left="1417" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr>`
	if len(p.Footnotes) > 0 {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:footnotes ` + ns + `>`)
		b.WriteString(`<w:footnote w:type="separator" w:id="-1"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:separator/></w:r></w:p></w:footnote>`)
		b.WriteString(`<w:footnote w:type="continuationSeparator" w:id="0"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:continuationSeparator/></w:r></w:p></w:footnote>`)
		for i, f := range p.Footnotes {
			id := itoa(i + 1)
			b.WriteString(`<w:footnote w:id="` + id + `">` + f + `</w:footnote>`)
		}
		b.WriteString(`</w:footnotes>`)
		files["word/footnotes.xml"] = b.String()
		ct = append(ct, `<Override PartName="/word/footnotes.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footnotes+xml"/>`)
		rels = append(rels, `<Relationship Id="rIdFootnotes" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footnotes" Target="footnotes.xml"/>`)
	}
	linkIDs := make([]string, 0, len(p.Links))
	for id := range p.Links {
		linkIDs = append(linkIDs, id)
	}
	sort.Strings(linkIDs)
	for _, id := range linkIDs {
		rels = append(rels, `<Relationship Id="`+id+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="`+p.Links[id]+`" TargetMode="External"/>`)
	}
	files["word/document.xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document ` + ns + `><w:body>` + p.Body + sect + `</w:body></w:document>`
	files["word/_rels/document.xml.rels"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(rels, "") + `</Relationships>`
	files["[Content_Types].xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>` + strings.Join(ct, "") + `</Types>`
	return Build(files)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func styles(lang string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles ` + ns + `>
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Calibri" w:cs="Calibri"/><w:sz w:val="22"/><w:szCs w:val="22"/><w:lang w:val="` + lang + `" w:eastAsia="en-US" w:bidi="ar-SA"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>
<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:spacing w:after="240"/><w:jc w:val="center"/></w:pPr><w:rPr><w:b/><w:sz w:val="36"/><w:szCs w:val="36"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="80"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="26"/><w:szCs w:val="26"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="160" w:after="40"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/><w:sz w:val="24"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="FootnoteText"><w:name w:val="footnote text"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:rPr><w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Header"><w:name w:val="header"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0"/><w:jc w:val="right"/></w:pPr><w:rPr><w:color w:val="666666"/><w:sz w:val="18"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Footer"><w:name w:val="footer"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0"/><w:jc w:val="center"/></w:pPr><w:rPr><w:sz w:val="18"/></w:rPr></w:style>
<w:style w:type="character" w:default="1" w:styleId="DefaultParagraphFont"><w:name w:val="Default Paragraph Font"/><w:uiPriority w:val="1"/><w:semiHidden/></w:style>
<w:style w:type="character" w:styleId="FootnoteReference"><w:name w:val="footnote reference"/><w:rPr><w:vertAlign w:val="superscript"/></w:rPr></w:style>
<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="0563C1"/><w:u w:val="single"/></w:rPr></w:style>
<w:style w:type="table" w:default="1" w:styleId="TableNormal"><w:name w:val="Normal Table"/><w:tblPr><w:tblInd w:w="0" w:type="dxa"/><w:tblCellMar><w:top w:w="0" w:type="dxa"/><w:left w:w="108" w:type="dxa"/><w:bottom w:w="0" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>
<w:style w:type="table" w:styleId="TableGrid"><w:name w:val="Table Grid"/><w:basedOn w:val="TableNormal"/><w:tblPr><w:tblBorders><w:top w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:left w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:bottom w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:right w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:insideH w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:insideV w:val="single" w:sz="4" w:space="0" w:color="auto"/></w:tblBorders></w:tblPr></w:style>
</w:styles>`
}

// Helpers for writing body XML by hand.

// Esc escapes text for XML.
func Esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// P is a paragraph with an optional style and alignment.
func P(style string, runs ...string) string {
	ppr := ""
	if style != "" {
		ppr = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + ppr + strings.Join(runs, "") + `</w:p>`
}

// R is a plain text run; RP a run with run properties.
func R(text string) string { return RP("", text) }

func RP(rpr, text string) string {
	if rpr != "" {
		rpr = `<w:rPr>` + rpr + `</w:rPr>`
	}
	return `<w:r>` + rpr + `<w:t xml:space="preserve">` + Esc(text) + `</w:t></w:r>`
}

// B and I are bold and italic runs.
func B(text string) string { return RP(`<w:b/><w:bCs/>`, text) }
func I(text string) string { return RP(`<w:i/><w:iCs/>`, text) }

// Tab is a tab character run.
func Tab() string { return `<w:r><w:tab/></w:r>` }

// FootnoteRef is the reference mark for footnote id in the body.
func FootnoteRef(id int) string {
	return `<w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr><w:footnoteReference w:id="` + itoa(id) + `"/></w:r>`
}

// Footnote is the text of a footnote with its number mark.
func Footnote(text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="FootnoteText"/></w:pPr><w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr><w:footnoteRef/></w:r>` + R(" "+text) + `</w:p>`
}

// Link is an external hyperlink around a run.
func Link(rid, text string) string {
	return `<w:hyperlink r:id="` + rid + `" w:history="1">` + RP(`<w:rStyle w:val="Hyperlink"/>`, text) + `</w:hyperlink>`
}

// Field is a complex field (code and current result), e.g. PAGE.
func Field(code, result string) string {
	return `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> ` + code + ` </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>` +
		R(result) + `<w:r><w:fldChar w:fldCharType="end"/></w:r>`
}

// Table builds a bordered table; each cell is a list of paragraphs.
func Table(widths []int, rows ...[]string) string {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="0" w:type="auto"/><w:tblLook w:val="04A0"/></w:tblPr><w:tblGrid>`)
	for _, w := range widths {
		b.WriteString(`<w:gridCol w:w="` + itoa(w) + `"/>`)
	}
	b.WriteString(`</w:tblGrid>`)
	for _, row := range rows {
		b.WriteString(`<w:tr>`)
		for i, cell := range row {
			b.WriteString(`<w:tc><w:tcPr><w:tcW w:w="` + itoa(widths[i]) + `" w:type="dxa"/></w:tcPr>` + cell + `</w:tc>`)
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	return b.String()
}

// Right aligns a paragraph to the right.
func Right(runs ...string) string {
	return `<w:p><w:pPr><w:jc w:val="right"/></w:pPr>` + strings.Join(runs, "") + `</w:p>`
}
