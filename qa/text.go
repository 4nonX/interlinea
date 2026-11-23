package qa

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Languages that write a decimal comma (1.250,00 or 1 250,00); the others
// write a decimal point (1,250.00).
var decimalComma = map[string]bool{
	"de": true, "fr": true, "es": true, "it": true, "nl": true, "pt": true, "pl": true, "cs": true,
	"sk": true, "sl": true, "hr": true, "sr": true, "bs": true, "da": true, "sv": true, "nb": true,
	"nn": true, "no": true, "fi": true, "et": true, "lv": true, "lt": true, "ru": true, "uk": true,
	"be": true, "bg": true, "ro": true, "hu": true, "el": true, "tr": true, "az": true, "id": true,
	"vi": true, "ca": true, "gl": true, "eu": true, "is": true, "lb": true, "kk": true, "ka": true,
}

// Languages that group thousands with a space (1 250,00); an ordinary
// space counts as a group separator only in these. No-break and narrow
// no-break spaces count in every language.
var spaceGroup = map[string]bool{
	"fr": true, "pl": true, "cs": true, "sk": true, "sv": true, "nb": true, "nn": true, "no": true,
	"fi": true, "et": true, "lv": true, "lt": true, "ru": true, "uk": true, "be": true, "bg": true,
	"hu": true, "kk": true, "ka": true,
}

var numberRe = regexp.MustCompile(`\d+(?:(?:[.,'’\x{00a0}\x{202f}]| )\d{3})*(?:[.,]\d+)?`)

// number is a number found in a text: as written and its value.
type number struct {
	text  string
	value string
}

// numbersIn reads the numbers of a text with the separators of the
// language (full tag, so de-CH and fr-CH use the Swiss apostrophe and a
// decimal point), so 1.250,00 in German and 1,250.00 in English have the
// same value.
func numbersIn(text, lang string) []number {
	primary, region := splitLang(lang)
	swiss := region == "ch" || region == "li"
	comma := decimalComma[primary] && !swiss
	decimal, group := byte('.'), byte(',')
	if comma {
		decimal, group = ',', '.'
	}
	var out []number
	for _, loc := range numberRe.FindAllStringIndex(text, -1) {
		raw := text[loc[0]:loc[1]]
		// An ordinary space groups digits only in languages that write
		// numbers so; elsewhere "5 100" is two numbers.
		parts := []string{raw}
		if !spaceGroup[primary] && strings.Contains(raw, " ") {
			parts = strings.Split(raw, " ")
		}
		for _, p := range parts {
			if v, ok := numberValue(p, decimal, group); ok {
				out = append(out, number{text: p, value: v})
			}
		}
	}
	return out
}

// numberValue normalizes a number to digits with an optional decimal
// point, so equal values compare equal ("1250", "1250.5").
func numberValue(s string, decimal, group byte) (string, bool) {
	var b strings.Builder
	seenDecimal := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == rune(decimal) && !seenDecimal && isLastSeparator(s, i):
			b.WriteByte('.')
			seenDecimal = true
		case r == rune(group), r == '\'', r == '’', r == ' ', r == ' ', r == ' ':
		default:
			// The other separator where a group separator could stand:
			// "1,5" in English is not 15; keep it as written.
			if r == '.' || r == ',' {
				b.WriteRune(r)
			}
		}
		i += size
	}
	v := b.String()
	if v == "" {
		return "", false
	}
	if seenDecimal {
		v = strings.TrimRight(strings.TrimRight(v, "0"), ".")
	}
	// Leading zeros are kept: "007" and "7" differ (article numbers).
	return v, true
}

func isLastSeparator(s string, i int) bool {
	return !strings.ContainsAny(s[i+1:], ".,")
}

func splitLang(tag string) (primary, region string) {
	tag = strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
	parts := strings.Split(tag, "-")
	primary = parts[0]
	for _, p := range parts[1:] {
		if len(p) == 2 || (len(p) == 3 && p[0] >= '0' && p[0] <= '9') {
			region = p
		}
	}
	return primary, region
}

