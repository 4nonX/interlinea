package typeset

import (
	"strconv"
)

// Document is pages of text, laid out in columns as it is added.
type Document struct {
	Fonts *FontSet
	// Page size and margins, in points.
	Width, Height                         float64
	MarginTop, MarginBottom, MarginInside float64
	MarginOutside                         float64
	// Columns of the text, and the gap between them.
	Columns int
	Gap     float64
	// Title goes into the running head and the document's properties.
	Title string
	// Head makes the running head of a page from its first and last mark
	// (the headwords on it); "" leaves the head with the title only.
	Head func(first, last string) string
	// RTL lays the running head out for a right-to-left document.
	RTL bool
	// Language of the document, for its properties.
	Lang string
	// ColumnRule draws a thin line between the columns.
	ColumnRule bool

	pages    []*page
	col      int
	y        float64 // from the top of the text area, in the column
	top      float64 // where the columns of this page start (below a banner)
	outline  []outlineEntry
	pendKeep []*Para
}

type outlineEntry struct {
	title string
	page  int
	y     float64
}

type textRun struct {
	face   *Face
	size   float64
	color  [3]float64
	x, y   float64 // baseline start, from the page's bottom left
	glyphs []uint16
}

type rule struct {
	x1, y1, x2, y2, width float64
	color                 [3]float64
}

type rect struct {
	x, y, w, h float64
	color      [3]float64
}

type page struct {
	top   float64 // where its columns start
	runs  []textRun
	rules []rule
	rects []rect
	marks []string
}

// A4 and Letter page sizes, in points.
const (
	A4Width, A4Height         = 595.28, 841.89
	LetterWidth, LetterHeight = 612, 792
)

// NewDocument starts a document on pages of the size given.
func NewDocument(fonts *FontSet, width, height float64) *Document {
	return &Document{Fonts: fonts, Width: width, Height: height, MarginTop: 64, MarginBottom: 56, MarginInside: 56, MarginOutside: 56, Columns: 1, Gap: 18}
}

func (d *Document) textTop() float64    { return d.Height - d.MarginTop }
func (d *Document) textHeight() float64 { return d.Height - d.MarginTop - d.MarginBottom }

// ColumnWidth is how wide a column of text is.
func (d *Document) ColumnWidth() float64 {
	full := d.Width - d.MarginInside - d.MarginOutside
	n := d.Columns
	if n < 1 {
		n = 1
	}
	return (full - float64(n-1)*d.Gap) / float64(n)
}

func (d *Document) colX(col int) float64 {
	return d.MarginInside + float64(col)*(d.ColumnWidth()+d.Gap)
}

func (d *Document) cur() *page {
	if len(d.pages) == 0 {
		d.pages = append(d.pages, &page{})
	}
	return d.pages[len(d.pages)-1]
}

// nextColumn moves on to the next column, or the next page.
func (d *Document) nextColumn() {
	d.cur()
	d.col++
	d.y = d.top
	if d.col >= max(1, d.Columns) {
		d.col = 0
		d.top, d.y = 0, 0
		d.pages = append(d.pages, &page{})
	}
}

// NewPage starts a new page.
func (d *Document) NewPage() {
	if len(d.pages) == 0 || d.y == d.top && d.col == 0 {
		d.cur()
		return
	}
	d.pages = append(d.pages, &page{})
	d.col, d.y, d.top = 0, 0, 0
}

// Banner sets paragraphs across the whole width of the page, above its
// columns (a title).
func (d *Document) Banner(ps ...*Para) {
	d.Flush()
	if d.col > 0 || d.y > d.top {
		d.NewPage()
	}
	cols := d.Columns
	d.Columns = 1
	for _, p := range ps {
		d.place(p, d.Fonts.lines(p, d.ColumnWidth()))
	}
	d.Columns = cols
	d.top = d.y
	d.cur().top = d.y
}

func (p *Para) leading() float64 {
	if p.Leading == 0 {
		return 1.25
	}
	return p.Leading
}

func (p *Para) size() float64 {
	if p.Size == 0 {
		return 10
	}
	return p.Size
}

// paraHeight is the height of a paragraph's lines (without its spaces).
func (d *Document) paraHeight(p *Para, ls []line) float64 {
	h := 0.0
	for i := range ls {
		h += ls[i].height(p.leading(), p.size())
	}
	return h
}

