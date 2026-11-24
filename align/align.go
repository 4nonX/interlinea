// Package align pairs the segments of a document with those of its
// translation, so translations done before can fill a translation memory.
//
// The method is the length-based alignment of Gale and Church (1993): a
// sentence and its translation are about as long as each other, up to the
// ratio of the two languages, so the cheapest way through both texts that
// pairs one segment with one, two with one, one with two, two with two, or
// skips one, follows the translation. Numbers both sides share pull a pair
// together and numbers only one side has push it apart, which holds the
// path where the lengths alone would drift. Where both documents have the
// same paragraphs, segments are only paired within a paragraph.
package align

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/internal/textnorm"
)

// Segment is one segment of a document, with the paragraph it is in.
type Segment struct {
	Block   string          `json:"block,omitempty"`
	Content content.Content `json:"content"`
}

// Pair is a stretch of the source with its translation. A stretch nothing
// was found for has an empty other side.
type Pair struct {
	Source content.Content `json:"source"`
	Target content.Content `json:"target"`
	// SourceSegments and TargetSegments are the segments joined on each
	// side, by their position in the documents.
	SourceSegments []int `json:"sourceSegments"`
	TargetSegments []int `json:"targetSegments"`
	// Score says how well the two sides fit, from 0 to 100: their lengths
	// and their numbers. Pairs below about 50 deserve a look.
	Score int `json:"score"`
}

// Costs of a pairing besides its lengths, as Gale and Church weigh them
// (minus 100 times the logarithm of how often each occurs).
const (
	skipCost  = 450
	mergeCost = 230
	bothCost  = 440
	// What agreeing and disagreeing numbers are worth.
	sameNumbers  = -150
	otherNumbers = 250
	lostNumbers  = 90
	// The variance of a translation's length per character of source.
	variance = 6.8
)

type item struct {
	length  int
	numbers []string
}

func measure(segs []Segment) []item {
	out := make([]item, len(segs))
	for i, s := range segs {
		text := s.Content.Plain()
		out[i] = item{length: utf8.RuneCountInString(strings.TrimSpace(text)), numbers: numbers(text)}
	}
	return out
}

// numbers returns the numbers of a text as digit strings, in whatever
// digits and with whatever separators they are written, sorted.
func numbers(text string) []string {
	text = textnorm.ASCIIDigits(text)
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	rs := []rune(text)
	for i, r := range rs {
		switch {
		case r >= '0' && r <= '9':
			cur.WriteRune(r)
		case separator(r) && cur.Len() > 0 && i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '9':
			// A separator inside a number: 1.250,00 and 1,250.00 are the same digits.
			if r == ' ' {
				// A plain space separates thousands only before three digits.
				if !(i+3 < len(rs) && unicode.IsDigit(rs[i+2]) && unicode.IsDigit(rs[i+3]) && (i+4 >= len(rs) || !unicode.IsDigit(rs[i+4]))) {
					flush()
				}
			}
		default:
			flush()
		}
	}
	flush()
	sort.Strings(out)
	return out
}

// separator reports whether a character groups the digits of a number or
// marks its decimals, in any of the ways numbers are written.
func separator(r rune) bool {
	switch r {
	case '.', ',', '\'', ' ', 0x00a0, 0x202f, 0x2009, 0x066b, 0x066c:
		return true
	}
	return false
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func shares(a, b []string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			return true
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return false
}

// lengthCost is how unlikely a translation of l2 characters is for a
// source of l1, with ratio characters of translation per source character.
func lengthCost(l1, l2 int, ratio float64) float64 {
	if l1 == 0 && l2 == 0 {
		return 0
	}
	mean := (float64(l1) + float64(l2)/ratio) / 2
	delta := (float64(l2) - float64(l1)*ratio) / math.Sqrt(mean*variance*ratio)
	// Twice the upper tail of the normal distribution.
	p := math.Erfc(math.Abs(delta) / math.Sqrt2)
	if p < 1e-30 {
		p = 1e-30
	}
	return -100 * math.Log(p)
}

func numberCost(a, b []string) float64 {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0 || len(b) == 0:
		return lostNumbers
	case sameSet(a, b):
		return sameNumbers
	case shares(a, b):
		return 0
	}
	return otherNumbers
}

