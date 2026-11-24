// Package fuzzy scores how similar two segments are, the way translators
// read a TM match percentage: 100 for an identical segment, lower values for
// segments that need more editing.
//
// The score is a weighted edit distance over tokens. Words and numbers
// weigh most, punctuation and inline tags less. What a changed token costs
// depends on how far apart the two are:
//
//   - the same word in another case, or written with other marks (an
//     accent left out, Arabic with and without its short vowels, Hebrew
//     with and without points), costs a fraction of a word;
//   - two forms of one word ("Mieter" and "Mieters", a Finnish case
//     ending, a Turkish suffix, a Korean particle) cost by the share of
//     their letters that differ, so a sentence in which only an ending
//     changed stays a close match;
//   - two numbers cost a quarter of a word, since a number is replaced,
//     not translated; the same number in other digits (Arabic-Indic,
//     Devanagari, full width) costs next to nothing;
//   - punctuation of the same kind in another form (straight and curly
//     quotes, a hyphen and a dash, a full-width or Arabic comma) costs a
//     fraction of a punctuation mark.
//
// Scripts written without spaces between words (Chinese, Japanese, Thai,
// Lao, Khmer, Burmese) are compared character by character, each with the
// marks written on it. Every script is composed first (NFC), so the same
// text scores the same however a file stores it.
package fuzzy

import (
	"github.com/4nonX/interlinea/internal/textnorm"
	"math"
	"unicode"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
)

// Kind classifies a token.
type Kind uint8

const (
	// Word is a run of letters and combining marks, or a single CJK ideograph or kana.
	Word Kind = iota + 1
	// Number is a run of digits, optionally with inner separators ("3.50", "1,000").
	Number
	// Punct is a single punctuation or symbol character.
	Punct
	// Tag is an inline tag; it compares by kind and type, not by ID.
	Tag
)

// Token is one comparable unit of a segment.
type Token struct {
	Kind Kind
	Text string
	// fold is the text without regard to case, width and the script of
	// digits; base is fold without marks and with letters that are written
	// several ways brought to one.
	fold string
	base string
	// perChar marks a character of a script written without spaces.
	perChar bool
}

// Weights of the token kinds, and what a difference of each degree costs
// relative to the token's weight.
const (
	weightWord   = 1.0
	weightNumber = 1.0
	weightPunct  = 0.5
	weightTag    = 0.5
	// The same word in another case.
	caseFactor = 0.2
	// The same word with other marks, or a letter written another way.
	markFactor = 0.15
	// Another number.
	numberFactor = 0.25
	// The same number in other digits, or with other separators.
	digitFactor = 0.1
	// Punctuation of the same kind in another form.
	punctFactor = 0.2
	// Two forms of a word cost formBase plus formSlope for each share of
	// letters that differs; from half the letters on they are two words.
	formBase  = 0.25
	formSlope = 1.5
	formLimit = 0.5
)

func (t Token) weight() float64 {
	switch t.Kind {
	case Punct:
		return weightPunct
	case Tag:
		return weightTag
	case Number:
		return weightNumber
	default:
		return weightWord
	}
}

// Tokenize splits content into tokens. Whitespace separates tokens and is
// otherwise ignored.
func Tokenize(c content.Content) []Token {
	var out []Token
	for _, r := range c {
		if r.IsTag() {
			key := r.Tag.Kind.String() + ":" + r.Tag.Type
			out = append(out, Token{Kind: Tag, Text: key, fold: key, base: key})
			continue
		}
		out = appendTextTokens(out, r.Text)
	}
	return out
}

// TokenizeText is Tokenize for plain text.
func TokenizeText(s string) []Token {
	return appendTextTokens(nil, s)
}

// isIdeographic reports whether r belongs to a script written without
// spaces between words: Chinese, Japanese, Thai, Lao, Khmer and Burmese.
// Each of their characters is a token of its own, so a sentence with one
// word changed still scores as a close match.
func isIdeographic(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) &&
		(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r))
}

