package textfmt

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// parseRESX reads a .NET resource file: the value of every data element
// that holds a string. Data with a type or MIME type (pictures, sizes,
// settings of a form) is not text and stays. The data's name is the key
// and its comment the note.
func parseRESX(src string) ([]*slot, error) {
	d := xml.NewDecoder(strings.NewReader(src))
	// The file is in memory as UTF-8, whatever its declaration says.
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	d.Strict = true
	var out []*slot
	var cur *slot
	inData, skip := false, false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the file is not well-formed XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "data" && !inData:
				inData, skip = true, false
				name := ""
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "name":
						name = a.Value
					case "type", "mimetype":
						skip = true
					}
				}
				cur = &slot{id: "r" + strconv.Itoa(len(out)+1), key: name, write: escapeXMLText}
			case t.Name.Local == "value" && inData && !skip && cur.end == 0:
				start := int(d.InputOffset())
				text, end, err := elementText(d)
				if err != nil {
					return nil, err
				}
				cur.start, cur.end, cur.text = start, end, text
			case t.Name.Local == "comment" && inData:
				text, _, err := elementText(d)
				if err != nil {
					return nil, err
				}
				if text = strings.TrimSpace(text); text != "" {
					cur.notes = append(cur.notes, text)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "data" && inData {
				if !skip && cur.end > 0 {
					out = append(out, cur)
				}
				inData, cur = false, nil
			}
		}
	}
	return out, nil
}

// elementText reads to the end of the element just opened and returns
// its text and where its end tag starts. An element inside it is an error.
func elementText(d *xml.Decoder) (string, int, error) {
	var b strings.Builder
	for {
		before := int(d.InputOffset())
		tok, err := d.Token()
		if err != nil {
			return "", 0, fmt.Errorf("the file is not well-formed XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), before, nil
		case xml.StartElement:
			return "", 0, fmt.Errorf("a value holds the element %s, which a resource string can't", t.Name.Local)
		}
	}
}

func escapeXMLText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
