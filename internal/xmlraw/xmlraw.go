// Package xmlraw reads XML into a tree that keeps the original bytes of
// every node, so filters can rewrite only what they change: namespace
// prefixes, attribute order, entities and unknown markup elsewhere survive
// exactly. Changes are made as edits to byte ranges of the original.
package xmlraw

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Node is an element, character data, or other markup (comment, processing
// instruction, directive) with the bytes it came from.
type Node struct {
	// Prefix is the namespace prefix as written ("sdl"), Local the local
	// name ("seg"). Local is empty for non-element nodes.
	Prefix string
	Local  string
	Attrs  []xml.Attr
	// Start is the raw start tag, the whole element when it is
	// self-closing; End the raw end tag, empty when self-closing.
	Start []byte
	End   []byte
	// Raw holds all bytes of the node; Off is its offset in the document.
	Raw []byte
	Off int
	// Text is the decoded character data of a text node.
	Text string
	// CharData is true for text nodes.
	CharData bool
	Kids     []*Node
}

// Span is a byte range [start, end) of the document.
type Span [2]int

// IsElement reports whether the node is an element.
func (n *Node) IsElement() bool { return n.Local != "" }

// SelfClosing reports whether the element was written as <x/>.
func (n *Node) SelfClosing() bool { return n.IsElement() && len(n.End) == 0 }

// Name returns the qualified name as written ("sdl:seg").
func (n *Node) Name() string {
	if n.Prefix == "" {
		return n.Local
	}
	return n.Prefix + ":" + n.Local
}

// Whole is the node's byte range.
func (n *Node) Whole() Span { return Span{n.Off, n.Off + len(n.Raw)} }

// StartTag is the range of the start tag (the whole element when self-closing).
func (n *Node) StartTag() Span { return Span{n.Off, n.Off + len(n.Start)} }

// EndTag is the range of the end tag (empty at the end for self-closing elements).
func (n *Node) EndTag() Span {
	return Span{n.Off + len(n.Raw) - len(n.End), n.Off + len(n.Raw)}
}

// Inner is the range between the start and the end tag.
func (n *Node) Inner() Span {
	if n.SelfClosing() {
		return Span{n.Off + len(n.Raw), n.Off + len(n.Raw)}
	}
	return Span{n.Off + len(n.Start), n.Off + len(n.Raw) - len(n.End)}
}

// InnerRaw returns the bytes between the start and the end tag.
func (n *Node) InnerRaw() []byte {
	if n.SelfClosing() {
		return nil
	}
	return n.Raw[len(n.Start) : len(n.Raw)-len(n.End)]
}

// Child returns the first child element with the local name.
func (n *Node) Child(local string) *Node {
	for _, k := range n.Kids {
		if k.Local == local {
			return k
		}
	}
	return nil
}

// ChildNS returns the first child element with the prefix and local name.
func (n *Node) ChildNS(prefix, local string) *Node {
	for _, k := range n.Kids {
		if k.Local == local && k.Prefix == prefix {
			return k
		}
	}
	return nil
}

// Children returns the child elements with the local name.
func (n *Node) Children(local string) []*Node {
	var out []*Node
	for _, k := range n.Kids {
		if k.Local == local {
			out = append(out, k)
		}
	}
	return out
}

// Elements returns the child elements.
func (n *Node) Elements() []*Node {
	var out []*Node
	for _, k := range n.Kids {
		if k.IsElement() {
			out = append(out, k)
		}
	}
	return out
}

// Attr returns an unprefixed attribute's value.
func (n *Node) Attr(local string) (string, bool) {
	return n.AttrNS("", local)
}

// AttrAny returns the value of the first attribute with the local name,
// whatever its prefix ("w:val" and "val" both match "val").
func (n *Node) AttrAny(local string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// AttrNS returns the value of an attribute with the prefix ("" for none).
func (n *Node) AttrNS(prefix, local string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Local == local && a.Name.Space == prefix {
			return a.Value, true
		}
	}
	return "", false
}

// Get returns an unprefixed attribute's value, or "".
func (n *Node) Get(local string) string {
	v, _ := n.Attr(local)
	return v
}

// InnerText concatenates the decoded text of all descendants.
func (n *Node) InnerText() string {
	if n.CharData {
		return n.Text
	}
	var b strings.Builder
	for _, k := range n.Kids {
		b.WriteString(k.InnerText())
	}
	return b.String()
}

// Walk calls fn for n and its descendants in document order; returning
// false skips a node's children.
func (n *Node) Walk(fn func(*Node) bool) {
	if !fn(n) {
		return
	}
	for _, k := range n.Kids {
		k.Walk(fn)
	}
}

// Namespaces returns the prefixes the element declares, by namespace URI
// ("" for the default namespace).
func (n *Node) Namespaces() map[string]string {
	out := map[string]string{}
	for _, a := range n.Attrs {
		switch {
		case a.Name.Space == "xmlns":
			out[a.Value] = a.Name.Local
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			out[a.Value] = ""
		}
	}
	return out
}

// Token is one raw token with its byte range in the document.
type Token struct {
	Tok        xml.Token
	Start, End int
}

// Tokenize splits a UTF-8 document into raw tokens without namespace
// processing, for callers that stream through a document.
func Tokenize(data []byte) ([]Token, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	d.Entity = xml.HTMLEntity
	// The document is UTF-8 by the time it is parsed (see Document); an
	// encoding declaration naming another charset is accepted as is.
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var out []Token
	for {
		start := int(d.InputOffset())
		t, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("xml at byte %d: %w", start, err)
		}
		out = append(out, Token{Tok: xml.CopyToken(t), Start: start, End: int(d.InputOffset())})
	}
}

