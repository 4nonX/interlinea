package tm

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/fuzzy"
)

// candidateLimit bounds how many units are scored per lookup. Candidates
// are ranked by shared tokens first, so the best fuzzy matches are almost
// always within this set.
const candidateLimit = 200

// InMemory is a Memory kept entirely in RAM. It is safe for concurrent use.
type InMemory struct {
	mu     sync.RWMutex
	info   Info
	units  []Unit
	byKey  map[string][]int // SourceKey -> unit indexes
	index  map[string][]int // folded word or number token -> unit indexes
	nextID int64
	now    func() time.Time
}

var _ Memory = (*InMemory)(nil)

// NewInMemory creates an empty memory.
func NewInMemory(info Info) (*InMemory, error) {
	info, err := ValidateInfo(info)
	if err != nil {
		return nil, err
	}
	return &InMemory{
		info:   info,
		byKey:  map[string][]int{},
		index:  map[string][]int{},
		nextID: 1,
		now:    func() time.Time { return time.Now().UTC() },
	}, nil
}

// Info implements Memory.
func (m *InMemory) Info() Info { return m.info }

// Add implements Memory.
func (m *InMemory) Add(ctx context.Context, u Unit, policy AddPolicy) (Unit, error) {
	if err := ctx.Err(); err != nil {
		return Unit{}, err
	}
	if err := ValidateUnit(u); err != nil {
		return Unit{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	key := SourceKey(u.Source)
	same := m.byKey[key]

	update := -1
	switch policy {
	case Overwrite:
		if n := len(same); n > 0 {
			update = same[n-1]
		}
	default:
		for _, i := range same {
			if m.units[i].Target.Equal(u.Target) {
				update = i
				break
			}
		}
	}
	if update >= 0 {
		ex := &m.units[update]
		ex.Target = u.Target.Clone()
		ex.Source = u.Source.Clone()
		ex.ChangedAt = firstNonZero(u.ChangedAt, now)
		if u.ChangedBy != "" {
			ex.ChangedBy = u.ChangedBy
		}
		if len(u.Attributes) > 0 {
			ex.Attributes = append([]Attribute(nil), u.Attributes...)
		}
		if len(u.Notes) > 0 {
			ex.Notes = append([]string(nil), u.Notes...)
		}
		if u.UsageCount > ex.UsageCount {
			ex.UsageCount = u.UsageCount
		}
		return cloneUnit(*ex), nil
	}

	stored, err := Prepare(u, now)
	if err != nil {
		return Unit{}, err
	}
	stored.ID = m.nextID
	m.nextID++
	idx := len(m.units)
	m.units = append(m.units, stored)
	m.byKey[key] = append(m.byKey[key], idx)
	for _, tok := range SearchTerms(stored.Source) {
		m.index[tok] = append(m.index[tok], idx)
	}
	return cloneUnit(stored), nil
}

// Lookup implements Memory.
func (m *InMemory) Lookup(ctx context.Context, source content.Content, opts LookupOptions) ([]Match, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts = opts.withDefaults()
	m.mu.RLock()
	defer m.mu.RUnlock()

	scored := map[int]int{}
	for _, i := range m.byKey[SourceKey(source)] {
		scored[i] = fuzzy.Score(source, m.units[i].Source)
	}
	for _, i := range m.candidates(source) {
		if _, done := scored[i]; done {
			continue
		}
		scored[i] = fuzzy.Score(source, m.units[i].Source)
	}

	matches := make([]Match, 0, len(scored))
	for i, s := range scored {
		if s >= opts.MinScore {
			matches = append(matches, Match{Unit: cloneUnit(m.units[i]), Score: s})
		}
	}
	SortMatches(matches)
	if len(matches) > opts.Limit {
		matches = matches[:opts.Limit]
	}
	return matches, nil
}

// candidates returns unit indexes sharing tokens with source, most shared first.
// Segments without word or number tokens (only punctuation or tags) fall
// back to all units of similar length.
func (m *InMemory) candidates(source content.Content) []int {
	toks := SearchTerms(source)
	if len(toks) == 0 {
		plainLen := len([]rune(source.Plain()))
		var out []int
		for i := range m.units {
			l := len([]rune(m.units[i].Source.Plain()))
			if l <= plainLen*2+2 && plainLen <= l*2+2 {
				out = append(out, i)
				if len(out) == candidateLimit {
					break
				}
			}
		}
		return out
	}
	shared := map[int]int{}
	for _, tok := range toks {
		for _, i := range m.index[tok] {
			shared[i]++
		}
	}
	out := make([]int, 0, len(shared))
	for i := range shared {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool {
		if shared[out[a]] != shared[out[b]] {
			return shared[out[a]] > shared[out[b]]
		}
		return out[a] > out[b]
	})
	if len(out) > candidateLimit {
		out = out[:candidateLimit]
	}
	return out
}

// Concordance implements Memory.
func (m *InMemory) Concordance(ctx context.Context, query string, opts ConcordanceOptions) ([]Unit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q := NormalizeQuery(query)
	if q == "" {
		return nil, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Unit
	for i := len(m.units) - 1; i >= 0 && len(out) < limit; i-- {
		c := m.units[i].Source
		if opts.InTarget {
			c = m.units[i].Target
		}
		if ContainsQuery(c, q) {
			out = append(out, cloneUnit(m.units[i]))
		}
	}
	return out, nil
}

// SetTarget changes a unit's translation; its source and everything else
// stay. ErrNoUnit if there is no unit with the ID.
func (m *InMemory) SetTarget(ctx context.Context, id int64, target content.Content, by string) (Unit, error) {
	if err := ctx.Err(); err != nil {
		return Unit{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.units {
		if m.units[i].ID != id {
			continue
		}
		changed := m.units[i]
		changed.Target = target.Clone()
		if err := ValidateUnit(changed); err != nil {
			return Unit{}, err
		}
		changed.ChangedAt = m.now()
		if by != "" {
			changed.ChangedBy = by
		}
		m.units[i] = changed
		return cloneUnit(changed), nil
	}
	return Unit{}, ErrNoUnit
}

// Delete removes a unit. ErrNoUnit if there is no unit with the ID.
func (m *InMemory) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	at := -1
	for i := range m.units {
		if m.units[i].ID == id {
			at = i
			break
		}
	}
	if at < 0 {
		return ErrNoUnit
	}
	m.units = append(m.units[:at], m.units[at+1:]...)
	// The indexes hold positions, which moved.
	m.byKey = map[string][]int{}
	m.index = map[string][]int{}
	for i, u := range m.units {
		key := SourceKey(u.Source)
		m.byKey[key] = append(m.byKey[key], i)
		for _, tok := range SearchTerms(u.Source) {
			m.index[tok] = append(m.index[tok], i)
		}
	}
	return nil
}

// Count implements Memory.
func (m *InMemory) Count(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.units), nil
}

// Each implements Memory. The memory is read-locked during iteration, so fn
// must not call Add on the same memory.
func (m *InMemory) Each(ctx context.Context, fn func(Unit) error) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.units {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(cloneUnit(u)); err != nil {
			return err
		}
	}
	return nil
}

func cloneUnit(u Unit) Unit {
	u.Source = u.Source.Clone()
	u.Target = u.Target.Clone()
	u.Attributes = append([]Attribute(nil), u.Attributes...)
	u.Notes = append([]string(nil), u.Notes...)
	return u
}

func firstNonZero(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t.UTC()
}
