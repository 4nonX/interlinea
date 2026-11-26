package spell

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// form is a hypothesis of how a word is built: a dictionary stem with up
// to two suffixes (or, with COMPLEXPREFIXES, two prefixes). prefix and
// suffix are the affixes next to the stem; prefix2 and suffix2 the outer
// ones.
type form struct {
	text, stem       string
	prefix, suffix   *affix
	prefix2, suffix2 *affix
	entry            *entry
}

func (f form) hasAffixes() bool { return f.prefix != nil || f.suffix != nil }

func (f form) flags() flagSet {
	var fs flagSet
	if f.entry != nil {
		fs = f.entry.flags
	}
	if f.prefix != nil {
		fs = union(fs, f.prefix.cont)
	}
	if f.suffix != nil {
		fs = union(fs, f.suffix.cont)
	}
	return fs
}

func (f form) affixes() []*affix {
	var out []*affix
	for _, x := range []*affix{f.prefix2, f.prefix, f.suffix, f.suffix2} {
		if x != nil {
			out = append(out, x)
		}
	}
	return out
}

// compoundBudget is the number of splits tried for one word before
// giving up on it as a compound.
const compoundBudget = 20000

type cpdPos int

const (
	posNone cpdPos = iota
	posBegin
	posMiddle
	posEnd
)

// Spell reports whether the word is spelled correctly.
func (d *Dictionary) Spell(word string) bool {
	word = strings.TrimSpace(word)
	if word == "" {
		return true
	}
	if d.check(word, true, true) {
		return true
	}
	// An abbreviation: "etc." may be in the dictionary with its dot.
	if strings.HasSuffix(word, ".") {
		trimmed := strings.TrimRight(word, ".")
		if trimmed == "" {
			return true
		}
		if d.check(trimmed, true, true) || d.check(trimmed+".", true, true) {
			return true
		}
	}
	return false
}

func (d *Dictionary) check(word string, capitalization, allowNosuggest bool) bool {
	a := d.a
	if a.forbidden != 0 && d.hasFlag(word, a.forbidden, true) {
		return false
	}
	word = a.iconv.apply(word)
	word = a.stripIgnored(word)
	if isNumber(word) {
		return true
	}
	splits := [][]string{{word}}
	if d.breakPoints(word) < maxBreakPoints {
		splits = d.breakWord(word, 0)
	}
	for _, parts := range splits {
		ok, any := true, false
		for _, p := range parts {
			if p != "" {
				any = true
			}
			if p != "" && !d.correct(p, capitalization, allowNosuggest) {
				ok = false
				break
			}
		}
		if ok && any {
			return true
		}
	}
	return false
}

// isNumber accepts digits with single separators between them
// ("1,250.50", "12-34"), as Hunspell does.
func isNumber(w string) bool {
	sep := false
	digits := false
	for i, r := range w {
		switch {
		case unicode.IsDigit(r):
			digits, sep = true, false
		case r == ',' || r == '.' || r == '-':
			if sep || i == 0 {
				return false
			}
			sep = true
		default:
			return false
		}
	}
	return digits && !sep
}

// maxBreakPoints is how many places a word may be broken at (dashes, by
// default) before only the whole word is checked: the ways to split a word
// double with each, and a word of many dashes would take minutes and
// gigabytes. A word with this many is wrong unless it is in the dictionary
// as it is, which is how Hunspell checks it too.
const maxBreakPoints = 10

// breakPoints counts the places the word has a BREAK pattern at.
func (d *Dictionary) breakPoints(word string) int {
	n := 0
	for _, p := range d.a.breaks {
		if p.text != "" {
			n += strings.Count(word, p.text)
		}
	}
	return n
}

