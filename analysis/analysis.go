// Package analysis counts the words of documents and analyses them
// against translation memories, as Trados Studio's and memoQ's analysis
// reports do: locked segments, context matches, repetitions within a file
// and across files, exact matches, fuzzy bands and new text, with word,
// character, tag and placeable counts per file and in total. Translators
// and agencies quote from this report.
package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/i18n"
	"github.com/4nonX/interlinea/tm"
)

// Band is a row of the analysis.
type Band string

const (
	Locked       Band = "locked"
	ContextMatch Band = "context"
	Repetition   Band = "repetition"
	CrossFile    Band = "cross-file"
	Exact        Band = "100"
	Fuzzy95      Band = "95"
	Fuzzy85      Band = "85"
	Fuzzy75      Band = "75"
	Fuzzy50      Band = "50"
	New          Band = "new"
)

// Bands lists the rows in report order with their names.
var Bands = []struct {
	Band Band   `json:"band"`
	Name string `json:"name"`
	Hint string `json:"hint"`
}{
	{Locked, i18n.N("Locked"), i18n.N("Locked in the file; not to be translated")},
	{ContextMatch, i18n.N("Context match"), i18n.N("100% match whose neighbouring segments match too (101%)")},
	{Repetition, i18n.N("Repetitions"), i18n.N("Same text and tags earlier in the same file")},
	{CrossFile, i18n.N("Cross-file repetitions"), i18n.N("Same text and tags in an earlier file of the project")},
	{Exact, i18n.N("100%"), i18n.N("Same text and tags as a memory unit")},
	{Fuzzy95, i18n.N("95% to 99%"), ""},
	{Fuzzy85, i18n.N("85% to 94%"), ""},
	{Fuzzy75, i18n.N("75% to 84%"), ""},
	{Fuzzy50, i18n.N("50% to 74%"), i18n.N("Usually charged like new text")},
	{New, i18n.N("New"), i18n.N("No match of 50% or more")},
}

// BandName is the readable name of a band.
func BandName(b Band) string {
	for _, x := range Bands {
		if x.Band == b {
			return x.Name
		}
	}
	return string(b)
}

// ContextRule says which neighbours make a 100% match a context match:
// the segment before (Trados) or the segments before and after (memoQ).
type ContextRule string

const (
	ContextPrevious ContextRule = "previous"
	ContextBoth     ContextRule = "both"
)

// The unit attributes holding a memory unit's neighbouring source
// segments, as memoQ writes them in TMX (x-context-pre, x-context-post).
const (
	AttrContextBefore = "x-context-pre"
	AttrContextAfter  = "x-context-post"
)

// Segment is a source segment to analyse.
type Segment struct {
	ID     string          `json:"id"`
	Source content.Content `json:"source"`
	Locked bool            `json:"locked,omitempty"`
}

// File is a document of the project.
type File struct {
	Name     string    `json:"name"`
	Segments []Segment `json:"segments"`
}

// Options configures an analysis.
type Options struct {
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	// Context is the context match rule (default previous).
	Context ContextRule `json:"context,omitempty"`
	// NoCrossFile counts repetitions across files with the matches they
	// get instead of as cross-file repetitions.
	NoCrossFile bool `json:"noCrossFile,omitempty"`
	// Weights are the share of the full rate charged per band, in
	// percent ({"100": 30, "new": 100}); with weights the report has a
	// weighted word count. Bands without a weight count fully, except
	// locked segments, which count 0 unless given a weight.
	Weights map[Band]float64 `json:"weights,omitempty"`
}

// Counts are the amounts of a band, a file or a project.
type Counts struct {
	Segments int `json:"segments"`
	Words    int `json:"words"`
	// Characters are counted without spaces.
	Characters int `json:"characters"`
	Tags       int `json:"tags"`
	// Placeables are numbers, which translators usually copy unchanged.
	Placeables int `json:"placeables"`
}

func (c *Counts) add(o Counts) {
	c.Segments += o.Segments
	c.Words += o.Words
	c.Characters += o.Characters
	c.Tags += o.Tags
	c.Placeables += o.Placeables
}

// Row is the counts of one band.
type Row struct {
	Band Band   `json:"band"`
	Name string `json:"name"`
	Counts
	// Weighted is Words times the band's weight, with Options.Weights.
	Weighted float64 `json:"weighted,omitempty"`
}

