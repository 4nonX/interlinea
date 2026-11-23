// Package qa checks translations the way the QA tools of Trados, memoQ
// and Xbench do: tags, numbers, punctuation and spacing, length,
// untranslated text, consistency across segments, terminology, spelling
// and rules of the user's own (regular expressions).
//
// Check takes all segments of a document so it can compare them with each
// other; CheckSegment checks one segment on its own, as an editor does
// while the translator types.
package qa

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/term"
)

// Kind names a check.
type Kind string

// The checks. Each issue names the check that found it.
const (
	Empty            Kind = "empty"
	Untranslated     Kind = "untranslated"
	TagsMissing      Kind = "tags-missing"
	TagsExtra        Kind = "tags-extra"
	TagOrder         Kind = "tag-order"
	NumbersMissing   Kind = "numbers-missing"
	NumbersExtra     Kind = "numbers-extra"
	EndPunctuation   Kind = "end-punctuation"
	DoubleSpace      Kind = "double-space"
	EdgeSpace        Kind = "edge-space"
	SpaceBeforePunct Kind = "space-before-punctuation"
	Brackets         Kind = "brackets"
	RepeatedWord     Kind = "repeated-word"
	Capitalization   Kind = "capitalization"
	Length           Kind = "length"
	LengthRatio      Kind = "length-ratio"
	Inconsistent     Kind = "inconsistent"
	InconsistentSrc  Kind = "inconsistent-source"
	TermMissing      Kind = "term-missing"
	TermForbidden    Kind = "term-forbidden"
	Spelling         Kind = "spelling"
	Rule             Kind = "rule"
)

// Kinds lists the checks with a short description, in the order reports
// group them.
var Kinds = []struct {
	Kind        Kind   `json:"kind"`
	Name        string `json:"name"`
	Default     bool   `json:"default"`
	Description string `json:"description"`
}{
	{Empty, "Empty translation", true, "A segment to translate has no translation."},
	{Untranslated, "Same as source", true, "The translation is the source text unchanged."},
	{TagsMissing, "Missing tags", true, "Tags of the source are not in the translation."},
	{TagsExtra, "Extra tags", true, "The translation has tags the source does not."},
	{TagOrder, "Tag order", true, "Paired tags are nested differently than in the source."},
	{NumbersMissing, "Missing numbers", true, "A number of the source is not in the translation (separators of each language understood)."},
	{NumbersExtra, "Extra numbers", true, "The translation has a number the source does not."},
	{EndPunctuation, "End punctuation", true, "The translation ends with different punctuation than the source."},
	{DoubleSpace, "Double spaces", true, "The translation has double spaces the source does not."},
	{EdgeSpace, "Leading and trailing spaces", true, "Spaces at the start or end differ from the source."},
	{SpaceBeforePunct, "Space before punctuation", true, "A space before a comma or full stop, or in French a missing space before ; : ! ?"},
	{Brackets, "Unpaired brackets", true, "Brackets in the translation do not pair up."},
	{RepeatedWord, "Repeated words", true, "The same word twice in a row."},
	{Capitalization, "Capital letter at the start", true, "The source starts with a capital letter and the translation does not, or the other way round."},
	{Length, "Length limit", true, "The translation is longer than the file allows."},
	{LengthRatio, "Length compared to the source", false, "The translation is much longer or shorter than the source."},
	{Inconsistent, "Inconsistent translations", true, "The same source text is translated differently."},
	{InconsistentSrc, "Same translation for different sources", false, "Different source texts have the same translation."},
	{TermMissing, "Terminology", true, "A recognized term is not translated as the termbase says."},
	{TermForbidden, "Forbidden terms", true, "The translation uses a term the termbase forbids."},
	{Spelling, "Spelling", true, "A word is not in the dictionary."},
	{Rule, "Own rules", true, "A rule of your own (a regular expression) is broken."},
}

// Severity says how serious an issue is.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

var severities = map[Kind]Severity{
	Empty: Error, TagsMissing: Error, TagsExtra: Error, NumbersMissing: Error, Length: Error, TermForbidden: Error,
}

func severityOf(k Kind) Severity {
	if s, ok := severities[k]; ok {
		return s
	}
	return Warning
}

// Issue is a problem found in a segment.
type Issue struct {
	Segment  string   `json:"segment"`
	Kind     Kind     `json:"kind"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	// Found is the text the issue is about, as it appears in the
	// translation; Suggestions are replacements (spelling).
	Found       string   `json:"found,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	// Rule names the user rule that found the issue.
	Rule string `json:"rule,omitempty"`
}

