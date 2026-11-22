// Package segment splits text into translation segments using SRX 2.0
// rules (Segmentation Rules eXchange), the format shared by most CAT tools.
package segment

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
)

//go:embed default.srx
var defaultSRX []byte

// Rule is one SRX rule. A break position lies between text matching Before
// and text matching After. Break decides whether the position is a segment
// boundary; the first rule that matches a position wins.
type Rule struct {
	Break  bool
	Before string
	After  string

	before *regexp.Regexp // Before reversed and anchored at the start, or nil for "always".
	after  *regexp.Regexp // After anchored at the start, or nil for "always".
	scan   *regexp.Regexp // Before unanchored, used to find candidate positions.
	longst *regexp.Regexp // Before unanchored, leftmost-longest variant.
	scanA  *regexp.Regexp // After unanchored, used when Before is empty.
}

// LanguageRule is a named, ordered list of rules.
type LanguageRule struct {
	Name  string
	Rules []Rule
}

// LanguageMap maps language codes matching Pattern to a LanguageRule.
type LanguageMap struct {
	Pattern string
	Rule    string
	re      *regexp.Regexp
}

// Rules is a parsed and compiled SRX document.
type Rules struct {
	// Cascade means that the rules of every matching language map apply, in
	// map order. Without cascade only the first matching map is used.
	Cascade bool
	// Languages holds the language rules by name.
	Languages map[string]*LanguageRule
	// Maps holds the language maps in document order.
	Maps []LanguageMap
}

type srxDoc struct {
	XMLName xml.Name `xml:"srx"`
	Version string   `xml:"version,attr"`
	Header  struct {
		Cascade string `xml:"cascade,attr"`
	} `xml:"header"`
	Body struct {
		LanguageRules []struct {
			Name  string `xml:"languagerulename,attr"`
			Rules []struct {
				Break  string `xml:"break,attr"`
				Before string `xml:"beforebreak"`
				After  string `xml:"afterbreak"`
			} `xml:"rule"`
		} `xml:"languagerules>languagerule"`
		Maps []struct {
			Pattern string `xml:"languagepattern,attr"`
			Rule    string `xml:"languagerulename,attr"`
		} `xml:"maprules>languagemap"`
	} `xml:"body"`
}

// Default returns the built-in rules (German, English, CJK and a generic
// fallback). The result is freshly parsed and safe to modify.
func Default() *Rules {
	r, err := Parse(bytes.NewReader(defaultSRX))
	if err != nil {
		panic(fmt.Sprintf("segment: built-in SRX is invalid: %v", err))
	}
	return r
}

// Parse reads an SRX 2.0 document and compiles its rules.
//
// SRX patterns are Java regular expressions. Interlinea uses Go's RE2 engine,
// which supports Unicode classes (\p{Lu}) but not look-around or
// back-references; rules using them are reported as errors.
func Parse(r io.Reader) (*Rules, error) {
	var doc srxDoc
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("segment: parse SRX: %w", err)
	}
	if doc.Version != "" && !strings.HasPrefix(doc.Version, "2") {
		return nil, fmt.Errorf("segment: unsupported SRX version %q", doc.Version)
	}
	out := &Rules{
		Cascade:   !strings.EqualFold(doc.Header.Cascade, "no"),
		Languages: map[string]*LanguageRule{},
	}
	for _, lr := range doc.Body.LanguageRules {
		if lr.Name == "" {
			return nil, fmt.Errorf("segment: language rule without name")
		}
		if _, dup := out.Languages[lr.Name]; dup {
			return nil, fmt.Errorf("segment: duplicate language rule %q", lr.Name)
		}
		l := &LanguageRule{Name: lr.Name}
		for i, xr := range lr.Rules {
			rule := Rule{
				Break:  !strings.EqualFold(strings.TrimSpace(xr.Break), "no"),
				Before: xr.Before,
				After:  xr.After,
			}
			if err := rule.compile(); err != nil {
				return nil, fmt.Errorf("segment: rule %d of %q: %w", i+1, lr.Name, err)
			}
			l.Rules = append(l.Rules, rule)
		}
		out.Languages[lr.Name] = l
	}
	for _, m := range doc.Body.Maps {
		if _, ok := out.Languages[m.Rule]; !ok {
			return nil, fmt.Errorf("segment: language map refers to unknown rule %q", m.Rule)
		}
		re, err := regexp.Compile(`(?i)^(?:` + m.Pattern + `)$`)
		if err != nil {
			return nil, fmt.Errorf("segment: language pattern %q: %w", m.Pattern, err)
		}
		out.Maps = append(out.Maps, LanguageMap{Pattern: m.Pattern, Rule: m.Rule, re: re})
	}
	return out, nil
}

// NewRule builds and compiles a rule programmatically.
func NewRule(brk bool, before, after string) (Rule, error) {
	r := Rule{Break: brk, Before: before, After: after}
	if err := r.compile(); err != nil {
		return Rule{}, err
	}
	return r, nil
}

func (r *Rule) compile() error {
	var err error
	if r.Before != "" {
		parsed, perr := syntax.Parse(r.Before, syntax.Perl)
		if perr != nil {
			return fmt.Errorf("beforebreak %q: %w", r.Before, perr)
		}
		// Matching the reversed pattern at the start of the reversed text is
		// equivalent to matching the pattern at the end of the text, but the
		// engine then tries a single start position instead of every one.
		if r.before, err = regexp.Compile(`^(?:` + reverseRegexp(parsed).String() + `)`); err != nil {
			return fmt.Errorf("beforebreak %q: %w", r.Before, err)
		}
		r.scan = regexp.MustCompile(r.Before)
		r.longst = regexp.MustCompile(r.Before)
		r.longst.Longest()
	}
	if r.After != "" {
		if r.after, err = regexp.Compile(`^(?:` + r.After + `)`); err != nil {
			return fmt.Errorf("afterbreak %q: %w", r.After, err)
		}
		r.scanA = regexp.MustCompile(r.After)
	}
	if r.Before == "" && r.After == "" {
		return fmt.Errorf("rule has neither beforebreak nor afterbreak")
	}
	return nil
}

// For returns the ordered rule list that applies to a language code.
func (rs *Rules) For(lang string) []Rule {
	var out []Rule
	for _, m := range rs.Maps {
		if !m.re.MatchString(lang) {
			continue
		}
		out = append(out, rs.Languages[m.Rule].Rules...)
		if !rs.Cascade {
			break
		}
	}
	return out
}

// reverseRegexp returns a regular expression that matches the reversed
// strings of re: concatenations and literals are reversed and text anchors
// swap sides. Word boundaries are symmetric and need no change.
func reverseRegexp(re *syntax.Regexp) *syntax.Regexp {
	out := *re
	out.Sub = nil
	switch re.Op {
	case syntax.OpLiteral:
		out.Rune = slices.Clone(re.Rune)
		slices.Reverse(out.Rune)
	case syntax.OpBeginText:
		out.Op = syntax.OpEndText
	case syntax.OpEndText:
		out.Op = syntax.OpBeginText
	case syntax.OpBeginLine:
		out.Op = syntax.OpEndLine
	case syntax.OpEndLine:
		out.Op = syntax.OpBeginLine
	default:
		out.Rune = slices.Clone(re.Rune)
	}
	if len(re.Sub) > 0 {
		out.Sub = make([]*syntax.Regexp, len(re.Sub))
		for i, sub := range re.Sub {
			out.Sub[i] = reverseRegexp(sub)
		}
		if re.Op == syntax.OpConcat {
			slices.Reverse(out.Sub)
		}
	}
	return &out
}