// breakWord lists ways to split the word at BREAK patterns (dashes by
// default), the whole word first.
func (d *Dictionary) breakWord(text string, depth int) [][]string {
	out := [][]string{{text}}
	if depth > 10 {
		return out
	}
	for _, p := range d.a.breaks {
		switch {
		case p.start:
			if strings.HasPrefix(text, p.text) {
				for _, rest := range d.breakWord(text[len(p.text):], depth+1) {
					out = append(out, append([]string{""}, rest...))
				}
			}
		case p.end:
			if strings.HasSuffix(text, p.text) {
				out = append(out, []string{text[:len(text)-len(p.text)]})
			}
		default:
			for i := 1; i < len(text); i++ {
				if !utf8.RuneStart(text[i]) || !strings.HasPrefix(text[i:], p.text) {
					continue
				}
				end := i + len(p.text)
				if end >= len(text) {
					continue
				}
				for _, rest := range d.breakWord(text[end:], depth+1) {
					out = append(out, append([]string{text[:i]}, rest...))
				}
			}
		}
	}
	return out
}

func (d *Dictionary) correct(word string, capitalization, allowNosuggest bool) bool {
	found := false
	d.goodForms(word, capitalization, allowNosuggest, true, true, func() bool {
		found = true
		return false
	})
	return found
}

// goodForms calls yield for each valid analysis of the word; yield
// returns false to stop.
func (d *Dictionary) goodForms(word string, capitalization, allowNosuggest, affixForms, compoundForms bool, yield func() bool) {
	a := d.a
	var c capType
	var vs []string
	if capitalization {
		c, vs = a.variants(word)
	} else {
		c, vs = a.guess(word), []string{word}
	}
	for i, v := range vs {
		// Variants are tried in Hunspell's order; a forbidden entry in
		// another capitalization ("Galls" for GALLS, Dutch "Ijs" for
		// IJS) stops the search: the word is wrong in that form.
		if i > 0 && a.forbidden != 0 && d.hasFlag(v, a.forbidden, true) {
			return
		}
		if affixForms {
			stop := false
			d.affixForms(v, c, allowNosuggest, nil, nil, nil, posNone, false, func(f form) bool {
				if a.checkSharps && a.keepcase != 0 && strings.Contains(f.entry.stem, "ß") &&
					f.flags().has(a.keepcase) && c == capAll && strings.ContainsAny(word, "ßẞ") {
					return true
				}
				stop = !yield()
				return !stop
			})
			if stop {
				return
			}
		}
		if compoundForms {
			stop := false
			d.compoundForms(v, c, allowNosuggest, func([]form) bool {
				stop = !yield()
				return !stop
			})
			if stop {
				return
			}
		}
	}
}

func (d *Dictionary) isGoodForm(f form, pos cpdPos, c capType, allowNosuggest bool) bool {
	a := d.a
	if f.entry == nil {
		return false
	}
	root := f.entry.flags
	all := f.flags()
	if all.has(a.forbidden) {
		return false
	}
	if !allowNosuggest && root.has(a.nosuggest) {
		return false
	}
	if c != f.entry.cap && root.has(a.keepcase) && !(a.checkSharps && strings.Contains(f.entry.stem, "ß")) {
		return false
	}
	if a.needaffix != 0 {
		if root.has(a.needaffix) && !f.hasAffixes() {
			return false
		}
		if f.hasAffixes() {
			allNeed := true
			for _, x := range f.affixes() {
				if !x.cont.has(a.needaffix) {
					allNeed = false
					break
				}
			}
			if allNeed {
				return false
			}
		}
	}
	if f.prefix != nil && !all.has(f.prefix.flag) {
		return false
	}
	if f.suffix != nil && !all.has(f.suffix.flag) {
		return false
	}
	if a.circumfix != 0 {
		sh := f.suffix != nil && f.suffix.cont.has(a.circumfix)
		ph := f.prefix != nil && f.prefix.cont.has(a.circumfix)
		if sh != ph {
			return false
		}
	}
	if pos == posNone {
		return !all.has(a.onlyInCompound)
	}
	// A suffix only allowed inside compounds (a German linking "s")
	// cannot end one, unless it says it may.
	if pos == posEnd && f.suffix != nil && f.suffix.cont.has(a.onlyInCompound) &&
		!f.suffix.cont.has(a.compoundEnd) && !f.suffix.cont.has(a.compoundFlag) {
		return false
	}
	if all.has(a.compoundFlag) {
		return true
	}
	switch pos {
	case posBegin:
		return all.has(a.compoundBegin)
	case posEnd:
		return all.has(a.compoundEnd)
	case posMiddle:
		return all.has(a.compoundMiddle)
	}
	return false
}

