package scc

import "fmt"

// The characters of CEA-608: the basic set (ASCII but for ten places), the
// special characters (11h 30h-3Fh) and the extended ones for Spanish,
// French and others (12h 20h-3Fh) and Portuguese, German and Danish
// (13h 20h-3Fh).

var basicDiffers = map[byte]rune{
	0x27: '’', 0x2A: 'á', 0x5C: 'é', 0x5E: 'í', 0x5F: 'ó', 0x60: 'ú',
	0x7B: 'ç', 0x7C: '÷', 0x7D: 'Ñ', 0x7E: 'ñ', 0x7F: '█',
}

func basic(b byte) rune {
	if r, ok := basicDiffers[b]; ok {
		return r
	}
	return rune(b)
}

// special has transparent space at 39h, read as a space.
var special = [16]rune{'®', '°', '½', '¿', '™', '¢', '£', '♪', 'à', ' ', 'è', 'â', 'ê', 'î', 'ô', 'û'}

var extended = [2][32]rune{
	{'Á', 'É', 'Ó', 'Ú', 'Ü', 'ü', '‘', '¡', '*', '\'', '—', '©', '℠', '•', '“', '”',
		'À', 'Â', 'Ç', 'È', 'Ê', 'Ë', 'ë', 'Î', 'Ï', 'ï', 'Ô', 'Ù', 'ù', 'Û', '«', '»'},
	{'Ã', 'ã', 'Í', 'Ì', 'ì', 'Ò', 'ò', 'Õ', 'õ', '{', '}', '\\', '^', '_', '|', '~',
		'Ä', 'ä', 'Ö', 'ö', 'ß', '¥', '¤', '¦', 'Å', 'å', 'Ø', 'ø', '┌', '┐', '└', '┘'},
}

// fallback is the basic letter sent before an extended one, for decoders
// that have only the basic set.
func fallback(r rune) byte {
	switch r {
	case 'Á', 'À', 'Â', 'Ã', 'Ä', 'Å':
		return 'A'
	case 'É', 'È', 'Ê', 'Ë':
		return 'E'
	case 'Í', 'Ì', 'Î', 'Ï':
		return 'I'
	case 'Ó', 'Ò', 'Ô', 'Õ', 'Ö', 'Ø':
		return 'O'
	case 'Ú', 'Ù', 'Û', 'Ü':
		return 'U'
	case 'ü', 'ù':
		return 'u'
	case 'ë':
		return 'e'
	case 'ì', 'ï':
		return 'i'
	case 'ò', 'ö', 'ø':
		return 'o'
	case 'ã', 'ä', 'å':
		return 'a'
	case 'Ç':
		return 'C'
	case 'ß':
		return 's'
	case '‘', '\'':
		return 0x27
	case '“', '”', '«', '»':
		return '"'
	case '—':
		return '-'
	case '{', '┌', '└':
		return '['
	case '}', '┐', '┘':
		return ']'
	case '\\', '|', '¦':
		return '/'
	}
	return ' '
}

var encodeTable = func() map[rune][]byte {
	m := map[rune][]byte{}
	for b := byte(0x20); b < 0x80; b++ {
		m[basic(b)] = []byte{b}
	}
	// The apostrophe is 27h too.
	m['\''] = []byte{0x27}
	for i, r := range special {
		if r != ' ' {
			m[r] = []byte{0x11, 0x30 + byte(i)}
		}
	}
	for set, rs := range extended {
		for i, r := range rs {
			if _, ok := m[r]; !ok || r == '\'' {
				if r == '\'' {
					continue
				}
				m[r] = []byte{0x12 + byte(set), 0x20 + byte(i), fallback(r)}
			}
		}
	}
	m[' '] = []byte{0x11, 0x39}
	return m
}()

// encode is how a character is sent: one basic byte, a special character's
// pair, or an extended character's pair and its basic fallback.
func encode(r rune) ([]byte, error) {
	if b, ok := encodeTable[r]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("closed captions (CEA-608) can't show %q", r)
}
