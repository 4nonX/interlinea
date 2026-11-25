package typeset

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

// PDF writes the document as a PDF file, with the fonts it uses embedded
// as subsets.
func (d *Document) PDF(created time.Time) []byte {
	d.finish()
	if len(d.pages) == 0 {
		d.pages = append(d.pages, &page{})
	}
	w := &pdfWriter{}
	w.buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")

	// Object numbers: catalog 1, pages 2, info 3, then the rest.
	w.next = 4
	faces := d.usedFaces()
	fontObj := map[*Face]int{}
	for _, f := range faces {
		fontObj[f] = w.reserve()
	}
	pageObjs := make([]int, len(d.pages))
	for i := range d.pages {
		pageObjs[i] = w.reserve()
	}
	outlineObj := 0
	var entryObjs []int
	if len(d.outline) > 0 {
		outlineObj = w.reserve()
		for range d.outline {
			entryObjs = append(entryObjs, w.reserve())
		}
	}

	// Catalog, page tree and properties.
	cat := "<< /Type /Catalog /Pages 2 0 R"
	if outlineObj > 0 {
		cat += fmt.Sprintf(" /Outlines %d 0 R /PageMode /UseOutlines", outlineObj)
	}
	if d.Lang != "" {
		cat += " /Lang " + pdfText(d.Lang)
	}
	cat += " /ViewerPreferences << /DisplayDocTitle true >> >>"
	w.object(1, cat)
	kids := make([]string, len(pageObjs))
	for i, o := range pageObjs {
		kids[i] = fmt.Sprintf("%d 0 R", o)
	}
	w.object(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageObjs)))
	date := created.UTC().Format("D:20060102150405Z")
	w.object(3, fmt.Sprintf("<< /Title %s /Producer (Interlinea) /Creator (Interlinea) /CreationDate (%s) /ModDate (%s) >>", pdfText(d.Title), date, date))

	// Pages.
	fontNames := map[*Face]string{}
	var resources []string
	for i, f := range faces {
		name := fmt.Sprintf("F%d", i+1)
		fontNames[f] = name
		resources = append(resources, fmt.Sprintf("/%s %d 0 R", name, fontObj[f]))
	}
	res := fmt.Sprintf("<< /Font << %s >> >>", strings.Join(resources, " "))
	for i, pg := range d.pages {
		content := w.stream(pageContent(pg, fontNames), "")
		w.object(pageObjs[i], fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources %s /Contents %d 0 R >>",
			num(d.Width), num(d.Height), res, content))
	}

	// Outline: one entry a bookmark (the letters of a glossary).
	if outlineObj > 0 {
		w.object(outlineObj, fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>", entryObjs[0], entryObjs[len(entryObjs)-1], len(entryObjs)))
		for i, e := range d.outline {
			s := fmt.Sprintf("<< /Title %s /Parent %d 0 R /Dest [%d 0 R /XYZ null %s null]", pdfText(e.title), outlineObj, pageObjs[e.page], num(e.y+12))
			if i > 0 {
				s += fmt.Sprintf(" /Prev %d 0 R", entryObjs[i-1])
			}
			if i < len(entryObjs)-1 {
				s += fmt.Sprintf(" /Next %d 0 R", entryObjs[i+1])
			}
			w.object(entryObjs[i], s+" >>")
		}
	}

	// Fonts.
	for _, f := range faces {
		w.font(f, fontObj[f])
	}
	return w.finish()
}

