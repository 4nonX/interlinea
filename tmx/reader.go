package tmx

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
)

const xmlNS = "http://www.w3.org/XML/1998/namespace"

// Reader reads translation units from a TMX document one at a time.
type Reader struct {
	dec    *xml.Decoder
	Header Header
	done   bool
}

// NewReader parses the document up to the start of the body and returns a
// reader positioned at the first unit. UTF-8, UTF-16 (with BOM),
// ISO-8859-1 and Windows-1252 input are accepted.
func NewReader(r io.Reader) (*Reader, error) {
	in, err := decodeInput(r)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(in)
	dec.CharsetReader = charsetReader
	rd := &Reader{dec: dec}
	if err := rd.readPrologue(); err != nil {
		return nil, err
	}
	return rd, nil
}

func (r *Reader) readPrologue() error {
	sawRoot := false
	for {
		tok, err := r.dec.Token()
		if err != nil {
			if err == io.EOF {
				return errors.New("tmx: missing <body>")
			}
			return fmt.Errorf("tmx: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "tmx":
			sawRoot = true
		case "header":
			if !sawRoot {
				return errors.New("tmx: <header> outside <tmx>")
			}
			if err := r.readHeader(se); err != nil {
				return err
			}
		case "body":
			if !sawRoot {
				return errors.New("tmx: not a TMX document")
			}
			return nil
		default:
			if !sawRoot {
				return fmt.Errorf("tmx: unexpected root element <%s>", se.Name.Local)
			}
			if err := r.dec.Skip(); err != nil {
				return fmt.Errorf("tmx: %w", err)
			}
		}
	}
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name && (a.Name.Space == "" || a.Name.Space == xmlNS) {
			return a.Value
		}
	}
	return ""
}

func langAttr(se xml.StartElement) string {
	for _, a := range se.Attr {
		if a.Name.Local == "lang" {
			return a.Value
		}
	}
	return ""
}

