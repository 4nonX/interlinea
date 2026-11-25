// Package glossary makes a termbase into a glossary to read and print: a
// dictionary sorted by one language, or a table with a column for each
// language, written as PDF, Word, a web page or plain text.
package glossary

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	"github.com/4nonX/interlinea/term"
)

// Glossary is a termbase laid out for reading.
type Glossary struct {
	Title     string
	Languages []string // in their order; the first is the sort language
	Sort      string
	// Fields shown, by level, in the termbase's order.
	EntryFields, LangFields, TermFields []string
	Sections                            []Section
	Options                             term.ExportOptions
	// Left out: entries without a term in the sort language.
	WithoutHeadword int
	Entries         int
	kinds           map[string]term.Kind
}

// Section is the headwords under one initial letter (one section without
// a letter when letters are not shown).
type Section struct {
	Letter   string
	Articles []Article
}

// Article is a headword and the entries under it: more than one for
// homonyms grouped together.
type Article struct {
	Headword string
	Status   string
	Entries  []term.Entry
}

// Build lays a termbase out as the options say.
func Build(tb *term.Termbase, o term.ExportOptions) *Glossary {
	sel := term.Select(tb, o)
	g := &Glossary{Title: o.Title, Options: o, kinds: kinds(tb)}
	if g.Title == "" {
		g.Title = tb.Name
	}
	g.Languages = append(g.Languages, sel.Languages...)
	g.Sort = o.SortBy
	if g.Sort == "" && len(g.Languages) > 0 {
		g.Sort = g.Languages[0]
	}
	// The sort language goes first.
	for i, l := range g.Languages {
		if strings.EqualFold(l, g.Sort) && i > 0 {
			g.Languages = append([]string{l}, append(g.Languages[:i:i], g.Languages[i+1:]...)...)
			break
		}
	}
	for _, f := range sel.Fields {
		switch f.Level {
		case term.LevelEntry:
			g.EntryFields = append(g.EntryFields, f.Name)
		case term.LevelLanguage:
			g.LangFields = append(g.LangFields, f.Name)
		default:
			g.TermFields = append(g.TermFields, f.Name)
		}
	}
	// Fields used in entries but not defined are shown too, at their level.
	seen := map[string]bool{}
	for _, n := range append(append(append([]string{}, g.EntryFields...), g.LangFields...), g.TermFields...) {
		seen[strings.ToLower(n)] = true
	}
	note := func(vs []term.Value, into *[]string) {
		for _, v := range vs {
			if k := strings.ToLower(v.Field); !seen[k] {
				seen[k] = true
				*into = append(*into, v.Field)
			}
		}
	}
	for _, e := range sel.Entries {
		note(e.Fields, &g.EntryFields)
		for _, l := range e.Languages {
			note(l.Fields, &g.LangFields)
			for _, t := range l.Terms {
				note(t.Fields, &g.TermFields)
			}
		}
	}

	var arts []Article
	for _, e := range sel.Entries {
		terms := TermsOf(e, g.Sort)
		if len(terms) == 0 {
			g.WithoutHeadword++
			continue
		}
		g.Entries++
		if o.PerTerm {
			for _, t := range terms {
				arts = append(arts, Article{Headword: t.Text, Status: t.Status, Entries: []term.Entry{e}})
			}
		} else {
			arts = append(arts, Article{Headword: terms[0].Text, Status: terms[0].Status, Entries: []term.Entry{e}})
		}
	}
	col := collate.New(language.Make(g.Sort), collate.IgnoreCase, collate.Numeric)
	sort.SliceStable(arts, func(i, j int) bool {
		if c := col.CompareString(arts[i].Headword, arts[j].Headword); c != 0 {
			return c < 0
		}
		return arts[i].Headword < arts[j].Headword
	})
	if o.GroupHomonyms {
		var merged []Article
		for _, a := range arts {
			if n := len(merged); n > 0 && merged[n-1].Headword == a.Headword {
				merged[n-1].Entries = append(merged[n-1].Entries, a.Entries...)
				continue
			}
			merged = append(merged, a)
		}
		arts = merged
	}
	if !o.Letters {
		g.Sections = []Section{{Articles: arts}}
		return g
	}
	loose := collate.New(language.Make(g.Sort), collate.Loose)
	for _, a := range arts {
		letter := Initial(a.Headword)
		if n := len(g.Sections); n > 0 {
			last := &g.Sections[n-1]
			if last.Letter == letter || letter != "" && last.Letter != "" && loose.CompareString(last.Letter, letter) == 0 {
				last.Articles = append(last.Articles, a)
				continue
			}
		}
		g.Sections = append(g.Sections, Section{Letter: letter, Articles: []Article{a}})
	}
	return g
}

// TermsOf gives an entry's terms in a language.
func TermsOf(e term.Entry, lang string) []term.Term {
	for _, l := range e.Languages {
		if strings.EqualFold(l.Lang, lang) {
			return l.Terms
		}
	}
	return nil
}

// LanguageOf gives an entry's language block.
func LanguageOf(e term.Entry, lang string) (term.Language, bool) {
	for _, l := range e.Languages {
		if strings.EqualFold(l.Lang, lang) {
			return l, true
		}
	}
	return term.Language{}, false
}

// Initial is the letter a headword is listed under: its first letter, in
// capitals, without accents a language does not count as letters of their
// own. Scripts without letters (Chinese, Japanese) have none.
func Initial(s string) string {
	for _, r := range norm.NFC.String(s) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		if unicode.IsDigit(r) {
			return "0–9"
		}
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return ""
		}
		return string(unicode.ToUpper(r))
	}
	return ""
}

// Value gives a field's values in a list of values.
func Value(vs []term.Value, field string) string {
	var out []string
	for _, v := range vs {
		if strings.EqualFold(v.Field, field) && strings.TrimSpace(v.Value) != "" {
			out = append(out, strings.TrimSpace(v.Value))
		}
	}
	return strings.Join(out, "; ")
}

// StatusMark is how a term's status is shown: nothing for approved terms.
func StatusMark(status string) string {
	switch status {
	case term.StatusProposed:
		return "proposed"
	case term.StatusForbidden:
		return "forbidden"
	}
	return ""
}

// Summary says what the glossary holds, for the person who made it.
func (g *Glossary) Summary() string {
	s := fmt.Sprintf("%d %s", g.Entries, plural(g.Entries, "entry", "entries"))
	if g.WithoutHeadword > 0 {
		s += fmt.Sprintf("; %d without a term in %s left out", g.WithoutHeadword, g.Sort)
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Made is when the glossary says it was made.
func Made(t time.Time) string { return t.Format("2 January 2006") }
