package spell

import (
	"sort"
	"strings"
	"unicode"
)

const maxCharDistance = 4

// Suggest returns up to max corrections for a word, best first.
func (d *Dictionary) Suggest(word string, max int) []string {
	word = strings.TrimSpace(word)
	if word == "" || max <= 0 {
		return nil
	}
	s := &suggester{d: d, word: word, seen: map[string]bool{}, max: max}
	s.run()
	return s.out
}

type suggester struct {
	d    *Dictionary
	word string
	c    capType
	seen map[string]bool
	out  []string
	max  int
	// compoundFound counts the suggestions from the compound pass.
	compoundFound int
}

func (s *suggester) full() bool { return len(s.out) >= s.max }

func (s *suggester) good(w string, compounds bool) bool {
	found := false
	s.d.goodForms(w, false, false, !compounds, compounds, func() bool {
		found = true
		return false
	})
	return found
}

func (s *suggester) goodAny(w string) bool {
	found := false
	s.d.goodForms(w, false, false, true, true, func() bool {
		found = true
		return false
	})
	return found
}

func (s *suggester) forbidden(w string) bool {
	return s.d.a.forbidden != 0 && s.d.hasFlag(w, s.d.a.forbidden, false)
}

// handle adds a found suggestion with the misspelling's capitalization.
func (s *suggester) handle(text string, checkInclusion bool) bool {
	a := s.d.a
	orig := text
	if !(a.keepcase != 0 && s.d.hasFlag(text, a.keepcase, false) && !(a.checkSharps && strings.Contains(text, "ß"))) {
		text = a.coerce(text, s.c)
		if text != orig && s.forbidden(text) {
			text = orig
		}
		if (s.c == capHuh || s.c == capHuhInit) && strings.Contains(text, " ") {
			// Keep the case the user typed at the split point.
			tr, wr := []rune(text), []rune(s.word)
			if pos := runeIndex(tr, ' '); pos >= 0 && pos+1 < len(tr) && pos < len(wr) {
				if tr[pos+1] != wr[pos] && unicode.ToUpper(tr[pos+1]) == wr[pos] {
					tr[pos+1] = wr[pos]
					text = string(tr)
				}
			}
		}
	}
	if s.forbidden(text) {
		return false
	}
	text = a.oconv.apply(text)
	if s.seen[text] {
		return false
	}
	if checkInclusion {
		lt := strings.ToLower(text)
		for prev := range s.seen {
			if strings.Contains(lt, strings.ToLower(prev)) {
				return false
			}
		}
	}
	s.seen[text] = true
	s.out = append(s.out, text)
	return true
}

func runeIndex(rs []rune, r rune) int {
	for i, x := range rs {
		if x == r {
			return i
		}
	}
	return -1
}

func (s *suggester) run() {
	a := s.d.a
	var variants []string
	s.c, variants = a.corrections(s.word)
	if a.forceUCase != 0 && s.c == capNo {
		for _, cap := range a.capitalize(s.word) {
			if s.goodAny(cap) {
				s.handle(cap, false)
				return
			}
		}
	}
	// "permanent.Vacation": a missing space after a full stop.
	if rs := []rune(s.word); len(rs) > 2 {
		for i := 1; i < len(rs)-1; i++ {
			if rs[i] == '.' && unicode.IsLetter(rs[i-1]) && unicode.IsLetter(rs[i+1]) {
				left, right := string(rs[:i]), string(rs[i+1:])
				if s.d.Spell(left) && s.d.Spell(right) {
					s.add(left + ". " + right)
				}
			}
		}
	}
	goodEdit := false
	for i, v := range variants {
		if s.full() {
			break
		}
		if i > 0 && s.goodAny(v) {
			s.handle(v, false)
		}
		if s.edits(v) {
			goodEdit = true
		}
	}
	if !goodEdit && !s.full() && a.maxNgramSugs > 0 {
		n := 0
		for _, g := range s.ngram() {
			if s.full() || n >= a.maxNgramSugs {
				break
			}
			if s.handle(g, true) {
				n++
			}
		}
	}
	if !goodEdit && !s.full() && len(a.phone) > 0 {
		n := 0
		for _, g := range s.phonet() {
			if s.full() || n >= 2 {
				break
			}
			if s.handle(g, true) {
				n++
			}
		}
	}
	// "Afo-American": suggest for the misspelled part of a hyphenated word.
	if strings.Contains(s.word, "-") && !s.full() {
		dashed := false
		for _, t := range s.out {
			if strings.Contains(t, "-") {
				dashed = true
			}
		}
		if !dashed {
			chunks := strings.Split(s.word, "-")
			for i, chunk := range chunks {
				if chunk == "" || s.d.Spell(chunk) {
					continue
				}
				for _, sug := range s.d.Suggest(chunk, s.max) {
					cand := strings.Join(append(append(append([]string{}, chunks[:i]...), sug), chunks[i+1:]...), "-")
					if s.d.Spell(cand) {
						s.handle(cand, false)
					}
				}
				break
			}
		}
	}
}

