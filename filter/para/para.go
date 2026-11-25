// Package para turns the content of one paragraph into translatable units
// and puts their translations back. Filters build a paragraph's content,
// text and tags with the original markup in each tag's Native, and hand it
// to Split: tags at the paragraph's edges stay out of sight, the rest is
// cut into sentences, and whitespace between sentences is kept as it is.
// Build returns the paragraph with each unit replaced by its translation,
// with the tags of the translation matched to the paragraph's own.
package para

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/segment"
)

// Piece is a unit, or the whitespace and tags between units.
type Piece struct {
	// Unit is true for a unit and false for a gap.
	Unit bool
	// Content is the unit's source with tag IDs from 1, or the gap with the
	// paragraph's tag IDs.
	Content content.Content
	// IDs maps a unit's tag IDs to the paragraph's.
	IDs map[string]string
}

// Para is a paragraph cut into units.
type Para struct {
	// Prefix and Suffix hold tags at the paragraph's edges that translators
	// do not see: placeholders before and after the text, and a pair that
	// surrounds everything.
	Prefix, Suffix content.Content
	Pieces         []Piece
	// First is the index of the paragraph's first unit in the extraction
	// it was added to, and Units how many it has.
	First, Units int
}

// Split cuts a paragraph's content into units. It reports false when the
// paragraph has no text to translate.
func Split(seg *segment.Segmenter, c content.Content) (Para, bool) {
	var p Para
	c, p.Prefix, p.Suffix = StripEdges(c)
	if !HasText(c) {
		return Para{}, false
	}
	for _, part := range Cut(c, Breaks(seg, c)) {
		lead, mid, trail := TrimSpace(part)
		if !HasText(mid) {
			p.gap(part)
			continue
		}
		p.gap(lead)
		src, ids := Renumber(mid)
		p.Pieces = append(p.Pieces, Piece{Unit: true, Content: src, IDs: ids})
		p.Units++
		p.gap(trail)
	}
	return p, true
}

func (p *Para) gap(g content.Content) {
	if len(g) == 0 {
		return
	}
	if n := len(p.Pieces); n > 0 && !p.Pieces[n-1].Unit {
		p.Pieces[n-1].Content = append(p.Pieces[n-1].Content, g...).Normalize()
		return
	}
	p.Pieces = append(p.Pieces, Piece{Content: g.Clone()})
}

// Add appends the paragraph's units and its block to the extraction. The
// units are named after the block: "<block>/s1", "<block>/s2".
func (p *Para) Add(ex *filter.Extraction, block filter.Block) {
	p.First = len(ex.Units)
	n := 0
	for _, pc := range p.Pieces {
		if !pc.Unit {
			continue
		}
		n++
		ex.Units = append(ex.Units, filter.Unit{ID: fmt.Sprintf("%s/s%d", block.ID, n), Block: block.ID, Source: pc.Content})
	}
	for _, r := range p.Prefix {
		if r.Tag.Kind != content.TagClose {
			block.Lead = append(block.Lead, r.Tag.Type)
		}
	}
	for _, r := range p.Suffix {
		if r.Tag.Kind == content.TagStandalone {
			block.Trail = append(block.Trail, r.Tag.Type)
		}
	}
	ex.Blocks = append(ex.Blocks, block)
}

// Translated reports whether any unit of the paragraph has a translation.
func (p *Para) Translated(ex *filter.Extraction, tr filter.Translations) bool {
	for i := 0; i < p.Units; i++ {
		if t, ok := tr[ex.Units[p.First+i].ID]; ok && len(t) > 0 {
			return true
		}
	}
	return false
}

