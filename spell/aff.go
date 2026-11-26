// Package spell checks spelling with Hunspell dictionaries (.aff and
// .dic), the format of LibreOffice, Firefox, Chrome and macOS, so every
// dictionary a translator already has works.
//
// It is an implementation of Hunspell's algorithms in Go, not a binding:
// affixes with continuation classes (two suffixes, or two prefixes with
// COMPLEXPREFIXES), circumfixes, compounding by flags (COMPOUNDFLAG,
// COMPOUNDBEGIN/MIDDLE/END with their checks) and by COMPOUNDRULE,
// FORBIDDENWORD, KEEPCASE, NEEDAFFIX, ONLYINCOMPOUND, NOSUGGEST, BREAK,
// ICONV/OCONV, IGNORE, CHECKSHARPS, Turkic casing, flag aliases (AF), all
// four flag types and the 8-bit character sets Hunspell reads.
// Suggestions follow Hunspell's order: REP, MAP, keyboard (KEY), swapped,
// missing, extra and moved characters, TRY replacements, word splits,
// then n-gram similarity, and PHONE rules where the dictionary has them.
package spell

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// flag is a decoded flag: a byte (FLAG default), two bytes (long), a
// number (num) or a character (UTF-8).
type flag uint32

// flagZero is a numeric flag written 0, which a file may use as an affix
// class; no flag written in the file is this large.
const flagZero flag = 1 << 16

type flagSet []flag

func (s flagSet) has(f flag) bool {
	if f == 0 {
		return false
	}
	for _, x := range s {
		if x == f {
			return true
		}
	}
	return false
}

func union(a, b flagSet) flagSet {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		return b
	}
	out := make(flagSet, 0, len(a)+len(b))
	out = append(out, a...)
	for _, f := range b {
		if !out.has(f) {
			out = append(out, f)
		}
	}
	return out
}

type flagMode int

const (
	flagShort flagMode = iota
	flagLong
	flagNum
	flagUTF8
)

// condition is an affix condition: a sequence of single-character
// patterns (a character, "." for any, [abc] or [^abc]).
type condPart struct {
	any   bool
	neg   bool
	chars []rune
}

type condition []condPart

func parseCondition(s string) (condition, error) {
	if s == "." || s == "" {
		return nil, nil
	}
	var out condition
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '.':
			out = append(out, condPart{any: true})
		case '[':
			j := i + 1
			p := condPart{}
			if j < len(rs) && rs[j] == '^' {
				p.neg = true
				j++
			}
			for j < len(rs) && rs[j] != ']' {
				p.chars = append(p.chars, rs[j])
				j++
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unclosed [ in condition %q", s)
			}
			out = append(out, p)
			i = j
		default:
			out = append(out, condPart{chars: []rune{rs[i]}})
		}
	}
	return out, nil
}

func (p condPart) match(r rune) bool {
	if p.any {
		return true
	}
	in := false
	for _, c := range p.chars {
		if c == r {
			in = true
			break
		}
	}
	return in != p.neg
}

// matchStart reports whether the condition matches the start of s
// (prefixes); matchEnd the end (suffixes).
func (c condition) matchStart(s string) bool {
	i := 0
	for _, r := range s {
		if i == len(c) {
			return true
		}
		if !c[i].match(r) {
			return false
		}
		i++
	}
	return i == len(c)
}

func (c condition) matchEnd(s string) bool {
	i := len(c) - 1
	for end := len(s); i >= 0; i-- {
		if end == 0 {
			return false
		}
		r, size := utf8.DecodeLastRuneInString(s[:end])
		if !c[i].match(r) {
			return false
		}
		end -= size
	}
	return true
}

type affix struct {
	flag   flag
	prefix bool
	cross  bool
	strip  string
	add    string
	cond   condition
	cont   flagSet
}

type repPattern struct {
	from, to   string
	start, end bool
}

type breakPattern struct {
	text       string
	start, end bool
}

type convPattern struct {
	from, to   string
	start, end bool
}

type conv []convPattern