func isWordRune(r rune) bool {
	return (unicode.IsLetter(r) || unicode.IsMark(r)) && !isIdeographic(r)
}

func appendTextTokens(out []Token, s string) []Token {
	// Composed first, so the same words score the same however a file
	// stores their accents.
	s = textnorm.NFC(s)
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case unicode.IsSpace(r):
			i += n
		case unicode.IsDigit(r):
			j := i + n
			for j < len(s) {
				r2, n2 := utf8.DecodeRuneInString(s[j:])
				if unicode.IsDigit(r2) {
					j += n2
					continue
				}
				if numberSeparator(r2) && j+n2 < len(s) {
					r3, _ := utf8.DecodeRuneInString(s[j+n2:])
					if unicode.IsDigit(r3) {
						j += n2
						continue
					}
				}
				break
			}
			w := s[i:j]
			f := textnorm.ASCIIDigits(w)
			out = append(out, Token{Kind: Number, Text: w, fold: f, base: digitsOnly(f)})
			i = j
		case isIdeographic(r):
			// A character with the vowel signs and tone marks written on it.
			j := i + n
			for j < len(s) {
				r2, n2 := utf8.DecodeRuneInString(s[j:])
				if !unicode.IsMark(r2) {
					break
				}
				j += n2
			}
			w := s[i:j]
			out = append(out, Token{Kind: Word, Text: w, fold: w, base: textnorm.StripMarks(w), perChar: true})
			i = j
		case isWordRune(r):
			j := i + n
			for j < len(s) {
				r2, n2 := utf8.DecodeRuneInString(s[j:])
				if isWordRune(r2) || unicode.IsDigit(r2) {
					j += n2
					continue
				}
				// Inner apostrophes belong to the word: "don't", "l'homme".
				if (r2 == '\'' || r2 == '’') && j+n2 < len(s) {
					r3, _ := utf8.DecodeRuneInString(s[j+n2:])
					if isWordRune(r3) {
						j += n2
						continue
					}
				}
				break
			}
			w := s[i:j]
			f := textnorm.Fold(textnorm.Compat(w))
			out = append(out, Token{Kind: Word, Text: w, fold: f, base: textnorm.Base(f)})
			i = j
		default:
			w := s[i : i+n]
			out = append(out, Token{Kind: Punct, Text: w, fold: w, base: punctClass(r)})
			i += n
		}
	}
	return out
}

// subCost is what it costs to turn token a into token b.
func subCost(a, b Token) float64 {
	if a.Kind == b.Kind && a.Text == b.Text {
		return 0
	}
	w := math.Max(a.weight(), b.weight())
	if a.Kind != b.Kind {
		return w
	}
	switch a.Kind {
	case Word:
		switch {
		case a.fold == b.fold:
			return caseFactor * w
		case a.base == b.base:
			return markFactor * w
		case a.perChar || b.perChar:
			return w
		}
		share := formDistance(a.base, b.base)
		if share >= formLimit {
			return w
		}
		return math.Min(1, formBase+formSlope*share) * w
	case Number:
		switch {
		case a.fold == b.fold, a.base == b.base:
			return digitFactor * w
		}
		return numberFactor * w
	case Punct:
		if a.base != "" && a.base == b.base {
			return punctFactor * w
		}
	}
	return w
}

// formDistance is the share of two words' letters that differ: their edit
// distance by the length of the longer. Korean syllables are compared by
// their letters, so a particle that changes one letter counts as one.
func formDistance(a, b string) float64 {
	ra, rb := letters(a), letters(b)
	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}
	if len(ra) == 0 {
		return 0
	}
	// Words whose lengths are too far apart cannot be forms of each other.
	if float64(len(ra)-len(rb)) >= formLimit*float64(len(ra)) {
		return 1
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			c := prev[j-1]
			if ra[i-1] != rb[j-1] {
				c++
			}
			if d := prev[j] + 1; d < c {
				c = d
			}
			if d := cur[j-1] + 1; d < c {
				c = d
			}
			cur[j] = c
		}
		prev, cur = cur, prev
	}
	return float64(prev[len(rb)]) / float64(len(ra))
}

