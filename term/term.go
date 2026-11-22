// Package term is Interlinea's termbase model and its exchange formats:
// TBX (ISO 30042, versions 3 and 2), MultiTerm XML, CSV/TSV and Excel.
//
// A termbase holds concept entries. Each entry has fields of its own,
// language sections with their own fields, and terms with a status and
// fields. Field names are free; the termbase definition lists them with
// the level they belong to.
package term

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/4nonX/interlinea/langtag"
)

// Status of a term, in the words translators use. In TBX these are the
// administrative statuses preferred, admitted and deprecated.
const (
	StatusApproved  = "approved"
	StatusProposed  = "proposed"
	StatusForbidden = "forbidden"
)

// Level says where a field lives.
type Level string

const (
	LevelEntry    Level = "entry"
	LevelLanguage Level = "language"
	LevelTerm     Level = "term"
)

// Kind is the type of a field's values.
type Kind string

const (
	KindText     Kind = "text"
	KindPicklist Kind = "picklist"
	KindDate     Kind = "date"
	KindBoolean  Kind = "boolean"
	KindNumber   Kind = "number"
)

// Field describes a field of the termbase definition.
type Field struct {
	Name   string   `json:"name"`
	Level  Level    `json:"level"`
	Kind   Kind     `json:"kind"`
	Values []string `json:"values,omitempty"` // picklist values
}

// Definition is a termbase's structure: languages and fields.
type Definition struct {
	Name      string   `json:"name,omitempty"`
	Languages []string `json:"languages"`
	Fields    []Field  `json:"fields,omitempty"`
}

// Value is one field value. Order is kept as in the source file.
type Value struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// Term is a term with its status and fields.
type Term struct {
	Text   string  `json:"text"`
	Status string  `json:"status,omitempty"`
	Fields []Value `json:"fields,omitempty"`
}

// Language is a language section of an entry.
type Language struct {
	Lang   string  `json:"lang"`
	Fields []Value `json:"fields,omitempty"`
	Terms  []Term  `json:"terms"`
}

// Entry is a concept with its terms in several languages.
type Entry struct {
	ID        string     `json:"id,omitempty"`
	Fields    []Value    `json:"fields,omitempty"`
	Languages []Language `json:"languages"`
	CreatedAt time.Time  `json:"createdAt,omitzero"`
	CreatedBy string     `json:"createdBy,omitempty"`
	ChangedAt time.Time  `json:"changedAt,omitzero"`
	ChangedBy string     `json:"changedBy,omitempty"`
}

// Termbase is a definition and its entries.
type Termbase struct {
	Definition
	Entries []Entry `json:"entries"`
}

// Report tells what an import read and what it had to leave out.
type Report struct {
	Entries  int      `json:"entries"`
	Terms    int      `json:"terms"`
	Skipped  int      `json:"skipped"`
	Warnings []string `json:"warnings,omitempty"`
}

func (r *Report) warn(format string, a ...any) {
	if len(r.Warnings) < 50 {
		r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...))
	} else if len(r.Warnings) == 50 {
		r.Warnings = append(r.Warnings, "More problems were found; only the first 50 are listed.")
	}
}

// Lang returns the entry's section for a language, or nil.
func (e *Entry) Lang(lang string) *Language {
	for i := range e.Languages {
		if strings.EqualFold(e.Languages[i].Lang, lang) {
			return &e.Languages[i]
		}
	}
	return nil
}

// TermCount is the number of terms in the entry.
func (e *Entry) TermCount() int {
	n := 0
	for _, l := range e.Languages {
		n += len(l.Terms)
	}
	return n
}

// Key normalizes a term for comparison: case, surrounding space and inner
// runs of space do not count.
func Key(s string) string {
	return strings.ToLower(strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " "))
}

// NormalizeLang returns a canonical BCP 47 tag, or the input trimmed when it
// is not one.
func NormalizeLang(s string) string {
	s = strings.TrimSpace(s)
	if t, err := langtag.Normalize(s); err == nil {
		return t
	}
	return s
}

