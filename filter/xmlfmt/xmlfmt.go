// Package xmlfmt reads XML that keeps text: any XML document, and
// OpenDocument text, spreadsheets and presentations (.odt, .ods, .odp).
//
// In any XML document, an element that holds text itself is a paragraph:
// what it holds is translated as one piece, cut into sentences, and the
// elements inside it are tags. An element that only holds other elements
// is looked into. An element with translate="no" (as ITS has it) is left
// alone with everything in it. Attributes are not translated.
//
// In OpenDocument the paragraphs are the format's own (text:p, text:h),
// with what is anchored in one of them (a footnote, a text box) read as
// paragraphs of its own.
//
// Everything outside the paragraphs stays byte for byte.
package xmlfmt

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/para"
	"github.com/4nonX/interlinea/internal/unzip"
	"github.com/4nonX/interlinea/segment"
)

// The formats, by the names stored with a project's files.
const (
	XML          = "xml"
	Text         = "odt"
	Spreadsheet  = "ods"
	Presentation = "odp"
)

// Is reports whether the format is one of this package's.
func Is(format string) bool {
	return format == XML || format == Text || format == Spreadsheet || format == Presentation
}

// node is an element of the document, or a piece of it between elements.
type node struct {
	// name is the element's name as written (text:p); empty for text, a
	// comment or an instruction.
	name string
	// start and end are the node's bytes; inner and innerEnd those of what
	// an element holds.
	start, end      int
	inner, innerEnd int
	text            string // for character data, unescaped
	isText          bool
	attrs           []xml.Attr
	kids            []*node
}

// mode says what a format takes for a paragraph and what it leaves alone.
type mode struct {
	// block reports whether an element is a paragraph by its name; nil
	// where an element is one by holding text.
	block func(name string) bool
	skip  func(n *node, up []*node) bool
}

type edit struct {
	start, end int
	para       para.Para
	// nested are the pieces of the paragraph read as paragraphs of their
	// own, by the mark their tag carries.
	nested map[string]*node
}

type doc struct {
	src   string
	seg   *segment.Segmenter
	mode  mode
	ext   *filter.Extraction
	edits []*edit
	file  string
	part  filter.Part
	n     *int
}

const nestedMark = "\x00nested:"

func parseTree(src string) (*node, error) {
	d := xml.NewDecoder(strings.NewReader(src))
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	root := &node{name: "#document", end: len(src), innerEnd: len(src)}
	stack := []*node{root}
	for {
		before := int(d.InputOffset())
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the file is not well-formed XML: %v", err)
		}
		after := int(d.InputOffset())
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: qname(t.Name), start: before, inner: after, attrs: append([]xml.Attr(nil), t.Attr...)}
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) < 2 || top.name != qname(t.Name) {
				return nil, fmt.Errorf("the file is not well-formed XML: </%s> closes <%s>", qname(t.Name), top.name)
			}
			// An element written <a/> ends where it starts.
			top.innerEnd, top.end = before, after
			stack = stack[:len(stack)-1]
		case xml.CharData:
			top.kids = append(top.kids, &node{start: before, end: after, text: string(t), isText: true})
		default:
			top.kids = append(top.kids, &node{start: before, end: after})
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("the file is not well-formed XML: <%s> is not closed", stack[len(stack)-1].name)
	}
	return root, nil
}

func qname(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func local(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		if qname(a.Name) == name {
			return a.Value, true
		}
	}
	return "", false
}

func (n *node) hasText() bool {
	for _, k := range n.kids {
		if k.isText && strings.TrimSpace(k.text) != "" {
			return true
		}
	}
	return false
}

func (d *doc) isBlock(n *node) bool {
	if n.name == "" || n.isText {
		return false
	}
	if d.mode.block != nil {
		return d.mode.block(n.name)
	}
	return n.hasText()
}

// holdsBlock reports whether a paragraph is somewhere inside the node.
func (d *doc) holdsBlock(n *node, up []*node) bool {
	if n.name == "" || d.mode.skip(n, up) {
		return false
	}
	if d.isBlock(n) {
		return true
	}
	for _, k := range n.kids {
		if d.holdsBlock(k, append(up, n)) {
			return true
		}
	}
	return false
}

// walk finds the paragraphs under a node.
func (d *doc) walk(n *node, up []*node) {
	for _, k := range n.kids {
		if k.name == "" || d.mode.skip(k, append(up, n)) {
			continue
		}
		if d.isBlock(k) {
			d.block(k, append(up, n))
		} else {
			d.walk(k, append(up, n))
		}
	}
}