// add records a suggestion as it is.
func (s *suggester) add(text string) {
	if !s.seen[text] && !s.full() {
		s.seen[text] = true
		s.out = append(s.out, text)
	}
}

// edits runs Hunspell's edit suggestions for one form of the word: first
// without compounds and, if those found nothing better, with compounds.
// It reports whether a "good" edit was found (an upper-case form, a REP
// or MAP replacement, or a word pair from the dictionary), which makes
// n-gram suggestions unnecessary.
func (s *suggester) edits(w string) bool {
	a := s.d.a
	rs := []rune(w)
	good := false
	noCompoundTwoWords := false
	for cpd := 0; cpd < 2 && !noCompoundTwoWords && !good; cpd++ {
		compounds := cpd == 1
		start := len(s.out)
		limitReached := func() bool {
			return s.full() || (compounds && len(s.out) >= start+a.maxCpdSugs)
		}
		test := func(cands ...string) bool {
			added := false
			for _, c := range cands {
				if limitReached() {
					break
				}
				if s.seen[c] || c == w {
					continue
				}
				if s.good(c, compounds) && s.handle(c, false) {
					added = true
					if compounds {
						s.compoundFound++
					}
				}
			}
			return added
		}
		test(a.upper(w))
		if !compounds || len(s.out) < start+a.maxCpdSugs {
			var multi []string
			for _, r := range replChars(w, a.rep) {
				if strings.Contains(r, " ") {
					// A replacement with a space: each word must be good.
					ok := true
					for _, part := range strings.Split(r, " ") {
						if part != "" && !s.good(part, compounds) {
							ok = false
							break
						}
					}
					if ok && !limitReached() && s.handle(r, false) {
						good = true
					}
					continue
				}
				multi = append(multi, r)
			}
			if test(multi...) {
				good = true
			}
		}
		if test(mapChars(w, a.maps)...) {
			good = true
		}
		if !compounds && len(s.out) > start {
			noCompoundTwoWords = true
		}
		test(swapChar(rs)...)
		test(longSwapChar(rs)...)
		test(badCharKey(rs, a)...)
		test(extraChar(rs)...)
		test(forgotChar(rs, a.try)...)
		test(moveChar(rs)...)
		test(badChar(rs, a.try)...)
		test(doubleTwoChars(rs)...)
		if !compounds || (!a.noSplitSugs && len(s.out) < start+a.maxCpdSugs) {
			if s.twoWords(rs, compounds, good) {
				good = true
			}
		}
	}
	return good
}