// Build turns toks[i:] into the tree of one element and returns the index
// after it. toks[i] must be a start element.
func Build(data []byte, toks []Token, i int) (*Node, int, error) {
	st, ok := toks[i].Tok.(xml.StartElement)
	if !ok {
		return nil, i, fmt.Errorf("expected start element at byte %d", toks[i].Start)
	}
	n := &Node{Prefix: st.Name.Space, Local: st.Name.Local, Attrs: st.Attr, Start: data[toks[i].Start:toks[i].End], Off: toks[i].Start}
	begin := toks[i].Start
	// RawToken reports a self-closing element as a start and an end token
	// at the same offset.
	if i+1 < len(toks) {
		if e, ok := toks[i+1].Tok.(xml.EndElement); ok && toks[i+1].Start == toks[i].End && toks[i+1].End == toks[i].End && e.Name == st.Name {
			n.Raw = data[begin:toks[i].End]
			return n, i + 2, nil
		}
	}
	i++
	for i < len(toks) {
		switch t := toks[i].Tok.(type) {
		case xml.EndElement:
			if t.Name.Local != n.Local || t.Name.Space != n.Prefix {
				return nil, i, fmt.Errorf("mismatched end tag %s at byte %d", qname(t.Name), toks[i].Start)
			}
			n.End = data[toks[i].Start:toks[i].End]
			n.Raw = data[begin:toks[i].End]
			return n, i + 1, nil
		case xml.StartElement:
			k, next, err := Build(data, toks, i)
			if err != nil {
				return nil, i, err
			}
			n.Kids = append(n.Kids, k)
			i = next
		case xml.CharData:
			n.Kids = append(n.Kids, &Node{Raw: data[toks[i].Start:toks[i].End], Text: string(t), CharData: true, Off: toks[i].Start})
			i++
		default:
			n.Kids = append(n.Kids, &Node{Raw: data[toks[i].Start:toks[i].End], Off: toks[i].Start})
			i++
		}
	}
	return nil, i, fmt.Errorf("unclosed element %s", n.Name())
}

func qname(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

// Parse returns the root element of a UTF-8 document.
func Parse(data []byte) (*Node, error) {
	toks, err := Tokenize(data)
	if err != nil {
		return nil, err
	}
	for i, t := range toks {
		if _, ok := t.Tok.(xml.StartElement); ok {
			root, next, err := Build(data, toks, i)
			if err != nil {
				return nil, err
			}
			for _, rest := range toks[next:] {
				switch rest.Tok.(type) {
				case xml.StartElement, xml.EndElement:
					return nil, fmt.Errorf("xml at byte %d: content after the root element", rest.Start)
				case xml.CharData:
					if len(bytes.TrimSpace(data[rest.Start:rest.End])) > 0 {
						return nil, fmt.Errorf("xml at byte %d: text after the root element", rest.Start)
					}
				}
			}
			return root, nil
		}
	}
	return nil, errors.New("xml: no root element")
}

// ParseFragment parses one well-formed element such as a copied tag.
func ParseFragment(b []byte) (*Node, error) {
	toks, err := Tokenize(b)
	if err != nil {
		return nil, err
	}
	for i, t := range toks {
		if _, ok := t.Tok.(xml.StartElement); ok {
			n, _, err := Build(b, toks, i)
			return n, err
		}
	}
	return nil, errors.New("xml: no element in fragment")
}

// Encoding is how a document was stored, so it can be written back the
// same way.
type Encoding int

const (
	UTF8 Encoding = iota
	UTF8BOM
	UTF16LE
	UTF16BE
)

// Document is a parsed file with its encoding.
type Document struct {
	// Data is the document as UTF-8 without a byte order mark; offsets of
	// nodes refer to it.
	Data     []byte
	Root     *Node
	Encoding Encoding
}

// Read decodes a file (UTF-8 with or without a byte order mark, or UTF-16
// with one) and parses it.
func Read(file []byte) (*Document, error) {
	doc := &Document{}
	switch {
	case bytes.HasPrefix(file, []byte{0xEF, 0xBB, 0xBF}):
		doc.Encoding, doc.Data = UTF8BOM, file[3:]
	case bytes.HasPrefix(file, []byte{0xFF, 0xFE}):
		doc.Encoding, doc.Data = UTF16LE, decodeUTF16(file[2:], false)
	case bytes.HasPrefix(file, []byte{0xFE, 0xFF}):
		doc.Encoding, doc.Data = UTF16BE, decodeUTF16(file[2:], true)
	default:
		doc.Encoding, doc.Data = UTF8, file
	}
	if !utf8.Valid(doc.Data) {
		return nil, errors.New("xml: the file is not valid UTF-8 or UTF-16")
	}
	root, err := Parse(doc.Data)
	if err != nil {
		return nil, err
	}
	doc.Root = root
	return doc, nil
}

func decodeUTF16(b []byte, big bool) []byte {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		if big {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
		}
	}
	return []byte(string(utf16.Decode(u)))
}