// Segment is a source and its translation.
type Segment struct {
	ID     string          `json:"id"`
	Source content.Content `json:"source"`
	Target content.Content `json:"target"`
	// MaxLength is the longest the translation may be, in characters.
	MaxLength int `json:"maxLength,omitempty"`
	// Skip leaves the segment out (locked, or not to be translated).
	Skip bool `json:"skip,omitempty"`
}

// UserRule is a check of the user's own: a regular expression the
// translation must or must not match, optionally only where the source
// matches another one. With Mode "count", the translation must match
// Target as often as the source matches Source.
type UserRule struct {
	Name     string   `json:"name"`
	Source   string   `json:"source,omitempty"`
	Target   string   `json:"target"`
	Mode     string   `json:"mode"` // "must", "must-not" or "count"
	Severity Severity `json:"severity,omitempty"`
	Message  string   `json:"message,omitempty"`
}

// Speller checks the spelling of single words.
type Speller interface {
	Spell(word string) bool
	Suggest(word string, max int) []string
}

// Options configures the checks.
type Options struct {
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	// Disabled checks; Enabled switches on checks that are off by
	// default. A check is run if it is on by default and not disabled, or
	// enabled.
	Disabled []Kind `json:"disabled,omitempty"`
	Enabled  []Kind `json:"enabled,omitempty"`
	// RatioMax and RatioMin bound the translation's length against the
	// source's, in characters (defaults 1.6 and 0.4); only for sources of
	// 20 characters or more.
	RatioMax float64    `json:"ratioMax,omitempty"`
	RatioMin float64    `json:"ratioMin,omitempty"`
	Rules    []UserRule `json:"rules,omitempty"`
	// Terms and Spell are optional.
	Terms *term.Index `json:"-"`
	Spell Speller     `json:"-"`
	// SpellIgnore lists words that are always correct (the user's own
	// dictionary); words in all capitals and with digits are skipped.
	SpellIgnore []string `json:"spellIgnore,omitempty"`
}

type checker struct {
	opts    Options
	on      map[Kind]bool
	rules   []compiledRule
	ignore  map[string]bool
	srcLang string
	tgtLang string

	spellCache map[string]bool
	sugCache   map[string][]string
}

type compiledRule struct {
	UserRule
	src, tgt *regexp.Regexp
}

// Check reports what is wrong with a rule: no target pattern, a pattern
// that doesn't compile, an unknown mode or severity, or "count" without a
// source pattern. Check (the function) refuses options with such a rule.
func (r UserRule) Check() error {
	_, err := compileRule(r)
	return err
}

func compileRule(r UserRule) (compiledRule, error) {
	cr := compiledRule{UserRule: r}
	var err error
	if r.Target == "" {
		return cr, fmt.Errorf("qa: rule %q has no target pattern", r.Name)
	}
	if cr.tgt, err = regexp.Compile(r.Target); err != nil {
		return cr, fmt.Errorf("qa: rule %q: %w", r.Name, err)
	}
	if r.Source != "" {
		if cr.src, err = regexp.Compile(r.Source); err != nil {
			return cr, fmt.Errorf("qa: rule %q: %w", r.Name, err)
		}
	}
	switch r.Mode {
	case "must", "must-not", "count":
	default:
		return cr, fmt.Errorf("qa: rule %q: mode must be must, must-not or count", r.Name)
	}
	switch r.Severity {
	case "", Error, Warning:
	default:
		return cr, fmt.Errorf("qa: rule %q: severity must be error or warning", r.Name)
	}
	if cr.Mode == "count" && cr.src == nil {
		return cr, fmt.Errorf("qa: rule %q: mode count needs a source pattern", r.Name)
	}
	return cr, nil
}

func newChecker(o Options) (*checker, error) {
	c := &checker{opts: o, on: map[Kind]bool{}, ignore: map[string]bool{}, srcLang: langtag.Primary(o.SourceLang), tgtLang: langtag.Primary(o.TargetLang),
		spellCache: map[string]bool{}, sugCache: map[string][]string{}}
	for _, k := range Kinds {
		c.on[k.Kind] = k.Default
	}
	for _, k := range o.Enabled {
		c.on[k] = true
	}
	for _, k := range o.Disabled {
		c.on[k] = false
	}
	if c.opts.RatioMax == 0 {
		c.opts.RatioMax = 1.6
	}
	if c.opts.RatioMin == 0 {
		c.opts.RatioMin = 0.4
	}
	for _, r := range o.Rules {
		cr, err := compileRule(r)
		if err != nil {
			return nil, err
		}
		c.rules = append(c.rules, cr)
	}
	for _, w := range o.SpellIgnore {
		c.ignore[strings.ToLower(w)] = true
	}
	return c, nil
}

