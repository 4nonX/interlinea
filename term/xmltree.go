package term

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// xnode is a generic XML element; files are read one entry at a time.
type xnode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []xnode    `xml:",any"`
	Text    string     `xml:",chardata"`
}

func (n *xnode) attr(local string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// lang returns xml:lang (or a plain lang attribute).
func (n *xnode) lang() string {
	for _, a := range n.Attrs {
		if a.Name.Local == "lang" {
			return a.Value
		}
	}
	return ""
}

func (n *xnode) text() string {
	if len(n.Nodes) == 0 {
		return strings.TrimSpace(n.Text)
	}
	var b strings.Builder
	b.WriteString(n.Text)
	for i := range n.Nodes {
		b.WriteString(n.Nodes[i].text())
	}
	return strings.TrimSpace(b.String())
}

func (n *xnode) child(local string) *xnode {
	for i := range n.Nodes {
		if n.Nodes[i].XMLName.Local == local {
			return &n.Nodes[i]
		}
	}
	return nil
}

// eachElement calls fn for every element named one of names, decoded in
// full, while streaming through the document. It returns the local name
// of the root element.
func eachElement(data []byte, names map[string]bool, fn func(*xnode) error) (string, error) {
	d := xml.NewDecoder(bytes.NewReader(repairAmpersands(ToUTF8(data))))
	d.CharsetReader = xmlCharset
	// Glossary tools write &nbsp; and the like without declaring them.
	d.Entity = xml.HTMLEntity
	root := ""
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return root, nil
		}
		if err != nil {
			return root, err
		}
		st, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if root == "" {
			root = st.Name.Local
		}
		if !names[st.Name.Local] {
			continue
		}
		var n xnode
		if err := d.DecodeElement(&n, &st); err != nil {
			return root, err
		}
		if err := fn(&n); err != nil {
			return root, err
		}
	}
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func escAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\n", "&#10;", "\t", "&#9;").Replace(s)
}

// repairAmpersands escapes every & that does not start an entity or a
// character reference. Exports of glossary tools often carry a bare "&"
// in their text ("syn & lay"), which is not well-formed XML; the text is
// plainly meant, so it is read as written instead of refusing the file.
// CDATA sections, comments and processing instructions are left alone.
func repairAmpersands(data []byte) []byte {
	if !bytes.Contains(data, []byte("&")) {
		return data
	}
	var out []byte
	last := 0
	for i := 0; i < len(data); i++ {
		switch {
		case bytes.HasPrefix(data[i:], []byte("<![CDATA[")):
			i = skipTo(data, i, "]]>")
		case bytes.HasPrefix(data[i:], []byte("<!--")):
			i = skipTo(data, i, "-->")
		case bytes.HasPrefix(data[i:], []byte("<?")):
			i = skipTo(data, i, "?>")
		case data[i] == '&' && !startsReference(data[i+1:]):
			if out == nil {
				out = make([]byte, 0, len(data)+16)
			}
			out = append(out, data[last:i]...)
			out = append(out, "&amp;"...)
			last = i + 1
		}
	}
	if out == nil {
		return data
	}
	return append(out, data[last:]...)
}

// skipTo returns the index of the last byte of the first end after i, or
// of the data's last byte when there is none.
func skipTo(data []byte, i int, end string) int {
	k := bytes.Index(data[i:], []byte(end))
	if k < 0 {
		return len(data) - 1
	}
	return i + k + len(end) - 1
}

// startsReference reports whether s, the text after an &, goes on as an
// entity ("amp;", "nbsp;") or a character reference ("#160;", "#xA0;").
func startsReference(s []byte) bool {
	n := 0
	for n < len(s) && n < 32 {
		c := s[n]
		switch {
		case c == ';':
			return n > 0 && (s[0] != '#' || n > 1)
		case c == '#' && n == 0:
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		default:
			return false
		}
		n++
	}
	return false
}