// Encode writes UTF-8 data in the document's original encoding.
func (d *Document) Encode(data []byte) []byte {
	switch d.Encoding {
	case UTF8BOM:
		return append([]byte{0xEF, 0xBB, 0xBF}, data...)
	case UTF16LE, UTF16BE:
		u := utf16.Encode([]rune(string(data)))
		out := make([]byte, 2, 2+2*len(u))
		if d.Encoding == UTF16LE {
			out[0], out[1] = 0xFF, 0xFE
		} else {
			out[0], out[1] = 0xFE, 0xFF
		}
		for _, c := range u {
			if d.Encoding == UTF16LE {
				out = append(out, byte(c), byte(c>>8))
			} else {
				out = append(out, byte(c>>8), byte(c))
			}
		}
		return out
	default:
		return data
	}
}

// Edit replaces the bytes of a span. An empty span inserts.
type Edit struct {
	Span Span
	With []byte
}

// Apply returns data with the edits made. Edits must not overlap; inserts
// at the same offset keep their order.
func Apply(data []byte, edits []Edit) ([]byte, error) {
	es := append([]Edit(nil), edits...)
	sort.SliceStable(es, func(i, j int) bool { return es[i].Span[0] < es[j].Span[0] })
	var out bytes.Buffer
	out.Grow(len(data))
	pos := 0
	for _, e := range es {
		if e.Span[0] < pos || e.Span[1] < e.Span[0] || e.Span[1] > len(data) {
			return nil, fmt.Errorf("xmlraw: overlapping or invalid edit at %d-%d", e.Span[0], e.Span[1])
		}
		out.Write(data[pos:e.Span[0]])
		out.Write(e.With)
		pos = e.Span[1]
	}
	out.Write(data[pos:])
	return out.Bytes(), nil
}

// SetAttr returns a start tag with the attribute set to value: changed in
// place if present, added before the closing ">" or "/>" otherwise. name
// is the qualified name as written ("state", "mq:status").
func SetAttr(startTag []byte, name, value string) []byte {
	if s, e, ok := findAttr(startTag, name); ok {
		out := append([]byte(nil), startTag[:s]...)
		out = append(out, name+`="`+EscapeAttr(value)+`"`...)
		return append(out, startTag[e:]...)
	}
	end := len(startTag) - 1
	if end > 0 && startTag[end-1] == '/' {
		end--
	}
	for end > 0 && isSpace(startTag[end-1]) {
		end--
	}
	out := append([]byte(nil), startTag[:end]...)
	out = append(out, ` `+name+`="`+EscapeAttr(value)+`"`...)
	return append(out, startTag[end:]...)
}

// RemoveAttr returns a start tag without the attribute.
func RemoveAttr(startTag []byte, name string) []byte {
	s, e, ok := findAttr(startTag, name)
	if !ok {
		return startTag
	}
	for s > 0 && isSpace(startTag[s-1]) {
		s--
	}
	out := append([]byte(nil), startTag[:s]...)
	return append(out, startTag[e:]...)
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// findAttr locates name="value" (or with single quotes) in a start tag.
func findAttr(tag []byte, name string) (start, end int, ok bool) {
	i := 1
	for i < len(tag) && !isSpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
		i++ // element name
	}
	for i < len(tag) {
		for i < len(tag) && isSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] == '>' || tag[i] == '/' {
			return 0, 0, false
		}
		ns := i
		for i < len(tag) && tag[i] != '=' && !isSpace(tag[i]) {
			i++
		}
		attr := string(tag[ns:i])
		for i < len(tag) && (isSpace(tag[i]) || tag[i] == '=') {
			i++
		}
		if i >= len(tag) {
			return 0, 0, false
		}
		q := tag[i]
		j := bytes.IndexByte(tag[i+1:], q)
		if j < 0 {
			return 0, 0, false
		}
		vEnd := i + 1 + j + 1
		if attr == name {
			return ns, vEnd, true
		}
		i = vEnd
	}
	return 0, 0, false
}

// EscapeText escapes character data and drops characters XML 1.0 forbids.
func EscapeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(r)
		case r < 0x20 || r == 0xFFFE || r == 0xFFFF:
			// not allowed in XML 1.0
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// EscapeAttr escapes an attribute value for double quotes.
func EscapeAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\n", "&#10;", "\r", "&#13;", "\t", "&#9;").Replace(s)
}
