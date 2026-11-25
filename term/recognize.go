package term

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/4nonX/interlinea/langtag"
)

// Index finds the terms of one or more termbases in text, as the term
// recognition window of Trados, memoQ and Phrase does, and checks
// translations against them (missing and forbidden terms).
//
// Matching ignores case and tolerates inflection: an ending added to the
// last word of a term ("Kaution" in "Kautionen", "Mieter" in "Mieters",
// "deposit" in "deposits"), umlaut plurals ("Vertrag" in "Verträge") and,
// in German, adjective endings on the words before it ("ordentliche
// Kündigung" in "der ordentlichen Kündigung"). It does not lemmatize, so
// irregular forms and changed stems are not found; nor does it look inside
// compounds. Korean particles after a term are allowed; Chinese, Japanese
// and Thai are matched as substrings, since they do not separate words.
type Index struct {
	entries []indexed
}

type indexed struct {
	source string // the caller's label, for example the termbase ID
	entry  Entry
}

// Hit is a term found in a text.
type Hit struct {
	// Source is the label the entry was added with.
	Source  string `json:"source"`
	EntryID string `json:"entryId"`
	// Term is the source-language term that matched.
	Term Term `json:"term"`
	// Start and End are byte offsets into the text.
	Start int `json:"start"`
	End   int `json:"end"`
	// Exact is set when the text has the term as written (case aside).
	Exact bool `json:"exact"`
	// Targets are the entry's terms in the target language, approved first.
	Targets []Term `json:"targets"`
}

// Issue is a terminology problem in a translation.
type Issue struct {
	// Kind is "missing" (no allowed translation of a recognized term) or
	// "forbidden" (a forbidden term is used).
	Kind string `json:"kind"`
	// Term is the source term concerned; empty for a forbidden term that
	// no recognized source term led to.
	Term string `json:"term,omitempty"`
	// Expected is the preferred translation (missing).
	Expected string `json:"expected,omitempty"`
	// Found is the forbidden term as written in the target, with its byte
	// offsets (forbidden).
	Found string `json:"found,omitempty"`
	Start int    `json:"start,omitempty"`
	End   int    `json:"end,omitempty"`
	// EntryID is the entry the term belongs to.
	EntryID string `json:"entryId,omitempty"`
}

// NewIndex returns an empty index.
func NewIndex() *Index { return &Index{} }

// Add adds entries under a label (typically the termbase). Entries added
// earlier win ties, so add the termbase with the highest priority first.
func (x *Index) Add(source string, entries []Entry) {
	for _, e := range entries {
		x.entries = append(x.entries, indexed{source: source, entry: e})
	}
}

// Len is the number of entries in the index.
func (x *Index) Len() int { return len(x.entries) }

// termsIn returns an entry's terms in lang: the exact language if the entry
// has it, else those of the same primary language (de-AT for de-DE).
func termsIn(e Entry, lang string) []Term {
	var primary []Term
	for _, l := range e.Languages {
		switch langtag.Match(l.Lang, lang) {
		case langtag.ExactMatch:
			return l.Terms
		case langtag.PrimaryMatch:
			primary = append(primary, l.Terms...)
		}
	}
	if primary == nil && langtag.Primary(lang) != "" {
		// Tags that do not parse (custom codes) still match on their prefix.
		for _, l := range e.Languages {
			if langtag.Primary(l.Lang) == langtag.Primary(lang) {
				primary = append(primary, l.Terms...)
			}
		}
	}
	return primary
}

var statusRank = map[string]int{StatusApproved: 0, "": 1, StatusProposed: 1, StatusForbidden: 2}

func sortedTargets(ts []Term) []Term {
	out := append([]Term(nil), ts...)
	sort.SliceStable(out, func(i, j int) bool { return statusRank[out[i].Status] < statusRank[out[j].Status] })
	return out
}