func (r *Reader) readHeader(se xml.StartElement) error {
	h := &r.Header
	h.CreationTool = attr(se, "creationtool")
	h.CreationToolVersion = attr(se, "creationtoolversion")
	h.SegType = attr(se, "segtype")
	h.OTMF = attr(se, "o-tmf")
	h.AdminLang = attr(se, "adminlang")
	h.SrcLang = attr(se, "srclang")
	h.DataType = attr(se, "datatype")
	h.CreationDate = parseDate(attr(se, "creationdate"))
	h.CreationID = attr(se, "creationid")
	for {
		tok, err := r.dec.Token()
		if err != nil {
			return fmt.Errorf("tmx: header: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "prop":
				v, err := r.text()
				if err != nil {
					return err
				}
				h.Props = append(h.Props, Prop{Type: attr(t, "type"), Value: v})
			case "note":
				v, err := r.text()
				if err != nil {
					return err
				}
				h.Notes = append(h.Notes, v)
			default:
				if err := r.dec.Skip(); err != nil {
					return fmt.Errorf("tmx: header: %w", err)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "header" {
				return nil
			}
		}
	}
}

// text returns all character data up to the end of the current element,
// including text of nested elements.
func (r *Reader) text() (string, error) {
	var b strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := r.dec.Token()
		if err != nil {
			return "", fmt.Errorf("tmx: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return b.String(), nil
}

// Next returns the next unit, or io.EOF after the last one.
func (r *Reader) Next() (*Unit, error) {
	if r.done {
		return nil, io.EOF
	}
	for {
		tok, err := r.dec.Token()
		if err != nil {
			if err == io.EOF {
				r.done = true
				return nil, io.ErrUnexpectedEOF
			}
			return nil, fmt.Errorf("tmx: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "tu" {
				return r.readUnit(t)
			}
			if err := r.dec.Skip(); err != nil {
				return nil, fmt.Errorf("tmx: %w", err)
			}
		case xml.EndElement:
			if t.Name.Local == "body" {
				r.done = true
				return nil, io.EOF
			}
		}
	}
}

func (r *Reader) readUnit(se xml.StartElement) (*Unit, error) {
	u := &Unit{
		ID:           attr(se, "tuid"),
		SrcLang:      attr(se, "srclang"),
		CreationDate: parseDate(attr(se, "creationdate")),
		CreationID:   attr(se, "creationid"),
		ChangeDate:   parseDate(attr(se, "changedate")),
		ChangeID:     attr(se, "changeid"),
		LastUsage:    parseDate(attr(se, "lastusagedate")),
	}
	if uc := attr(se, "usagecount"); uc != "" {
		if n, err := strconv.Atoi(uc); err == nil && n >= 0 {
			u.UsageCount = n
		}
	}
	for {
		tok, err := r.dec.Token()
		if err != nil {
			return nil, fmt.Errorf("tmx: tu %q: %w", u.ID, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "prop":
				v, err := r.text()
				if err != nil {
					return nil, err
				}
				u.Props = append(u.Props, Prop{Type: attr(t, "type"), Value: v})
			case "note":
				v, err := r.text()
				if err != nil {
					return nil, err
				}
				u.Notes = append(u.Notes, v)
			case "tuv":
				v, err := r.readVariant(t)
				if err != nil {
					return nil, fmt.Errorf("tmx: tu %q: %w", u.ID, err)
				}
				u.Variants = append(u.Variants, *v)
			default:
				if err := r.dec.Skip(); err != nil {
					return nil, fmt.Errorf("tmx: %w", err)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "tu" {
				return u, nil
			}
		}
	}
}

func (r *Reader) readVariant(se xml.StartElement) (*Variant, error) {
	v := &Variant{
		Lang:         langAttr(se),
		CreationDate: parseDate(attr(se, "creationdate")),
		CreationID:   attr(se, "creationid"),
		ChangeDate:   parseDate(attr(se, "changedate")),
		ChangeID:     attr(se, "changeid"),
		Content:      content.Content{},
	}
	if v.Lang == "" {
		return nil, errors.New("tuv without language")
	}
	for {
		tok, err := r.dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "seg":
				c, err := r.readSeg()
				if err != nil {
					return nil, err
				}
				v.Content = c
			case "prop":
				val, err := r.text()
				if err != nil {
					return nil, err
				}
				v.Props = append(v.Props, Prop{Type: attr(t, "type"), Value: val})
			case "note":
				val, err := r.text()
				if err != nil {
					return nil, err
				}
				v.Notes = append(v.Notes, val)
			default:
				if err := r.dec.Skip(); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "tuv" {
				return v, nil
			}
		}
	}
}

// readSeg converts <seg> content to tagged content.
func (r *Reader) readSeg() (content.Content, error) {
	c := content.Content{}
	gen := 0
	used := map[string]bool{}
	newID := func(prefix, want string) string {
		if want != "" && !used[prefix+want] {
			used[prefix+want] = true
			return prefix + want
		}
		for {
			gen++
			id := fmt.Sprintf("%s%d", prefix, gen)
			if !used[id] {
				used[id] = true
				return id
			}
		}
	}
	type hiOpen struct{ id, typ string }
	var hiStack []hiOpen
	for {
		tok, err := r.dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			c = c.Append(string(t))
		case xml.StartElement:
			typ := attr(t, "type")
			switch t.Name.Local {
			case "bpt":
				native, err := r.text()
				if err != nil {
					return nil, err
				}
				c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: newID("", attr(t, "i")), Type: typ, Native: native})
			case "ept":
				native, err := r.text()
				if err != nil {
					return nil, err
				}
				// Close tags reuse the ID of their opening bpt.
				id := attr(t, "i")
				if id == "" {
					id = newID("e", "")
				}
				c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: id, Type: typ, Native: native})
			case "ph", "ut":
				native, err := r.text()
				if err != nil {
					return nil, err
				}
				c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: newID("ph", attr(t, "x")), Type: typ, Native: native})
			case "it":
				native, err := r.text()
				if err != nil {
					return nil, err
				}
				kind := content.TagOpen
				if attr(t, "pos") == "end" {
					kind = content.TagClose
				}
				c = c.AppendTag(content.Tag{Kind: kind, ID: newID("it", attr(t, "x")), Type: typ, Native: native})
			case "hi":
				id := newID("hi", attr(t, "x"))
				hiStack = append(hiStack, hiOpen{id: id, typ: typ})
				c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: id, Type: typ})
			default:
				// Unknown inline element: keep its text, drop the markup.
				txt, err := r.text()
				if err != nil {
					return nil, err
				}
				c = c.Append(txt)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "hi":
				if n := len(hiStack); n > 0 {
					h := hiStack[n-1]
					hiStack = hiStack[:n-1]
					c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: h.id, Type: h.typ})
				}
			case "seg":
				c = c.Normalize()
				if err := c.Validate(); err != nil {
					return nil, err
				}
				return c, nil
			}
		}
	}
}