// Check runs the checks on all segments, including those that compare
// segments with each other. Issues come in segment order.
func Check(segs []Segment, o Options) ([]Issue, error) {
	c, err := newChecker(o)
	if err != nil {
		return nil, err
	}
	var out []Issue
	for _, s := range segs {
		if s.Skip {
			continue
		}
		out = append(out, c.segment(s)...)
	}
	out = append(out, c.consistency(segs)...)
	order := map[string]int{}
	for i, s := range segs {
		order[s.ID] = i
	}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Segment] < order[out[j].Segment] })
	return out, nil
}

// CheckSegment runs the checks that need only the one segment.
func CheckSegment(s Segment, o Options) ([]Issue, error) {
	c, err := newChecker(o)
	if err != nil {
		return nil, err
	}
	return c.segment(s), nil
}

func (c *checker) add(out *[]Issue, s Segment, k Kind, msg string, found string) {
	if !c.on[k] {
		return
	}
	*out = append(*out, Issue{Segment: s.ID, Kind: k, Severity: severityOf(k), Message: msg, Found: found})
}

func (c *checker) segment(s Segment) []Issue {
	var out []Issue
	src, tgt := s.Source.Plain(), s.Target.Plain()
	if strings.TrimSpace(tgt) == "" && !s.Target.HasTags() {
		if strings.TrimSpace(src) != "" {
			c.add(&out, s, Empty, "The segment has no translation.", "")
		}
		return out
	}
	if hasLetters(src) && norm(src) == norm(tgt) && c.srcLang != c.tgtLang {
		c.add(&out, s, Untranslated, "The translation is the same as the source.", "")
	}
	c.tags(&out, s)
	c.numbers(&out, s, src, tgt)
	c.punctuation(&out, s, src, tgt)
	c.length(&out, s, src, tgt)
	c.terms(&out, s, src, tgt)
	c.spelling(&out, s, tgt)
	c.userRules(&out, s, src, tgt)
	return out
}

