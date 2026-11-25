// Package pdf reads the text of PDF files. A PDF says where each piece of
// text sits on a page and nothing of paragraphs, so the paragraphs are
// found from the places: lines from baselines, columns from the white
// between them, paragraphs from spacing, indents and short lines.
//
// The text is handed on as a Word document, which the Word filter cuts
// into units and writes back with the translation. A PDF itself is not
// written: its layout cannot be rebuilt around text of another length.
package pdf

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// The reasons a PDF has no text to give.
var (
	// ErrEncrypted: the file is locked with a password or with rights
	// that do not allow copying its text.
	ErrEncrypted = errors.New("it is protected: save a copy without protection in the program it comes from, or ask its author for one")
	// ErrNoText: the pages are pictures, as from a scanner.
	ErrNoText = errors.New("it has no text, only pictures of its pages: run it through text recognition (OCR) first")
	// ErrUnreadable: the fonts do not say which characters they draw.
	ErrUnreadable = errors.New("its fonts do not say which characters they draw: export it again with its text kept, or run it through text recognition (OCR)")
)

// Document is the text of a PDF.
type Document struct {
	Paragraphs []Paragraph
	Pages      int
	// Width and Height are the size of the first page in points.
	Width, Height float64
}

// Read finds the paragraphs of a PDF's text.
func Read(data []byte) (*Document, error) {
	f, err := open(data)
	if err != nil {
		return nil, err
	}
	if f.trailer["Encrypt"] != nil {
		return nil, ErrEncrypted
	}
	pages := f.pages()
	if len(pages) == 0 {
		return nil, errors.New("it has no pages")
	}
	doc := &Document{Pages: len(pages), Width: 595, Height: 842}
	if box := f.array(inherited(f, pages[0].d, "MediaBox")); len(box) == 4 {
		x0, _ := f.number(box[0])
		y0, _ := f.number(box[1])
		x1, _ := f.number(box[2])
		y1, _ := f.number(box[3])
		w, h := x1-x0, y1-y0
		if pages[0].rotate%180 != 0 {
			w, h = h, w
		}
		if w >= 72 && h >= 72 && w <= 14400 && h <= 14400 {
			doc.Width, doc.Height = w, h
		}
	}
	r := &reader{f: f, byRef: map[int]*font{}}
	for i, p := range pages {
		for _, para := range layout(r.read(p)) {
			para.Page = i + 1
			para.Text = clean(para.Text)
			if para.Text != "" {
				doc.Paragraphs = append(doc.Paragraphs, para)
			}
		}
		if r.ops > maxOps {
			return nil, errors.New("its pages hold more than is read")
		}
	}
	if r.unknown > r.known {
		return nil, ErrUnreadable
	}
	if len(doc.Paragraphs) == 0 {
		return nil, ErrNoText
	}
	return doc, nil
}

func inherited(f *file, d dict, key name) any {
	for i := 0; i < 64 && d != nil; i++ {
		if v, ok := d[key]; ok {
			return v
		}
		d = f.dict(d["Parent"])
	}
	return nil
}

// ligatures are the letters some fonts draw as one sign.
var ligatures = strings.NewReplacer("ﬀ", "ff", "ﬁ", "fi", "ﬂ", "fl", "ﬃ", "ffi", "ﬄ", "ffl", "ﬅ", "st", "ﬆ", "st")

// marks are the accents that stand alone, as the marks they are when a
// letter follows: some programs draw an accented letter as two signs.
var marks = map[rune]rune{'ˆ': 0x302, '˜': 0x303, 'ˇ': 0x30C, '˘': 0x306, '˙': 0x307, '˚': 0x30A, '˝': 0x30B, '¨': 0x308, '´': 0x301, '¸': 0x327, '¯': 0x304}

// clean makes plain text of what the fonts gave: no control characters
// and no characters of glyphs that have none, ligatures as their letters,
// and an accent drawn before its letter as the accented letter.
func clean(s string) string {
	rs := []rune(ligatures.Replace(s))
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if mark, ok := marks[r]; ok {
			j := i + 1
			if j < len(rs) && rs[j] == ' ' {
				j++
			}
			if j < len(rs) && unicode.IsLetter(rs[j]) {
				if composed := []rune(norm.NFC.String(string([]rune{rs[j], mark}))); len(composed) == 1 {
					b.WriteRune(composed[0])
					i = j
					continue
				}
			}
		}
		switch {
		case r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7F || r == 0xFFFD || r == 0xFFFE || r == 0xFFFF || (r >= 0xD800 && r <= 0xDFFF) || (r >= 0xE000 && r <= 0xF8FF):
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// ToWord reads a PDF and writes its text as a Word document: a paragraph
// for each paragraph found, in the size of its font, bold or italic where
// all of it is, and each page starting a page. The same PDF gives the
// same document, byte for byte.
func ToWord(data []byte) ([]byte, error) {
	doc, err := Read(data)
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	page := 1
	for _, p := range doc.Paragraphs {
		body.WriteString("<w:p>")
		var ppr, rpr strings.Builder
		if p.Page != page {
			ppr.WriteString("<w:pageBreakBefore/>")
			page = p.Page
		}
		if p.Bold {
			rpr.WriteString("<w:b/>")
		}
		if p.Italic {
			rpr.WriteString("<w:i/>")
		}
		if half := int(p.Size*2 + 0.5); half >= 8 && half <= 400 {
			fmt.Fprintf(&rpr, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, half, half)
		}
		if ppr.Len() > 0 {
			body.WriteString("<w:pPr>" + ppr.String() + "</w:pPr>")
		}
		body.WriteString("<w:r>")
		if rpr.Len() > 0 {
			body.WriteString("<w:rPr>" + rpr.String() + "</w:rPr>")
		}
		body.WriteString(`<w:t xml:space="preserve">` + escape(p.Text) + "</w:t></w:r></w:p>")
	}
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() +
		fmt.Sprintf(`<w:sectPr><w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="1417" w:right="1417" w:bottom="1134" w:left="1417" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr>`,
			int(doc.Width*20+0.5), int(doc.Height*20+0.5)) +
		`</w:body></w:document>`
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`},
		{"word/document.xml", document},
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, p := range parts {
		// Stored as it is, so the bytes do not depend on a compressor.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
