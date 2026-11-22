package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// The filter never re-encodes XML it did not change. Elements are kept as
// slices of the original bytes, so namespace prefixes, attribute order and
// unknown markup survive exactly.

// node is an element or a text node with the raw bytes it came from.
type node struct {
	prefix string // namespace prefix as written ("w")
	local  string // local name ("p"); empty for text nodes
	attrs  []xml.Attr
	start  []byte // raw start tag; the whole element when self-closing
	end    []byte // raw end tag; empty when self-closing
	raw    []byte // all bytes of the node
	off    int    // byte offset of the node in its part
	text   string // decoded character data (text nodes only)
	kids   []*node
}

func (n *node) isText() bool { return n.local == "" }

// span is a byte range [start, end) of a part.
type span [2]int

func (n *node) whole() span    { return span{n.off, n.off + len(n.raw)} }
func (n *node) startTag() span { return span{n.off, n.off + len(n.start)} }
func (n *node) endTag() span   { return span{n.off + len(n.raw) - len(n.end), n.off + len(n.raw)} }

func (n *node) child(local string) *node {
	for _, k := range n.kids {
		if k.local == local {
			return k
		}
	}
	return nil
}

func (n *node) attr(local string) (string, bool) {
	for _, a := range n.attrs {
		if a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// innerText concatenates the decoded text of all descendants.
func (n *node) innerText() string {
	if n.isText() {
		return n.text
	}
	var b strings.Builder
	for _, k := range n.kids {
		b.WriteString(k.innerText())
	}
	return b.String()
}

// token is one raw token with its byte range in the part.
type token struct {
	tok        xml.Token
	start, end int
}

// tokenize splits an XML part into raw tokens without namespace processing.
func tokenize(data []byte) ([]token, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	var out []token
	for {
		start := int(d.InputOffset())
		t, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("xml at byte %d: %w", start, err)
		}
		out = append(out, token{tok: xml.CopyToken(t), start: start, end: int(d.InputOffset())})
	}
}

// build turns toks[i:] into a node tree for one element and returns the
// index after it. toks[i] must be a start element.
func build(data []byte, toks []token, i int) (*node, int, error) {
	st, ok := toks[i].tok.(xml.StartElement)
	if !ok {
		return nil, i, fmt.Errorf("expected start element at byte %d", toks[i].start)
	}
	n := &node{prefix: st.Name.Space, local: st.Name.Local, attrs: st.Attr, start: data[toks[i].start:toks[i].end], off: toks[i].start}
	begin := toks[i].start
	i++
	for i < len(toks) {
		switch t := toks[i].tok.(type) {
		case xml.EndElement:
			if t.Name.Local != n.local || t.Name.Space != n.prefix {
				return nil, i, fmt.Errorf("mismatched end tag %s:%s at byte %d", t.Name.Space, t.Name.Local, toks[i].start)
			}
			n.end = data[toks[i].start:toks[i].end]
			n.raw = data[begin:toks[i].end]
			return n, i + 1, nil
		case xml.StartElement:
			k, next, err := build(data, toks, i)
			if err != nil {
				return nil, i, err
			}
			n.kids = append(n.kids, k)
			i = next
		case xml.CharData:
			n.kids = append(n.kids, &node{raw: data[toks[i].start:toks[i].end], text: string(t), off: toks[i].start})
			i++
		default: // comments, processing instructions
			n.kids = append(n.kids, &node{raw: data[toks[i].start:toks[i].end], off: toks[i].start})
			i++
		}
	}
	return nil, i, fmt.Errorf("unclosed element %s:%s", n.prefix, n.local)
}

// parseFragment parses a well-formed fragment such as a raw rPr element.
func parseFragment(b []byte) (*node, error) {
	toks, err := tokenize(b)
	if err != nil {
		return nil, err
	}
	for i, t := range toks {
		if _, ok := t.tok.(xml.StartElement); ok {
			n, _, err := build(b, toks, i)
			return n, err
		}
	}
	return nil, errors.New("no element in fragment")
}

// escapeText escapes character data and drops characters XML 1.0 forbids.
func escapeText(s string) string {
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

func escapeAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