func (c *checker) numbers(out *[]Issue, s Segment, src, tgt string) {
	if !c.on[NumbersMissing] && !c.on[NumbersExtra] {
		return
	}
	sn, tn := numbersIn(src, c.opts.SourceLang), numbersIn(tgt, c.opts.TargetLang)
	left := map[string]int{}
	for _, n := range tn {
		left[n.value]++
	}
	var missing []string
	for _, n := range sn {
		if left[n.value] > 0 {
			left[n.value]--
			continue
		}
		missing = append(missing, n.text)
	}
	need := map[string]int{}
	for _, n := range sn {
		need[n.value]++
	}
	var extra []string
	for _, n := range tn {
		if need[n.value] > 0 {
			need[n.value]--
			continue
		}
		extra = append(extra, n.text)
	}
	if len(missing) > 0 {
		c.add(out, s, NumbersMissing, listMessage("The number %s from the source is not in the translation.", "The numbers %s from the source are not in the translation.", missing), strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		c.add(out, s, NumbersExtra, listMessage("The number %s is not in the source.", "The numbers %s are not in the source.", extra), strings.Join(extra, ", "))
	}
}

func listMessage(one, many string, items []string) string {
	if len(items) == 1 {
		return fmt.Sprintf(one, items[0])
	}
	return fmt.Sprintf(many, strings.Join(items[:len(items)-1], ", ")+" and "+items[len(items)-1])
}

// endPunct maps sentence-final punctuation to a common form, so the
// Chinese full stop matches the Latin one.
var endPunct = map[rune]string{
	'.': ".", '。': ".", '．': ".", '!': "!", '！': "!", '?': "?", '？': "?", '؟': "?",
	':': ":", '：': ":", ';': ";", '；': ";", '…': "…", '।': ".", '۔': ".",
}

// trailingClosers are skipped when looking for the final punctuation:
// "(see below.)" ends with a full stop.
const trailingClosers = ")]}»”’\"'›」』）】"

func finalPunct(s, lang string) string {
	s = strings.TrimRightFunc(s, unicode.IsSpace)
	s = strings.TrimRight(s, trailingClosers)
	if strings.HasSuffix(s, "...") {
		return "…"
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	if r == ';' && lang == "el" {
		return "?" // the Greek question mark is a semicolon
	}
	return endPunct[r]
}

func (c *checker) punctuation(out *[]Issue, s Segment, src, tgt string) {
	if c.on[EndPunctuation] && hasLetters(src) {
		sp, tp := finalPunct(src, c.srcLang), finalPunct(tgt, c.tgtLang)
		if sp != tp {
			switch {
			case sp == "":
				c.add(out, s, EndPunctuation, fmt.Sprintf("The translation ends with “%s”; the source has no final punctuation.", tp), tp)
			case tp == "":
				c.add(out, s, EndPunctuation, fmt.Sprintf("The source ends with “%s”; the translation has no final punctuation.", sp), "")
			default:
				c.add(out, s, EndPunctuation, fmt.Sprintf("The source ends with “%s”, the translation with “%s”.", sp, tp), tp)
			}
		}
	}
	if c.on[DoubleSpace] && strings.Count(tgt, "  ") > strings.Count(src, "  ") {
		c.add(out, s, DoubleSpace, "The translation has a double space.", "  ")
	}
	if c.on[EdgeSpace] {
		lead := func(x string) bool { r, _ := utf8.DecodeRuneInString(x); return unicode.IsSpace(r) }
		trail := func(x string) bool { r, _ := utf8.DecodeLastRuneInString(x); return unicode.IsSpace(r) }
		if lead(src) != lead(tgt) {
			if lead(tgt) {
				c.add(out, s, EdgeSpace, "The translation starts with a space; the source does not.", "")
			} else {
				c.add(out, s, EdgeSpace, "The source starts with a space; the translation does not.", "")
			}
		}
		if trail(src) != trail(tgt) {
			if trail(tgt) {
				c.add(out, s, EdgeSpace, "The translation ends with a space; the source does not.", "")
			} else {
				c.add(out, s, EdgeSpace, "The source ends with a space; the translation does not.", "")
			}
		}
	}
	if c.on[SpaceBeforePunct] {
		for _, m := range spacingIssues(tgt, c.tgtLang) {
			if !strings.Contains(src, m.found) || c.tgtLang == "fr" {
				c.add(out, s, SpaceBeforePunct, m.message, m.found)
			}
		}
	}
	if c.on[Brackets] && bracketsBalanced(src) && !bracketsBalanced(tgt) {
		c.add(out, s, Brackets, "Brackets in the translation do not pair up.", "")
	}
	if c.on[RepeatedWord] {
		// A doubled word in the source ("bye bye") may rightly be one in
		// the translation too, in other words; only extra ones count.
		allowed := len(repeatedWords(src))
		for i, w := range repeatedWords(tgt) {
			if i >= allowed {
				c.add(out, s, RepeatedWord, fmt.Sprintf("“%s” is repeated.", w), w+" "+w)
			}
		}
	}
	if c.on[Capitalization] {
		sr, tr := firstLetter(src), firstLetter(tgt)
		if sr != 0 && tr != 0 && hasCase(sr) && hasCase(tr) {
			su, tu := unicode.IsUpper(sr), unicode.IsUpper(tr)
			switch {
			case su && !tu:
				c.add(out, s, Capitalization, "The source starts with a capital letter; the translation does not.", "")
			case !su && tu && c.tgtLang != "de" && c.tgtLang != "lb":
				// German nouns are capitalized, so a lower-case English
				// start may rightly become a capital in German.
				c.add(out, s, Capitalization, "The translation starts with a capital letter; the source does not.", "")
			}
		}
	}
}

func hasCase(r rune) bool { return unicode.IsUpper(r) || unicode.IsLower(r) }

// firstLetter is the first character of the text after opening quotes,
// brackets and the Spanish inverted marks; 0 if that is not a letter.
func firstLetter(s string) rune {
	for _, r := range s {
		if unicode.IsSpace(r) || strings.ContainsRune("([{«„“”‘’\"'‹¿¡「『（【-–•*", r) {
			continue
		}
		if unicode.IsLetter(r) {
			return r
		}
		return 0
	}
	return 0
}

type spacing struct{ found, message string }

// spacingIssues finds spaces before punctuation that takes none and, in
// French, missing spaces before ; : ! ? and inside « ».
func spacingIssues(t, lang string) []spacing {
	var out []spacing
	seen := map[string]bool{}
	addIssue := func(found, msg string) {
		if !seen[found] {
			seen[found] = true
			out = append(out, spacing{found, msg})
		}
	}
	rs := []rune(t)
	isSpace := func(r rune) bool { return r == ' ' || r == ' ' || r == ' ' || r == '\t' }
	followedByBoundary := func(i int) bool {
		return i+1 >= len(rs) || unicode.IsSpace(rs[i+1]) || strings.ContainsRune(trailingClosers, rs[i+1])
	}
	for i, r := range rs {
		switch r {
		case ',', '.':
			if i > 0 && rs[i-1] == ' ' && followedByBoundary(i) && !(r == '.' && i+1 < len(rs) && rs[i+1] == '.') && !(i > 1 && rs[i-2] == '.') {
				addIssue(" "+string(r), fmt.Sprintf("A space before “%s”.", string(r)))
			}
		case ';', ':', '!', '?':
			if lang == "fr" {
				if i == 0 || isSpace(rs[i-1]) || strings.ContainsRune(";:!?…", rs[i-1]) {
					continue
				}
				if r == ':' && !followedByBoundary(i) {
					continue // 10:30, http://
				}
				if !unicode.IsLetter(rs[i-1]) && !unicode.IsDigit(rs[i-1]) && !strings.ContainsRune(trailingClosers, rs[i-1]) {
					continue
				}
				addIssue(string(rs[i-1])+string(r), fmt.Sprintf("French needs a (narrow) no-break space before “%s”.", string(r)))
				continue
			}
			if i > 0 && rs[i-1] == ' ' && followedByBoundary(i) {
				addIssue(" "+string(r), fmt.Sprintf("A space before “%s”.", string(r)))
			}
		case '«':
			if lang == "fr" && i+1 < len(rs) && !isSpace(rs[i+1]) {
				addIssue("«"+string(rs[i+1]), "French needs a no-break space after “«”.")
			}
		case '»':
			if lang == "fr" && i > 0 && !isSpace(rs[i-1]) {
				addIssue(string(rs[i-1])+"»", "French needs a no-break space before “»”.")
			}
		}
	}
	return out
}

var bracketPairs = map[rune]rune{')': '(', ']': '[', '}': '{', '）': '（', '」': '「', '』': '『', '】': '【'}

func bracketsBalanced(s string) bool {
	var stack []rune
	for _, r := range s {
		switch r {
		case '(', '[', '{', '（', '「', '『', '【':
			stack = append(stack, r)
		case ')', ']', '}', '）', '」', '』', '】':
			if len(stack) == 0 || stack[len(stack)-1] != bracketPairs[r] {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0
}

// words splits a text into words: letters and marks, with apostrophes
// and hyphens inside a word kept. Offsets are bytes.
type word struct {
	text       string
	start, end int
}

func words(s string) []word {
	var out []word
	start := -1
	isPart := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r) }
	for i := 0; i <= len(s); {
		var r rune
		size := 1
		if i < len(s) {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		inner := false
		if i < len(s) && start >= 0 && (r == '\'' || r == '’' || r == '-' || r == '­') {
			next, _ := utf8.DecodeRuneInString(s[i+size:])
			inner = i+size < len(s) && isPart(next)
		}
		switch {
		case i < len(s) && (isPart(r) || inner):
			if start < 0 {
				start = i
			}
		case start >= 0:
			out = append(out, word{s[start:i], start, i})
			start = -1
		}
		if i == len(s) {
			break
		}
		i += size
	}
	return out
}

func repeatedWords(s string) []string {
	ws := words(s)
	var out []string
	for i := 1; i < len(ws); i++ {
		a, b := ws[i-1], ws[i]
		if strings.TrimSpace(s[a.end:b.start]) != "" {
			continue
		}
		if !hasLetters(a.text) || !strings.EqualFold(a.text, b.text) {
			continue
		}
		out = append(out, b.text)
	}
	return out
}

// spelling checks the words of the translation. Words also in the source
// (names, product names) and target terms of the termbase pass, as do
// words in all capitals, with digits, and the user's own words.
func (c *checker) spelling(out *[]Issue, s Segment, tgt string) {
	if !c.on[Spelling] || c.opts.Spell == nil {
		return
	}
	known := map[string]bool{}
	for _, w := range words(s.Source.Plain()) {
		known[strings.ToLower(w.text)] = true
	}
	if c.opts.Terms != nil {
		for _, h := range c.opts.Terms.Recognize(s.Source.Plain(), c.opts.SourceLang, c.opts.TargetLang) {
			for _, t := range h.Targets {
				for _, w := range words(t.Text) {
					known[strings.ToLower(w.text)] = true
				}
			}
		}
	}
	reported := map[string]bool{}
	for _, w := range words(tgt) {
		if skipSpelling(tgt, w) {
			continue
		}
		lw := strings.ToLower(w.text)
		if known[lw] || c.ignore[lw] || reported[w.text] {
			continue
		}
		if c.spellOK(w.text) {
			continue
		}
		// An abbreviation the dictionary has with its dot ("z.", "etc.").
		if strings.HasPrefix(tgt[w.end:], ".") && c.spellOK(w.text+".") {
			continue
		}
		reported[w.text] = true
		*out = append(*out, Issue{
			Segment: s.ID, Kind: Spelling, Severity: severityOf(Spelling),
			Message:     fmt.Sprintf("“%s” is not in the dictionary.", w.text),
			Found:       w.text,
			Suggestions: c.suggest(w.text),
		})
	}
}

func skipSpelling(text string, w word) bool {
	hasLetter, allUpper := false, true
	for _, r := range w.text {
		if unicode.IsDigit(r) {
			return true
		}
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				allUpper = false
			}
		}
	}
	if !hasLetter || (allUpper && utf8.RuneCountInString(w.text) > 1) {
		return true
	}
	// Inside a web or e-mail address.
	lo, hi := w.start, w.end
	for lo > 0 && !unicode.IsSpace(rune(text[lo-1])) {
		lo--
	}
	for hi < len(text) && !unicode.IsSpace(rune(text[hi])) {
		hi++
	}
	tok := text[lo:hi]
	return strings.Contains(tok, "@") || strings.Contains(tok, "://") || strings.HasPrefix(tok, "www.")
}

// spellOK and suggest cache the dictionary's answers for a run: a word
// recurs across segments, and suggestions take the most time.
func (c *checker) spellOK(w string) bool {
	if ok, seen := c.spellCache[w]; seen {
		return ok
	}
	ok := c.opts.Spell.Spell(w)
	c.spellCache[w] = ok
	return ok
}

func (c *checker) suggest(w string) []string {
	if s, seen := c.sugCache[w]; seen {
		return s
	}
	var s []string
	if len(c.sugCache) < maxSuggested {
		s = c.opts.Spell.Suggest(w, 5)
	}
	c.sugCache[w] = s
	return s
}

// maxSuggested bounds the misspellings per run that get suggestions, so
// a document in the wrong language does not take minutes.
const maxSuggested = 200