// twoWords suggests splitting a word in two. A pair the dictionary lists
// as one entry ("a lot") replaces every other suggestion.
func (s *suggester) twoWords(rs []rune, compounds, good bool) bool {
	a := s.d.a
	if len(rs) < 3 {
		return good
	}
	useDash := strings.Contains(a.try, "-") || strings.Contains(a.try, "a")
	for i := 1; i < len(rs); i++ {
		left, right := string(rs[:i]), string(rs[i:])
		pair := left + " " + right
		if !compounds && s.good(pair, false) {
			if !good {
				good = true
				s.out = s.out[:0]
				s.seen = map[string]bool{}
			}
			if !s.seen[pair] {
				s.seen[pair] = true
				s.out = append([]string{pair}, s.out...)
			}
			continue
		}
		if a.noSplitSugs {
			continue
		}
		if !s.good(left, compounds) || !s.good(right, compounds) {
			continue
		}
		if s.full() {
			return good
		}
		s.handle(pair, false)
		if useDash && i > 1 && len(rs)-i > 1 && !s.full() {
			s.handle(left+"-"+right, false)
		}
	}
	return good
}

func replChars(w string, table []repPattern) []string {
	if len([]rune(w)) < 2 || len(table) == 0 {
		return nil
	}
	var out []string
	for _, p := range table {
		for i := 0; i+len(p.from) <= len(w); i++ {
			if !strings.HasPrefix(w[i:], p.from) {
				continue
			}
			if p.start && i != 0 {
				continue
			}
			if p.end && i+len(p.from) != len(w) {
				continue
			}
			out = append(out, w[:i]+p.to+w[i+len(p.from):])
		}
	}
	return out
}

func mapChars(w string, table [][]string) []string {
	if len([]rune(w)) < 2 || len(table) == 0 {
		return nil
	}
	var out []string
	var rec func(word string, start int, depth int)
	rec = func(word string, start int, depth int) {
		if start >= len(word) || depth > 8 || len(out) > 2000 {
			return
		}
		for _, group := range table {
			for _, opt := range group {
				pos := strings.Index(word[start:], opt)
				if pos < 0 {
					continue
				}
				pos += start
				for _, other := range group {
					if other == opt {
						continue
					}
					replaced := word[:pos] + other + word[pos+len(opt):]
					out = append(out, replaced)
					rec(replaced, pos+len(other), depth+1)
				}
			}
		}
	}
	rec(w, 0, 0)
	return out
}

func swapChar(rs []rune) []string {
	n := len(rs)
	if n < 2 {
		return nil
	}
	var out []string
	for i := 0; i < n-1; i++ {
		c := append([]rune{}, rs...)
		c[i], c[i+1] = c[i+1], c[i]
		out = append(out, string(c))
	}
	if n == 4 || n == 5 {
		c := []rune{rs[1], rs[0]}
		if n == 5 {
			c = append(c, rs[2])
		}
		c = append(c, rs[n-1], rs[n-2])
		out = append(out, string(c))
		if n == 5 {
			out = append(out, string([]rune{rs[0], rs[2], rs[1], rs[4], rs[3]}))
		}
	}
	return out
}

func longSwapChar(rs []rune) []string {
	var out []string
	for first := 0; first < len(rs)-2; first++ {
		for second := first + 2; second < len(rs) && second < first+maxCharDistance; second++ {
			c := append([]rune{}, rs...)
			c[first], c[second] = c[second], c[first]
			out = append(out, string(c))
		}
	}
	return out
}

func badCharKey(rs []rune, a *aff) []string {
	var out []string
	layout := []rune(a.key)
	for i, c := range rs {
		up := []rune(a.upper(string(c)))
		if len(up) == 1 && up[0] != c {
			x := append([]rune{}, rs...)
			x[i] = up[0]
			out = append(out, string(x))
		}
		for p, k := range layout {
			if k != c {
				continue
			}
			if p > 0 && layout[p-1] != '|' {
				x := append([]rune{}, rs...)
				x[i] = layout[p-1]
				out = append(out, string(x))
			}
			if p+1 < len(layout) && layout[p+1] != '|' {
				x := append([]rune{}, rs...)
				x[i] = layout[p+1]
				out = append(out, string(x))
			}
		}
	}
	return out
}

func extraChar(rs []rune) []string {
	if len(rs) < 2 {
		return nil
	}
	var out []string
	for i := range rs {
		out = append(out, string(rs[:i])+string(rs[i+1:]))
	}
	return out
}