func norm(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func hasLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func tagKey(t content.Tag) string { return t.Kind.String() + "/" + t.ID }

func (c *checker) tags(out *[]Issue, s Segment) {
	srcTags, tgtTags := s.Source.Tags(), s.Target.Tags()
	count := func(ts []content.Tag) map[string]int {
		m := map[string]int{}
		for _, t := range ts {
			m[tagKey(t)]++
		}
		return m
	}
	sc, tc := count(srcTags), count(tgtTags)
	missing, extra := 0, 0
	for k, n := range sc {
		if tc[k] < n {
			missing += n - tc[k]
		}
	}
	for k, n := range tc {
		if sc[k] < n {
			extra += n - sc[k]
		}
	}
	switch {
	case missing == 1:
		c.add(out, s, TagsMissing, "A tag of the source is missing.", "")
	case missing > 1:
		c.add(out, s, TagsMissing, fmt.Sprintf("%d tags of the source are missing.", missing), "")
	}
	switch {
	case extra == 1:
		c.add(out, s, TagsExtra, "The translation has a tag the source does not.", "")
	case extra > 1:
		c.add(out, s, TagsExtra, fmt.Sprintf("The translation has %d tags the source does not.", extra), "")
	}
	if missing == 0 && extra == 0 && !sameNesting(srcTags, tgtTags) {
		c.add(out, s, TagOrder, "Paired tags are nested differently than in the source.", "")
	}
}

// sameNesting reports whether paired tags enclose each other the same
// way (the order of standalone tags and of separate spans may change).
func sameNesting(a, b []content.Tag) bool {
	parents := func(ts []content.Tag) (map[string]string, bool) {
		m := map[string]string{}
		var stack []string
		for _, t := range ts {
			switch t.Kind {
			case content.TagOpen:
				parent := ""
				if len(stack) > 0 {
					parent = stack[len(stack)-1]
				}
				m[t.ID] = parent
				stack = append(stack, t.ID)
			case content.TagClose:
				if len(stack) == 0 || stack[len(stack)-1] != t.ID {
					return m, false
				}
				stack = stack[:len(stack)-1]
			}
		}
		return m, len(stack) == 0
	}
	pa, okA := parents(a)
	pb, okB := parents(b)
	if okA != okB {
		return false
	}
	for id, p := range pa {
		if pb[id] != p {
			return false
		}
	}
	return true
}

func (c *checker) length(out *[]Issue, s Segment, src, tgt string) {
	n := utf8.RuneCountInString(tgt)
	if s.MaxLength > 0 && n > s.MaxLength {
		c.add(out, s, Length, fmt.Sprintf("The translation has %d characters; at most %d are allowed.", n, s.MaxLength), "")
	}
	sn := utf8.RuneCountInString(strings.TrimSpace(src))
	if sn >= 20 {
		r := float64(utf8.RuneCountInString(strings.TrimSpace(tgt))) / float64(sn)
		switch {
		case r > c.opts.RatioMax:
			c.add(out, s, LengthRatio, fmt.Sprintf("The translation is %d%% of the source's length.", int(r*100+0.5)), "")
		case r < c.opts.RatioMin:
			c.add(out, s, LengthRatio, fmt.Sprintf("The translation is only %d%% of the source's length.", int(r*100+0.5)), "")
		}
	}
}

func (c *checker) terms(out *[]Issue, s Segment, src, tgt string) {
	if c.opts.Terms == nil || (!c.on[TermMissing] && !c.on[TermForbidden]) {
		return
	}
	hits := c.opts.Terms.Recognize(src, c.opts.SourceLang, c.opts.TargetLang)
	for _, is := range c.opts.Terms.Check(hits, tgt, c.opts.TargetLang) {
		switch is.Kind {
		case "missing":
			c.add(out, s, TermMissing, fmt.Sprintf("“%s” is usually “%s”.", is.Term, is.Expected), is.Term)
		case "forbidden":
			msg := fmt.Sprintf("“%s” is a forbidden term.", is.Found)
			if is.Term != "" {
				msg = fmt.Sprintf("“%s” is a forbidden term for “%s”.", is.Found, is.Term)
			}
			c.add(out, s, TermForbidden, msg, is.Found)
		}
	}
}

func (c *checker) userRules(out *[]Issue, s Segment, src, tgt string) {
	if !c.on[Rule] {
		return
	}
	for _, r := range c.rules {
		if r.src != nil && r.Mode != "count" && !r.src.MatchString(src) {
			continue
		}
		var found string
		broken := false
		switch r.Mode {
		case "must":
			broken = !r.tgt.MatchString(tgt)
		case "must-not":
			if loc := r.tgt.FindStringIndex(tgt); loc != nil {
				broken, found = true, tgt[loc[0]:loc[1]]
			}
		case "count":
			ns, nt := len(r.src.FindAllStringIndex(src, -1)), len(r.tgt.FindAllStringIndex(tgt, -1))
			broken = ns != nt
		}
		if !broken {
			continue
		}
		msg := r.Message
		if msg == "" {
			msg = fmt.Sprintf("Rule “%s” is broken.", r.Name)
		}
		sev := r.Severity
		if sev == "" {
			sev = Warning
		}
		*out = append(*out, Issue{Segment: s.ID, Kind: Rule, Severity: sev, Message: msg, Found: found, Rule: r.Name})
	}
}

// consistency compares segments: the same source translated differently,
// and (if enabled) the same translation for different sources.
func (c *checker) consistency(segs []Segment) []Issue {
	var out []Issue
	if c.on[Inconsistent] {
		type group struct {
			ids     []string
			targets map[string]string
			order   []string
		}
		bySource := map[string]*group{}
		var keys []string
		for _, s := range segs {
			if s.Skip || strings.TrimSpace(s.Target.Plain()) == "" {
				continue
			}
			k := norm(s.Source.Plain())
			if k == "" {
				continue
			}
			g, ok := bySource[k]
			if !ok {
				g = &group{targets: map[string]string{}}
				bySource[k] = g
				keys = append(keys, k)
			}
			g.ids = append(g.ids, s.ID)
			t := norm(s.Target.Plain())
			if _, seen := g.targets[t]; !seen {
				g.targets[t] = strings.TrimSpace(s.Target.Plain())
				g.order = append(g.order, t)
			}
		}
		for _, k := range keys {
			g := bySource[k]
			if len(g.targets) < 2 {
				continue
			}
			var shown []string
			for _, t := range g.order {
				shown = append(shown, "“"+g.targets[t]+"”")
			}
			msg := fmt.Sprintf("The same source is translated in %d ways: %s.", len(g.targets), strings.Join(shown, ", "))
			for _, id := range g.ids {
				out = append(out, Issue{Segment: id, Kind: Inconsistent, Severity: Warning, Message: msg})
			}
		}
	}
	if c.on[InconsistentSrc] {
		byTarget := map[string]map[string]bool{}
		ids := map[string][]string{}
		var keys []string
		for _, s := range segs {
			t := norm(s.Target.Plain())
			if s.Skip || t == "" {
				continue
			}
			if _, ok := byTarget[t]; !ok {
				byTarget[t] = map[string]bool{}
				keys = append(keys, t)
			}
			byTarget[t][norm(s.Source.Plain())] = true
			ids[t] = append(ids[t], s.ID)
		}
		for _, t := range keys {
			if len(byTarget[t]) < 2 {
				continue
			}
			msg := fmt.Sprintf("%d different sources have this translation.", len(byTarget[t]))
			for _, id := range ids[t] {
				out = append(out, Issue{Segment: id, Kind: InconsistentSrc, Severity: Warning, Message: msg})
			}
		}
	}
	return out
}
