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
	d := xml.NewDecoder(bytes.NewReader(ToUTF8(data)))
	d.CharsetReader = xmlCharset
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
