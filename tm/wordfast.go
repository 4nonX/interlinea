package tm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/4nonX/interlinea/content"
	"golang.org/x/text/encoding/charmap"
)

// A Wordfast translation memory is a text file of tab-separated lines,
// written as UTF-16 or as Latin-1. The first line is a header whose
// fields start with "%": a date, the users, the unit count ("%TU=…"), the
// source language, the version ("%Wordfast TM …") and the target
// language. Every other line is a unit: date (YYYYMMDD~hhmmss), user,
// times used, source language, source, target language, target, then
// up to five fields of the user's own.
//
// Wordfast writes characters that Word or Excel could mistake as escapes
// of the form &'XX; (&'26; for "&"), a tab as \t and a line break as \n.
// &tA;, &tB; and the like stand for the formatting codes of the document,
// and become placeholders.

const wordfastTime = "20060102~150405"

// wordfastEscapes are the characters Wordfast writes as &'XX;: the
// Windows codes, then those the Mac version writes differently.
var wordfastEscapes = map[string]string{
	"26": "&", "82": "\u201a", "85": "\u2026", "91": "\u2018", "92": "\u2019", "93": "\u201c", "94": "\u201d",
	"96": "\u2013", "97": "\u2014", "99": "\u2122", "A0": "\u00a0", "A9": "\u00a9", "AE": "\u00ae",
	"BC": "\u00bc", "BD": "\u00bd", "BE": "\u00be",
	"A8": "\u00ae", "AA": "\u2122", "C7": "\u00ab", "C8": "\u00bb", "C9": "\u2026", "CA": "\u00a0",
	"D0": "\u2013", "D1": "\u2014", "D2": "\u201c", "D3": "\u201d", "D4": "\u2018", "D5": "\u2019",
}

var wordfastCode = regexp.MustCompile(`&'([0-9A-Fa-f]{2});|&t[A-Za-z];|\\[nt]`)

// IsWordfastTM reports whether data looks like a Wordfast memory: its
// first line, read as text, names the Wordfast TM version.
func IsWordfastTM(data []byte) bool {
	text, err := wordfastText(data)
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(text, "\n")
	return wordfastHeader(first)
}

var wordfastDate = regexp.MustCompile(`^%\d{8}~\d{6}$`)

// wordfastHeader reports whether line is the header of a Wordfast memory:
// "%" and a date, the users, "%TU=" and the unit count, the languages and
// the version between them ("%Wordfast TM v.546/00", or "%Wordfast
// translation memory version v.5" in later versions).
func wordfastHeader(line string) bool {
	f := strings.Split(strings.TrimRight(strings.TrimLeft(line, "\ufeff"), "\r"), "\t")
	return len(f) >= 6 && wordfastDate.MatchString(strings.TrimSpace(f[0])) && strings.HasPrefix(f[2], "%TU=") && strings.HasPrefix(strings.ToLower(f[4]), "%wordfast")
}

// ReadWordfast reads a Wordfast translation memory (.txt).
func ReadWordfast(data []byte) (*File, error) {
	text, err := wordfastText(data)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || !wordfastHeader(lines[0]) {
		return nil, errors.New("wordfast: the first line is not the header of a Wordfast translation memory")
	}
	f := &File{Format: FormatWordfast}
	head := strings.Split(strings.TrimLeft(lines[0], "\ufeff"), "\t")
	if len(head) > 3 {
		f.SourceLang = wordfastLang(head[3])
	}
	if len(head) > 5 {
		f.TargetLang = wordfastLang(head[5])
	}
	var attrNames []string
	for i := 7; i < len(head) && i < 12; i++ {
		// "%Attr1,…" lists the values the user defined; its first part
		// names the field.
		name, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(head[i]), "%"), ",")
		attrNames = append(attrNames, strings.TrimSpace(name))
	}
	for n, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 7 {
			f.Skipped++
			if len(f.Warnings) < 20 {
				f.Warnings = append(f.Warnings, fmt.Sprintf("Line %d has %d fields, not 7 or more; it was left out.", n+2, len(cols)))
			}
			continue
		}
		u := Unit{Source: wordfastContent(cols[4]), Target: wordfastContent(cols[6]), CreatedBy: strings.TrimSpace(cols[1])}
		if t, err := time.Parse(wordfastTime, strings.TrimSpace(cols[0])); err == nil {
			u.CreatedAt, u.ChangedAt = t, t
			u.ChangedBy = u.CreatedBy
		}
		u.UsageCount, _ = strconv.Atoi(strings.TrimSpace(cols[2]))
		for i := 7; i < len(cols) && i < 12; i++ {
			v := strings.TrimSpace(cols[i])
			if v == "" {
				continue
			}
			name := fmt.Sprintf("Attribute %d", i-6)
			if i-7 < len(attrNames) && attrNames[i-7] != "" {
				name = attrNames[i-7]
			}
			u.Attributes = append(u.Attributes, Attribute{Name: name, Value: v})
		}
		f.Units = append(f.Units, FileUnit{Unit: u, SourceLang: wordfastLang(cols[3]), TargetLang: wordfastLang(cols[5])})
	}
	return f, nil
}

// wordfastText decodes the file: UTF-16 with or without a byte order mark
// (told by where its tabs fall), UTF-8 with a mark or when valid, else
// Latin-1.
func wordfastText(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		return decodeUTF16(data[2:], binary.LittleEndian), nil
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		return decodeUTF16(data[2:], binary.BigEndian), nil
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return string(data[3:]), nil
	}
	first := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		first = data[:i]
	}
	for i := 0; i+1 < len(first); i += 2 {
		if first[i] == '\t' && first[i+1] == 0 {
			return decodeUTF16(data, binary.LittleEndian), nil
		}
		if first[i] == 0 && first[i+1] == '\t' {
			return decodeUTF16(data, binary.BigEndian), nil
		}
	}
	if isUTF8(data) {
		return string(data), nil
	}
	return charmap.ISO8859_1.NewDecoder().String(string(data))
}

func decodeUTF16(b []byte, order binary.ByteOrder) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

func isUTF8(b []byte) bool {
	return strings.ToValidUTF8(string(b), "\ufffd") == string(b)
}

// wordfastLang turns "%EN-US" or "DE-01" (the language with no variant)
// into a language tag.
func wordfastLang(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "%"))
	s = strings.TrimSuffix(s, "-01")
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if i == 0 {
			parts[i] = strings.ToLower(p)
		} else if len(p) == 2 {
			parts[i] = strings.ToUpper(p)
		}
	}
	return strings.Join(parts, "-")
}

// wordfastContent unescapes a source or target. Formatting codes become
// placeholders numbered in their order.
func wordfastContent(s string) content.Content {
	c := content.Content{}
	n := 0
	last := 0
	for _, m := range wordfastCode.FindAllStringSubmatchIndex(s, -1) {
		c = c.Append(s[last:m[0]])
		code := s[m[0]:m[1]]
		switch {
		case code == `\n`:
			c = c.Append("\n")
		case code == `\t`:
			c = c.Append("\t")
		case strings.HasPrefix(code, "&t"):
			n++
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(n), Native: code})
		default:
			if ch, ok := wordfastEscapes[strings.ToUpper(s[m[2]:m[3]])]; ok {
				c = c.Append(ch)
			} else {
				c = c.Append(code)
			}
		}
		last = m[1]
	}
	c = c.Append(s[last:])
	return c
}