// Clean tidies an entry: trims text, drops empty terms, languages and
// values, removes duplicate terms within a language and normalizes language
// tags. It reports whether anything usable is left.
func (e *Entry) Clean() bool {
	e.Fields = cleanValues(e.Fields)
	langs := e.Languages[:0]
	for _, l := range e.Languages {
		l.Lang = NormalizeLang(l.Lang)
		l.Fields = cleanValues(l.Fields)
		seen := map[string]bool{}
		terms := l.Terms[:0]
		for _, t := range l.Terms {
			t.Text = strings.TrimSpace(t.Text)
			if t.Text == "" || seen[Key(t.Text)] {
				continue
			}
			seen[Key(t.Text)] = true
			t.Status = NormalizeStatus(t.Status)
			t.Fields = cleanValues(t.Fields)
			terms = append(terms, t)
		}
		l.Terms = terms
		if len(l.Terms) > 0 || len(l.Fields) > 0 {
			langs = append(langs, l)
		}
	}
	e.Languages = langs
	return e.TermCount() > 0
}

func cleanValues(vs []Value) []Value {
	out := vs[:0]
	for _, v := range vs {
		v.Field, v.Value = strings.TrimSpace(v.Field), strings.TrimSpace(v.Value)
		if v.Field != "" && v.Value != "" {
			out = append(out, v)
		}
	}
	return out
}

// NormalizeStatus maps the many spellings of term status found in
// termbases to approved, proposed or forbidden ("" when unknown).
func NormalizeStatus(s string) string {
	switch k := strings.ToLower(strings.TrimSpace(s)); k {
	case "":
		return ""
	case "approved", "preferred", "preferredterm-admn-sts", "preferredterm", "preferred term", "freigegeben", "bevorzugt", "standardized", "normativeauthorization", "1", "yes", "ja":
		return StatusApproved
	case "forbidden", "deprecated", "deprecatedterm-admn-sts", "deprecatedterm", "supersededterm-admn-sts", "superseded", "obsolete", "verboten", "nicht verwenden", "do not use", "rejected", "abgelehnt", "0", "no", "nein":
		return StatusForbidden
	case "proposed", "admitted", "admittedterm-admn-sts", "admittedterm", "new", "draft", "vorgeschlagen", "zugelassen", "neu", "pending", "none":
		return StatusProposed
	default:
		return StatusProposed
	}
}

// InferDefinition builds the definition from the entries: every language
// and every field name with its level. Fields whose values come from a
// small set of short values, each used several times, become picklists.
func InferDefinition(name string, entries []Entry) Definition {
	d := Definition{Name: name}
	langSeen := map[string]bool{}
	type fk struct {
		name  string
		level Level
	}
	values := map[fk]map[string]bool{}
	counts := map[fk]int{}
	var order []fk
	add := func(level Level, vs []Value) {
		for _, v := range vs {
			k := fk{v.Field, level}
			if values[k] == nil {
				values[k] = map[string]bool{}
				order = append(order, k)
			}
			values[k][v.Value] = true
			counts[k]++
		}
	}
	for _, e := range entries {
		add(LevelEntry, e.Fields)
		for _, l := range e.Languages {
			if !langSeen[l.Lang] {
				langSeen[l.Lang] = true
				d.Languages = append(d.Languages, l.Lang)
			}
			add(LevelLanguage, l.Fields)
			for _, t := range l.Terms {
				add(LevelTerm, t.Fields)
			}
		}
	}
	for _, k := range order {
		f := Field{Name: k.name, Level: k.level, Kind: KindText}
		vs := values[k]
		if picklistLike(vs, counts[k]) {
			f.Kind = KindPicklist
			for v := range vs {
				f.Values = append(f.Values, v)
			}
			sort.Strings(f.Values)
		}
		d.Fields = append(d.Fields, f)
	}
	return d
}

// picklistLike reports whether a field's values look like choices from a
// list: a few short values, each used several times.
func picklistLike(vs map[string]bool, uses int) bool {
	if len(vs) > 12 || uses < 6 || uses < 3*len(vs) {
		return false
	}
	for v := range vs {
		if len([]rune(v)) > 40 {
			return false
		}
	}
	return true
}

// Stats counts entries and terms per language.
func (tb *Termbase) Stats() (entries, terms int, perLang map[string]int) {
	perLang = map[string]int{}
	for _, e := range tb.Entries {
		for _, l := range e.Languages {
			perLang[l.Lang] += len(l.Terms)
			terms += len(l.Terms)
		}
	}
	return len(tb.Entries), terms, perLang
}
