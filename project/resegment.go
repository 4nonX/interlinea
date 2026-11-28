package project

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

// A file's segments start as the units its filter read. A translator may
// split any of them, and join any two that follow each other, in one
// paragraph or across paragraphs, as often as wanted:
// a stretch of one or more units of the file is then translated as one
// segment or as parts cut wherever the translator chose. The file itself
// is never read differently: the translations of a stretch's parts go
// back one after the other where its units stood. Translations turns
// segments into what the filter takes.

// SplitPart says that a segment is a part of a stretch that was split.
type SplitPart struct {
	// Unit is the ID of the first unit of the stretch the part belongs to.
	Unit string `json:"unit"`
	// Space is the whitespace that stood before the part where the
	// stretch was cut; the first part has none.
	Space string `json:"space,omitempty"`
}

// JoinedUnit is one of the units of the file a stretch of several was
// joined from. Every part of such a stretch lists them.
type JoinedUnit struct {
	ID     string          `json:"id"`
	Source content.Content `json:"source"`
}

// ErrResegment is what Split and Join return when the segments
// do not allow it; its text says why, for the person asking.
type ErrResegment struct{ Message string }

func (e *ErrResegment) Error() string { return e.Message }

func refuse(format string, args ...any) error {
	return &ErrResegment{Message: fmt.Sprintf(format, args...)}
}

func blank(c content.Content) bool { return strings.TrimSpace(c.Plain()) == "" }

func statusOf(c content.Content) Status {
	if blank(c) {
		return StatusNew
	}
	return StatusDraft
}

// cut divides src where before ends. It reports false when before is not
// how src starts.
func cut(src, before content.Content) (left, right content.Content, ok bool) {
	src, before = src.Normalize(), before.Normalize()
	left, right = content.Content{}, content.Content{}
	i := 0
	for k, b := range before {
		if i >= len(src) || src[i].IsTag() != b.IsTag() {
			return nil, nil, false
		}
		if b.IsTag() {
			if src[i].Tag.Kind != b.Tag.Kind || src[i].Tag.ID != b.Tag.ID {
				return nil, nil, false
			}
			left = left.AppendTag(*src[i].Tag)
			i++
			continue
		}
		switch {
		case src[i].Text == b.Text:
			left = left.Append(b.Text)
			i++
		case k == len(before)-1 && strings.HasPrefix(src[i].Text, b.Text) && utf8.ValidString(src[i].Text[len(b.Text):]):
			left = left.Append(b.Text)
			right = right.Append(src[i].Text[len(b.Text):])
			i++
		default:
			return nil, nil, false
		}
	}
	for ; i < len(src); i++ {
		if src[i].IsTag() {
			right = right.AppendTag(*src[i].Tag)
		} else {
			right = right.Append(src[i].Text)
		}
	}
	return left, right, true
}

// trimRight takes the whitespace off the end of c.
func trimRight(c content.Content) (content.Content, string) {
	if n := len(c); n > 0 && !c[n-1].IsTag() {
		t := strings.TrimRightFunc(c[n-1].Text, unicode.IsSpace)
		space := c[n-1].Text[len(t):]
		if t == "" {
			return c[:n-1], space
		}
		c[n-1].Text = t
		return c, space
	}
	return c, ""
}

// trimLeft takes the whitespace off the start of c.
func trimLeft(c content.Content) (content.Content, string) {
	if len(c) > 0 && !c[0].IsTag() {
		t := strings.TrimLeftFunc(c[0].Text, unicode.IsSpace)
		space := c[0].Text[:len(c[0].Text)-len(t)]
		if t == "" {
			return c[1:], space
		}
		c[0].Text = t
		return c, space
	}
	return c, ""
}

