// Package fuzzy scores how similar two segments are, the way translators
// read a TM match percentage: 100 for an identical segment, lower values for
// segments that need more editing.
//
// The score is a weighted, token-based edit distance. Words and numbers weigh
// most, punctuation and inline tags less, and case-only differences very
// little, so "Der Mieter haftet." against "der Mieter haftet." scores high
// while a changed word costs a full token.
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
	fold string
}

// Weights of the token kinds and of a case-only difference (relative to the token weight).
const (
	weightWord   = 1.0
	weightNumber = 1.0
	weightPunct  = 0.5
	weightTag    = 0.5
	caseFactor   = 0.2
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
			out = append(out, Token{Kind: Tag, Text: key, fold: key})
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
			out = append(out, Token{Kind: Word, Text: w, fold: textnorm.NFC(w)})
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
			out = append(out, Token{Kind: Word, Text: w, fold: textnorm.Fold(w)})
			i = j
		case unicode.IsDigit(r):
			j := i + n
			for j < len(s) {
				r2, n2 := utf8.DecodeRuneInString(s[j:])
				if unicode.IsDigit(r2) {
					j += n2
					continue
				}
				if (r2 == '.' || r2 == ',') && j+n2 < len(s) {
					r3, _ := utf8.DecodeRuneInString(s[j+n2:])
					if unicode.IsDigit(r3) {
						j += n2
						continue
					}
				}
				break
			}
			w := s[i:j]
			out = append(out, Token{Kind: Number, Text: w, fold: w})
			i = j
		default:
			w := s[i : i+n]
			out = append(out, Token{Kind: Punct, Text: w, fold: w})
			i += n
		}
	}
	return out
}

func subCost(a, b Token) float64 {
	if a.Kind == b.Kind && a.Text == b.Text {
		return 0
	}
	wa, wb := a.weight(), b.weight()
	w := math.Max(wa, wb)
	if a.Kind == b.Kind && a.fold == b.fold {
		return caseFactor * w
	}
	return w
}

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
