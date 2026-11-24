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
