// Package textnorm makes text comparable across the ways the same words
// are stored: composed or decomposed accents (the é of a Windows file and
// the e with a combining accent of a macOS one), and upper or lower case
// in every script.
package textnorm

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NFC returns s with accents composed (Unicode Normalization Form C). Text
// that is already composed, which is nearly all of it, is returned as it is.
func NFC(s string) string {
	if norm.NFC.IsNormalString(s) {
		return s
	}
	return norm.NFC.String(s)
}

// Fold returns s composed and in lower case, for comparing without regard
// to case. The Turkish capital İ folds to i, as a search for "istanbul"
// expects, and the Greek final sigma to the ordinary one.
func Fold(s string) string {
	s = strings.ToLower(NFC(s))
	if strings.ContainsAny(s, "̇ς") {
		// strings.ToLower turns İ into i followed by a combining dot.
		s = strings.ReplaceAll(s, "i̇", "i")
		s = strings.ReplaceAll(s, "ς", "σ")
	}
	return s
}

// StripMarks removes the marks written on or under letters: accents, the
// Arabic short vowels, the Hebrew points. It also removes the Arabic
// tatweel, which only stretches a word. Use it where a word is to be
// found whether or not it is written with them.
func StripMarks(s string) string {
	plain := true
	for _, r := range s {
		if r > unicode.MaxASCII {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) || r == 'ـ' {
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// Compat returns s with characters that are another form of the same one
// brought to the usual form (Unicode NFKC): full-width Latin letters and
// half-width katakana, ligatures, the presentation forms of Arabic.
func Compat(s string) string {
	if norm.NFKC.IsNormalString(s) {
		return s
	}
	return norm.NFKC.String(s)
}

// Letters that are written in several ways, each brought to one: the
// Arabic alef with and without hamza, the Arabic and the Persian yeh and
// kaf, the German sharp s, and the Turkish dotless i, which capitals
// written without regard to the language turn into an ordinary one.
var variants = strings.NewReplacer(
	"أ", "ا", "إ", "ا", "آ", "ا", "ٱ", "ا",
	"ى", "ي", "ی", "ي", "ک", "ك",
	"ß", "ss",
	"ı", "i",
)

// Base returns a folded word without the marks written on its letters and
// with letters that are written in several ways brought to one, so that
// two spellings of a word compare equal.
func Base(folded string) string {
	plain := true
	for i := 0; i < len(folded); i++ {
		if folded[i] >= 0x80 {
			plain = false
			break
		}
	}
	if plain {
		return folded
	}
	return variants.Replace(StripMarks(folded))
}

// ASCIIDigits returns s with the digits of every script as 0 to 9, so a
// number compares equal whichever digits it is written in.
func ASCIIDigits(s string) string {
	plain := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r > unicode.MaxASCII && unicode.IsDigit(r) {
			b.WriteByte(byte('0' + digitValue(r)))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// digitValue is the value of a decimal digit of any script: the digits of
// a script are ten code points in a row, from zero.
func digitValue(r rune) int {
	for _, rg := range unicode.Nd.R16 {
		if rune(rg.Lo) <= r && r <= rune(rg.Hi) {
			return int(r-rune(rg.Lo)) % 10
		}
	}
	for _, rg := range unicode.Nd.R32 {
		if rune(rg.Lo) <= r && r <= rune(rg.Hi) {
			return int(r-rune(rg.Lo)) % 10
		}
	}
	return 0
}
