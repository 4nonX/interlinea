// Package tm defines Interlinea's translation memory: the data model, the
// Memory interface implemented by every storage backend, and an in-memory
// backend used for tests, ad-hoc lookups against TMX files and short-lived
// project memories.
package tm

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/fuzzy"
	"github.com/4nonX/interlinea/langtag"
)

// Attribute is a named value attached to a unit (client, domain, project).
type Attribute struct {
	Name  string
	Value string
}

// Unit is a translation unit: a source segment and its translation.
type Unit struct {
	ID         int64
	Source     content.Content
	Target     content.Content
	CreatedAt  time.Time
	CreatedBy  string
	ChangedAt  time.Time
	ChangedBy  string
	UsageCount int
	Attributes []Attribute
	Notes      []string
}

// Info describes a memory.
type Info struct {
	Name       string
	SourceLang string
	TargetLang string
}

// Match is a lookup result.
type Match struct {
	Unit  Unit
	Score int
}

// AddPolicy decides what happens when a unit with the same source exists.
type AddPolicy int

const (
	// KeepBoth adds the unit unless an identical source/target pair exists,
	// in which case that unit is updated (change date, attributes).
	KeepBoth AddPolicy = iota
	// Overwrite replaces the target of the most recent unit with the same source.
	Overwrite
)

// LookupOptions controls fuzzy lookup.
type LookupOptions struct {
	// MinScore is the lowest match percentage returned (default 70).
	MinScore int
	// Limit is the maximum number of matches (default 5).
	Limit int
}

func (o LookupOptions) withDefaults() LookupOptions {
	if o.MinScore <= 0 {
		o.MinScore = 70
	}
	if o.MinScore > 100 {
		o.MinScore = 100
	}
	if o.Limit <= 0 {
		o.Limit = 5
	}
	return o
}

// ConcordanceOptions controls concordance search.
type ConcordanceOptions struct {
	// InTarget searches target segments instead of source segments.
	InTarget bool
	// Limit is the maximum number of results (default 50).
	Limit int
}

// Memory is implemented by every translation memory backend.
type Memory interface {
	// Info returns the memory's name and languages.
	Info() Info
	// Add stores a unit according to policy and returns the stored unit.
	Add(ctx context.Context, u Unit, policy AddPolicy) (Unit, error)
	// Lookup returns fuzzy matches for a source segment, best first.
	Lookup(ctx context.Context, source content.Content, opts LookupOptions) ([]Match, error)
	// Concordance returns units containing query (case-insensitive).
	Concordance(ctx context.Context, query string, opts ConcordanceOptions) ([]Unit, error)
	// Count returns the number of units.
	Count(ctx context.Context) (int, error)
	// Each calls fn for every unit in insertion order; an error stops iteration.
	Each(ctx context.Context, fn func(Unit) error) error
}

// BatchAdder is implemented by backends that can store many units in one
// transaction. ImportTMX uses it when available.
type BatchAdder interface {
	AddBatch(ctx context.Context, units []Unit, policy AddPolicy) error
}

// Errors returned by backends.
var (
	ErrEmptySource = errors.New("tm: unit has an empty source")
	ErrEmptyTarget = errors.New("tm: unit has an empty target")
	ErrInvalidInfo = errors.New("tm: memory needs a name and valid source and target languages")
)

// ValidateInfo checks a memory description and normalizes its language tags.
func ValidateInfo(info Info) (Info, error) {
	if strings.TrimSpace(info.Name) == "" {
		return Info{}, ErrInvalidInfo
	}
	src, err := langtag.Normalize(info.SourceLang)
	if err != nil {
		return Info{}, ErrInvalidInfo
	}
	tgt, err := langtag.Normalize(info.TargetLang)
	if err != nil {
		return Info{}, ErrInvalidInfo
	}
	info.Name = strings.TrimSpace(info.Name)
	info.SourceLang, info.TargetLang = src, tgt
	return info, nil
}

// ValidateUnit checks a unit before it is stored.
func ValidateUnit(u Unit) error {
	if strings.TrimSpace(u.Source.Plain()) == "" {
		return ErrEmptySource
	}
	if strings.TrimSpace(u.Target.Plain()) == "" && !u.Target.HasTags() {
		return ErrEmptyTarget
	}
	if err := u.Source.Validate(); err != nil {
		return err
	}
	return u.Target.Validate()
}

// SourceKey returns the exact-match key of a source segment: the text with
// whitespace runs collapsed, plus the tag structure.
func SourceKey(c content.Content) string {
	return collapseSpace(c.Plain()) + "\x00" + c.Signature()
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// SearchTerms returns the distinct, lower-cased word and number tokens of a
// segment, in order of appearance. Backends use them to find candidates.
func SearchTerms(c content.Content) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range fuzzy.Tokenize(c) {
		if t.Kind != fuzzy.Word && t.Kind != fuzzy.Number {
			continue
		}
		k := strings.ToLower(t.Text)
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// SortMatches orders matches best first: higher score, then more recently
// changed, then higher ID.
func SortMatches(ms []Match) {
	sort.SliceStable(ms, func(a, b int) bool {
		if ms[a].Score != ms[b].Score {
			return ms[a].Score > ms[b].Score
		}
		if !ms[a].Unit.ChangedAt.Equal(ms[b].Unit.ChangedAt) {
			return ms[a].Unit.ChangedAt.After(ms[b].Unit.ChangedAt)
		}
		return ms[a].Unit.ID > ms[b].Unit.ID
	})
}

// NormalizeQuery prepares a concordance query: whitespace collapsed, lower case.
func NormalizeQuery(q string) string {
	return strings.ToLower(collapseSpace(q))
}

// ContainsQuery reports whether c contains a query prepared by NormalizeQuery.
func ContainsQuery(c content.Content, normalizedQuery string) bool {
	return normalizedQuery != "" && strings.Contains(strings.ToLower(collapseSpace(c.Plain())), normalizedQuery)
}

// Prepare validates and completes a unit for storage: timestamps default to
// now, ChangedAt to CreatedAt and ChangedBy to CreatedBy. The result is a copy.
func Prepare(u Unit, now time.Time) (Unit, error) {
	if err := ValidateUnit(u); err != nil {
		return Unit{}, err
	}
	out := cloneUnit(u)
	out.CreatedAt = firstNonZero(u.CreatedAt, now)
	out.ChangedAt = firstNonZero(u.ChangedAt, out.CreatedAt)
	if out.ChangedBy == "" {
		out.ChangedBy = out.CreatedBy
	}
	return out, nil
}