// apply replaces, left to right, the longest matching pattern at each
// position ("_" at either end of a pattern anchors it to the word's
// start or end).
func (c conv) apply(word string) string {
	if len(c) == 0 {
		return word
	}
	var b strings.Builder
	for pos := 0; pos < len(word); {
		best := -1
		for i, p := range c {
			if p.start && pos != 0 {
				continue
			}
			if !strings.HasPrefix(word[pos:], p.from) {
				continue
			}
			if p.end && pos+len(p.from) != len(word) {
				continue
			}
			if best < 0 || len(p.from) > len(c[best].from) {
				best = i
			}
		}
		if best >= 0 && c[best].from != "" {
			b.WriteString(c[best].to)
			pos += len(c[best].from)
			continue
		}
		_, size := utf8.DecodeRuneInString(word[pos:])
		b.WriteString(word[pos : pos+size])
		pos += size
	}
	return b.String()
}

type ruleElem struct {
	flag  flag
	quant byte // 0, '*' or '?'
}

type cpdPattern struct {
	left, right   string
	leftFlag      flag
	rightFlag     flag
	leftUnchanged bool // "0": the left part ends with its unmodified stem
	replacement   string
}

type phoneRule struct {
	letters   []condPart
	lookahead int
	start     bool
	end       bool
	replace   string
}

// aff holds the settings and affixes of a .aff file.
type aff struct {
	charset *[128]rune // nil for UTF-8
	mode    flagMode
	aliases []flagSet

	forbidden, keepcase, nosuggest, needaffix, circumfix, onlyInCompound  flag
	compoundFlag, compoundBegin, compoundMiddle, compoundEnd              flag
	compoundPermit, compoundForbid, forceUCase, compoundRoot, warn, subst flag

	try, key, lang, wordchars string
	ignore                    map[rune]bool
	rep                       []repPattern
	maps                      [][]string
	breaks                    []breakPattern
	iconv, oconv              conv
	compoundRules             [][]ruleElem
	ruleFlags                 flagSet
	patterns                  []cpdPattern
	phone                     map[rune][]phoneRule

	compoundMin, compoundWordMax, compoundSyllableMax int
	syllableVowels                                    string
	checkDup, checkRep, checkCase, checkTriple        bool
	simplifiedTriple, checkSharps, complexPrefixes    bool
	fullStrip, noSplitSugs, onlyMaxDiff, forbidWarn   bool
	maxCpdSugs, maxNgramSugs, maxDiff                 int
	turkic                                            bool

	prefixes, suffixes []*affix
	pfxByAdd, sfxByAdd map[string][]*affix
	byFlag             map[flag][]*affix
	// pfxTrie and sfxTrie index the affixes by their text, forwards and
	// backwards, so a word's possible affixes are found in one walk.
	pfxTrie, sfxTrie trieNode
}

type trieNode struct {
	keys     []byte
	children []*trieNode
	affixes  []*affix
}

func (n *trieNode) child(b byte, create bool) *trieNode {
	for i, k := range n.keys {
		if k == b {
			return n.children[i]
		}
	}
	if !create {
		return nil
	}
	c := &trieNode{}
	n.keys = append(n.keys, b)
	n.children = append(n.children, c)
	return c
}

func (n *trieNode) insert(key string, x *affix, reverse bool) {
	cur := n
	for i := 0; i < len(key); i++ {
		b := key[i]
		if reverse {
			b = key[len(key)-1-i]
		}
		cur = cur.child(b, true)
	}
	cur.affixes = append(cur.affixes, x)
}

// line is a line of a .aff or .dic file: its fields as raw bytes.
type line struct {
	no     int
	fields [][]byte
}

func splitLines(data []byte) [][]byte {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	return bytes.FieldsFunc(data, func(r rune) bool { return r == '\n' || r == '\r' })
}

func fieldsOf(b []byte) [][]byte {
	return bytes.FieldsFunc(b, func(r rune) bool { return r == ' ' || r == '\t' })
}

func (a *aff) decode(b []byte) string {
	if a.charset == nil {
		if utf8.Valid(b) {
			return string(b)
		}
		return strings.ToValidUTF8(string(b), "�")
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
		} else {
			sb.WriteRune(a.charset[c-0x80])
		}
	}
	return sb.String()
}

func (a *aff) stripIgnored(s string) string {
	if len(a.ignore) == 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if a.ignore[r] {
			return -1
		}
		return r
	}, s)
}

