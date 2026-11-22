package term

import "fmt"

// Match says how incoming entries are matched to existing ones.
type Match string

const (
	// MatchNone adds every incoming entry as a new entry.
	MatchNone Match = "none"
	// MatchTerm matches entries that share a term in a language (MultiTerm's
	// "synchronize on term").
	MatchTerm Match = "term"
	// MatchID matches entries with the same entry ID ("synchronize on entry").
	MatchID Match = "id"
)

// OnMatch says what happens to an existing entry an incoming one matches.
type OnMatch string

const (
	// Merge adds what the existing entry lacks: languages, terms and field
	// values. Existing content is never removed or changed.
	Merge OnMatch = "merge"
	// Overwrite replaces the existing entry's content with the incoming one.
	Overwrite OnMatch = "overwrite"
	// Skip keeps the existing entry and ignores the incoming one.
	Skip OnMatch = "skip"
	// AddNew adds the incoming entry as a new entry anyway.
	AddNew OnMatch = "add"
)

// Policy controls an import into an existing termbase.
type Policy struct {
	Match   Match   `json:"match"`
	Lang    string  `json:"lang,omitempty"` // for MatchTerm: the language compared; "" for any
	OnMatch OnMatch `json:"onMatch"`
}

// Action is what an import does with one incoming entry.
type Action struct {
	Kind string `json:"kind"` // "add", "merge", "overwrite", "skip"
	// Existing is the ID of the matched entry (merge, overwrite, skip).
	Existing string `json:"existing,omitempty"`
	// Entry is the resulting entry (add, merge, overwrite).
	Entry *Entry `json:"entry,omitempty"`
	// Incoming is the index of the incoming entry.
	Incoming int `json:"incoming"`
	// Ambiguous lists further entries that matched too.
	Ambiguous []string `json:"ambiguous,omitempty"`
}

// PlanReport sums up a plan.
type PlanReport struct {
	Added       int      `json:"added"`
	Merged      int      `json:"merged"`
	Overwritten int      `json:"overwritten"`
	Skipped     int      `json:"skipped"`
	Unchanged   int      `json:"unchanged"`
	Warnings    []string `json:"warnings,omitempty"`
}

// Plan works out what importing incoming into existing does, without
// changing either. Callers apply the actions (and record history).
// Entries added during the plan are matched too, so duplicates inside the
// file are merged with each other under MatchTerm.
func Plan(existing, incoming []Entry, p Policy) ([]Action, PlanReport) {
	var rep PlanReport
	byTerm := map[string][]int{} // lang|key -> indexes into pool
	byID := map[string]int{}
	pool := make([]Entry, 0, len(existing)+len(incoming))
	poolAction := make([]int, 0, cap(pool)) // index into actions for pool entries created by the plan; -1 for existing
	index := func(i int) {
		e := pool[i]
		if e.ID != "" {
			if _, ok := byID[e.ID]; !ok {
				byID[e.ID] = i
			}
		}
		for _, l := range e.Languages {
			for _, t := range l.Terms {
				k := l.Lang + "|" + Key(t.Text)
				byTerm[k] = appendUnique(byTerm[k], i)
			}
		}
	}
	for _, e := range existing {
		pool = append(pool, e)
		poolAction = append(poolAction, -1)
		index(len(pool) - 1)
	}
	var actions []Action
	for n, in := range incoming {
		var hits []int
		switch p.Match {
		case MatchID:
			if i, ok := byID[in.ID]; ok && in.ID != "" {
				hits = []int{i}
			}
		case MatchTerm:
			for _, l := range in.Languages {
				if p.Lang != "" && !langEqual(l.Lang, p.Lang) {
					continue
				}
				for _, t := range l.Terms {
					for _, i := range byTerm[l.Lang+"|"+Key(t.Text)] {
						hits = appendUnique(hits, i)
					}
				}
			}
		}
		if len(hits) == 0 || p.OnMatch == AddNew {
			e := in
			actions = append(actions, Action{Kind: "add", Entry: &e, Incoming: n})
			rep.Added++
			pool = append(pool, e)
			poolAction = append(poolAction, len(actions)-1)
			index(len(pool) - 1)
			continue
		}
		target := hits[0]
		var ambiguous []string
		for _, h := range hits[1:] {
			ambiguous = append(ambiguous, idOf(pool[h], h))
		}
		if len(ambiguous) > 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("Entry %d of the file matches %d entries; it was combined with the first.", n+1, len(hits)))
		}
		switch p.OnMatch {
		case Skip:
			actions = append(actions, Action{Kind: "skip", Existing: idOf(pool[target], target), Incoming: n, Ambiguous: ambiguous})
			rep.Skipped++
		case Overwrite:
			e := in
			e.ID = pool[target].ID
			e.CreatedAt, e.CreatedBy = pool[target].CreatedAt, pool[target].CreatedBy
			pool[target] = e
			index(target)
			if a := poolAction[target]; a >= 0 {
				actions[a].Entry = &e
			} else {
				actions = append(actions, Action{Kind: "overwrite", Existing: e.ID, Entry: &e, Incoming: n, Ambiguous: ambiguous})
			}
			rep.Overwritten++
		default: // Merge
			merged, changed := MergeEntries(pool[target], in)
			if !changed {
				actions = append(actions, Action{Kind: "skip", Existing: idOf(pool[target], target), Incoming: n})
				rep.Unchanged++
				continue
			}
			pool[target] = merged
			index(target)
			if a := poolAction[target]; a >= 0 {
				actions[a].Entry = &merged
			} else {
				actions = append(actions, Action{Kind: "merge", Existing: merged.ID, Entry: &merged, Incoming: n, Ambiguous: ambiguous})
			}
			rep.Merged++
		}
	}
	return actions, rep
}