// Add places a paragraph in the column, moving on when it is full. A
// paragraph that must stay with the next waits for it.
func (d *Document) Add(p *Para) {
	if p.KeepWithNext {
		d.pendKeep = append(d.pendKeep, p)
		return
	}
	group := append(d.pendKeep, p)
	d.pendKeep = nil
	width := d.ColumnWidth()
	lines := make([][]line, len(group))
	need := 0.0
	for i, g := range group {
		lines[i] = d.Fonts.lines(g, width)
		if i < len(group)-1 {
			need += g.SpaceBefore + d.paraHeight(g, lines[i]) + g.SpaceAfter
		}
	}
	// The kept paragraphs and the first two lines of the last together.
	last := group[len(group)-1]
	firstLines := lines[len(group)-1]
	n := min(2, len(firstLines))
	need += last.SpaceBefore + d.paraHeight(last, firstLines[:n])
	if d.y > d.top && d.y+need > d.textHeight() {
		d.nextColumn()
	}
	for i, g := range group {
		d.place(g, lines[i])
	}
}

// Flush places paragraphs still waiting for the one after them.
func (d *Document) Flush() {
	pend := d.pendKeep
	d.pendKeep = nil
	for _, p := range pend {
		d.place(p, d.Fonts.lines(p, d.ColumnWidth()))
	}
}

func (d *Document) place(p *Para, ls []line) {
	if d.y > d.top {
		d.y += p.SpaceBefore
	}
	pg := d.cur()
	if p.Bookmark != "" {
		d.outline = append(d.outline, outlineEntry{p.Bookmark, len(d.pages) - 1, d.textTop() - d.y})
	}
	if p.Mark != "" {
		pg.marks = append(pg.marks, p.Mark)
	}
	x0 := d.colX(d.col)
	width := d.ColumnWidth()
	for i := range ls {
		l := &ls[i]
		h := l.height(p.leading(), p.size())
		if d.y > d.top && d.y+h > d.textHeight() {
			d.nextColumn()
			pg = d.cur()
			x0 = d.colX(d.col)
			if p.Mark != "" && len(pg.marks) == 0 {
				pg.marks = append(pg.marks, p.Mark)
			}
		}
		base := d.textTop() - d.y - (h-l.ascent-l.descent)/2 - l.ascent
		d.drawLine(pg, l, p, x0, width, base)
		d.y += h
	}
	d.y += p.SpaceAfter
}

func (d *Document) drawLine(pg *page, l *line, p *Para, x0, width, base float64) {
	var x float64
	align := p.Align
	switch {
	case align == AlignCenter:
		x = x0 + (width-l.width)/2
	case (align == AlignStart) == !l.rtl:
		x = x0 + l.start
	default:
		x = x0 + width - l.start - l.width
	}
	for _, it := range l.items {
		if it.face == nil || it.newline {
			x += it.width
			continue
		}
		sp := l.spans[it.span]
		size := sp.Size
		if size == 0 {
			size = p.size()
		}
		// Runs of one font, size and colour are drawn together.
		if n := len(pg.runs); n > 0 {
			r := &pg.runs[n-1]
			if r.face == it.face && r.size == size && r.color == sp.Color && r.y == base && r.x+runWidth(r) == x {
				r.glyphs = append(r.glyphs, it.glyphs...)
				if sp.Strike && !it.space {
					y := base + size*0.3
					pg.rules = append(pg.rules, rule{x, y, x + it.width, y, size * 0.06, sp.Color})
				}
				x += it.width
				continue
			}
		}
		pg.runs = append(pg.runs, textRun{face: it.face, size: size, color: sp.Color, x: x, y: base, glyphs: append([]uint16(nil), it.glyphs...)})
		if sp.Strike && !it.space {
			y := base + size*0.3
			pg.rules = append(pg.rules, rule{x, y, x + it.width, y, size * 0.06, sp.Color})
		}
		x += it.width
	}
}

func runWidth(r *textRun) float64 {
	u := 0
	for _, g := range r.glyphs {
		u += r.face.Font.Advance(g)
	}
	return float64(u) * r.size / float64(r.face.Font.UnitsPerEm)
}

