// Package ttml reads Timed Text subtitles: TTML, and the files built on it,
// DFXP (.dfxp), Apple's iTT (.itt), EBU-TT and IMSC (often .xml).
//
// Each paragraph (p) of the body is one subtitle and one unit; it is not
// cut into sentences. Line breaks (br) are tags a translator moves or
// drops, spans and other elements in a paragraph are tags in pairs. The
// times are what a translator is first told about a subtitle. On merge,
// only the paragraphs that were translated are rewritten, and the root's
// xml:lang says the target language; everything else stays byte for byte.
package ttml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/para"
)

// Format is the name stored with a project's files.
const Format = "ttml"

// IsTTML reports whether an XML file is Timed Text: its root element is tt.
func IsTTML(data []byte) bool {
	src, _, err := decode(data)
	if err != nil {
		return false
	}
	d := xml.NewDecoder(strings.NewReader(src))
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	for {
		tok, err := d.RawToken()
		if err != nil {
			return false
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local == "tt"
		}
	}
}

type node struct {
	name            string // as written, with its prefix
	start, end      int
	inner, innerEnd int
	text            string
	isText          bool
	attrs           []xml.Attr
	kids            []*node
}

func local(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func (n *node) attr(localName string) string {
	for _, a := range n.attrs {
		if a.Name.Local == localName {
			return a.Value
		}
	}
	return ""
}

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
			name := t.Name.Local
			if t.Name.Space != "" {
				name = t.Name.Space + ":" + name
			}
			n := &node{name: name, start: before, inner: after, attrs: append([]xml.Attr(nil), t.Attr...)}
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			name := t.Name.Local
			if t.Name.Space != "" {
				name = t.Name.Space + ":" + name
			}
			if len(stack) < 2 || top.name != name {
				return nil, fmt.Errorf("the file is not well-formed XML: </%s> closes <%s>", name, top.name)
			}
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

// subtitle is a paragraph of the body, read into units.
type subtitle struct {
	n    *node
	para para.Para
}

type doc struct {
	src  string
	enc  func(string) []byte
	root *node // the tt element
	subs []*subtitle
	ext  *filter.Extraction
}

var spaces = regexp.MustCompile(`[ \t\r\n]+`)

func parse(data []byte, lang string) (*doc, error) {
	src, enc, err := decode(data)
	if err != nil {
		return nil, err
	}
	top, err := parseTree(src)
	if err != nil {
		return nil, err
	}
	d := &doc{src: src, enc: enc, ext: &filter.Extraction{Units: []filter.Unit{}, Blocks: []filter.Block{}, Format: Format, SourceLang: lang}}
	for _, k := range top.kids {
		if k.name != "" && !k.isText {
			d.root = k
			break
		}
	}
	if d.root == nil || local(d.root.name) != "tt" {
		return nil, fmt.Errorf("the file is not Timed Text (TTML): its root element is not tt")
	}
	var walk func(n *node, inBody, preserve bool)
	walk = func(n *node, inBody, preserve bool) {
		for _, k := range n.kids {
			if k.isText || k.name == "" {
				continue
			}
			p := preserve
			switch k.attr("space") {
			case "preserve":
				p = true
			case "default":
				p = false
			}
			switch local(k.name) {
			case "body":
				walk(k, true, p)
			case "p":
				if inBody {
					d.read(k, p)
				}
			case "metadata", "head", "styling", "layout":
				// Not shown as subtitles.
			default:
				walk(k, inBody, p)
			}
		}
	}
	walk(d.root, false, false)
	return d, nil
}

// read reads a subtitle: its text, with its line breaks and spans as tags.
func (d *doc) read(p *node, preserve bool) {
	var c content.Content
	id := 0
	var add func(k *node)
	add = func(k *node) {
		switch {
		case k.isText:
			t := k.text
			if !preserve {
				t = spaces.ReplaceAllString(t, " ")
			}
			c = c.Append(t)
		case k.name == "":
			id++
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(id), Type: "comment", Native: d.src[k.start:k.end]})
		case local(k.name) == "br":
			id++
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(id), Type: "break", Desc: "line break", Native: d.src[k.start:k.end]})
		case k.inner == k.end || len(k.kids) == 0:
			id++
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(id), Type: local(k.name), Native: d.src[k.start:k.end]})
		default:
			id++
			mine := strconv.Itoa(id)
			c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: mine, Type: local(k.name), Native: d.src[k.start:k.inner]})
			for _, kk := range k.kids {
				add(kk)
			}
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: mine, Type: local(k.name), Native: d.src[k.innerEnd:k.end]})
		}
	}
	for _, k := range p.kids {
		add(k)
	}
	if !preserve {
		c = trimAroundBreaks(c)
	}
	// One unit for the subtitle: its edges kept out of sight, not cut
	// into sentences.
	body, prefix, suffix := para.StripEdges(c)
	if !para.HasText(body) {
		return
	}
	lead, mid, trail := para.TrimSpace(body)
	src, ids := para.Renumber(mid)
	pr := para.Para{Prefix: prefix, Suffix: suffix, Units: 1}
	if len(lead) > 0 {
		pr.Pieces = append(pr.Pieces, para.Piece{Content: lead})
	}
	pr.Pieces = append(pr.Pieces, para.Piece{Unit: true, Content: src, IDs: ids})
	if len(trail) > 0 {
		pr.Pieces = append(pr.Pieces, para.Piece{Content: trail})
	}
	block := filter.Block{ID: "p" + strconv.Itoa(len(d.subs)+1), Part: filter.PartBody}
	pr.Add(d.ext, block)
	u := &d.ext.Units[len(d.ext.Units)-1]
	if begin := p.attr("begin"); begin != "" {
		u.Key = begin + " --> " + p.attr("end")
		if p.attr("end") == "" && p.attr("dur") != "" {
			u.Key = begin + " + " + p.attr("dur")
		}
		u.Notes = append(u.Notes, u.Key)
	}
	if who := p.attr("agent"); who != "" {
		u.Notes = append(u.Notes, "Speaker: "+who)
	}
	d.subs = append(d.subs, &subtitle{n: p, para: pr})
}