// block reads a paragraph: its text and, as tags, the elements in it.
func (d *doc) block(n *node, up []*node) {
	e := &edit{start: n.inner, end: n.innerEnd, nested: map[string]*node{}}
	var c content.Content
	id := 0
	var inside []*node
	var add func(k *node, up []*node)
	add = func(k *node, up []*node) {
		switch {
		case k.isText:
			c = c.Append(k.text)
		case k.name == "":
			id++
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(id), Type: "comment", Native: d.src[k.start:k.end]})
		case d.mode.skip(k, up) || (d.mode.block != nil && d.holdsBlock(k, up)):
			// A piece with paragraphs of its own, or one to leave alone:
			// one tag here.
			id++
			mark := nestedMark + strconv.Itoa(id)
			e.nested[mark] = k
			inside = append(inside, k)
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(id), Type: local(k.name), Native: mark})
		case k.inner == k.end || len(k.kids) == 0:
			id++
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(id), Type: local(k.name), Native: d.src[k.start:k.end]})
		default:
			id++
			mine := strconv.Itoa(id)
			c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: mine, Type: local(k.name), Native: d.src[k.start:k.inner]})
			for _, kk := range k.kids {
				add(kk, append(up, k))
			}
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: mine, Type: local(k.name), Native: d.src[k.innerEnd:k.end]})
		}
	}
	for _, k := range n.kids {
		add(k, append(up, n))
	}
	if p, ok := para.Split(d.seg, c); ok {
		*d.n++
		e.para = p
		block := filter.Block{ID: "b" + strconv.Itoa(*d.n), Part: d.part, File: d.file}
		if local(n.name) == "h" {
			block.Heading = 1
			if l, ok := n.attr("text:outline-level"); ok {
				if v, err := strconv.Atoi(l); err == nil && v > 0 {
					block.Heading = v
				}
			}
		}
		e.para.Add(d.ext, block)
		d.edits = append(d.edits, e)
	}
	// What is anchored in the paragraph comes after it.
	for _, k := range inside {
		if d.mode.skip(k, append(up, n)) {
			continue
		}
		if d.isBlock(k) {
			d.block(k, append(up, n))
		} else {
			d.walk(k, append(up, n))
		}
	}
}