func forgotChar(rs []rune, try string) []string {
	var out []string
	for _, c := range try {
		for i := 0; i <= len(rs); i++ {
			out = append(out, string(rs[:i])+string(c)+string(rs[i:]))
		}
	}
	return out
}

func moveChar(rs []rune) []string {
	n := len(rs)
	if n < 2 {
		return nil
	}
	var out []string
	for from := 0; from < n; from++ {
		for to := from + 3; to < n+1 && to <= from+maxCharDistance; to++ {
			if to > n {
				break
			}
			x := string(rs[:from]) + string(rs[from+1:to]) + string(rs[from]) + string(rs[to:])
			out = append(out, x)
		}
	}
	for from := n - 1; from >= 0; from-- {
		lo := from - maxCharDistance + 1
		if lo < 0 {
			lo = 0
		}
		for to := from - 2; to >= lo; to-- {
			x := string(rs[:to]) + string(rs[from]) + string(rs[to:from]) + string(rs[from+1:])
			out = append(out, x)
		}
	}
	return out
}

func badChar(rs []rune, try string) []string {
	var out []string
	for _, c := range try {
		for i := len(rs) - 1; i >= 0; i-- {
			if rs[i] == c {
				continue
			}
			x := append([]rune{}, rs...)
			x[i] = c
			out = append(out, string(x))
		}
	}
	return out
}

func doubleTwoChars(rs []rune) []string {
	if len(rs) < 5 {
		return nil
	}
	var out []string
	for i := 3; i < len(rs); i++ {
		if rs[i-2] == rs[i] && rs[i-3] == rs[i-1] {
			out = append(out, string(rs[:i-1])+string(rs[i+1:]))
		}
	}
	return out
}

// N-gram suggestions: the dictionary stems most similar to the
// misspelling, expanded with their affixes, scored again.

const (
	maxRoots   = 100
	maxGuesses = 200
)

type scored struct {
	score float64
	text  string
	real  string
	order int
}

func (s *suggester) ngram() []string {
	a := s.d.a
	mis := []rune(a.lowerOne(s.word))
	lowered := s.d.loweredStems()
	top := newTopN(maxRoots)
	for i, e := range s.d.ngram {
		sr := lowered[i]
		if abs(len(sr)-len(mis)) > 4 {
			continue
		}
		sc := rootScore(mis, sr)
		for _, alt := range e.alt {
			if x := rootScore(mis, []rune(a.lowerOne(alt))); x > sc {
				sc = x
			}
		}
		top.add(scored{score: sc, order: i})
	}
	roots := top.sorted()
	threshold := detectThreshold(mis)
	var guesses []scored
	for _, r := range roots {
		e := s.d.ngram[r.order]
		for _, alt := range e.alt {
			if sc := roughAffixScore(mis, []rune(a.lowerOne(alt))); sc > threshold {
				guesses = append(guesses, scored{score: sc, text: alt, real: e.stem})
			}
		}
		for _, f := range s.formsFor(e, string(mis)) {
			if sc := roughAffixScore(mis, []rune(a.lowerOne(f))); sc > threshold {
				guesses = append(guesses, scored{score: sc, text: f, real: f})
			}
		}
	}
	sort.SliceStable(guesses, func(i, j int) bool { return guesses[i].score > guesses[j].score })
	if len(guesses) > maxGuesses {
		guesses = guesses[:maxGuesses]
	}
	fact := 1.0
	if a.maxDiff >= 0 {
		fact = (10.0 - float64(a.maxDiff)) / 5.0
	}
	for i := range guesses {
		guesses[i].score = preciseAffixScore(mis, []rune(a.lowerOne(guesses[i].text)), fact, guesses[i].score, len(a.phone) > 0)
	}
	sort.SliceStable(guesses, func(i, j int) bool { return guesses[i].score > guesses[j].score })
	known := map[string]bool{}
	for k := range s.seen {
		known[a.lowerOne(k)] = true
	}
	var out []string
	seen := false
	found := 0
	for _, g := range guesses {
		if seen && g.score <= 1000 {
			break
		}
		if g.score > 1000 {
			seen = true
		} else if g.score < -100 {
			if found > 0 || a.onlyMaxDiff {
				break
			}
			seen = true
		}
		dup := false
		for k := range known {
			if strings.Contains(g.real, k) {
				dup = true
				break
			}
		}
		// A form built from a root and its affixes must still be a
		// word (affix conditions and compound-only affixes).
		if !dup && s.d.check(g.real, false, false) {
			found++
			out = append(out, g.real)
		}
	}
	return out
}