// trimAroundBreaks drops the spaces around line breaks, which the
// layout of the file puts there and a subtitle does not show.
func trimAroundBreaks(c content.Content) content.Content {
	for i := range c {
		if c[i].Tag != nil {
			continue
		}
		if i > 0 && c[i-1].Tag != nil && c[i-1].Tag.Type == "break" {
			c[i].Text = strings.TrimLeft(c[i].Text, " ")
		}
		if i+1 < len(c) && c[i+1].Tag != nil && c[i+1].Tag.Type == "break" {
			c[i].Text = strings.TrimRight(c[i].Text, " ")
		}
	}
	var out content.Content
	for _, r := range c {
		if r.Tag == nil && r.Text == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Extract returns the subtitles of a Timed Text file as units.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	return d.ext, nil
}

// Merge returns the file with its subtitles translated and its language
// set to targetLang.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	at := 0
	// The root's xml:lang, written or added.
	if targetLang != "" {
		start := d.src[d.root.start:d.root.inner]
		var tag string
		if m := langAttr.FindStringSubmatchIndex(start); m != nil {
			tag = start[:m[2]] + targetLang + start[m[3]:]
		} else {
			end := strings.LastIndex(start, ">")
			if strings.HasSuffix(start[:end], "/") {
				end--
			}
			tag = start[:end] + ` xml:lang="` + targetLang + `"` + start[end:]
		}
		out.WriteString(d.src[:d.root.start])
		out.WriteString(tag)
		at = d.root.inner
	}
	for _, s := range d.subs {
		if !s.para.Translated(d.ext, tr) {
			continue
		}
		out.WriteString(d.src[at:s.n.inner])
		for _, r := range s.para.Build(d.ext, tr) {
			if r.Tag == nil {
				out.WriteString(escape(r.Text))
			} else {
				out.WriteString(r.Tag.Native)
			}
		}
		at = s.n.innerEnd
	}
	out.WriteString(d.src[at:])
	return d.enc(out.String()), nil
}

var langAttr = regexp.MustCompile(`\sxml:lang\s*=\s*["']([^"']*)["']`)

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// decode reads XML as UTF-8, or UTF-16 with a byte order mark, and
// returns how to write it back the same way.
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
			w := utf16.Encode([]rune(s))
			out := []byte{data[0], data[1]}
			for _, x := range w {
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
		return "", nil, fmt.Errorf("the file is not UTF-8 or UTF-16")
	}
	return string(data), func(s string) []byte { return []byte(s) }, nil
}
