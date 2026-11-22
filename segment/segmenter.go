package segment

import (
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
)

// window limits how much text around a position is examined when a rule is
// checked. SRX rules look at abbreviations and punctuation, so 64
// bytes are ample and keep segmentation linear in the text length.
const window = 64

// Span is a segment as byte offsets into the segmented text. Text between
// spans (inter-segment whitespace) is not part of any segment.
type Span struct {
	Start, End int
}

// Segmenter applies the rules for one language.
type Segmenter struct {
	rules []Rule
}

// New returns a segmenter for lang using rules. A nil rules value selects
// Default(). Languages without any matching map get no breaks except those
// of the rules themselves, so callers normally keep a catch-all map.
func New(rules *Rules, lang string) *Segmenter {
	if rules == nil {
		rules = Default()
	}
	return &Segmenter{rules: rules.For(lang)}
}

// FromRules returns a segmenter for an explicit rule list.
func FromRules(rules []Rule) *Segmenter {
	return &Segmenter{rules: rules}
}

// Breaks returns the sorted byte offsets in text where a segment ends.
// Offsets 0 and len(text) are never included.
func (s *Segmenter) Breaks(text string) []int {
	if text == "" || len(s.rules) == 0 {
		return nil
	}
	cands := s.candidates(text)
	var out []int
	for _, p := range cands {
		if s.decide(text, p) {
			out = append(out, p)
		}
	}
	return out
}

// candidates collects every position where at least one break rule could
// apply. Only these positions are then evaluated against the full ordered
// rule list, which keeps the cost proportional to the number of punctuation
// marks rather than to the text length.
func (s *Segmenter) candidates(text string) []int {
	set := map[int]struct{}{}
	add := func(p int) {
		if p > 0 && p < len(text) && utf8.RuneStart(text[p]) {
			set[p] = struct{}{}
		}
	}
	for i := range s.rules {
		r := &s.rules[i]
		if !r.Break {
			continue
		}
		if r.scan != nil {
			for _, re := range [...]interface {
				FindStringIndex(string) []int
			}{r.scan, r.longst} {
				for off := 0; off < len(text); {
					loc := re.FindStringIndex(text[off:])
					if loc == nil {
						break
					}
					add(off + loc[1])
					off += loc[0] + runeLen(text[off+loc[0]:])
				}
			}
			continue
		}
		// Rule without Before: any position where After starts is a candidate.
		for off := 0; off < len(text); {
			loc := r.scanA.FindStringIndex(text[off:])
			if loc == nil {
				break
			}
			add(off + loc[0])
			off += loc[0] + runeLen(text[off+loc[0]:])
		}
	}
	out := make([]int, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// decide evaluates the ordered rules at position p; the first matching rule wins.
func (s *Segmenter) decide(text string, p int) bool {
	lo := p - window
	if lo < 0 {
		lo = 0
	}
	for lo < p && !utf8.RuneStart(text[lo]) {
		lo++
	}
	hi := p + window
	if hi > len(text) {
		hi = len(text)
	}
	for hi > p && hi < len(text) && !utf8.RuneStart(text[hi]) {
		hi--
	}
	before, after := reverseString(text[lo:p]), text[p:hi]
	for i := range s.rules {
		r := &s.rules[i]
		if r.before != nil && !r.before.MatchString(before) {
			continue
		}
		if r.after != nil && !r.after.MatchString(after) {
			continue
		}
		return r.Break
	}
	return false
}

// Split segments text and returns the segment spans. Whitespace at the edges
// of each segment is left outside the span; empty segments are dropped.
func (s *Segmenter) Split(text string) []Span {
	bounds := append([]int{0}, s.Breaks(text)...)
	bounds = append(bounds, len(text))
	var out []Span
	for i := 0; i+1 < len(bounds); i++ {
		st, en := trimSpan(text, bounds[i], bounds[i+1])
		if st < en {
			out = append(out, Span{Start: st, End: en})
		}
	}
	return out
}

// Strings segments text and returns the segment texts.
func (s *Segmenter) Strings(text string) []string {
	spans := s.Split(text)
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = text[sp.Start:sp.End]
	}
	return out
}

// SplitContent segments tagged content. Break positions are computed on the
// plain text; tags at a boundary follow the rules of content.Slice, so an
// opening tag starts the next segment and a closing tag ends the previous one.
func (s *Segmenter) SplitContent(c content.Content) []content.Content {
	plain := c.Plain()
	spans := s.Split(plain)
	out := make([]content.Content, 0, len(spans))
	for _, sp := range spans {
		out = append(out, c.Slice(sp.Start, sp.End))
	}
	return out
}

func trimSpan(text string, st, en int) (int, int) {
	for st < en {
		r, n := utf8.DecodeRuneInString(text[st:en])
		if !unicode.IsSpace(r) {
			break
		}
		st += n
	}
	for en > st {
		r, n := utf8.DecodeLastRuneInString(text[st:en])
		if !unicode.IsSpace(r) {
			break
		}
		en -= n
	}
	return st, en
}

func runeLen(s string) int {
	if s == "" {
		return 1
	}
	_, n := utf8.DecodeRuneInString(s)
	return n
}

func reverseString(s string) string {
	out := make([]byte, len(s))
	w := len(s)
	for i := 0; i < len(s); {
		_, n := utf8.DecodeRuneInString(s[i:])
		w -= n
		copy(out[w:], s[i:i+n])
		i += n
	}
	return string(out)
}
