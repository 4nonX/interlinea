package term

import "strings"

// FileLanguages lists the languages a termbase read from a file has: those
// of its definition first, then any its entries use, in the order found.
func (tb *Termbase) FileLanguages() []string {
	seen := map[string]bool{}
	var out []string
	add := func(l string) {
		if l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	for _, l := range tb.Languages {
		add(l)
	}
	for _, e := range tb.Entries {
		for _, l := range e.Languages {
			add(l.Lang)
		}
	}
	return out
}

// Remap renames the languages of entries: to[lang] is the language its
// terms go under, and an empty value leaves the language out. Languages
// not named stay as they are; two that end up the same are joined. Entries
// left without a term are dropped and counted.
func Remap(entries []Entry, to map[string]string) (out []Entry, dropped int) {
	for _, e := range entries {
		var langs []Language
		at := map[string]int{}
		for _, l := range e.Languages {
			code, named := to[l.Lang]
			if named && code == "" {
				continue
			}
			if !named {
				code = l.Lang
			}
			if i, ok := at[code]; ok {
				langs[i].Fields = append(langs[i].Fields, l.Fields...)
				langs[i].Terms = append(langs[i].Terms, l.Terms...)
				continue
			}
			at[code] = len(langs)
			langs = append(langs, Language{
				Lang:   code,
				Fields: append([]Value(nil), l.Fields...),
				Terms:  append([]Term(nil), l.Terms...),
			})
		}
		terms := 0
		for _, l := range langs {
			terms += len(l.Terms)
		}
		if terms == 0 {
			dropped++
			continue
		}
		e.Languages = langs
		out = append(out, e)
	}
	return out, dropped
}

// SetDefaultStatus gives every term that has no status the one named.
func SetDefaultStatus(entries []Entry, status string) {
	for i := range entries {
		for j := range entries[i].Languages {
			ts := entries[i].Languages[j].Terms
			for k := range ts {
				if ts[k].Status == "" {
					ts[k].Status = status
				}
			}
		}
	}
}

// Label names an entry by its first terms, for lists: at most two terms of
// a language and four in all.
func (e Entry) Label() string {
	var parts []string
	for _, l := range e.Languages {
		for i, t := range l.Terms {
			if i == 2 || len(parts) == 4 {
				break
			}
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, ", ")
}