func (d *Document) usedFaces() []*Face {
	seen := map[*Face]bool{}
	var out []*Face
	for _, pg := range d.pages {
		for _, r := range pg.runs {
			if !seen[r.face] {
				seen[r.face] = true
				out = append(out, r.face)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func num(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

func colorOp(c [3]float64, op string) string {
	return fmt.Sprintf("%s %s %s %s", num(c[0]), num(c[1]), num(c[2]), op)
}

func pageContent(pg *page, names map[*Face]string) []byte {
	var b bytes.Buffer
	for _, r := range pg.rects {
		fmt.Fprintf(&b, "%s %s %s %s %s re f\n", colorOp(r.color, "rg"), num(r.x), num(r.y), num(r.w), num(r.h))
	}
	for _, r := range pg.rules {
		fmt.Fprintf(&b, "%s %s w %s %s m %s %s l S\n", colorOp(r.color, "RG"), num(r.width), num(r.x1), num(r.y1), num(r.x2), num(r.y2))
	}
	for _, r := range pg.runs {
		fmt.Fprintf(&b, "BT /%s %s Tf %s 1 0 0 1 %s %s Tm <", names[r.face], num(r.size), colorOp(r.color, "rg"), num(r.x), num(r.y))
		for _, g := range r.glyphs {
			fmt.Fprintf(&b, "%04X", r.face.Font.CID(g))
		}
		b.WriteString("> Tj ET\n")
	}
	return b.Bytes()
}

// pdfText writes a text string: plain if it is ASCII, else UTF-16.
func pdfText(s string) string {
	ascii := true
	for _, r := range s {
		if r > 126 || r < 32 {
			ascii = false
			break
		}
	}
	if ascii {
		r := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
		return "(" + r.Replace(s) + ")"
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteString(">")
	return b.String()
}

type pdfWriter struct {
	buf     bytes.Buffer
	offsets map[int]int
	next    int
}

func (w *pdfWriter) reserve() int {
	n := w.next
	w.next++
	return n
}

func (w *pdfWriter) object(n int, body string) {
	if w.offsets == nil {
		w.offsets = map[int]int{}
	}
	w.offsets[n] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n%s\nendobj\n", n, body)
}

// stream writes a compressed stream as a new object.
func (w *pdfWriter) stream(data []byte, dict string) int {
	n := w.reserve()
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	zw.Write(data)
	zw.Close()
	if w.offsets == nil {
		w.offsets = map[int]int{}
	}
	w.offsets[n] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n<< /Length %d /Filter /FlateDecode%s >>\nstream\n", n, z.Len(), dict)
	w.buf.Write(z.Bytes())
	w.buf.WriteString("\nendstream\nendobj\n")
	return n
}

func (w *pdfWriter) font(f *Face, obj int) {
	ft := f.Font
	scale := 1000 / float64(ft.UnitsPerEm)
	glyphs := make([]uint16, 0, len(f.used))
	for g := range f.used {
		glyphs = append(glyphs, g)
	}
	sort.Slice(glyphs, func(i, j int) bool { return ft.CID(glyphs[i]) < ft.CID(glyphs[j]) })

	// A tag of six capitals names the subset, as PDF asks.
	h := sha256.New()
	fmt.Fprint(h, ft.Name, glyphs)
	sum := h.Sum(nil)
	tag := make([]byte, 6)
	for i := range tag {
		tag[i] = 'A' + sum[i]%26
	}
	base := string(tag) + "+" + ft.Name

	program := ft.Subset(f.used)
	var file string
	if ft.CFF {
		file = fmt.Sprintf("/FontFile3 %d 0 R", w.stream(program, " /Subtype /CIDFontType0C"))
	} else {
		file = fmt.Sprintf("/FontFile2 %d 0 R", w.stream(program, fmt.Sprintf(" /Length1 %d", len(program))))
	}
	flags := 4 // symbolic: the glyphs are not of a standard set
	if ft.Fixed {
		flags |= 1
	}
	if ft.Italic {
		flags |= 64
	}
	stemV := 80
	if ft.Bold {
		stemV = 140
	}
	desc := w.reserve()
	w.object(desc, fmt.Sprintf("<< /Type /FontDescriptor /FontName /%s /Flags %d /FontBBox [%d %d %d %d] /ItalicAngle %s /Ascent %d /Descent %d /CapHeight %d /StemV %d %s >>",
		base, flags, int(float64(ft.XMin)*scale), int(float64(ft.YMin)*scale), int(float64(ft.XMax)*scale), int(float64(ft.YMax)*scale),
		num(ft.ItalicAngle), int(float64(ft.Ascent)*scale), int(float64(ft.Descent)*scale), int(float64(ft.CapHeight)*scale), stemV, file))

	var widths strings.Builder
	for _, g := range glyphs {
		fmt.Fprintf(&widths, "%d [%d] ", ft.CID(g), int(float64(ft.Advance(g))*scale+0.5))
	}
	sub := "CIDFontType2"
	extra := " /CIDToGIDMap /Identity"
	if ft.CFF {
		sub, extra = "CIDFontType0", ""
	}
	cid := w.reserve()
	w.object(cid, fmt.Sprintf("<< /Type /Font /Subtype /%s /BaseFont /%s /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor %d 0 R /W [%s]%s >>",
		sub, base, desc, strings.TrimSpace(widths.String()), extra))

	// ToUnicode: what each glyph stands for, so text can be copied.
	var cm strings.Builder
	cm.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for i := 0; i < len(glyphs); i += 100 {
		chunk := glyphs[i:min(i+100, len(glyphs))]
		fmt.Fprintf(&cm, "%d beginbfchar\n", len(chunk))
		for _, g := range chunk {
			fmt.Fprintf(&cm, "<%04X> <", ft.CID(g))
			for _, u := range utf16.Encode([]rune(f.text[g])) {
				fmt.Fprintf(&cm, "%04X", u)
			}
			cm.WriteString(">\n")
		}
		cm.WriteString("endbfchar\n")
	}
	cm.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	tu := w.stream([]byte(cm.String()), "")
	w.object(obj, fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H /DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>", base, cid, tu))
}

func (w *pdfWriter) finish() []byte {
	xref := w.buf.Len()
	fmt.Fprintf(&w.buf, "xref\n0 %d\n0000000000 65535 f \n", w.next)
	for n := 1; n < w.next; n++ {
		if off, ok := w.offsets[n]; ok {
			fmt.Fprintf(&w.buf, "%010d 00000 n \n", off)
		} else {
			w.buf.WriteString("0000000000 65535 f \n")
		}
	}
	id := sha256.Sum256(w.buf.Bytes())
	fmt.Fprintf(&w.buf, "trailer\n<< /Size %d /Root 1 0 R /Info 3 0 R /ID [<%X> <%X>] >>\nstartxref\n%d\n%%%%EOF\n", w.next, id[:16], id[:16], xref)
	return w.buf.Bytes()
}