// affixForms calls yield for each valid affix analysis of the word.
func (d *Dictionary) affixForms(word string, c capType, allowNosuggest bool, prefixFlags, suffixFlags, forbiddenFlags flagSet, pos cpdPos, withForbidden bool, yield func(form) bool) bool {
	a := d.a
	cont := true
	d.produceAffixForms(word, pos, prefixFlags, suffixFlags, forbiddenFlags, func(f form) bool {
		hs := d.homonyms(f.stem)
		if !withForbidden && a.forbidden != 0 && (pos != posNone || f.hasAffixes()) {
			for _, h := range hs {
				if h.flags.has(a.forbidden) {
					cont = false
					return false
				}
			}
		}
		found := false
		for _, h := range hs {
			g := f
			g.entry = h
			if (withForbidden && g.flags().has(a.forbidden)) || d.isGoodForm(g, pos, c, allowNosuggest) {
				found = true
				if !yield(g) {
					cont = false
					return false
				}
			}
		}
		if pos == posBegin && a.forceUCase != 0 && c == capInit {
			for _, h := range d.homonyms(a.lowerOne(f.stem)) {
				g := f
				g.entry = h
				if d.isGoodForm(g, pos, c, allowNosuggest) {
					found = true
					if !yield(g) {
						cont = false
						return false
					}
				}
			}
		}
		if found || pos != posNone || c != capAll {
			return true
		}
		if a.guess(word) == capNo {
			for _, h := range d.lower[f.stem] {
				g := f
				g.entry = h
				if d.isGoodForm(g, pos, c, allowNosuggest) {
					if !yield(g) {
						cont = false
						return false
					}
				}
			}
		}
		return true
	})
	return cont
}

func (d *Dictionary) produceAffixForms(word string, pos cpdPos, prefixFlags, suffixFlags, forbiddenFlags flagSet, yield func(form) bool) {
	if !yield(form{text: word, stem: word}) {
		return
	}
	suffixOK := pos == posNone || pos == posEnd || len(suffixFlags) > 0
	prefixOK := pos == posNone || pos == posBegin || len(prefixFlags) > 0
	if suffixOK && !d.desuffix(word, suffixFlags, forbiddenFlags, false, false, yield) {
		return
	}
	if !prefixOK {
		return
	}
	d.deprefix(word, prefixFlags, forbiddenFlags, false, func(f form) bool {
		if !yield(f) {
			return false
		}
		if suffixOK && f.prefix != nil && f.prefix.cross {
			return d.desuffix(f.stem, suffixFlags, forbiddenFlags, false, true, func(g form) bool {
				g.text = f.text
				g.prefix = f.prefix
				g.prefix2 = f.prefix2
				return yield(g)
			})
		}
		return true
	})
}

func goodAffix(x *affix, required, forbidden flagSet) bool {
	for _, f := range required {
		if !x.cont.has(f) {
			return false
		}
	}
	for _, f := range forbidden {
		if x.cont.has(f) {
			return false
		}
	}
	return true
}

func (d *Dictionary) desuffix(word string, required, forbidden flagSet, nested, cross bool, yield func(form) bool) bool {
	a := d.a
	node := &a.sfxTrie
	for i := len(word); i >= 0 && node != nil; i-- {
		if i < len(word) {
			node = node.child(word[i], false)
			if node == nil {
				break
			}
		}
		xs := node.affixes
		if len(xs) == 0 || (i < len(word) && !utf8.RuneStart(word[i])) {
			continue
		}
		rest := word[:i]
		if rest == "" && !a.fullStrip {
			continue
		}
		for _, x := range xs {
			if (cross && !x.cross) || !goodAffix(x, required, forbidden) {
				continue
			}
			stem := rest + x.strip
			if stem == "" || !x.cond.matchEnd(stem) {
				continue
			}
			if !yield(form{text: word, stem: stem, suffix: x}) {
				return false
			}
			if !nested {
				ok := d.desuffix(stem, append(flagSet{x.flag}, required...), forbidden, true, cross, func(g form) bool {
					g.text = word
					g.suffix2 = x
					return yield(g)
				})
				if !ok {
					return false
				}
			}
		}
	}
	return true
}

