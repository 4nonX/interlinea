package spell

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// capType is Hunspell's capitalization type of a word.
type capType int

const (
	capNo      capType = iota // all lower case, or no cased letters
	capInit                   // first letter upper case, the rest lower
	capAll                    // all upper case
	capHuhInit                // mixed, first letter upper case
	capHuh                    // mixed, first letter lower case
)

func (a *aff) isUpper(r rune) bool {
	if a.turkic && r == 'İ' {
		return true
	}
	return unicode.IsUpper(r) || unicode.IsTitle(r)
}

// cased reports whether the letter has an upper and a lower case form.
func cased(r rune) bool { return unicode.ToLower(r) != r || unicode.ToUpper(r) != r }

// guess classifies the word as Hunspell does: by the number of upper
// case letters and of letters without case.
func (a *aff) guess(w string) capType {
	if w == "" {
		return capNo
	}
	ncap, nneutral, n := 0, 0, 0
	first := false
	for i, r := range w {
		n++
		if a.isUpper(r) {
			ncap++
			if i == 0 {
				first = true
			}
		} else if !cased(r) || (a.checkSharps && r == 'ß') {
			nneutral++
		}
	}
	switch {
	case ncap == 0:
		return capNo
	case ncap == 1 && first:
		return capInit
	case ncap == n || ncap+nneutral == n:
		return capAll
	case ncap > 1 && first:
		return capHuhInit
	default:
		return capHuh
	}
}

func (a *aff) lowerOne(w string) string {
	if a.turkic {
		w = strings.Map(func(r rune) rune {
			switch r {
			case 'I':
				return 'ı'
			case 'İ':
				return 'i'
			}
			return r
		}, w)
	}
	return strings.ToLower(w)
}

// lower lists the lower-case forms of a word: one, except with
// CHECKSHARPS, where "SS" may stand for "ß" ("STRASSE" is "straße" or
// "strasse").
func (a *aff) lower(w string) []string {
	// Outside Turkic languages, a dotted capital I has no lower case
	// form that a dictionary would hold.
	if !a.turkic && strings.HasPrefix(w, "İ") {
		return nil
	}
	l := a.lowerOne(w)
	if a.checkSharps && strings.Contains(w, "SS") {
		return append(sharpVariants(l, 0), l)
	}
	return []string{l}
}

func sharpVariants(s string, start int) []string {
	pos := strings.Index(s[start:], "ss")
	if pos < 0 {
		return nil
	}
	pos += start
	replaced := s[:pos] + "ß" + s[pos+2:]
	out := []string{replaced}
	out = append(out, sharpVariants(replaced, pos+len("ß"))...)
	out = append(out, sharpVariants(s, pos+2)...)
	return out
}

func (a *aff) upper(w string) string {
	if a.turkic {
		w = strings.Map(func(r rune) rune {
			switch r {
			case 'i':
				return 'İ'
			case 'ı':
				return 'I'
			}
			return r
		}, w)
	}
	return strings.Map(func(r rune) rune {
		if r == 'ß' {
			return r
		}
		return unicode.ToUpper(r)
	}, w)
}

func (a *aff) capitalize(w string) []string {
	r, size := utf8.DecodeRuneInString(w)
	if size == len(w) {
		return []string{a.upper(w)}
	}
	var out []string
	head := a.upper(string(r))
	for _, l := range a.lower(w[size:]) {
		out = append(out, head+l)
	}
	return out
}

func (a *aff) lowerFirst(w string) string {
	r, size := utf8.DecodeRuneInString(w)
	return a.lowerOne(string(r)) + w[size:]
}

func (a *aff) upperFirst(w string) string {
	r, size := utf8.DecodeRuneInString(w)
	return a.upper(string(r)) + w[size:]
}

// variants are the forms a correctly spelled word may have in the
// dictionary ("Kitten" may be "kitten").
func (a *aff) variants(w string) (capType, []string) {
	c := a.guess(w)
	switch c {
	case capInit:
		return c, append([]string{w}, a.lower(w)...)
	case capHuhInit:
		if !a.turkic && strings.HasPrefix(w, "İ") {
			return c, []string{w}
		}
		return c, []string{w, a.lowerFirst(w)}
	case capAll:
		out := []string{w}
		out = append(out, a.capitalize(w)...)
		out = append(out, a.lower(w)...)
		// Catalan, French, Italian: SANT'ELIA is Sant'Elia.
		if i := strings.IndexAny(w, "'’"); i > 0 && i < len(w)-1 {
			_, size := utf8.DecodeRuneInString(w[i:])
			low := a.lowerOne(w)
			li := strings.IndexAny(low, "'’")
			if li > 0 {
				p1, p2 := low[:li+size], low[li+size:]
				if p2 != "" {
					out = append(out, p1+a.upperFirst(p2), a.upperFirst(p1)+a.upperFirst(p2))
				}
			}
		}
		return c, out
	default:
		return c, []string{w}
	}
}

// corrections are the forms to try when suggesting for a misspelling.
func (a *aff) corrections(w string) (capType, []string) {
	c := a.guess(w)
	switch c {
	case capInit:
		return c, append([]string{w}, a.lower(w)...)
	case capHuhInit:
		out := []string{w, a.lowerFirst(w)}
		out = append(out, a.lower(w)...)
		out = append(out, a.capitalize(w)...)
		return c, out
	case capHuh:
		return c, append([]string{w}, a.lower(w)...)
	case capAll:
		out := []string{w}
		out = append(out, a.lower(w)...)
		out = append(out, a.capitalize(w)...)
		return c, out
	default:
		return c, []string{w}
	}
}

// coerce gives a suggestion the capitalization of the misspelling.
func (a *aff) coerce(w string, c capType) string {
	switch c {
	case capInit, capHuhInit:
		return a.upperFirst(w)
	case capAll:
		return a.upper(w)
	}
	return w
}