// FileReport is the analysis of one file.
type FileReport struct {
	Name     string  `json:"name"`
	Rows     []Row   `json:"rows"`
	Total    Counts  `json:"total"`
	Weighted float64 `json:"weighted,omitempty"`
}

// SegmentResult says how a segment was counted.
type SegmentResult struct {
	File    string `json:"file"`
	ID      string `json:"id"`
	Band    Band   `json:"band"`
	Score   int    `json:"score,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Counted Counts `json:"counted"`
}

// Report is the analysis of a project.
type Report struct {
	Created    time.Time        `json:"created"`
	SourceLang string           `json:"sourceLang"`
	TargetLang string           `json:"targetLang"`
	Memories   []string         `json:"memories"`
	Context    ContextRule      `json:"context"`
	Files      []FileReport     `json:"files"`
	Rows       []Row            `json:"rows"`
	Total      Counts           `json:"total"`
	Weighted   float64          `json:"weighted,omitempty"`
	Weights    map[Band]float64 `json:"weights,omitempty"`
	Segments   []SegmentResult  `json:"segments"`
}

// Count counts a segment's words, characters, tags and placeables.
func Count(c content.Content) Counts {
	text := c.Plain()
	n := Counts{Segments: 1, Tags: len(c.Tags())}
	for _, r := range text {
		if !unicode.IsSpace(r) {
			n.Characters++
		}
	}
	for _, w := range Words(text) {
		n.Words++
		if isNumber(w) {
			n.Placeables++
		}
	}
	return n
}

// Words splits text into the words that are counted: runs of letters,
// marks and digits, with apostrophes and hyphens inside a word and the
// separators inside a number ("1,250.50") kept. Each character of
// scripts written without spaces (Chinese, Japanese kana, Thai, Lao,
// Khmer, Myanmar) counts as a word, as in Trados; the vowel signs and
// tone marks written on a character belong to it and are not counted.
func Words(text string) []string {
	var out []string
	start := -1
	flush := func(end int) {
		if start >= 0 {
			out = append(out, text[start:end])
			start = -1
		}
	}
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		switch {
		case perCharacter(r) && unicode.IsMark(r):
			// A sign on the character before it, which was counted.
			if n := len(out); n > 0 && start < 0 && strings.HasSuffix(text[:i], out[n-1]) {
				out[n-1] = text[i-len(out[n-1]) : i+size]
			}
		case perCharacter(r):
			flush(i)
			out = append(out, text[i:i+size])
		case unicode.IsLetter(r) || unicode.IsDigit(r) || (unicode.IsMark(r) && start >= 0):
			if start < 0 {
				start = i
			}
		case start >= 0 && joins(text, start, i, r, size):
		default:
			flush(i)
		}
		i += size
	}
	flush(len(text))
	return out
}

// joins reports whether the character at i keeps the current word going:
// an apostrophe or hyphen between letters, or a separator between digits.
func joins(text string, start, i int, r rune, size int) bool {
	if i+size >= len(text) {
		return false
	}
	prev, _ := utf8.DecodeLastRuneInString(text[start:i])
	next, _ := utf8.DecodeRuneInString(text[i+size:])
	switch r {
	case '\u200c', '\u200d':
		// The joiners of Persian and the Indic scripts shape the letters
		// around them; they do not end the word.
		return unicode.IsLetter(prev) || unicode.IsMark(prev)
	case '\'', '’', '-', '‐', '­':
		return (unicode.IsLetter(prev) || unicode.IsDigit(prev)) && (unicode.IsLetter(next) || unicode.IsDigit(next)) && !perCharacter(next)
	case '.', ',', ' ', ' ':
		return unicode.IsDigit(prev) && unicode.IsDigit(next)
	}
	return false
}

func perCharacter(r rune) bool {
	// Their letters, digits and signs; their punctuation (the Burmese and
	// Khmer full stops) is no word.
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) &&
		(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r))
}

func isNumber(w string) bool {
	digits := false
	for _, r := range w {
		switch {
		case unicode.IsDigit(r):
			digits = true
		case r == '.' || r == ',' || r == ' ' || r == ' ' || r == '-':
		default:
			return false
		}
	}
	return digits
}

func bandOf(score int) Band {
	switch {
	case score >= 100:
		return Exact
	case score >= 95:
		return Fuzzy95
	case score >= 85:
		return Fuzzy85
	case score >= 75:
		return Fuzzy75
	case score >= 50:
		return Fuzzy50
	}
	return New
}

// contextKey is how neighbouring segments are compared: text with
// whitespace collapsed (case kept).
func contextKey(c content.Content) string { return strings.Join(strings.Fields(c.Plain()), " ") }

func contextText(s string) string { return strings.Join(strings.Fields(s), " ") }

type best struct {
	score   int
	memory  string
	context bool
	match   tm.Match
}

// Analyze analyses the files against the memories (none for a plain word
// count). Every segment is looked up in every memory; the best match
// decides its band unless it is locked, a context match or a repetition,
// in that order of precedence.
func Analyze(ctx context.Context, files []File, memories []tm.Memory, o Options, now time.Time) (Report, error) {
	if o.Context == "" {
		o.Context = ContextPrevious
	}
	if o.Context != ContextPrevious && o.Context != ContextBoth {
		return Report{}, fmt.Errorf("analysis: context rule must be %q or %q", ContextPrevious, ContextBoth)
	}
	for b, w := range o.Weights {
		if BandName(b) == string(b) {
			return Report{}, fmt.Errorf("analysis: unknown band %q in weights", b)
		}
		if w < 0 {
			return Report{}, fmt.Errorf("analysis: negative weight for %q", b)
		}
	}
	r := Report{Created: now.UTC(), SourceLang: o.SourceLang, TargetLang: o.TargetLang, Context: o.Context, Weights: o.Weights}
	for _, m := range memories {
		r.Memories = append(r.Memories, m.Info().Name)
	}
	type lookupResult struct {
		matches []scoredMatch
	}
	cache := map[string]lookupResult{}
	lookup := func(src content.Content) ([]scoredMatch, error) {
		key := tm.SourceKey(src)
		if res, ok := cache[key]; ok {
			return res.matches, nil
		}
		var all []scoredMatch
		for _, m := range memories {
			ms, err := m.Lookup(ctx, src, tm.LookupOptions{MinScore: 50, Limit: 10})
			if err != nil {
				return nil, fmt.Errorf("analysis: %s: %w", m.Info().Name, err)
			}
			for _, x := range ms {
				all = append(all, scoredMatch{Match: x, memory: m.Info().Name})
			}
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].Score > all[j].Score })
		cache[key] = lookupResult{all}
		return all, nil
	}
	weight := func(b Band) float64 {
		if w, ok := o.Weights[b]; ok {
			return w / 100
		}
		if b == Locked {
			return 0 // locked text is not translated, so not charged
		}
		return 1
	}
	projectRows := map[Band]*Row{}
	seenProject := map[string]bool{}
	for _, f := range files {
		fileRows := map[Band]*Row{}
		seenFile := map[string]bool{}
		fr := FileReport{Name: f.Name}
		for i, s := range f.Segments {
			if err := ctx.Err(); err != nil {
				return Report{}, err
			}
			n := Count(s.Source)
			key := tm.SourceKey(s.Source)
			res := SegmentResult{File: f.Name, ID: s.ID, Counted: n}
			switch {
			case s.Locked:
				res.Band = Locked
			case n.Words == 0 && n.Characters == 0:
				// Tags only: nothing to translate; counted as new so
				// the segment total stays complete.
				res.Band = New
			default:
				ms, err := lookup(s.Source)
				if err != nil {
					return Report{}, err
				}
				b := pick(ms, f.Segments, i, o.Context)
				switch {
				case b.context:
					res.Band = ContextMatch
				case seenFile[key]:
					res.Band = Repetition
				case seenProject[key] && !o.NoCrossFile:
					res.Band = CrossFile
				default:
					res.Band = bandOf(b.score)
				}
				if res.Band != Repetition && res.Band != CrossFile && b.score >= 50 {
					res.Score, res.Memory = b.score, b.memory
				}
			}
			if !s.Locked {
				seenFile[key] = true
			}
			for _, rows := range []map[Band]*Row{fileRows, projectRows} {
				row := rows[res.Band]
				if row == nil {
					row = &Row{Band: res.Band, Name: BandName(res.Band)}
					rows[res.Band] = row
				}
				row.add(n)
			}
			fr.Total.add(n)
			r.Segments = append(r.Segments, res)
		}
		for k := range seenFile {
			seenProject[k] = true
		}
		fr.Rows = rowsInOrder(fileRows, weight, o.Weights != nil)
		for _, row := range fr.Rows {
			fr.Weighted += row.Weighted
		}
		r.Files = append(r.Files, fr)
		r.Total.add(fr.Total)
	}
	r.Rows = rowsInOrder(projectRows, weight, o.Weights != nil)
	for _, row := range r.Rows {
		r.Weighted += row.Weighted
	}
	return r, nil
}

type scoredMatch struct {
	tm.Match
	memory string
}

func attr(u tm.Unit, name string) (string, bool) {
	for _, a := range u.Attributes {
		if strings.EqualFold(a.Name, name) {
			return a.Value, true
		}
	}
	return "", false
}

// pick returns the best match, preferring a 100% match with matching
// context.
func pick(ms []scoredMatch, segs []Segment, i int, rule ContextRule) best {
	if len(ms) == 0 {
		return best{}
	}
	prev, next := "", ""
	if i > 0 {
		prev = contextKey(segs[i-1].Source)
	}
	if i+1 < len(segs) {
		next = contextKey(segs[i+1].Source)
	}
	for _, m := range ms {
		if m.Score < 100 {
			break
		}
		before, ok := attr(m.Unit, AttrContextBefore)
		if !ok || contextText(before) != prev {
			continue
		}
		if rule == ContextBoth {
			after, ok := attr(m.Unit, AttrContextAfter)
			if !ok || contextText(after) != next {
				continue
			}
		}
		return best{score: 100, memory: m.memory, context: true, match: m.Match}
	}
	return best{score: ms[0].Score, memory: ms[0].memory, match: ms[0].Match}
}

// Best is a segment's best memory match, chosen as the analysis chooses it.
type Best struct {
	Match tm.Match
	// Memory is the name of the memory the match comes from.
	Memory string
	// Context reports a context match: a 100% match whose neighbouring
	// segments match too, under the rule.
	Context bool
}

// BestMatch looks segs[i] up in every memory and returns its best match
// of at least min percent: a context match first, then the highest score
// (the first memory wins a tie). ok is false when no memory has one.
func BestMatch(ctx context.Context, segs []Segment, i int, memories []tm.Memory, min int, rule ContextRule) (b Best, ok bool, err error) {
	if i < 0 || i >= len(segs) {
		return Best{}, false, fmt.Errorf("analysis: segment %d of %d", i, len(segs))
	}
	if rule == "" {
		rule = ContextPrevious
	}
	if rule != ContextPrevious && rule != ContextBoth {
		return Best{}, false, fmt.Errorf("analysis: context rule must be %q or %q", ContextPrevious, ContextBoth)
	}
	if min <= 0 {
		min = 50
	}
	var all []scoredMatch
	for _, m := range memories {
		ms, err := m.Lookup(ctx, segs[i].Source, tm.LookupOptions{MinScore: min, Limit: 10})
		if err != nil {
			return Best{}, false, fmt.Errorf("analysis: %s: %w", m.Info().Name, err)
		}
		for _, x := range ms {
			all = append(all, scoredMatch{Match: x, memory: m.Info().Name})
		}
	}
	if len(all) == 0 {
		return Best{}, false, nil
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].Score > all[b].Score })
	p := pick(all, segs, i, rule)
	return Best{Match: p.match, Memory: p.memory, Context: p.context}, true, nil
}

// rowsInOrder lists every band in report order, empty ones included, so
// reports of different projects line up.
func rowsInOrder(rows map[Band]*Row, weight func(Band) float64, weighted bool) []Row {
	out := make([]Row, 0, len(Bands))
	for _, b := range Bands {
		row := Row{Band: b.Band, Name: b.Name}
		if r := rows[b.Band]; r != nil {
			row = *r
		}
		if weighted {
			row.Weighted = float64(row.Words) * weight(b.Band)
		}
		out = append(out, row)
	}
	return out
}
