package term

import (
	"fmt"
	"strings"
)

// Conflict is a place where both sides changed an entry differently since
// their common version.
type Conflict struct {
	// Path names the place in words: "entry field Subject",
	// "de-DE language field Note", "de-DE term Kaution", "de-DE term
	// Kaution status", "de-DE term Kaution field Source".
	Path string `json:"path"`
	// Base, Ours and Theirs are the values in the common version, in our
	// version (the project) and in theirs (the master); "" where absent.
	Base   string `json:"base"`
	Ours   string `json:"ours"`
	Theirs string `json:"theirs"`
}

// Merge3 merges two versions of an entry that both started from base, as
// git merges files: a change on one side only is taken; the same change
// on both sides is taken once; different changes to the same place are a
// conflict. Places are an entry's or language's values per field, a
// language's terms by their text, and a term's status and values per
// field. Where there is a conflict the result keeps theirs at that place,
// so it is a complete entry the reviewer can start from.
//
// The result has theirs' order, with what ours added after it.
func Merge3(base, ours, theirs Entry) (Entry, []Conflict) {
	m := &merger{}
	out := cloneEntry(theirs)
	out.Fields = m.values("entry field", base.Fields, ours.Fields, theirs.Fields)

	langs := unionLangs(base, ours, theirs)
	var result []Language
	for _, lang := range langs {
		b, o, t := langOf(base, lang), langOf(ours, lang), langOf(theirs, lang)
		l, keep := m.language(lang, b, o, t)
		if keep {
			result = append(result, l)
		}
	}
	out.Languages = result
	out.ID = theirs.ID
	return out, m.conflicts
}

type merger struct {
	conflicts []Conflict
}

func (m *merger) conflict(path, base, ours, theirs string) {
	m.conflicts = append(m.conflicts, Conflict{Path: path, Base: base, Ours: ours, Theirs: theirs})
}

func langOf(e Entry, lang string) *Language {
	for i := range e.Languages {
		if langEqual(e.Languages[i].Lang, lang) {
			return &e.Languages[i]
		}
	}
	return nil
}

// unionLangs lists the languages of theirs, then those only ours or the
// base have.
func unionLangs(es ...Entry) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range []Entry{es[2], es[1], es[0]} {
		for _, l := range e.Languages {
			k := NormalizeLang(l.Lang)
			if !seen[k] {
				seen[k] = true
				out = append(out, l.Lang)
			}
		}
	}
	return out
}

// pick is the three-way rule for one place: ok is false for a conflict.
func pick[T any](base, ours, theirs T, eq func(a, b T) bool) (T, bool) {
	switch {
	case eq(ours, theirs):
		return theirs, true
	case eq(ours, base):
		return theirs, true
	case eq(theirs, base):
		return ours, true
	}
	return theirs, false
}

func (m *merger) language(lang string, b, o, t *Language) (Language, bool) {
	present := func(l *Language) bool { return l != nil }
	// The section as a whole added or removed: removal wins only where
	// the other side did not change the section.
	if b != nil && (o == nil || t == nil) {
		other := o
		if o == nil {
			other = t
		}
		if other != nil && !sameLanguage(*b, *other) {
			m.conflict(lang+" language", "present", presence(o), presence(t))
			if t != nil {
				return cloneLang(*t), true
			}
			return cloneLang(*o), true
		}
		return Language{}, false
	}
	if b == nil && !present(o) {
		return cloneLang(*t), true
	}
	if b == nil && !present(t) {
		return cloneLang(*o), true
	}
	var bl Language
	if b != nil {
		bl = *b
	} else {
		bl = Language{Lang: lang}
	}
	out := Language{Lang: t.Lang}
	out.Fields = m.values(lang+" language field", bl.Fields, o.Fields, t.Fields)
	keys := unionTerms(bl, *o, *t)
	for _, k := range keys {
		bt, ot, tt := termOf(bl, k), termOf(*o, k), termOf(*t, k)
		if tm, keep := m.term(lang, bt, ot, tt); keep {
			out.Terms = append(out.Terms, tm)
		}
	}
	return out, true
}

func presence(l *Language) string {
	if l == nil {
		return "removed"
	}
	return "changed"
}

func sameLanguage(a, b Language) bool {
	return langEqual(a.Lang, b.Lang) && sameValues(a.Fields, b.Fields) && sameTerms(a.Terms, b.Terms)
}

func sameTerms(a, b []Term) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameTerm(a[i], b[i]) {
			return false
		}
	}
	return true
}

func sameTerm(a, b Term) bool {
	return a.Text == b.Text && a.Status == b.Status && sameValues(a.Fields, b.Fields)
}