func (d *Dictionary) deprefix(word string, required, forbidden flagSet, nested bool, yield func(form) bool) bool {
	a := d.a
	node := &a.pfxTrie
	for i := 0; i <= len(word) && node != nil; i++ {
		if i > 0 {
			node = node.child(word[i-1], false)
			if node == nil {
				break
			}
		}
		xs := node.affixes
		if len(xs) == 0 || (i < len(word) && !utf8.RuneStart(word[i])) {
			continue
		}
		rest := word[i:]
		if rest == "" && !a.fullStrip {
			continue
		}
		for _, x := range xs {
			if !goodAffix(x, required, forbidden) {
				continue
			}
			stem := x.strip + rest
			if stem == "" || !x.cond.matchStart(stem) {
				continue
			}
			if !yield(form{text: word, stem: stem, prefix: x}) {
				return false
			}
			if !nested && a.complexPrefixes {
				ok := d.deprefix(stem, append(flagSet{x.flag}, required...), forbidden, true, func(g form) bool {
					g.text = word
					g.prefix2 = x
					return yield(g)
				})
				if !ok {
					return false
				}
			}
		}
	}
	return true
}

// compoundForms calls yield for each valid compound analysis.
func (d *Dictionary) compoundForms(word string, c capType, allowNosuggest bool, yield func([]form) bool) {
	a := d.a
	if a.forbidden != 0 {
		forbidden := false
		d.affixForms(word, c, true, nil, nil, nil, posNone, true, func(f form) bool {
			if f.flags().has(a.forbidden) {
				forbidden = true
				return false
			}
			return true
		})
		// "Borstweer" is not Borst + weer when the dictionary forbids
		// "borstweer".
		if forbidden || d.hasFlag(a.lowerOne(word), a.forbidden, true) {
			return
		}
	}
	rs := []rune(word)
	// Hunspell limits the time spent on one compound; a budget of steps
	// does the same without a clock, so results do not depend on speed.
	budget := compoundBudget
	if a.compoundBegin != 0 || a.compoundFlag != 0 {
		ok := d.compoundsByFlags(rs, c, 0, allowNosuggest, &budget, func(parts []form, replaced []int) bool {
			if d.badCompound(parts, replaced, c) {
				return true
			}
			return yield(parts)
		})
		if !ok {
			return
		}
	}
	if len(a.compoundRules) > 0 {
		budget = compoundBudget
		d.compoundsByRules(rs, nil, allowNosuggest, &budget, func(parts []form) bool {
			if d.badCompound(parts, nil, c) {
				return true
			}
			return yield(parts)
		})
	}
}