// Rule draws a line across the column.
func (d *Document) Rule(width float64, color [3]float64, space float64) {
	if d.y+space*2 > d.textHeight() {
		d.nextColumn()
	}
	d.y += space
	y := d.textTop() - d.y
	x := d.colX(d.col)
	d.cur().rules = append(d.cur().rules, rule{x, y, x + d.ColumnWidth(), y, width, color})
	d.y += space
}

// Space leaves room in the column.
func (d *Document) Space(h float64) {
	if d.y > d.top {
		d.y += h
	}
}

// Cell is a table cell: paragraphs one under the other.
type Cell []*Para

// Table is rows of cells, with widths as parts of the column's width, and
// a header row repeated on every page it runs over.
type Table struct {
	Widths []float64
	Header []Cell
	Rows   [][]Cell
	// Padding inside each cell, in points.
	Padding float64
	// Shade of the header row and of the lines between rows.
	HeaderFill, LineColor [3]float64
	// Marks for the running head, one a row.
	Marks []string
}

type cellLines struct {
	paras []*Para
	lines [][]line
}

// AddTable places a table, its rows split over pages where they must be.
func (d *Document) AddTable(t *Table) {
	pend := d.pendKeep
	d.pendKeep = nil
	width := d.ColumnWidth()
	total := 0.0
	for _, w := range t.Widths {
		total += w
	}
	ws := make([]float64, len(t.Widths))
	for i, w := range t.Widths {
		ws[i] = width * w / total
	}
	pad := t.Padding
	prepare := func(row []Cell) []cellLines {
		out := make([]cellLines, len(ws))
		for c := range ws {
			if c >= len(row) {
				continue
			}
			for _, p := range row[c] {
				out[c].paras = append(out[c].paras, p)
				out[c].lines = append(out[c].lines, d.Fonts.lines(p, ws[c]-2*pad))
			}
		}
		return out
	}
	var header []cellLines
	if t.Header != nil {
		header = prepare(t.Header)
	}
	cellHeight := func(cl cellLines) float64 {
		h := 0.0
		for i, p := range cl.paras {
			h += d.paraHeight(p, cl.lines[i])
			if i > 0 {
				h += p.SpaceBefore
			}
		}
		return h
	}
	rowHeight := func(cells []cellLines) float64 {
		h := 0.0
		for _, c := range cells {
			if v := cellHeight(c); v > h {
				h = v
			}
		}
		return h + 2*pad
	}
	drawRow := func(cells []cellLines, fill bool) {
		pg := d.cur()
		h := rowHeight(cells)
		x := d.colX(d.col)
		top := d.textTop() - d.y
		if fill {
			pg.rects = append(pg.rects, rect{x, top - h, width, h, t.HeaderFill})
		}
		cx := x
		for c, cl := range cells {
			y := pad
			for i, p := range cl.paras {
				if i > 0 {
					y += p.SpaceBefore
				}
				for k := range cl.lines[i] {
					l := &cl.lines[i][k]
					lh := l.height(p.leading(), p.size())
					base := top - y - (lh-l.ascent-l.descent)/2 - l.ascent
					d.drawLine(pg, l, p, cx+pad, ws[c]-2*pad, base)
					y += lh
				}
			}
			cx += ws[c]
		}
		d.y += h
		yb := d.textTop() - d.y
		pg.rules = append(pg.rules, rule{x, yb, x + width, yb, 0.4, t.LineColor})
	}
	startPage := func() {
		if header != nil {
			drawRow(header, true)
		}
	}
	// What waits for the table (a heading), its header and its first row
	// go together.
	need := 0.0
	var pendLines [][]line
	for _, p := range pend {
		ls := d.Fonts.lines(p, width)
		pendLines = append(pendLines, ls)
		need += p.SpaceBefore + d.paraHeight(p, ls) + p.SpaceAfter
	}
	if header != nil {
		need += rowHeight(header)
	}
	if len(t.Rows) > 0 {
		need += rowHeight(prepare(t.Rows[0]))
	}
	if d.y > d.top && d.y+need > d.textHeight() {
		d.nextColumn()
	}
	for i, p := range pend {
		d.place(p, pendLines[i])
	}
	startPage()
	for ri, row := range t.Rows {
		cells := prepare(row)
		for {
			h := rowHeight(cells)
			room := d.textHeight() - d.y
			if h <= room {
				if ri < len(t.Marks) && t.Marks[ri] != "" {
					d.cur().marks = append(d.cur().marks, t.Marks[ri])
				}
				drawRow(cells, false)
				break
			}
			// Too tall for what is left: split it if a line fits, else go on.
			first, rest, ok := splitRow(d, cells, room-2*pad)
			if !ok {
				if d.y <= d.top+rowHeight(header)+0.01 || header == nil && d.y == d.top {
					// Not one line fits on a page of its own: set it anyway.
					drawRow(cells, false)
					break
				}
				d.nextColumn()
				startPage()
				continue
			}
			if ri < len(t.Marks) && t.Marks[ri] != "" {
				d.cur().marks = append(d.cur().marks, t.Marks[ri])
			}
			drawRow(first, false)
			d.nextColumn()
			startPage()
			cells = rest
		}
	}
}

