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
// split one in two where the filter saw one sentence, and join two of a
// paragraph where it saw two. The file itself is never read differently:
// a split segment's parts go back into their unit one after the other,
// and joined segments are written as one translation where their units
// stood. Translations turns segments into what the filter takes.

// SplitPart says that a segment is a part of a unit that was split.
type SplitPart struct {
	// Unit is the ID of the unit the part belongs to.
	Unit string `json:"unit"`
	// Space is the whitespace that stood before the part where the unit
	// was cut; the first part has none.
	Space string `json:"space,omitempty"`
}

// JoinedUnit is one of the units a joined segment was made of.
type JoinedUnit struct {
	ID     string          `json:"id"`
	Source content.Content `json:"source"`
}

// ErrResegment is what Split, Join and Separate return when the segments
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
	case len(s.Joined) > 0:
		return nil, refuse("The segment was joined from %d: separate it instead.", len(s.Joined))
	case s.File != nil:
		return nil, refuse("The file brings its own segments, which stay as they are.")
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

	unit := s.ID
	if s.Split != nil {
		unit = s.Split.Unit
	}
	a, b := s, s
	a.Source, b.Source = left, right
	a.Target = s.Target.Clone()
	b.Target = content.Content{}
	a.Status, b.Status = statusOf(a.Target), StatusNew
	a.Origin, a.Warnings, a.Ignored = nil, nil, nil
	b.Origin, b.Warnings, b.Ignored, b.Comments = nil, nil, nil, nil
	if s.Split == nil {
		a.ID = unit + "#1"
		a.Split = &SplitPart{Unit: unit}
	} else {
		part := *s.Split
		a.Split = &part
	}
	taken := map[string]bool{a.ID: true}
	for _, o := range segs {
		taken[o.ID] = true
	}
	for n := 2; ; n++ {
		if id := unit + "#" + strconv.Itoa(n); !taken[id] {
			b.ID = id
			break
		}
	}
	b.Split = &SplitPart{Unit: unit, Space: trail + lead}
	out := make([]Segment, 0, len(segs)+1)
	out = append(out, segs[:i]...)
	out = append(out, a, b)
	return append(out, segs[i+1:]...), nil
}