// compoundsByFlags splits the word into parts allowed in compounds by
// their flags, first part first. replaced lists the boundaries (index of
// the part before them) where a CHECKCOMPOUNDPATTERN replacement was
// undone.
func (d *Dictionary) compoundsByFlags(rs []rune, c capType, depth int, allowNosuggest bool, budget *int, yield func(parts []form, replaced []int) bool) bool {
	a := d.a
	if *budget--; *budget <= 0 {
		return false
	}
	var forbidden, permit flagSet
	if a.compoundForbid != 0 {
		forbidden = flagSet{a.compoundForbid}
	}
	if a.compoundPermit != 0 {
		permit = flagSet{a.compoundPermit}
	}
	word := string(rs)
	if depth > 0 {
		ok := d.affixForms(word, c, allowNosuggest, permit, nil, forbidden, posEnd, false, func(f form) bool {
			return yield([]form{f}, nil)
		})
		if !ok {
			return false
		}
	}
	maxDepth := a.compoundWordMax
	if a.compoundSyllableMax > 0 {
		maxDepth = 0 // checked on the whole compound instead
	}
	if len(rs) < a.compoundMin*2 || (maxDepth > 0 && depth >= maxDepth) {
		return true
	}
	pos := posBegin
	var prefixFlags flagSet
	if depth > 0 {
		pos, prefixFlags = posMiddle, permit
	}
	try := func(left string, text string, right []rune, replacedHere bool) bool {
		return d.affixForms(left, c, allowNosuggest, prefixFlags, permit, forbidden, pos, false, func(f form) bool {
			f.text = text
			return d.compoundsByFlags(right, c, depth+1, allowNosuggest, budget, func(rest []form, replaced []int) bool {
				parts := append([]form{f}, rest...)
				var rep []int
				if replacedHere {
					rep = append(rep, 0)
				}
				for _, r := range replaced {
					rep = append(rep, r+1)
				}
				return yield(parts, rep)
			})
		})
	}
	for p := a.compoundMin; p <= len(rs)-a.compoundMin; p++ {
		beg := string(rs[:p])
		if !try(beg, beg, rs[p:], false) {
			return false
		}
		if a.simplifiedTriple && rs[p-1] == rs[p] {
			if !try(beg+string(rs[p-1]), beg, rs[p:], false) {
				return false
			}
		}
	}
	// A simplified form at the boundary ("o b z": foo + bar written
	// fozar): undo the replacement and split there.
	for _, pat := range a.patterns {
		if pat.replacement == "" {
			continue
		}
		rep := []rune(pat.replacement)
		for p := 1; p+len(rep) <= len(rs); p++ {
			if string(rs[p:p+len(rep)]) != pat.replacement {
				continue
			}
			left := string(rs[:p]) + pat.left
			right := append([]rune(pat.right), rs[p+len(rep):]...)
			if utf8.RuneCountInString(left) < a.compoundMin || len(right) < a.compoundMin {
				continue
			}
			okPat := d.affixForms(left, c, allowNosuggest, prefixFlags, permit, forbidden, pos, false, func(f form) bool {
				if pat.leftFlag != 0 && !f.flags().has(pat.leftFlag) {
					return true
				}
				return d.compoundsByFlags(right, c, depth+1, allowNosuggest, budget, func(rest []form, replaced []int) bool {
					if pat.rightFlag != 0 && !rest[0].flags().has(pat.rightFlag) {
						return true
					}
					parts := append([]form{f}, rest...)
					repl := []int{0}
					for _, r := range replaced {
						repl = append(repl, r+1)
					}
					return yield(parts, repl)
				})
			})
			if !okPat {
				return false
			}
		}
	}
	return true
}

func (d *Dictionary) compoundsByRules(rs []rune, prev []*entry, allowNosuggest bool, budget *int, yield func([]form) bool) bool {
	a := d.a
	if *budget--; *budget <= 0 {
		return false
	}
	word := string(rs)
	if len(prev) > 0 {
		for _, h := range d.homonyms(word) {
			if !allowNosuggest && h.flags.has(a.nosuggest) {
				continue
			}
			parts := append(append([]*entry{}, prev...), h)
			if d.rulesMatch(parts, false) {
				if !yield(append(entriesToForms(prev), form{text: word, stem: word, entry: h})) {
					return false
				}
			}
		}
	}
	if len(rs) < a.compoundMin*2 || (a.compoundWordMax > 0 && len(prev) >= a.compoundWordMax) {
		return true
	}
	for p := a.compoundMin; p <= len(rs)-a.compoundMin; p++ {
		beg := string(rs[:p])
		for _, h := range d.homonyms(beg) {
			if !allowNosuggest && h.flags.has(a.nosuggest) {
				continue
			}
			parts := append(append([]*entry{}, prev...), h)
			if !d.rulesMatch(parts, true) {
				continue
			}
			if !d.compoundsByRules(rs[p:], parts, allowNosuggest, budget, yield) {
				return false
			}
		}
	}
	return true
}

func entriesToForms(es []*entry) []form {
	out := make([]form, len(es))
	for i, e := range es {
		out[i] = form{text: e.stem, stem: e.stem, entry: e}
	}
	return out
}

// rulesMatch reports whether some COMPOUNDRULE matches the parts' flags
// (partial: the parts are a beginning of a match).
func (d *Dictionary) rulesMatch(parts []*entry, partial bool) bool {
	for _, r := range d.a.compoundRules {
		if ruleMatch(r, 0, parts, 0, partial) {
			return true
		}
	}
	return false
}