// render writes the document from one place to another with the
// paragraphs between them translated.
func (d *doc) render(out *strings.Builder, tr filter.Translations, from, to int) {
	at := from
	for _, e := range d.edits {
		if e.start < at || e.end > to {
			continue
		}
		out.WriteString(d.src[at:e.start])
		at = e.end
		// A paragraph nothing of which is translated stays byte for byte.
		if len(e.nested) == 0 && !e.para.Translated(d.ext, tr) {
			out.WriteString(d.src[e.start:e.end])
			continue
		}
		for _, r := range e.para.Build(d.ext, tr) {
			switch {
			case r.Tag == nil:
				out.WriteString(escape(r.Text))
			case strings.HasPrefix(r.Tag.Native, nestedMark):
				if k := e.nested[r.Tag.Native]; k != nil {
					d.render(out, tr, k.start, k.end)
				}
			default:
				out.WriteString(r.Tag.Native)
			}
		}
		at = e.end
	}
	out.WriteString(d.src[at:to])
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func read(src, lang string, m mode, ext *filter.Extraction, file string, part filter.Part, n *int) (*doc, error) {
	root, err := parseTree(src)
	if err != nil {
		return nil, err
	}
	d := &doc{src: src, seg: segment.New(nil, lang), mode: m, ext: ext, file: file, part: part, n: n}
	d.walk(root, nil)
	return d, nil
}

var anyXML = mode{skip: func(n *node, _ []*node) bool {
	for _, name := range []string{"translate", "its:translate"} {
		if v, ok := n.attr(name); ok && v == "no" {
			return true
		}
	}
	return false
}}

var openDocument = mode{
	block: func(name string) bool { return name == "text:p" || name == "text:h" },
	skip: func(n *node, up []*node) bool {
		switch n.name {
		case "text:tracked-changes", "office:scripts", "office:font-face-decls", "office:settings", "text:sequence-decls", "office:binary-data", "table:tracked-changes":
			return true
		case "table:table-cell":
			// A cell with a number, a date or a formula shows its value as
			// text; only a cell of text is text.
			if v, ok := n.attr("office:value-type"); ok && v != "string" {
				return true
			}
			if _, ok := n.attr("table:formula"); ok {
				return true
			}
		}
		return false
	},
}

// decode reads XML as UTF-8, or UTF-16 with a byte order mark.
func decode(data []byte) (string, func(string) []byte, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		if !utf8.Valid(data[3:]) {
			return "", nil, fmt.Errorf("the file is not valid UTF-8")
		}
		return string(data[3:]), func(s string) []byte { return append([]byte{0xEF, 0xBB, 0xBF}, s...) }, nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		body := data[2:]
		if len(body)%2 != 0 {
			return "", nil, fmt.Errorf("the UTF-16 text has an odd number of bytes")
		}
		u := make([]uint16, len(body)/2)
		for i := range u {
			if le {
				u[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
			} else {
				u[i] = uint16(body[2*i])<<8 | uint16(body[2*i+1])
			}
		}
		return string(utf16.Decode(u)), func(s string) []byte {
			out := append([]byte{}, data[:2]...)
			for _, x := range utf16.Encode([]rune(s)) {
				if le {
					out = append(out, byte(x), byte(x>>8))
				} else {
					out = append(out, byte(x>>8), byte(x))
				}
			}
			return out
		}, nil
	}
	if !utf8.Valid(data) {
		return "", nil, fmt.Errorf("the file is not UTF-8 or UTF-16; XML in another encoding is not read")
	}
	return string(data), func(s string) []byte { return []byte(s) }, nil
}

// The parts of an OpenDocument file that hold text, and what they are.
var odfParts = []struct {
	name string
	part filter.Part
}{{"content.xml", filter.PartBody}, {"styles.xml", filter.PartHeader}}

type parsed struct {
	ext  *filter.Extraction
	docs map[string]*doc
	enc  func(string) []byte
	zip  *zip.Reader
}

func parse(format string, data []byte, lang string) (*parsed, error) {
	p := &parsed{ext: &filter.Extraction{Units: []filter.Unit{}, Blocks: []filter.Block{}, Format: format, SourceLang: lang}, docs: map[string]*doc{}}
	n := 0
	if format == XML {
		src, enc, err := decode(data)
		if err != nil {
			return nil, err
		}
		d, err := read(src, lang, anyXML, p.ext, "", filter.PartBody, &n)
		if err != nil {
			return nil, err
		}
		p.docs[""], p.enc = d, enc
		return p, nil
	}
	zr, err := unzip.Open(data)
	if err != nil {
		return nil, fmt.Errorf("the file is not an OpenDocument file: %v", err)
	}
	p.zip = zr
	found := false
	for _, part := range odfParts {
		f := entry(zr, part.name)
		if f == nil {
			continue
		}
		raw, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		src, _, err := decode(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", part.name, err)
		}
		d, err := read(src, lang, openDocument, p.ext, part.name, part.part, &n)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", part.name, err)
		}
		p.docs[part.name] = d
		found = found || part.name == "content.xml"
	}
	if !found {
		return nil, fmt.Errorf("the file is not an OpenDocument file: it has no content.xml")
	}
	return p, nil
}

func entry(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// maxEntry is the most a part of an OpenDocument file may unpack to.
const maxEntry = 512 << 20

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxEntry+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxEntry {
		return nil, fmt.Errorf("%s is too large", f.Name)
	}
	return b, nil
}

// Extract returns the text of a document as units.
func Extract(format string, data []byte, lang string) (*filter.Extraction, error) {
	p, err := parse(format, data, lang)
	if err != nil {
		return nil, err
	}
	return p.ext, nil
}

// Merge returns the document with its units replaced by their
// translations. lang must be the language used for Extract.
func Merge(format string, data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	p, err := parse(format, data, lang)
	if err != nil {
		return nil, err
	}
	if format == XML {
		d := p.docs[""]
		var out strings.Builder
		d.render(&out, tr, 0, len(d.src))
		return p.enc(out.String()), nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range p.zip.File {
		h := &zip.FileHeader{Name: f.Name, Method: f.Method, Modified: f.Modified}
		if f.Name == "mimetype" {
			// The first entry, as it is, says what the file is.
			h.Method = zip.Store
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if d := p.docs[f.Name]; d != nil {
			var out strings.Builder
			d.render(&out, tr, 0, len(d.src))
			if _, err := io.WriteString(w, out.String()); err != nil {
				return nil, err
			}
			continue
		}
		raw, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(raw); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