// hasText reports whether c has letters or digits to translate.
func hasText(c content.Content) bool {
	for _, r := range c.Plain() {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// tagOf finds src's tag of a kind and ID.
func tagOf(src content.Content, kind content.TagKind, id string) (content.Tag, bool) {
	for _, r := range src {
		if r.IsTag() && r.Tag.Kind == kind && r.Tag.ID == id {
			return *r.Tag, true
		}
	}
	return content.Tag{}, false
}

// Split cuts segment i in two where before, the start of its source,
// ends. The translation stays with the first part. A pair of tags open
// at the cut is closed in the first part and opened again in the second.
// It returns the file's segments with the two parts in place of the one.
func Split(segs []Segment, i int, before content.Content) ([]Segment, error) {
	if i < 0 || i >= len(segs) {
		return nil, errors.New("project: no such segment")
	}
	s := segs[i]
	switch {
	case s.Locked:
		return nil, refuse("The segment is locked.")
	case fixed(s):
		return nil, refuse("%s", FixedMessage)
	}
	left, right, ok := cut(s.Source, before)
	if !ok {
		return nil, refuse("The place to split at is not in the segment's source.")
	}
	left, trail := trimRight(left)
	right, lead := trimLeft(right)
	if !hasText(left) || !hasText(right) {
		return nil, refuse("Both parts need text: choose a place inside the segment.")
	}
	// Pairs open at the cut.
	var open []content.Tag
	for _, r := range left {
		if !r.IsTag() {
			continue
		}
		switch r.Tag.Kind {
		case content.TagOpen:
			open = append(open, *r.Tag)
		case content.TagClose:
			for k := len(open) - 1; k >= 0; k-- {
				if open[k].ID == r.Tag.ID {
					open = append(open[:k], open[k+1:]...)
					break
				}
			}
		}
	}
	var reopened content.Content
	for k := len(open) - 1; k >= 0; k-- {
		cl, ok := tagOf(s.Source, content.TagClose, open[k].ID)
		if !ok {
			continue // a pair that ends in a later segment needs no closing here
		}
		left = left.AppendTag(cl)
		reopened = append(content.Content{}.AppendTag(open[k]), reopened...)
	}
	right = append(reopened, right...)

	r := runAt(segs, i)
	a, b := s, s
	a.Source, b.Source = left, right
	a.Target = s.Target.Clone()
	b.Target = content.Content{}
	a.Status, b.Status = statusOf(a.Target), StatusNew
	a.Origin, a.Warnings, a.Ignored, a.File = nil, nil, nil, nil
	b.Origin, b.Warnings, b.Ignored, b.Comments, b.File = nil, nil, nil, nil, nil
	b.Issues = nil
	if s.Split == nil {
		a.ID = r.key + "#1"
		a.Split = &SplitPart{Unit: r.key}
	} else {
		part := *s.Split
		a.Split = &part
	}
	b.ID = freshID(segs, r.key, a.ID)
	b.Split = &SplitPart{Unit: r.key, Space: trail + lead}
	out := make([]Segment, 0, len(segs)+1)
	out = append(out, segs[:i]...)
	out = append(out, a, b)
	out = append(out, segs[i+1:]...)
	return settle(out, i), nil
}

// FixedMessage says why a segment of a file whose tool keeps its own
// record of each segment is not cut differently.
const FixedMessage = "The program this file comes from keeps its own record of every segment, so its segments stay as they are."

// fixed reports whether the segment's file keeps a record of it that
// another cut would break.
func fixed(s Segment) bool { return s.File != nil && s.File.Fixed }

// freshID is a name for a new part of the run key that no segment has.
func freshID(segs []Segment, key string, also ...string) string {
	taken := map[string]bool{}
	for _, o := range segs {
		taken[o.ID] = true
	}
	for _, id := range also {
		taken[id] = true
	}
	for n := 1; ; n++ {
		if id := key + "#" + strconv.Itoa(n); !taken[id] {
			return id
		}
	}
}

// A run is the segments that stand for one stretch of the file: one unit
// or several joined ones, as one segment or cut into parts.
type run struct {
	// key names the run: the ID of its first unit.
	key string
	// units are the file's units the run stands for.
	units []JoinedUnit
	// The run is segs[lo:hi].
	lo, hi int
}

// pieces puts the sources of parts together as their run reads.
func pieces(parts []Segment) content.Content {
	out := content.Content{}
	for k, p := range parts {
		if k == 0 {
			out = p.Source.Clone()
			continue
		}
		space := ""
		if p.Split != nil {
			space = p.Split.Space
		}
		a, b := heal(out, p.Source.Clone())
		out = append(a.Append(space), b...).Normalize()
	}
	return out
}

// runAt finds the run segment i belongs to.
func runAt(segs []Segment, i int) run {
	s := segs[i]
	if s.Split == nil {
		r := run{key: s.ID, lo: i, hi: i + 1, units: s.Joined}
		if len(r.units) == 0 {
			r.units = []JoinedUnit{{ID: s.ID, Source: s.Source}}
		} else {
			r.key = s.Joined[0].ID
		}
		return r
	}
	r := run{key: s.Split.Unit, lo: i, hi: i + 1}
	for r.lo > 0 && segs[r.lo-1].Split != nil && segs[r.lo-1].Split.Unit == r.key {
		r.lo--
	}
	for r.hi < len(segs) && segs[r.hi].Split != nil && segs[r.hi].Split.Unit == r.key {
		r.hi++
	}
	if r.units = segs[r.lo].Joined; len(r.units) == 0 {
		r.units = []JoinedUnit{{ID: r.key, Source: pieces(segs[r.lo:r.hi])}}
	}
	return r
}

// retag gives the tags of c the IDs the map has for them.
func retag(c content.Content, ids map[string]string) content.Content {
	out := c.Clone()
	for k := range out {
		if out[k].IsTag() {
			if id, ok := ids[out[k].Tag.ID]; ok {
				out[k].Tag.ID = id
			}
		}
	}
	return out
}

// shape gives the segments of a run, segs[lo:hi], the names and marks of
// what they are now: the run of units, as one segment or as its parts.
func shape(segs []Segment, lo, hi int, units []JoinedUnit) {
	key := units[0].ID
	var joined []JoinedUnit
	if len(units) > 1 {
		joined = units
	}
	if hi-lo == 1 {
		s := &segs[lo]
		s.ID, s.Split, s.Joined = key, nil, joined
		if joined != nil {
			s.Source = JoinedSource(units)
		} else {
			s.Source = units[0].Source.Clone()
		}
		return
	}
	// Parts keep their names where they are parts of this run already.
	keep := map[string]bool{}
	for k := lo; k < hi; k++ {
		if strings.HasPrefix(segs[k].ID, key+"#") && !keep[segs[k].ID] {
			keep[segs[k].ID] = true
		} else {
			segs[k].ID = ""
		}
	}
	var given []string
	for k := lo; k < hi; k++ {
		s := &segs[k]
		space := ""
		if s.Split != nil && k > lo {
			space = s.Split.Space
		}
		if s.ID == "" {
			s.ID = freshID(segs, key, given...)
			given = append(given, s.ID)
		}
		s.Split, s.Joined = &SplitPart{Unit: key, Space: space}, joined
	}
}

// settle takes a run of several units apart again where its parts end
// where its units do: each side is a run of its own then, and a unit
// that is one part again is the segment the file had. This is how what
// was joined comes apart: by splitting it where it was joined.
func settle(segs []Segment, i int) []Segment {
	r := runAt(segs, i)
	if len(r.units) < 2 || r.hi-r.lo < 2 {
		return segs
	}
	// Where the units and the parts start and end in the run's text.
	starts, ends := make([]int, len(r.units)), make([]int, len(r.units))
	for k := range r.units {
		whole, _ := fold(r.units[:k+1])
		ends[k] = len(whole.Plain())
		starts[k] = ends[k] - len(r.units[k].Source.Plain())
	}
	pStart, pEnd := make([]int, r.hi-r.lo), make([]int, r.hi-r.lo)
	at := 0
	for j := r.lo; j < r.hi; j++ {
		if j > r.lo {
			at += len(segs[j].Split.Space)
		}
		pStart[j-r.lo] = at
		at += len(segs[j].Source.Plain())
		pEnd[j-r.lo] = at
	}
	// cutAfter[k]: the part that ends where unit k ends, with the next
	// part starting where the next unit does.
	type group struct{ u0, u1, p0, p1 int }
	var groups []group
	u0, p0 := 0, 0
	for k := 0; k+1 < len(r.units); k++ {
		for j := p0; j+1 < len(pEnd); j++ {
			if pEnd[j] == ends[k] && pStart[j+1] == starts[k+1] {
				groups = append(groups, group{u0, k + 1, p0, j + 1})
				u0, p0 = k+1, j+1
				break
			}
		}
	}
	if len(groups) == 0 {
		return segs
	}
	groups = append(groups, group{u0, len(r.units), p0, len(pEnd)})
	_, whole := fold(r.units)
	out := append([]Segment{}, segs...)
	for _, g := range groups {
		units := append([]JoinedUnit{}, r.units[g.u0:g.u1]...)
		// The tags take the IDs they have in the smaller run.
		_, own := fold(units)
		ids := map[string]string{}
		for k := range units {
			for id, was := range whole[g.u0+k] {
				ids[was] = own[k][id]
			}
		}
		for j := r.lo + g.p0; j < r.lo+g.p1; j++ {
			out[j].Source = retag(out[j].Source, ids)
			out[j].Target = retag(out[j].Target, ids)
			out[j].Joined = nil
		}
		shape(out, r.lo+g.p0, r.lo+g.p1, units)
	}
	return out
}

// separator is what stands between two joined sentences: a space, or
// nothing in writing that sets none between sentences.
func separator(a, b content.Content) string {
	last, _ := utf8.DecodeLastRuneInString(a.Plain())
	first, _ := utf8.DecodeRuneInString(b.Plain())
	spaceless := func(r rune) bool {
		return r >= 0x2E80 || unicode.In(r, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar)
	}
	if spaceless(last) && spaceless(first) {
		return ""
	}
	return " "
}

// fold joins units into one source. ids[k] maps unit k's own tag IDs to
// the ones they have in it.
func fold(units []JoinedUnit) (content.Content, []map[string]string) {
	out := content.Content{}
	ids := make([]map[string]string, len(units))
	for k, u := range units {
		ids[k] = map[string]string{}
		if k == 0 {
			out = u.Source.Clone()
			for _, t := range u.Source.Tags() {
				ids[k][t.ID] = t.ID
			}
			continue
		}
		more, back := filter.JoinTags(out, u.Source)
		for id, own := range back {
			ids[k][own] = id
		}
		out = out.Append(separator(out, more))
		out = append(out, more...).Normalize()
	}
	return out, ids
}

// JoinedSource is the source of a segment joined from units.
func JoinedSource(units []JoinedUnit) content.Content {
	src, _ := fold(units)
	return src
}

// heal drops a pair's closing at the end of a and its opening again at
// the start of b, which a split put there.
func heal(a, b content.Content) (content.Content, content.Content) {
	for len(a) > 0 && len(b) > 0 && a[len(a)-1].IsTag() && b[0].IsTag() &&
		a[len(a)-1].Tag.Kind == content.TagClose && b[0].Tag.Kind == content.TagOpen && a[len(a)-1].Tag.ID == b[0].Tag.ID {
		a, b = a[:len(a)-1], b[1:]
	}
	return a, b
}

// onePair makes one pair of a pair of tags that stands twice in c, as the
// translations of two parts of a split segment have it: the first opening
// and the last closing stay.
func onePair(c content.Content) content.Content {
	opens, closes := map[string]int{}, map[string]int{}
	for _, r := range c {
		if r.IsTag() && r.Tag.Kind == content.TagOpen {
			opens[r.Tag.ID]++
		}
		if r.IsTag() && r.Tag.Kind == content.TagClose {
			closes[r.Tag.ID]++
		}
	}
	out := content.Content{}
	seenOpen, seenClose := map[string]int{}, map[string]int{}
	for _, r := range c {
		switch {
		case !r.IsTag():
			out = out.Append(r.Text)
			continue
		case r.Tag.Kind == content.TagOpen && opens[r.Tag.ID] > 1:
			seenOpen[r.Tag.ID]++
			if seenOpen[r.Tag.ID] > 1 {
				continue
			}
		case r.Tag.Kind == content.TagClose && opens[r.Tag.ID] > 1:
			seenClose[r.Tag.ID]++
			if seenClose[r.Tag.ID] < closes[r.Tag.ID] {
				continue
			}
		}
		out = out.AppendTag(*r.Tag)
	}
	return out
}

func mergeNotes(a, b []string) []string {
	out := append([]string{}, a...)
	for _, n := range b {
		found := false
		for _, o := range out {
			found = found || o == n
		}
		if !found {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Join makes one segment of segment i and the one after it, in one
// paragraph or not: what the source has in two paragraphs a translation
// may have in one, as languages divide a text differently. Their
// translations are put together. Two parts of what was split become one
// part; otherwise the two stretches of the file they belong to become
// one, with all their parts. The file is written with the translation
// where the first stood, the paragraphs after it without what was joined
// to it (filter.Joined). It returns the file's segments as they are then.
func Join(segs []Segment, i int) ([]Segment, error) {
	return join(segs, i, false)
}

// JoinInParagraph joins as Join does, but only segments of one paragraph:
// in XLIFF a unit is a paragraph of the program that made the file, which
// reads a unit without a translation as not translated, and so writes its
// source again.
func JoinInParagraph(segs []Segment, i int) ([]Segment, error) {
	return join(segs, i, true)
}

func join(segs []Segment, i int, inParagraph bool) ([]Segment, error) {
	if i < 0 || i+1 >= len(segs) {
		return nil, refuse("There is no segment after this one to join it with.")
	}
	x, y := segs[i], segs[i+1]
	switch {
	case x.Locked || y.Locked:
		return nil, refuse("One of the two segments is locked.")
	case fixed(x) || fixed(y):
		return nil, refuse("%s", FixedMessage)
	}
	rx, ry := runAt(segs, i), runAt(segs, i+1)
	if inParagraph && rx.lo != ry.lo && (x.Block == "" || x.Block != y.Block) {
		return nil, refuse("The two segments are in different units of the file. The program the file goes back to would read the second as not translated.")
	}
	glue := func(a, b content.Content, space string) content.Content {
		switch {
		case blank(a):
			return b.Clone()
		case blank(b):
			return a.Clone()
		}
		out := a.Clone().Append(space)
		return onePair(append(out, b.Clone()...).Normalize())
	}
	out := append([]Segment{}, segs...)
	units := rx.units
	space := separator(x.Source, y.Source)
	if rx.lo == ry.lo {
		space = y.Split.Space
	} else {
		// The second stretch's tags count on from the first's.
		units = append(append([]JoinedUnit{}, rx.units...), ry.units...)
		_, now := fold(units)
		_, was := fold(ry.units)
		ids := map[string]string{}
		for k := range ry.units {
			for own, id := range was[k] {
				ids[id] = now[len(rx.units)+k][own]
			}
		}
		for j := ry.lo; j < ry.hi; j++ {
			out[j].Source = retag(out[j].Source, ids)
			out[j].Target = retag(out[j].Target, ids)
		}
		y = out[i+1]
	}
	z := x
	a, b := heal(x.Source.Clone(), y.Source.Clone())
	z.Source = append(a.Append(space), b...).Normalize()
	z.Target = glue(x.Target, y.Target, space)
	z.Status = statusOf(z.Target)
	z.Origin, z.Warnings, z.Ignored, z.File = nil, nil, nil, nil
	z.Comments = append(append([]Comment{}, x.Comments...), y.Comments...)
	if len(z.Comments) == 0 {
		z.Comments = nil
	}
	z.Issues = append(append([]Issue{}, x.Issues...), y.Issues...)
	if len(z.Issues) == 0 {
		z.Issues = nil
	}
	z.Notes = mergeNotes(x.Notes, y.Notes)
	if x.MaxLength > 0 && y.MaxLength > 0 {
		z.MaxLength = x.MaxLength + y.MaxLength
	} else {
		z.MaxLength = 0
	}
	// Two subtitles joined are shown from the first's start to the
	// second's end.
	if x.Timing != nil && y.Timing != nil {
		z.Timing = &filter.Timing{Start: min(x.Timing.Start, y.Timing.Start), End: max(x.Timing.End, y.Timing.End)}
	} else {
		z.Timing = nil
	}
	out[i] = z
	out = append(out[:i+1], out[i+2:]...)
	lo, hi := rx.lo, ry.hi-1
	// Parts of the second stretch are named after the first.
	shape(out, lo, hi, units)
	return out, nil
}

// Unit is a unit of the file as its segments stand for it.
type Unit struct {
	ID string
	// Source is the unit's source: as the file has it for joined units,
	// put together from its parts for a unit that was only split.
	Source content.Content
	// Segments are the positions of the segments that stand for it,
	// which joined units share.
	Segments []int
}

// Units lists the units a file's segments stand for, in the file's order,
// so the segments can be checked against the file.
func Units(segs []Segment) []Unit {
	var out []Unit
	for i := 0; i < len(segs); {
		r := runAt(segs, i)
		var at []int
		for j := r.lo; j < r.hi; j++ {
			at = append(at, j)
		}
		for _, u := range r.units {
			out = append(out, Unit{ID: u.ID, Source: u.Source, Segments: at})
		}
		i = r.hi
	}
	return out
}

// squeeze is text without its whitespace, to compare what parts and
// units read.
func squeeze(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// CheckRuns reports the first segment whose stretch of the file is not
// what its parts read: segments sent by an app must stand for their
// units, cut however they are. It returns -1 when all do.
func CheckRuns(segs []Segment) int {
	for i := 0; i < len(segs); {
		r := runAt(segs, i)
		if r.hi-r.lo > 1 || len(segs[i].Joined) > 0 {
			if squeeze(pieces(segs[r.lo:r.hi]).Plain()) != squeeze(JoinedSource(r.units).Plain()) {
				return i
			}
			seen := map[string]bool{}
			for j := r.lo; j < r.hi; j++ {
				if seen[segs[j].ID] || (j > r.lo && len(segs[j].Joined) != len(segs[r.lo].Joined)) {
					return j
				}
				seen[segs[j].ID] = true
			}
		}
		i = r.hi
	}
	return -1
}

// A Stretch is what stands for one stretch of the file when it is
// written: the units, and the segments that translate them.
type Stretch struct {
	// Units are the IDs of the file's units, in order.
	Units []string
	// Segments are the segments that stand for them: one, or the parts.
	Segments []Segment
}

// Stretches lists the file's stretches in order.
func Stretches(segs []Segment) []Stretch {
	var out []Stretch
	for i := 0; i < len(segs); {
		r := runAt(segs, i)
		st := Stretch{Segments: segs[r.lo:r.hi]}
		for _, u := range r.units {
			st.Units = append(st.Units, u.ID)
		}
		out = append(out, st)
		i = r.hi
	}
	return out
}

// Translated reports whether any of the stretch is translated.
func (st Stretch) Translated() bool {
	for _, s := range st.Segments {
		if !blank(s.Target) {
			return true
		}
	}
	return false
}

// Target is the stretch's translation: its segments' translations one
// after the other. A part without one stands as its source when
// sourceForBlank is set, and is left out otherwise.
func (st Stretch) Target(sourceForBlank bool) content.Content {
	if len(st.Segments) == 1 {
		return st.Segments[0].Target
	}
	whole := content.Content{}
	for k, s := range st.Segments {
		part := s.Target
		if blank(part) {
			if !sourceForBlank {
				continue
			}
			part = s.Source
		}
		if k > 0 && len(whole) > 0 && s.Split != nil {
			whole = whole.Append(s.Split.Space)
		}
		whole = append(whole, part.Clone()...)
	}
	return whole.Normalize()
}

// Translations is what a document's filter takes for the segments: each
// stretch's translation under its first unit, the other units marked as
// joined to it. Parts are put together in order, a part without a
// translation keeping its source. Stretches without any translation are
// left out, so the document keeps their source.
func Translations(segs []Segment) filter.Translations {
	tr := filter.Translations{}
	for _, st := range Stretches(segs) {
		if !st.Translated() {
			continue
		}
		tr[st.Units[0]] = st.Target(true)
		for _, id := range st.Units[1:] {
			tr[id] = filter.Joined()
		}
	}
	return tr
}