// parseFlags decodes a flag field; with aliases (AF), a number is an
// index into them.
func (a *aff) parseFlags(b []byte) (flagSet, error) {
	if len(b) == 0 {
		return nil, nil
	}
	if len(a.aliases) > 0 && isDigits(b) {
		n, _ := strconv.Atoi(string(b))
		if n < 1 || n > len(a.aliases) {
			return nil, fmt.Errorf("flag alias %d out of range (1 to %d)", n, len(a.aliases))
		}
		return a.aliases[n-1], nil
	}
	return a.parseRawFlags(b)
}

func (a *aff) parseRawFlags(b []byte) (flagSet, error) {
	var out flagSet
	switch a.mode {
	case flagShort:
		for _, c := range b {
			out = append(out, flag(c))
		}
	case flagLong:
		for i := 0; i+1 < len(b); i += 2 {
			out = append(out, flag(b[i])<<8|flag(b[i+1]))
		}
		if len(b)%2 == 1 {
			// Hunspell ignores the odd byte as well.
			return out, nil
		}
	case flagNum:
		// Read as Hunspell reads them, with C's atoi: the digits a part
		// starts with, 0 where it starts with none (Hunspell warns of flag
		// 0 and keeps it), and a number too large cut to 16 bits. The
		// dictionaries LibreOffice ships rely on it: Turkish names an affix
		// class 0, and Danish has "A/S", whose S" is no number.
		for _, p := range bytes.Split(b, []byte(",")) {
			p = bytes.TrimSpace(p)
			if len(p) == 0 {
				continue
			}
			f := flag(atoi(p) & 0xFFFF)
			if f == 0 {
				// 0 stands for "no flag" in the options here; a flag 0
				// of the file is a flag like any other.
				f = flagZero
			}
			out = append(out, f)
		}
	case flagUTF8:
		s := string(b)
		if !utf8.ValidString(s) {
			return nil, fmt.Errorf("flags %q are not UTF-8", b)
		}
		for _, r := range s {
			out = append(out, flag(r))
		}
	}
	return out, nil
}

func (a *aff) parseFlag(b []byte) (flag, error) {
	fs, err := a.parseRawFlags(b)
	if err != nil {
		return 0, err
	}
	if len(fs) == 0 {
		return 0, fmt.Errorf("empty flag")
	}
	return fs[0], nil
}

func isDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(b) > 0
}

// errors of the reader, wrapped with the line number.
var errAff = errors.New("spell: .aff")

func affErr(no int, format string, args ...any) error {
	return fmt.Errorf("%w line %d: %s", errAff, no, fmt.Sprintf(format, args...))
}