// letters are the runes of a word; a Hangul syllable gives its two or
// three letters.
func letters(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			i := r - 0xAC00
			out = append(out, 0x1100+i/588, 0x1161+(i%588)/28)
			if t := i % 28; t != 0 {
				out = append(out, 0x11A7+t)
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// numberSeparator reports whether r groups or divides the digits of a
// number when it stands between two of them: the point and the comma, the
// Swiss apostrophe, the no-break spaces of French and SI style, and the
// Arabic decimal and thousands marks.
func numberSeparator(r rune) bool {
	switch r {
	case '.', ',', '\'', '’', '\u00a0', '\u202f', '\u2009', '٫', '٬':
		return true
	}
	return false
}

// digitsOnly is a number without its separators.
func digitsOnly(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b = append(b, s[i])
		}
	}
	return string(b)
}

// Punctuation that is the same mark in another form, by class.
var punctClasses = map[string]string{
	"quote":  "\"“”„‟«»‹›「」『』〝〞＂",
	"apos":   "'‘’‚‛ʼ＇′",
	"dash":   "-‐‑‒–—―−－﹣",
	"comma":  ",，、،﹐",
	"stop":   ".。．｡۔",
	"colon":  ":：",
	"semi":   ";；؛",
	"quest":  "?？؟;",
	"excl":   "!！",
	"open":   "(（",
	"close":  ")）",
	"sopen":  "[［【",
	"sclose": "]］】",
	"pct":    "%％٪",
	"slash":  "/／",
	"amp":    "&＆",
}

var punctOf = func() map[rune]string {
	m := map[rune]string{}
	for class, marks := range punctClasses {
		for _, r := range marks {
			m[r] = class
		}
	}
	return m
}()

// punctClass names the kind of a punctuation mark, or "" for a mark that
// has no other form.
func punctClass(r rune) string { return punctOf[r] }

// Distance returns the weighted edit distance between two token sequences.
func Distance(a, b []Token) float64 {
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]float64, len(b)+1)
	cur := make([]float64, len(b)+1)
	for j := 1; j <= len(b); j++ {
		prev[j] = prev[j-1] + b[j-1].weight()
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = prev[0] + a[i-1].weight()
		for j := 1; j <= len(b); j++ {
			del := prev[j] + a[i-1].weight()
			ins := cur[j-1] + b[j-1].weight()
			sub := prev[j-1] + subCost(a[i-1], b[j-1])
			cur[j] = math.Min(sub, math.Min(del, ins))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func total(ts []Token) float64 {
	var s float64
	for _, t := range ts {
		s += t.weight()
	}
	return s
}

// Score returns the match percentage (0 to 100) between two segments.
// 100 is reserved for segments with identical text and identical tags
// (compared by kind and type); any difference yields at most 99.
func Score(a, b content.Content) int {
	if textnorm.NFC(a.Plain()) == textnorm.NFC(b.Plain()) && sameTagShape(a, b) {
		return 100
	}
	return score(Tokenize(a), Tokenize(b))
}

// ScoreText is Score for plain text.
func ScoreText(a, b string) int {
	if textnorm.NFC(a) == textnorm.NFC(b) {
		return 100
	}
	return score(TokenizeText(a), TokenizeText(b))
}

func score(ta, tb []Token) int {
	m := math.Max(total(ta), total(tb))
	if m == 0 {
		// Both segments are whitespace only but differ in it.
		return 99
	}
	s := int(math.Floor((1 - Distance(ta, tb)/m) * 100))
	if s >= 100 {
		s = 99
	}
	if s < 0 {
		s = 0
	}
	return s
}

func sameTagShape(a, b content.Content) bool {
	ta, tb := a.Tags(), b.Tags()
	if len(ta) != len(tb) {
		return false
	}
	for i := range ta {
		if ta[i].Kind != tb[i].Kind || ta[i].Type != tb[i].Type {
			return false
		}
	}
	return true
}