func (s *suggester) formsFor(e *entry, similar string) []string {
	a := s.d.a
	out := []string{e.stem}
	var sfx, pfx []*affix
	for _, f := range e.flags {
		for _, x := range a.byFlag[f] {
			if x.cont.has(a.onlyInCompound) || x.cont.has(a.forbidden) || x.cont.has(a.nosuggest) {
				continue
			}
			if x.prefix {
				if x.cond.matchStart(e.stem) && strings.HasPrefix(similar, x.add) && strings.HasPrefix(e.stem, x.strip) {
					pfx = append(pfx, x)
				}
			} else if x.cond.matchEnd(e.stem) && strings.HasSuffix(similar, x.add) && strings.HasSuffix(e.stem, x.strip) {
				sfx = append(sfx, x)
			}
		}
	}
	for _, x := range sfx {
		out = append(out, strings.TrimSuffix(e.stem, x.strip)+x.add)
	}
	for _, p := range pfx {
		for _, x := range sfx {
			if p.cross && x.cross {
				root := strings.TrimSuffix(strings.TrimPrefix(e.stem, p.strip), x.strip)
				out = append(out, p.add+root+x.add)
			}
		}
	}
	for _, p := range pfx {
		out = append(out, p.add+strings.TrimPrefix(e.stem, p.strip))
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func rootScore(a, b []rune) float64 {
	return float64(ngram(3, a, b, false, false, true) + leftCommon(a, b))
}

func roughAffixScore(a, b []rune) float64 {
	return float64(ngram(len(a), a, b, false, true, false) + leftCommon(a, b))
}

func preciseAffixScore(a, b []rune, fact, base float64, phonetic bool) float64 {
	l := lcs(a, b)
	if len(a) == len(b) && len(a) == l {
		return base + 2000
	}
	r := float64(2*l - abs(len(a)-len(b)))
	r += float64(leftCommon(a, b))
	if commonChars(a, b) > 0 {
		r++
	}
	r += float64(ngram(4, a, b, false, true, false))
	bigrams := float64(ngram(2, a, b, true, true, false) + ngram(2, b, a, true, true, false))
	r += bigrams
	limit := float64(len(a)+len(b)) * fact
	if phonetic {
		limit = float64(len(b)) * fact
	}
	if bigrams < limit {
		r -= 1000
	}
	return r
}

func detectThreshold(w []rune) float64 {
	t := 0
	for start := 1; start < 4; start++ {
		m := append([]rune{}, w...)
		for p := start; p < len(m); p += 4 {
			m[p] = '*'
		}
		t += ngram(len(w), w, m, false, true, false)
	}
	return float64(t/3 - 1)
}

func commonChars(a, b []rune) int {
	n := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			n++
		}
	}
	return n
}

func leftCommon(a, b []rune) int {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return i
}

func containsRunes(hay, needle []rune) bool {
	if len(needle) == 0 {
		return true
	}
outer:
	for i := 0; i+len(needle) <= len(hay); i++ {
		for j := range needle {
			if hay[i+j] != needle[j] {
				continue outer
			}
		}
		return true
	}
	return false
}

func ngram(n int, s1, s2 []rune, weighted, anyMismatch, longerWorse bool) int {
	l1, l2 := len(s1), len(s2)
	if l2 == 0 {
		return 0
	}
	score := 0
	for size := 1; size <= n; size++ {
		ns := 0
		for pos := 0; pos+size <= l1; pos++ {
			if containsRunes(s2, s1[pos:pos+size]) {
				ns++
			} else if weighted {
				ns--
				if pos == 0 || pos+size == l1 {
					ns--
				}
			}
		}
		score += ns
		if ns < 2 && !weighted {
			break
		}
	}
	penalty := 0
	if longerWorse {
		penalty = (l2 - l1) - 2
	} else if anyMismatch {
		penalty = abs(l2-l1) - 2
	}
	if penalty > 0 {
		return score - penalty
	}
	return score
}