func parseAff(data []byte) (*aff, error) {
	a := &aff{
		charset:      charsets["ISO8859-1"],
		compoundMin:  3,
		maxCpdSugs:   3,
		maxNgramSugs: 4,
		maxDiff:      -1,
		key:          "qwertyuiop|asdfghjkl|zxcvbnm",
		pfxByAdd:     map[string][]*affix{},
		sfxByAdd:     map[string][]*affix{},
		byFlag:       map[flag][]*affix{},
	}
	if bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		a.charset = nil
	}
	raw := splitLines(data)
	var lines []line
	for i, l := range raw {
		f := fieldsOf(l)
		if len(f) == 0 || f[0][0] == '#' {
			continue
		}
		lines = append(lines, line{no: i + 1, fields: f})
	}
	// SET and FLAG decide how everything else is read, wherever they are.
	for _, l := range lines {
		if len(l.fields) < 2 {
			continue
		}
		switch string(l.fields[0]) {
		case "SET":
			name := strings.ToUpper(string(l.fields[1]))
			switch name {
			case "UTF-8", "UTF8":
				a.charset = nil
			default:
				name = strings.ReplaceAll(name, "ISO-8859-", "ISO8859-")
				name = strings.ReplaceAll(name, "CP1251", "MICROSOFT-CP1251")
				name = strings.ReplaceAll(name, "MICROSOFT-MICROSOFT-", "MICROSOFT-")
				if name == "WINDOWS-1251" {
					name = "MICROSOFT-CP1251"
				}
				if name == "TIS-620" || name == "TIS620" {
					name = "TIS620-2533"
				}
				cs, ok := charsets[name]
				if !ok {
					return nil, affErr(l.no, "character set %s is not supported", l.fields[1])
				}
				a.charset = cs
			}
		case "FLAG":
			switch strings.ToLower(string(l.fields[1])) {
			case "long":
				a.mode = flagLong
			case "num":
				a.mode = flagNum
			case "utf-8", "utf8":
				a.mode = flagUTF8
			default:
				return nil, affErr(l.no, "unknown flag type %s", l.fields[1])
			}
		}
	}
	// Flag aliases come before the affixes that use them.
	for i := 0; i < len(lines); i++ {
		if string(lines[i].fields[0]) != "AF" || len(lines[i].fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(string(lines[i].fields[1]))
		if err != nil {
			continue // an AF entry, read below
		}
		for j := 1; j <= n && i+j < len(lines); j++ {
			l := lines[i+j]
			if string(l.fields[0]) != "AF" {
				// Hunspell accepts fewer aliases than announced.
				break
			}
			var fs flagSet
			if len(l.fields) > 1 {
				var err error
				if fs, err = a.parseRawFlags(l.fields[1]); err != nil {
					return nil, affErr(l.no, "%v", err)
				}
			}
			a.aliases = append(a.aliases, fs)
		}
		break
	}
	breakSet := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		name := string(l.fields[0])
		arg := func(n int) []byte {
			if n < len(l.fields) {
				return l.fields[n]
			}
			return nil
		}
		// table reads the n lines that follow a counted directive.
		table := func() ([]line, error) {
			n, err := strconv.Atoi(string(arg(1)))
			if err != nil || n < 0 {
				return nil, affErr(l.no, "%s needs a count", name)
			}
			var out []line
			for j := 1; j <= n; j++ {
				if i+j >= len(lines) || string(lines[i+j].fields[0]) != name {
					// Hunspell accepts fewer lines than announced.
					break
				}
				out = append(out, lines[i+j])
			}
			i += len(out)
			return out, nil
		}
		flagArg := func(dst *flag) error {
			if arg(1) == nil {
				return affErr(l.no, "%s needs a flag", name)
			}
			f, err := a.parseFlag(arg(1))
			if err != nil {
				return affErr(l.no, "%s: %v", name, err)
			}
			*dst = f
			return nil
		}
		intArg := func(dst *int) error {
			n, err := strconv.Atoi(string(arg(1)))
			if err != nil {
				return affErr(l.no, "%s needs a number", name)
			}
			*dst = n
			return nil
		}
		var err error
		switch name {
		case "SET", "FLAG", "AF", "AM", "NAME", "VERSION", "HOME", "LEMMA_PRESENT", "COMPOUNDMORESUFFIXES", "LANG":
			if name == "LANG" {
				a.lang = string(arg(1))
				p := strings.ToLower(strings.SplitN(strings.ReplaceAll(a.lang, "-", "_"), "_", 2)[0])
				a.turkic = p == "tr" || p == "az" || p == "crh" || p == "tt" || p == "kk"
			}
		case "TRY":
			a.try = a.decode(arg(1))
		case "KEY":
			a.key = a.decode(arg(1))
		case "WORDCHARS":
			a.wordchars = a.decode(arg(1))
		case "IGNORE":
			a.ignore = map[rune]bool{}
			for _, r := range a.decode(arg(1)) {
				a.ignore[r] = true
			}
		case "FORBIDDENWORD":
			err = flagArg(&a.forbidden)
		case "KEEPCASE":
			err = flagArg(&a.keepcase)
		case "NOSUGGEST":
			err = flagArg(&a.nosuggest)
		case "NEEDAFFIX", "PSEUDOROOT":
			err = flagArg(&a.needaffix)
		case "CIRCUMFIX":
			err = flagArg(&a.circumfix)
		case "ONLYINCOMPOUND":
			err = flagArg(&a.onlyInCompound)
		case "COMPOUNDFLAG":
			err = flagArg(&a.compoundFlag)
		case "COMPOUNDBEGIN", "COMPOUNDFIRST":
			err = flagArg(&a.compoundBegin)
		case "COMPOUNDMIDDLE":
			err = flagArg(&a.compoundMiddle)
		case "COMPOUNDEND", "COMPOUNDLAST":
			err = flagArg(&a.compoundEnd)
		case "COMPOUNDPERMITFLAG":
			err = flagArg(&a.compoundPermit)
		case "COMPOUNDFORBIDFLAG":
			err = flagArg(&a.compoundForbid)
		case "FORCEUCASE":
			err = flagArg(&a.forceUCase)
		case "COMPOUNDROOT":
			err = flagArg(&a.compoundRoot)
		case "WARN":
			err = flagArg(&a.warn)
		case "SUBSTANDARD":
			err = flagArg(&a.subst)
		case "COMPOUNDMIN":
			err = intArg(&a.compoundMin)
			if a.compoundMin < 1 {
				a.compoundMin = 1
			}
		case "COMPOUNDWORDMAX":
			err = intArg(&a.compoundWordMax)
		case "MAXCPDSUGS":
			err = intArg(&a.maxCpdSugs)
		case "MAXNGRAMSUGS":
			err = intArg(&a.maxNgramSugs)
		case "MAXDIFF":
			err = intArg(&a.maxDiff)
		case "COMPOUNDSYLLABLE":
			if err = intArg(&a.compoundSyllableMax); err == nil {
				a.syllableVowels = a.decode(arg(2))
			}
		case "SYLLABLENUM":
			// Hungarian: affixes that count as syllables; not needed
			// for checking.
		case "CHECKCOMPOUNDDUP":
			a.checkDup = true
		case "CHECKCOMPOUNDREP":
			a.checkRep = true
		case "CHECKCOMPOUNDCASE":
			a.checkCase = true
		case "CHECKCOMPOUNDTRIPLE":
			a.checkTriple = true
		case "SIMPLIFIEDTRIPLE":
			a.simplifiedTriple = true
		case "CHECKSHARPS":
			a.checkSharps = true
		case "COMPLEXPREFIXES":
			a.complexPrefixes = true
		case "FULLSTRIP":
			a.fullStrip = true
		case "NOSPLITSUGS":
			a.noSplitSugs = true
		case "ONLYMAXDIFF":
			a.onlyMaxDiff = true
		case "FORBIDWARN":
			a.forbidWarn = true
		case "SUGSWITHDOTS", "ONLYROOT", "GENERATE", "CHECKNUM", "NOSPLITSUG":
			// Obsolete or for morphological generation only.
		case "REP":
			var t []line
			if t, err = table(); err == nil {
				for _, r := range t {
					if len(r.fields) < 3 {
						continue
					}
					p := repPattern{from: a.decode(r.fields[1]), to: strings.ReplaceAll(a.decode(r.fields[2]), "_", " ")}
					if strings.HasPrefix(p.from, "^") {
						p.start, p.from = true, p.from[1:]
					}
					if strings.HasSuffix(p.from, "$") {
						p.end, p.from = true, p.from[:len(p.from)-1]
					}
					if p.from != "" {
						a.rep = append(a.rep, p)
					}
				}
			}
		case "MAP":
			var t []line
			if t, err = table(); err == nil {
				for _, r := range t {
					if len(r.fields) < 2 {
						continue
					}
					var group []string
					s := []rune(a.decode(r.fields[1]))
					for k := 0; k < len(s); k++ {
						if s[k] == '(' {
							end := k + 1
							for end < len(s) && s[end] != ')' {
								end++
							}
							group = append(group, string(s[k+1:end]))
							k = end
							continue
						}
						group = append(group, string(s[k]))
					}
					a.maps = append(a.maps, group)
				}
			}
		case "BREAK":
			breakSet = true
			var t []line
			if t, err = table(); err == nil {
				for _, r := range t {
					if len(r.fields) < 2 {
						continue
					}
					p := breakPattern{text: a.decode(r.fields[1])}
					if strings.HasPrefix(p.text, "^") {
						p.start, p.text = true, p.text[1:]
					}
					if strings.HasSuffix(p.text, "$") {
						p.end, p.text = true, p.text[:len(p.text)-1]
					}
					if p.text != "" {
						a.breaks = append(a.breaks, p)
					}
				}
			}
		case "ICONV", "OCONV":
			var t []line
			if t, err = table(); err == nil {
				var c conv
				for _, r := range t {
					if len(r.fields) < 3 {
						continue
					}
					p := convPattern{from: a.decode(r.fields[1]), to: strings.ReplaceAll(a.decode(r.fields[2]), "_", " ")}
					if strings.HasPrefix(p.from, "_") && len(p.from) > 1 {
						p.start, p.from = true, p.from[1:]
					}
					if strings.HasSuffix(p.from, "_") && len(p.from) > 1 {
						p.end, p.from = true, p.from[:len(p.from)-1]
					}
					c = append(c, p)
				}
				if name == "ICONV" {
					a.iconv = c
				} else {
					a.oconv = c
				}
			}
		case "COMPOUNDRULE":
			var t []line
			if t, err = table(); err == nil {
				for _, r := range t {
					if len(r.fields) < 2 {
						continue
					}
					rule, perr := a.parseRule(r.fields[1])
					if perr != nil {
						return nil, affErr(r.no, "%v", perr)
					}
					a.compoundRules = append(a.compoundRules, rule)
					for _, e := range rule {
						if !a.ruleFlags.has(e.flag) {
							a.ruleFlags = append(a.ruleFlags, e.flag)
						}
					}
				}
			}
		case "CHECKCOMPOUNDPATTERN":
			var t []line
			if t, err = table(); err == nil {
				for _, r := range t {
					if len(r.fields) < 3 {
						continue
					}
					p := cpdPattern{}
					left, lf, _ := bytes.Cut(r.fields[1], []byte("/"))
					right, rf, _ := bytes.Cut(r.fields[2], []byte("/"))
					p.left, p.right = a.decode(left), a.decode(right)
					if p.left == "0" {
						p.left, p.leftUnchanged = "", true
					}
					if p.right == "0" {
						p.right = ""
					}
					if len(lf) > 0 {
						if p.leftFlag, err = a.parseFlag(lf); err != nil {
							return nil, affErr(r.no, "%v", err)
						}
					}
					if len(rf) > 0 {
						if p.rightFlag, err = a.parseFlag(rf); err != nil {
							return nil, affErr(r.no, "%v", err)
						}
					}
					if len(r.fields) > 3 && r.fields[3][0] != '#' {
						p.replacement = a.decode(r.fields[3])
					}
					a.patterns = append(a.patterns, p)
				}
			}
		case "PHONE":
			var t []line
			if t, err = table(); err == nil {
				a.phone = map[rune][]phoneRule{}
				for _, r := range t {
					if len(r.fields) < 3 {
						continue
					}
					rule, ok := parsePhoneRule(a.decode(r.fields[1]), a.decode(r.fields[2]))
					if !ok || len(rule.letters) == 0 {
						continue
					}
					for _, c := range rule.letters[0].chars {
						a.phone[c] = append(a.phone[c], rule)
					}
				}
			}
		case "PFX", "SFX":
			if err = a.readAffixes(lines, &i); err != nil {
				return nil, err
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if !breakSet {
		a.breaks = []breakPattern{{text: "-"}, {text: "-", start: true}, {text: "-", end: true}}
	}
	return a, nil
}

func (a *aff) parseRule(b []byte) ([]ruleElem, error) {
	var out []ruleElem
	for i := 0; i < len(b); {
		var f flag
		if b[i] == '(' {
			end := bytes.IndexByte(b[i:], ')')
			if end < 0 {
				return nil, fmt.Errorf("unclosed ( in COMPOUNDRULE %q", b)
			}
			var err error
			if f, err = a.parseFlag(b[i+1 : i+end]); err != nil {
				return nil, err
			}
			i += end + 1
		} else if b[i] == '*' || b[i] == '?' {
			return nil, fmt.Errorf("misplaced %c in COMPOUNDRULE %q", b[i], b)
		} else {
			size := 1
			if a.mode == flagUTF8 {
				_, size = utf8.DecodeRune(b[i:])
			} else if a.mode == flagLong {
				size = 2
			}
			if i+size > len(b) {
				size = len(b) - i
			}
			var err error
			if f, err = a.parseFlag(b[i : i+size]); err != nil {
				return nil, err
			}
			i += size
		}
		e := ruleElem{flag: f}
		if i < len(b) && (b[i] == '*' || b[i] == '?') {
			e.quant = b[i]
			i++
		}
		out = append(out, e)
	}
	return out, nil
}

func (a *aff) readAffixes(lines []line, i *int) error {
	head := lines[*i]
	if len(head.fields) < 4 {
		return affErr(head.no, "%s header needs a flag, Y or N and a count", head.fields[0])
	}
	kind := string(head.fields[0])
	f, err := a.parseFlag(head.fields[1])
	if err != nil {
		return affErr(head.no, "%v", err)
	}
	cross := head.fields[2][0] == 'Y'
	n, err := strconv.Atoi(string(head.fields[3]))
	if err != nil {
		return affErr(head.no, "%s header needs a count", kind)
	}
	consumed := 0
	for j := 1; j <= n; j++ {
		if *i+j >= len(lines) {
			break
		}
		l := lines[*i+j]
		if string(l.fields[0]) != kind || !bytes.Equal(l.fields[1], head.fields[1]) {
			// Hunspell accepts fewer entries than announced.
			break
		}
		consumed++
		if len(l.fields) < 4 {
			return affErr(l.no, "%s entry needs a strip and an affix", kind)
		}
		x := &affix{flag: f, prefix: kind == "PFX", cross: cross}
		if s := a.decode(l.fields[2]); s != "0" {
			x.strip = s
		}
		add, cont, hasCont := bytes.Cut(l.fields[3], []byte("/"))
		if hasCont {
			if x.cont, err = a.parseFlags(cont); err != nil {
				return affErr(l.no, "%v", err)
			}
		}
		if s := a.decode(add); s != "0" {
			x.add = a.stripIgnored(s)
		}
		cond := "."
		if len(l.fields) > 4 {
			cond = a.decode(l.fields[4])
		}
		if x.cond, err = parseCondition(cond); err != nil {
			return affErr(l.no, "%v", err)
		}
		if x.prefix {
			a.prefixes = append(a.prefixes, x)
			a.pfxByAdd[x.add] = append(a.pfxByAdd[x.add], x)
			a.pfxTrie.insert(x.add, x, false)
		} else {
			a.suffixes = append(a.suffixes, x)
			a.sfxByAdd[x.add] = append(a.sfxByAdd[x.add], x)
			a.sfxTrie.insert(x.add, x, true)
		}
		a.byFlag[f] = append(a.byFlag[f], x)
	}
	*i += consumed
	return nil
}

// parsePhoneRule reads a PHONE rule: letters, an optional group in
// parentheses, "-" for each letter matched but not consumed, "^" and "$"
// anchors, "<" and a priority digit (read, not used). "_" replaces with
// nothing.
func parsePhoneRule(search, replace string) (phoneRule, bool) {
	r := phoneRule{}
	if replace != "_" {
		r.replace = replace
	}
	rs := []rune(search)
	i := 0
	for i < len(rs) {
		c := rs[i]
		switch {
		case c == '(':
			end := i + 1
			var group []rune
			for end < len(rs) && rs[end] != ')' {
				group = append(group, rs[end])
				end++
			}
			if end >= len(rs) {
				return r, false
			}
			r.letters = append(r.letters, condPart{chars: group})
			i = end + 1
		case c == '-':
			r.lookahead++
			i++
		case c == '^':
			r.start = true
			i++
		case c == '$':
			r.end = true
			i++
		case c == '<' || (c >= '0' && c <= '9'):
			i++
		default:
			r.letters = append(r.letters, condPart{chars: []rune{c}})
			i++
		}
	}
	if r.lookahead > len(r.letters) {
		r.lookahead = len(r.letters)
	}
	return r, true
}

// atoi is C's atoi: an optional sign and the digits a text starts with; 0
// where there are none.
func atoi(b []byte) int {
	neg := false
	if len(b) > 0 && (b[0] == '-' || b[0] == '+') {
		neg = b[0] == '-'
		b = b[1:]
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' || n > 1<<24 {
			break
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		return -n
	}
	return n
}