// splitRow takes the lines of each cell that fit in h; ok is false when
// none does.
func splitRow(d *Document, cells []cellLines, h float64) (first, rest []cellLines, ok bool) {
	first = make([]cellLines, len(cells))
	rest = make([]cellLines, len(cells))
	for c, cl := range cells {
		y := 0.0
		done := false
		for i, p := range cl.paras {
			if done {
				rest[c].paras = append(rest[c].paras, p)
				rest[c].lines = append(rest[c].lines, cl.lines[i])
				continue
			}
			if i > 0 {
				y += p.SpaceBefore
			}
			k := 0
			for k < len(cl.lines[i]) {
				lh := cl.lines[i][k].height(p.leading(), p.size())
				if y+lh > h {
					break
				}
				y += lh
				k++
			}
			if k > 0 {
				first[c].paras = append(first[c].paras, p)
				first[c].lines = append(first[c].lines, cl.lines[i][:k])
				ok = true
			}
			if k < len(cl.lines[i]) {
				rest[c].paras = append(rest[c].paras, p)
				rest[c].lines = append(rest[c].lines, cl.lines[i][k:])
				done = true
			}
		}
	}
	return first, rest, ok
}

// Pages is how many pages the document has.
func (d *Document) Pages() int { return len(d.pages) }

// finish adds each page's running head and number.
func (d *Document) finish() {
	d.Flush()
	grey := [3]float64{0.4, 0.4, 0.4}
	for i, pg := range d.pages {
		var first, last string
		if len(pg.marks) > 0 {
			first, last = pg.marks[0], pg.marks[len(pg.marks)-1]
		}
		head := d.Title
		right := ""
		if d.Head != nil && first != "" {
			right = d.Head(first, last)
		}
		w := d.Width - d.MarginInside - d.MarginOutside
		y := d.Height - d.MarginTop + 22
		left, end := head, right
		if d.RTL {
			left, end = right, head
		}
		if left != "" {
			d.headLine(pg, left, d.MarginInside, w, y, AlignStart, grey, d.RTL)
		}
		if end != "" {
			d.headLine(pg, end, d.MarginInside, w, y, AlignEnd, grey, d.RTL)
		}
		pg.rules = append(pg.rules, rule{d.MarginInside, y - 6, d.Width - d.MarginOutside, y - 6, 0.5, [3]float64{0.75, 0.75, 0.75}})
		if d.ColumnRule && d.Columns > 1 {
			for c := 1; c < d.Columns; c++ {
				x := d.colX(c) - d.Gap/2
				pg.rules = append(pg.rules, rule{x, d.textTop() - pg.top, x, d.MarginBottom, 0.4, [3]float64{0.8, 0.8, 0.8}})
			}
		}
		d.headLine(pg, strconv.Itoa(i+1), d.MarginInside, w, d.MarginBottom-28, AlignCenter, grey, false)
	}
}

func (d *Document) headLine(pg *page, text string, x, w, base float64, align Align, color [3]float64, rtl bool) {
	p := &Para{Spans: []Span{{Text: text, Size: 8.5, Color: color}}, Size: 8.5, Align: align}
	if align != AlignCenter && rtl {
		// In a right-to-left document the edges swap.
		if align == AlignStart {
			p.Align = AlignEnd
		} else {
			p.Align = AlignStart
		}
	}
	ls := d.Fonts.lines(p, w)
	if len(ls) == 0 {
		return
	}
	d.drawLine(pg, &ls[0], p, x, w, base)
}