func idOf(e Entry, i int) string {
	if e.ID != "" {
		return e.ID
	}
	return fmt.Sprintf("#%d", i+1)
}

func appendUnique(xs []int, x int) []int {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func langEqual(a, b string) bool {
	return NormalizeLang(a) == NormalizeLang(b)
}

// MergeEntries adds to a what it lacks from b: languages, terms (matched
// by text), and field values (matched by field and value). A term status
// is only filled in where a has none.
func MergeEntries(a, b Entry) (Entry, bool) {
	out := cloneEntry(a)
	changed := false
	out.Fields, changed = mergeValues(out.Fields, b.Fields, changed)
	for _, bl := range b.Languages {
		al := out.Lang(bl.Lang)
		if al == nil {
			out.Languages = append(out.Languages, cloneLang(bl))
			changed = true
			continue
		}
		al.Fields, changed = mergeValues(al.Fields, bl.Fields, changed)
		for _, bt := range bl.Terms {
			found := false
			for i := range al.Terms {
				at := &al.Terms[i]
				if Key(at.Text) != Key(bt.Text) {
					continue
				}
				found = true
				if at.Status == "" && bt.Status != "" {
					at.Status = bt.Status
					changed = true
				}
				at.Fields, changed = mergeValues(at.Fields, bt.Fields, changed)
			}
			if !found {
				al.Terms = append(al.Terms, cloneTerm(bt))
				changed = true
			}
		}
	}
	return out, changed
}

func mergeValues(a, b []Value, changed bool) ([]Value, bool) {
	for _, v := range b {
		has := false
		for _, w := range a {
			if w.Field == v.Field && w.Value == v.Value {
				has = true
				break
			}
		}
		if !has {
			a = append(a, v)
			changed = true
		}
	}
	return a, changed
}

func cloneTerm(t Term) Term {
	t.Fields = append([]Value(nil), t.Fields...)
	return t
}

func cloneLang(l Language) Language {
	l.Fields = append([]Value(nil), l.Fields...)
	terms := make([]Term, len(l.Terms))
	for i, t := range l.Terms {
		terms[i] = cloneTerm(t)
	}
	l.Terms = terms
	return l
}

func cloneEntry(e Entry) Entry {
	e.Fields = append([]Value(nil), e.Fields...)
	langs := make([]Language, len(e.Languages))
	for i, l := range e.Languages {
		langs[i] = cloneLang(l)
	}
	e.Languages = langs
	return e
}

// Duplicates groups entries that share a term in lang ("" for any
// language): candidates for merging, as MultiTerm's duplicate filter shows.
func Duplicates(entries []Entry, lang string) [][]int {
	byTerm := map[string][]int{}
	var keys []string
	for i, e := range entries {
		for _, l := range e.Languages {
			if lang != "" && !langEqual(l.Lang, lang) {
				continue
			}
			for _, t := range l.Terms {
				k := NormalizeLang(l.Lang) + "|" + Key(t.Text)
				if _, ok := byTerm[k]; !ok {
					keys = append(keys, k)
				}
				byTerm[k] = appendUnique(byTerm[k], i)
			}
		}
	}
	// Join overlapping groups (A shares with B, B with C).
	parent := make([]int, len(entries))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for _, k := range keys {
		g := byTerm[k]
		for _, i := range g[1:] {
			parent[find(i)] = find(g[0])
		}
	}
	groups := map[int][]int{}
	var roots []int
	for i := range entries {
		r := find(i)
		if _, ok := groups[r]; !ok {
			roots = append(roots, r)
		}
		groups[r] = append(groups[r], i)
	}
	var out [][]int
	for _, r := range roots {
		if len(groups[r]) > 1 {
			out = append(out, groups[r])
		}
	}
	return out
}