func unionTerms(ls ...Language) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range []Language{ls[2], ls[1], ls[0]} {
		for _, t := range l.Terms {
			k := SameKey(t.Text)
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

func termOf(l Language, key string) *Term {
	for i := range l.Terms {
		if SameKey(l.Terms[i].Text) == key {
			return &l.Terms[i]
		}
	}
	return nil
}

func (m *merger) term(lang string, b, o, t *Term) (Term, bool) {
	name := func() string {
		for _, x := range []*Term{t, o, b} {
			if x != nil {
				return x.Text
			}
		}
		return ""
	}()
	path := lang + " term " + name
	switch {
	case b == nil && o == nil:
		return cloneTerm(*t), true
	case b == nil && t == nil:
		return cloneTerm(*o), true
	case b == nil:
		// Added on both sides: merge as if from an empty term.
		b = &Term{Text: t.Text}
	case o == nil || t == nil:
		other := o
		if o == nil {
			other = t
		}
		if other == nil || sameTerm(*b, *other) {
			return Term{}, false
		}
		m.conflict(path, "present", termPresence(o), termPresence(t))
		return cloneTerm(*other), true
	}
	out := Term{}
	text, ok := pick(b.Text, o.Text, t.Text, func(x, y string) bool { return x == y })
	if !ok {
		m.conflict(path+" spelling", b.Text, o.Text, t.Text)
	}
	out.Text = text
	status, ok := pick(b.Status, o.Status, t.Status, func(x, y string) bool { return x == y })
	if !ok {
		m.conflict(path+" status", b.Status, o.Status, t.Status)
	}
	out.Status = status
	out.Fields = m.values(path+" field", b.Fields, o.Fields, t.Fields)
	return out, true
}

func termPresence(t *Term) string {
	if t == nil {
		return "removed"
	}
	return "changed"
}

// values merges field values field by field: each field's values (in
// order) are one place.
func (m *merger) values(path string, base, ours, theirs []Value) []Value {
	names := fieldNames(theirs, ours, base)
	var out []Value
	for _, n := range names {
		b, o, t := valuesOf(base, n), valuesOf(ours, n), valuesOf(theirs, n)
		v, ok := pick(b, o, t, equalStrings)
		if !ok {
			m.conflict(path+" "+n, strings.Join(b, "; "), strings.Join(o, "; "), strings.Join(t, "; "))
		}
		for _, x := range v {
			out = append(out, Value{Field: n, Value: x})
		}
	}
	return out
}

func fieldNames(lists ...[]Value) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range lists {
		for _, v := range l {
			if !seen[v.Field] {
				seen[v.Field] = true
				out = append(out, v.Field)
			}
		}
	}
	return out
}

func valuesOf(vs []Value, field string) []string {
	var out []string
	for _, v := range vs {
		if v.Field == field {
			out = append(out, v.Value)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
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

func sameValues(a, b []Value) bool {
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

// Change is a difference between two versions of an entry, for showing a
// reviewer what a project changed.
type Change struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

// Diff lists what changed from a to b, place by place (the places Merge3
// uses): entry fields, then each language in b's order (with a's removed
// ones after), its fields and its terms.
func Diff(a, b Entry) []Change {
	var out []Change
	add := func(path, old, new string) {
		if old != new {
			out = append(out, Change{Path: path, Old: old, New: new})
		}
	}
	diffValues := func(path string, x, y []Value) {
		for _, n := range fieldNames(x, y) {
			add(path+" "+n, strings.Join(valuesOf(x, n), "; "), strings.Join(valuesOf(y, n), "; "))
		}
	}
	diffValues("entry field", a.Fields, b.Fields)
	for _, lang := range unionLangs(a, b, b) {
		la, lb := langOf(a, lang), langOf(b, lang)
		switch {
		case la == nil:
			add(lang+" language", "", "added")
			for _, t := range lb.Terms {
				add(lang+" term "+t.Text, "", describeTerm(t))
			}
			continue
		case lb == nil:
			add(lang+" language", "present", "removed")
			continue
		}
		diffValues(lang+" language field", la.Fields, lb.Fields)
		for _, k := range unionTerms(*la, *lb, *lb) {
			ta, tb := termOf(*la, k), termOf(*lb, k)
			switch {
			case ta == nil:
				add(lang+" term "+tb.Text, "", describeTerm(*tb))
			case tb == nil:
				add(lang+" term "+ta.Text, describeTerm(*ta), "")
			default:
				p := lang + " term " + tb.Text
				add(p+" spelling", ta.Text, tb.Text)
				add(p+" status", ta.Status, tb.Status)
				diffValues(p+" field", ta.Fields, tb.Fields)
			}
		}
	}
	return out
}

func describeTerm(t Term) string {
	if t.Status == "" {
		return t.Text
	}
	return fmt.Sprintf("%s (%s)", t.Text, t.Status)
}