// Build returns the whole paragraph with each unit replaced by its
// translation; units without one keep their source. Every tag carries the
// paragraph's markup in Native. A tag the source does not have is left
// out, a pair the translation opens and does not close is closed at the
// end of its unit, and pairs that overlap are closed and opened again so
// that the result nests.
func (p *Para) Build(ex *filter.Extraction, tr filter.Translations) content.Content {
	out := append(content.Content{}, p.Prefix...)
	n := 0
	for i := 0; i < len(p.Pieces); i++ {
		pc := p.Pieces[i]
		if !pc.Unit {
			out = append(out, pc.Content...)
			continue
		}
		target, ok := tr[ex.Units[p.First+n].ID]
		n++
		if !ok || len(target) == 0 || filter.IsJoined(target) {
			out = append(out, nest(pc.Content, pc)...)
			continue
		}
		// The units joined to this one are translated with it: their
		// tags count on from its own, and the tags that stood between
		// them follow the translation.
		var between content.Content
		for {
			k := i + 1
			var gap content.Content
			for k < len(p.Pieces) && !p.Pieces[k].Unit {
				gap = append(gap, p.Pieces[k].Content...)
				k++
			}
			if k >= len(p.Pieces) || !filter.IsJoined(tr[ex.Units[p.First+n].ID]) {
				break
			}
			pc = joinPieces(pc, p.Pieces[k])
			for _, r := range gap {
				if r.IsTag() {
					between = between.AppendTag(*r.Tag)
				}
			}
			i, n = k, n+1
		}
		out = append(out, nest(target, pc)...)
		out = append(out, between...)
	}
	return append(out, p.Suffix...)
}

// joinPieces is the unit a and the unit b joined to it, as one.
func joinPieces(a, b Piece) Piece {
	more, back := filter.JoinTags(a.Content, b.Content)
	out := Piece{Unit: true, Content: append(a.Content.Clone(), more...), IDs: map[string]string{}}
	for id, para := range a.IDs {
		out.IDs[id] = para
	}
	for id, own := range back {
		out.IDs[id] = b.IDs[own]
	}
	return out
}