// unitsOf lists the units a segment that is no part stands for.
func unitsOf(s Segment) []JoinedUnit {
	if len(s.Joined) > 0 {
		return s.Joined
	}
	return []JoinedUnit{{ID: s.ID, Source: s.Source}}
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

// Join makes one segment of segment i and the one after it: two parts of
// a split segment, or two segments of the same paragraph. Their
// translations are put together. It returns the file's segments with the
// one in place of the two.
func Join(segs []Segment, i int) ([]Segment, error) {
	if i < 0 || i+1 >= len(segs) {
		return nil, refuse("There is no segment after this one to join it with.")
	}
	x, y := segs[i], segs[i+1]
	switch {
	case x.Locked || y.Locked:
		return nil, refuse("One of the two segments is locked.")
	case x.File != nil || y.File != nil:
		return nil, refuse("The file brings its own segments, which stay as they are.")
	}
	z := x
	z.Origin, z.Warnings, z.Ignored = nil, nil, nil
	z.Comments = append(append([]Comment{}, x.Comments...), y.Comments...)
	if len(z.Comments) == 0 {
		z.Comments = nil
	}
	z.Notes = mergeNotes(x.Notes, y.Notes)
	glue := func(a, b content.Content, space string) content.Content {
		switch {
		case blank(a):
			return b.Clone()
		case blank(b):
			return a.Clone()
		}
		out := a.Clone().Append(space)
		return append(out, b.Clone()...).Normalize()
	}
	switch {
	case x.Split != nil && y.Split != nil && x.Split.Unit == y.Split.Unit:
		a, b := heal(x.Source.Clone(), y.Source.Clone())
		z.Source = append(a.Append(y.Split.Space), b...).Normalize()
		z.Target = onePair(glue(x.Target, y.Target, y.Split.Space))
		others := 0
		for k, o := range segs {
			if k != i && k != i+1 && o.Split != nil && o.Split.Unit == x.Split.Unit {
				others++
			}
		}
		if others == 0 && x.Split.Space == "" {
			z.ID, z.Split = x.Split.Unit, nil
		}
	case x.Split != nil || y.Split != nil:
		return nil, refuse("A split segment is joined with its own parts first.")
	case x.Block == "" || x.Block != y.Block:
		return nil, refuse("The two segments are in different paragraphs, which the document keeps apart.")
	default:
		ux, uy := unitsOf(x), unitsOf(y)
		all := append(append([]JoinedUnit{}, ux...), uy...)
		src, ids := fold(all)
		// The second translation's tags take the IDs they have now.
		_, was := fold(uy)
		now := map[string]string{}
		for k := range uy {
			for own, id := range was[k] {
				now[id] = ids[len(ux)+k][own]
			}
		}
		moved := y.Target.Clone()
		for k := range moved {
			if moved[k].IsTag() {
				if id, ok := now[moved[k].Tag.ID]; ok {
					moved[k].Tag.ID = id
				}
			}
		}
		z.Source, z.Joined = src, all
		z.Target = glue(x.Target, moved, separator(x.Target, moved))
		if x.MaxLength > 0 && y.MaxLength > 0 {
			z.MaxLength = x.MaxLength + y.MaxLength
		} else {
			z.MaxLength = 0
		}
	}
	z.Status = statusOf(z.Target)
	out := make([]Segment, 0, len(segs)-1)
	out = append(out, segs[:i]...)
	out = append(out, z)
	return append(out, segs[i+2:]...), nil
}

// Separate undoes the joining of segment i: its units are segments of
// their own again. The translation stays with the first.
func Separate(segs []Segment, i int) ([]Segment, error) {
	if i < 0 || i >= len(segs) {
		return nil, errors.New("project: no such segment")
	}
	s := segs[i]
	switch {
	case s.Locked:
		return nil, refuse("The segment is locked.")
	case len(s.Joined) < 2:
		return nil, refuse("The segment was not joined.")
	}
	out := make([]Segment, 0, len(segs)+len(s.Joined)-1)
	out = append(out, segs[:i]...)
	for k, u := range s.Joined {
		p := Segment{ID: u.ID, Source: u.Source.Clone(), Target: content.Content{}, Status: StatusNew, Block: s.Block}
		if k == 0 {
			p.Target, p.Status = s.Target.Clone(), statusOf(s.Target)
			p.Comments, p.Notes = s.Comments, s.Notes
		}
		out = append(out, p)
	}
	return append(out, segs[i+1:]...), nil
}

// Unit is a unit of the file as its segments stand for it.
type Unit struct {
	ID string
	// Source is the unit's source put together from its segments: a
	// split unit's parts in a row, a joined segment's unit as it was.
	Source content.Content
	// Segments are the positions of the segments that stand for it: the
	// parts of a split unit, or the one segment, which a joined segment
	// shares with the units joined in it.
	Segments []int
}

// Units lists the units a file's segments stand for, in the file's order,
// so the segments can be checked against the file.
func Units(segs []Segment) []Unit {
	var out []Unit
	for i := 0; i < len(segs); i++ {
		s := segs[i]
		switch {
		case s.Split != nil:
			u := Unit{ID: s.Split.Unit, Source: s.Source.Clone(), Segments: []int{i}}
			for i+1 < len(segs) && segs[i+1].Split != nil && segs[i+1].Split.Unit == s.Split.Unit {
				i++
				a, b := heal(u.Source, segs[i].Source.Clone())
				u.Source = append(a.Append(segs[i].Split.Space), b...).Normalize()
				u.Segments = append(u.Segments, i)
			}
			out = append(out, u)
		case len(s.Joined) > 0:
			for _, j := range s.Joined {
				out = append(out, Unit{ID: j.ID, Source: j.Source, Segments: []int{i}})
			}
		default:
			out = append(out, Unit{ID: s.ID, Source: s.Source, Segments: []int{i}})
		}
	}
	return out
}

// Translations is what a document's filter takes for the segments: each
// unit's translation under its ID. A split unit's parts are put together
// in order, a part without a translation keeping its source; a joined
// segment's translation stands under its first unit, the others marked
// as joined to it. Segments without a translation are left out, so the
// document keeps their source.
func Translations(segs []Segment) filter.Translations {
	tr := filter.Translations{}
	for i := 0; i < len(segs); i++ {
		s := segs[i]
		switch {
		case s.Split != nil:
			var whole content.Content
			any := false
			for k := i; k < len(segs) && segs[k].Split != nil && segs[k].Split.Unit == s.Split.Unit; k++ {
				part := segs[k].Target
				if blank(part) {
					part = segs[k].Source
				} else {
					any = true
				}
				if k > i {
					whole = whole.Append(segs[k].Split.Space)
				}
				whole = append(whole, part.Clone()...)
				i = k
			}
			if any {
				tr[s.Split.Unit] = whole.Normalize()
			}
		case blank(s.Target):
		case len(s.Joined) > 0:
			tr[s.Joined[0].ID] = s.Target
			for _, j := range s.Joined[1:] {
				tr[j.ID] = filter.Joined()
			}
		default:
			tr[s.ID] = s.Target
		}
	}
	return tr
}
