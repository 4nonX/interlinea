package typeset

// Span is a piece of text in one style.
type Span struct {
	Text  string
	Style Style
	// Size in points; 0 is the paragraph's.
	Size float64
	// Color as red, green and blue from 0 to 1.
	Color [3]float64
	// Strike draws a line through the text.
	Strike bool
}

// Align is how lines sit between the margins.
type Align int

const (
	// AlignStart is left, or right in a right-to-left paragraph.
	AlignStart Align = iota
	AlignEnd
	AlignCenter
)

// Para is a paragraph.
type Para struct {
	Spans []Span
	// Size of text whose span says none, in points.
	Size float64
	// RTL sets the paragraph from right to left.
	RTL   bool
	Align Align
	// Indent moves every line in from the start edge; Hanging moves the
	// lines after the first further in.
	Indent, Hanging float64
	// SpaceBefore and SpaceAfter, in points.
	SpaceBefore, SpaceAfter float64
	// Leading is the line height as a multiple of the size (1.25 if 0).
	Leading float64
	// KeepWithNext keeps it in the same column as the next paragraph.
	KeepWithNext bool
	// Mark is what the running head of its page shows (a headword).
	Mark string
	// Bookmark makes an entry of the document's outline.
	Bookmark string
}

type line struct {
	items   []item // in the order drawn
	width   float64
	ascent  float64
	descent float64
	spans   []Span
	start   float64 // offset of the first item from the start edge
	rtl     bool
}

func (l *line) height(leading, size float64) float64 {
	h := size * leading
	if l.ascent+l.descent > h {
		h = l.ascent + l.descent
	}
	return h
}

// lines breaks a paragraph into lines no wider than width.
func (s *FontSet) lines(p *Para, width float64) []line {
	size := p.Size
	if size == 0 {
		size = 10
	}
	var items []item
	for i, sp := range p.Spans {
		sz := sp.Size
		if sz == 0 {
			sz = size
		}
		items = append(items, s.shapeSpan(sp.Text, sp.Style, sz, i)...)
	}
	var out []line
	first := true
	for len(items) > 0 {
		avail := width - p.Indent
		if !first {
			avail -= p.Hanging
		}
		w := 0.0
		cut := -1 // break after this item
		end := len(items)
		for i, it := range items {
			if it.newline {
				cut, end = i, i
				break
			}
			if w+it.width > avail && i > 0 && !it.space {
				end = i
				break
			}
			w += it.width
			if it.breakAfter {
				cut = i
			}
		}
		var take, rest []item
		switch {
		case end == len(items):
			take, rest = items, nil
		case end < len(items) && items[end].newline:
			take, rest = items[:end], items[end+1:]
		case cut >= 0:
			take, rest = items[:cut+1], items[cut+1:]
		default:
			take, rest = items[:end], items[end:]
		}
		// Spaces at the end of a line or the start of the next are not set.
		for len(take) > 0 && take[len(take)-1].space {
			take = take[:len(take)-1]
		}
		for len(rest) > 0 && rest[0].space && !rest[0].newline {
			rest = rest[1:]
		}
		l := line{items: reorder(take, p.RTL), spans: p.Spans, rtl: p.RTL}
		l.start = p.Indent
		if !first {
			l.start += p.Hanging
		}
		for _, it := range l.items {
			l.width += it.width
			sz := size
			if sp := p.Spans[it.span]; sp.Size != 0 {
				sz = sp.Size
			}
			if it.face != nil {
				f := it.face.Font
				if a := float64(f.Ascent) * sz / float64(f.UnitsPerEm); a > l.ascent {
					l.ascent = a
				}
				if d := -float64(f.Descent) * sz / float64(f.UnitsPerEm); d > l.descent {
					l.descent = d
				}
			}
		}
		if l.ascent == 0 {
			l.ascent, l.descent = size*0.9, size*0.25
		}
		out = append(out, l)
		items = rest
		first = false
	}
	if len(out) == 0 {
		out = append(out, line{ascent: size * 0.9, descent: size * 0.25, spans: p.Spans})
	}
	return out
}