// nest maps a translation's tags to the unit's own and makes pairs nest.
func nest(target content.Content, pc Piece) content.Content {
	find := func(kind content.TagKind, id string) (content.Tag, bool) {
		for _, r := range pc.Content {
			if r.IsTag() && r.Tag.Kind == kind && r.Tag.ID == id {
				t := *r.Tag
				t.ID = pc.IDs[id]
				return t, true
			}
		}
		return content.Tag{}, false
	}
	var out content.Content
	var open []content.Tag // the pairs open now, outermost first
	closeTag := func(t content.Tag) {
		if c, ok := find(content.TagClose, unitID(pc, t.ID)); ok {
			out = out.AppendTag(c)
		}
	}
	for _, r := range target {
		if !r.IsTag() {
			out = out.Append(r.Text)
			continue
		}
		t, ok := find(r.Tag.Kind, r.Tag.ID)
		if !ok {
			continue
		}
		switch t.Kind {
		case content.TagStandalone:
			out = out.AppendTag(t)
		case content.TagOpen:
			if _, has := find(content.TagClose, r.Tag.ID); !has {
				continue
			}
			open = append(open, t)
			out = out.AppendTag(t)
		case content.TagClose:
			at := -1
			for i := len(open) - 1; i >= 0; i-- {
				if open[i].ID == t.ID {
					at = i
					break
				}
			}
			if at < 0 {
				continue // closed without having been opened
			}
			inner := append([]content.Tag{}, open[at+1:]...)
			for i := len(inner) - 1; i >= 0; i-- {
				closeTag(inner[i])
			}
			out = out.AppendTag(t)
			open = open[:at]
			for _, o := range inner {
				open = append(open, o)
				out = out.AppendTag(o)
			}
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		closeTag(open[i])
	}
	return out
}

// unitID is the unit's ID of a paragraph tag ID.
func unitID(pc Piece, paraID string) string {
	for u, p := range pc.IDs {
		if p == paraID {
			return u
		}
	}
	return ""
}

// StripEdges takes the tags at the edges of a paragraph off its content:
// placeholders before and after the text, and a pair around all of it.
func StripEdges(c content.Content) (body, prefix, suffix content.Content) {
	body = c
	prefix, suffix = content.Content{}, content.Content{}
	for changed := true; changed; {
		changed = false
		for len(body) > 0 && body[0].IsTag() && body[0].Tag.Kind == content.TagStandalone {
			prefix = append(prefix, body[0])
			body = body[1:]
			changed = true
		}
		for len(body) > 0 && body[len(body)-1].IsTag() && body[len(body)-1].Tag.Kind == content.TagStandalone {
			suffix = append(content.Content{body[len(body)-1]}, suffix...)
			body = body[:len(body)-1]
			changed = true
		}
		if len(body) >= 2 && body[0].IsTag() && body[0].Tag.Kind == content.TagOpen {
			last := body[len(body)-1]
			if last.IsTag() && last.Tag.Kind == content.TagClose && last.Tag.ID == body[0].Tag.ID {
				prefix = append(prefix, body[0])
				suffix = append(content.Content{last}, suffix...)
				body = body[1 : len(body)-1]
				changed = true
			}
		}
	}
	return body, prefix, suffix
}

// HasText reports whether the content has text other than whitespace.
func HasText(c content.Content) bool {
	for _, r := range c {
		if !r.IsTag() && strings.TrimFunc(r.Text, unicode.IsSpace) != "" {
			return true
		}
	}
	return false
}

// Breaks returns the sentence breaks of the content as offsets into its
// plain text. A break inside a pair of tags is not used, so tags never
// cross units.
func Breaks(seg *segment.Segmenter, c content.Content) []int {
	var ok []int
	for _, b := range seg.Breaks(c.Plain()) {
		depth, pos := 0, 0
		for _, r := range c {
			if !r.IsTag() {
				pos += len(r.Text)
				continue
			}
			switch r.Tag.Kind {
			case content.TagOpen:
				if pos < b {
					depth++
				}
			case content.TagClose:
				if pos <= b {
					depth--
				}
			}
		}
		if depth == 0 {
			ok = append(ok, b)
		}
	}
	return ok
}

// Cut splits content at plain-text offsets without losing anything. Close
// tags at a cut stay with the left part; open and standalone tags go right.
func Cut(c content.Content, at []int) []content.Content {
	var out []content.Content
	cur := content.Content{}
	pos, k := 0, 0
	flush := func() {
		out = append(out, cur)
		cur = content.Content{}
		k++
	}
	for _, r := range c {
		if r.IsTag() {
			if k < len(at) && pos == at[k] && r.Tag.Kind != content.TagClose {
				flush()
			}
			cur = cur.AppendTag(*r.Tag)
			continue
		}
		s := r.Text
		for s != "" {
			if k < len(at) && pos == at[k] {
				flush()
				continue
			}
			n := len(s)
			if k < len(at) && at[k]-pos < n {
				n = at[k] - pos
			}
			cur = cur.Append(s[:n])
			s = s[n:]
			pos += n
		}
	}
	for k < len(at) && pos >= at[k] {
		flush()
	}
	return append(out, cur)
}

// TrimSpace moves whitespace at the start and end of a part into gaps.
func TrimSpace(c content.Content) (lead, mid, trail content.Content) {
	mid = c.Clone()
	lead, trail = content.Content{}, content.Content{}
	if len(mid) > 0 && !mid[0].IsTag() {
		t := mid[0].Text
		s := strings.TrimLeftFunc(t, unicode.IsSpace)
		if s != t {
			lead = content.Text(t[:len(t)-len(s)])
			if s == "" {
				mid = mid[1:]
			} else {
				mid[0].Text = s
			}
		}
	}
	if n := len(mid); n > 0 && !mid[n-1].IsTag() {
		t := mid[n-1].Text
		s := strings.TrimRightFunc(t, unicode.IsSpace)
		if s != t {
			trail = content.Text(t[len(s):])
			if s == "" {
				mid = mid[:n-1]
			} else {
				mid[n-1].Text = s
			}
		}
	}
	return lead, mid, trail
}

// Renumber gives the tags of a unit IDs from 1 in order of appearance and
// returns the map from the new IDs to the paragraph's.
func Renumber(c content.Content) (content.Content, map[string]string) {
	ids := map[string]string{}
	back := map[string]string{}
	out := c.Clone()
	for i := range out {
		if !out[i].IsTag() {
			continue
		}
		t := out[i].Tag
		id, ok := ids[t.ID]
		if !ok {
			id = strconv.Itoa(len(ids) + 1)
			ids[t.ID] = id
			back[id] = t.ID
		}
		t.ID = id
	}
	return out, back
}
