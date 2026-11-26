package ebustl

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

// The character code tables of the text fields (EBU Tech 3264, Appendix 2):
// 00 Latin, ISO 6937 with its accents written before their letter; 01 to
// 04 Latin with Cyrillic, Arabic, Greek or Hebrew, as ISO 8859-5 to 8.
// The bytes 20h to 7Fh are ASCII in all of them, but for 24h, which is the
// currency sign in table 00 (the dollar is A4h there).

// latinHigh are the characters of table 00 from A0h on; 0 where it has
// none, and the accents C1h to CFh, which are written before their letter.
var latinHigh = [96]rune{
	0xA0, '¡', '¢', '£', '$', '¥', '#', '§', '¤', '‘', '“', '«', '←', '↑', '→', '↓',
	'°', '±', '²', '³', '×', 'µ', '¶', '·', '÷', '’', '”', '»', '¼', '½', '¾', '¿',
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	'―', '¹', '®', '©', '™', '♪', '¬', '¦', '⅛', '⅜', '⅝', '⅞', 0, 0, 0, 0,
	'Ω', 'Æ', 'Đ', 'ª', 'Ħ', 'Ĳ', 'Ŀ', 'Ł', 'Ø', 'Œ', 'º', 'Þ', 'Ŧ', 'Ŋ', 'ŉ', 0,
	'ĸ', 'æ', 'đ', 'ð', 'ħ', 'ı', 'ĳ', 'ŀ', 'ł', 'ø', 'œ', 'ß', 'þ', 'ŧ', 'ŋ', 0xAD,
}

// accents are the combining marks of C1h to CFh; 0 where there is none.
var accents = [15]rune{0x300, 0x301, 0x302, 0x303, 0x304, 0x306, 0x307, 0x308, 0, 0x30A, 0x327, 0x332, 0x30B, 0x328, 0x30C}

var latinOf = func() map[rune]byte {
	m := map[rune]byte{}
	for i, r := range latinHigh {
		if r != 0 {
			m[r] = byte(0xA0 + i)
		}
	}
	for c := 0x20; c < 0x7F; c++ {
		m[rune(c)] = byte(c)
	}
	// 24h is the currency sign; the dollar is A4h.
	m['$'], m['¤'] = 0xA4, 0x24
	return m
}()

var accentOf = func() map[rune]byte {
	m := map[rune]byte{}
	for i, r := range accents {
		if r != 0 {
			m[r] = byte(0xC1 + i)
		}
	}
	return m
}()

var otherTables = [5]*charmap.Charmap{nil, charmap.ISO8859_5, charmap.ISO8859_6, charmap.ISO8859_7, charmap.ISO8859_8}

// decodeText reads text bytes (no control codes) of a code table.
func decodeText(table byte, b []byte) (string, error) {
	var out strings.Builder
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c < 0x80 && c != 0x24:
			out.WriteByte(c)
		case c == 0x24:
			if table == 0 {
				out.WriteRune('¤')
			} else {
				out.WriteByte('$')
			}
		case table != 0:
			r := otherTables[table].DecodeByte(c)
			if r == utf8.RuneError {
				return "", fmt.Errorf("the byte %02Xh is no character of code table %02d", c, table)
			}
			out.WriteRune(r)
		case c >= 0xC1 && c <= 0xCF:
			mark := accents[c-0xC1]
			if mark == 0 || i+1 >= len(b) || b[i+1] < 0x20 || b[i+1] >= 0x80 {
				return "", fmt.Errorf("the accent %02Xh is not followed by a letter", c)
			}
			out.WriteString(norm.NFC.String(string(rune(b[i+1])) + string(mark)))
			i++
		default:
			r := latinHigh[c-0xA0]
			if c < 0xA0 || r == 0 {
				return "", fmt.Errorf("the byte %02Xh is no character of code table 00", c)
			}
			out.WriteRune(r)
		}
	}
	return out.String(), nil
}

// encodeText writes text in a code table; an error names a character the
// table does not have.
func encodeText(table byte, s string) ([]byte, error) {
	var out []byte
	for _, r := range s {
		if r == '\n' || r == '\r' {
			continue
		}
		if table != 0 {
			if r < 0x80 {
				out = append(out, byte(r))
				continue
			}
			if b, ok := otherTables[table].EncodeRune(r); ok && b >= 0xA0 {
				out = append(out, b)
				continue
			}
			return nil, fmt.Errorf("code table %02d has no %q", table, r)
		}
		if b, ok := latinOf[r]; ok {
			out = append(out, b)
			continue
		}
		// A letter with an accent: the accent, then the letter.
		d := []rune(norm.NFD.String(string(r)))
		if len(d) == 2 && d[0] < 0x80 {
			if a, ok := accentOf[d[1]]; ok {
				out = append(out, a, byte(d[0]))
				continue
			}
		}
		return nil, fmt.Errorf("code table 00 has no %q", r)
	}
	return out, nil
}

// languageCodes are the codes of the GSI block's language field
// (EBU Tech 3264, Appendix 3), by ISO 639-1 code.
var languageCodes = map[string]string{
	"sq": "01", "br": "02", "ca": "03", "hr": "04", "cy": "05", "cs": "06", "da": "07", "de": "08", "en": "09",
	"es": "0A", "eo": "0B", "et": "0C", "eu": "0D", "fo": "0E", "fr": "0F", "fy": "10", "ga": "11", "gd": "12",
	"gl": "13", "is": "14", "it": "15", "se": "16", "la": "17", "lv": "18", "lb": "19", "lt": "1A", "hu": "1B",
	"mt": "1C", "nl": "1D", "no": "1E", "nb": "1E", "nn": "1E", "oc": "1F", "pl": "20", "pt": "21", "ro": "22",
	"rm": "23", "sr": "24", "sk": "25", "sl": "26", "fi": "27", "sv": "28", "tr": "29",
	"zu": "45", "vi": "46", "uz": "47", "ur": "48", "uk": "49", "th": "4A", "te": "4B", "tt": "4C", "ta": "4D",
	"tg": "4E", "sw": "4F", "so": "51", "si": "52", "sn": "53", "ru": "56", "qu": "57", "ps": "58",
	"pa": "59", "fa": "5A", "or": "5C", "ne": "5D", "nr": "5E", "mr": "5F", "ms": "61", "mg": "62",
	"mk": "63", "lo": "64", "ko": "65", "km": "66", "kk": "67", "kn": "68", "ja": "69", "id": "6A", "hi": "6B",
	"he": "6C", "ha": "6D", "gn": "6E", "gu": "6F", "el": "70", "ka": "71", "ff": "72", "zh": "75", "my": "76",
	"bg": "77", "bn": "78", "be": "79", "bm": "7A", "az": "7B", "as": "7C", "hy": "7D", "ar": "7E", "am": "7F",
}

// languageCode is the GSI block's code of a language tag.
func languageCode(tag string) (string, bool) {
	if tag == "" {
		return "", false
	}
	primary := strings.ToLower(strings.SplitN(strings.ReplaceAll(tag, "_", "-"), "-", 2)[0])
	// Flemish has its own code.
	if strings.EqualFold(tag, "nl-BE") {
		return "2A", true
	}
	lc, ok := languageCodes[primary]
	return lc, ok
}