func ruleMatch(r []ruleElem, i int, parts []*entry, j int, partial bool) bool {
	if j == len(parts) {
		if partial {
			return true
		}
		for ; i < len(r); i++ {
			if r[i].quant == 0 {
				return false
			}
		}
		return true
	}
	if i == len(r) {
		return false
	}
	e := r[i]
	has := parts[j].flags.has(e.flag)
	switch e.quant {
	case '*':
		return ruleMatch(r, i+1, parts, j, partial) || (has && ruleMatch(r, i, parts, j+1, partial))
	case '?':
		return ruleMatch(r, i+1, parts, j, partial) || (has && ruleMatch(r, i+1, parts, j+1, partial))
	default:
		return has && ruleMatch(r, i+1, parts, j+1, partial)
	}
}

func (d *Dictionary) badCompound(parts []form, replaced []int, c capType) bool {
	a := d.a
	last := parts[len(parts)-1]
	if a.forceUCase != 0 && c != capAll && c != capInit {
		if d.hasFlag(last.text, a.forceUCase, false) || (last.entry != nil && last.entry.flags.has(a.forceUCase)) {
			return true
		}
	}
	if a.compoundWordMax > 0 {
		words := 0
		for _, p := range parts {
			words++
			if p.entry != nil && p.entry.flags.has(a.compoundRoot) {
				words++
			}
		}
		if words > a.compoundWordMax {
			if a.compoundSyllableMax == 0 {
				return true
			}
			syll := 0
			for _, p := range parts {
				for _, r := range p.text {
					if strings.ContainsRune(a.syllableVowels, r) {
						syll++
					}
				}
			}
			if syll > a.compoundSyllableMax {
				return true
			}
		}
	}
	isReplaced := func(i int) bool {
		for _, r := range replaced {
			if r == i {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(parts)-1; i++ {
		lp, rp := parts[i], parts[i+1]
		left, right := lp.text, rp.text
		if a.compoundForbid != 0 && d.hasFlag(left, a.compoundForbid, false) {
			return true
		}
		spaced := false
		d.affixForms(left+" "+right, c, true, nil, nil, nil, posNone, false, func(form) bool {
			spaced = true
			return false
		})
		if spaced {
			return true
		}
		if a.checkRep {
			for _, cand := range replChars(left+right, a.rep) {
				if strings.Contains(cand, " ") {
					continue
				}
				found := false
				d.affixForms(cand, c, true, nil, nil, nil, posNone, false, func(form) bool {
					found = true
					return false
				})
				if found {
					return true
				}
			}
		}
		if a.checkTriple {
			lr, rr := []rune(left), []rune(right)
			if len(lr) >= 2 && len(rr) >= 1 && lr[len(lr)-1] == lr[len(lr)-2] && lr[len(lr)-1] == rr[0] {
				return true
			}
			if len(lr) >= 1 && len(rr) >= 2 && lr[len(lr)-1] == rr[0] && rr[0] == rr[1] {
				return true
			}
		}
		if a.checkCase {
			lc, _ := utf8.DecodeLastRuneInString(left)
			rc, _ := utf8.DecodeRuneInString(right)
			if (a.isUpper(rc) || a.isUpper(lc)) && rc != '-' && lc != '-' {
				return true
			}
		}
		if len(a.patterns) > 0 && !isReplaced(i) {
			for _, p := range a.patterns {
				if p.matches(lp, rp) {
					return true
				}
			}
		}
		if a.checkDup && left == right && i == len(parts)-2 {
			return true
		}
	}
	return false
}

// matches reports whether a CHECKCOMPOUNDPATTERN forbids the boundary
// between the two parts: the left part ends with the pattern (or, with
// "0", with its unmodified stem), the right one begins with its second
// pattern, and their stems have the flags it names.
func (p cpdPattern) matches(l, r form) bool {
	if !strings.HasPrefix(r.text, p.right) {
		return false
	}
	if p.leftFlag != 0 && !l.flags().has(p.leftFlag) {
		return false
	}
	if p.rightFlag != 0 && !r.flags().has(p.rightFlag) {
		return false
	}
	if p.leftUnchanged {
		return l.entry != nil && strings.HasSuffix(l.text, l.entry.stem)
	}
	return strings.HasSuffix(l.text, p.left)
}