func join(items []item, from, n int) item {
	var out item
	for i := from; i < from+n; i++ {
		out.length += items[i].length
		out.numbers = append(out.numbers, items[i].numbers...)
	}
	if n > 1 {
		sort.Strings(out.numbers)
	}
	return out
}

type bead struct{ ds, dt int }

var beads = []bead{{1, 1}, {1, 0}, {0, 1}, {2, 1}, {1, 2}, {2, 2}}

func beadCost(src, tgt []item, i, j int, b bead, ratio float64) float64 {
	s, t := join(src, i, b.ds), join(tgt, j, b.dt)
	c := lengthCost(s.length, t.length, ratio)
	switch {
	case b.ds == 0 || b.dt == 0:
		// What is skipped costs the same whatever its length: a long
		// sentence is as easily left out as a short one.
		return skipCost
	case b.ds+b.dt == 3:
		c += mergeCost
	case b.ds == 2 && b.dt == 2:
		c += bothCost
	}
	return c + numberCost(s.numbers, t.numbers)
}

type step struct {
	si, ti int
	bead   bead
	cost   float64
}

// path finds the cheapest pairing of two runs of segments.
func path(src, tgt []item, ratio float64) []step {
	n, m := len(src), len(tgt)
	// A band around the diagonal keeps long documents in reach: the
	// translation is never far from where the source is.
	band := m + 1
	if n*m > 4_000_000 {
		band = 200
		if d := abs(n-m) + 100; d > band {
			band = d
		}
	}
	const inf = math.MaxFloat64
	width := m + 1
	cost := make([]float64, (n+1)*width)
	from := make([]int8, (n+1)*width)
	for k := range cost {
		cost[k] = inf
	}
	cost[0] = 0
	for i := 0; i <= n; i++ {
		lo, hi := 0, m
		if band <= m {
			mid := 0
			if n > 0 {
				mid = i * m / n
			}
			if lo = mid - band; lo < 0 {
				lo = 0
			}
			if hi = mid + band; hi > m {
				hi = m
			}
		}
		for j := lo; j <= hi; j++ {
			here := cost[i*width+j]
			if here == inf {
				continue
			}
			for k, b := range beads {
				if i+b.ds > n || j+b.dt > m {
					continue
				}
				c := here + beadCost(src, tgt, i, j, b, ratio)
				at := (i+b.ds)*width + j + b.dt
				if c < cost[at] {
					cost[at] = c
					from[at] = int8(k)
				}
			}
		}
	}
	if cost[n*width+m] == inf {
		// Outside the band: pair what there is in order.
		var out []step
		for i, j := 0, 0; i < n || j < m; {
			b := bead{1, 1}
			if i >= n {
				b = bead{0, 1}
			} else if j >= m {
				b = bead{1, 0}
			}
			out = append(out, step{i, j, b, beadCost(src, tgt, i, j, b, ratio)})
			i += b.ds
			j += b.dt
		}
		return out
	}
	var out []step
	for i, j := n, m; i > 0 || j > 0; {
		b := beads[from[i*width+j]]
		i -= b.ds
		j -= b.dt
		out = append(out, step{i, j, b, cost[(i+b.ds)*width+j+b.dt] - cost[i*width+j]})
	}
	for a, z := 0, len(out)-1; a < z; a, z = a+1, z-1 {
		out[a], out[z] = out[z], out[a]
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ratio is how many characters of translation there are per character of
// source, over the whole of both documents.
func ratio(src, tgt []item) float64 {
	s, t := 0, 0
	for _, x := range src {
		s += x.length
	}
	for _, x := range tgt {
		t += x.length
	}
	if s == 0 || t == 0 {
		return 1
	}
	r := float64(t) / float64(s)
	// Two documents that are not each other's translation give no ratio
	// to go by.
	if r < 0.2 || r > 5 {
		return 1
	}
	return r
}

// blocks returns the paragraphs of a document as ranges of its segments,
// or nothing where a segment does not say which paragraph it is in.
func blocks(segs []Segment) [][2]int {
	var out [][2]int
	for i := range segs {
		if segs[i].Block == "" {
			return nil
		}
		if i == 0 || segs[i].Block != segs[i-1].Block {
			out = append(out, [2]int{i, i + 1})
		} else {
			out[len(out)-1][1] = i + 1
		}
	}
	return out
}

func joined(segs []Segment, from, n int) (content.Content, []int) {
	var c content.Content
	idx := []int{}
	for i := from; i < from+n; i++ {
		if i > from {
			// Languages written without spaces join without one.
			if a, b := c.Plain(), segs[i].Content.Plain(); a != "" && b != "" && !spaceless(lastRune(a)) && !spaceless(firstRune(b)) {
				c = c.Append(" ")
			}
		}
		for _, r := range segs[i].Content {
			if r.Tag != nil {
				c = c.AppendTag(*r.Tag)
			} else {
				c = c.Append(r.Text)
			}
		}
		idx = append(idx, i)
	}
	if c == nil {
		c = content.Content{}
	}
	return c, idx
}

func lastRune(s string) rune  { r, _ := utf8.DecodeLastRuneInString(s); return r }
func firstRune(s string) rune { r, _ := utf8.DecodeRuneInString(s); return r }

func spaceless(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) || unicode.IsSpace(r)
}

// score turns the cost of a pairing into 0 to 100.
func score(st step) int {
	if st.bead.ds == 0 || st.bead.dt == 0 {
		return 0
	}
	c := st.cost
	switch {
	case st.bead.ds+st.bead.dt == 3:
		c -= mergeCost
	case st.bead.ds == 2 && st.bead.dt == 2:
		c -= bothCost
	}
	// Agreeing numbers lift a pair, a plain fit of the lengths is about
	// 85, and a fit as unlikely as a skipped segment is 0.
	s := 85 - c*85/skipCost
	if s > 100 {
		s = 100
	}
	if s < 1 {
		s = 1
	}
	return int(math.Round(s))
}

// Align pairs the segments of a document with those of its translation,
// in the order of the documents. Every segment of both is in exactly one
// pair.
func Align(source, target []Segment) []Pair {
	src, tgt := measure(source), measure(target)
	r := ratio(src, tgt)
	out := []Pair{}
	add := func(so, to int, steps []step) {
		for _, st := range steps {
			var p Pair
			p.Source, p.SourceSegments = joined(source, so+st.si, st.bead.ds)
			p.Target, p.TargetSegments = joined(target, to+st.ti, st.bead.dt)
			p.Score = score(st)
			out = append(out, p)
		}
	}
	sb, tb := blocks(source), blocks(target)
	if len(sb) == len(tb) && len(sb) > 1 {
		// The same paragraphs on both sides: a sentence stays in its own.
		for k := range sb {
			add(sb[k][0], tb[k][0], path(src[sb[k][0]:sb[k][1]], tgt[tb[k][0]:tb[k][1]], r))
		}
		return out
	}
	add(0, 0, path(src, tgt, r))
	return out
}

// Result is an alignment with the segments it was made of, for a review
// that joins and splits them otherwise.
type Result struct {
	// Source and Target are the segments of the two documents, which the
	// pairs name by their position.
	Source []content.Content `json:"source"`
	Target []content.Content `json:"target"`
	// Pairs are the segments of the document with their translations, in
	// the order of the documents. A pair with an empty side is a segment
	// nothing was found for; a score under 50 deserves a look.
	Pairs []Pair `json:"pairs"`
}

// Documents aligns two documents and keeps their segments with the pairs.
func Documents(source, target []Segment) Result {
	r := Result{Source: make([]content.Content, len(source)), Target: make([]content.Content, len(target)), Pairs: Align(source, target)}
	for i, s := range source {
		r.Source[i] = s.Content
	}
	for i, s := range target {
		r.Target[i] = s.Content
	}
	return r
}