func lcs(a, b []rune) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			switch {
			case a[i-1] == b[j-1]:
				cur[j] = prev[j-1] + 1
			case prev[j] >= cur[j-1]:
				cur[j] = prev[j]
			default:
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// Phonetic suggestions (PHONE): stems that sound like the misspelling.

func (s *suggester) phonet() []string {
	a := s.d.a
	mis := a.lowerOne(s.word)
	misR := []rune(mis)
	code := []rune(a.metaphone(mis))
	lowered := s.d.loweredStems()
	top := newTopN(maxRoots)
	for i, e := range s.d.ngram {
		sr := lowered[i]
		if abs(len(sr)-len(misR)) > 3 {
			continue
		}
		n := rootScore(misR, sr)
		for _, alt := range e.alt {
			if x := rootScore(misR, []rune(a.lowerOne(alt))); x > n {
				n = x
			}
		}
		if n <= 2 {
			continue
		}
		sc := 2 * float64(ngram(3, code, []rune(a.metaphone(e.stem)), false, false, true))
		top.add(scored{score: sc, text: e.stem, order: i})
	}
	scores := top.sorted()
	for i := range scores {
		w := []rune(a.lowerOne(scores[i].text))
		scores[i].score += float64(2*lcs(misR, w) - abs(len(misR)-len(w)) + leftCommon(misR, w))
	}
	sort.SliceStable(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	out := make([]string, len(scores))
	for i, x := range scores {
		out[i] = x.text
	}
	return out
}

func (a *aff) metaphone(word string) string {
	w := []rune(strings.ToUpper(word))
	var b strings.Builder
	for pos := 0; pos < len(w); {
		matched := false
		for _, r := range a.phone[w[pos]] {
			if r.start && pos > 0 {
				continue
			}
			n := len(r.letters)
			if pos+n > len(w) {
				continue
			}
			if r.end && pos+n != len(w) {
				continue
			}
			ok := true
			for k, l := range r.letters {
				if !l.match(w[pos+k]) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			b.WriteString(r.replace)
			consumed := n - r.lookahead
			if consumed < 1 {
				consumed = 1
			}
			pos += consumed
			matched = true
			break
		}
		if !matched {
			pos++
		}
	}
	return b.String()
}

// topN keeps the n best scores seen, earlier ones first among equals.
type topN struct {
	n     int
	items []scored // a min-heap on (score, then later order)
}

func newTopN(n int) *topN { return &topN{n: n} }

func (t *topN) less(i, j int) bool {
	a, b := t.items[i], t.items[j]
	if a.score != b.score {
		return a.score < b.score
	}
	return a.order > b.order
}

func (t *topN) add(x scored) {
	if len(t.items) < t.n {
		t.items = append(t.items, x)
		for i := len(t.items) - 1; i > 0; {
			p := (i - 1) / 2
			if !t.less(i, p) {
				break
			}
			t.items[i], t.items[p] = t.items[p], t.items[i]
			i = p
		}
		return
	}
	root := t.items[0]
	if x.score < root.score || (x.score == root.score && x.order > root.order) {
		return
	}
	t.items[0] = x
	for i := 0; ; {
		l, r, m := 2*i+1, 2*i+2, i
		if l < len(t.items) && t.less(l, m) {
			m = l
		}
		if r < len(t.items) && t.less(r, m) {
			m = r
		}
		if m == i {
			break
		}
		t.items[i], t.items[m] = t.items[m], t.items[i]
		i = m
	}
}

// sorted returns the kept items, best first.
func (t *topN) sorted() []scored {
	out := append([]scored{}, t.items...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].order < out[j].order
	})
	return out
}
