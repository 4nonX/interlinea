package align

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/segment"
)

// Read cuts a document into the segments a project would cut it into: a
// document of a format with a filter through its filter, anything else as
// text, a paragraph a line. name tells the format and lang the sentence
// rules.
func Read(name string, data []byte, lang string) ([]Segment, error) {
	format := formats.Of(name)
	switch {
	case formats.Document(format):
		ex, err := formats.Extract(format, data, lang)
		if err != nil {
			return nil, err
		}
		out := make([]Segment, 0, len(ex.Units))
		for _, u := range ex.Units {
			if strings.TrimSpace(u.Source.Plain()) == "" {
				continue
			}
			out = append(out, Segment{Block: u.Block, Content: u.Source})
		}
		return out, nil
	case format != formats.Text:
		return nil, fmt.Errorf("a %s holds both languages already: its translations are taken from it as they are", formats.Name(format))
	}
	text, err := decode(data)
	if err != nil {
		return nil, err
	}
	seg := segment.New(nil, lang)
	out := []Segment{}
	para := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		para++
		for _, sp := range seg.Split(line) {
			if s := strings.TrimSpace(line[sp.Start:sp.End]); s != "" {
				out = append(out, Segment{Block: fmt.Sprintf("p%d", para), Content: content.Text(s)})
			}
		}
	}
	return out, nil
}

// decode reads text as UTF-8 or, with a byte order mark, UTF-16.
func decode(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		body := data[2:]
		if len(body)%2 != 0 {
			return "", fmt.Errorf("the UTF-16 text has an odd number of bytes")
		}
		u := make([]uint16, len(body)/2)
		for i := range u {
			if le {
				u[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
			} else {
				u[i] = uint16(body[2*i])<<8 | uint16(body[2*i+1])
			}
		}
		return string(utf16.Decode(u)), nil
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("the file is not a document Interlinea reads, and not text in UTF-8 or UTF-16")
	}
	return string(data), nil
}