// Recognize finds the source-language terms of the index in text. Where
// matches overlap the longest wins; the same span can belong to several
// entries (for example the same term in two termbases).
func (x *Index) Recognize(text, srcLang, tgtLang string) []Hit {
	m := newMatcher(srcLang)
	toks := m.tokens(text)
	var cands []Hit
	for _, ie := range x.entries {
		targets := sortedTargets(termsIn(ie.entry, tgtLang))
		for _, t := range termsIn(ie.entry, srcLang) {
			if t.Status == StatusForbidden {
				continue
			}
			for _, sp := range m.find(t.Text, text, toks) {
				cands = append(cands, Hit{Source: ie.source, EntryID: ie.entry.ID, Term: t, Start: sp.start, End: sp.end, Exact: sp.exact, Targets: targets})
			}
		}
	}
	// Longest first; then exact matches; then index order (stable).
	sort.SliceStable(cands, func(i, j int) bool {
		li, lj := cands[i].End-cands[i].Start, cands[j].End-cands[j].Start
		if li != lj {
			return li > lj
		}
		return cands[i].Exact && !cands[j].Exact
	})
	var out []Hit
	for _, c := range cands {
		keep := true
		for _, o := range out {
			if c.Start < o.End && c.End > o.Start {
				same := c.Start == o.Start && c.End == o.End
				if !same || (o.Source == c.Source && o.EntryID == c.EntryID) {
					keep = false
					break
				}
			}
		}
		if keep {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// Check reports recognized terms whose translation is missing from target
// and forbidden terms used in target: those of the recognized entries, and
// any other forbidden term of the index that no recognized entry allows.
func (x *Index) Check(hits []Hit, target, tgtLang string) []Issue {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	m := newMatcher(tgtLang)
	toks := m.tokens(target)
	var out []Issue
	reported := map[[2]int]bool{}
	// allowed holds the target terms the recognized entries allow, by Key
	// (case aside) and by SameKey (as spelled).
	allowed, allowedAs := map[string]bool{}, map[string]bool{}
	for _, h := range hits {
		for _, t := range h.Targets {
			if t.Status != StatusForbidden {
				allowed[Key(t.Text)] = true
				allowedAs[SameKey(t.Text)] = true
			}
		}
	}
	// A forbidden term that an allowed term spells the same, case aside
	// (a forbidden "Mistjet" beside a preferred "MistJet"), is found only
	// as it is written; otherwise the allowed spelling would count as it.
	forbidden := func(term, entryID string, t Term, siblings []Term) {
		strict := allowed[Key(t.Text)]
		for _, o := range siblings {
			if o.Status != StatusForbidden && Key(o.Text) == Key(t.Text) {
				strict = true
			}
		}
		for _, sp := range m.find(t.Text, target, toks) {
			k := [2]int{sp.start, sp.end}
			if reported[k] {
				continue
			}
			if strict && !sameCase(target[sp.start:sp.end], t.Text) {
				continue
			}
			reported[k] = true
			out = append(out, Issue{Kind: "forbidden", Term: term, Found: target[sp.start:sp.end], Start: sp.start, End: sp.end, EntryID: entryID})
		}
	}
	seenEntry := map[string]bool{}
	for _, h := range hits {
		key := h.Source + "\x00" + h.EntryID
		var ok, any bool
		var first string
		for _, t := range h.Targets {
			if t.Status == StatusForbidden {
				continue
			}
			any = true
			if first == "" {
				first = t.Text
			}
			if len(m.find(t.Text, target, toks)) > 0 {
				ok = true
			}
		}
		if any && !ok && !seenEntry[key] {
			out = append(out, Issue{Kind: "missing", Term: h.Term.Text, Expected: first, EntryID: h.EntryID})
		}
		seenEntry[key] = true
		for _, t := range h.Targets {
			if t.Status == StatusForbidden {
				forbidden(h.Term.Text, h.EntryID, t, h.Targets)
			}
		}
	}
	for _, ie := range x.entries {
		terms := termsIn(ie.entry, tgtLang)
		for _, t := range terms {
			if t.Status == StatusForbidden && !allowedAs[SameKey(t.Text)] {
				forbidden("", ie.entry.ID, t, terms)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == "forbidden"
		}
		return out[i].Start < out[j].Start
	})
	return out
}

// sameCase reports whether found, a match of term in text, has the case
// term has: letter by letter from the start, as far as the two spell the
// same letters (an inflected ending may differ). Space is not compared.
func sameCase(found, term string) bool {
	a := []rune(strings.Join(strings.Fields(found), " "))
	b := []rune(strings.Join(strings.Fields(term), " "))
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		if unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return true
		}
		return false
	}
	return true
}

// matcher holds the language rules for finding terms in text.
type matcher struct {
	lang    string // primary subtag
	noSpace bool   // the script does not separate words
}

func newMatcher(lang string) matcher {
	p := langtag.Primary(lang)
	return matcher{lang: p, noSpace: p == "zh" || p == "ja" || p == "th" || p == "lo" || p == "km" || p == "my"}
}

type token struct {
	start, end int
	fold       string
}

type span struct {
	start, end int
	exact      bool
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

// tokens splits text into words with byte offsets. Apostrophes inside a
// word stay in it ("tenant's").
func (m matcher) tokens(text string) []token {
	var out []token
	start := -1
	for i, r := range text {
		if isWordRune(r) || (start >= 0 && (r == '\'' || r == '’') && i+utf8.RuneLen(r) < len(text) && isWordRune(firstRune(text[i+utf8.RuneLen(r):]))) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			out = append(out, token{start, i, m.fold(text[start:i])})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, token{start, len(text), m.fold(text[start:])})
	}
	return out
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

// umlauts folds the German umlauts so plurals such as Verträge match.
var umlauts = strings.NewReplacer("ä", "a", "ö", "o", "ü", "u")

// find returns where term occurs in text.
func (m matcher) find(term, text string, toks []token) []span {
	if m.noSpace {
		return m.findSubstring(term, text)
	}
	words := m.tokens(term)
	if len(words) == 0 {
		return nil
	}
	var out []span
	for i := 0; i+len(words) <= len(toks); i++ {
		exact := true
		ok := true
		for j, w := range words {
			tk := toks[i+j]
			if j > 0 && strings.TrimSpace(text[toks[i+j-1].end:tk.start]) != "" && text[toks[i+j-1].end:tk.start] != "-" {
				ok = false
				break
			}
			e, match := m.word(w.fold, tk.fold, j == len(words)-1, len(words) > 1)
			if !match {
				ok = false
				break
			}
			exact = exact && e
		}
		if ok {
			out = append(out, span{toks[i].start, toks[i+len(words)-1].end, exact})
			i += len(words) - 1
		}
	}
	return out
}

// findSubstring matches term inside text, ignoring case and runs of space.
func (m matcher) findSubstring(term, text string) []span {
	key := strings.Join(strings.Fields(m.fold(term)), "")
	if key == "" {
		return nil
	}
	// Map positions in the space-free folded text back to byte offsets.
	var squeezed strings.Builder
	var offs []int // byte offset in text for each byte in squeezed
	for i, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		f := m.fold(string(r))
		squeezed.WriteString(f)
		for range len(f) {
			offs = append(offs, i)
		}
	}
	s := squeezed.String()
	var out []span
	for from := 0; from < len(s); {
		k := strings.Index(s[from:], key)
		if k < 0 {
			break
		}
		a := from + k
		b := a + len(key)
		end := len(text)
		if b < len(offs) {
			end = offs[b]
			// Step back over spaces the squeeze removed.
			for end > offs[b-1] && unicode.IsSpace(lastRune(text[:end])) {
				_, n := utf8.DecodeLastRuneInString(text[:end])
				end -= n
			}
		}
		out = append(out, span{offs[a], end, strings.Contains(m.fold(text[offs[a]:end]), m.fold(term))})
		from = b
	}
	return out
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

// Endings a term word may take in the text, per language. The rules are
// deliberately additive: an ending is appended to the term as written, so
// "Miete" does not match "Mieter" (a different word), while "Kaution"
// matches "Kautionen".
var endings = map[string][]string{
	"de": {"e", "n", "en", "s", "es", "er", "ern", "ns", "nen"},
	"en": {"s", "es", "'s", "s'"},
	"fr": {"s", "x", "es"},
	"es": {"s", "es"},
	"it": {},
	"pt": {"s", "es"},
	"nl": {"s", "en", "'s"},
	"pl": {"y", "i", "a", "u", "ie", "em", "ami", "ach", "om", "ów"},
	"ca": {"s", "es"},
	"gl": {"s", "es"},
	"af": {"e", "s", "te"},
}

// everyWord lists the languages in which each word of a term takes the
// plural, not only the last: contrat social, contrats sociaux.
var everyWord = map[string]bool{"fr": true, "es": true, "it": true, "pt": true, "ca": true, "gl": true, "pl": true}

// German adjective endings, for the words before the last in a term.
var deAdjective = []string{"e", "en", "er", "es", "em"}

// word compares a term word with a text word (both folded). last says
// whether it is the term's last word, multi whether the term has several.
func (m matcher) word(term, text string, last, multi bool) (exact, ok bool) {
	if term == text {
		return true, true
	}
	n := utf8.RuneCountInString(term)
	if m.lang == "ko" {
		// Particles and endings follow the word: 임대인 in 임대인은.
		return false, last && n >= 2 && strings.HasPrefix(text, term)
	}
	if _, ok := m.rules(term); ok {
		return false, m.inflected(term, text, last)
	}
	if n < 4 {
		return false, false
	}
	if !last && multi && m.lang == "de" {
		// Adjectives before the noun: ordentliche, ordentlichen, ordentlicher.
		base := strings.TrimSuffix(term, "e")
		for _, e := range append([]string{""}, deAdjective...) {
			if base+e == text {
				return false, true
			}
		}
		return false, false
	}
	if !last && !everyWord[m.lang] {
		return false, false
	}
	ends := endings[m.lang]
	if ends == nil {
		ends = []string{"s"}
	}
	for _, e := range ends {
		if term+e == text {
			return false, true
		}
	}
	switch m.lang {
	case "de":
		// Umlaut plurals: Vertrag, Verträge, Verträgen; Mutter, Mütter. Only
		// the plural endings, so Kauf does not match Käufer.
		if umlauts.Replace(text) != text {
			for _, e := range []string{"", "e", "en"} {
				if umlauts.Replace(text) == umlauts.Replace(term)+e {
					return false, true
				}
			}
		}
	case "en":
		if strings.HasSuffix(term, "y") && strings.TrimSuffix(term, "y")+"ies" == text {
			return false, true
		}
	case "fr":
		// travail, travaux; social, sociaux.
		if stem, ok := strings.CutSuffix(term, "al"); ok && stem+"aux" == text {
			return false, true
		}
		if stem, ok := strings.CutSuffix(term, "ail"); ok && stem+"aux" == text {
			return false, true
		}
	case "pt":
		// contrato de locação, locações; animal, animais; papel, papéis.
		for from, tos := range map[string][]string{"ão": {"ões", "ães", "ãos"}, "al": {"ais"}, "el": {"éis"}, "ol": {"óis"}, "m": {"ns"}} {
			if stem, ok := strings.CutSuffix(term, from); ok {
				for _, to := range tos {
					if stem+to == text {
						return false, true
					}
				}
			}
		}
	case "es", "ca", "gl":
		// luz, luces.
		if stem, ok := strings.CutSuffix(term, "z"); ok && stem+"ces" == text {
			return false, true
		}
	case "it":
		// Italian changes the final vowel: contratto, contratti.
		base := strings.TrimRight(term, "aeio")
		if base != term && len(text) == len(term) && strings.HasPrefix(text, base) && strings.ContainsAny(text[len(base):], "aeio") {
			return false, true
		}
	}
	return false, false
}
