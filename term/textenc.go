package term

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ToUTF8 converts file content to UTF-8: UTF-16 and UTF-8 with a byte order
// mark, UTF-16 without one (detected by zero bytes), and Windows-1252 when
// the bytes are not valid UTF-8 (older CSV exports from Excel).
func ToUTF8(b []byte) []byte {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return decodeUTF16(b[2:], binary.LittleEndian)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return decodeUTF16(b[2:], binary.BigEndian)
	case len(b) >= 4 && b[0] == '<' && b[1] == 0:
		return decodeUTF16(b, binary.LittleEndian)
	case len(b) >= 4 && b[0] == 0 && b[1] == '<':
		return decodeUTF16(b, binary.BigEndian)
	case !utf8.Valid(b):
		return cp1252(b)
	}
	return b
}

func decodeUTF16(b []byte, order binary.ByteOrder) []byte {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[2*i:])
	}
	return []byte(string(utf16.Decode(u)))
}

// The Windows-1252 characters that differ from ISO-8859-1.
var cp1252High = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž',
	0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

func cp1252(b []byte) []byte {
	var out strings.Builder
	for _, c := range b {
		if r, ok := cp1252High[c]; ok {
			out.WriteRune(r)
		} else {
			out.WriteRune(rune(c))
		}
	}
	return []byte(out.String())
}

// xmlCharset lets encoding/xml read documents whose declaration names an
// encoding after ToUTF8 already converted them.
func xmlCharset(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "utf-8", "utf8", "utf-16", "utf16", "utf-16le", "utf-16be", "unicode", "us-ascii", "ascii":
		return in, nil
	case "iso-8859-1", "latin1", "windows-1252", "cp1252":
		// ToUTF8 converted invalid UTF-8 already; valid UTF-8 passes.
		return in, nil
	}
	return nil, fmt.Errorf("unsupported encoding %q", label)
}

// utf16LE encodes s as UTF-16 little endian with a byte order mark, as
// MultiTerm writes its XML.
func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 2+2*len(u))
	out[0], out[1] = 0xFF, 0xFE
	for i, v := range u {
		binary.LittleEndian.PutUint16(out[2+2*i:], v)
	}
	return out
}
