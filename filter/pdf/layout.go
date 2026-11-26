package pdf

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Paragraph is a paragraph of a page's text.
type Paragraph struct {
	Text string
	// Size is the height of its font in points.
	Size         float64
	Bold, Italic bool
	// Page is the page it is on, from 1.
	Page int
}

// piece is text of one line with nothing but word spaces in it.
type piece struct {
	text         string
	x0, x1, y    float64
	size         float64
	bold, italic bool
	chars        int
	// cell: the line it is from was broken at the white between columns.
	cell bool
	// room is where the text to its right on the line starts, and wall
	// where the text to its left ends; 0 without any.
	room, wall float64
}

// layout turns the text shown on a page into paragraphs in reading order.
// Text written in another direction than most follows, each direction by
// itself.
func layout(chunks []chunk) []Paragraph {
	byAngle := map[int][]chunk{}
	for _, c := range chunks {
		byAngle[c.angle] = append(byAngle[c.angle], c)
	}
	var angles []int
	for a := range byAngle {
		angles = append(angles, a)
	}
	// The direction with the most text first; then by angle.
	weight := func(a int) int {
		n := 0
		for _, c := range byAngle[a] {
			n += len(c.text)
		}
		return n
	}
	sort.Slice(angles, func(i, j int) bool {
		wi, wj := weight(angles[i]), weight(angles[j])
		if wi != wj {
			return wi > wj
		}
		return angles[i] < angles[j]
	})
	var out []Paragraph
	for _, a := range angles {
		cs := byAngle[a]
		if a != 0 {
			sin, cos := math.Sincos(float64(a) * math.Pi / 180)
			turned := make([]chunk, len(cs))
			for i, c := range cs {
				turned[i] = c
				turned[i].x = c.x*cos + c.y*sin
				turned[i].y = -c.x*sin + c.y*cos
			}
			cs = turned
		}
		out = append(out, layoutUpright(cs)...)
	}
	return out
}

func layoutUpright(chunks []chunk) []Paragraph {
	lines := linesOf(chunks)
	pieces := split(lines)
	blocks := blocksOf(pieces)
	right := math.Inf(-1)
	for _, p := range pieces {
		right = math.Max(right, p.x1)
	}
	for _, b := range blocks {
		b.right = right
	}
	blocks = inOrder(blocks)
	var out []Paragraph
	for _, b := range blocks {
		out = append(out, b.paragraphs()...)
	}
	return out
}

// line is the chunks on one baseline, from the left.
type line struct {
	y, size float64
	chunks  []chunk
}

func linesOf(chunks []chunk) []line {
	sorted := append([]chunk(nil), chunks...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].y > sorted[j].y })
	var lines []line
	for _, c := range sorted {
		if strings.TrimSpace(c.text) == "" {
			continue
		}
		n := len(lines)
		if n > 0 && math.Abs(lines[n-1].y-c.y) <= 0.45*math.Max(lines[n-1].size, c.size) {
			l := &lines[n-1]
			if c.size > l.size {
				// The larger text says where the baseline is; smaller
				// text beside it is raised or lowered.
				l.y, l.size = c.y, c.size
			}
			l.chunks = append(l.chunks, c)
			continue
		}
		lines = append(lines, line{y: c.y, size: c.size, chunks: []chunk{c}})
	}
	for i := range lines {
		cs := lines[i].chunks
		sort.SliceStable(cs, func(a, b int) bool { return cs[a].x < cs[b].x })
	}
	return lines
}

// gapLine is a line as pieces with wide gaps between them.
type gapLine struct {
	y, size float64
	pieces  []piece
}

// split joins the chunks of each line into pieces, and breaks a line at a
// wide gap where the lines around it have a gap too: the white between
// columns, or between the cells of a table.
func split(lines []line) []piece {
	gl := make([]gapLine, len(lines))
	for i, l := range lines {
		g := gapLine{y: l.y, size: l.size}
		var cur *piece
		var end float64
		var last chunk
		for _, c := range l.chunks {
			if cur != nil && c.text == last.text && math.Abs(c.x-last.x) < 0.15*c.size {
				continue // the same text drawn twice, to look bold
			}
			gap := 0.0
			if cur != nil {
				gap = c.x - end
			}
			if cur == nil || gap > c.size {
				g.pieces = append(g.pieces, piece{x0: c.x, y: l.y, size: c.size, bold: c.bold, italic: c.italic})
				cur = &g.pieces[len(g.pieces)-1]
			} else if gap > 0.17*c.size || gap < -0.5*c.size {
				if !strings.HasSuffix(cur.text, " ") && !strings.HasPrefix(c.text, " ") {
					cur.text += " "
				}
			}
			cur.text += c.text
			cur.bold = cur.bold && c.bold
			cur.italic = cur.italic && c.italic
			cur.size = math.Max(cur.size, c.size)
			end = math.Max(end, c.x+c.w)
			if cur.x1 < end {
				cur.x1 = end
			}
			last = c
		}
		gl[i] = g
	}
	var out []piece
	for i, g := range gl {
		var cur *piece
		for j, p := range g.pieces {
			if j > 0 && (!breaksAt(gl, i, g.pieces[j-1].x1, p.x0) || (listMark.MatchString(cur.text) && p.x0-cur.x1 < 4*p.size)) {
				cur.text = strings.TrimRight(cur.text, " ") + " " + strings.TrimLeft(p.text, " ")
				cur.x1 = p.x1
				cur.bold = cur.bold && p.bold
				cur.italic = cur.italic && p.italic
				continue
			}
			out = append(out, p)
			cur = &out[len(out)-1]
			if j > 0 {
				cur.cell = true
				out[len(out)-2].cell = true
				out[len(out)-2].room = p.x0
				cur.wall = out[len(out)-2].x1
			}
		}
	}
	for i := range out {
		out[i].text = strings.Join(strings.Fields(out[i].text), " ")
		out[i].chars = utf8.RuneCountInString(out[i].text)
	}
	return out
}

// breaksAt reports whether the gap from x0 to x1 in line i parts two
// texts. A very wide gap does. Otherwise the lines around it decide: a
// strip of white that runs on through the lines above or below is the
// white between columns, while the wide spaces of a justified line have
// text over and under them.
func breaksAt(lines []gapLine, i int, x0, x1 float64) bool {
	size := lines[i].size
	with := 0
	// blocked: the line right above or below has text over the gap.
	blocked := false
	lo, hi := x0, x1
	look := func(j int) bool {
		a, b, how := stripIn(lines[j], lo, hi, size)
		if how < 0 {
			if math.Abs(lines[j].y-lines[i].y) <= 2.2*size {
				blocked = true
			}
			return false
		}
		if how > 0 {
			lo, hi = a, b
			with++
		}
		return true
	}
	for j := i - 1; j >= 0 && lines[j].y-lines[i].y <= 8*size && look(j); j-- {
	}
	for j := i + 1; j < len(lines) && lines[i].y-lines[j].y <= 8*size && look(j); j++ {
	}
	return with >= 2 || (x1-x0 >= 4*size && !blocked)
}

// stripIn looks at the strip from x0 to x1 in another line. With text on
// both sides of a part of the strip that is free, it returns that part and
// 1. With text across the strip, -1. A line that is all on one side of the
// strip says nothing: 0.
func stripIn(l gapLine, x0, x1, size float64) (float64, float64, int) {
	left, right := false, false
	lo, hi := x0, x1
	for _, p := range l.pieces {
		switch {
		case p.x1 <= lo+0.25*size:
			left = true
		case p.x0 >= hi-0.25*size:
			right = true
		default:
			// The piece reaches into the strip: what is free is the wider
			// side of it.
			if p.x0-lo >= hi-p.x1 {
				hi = p.x0
				right = true
			} else {
				lo = p.x1
				left = true
			}
		}
	}
	if hi-lo < 0.5*size {
		return 0, 0, -1
	}
	if left && right {
		return lo, hi, 1
	}
	return 0, 0, 0
}

// block is lines of text under each other.
type block struct {
	lines          []piece
	x0, x1, y0, y1 float64 // y0 the lowest baseline, y1 the top of the first line
	// room is where the column to the right of the block starts, and
	// wall where the column to its left ends; 0 with nothing there.
	room, wall float64
	// right is how far the text of the page reaches to the right.
	right float64
}

func blocksOf(pieces []piece) []*block {
	sort.SliceStable(pieces, func(i, j int) bool {
		if pieces[i].y != pieces[j].y {
			return pieces[i].y > pieces[j].y
		}
		return pieces[i].x0 < pieces[j].x0
	})
	var blocks []*block
	for _, p := range pieces {
		var best *block
		for _, b := range blocks {
			last := b.lines[len(b.lines)-1]
			drop := last.y - p.y
			big, small := math.Max(last.size, p.size), math.Min(last.size, p.size)
			if drop <= 0.3*small || drop > 2.6*big || small/big < 0.87 {
				continue
			}
			if math.Min(p.x1, b.x1)-math.Max(p.x0, b.x0) <= 0 {
				continue
			}
			// A line of a column does not go on a block wider than its
			// column, nor a wide line on a column.
			if (p.room > 0 && b.x1 > p.room) || (b.room > 0 && p.x1 > b.room) || (p.wall > 0 && b.x0 < p.wall) || (b.wall > 0 && p.x0 < b.wall) {
				continue
			}
			if last.bold != p.bold && (utf8.RuneCountInString(last.text) < 80 || drop > 1.6*big) {
				continue // a heading over its text
			}
			if best == nil || last.y < best.lines[len(best.lines)-1].y {
				best = b
			}
		}
		if best == nil {
			blocks = append(blocks, &block{lines: []piece{p}, x0: p.x0, x1: p.x1, y0: p.y, y1: p.y + p.size, room: p.room, wall: p.wall})
			continue
		}
		best.lines = append(best.lines, p)
		if p.room > 0 && (best.room == 0 || p.room < best.room) {
			best.room = p.room
		}
		best.wall = math.Max(best.wall, p.wall)
		best.x0, best.x1, best.y0 = math.Min(best.x0, p.x0), math.Max(best.x1, p.x1), p.y
	}
	return blocks
}

// inOrder puts blocks in the order they are read. The widest white that
// runs across all of them parts them: what is over it is read before what
// is under it, what is left of it before what is right of it; and so on
// within each part. Columns are read one after the other that way, and
// what stands under them all, last.
func inOrder(blocks []*block) []*block {
	if len(blocks) < 2 {
		return blocks
	}
	// The widest gap between the blocks' extents along one axis.
	cut := func(lo, hi func(*block) float64) (float64, float64) {
		sorted := append([]*block(nil), blocks...)
		sort.SliceStable(sorted, func(i, j int) bool { return lo(sorted[i]) < lo(sorted[j]) })
		reach := hi(sorted[0])
		best, at := 0.0, 0.0
		for _, b := range sorted[1:] {
			if gap := lo(b) - reach; gap > best {
				best, at = gap, reach+gap/2
			}
			reach = math.Max(reach, hi(b))
		}
		return best, at
	}
	gapY, atY := cut(func(b *block) float64 { return b.y0 }, func(b *block) float64 { return b.y1 })
	gapX, atX := cut(func(b *block) float64 { return b.x0 }, func(b *block) float64 { return b.x1 })
	var first, second []*block
	switch {
	case gapY > 0 && gapY >= gapX:
		for _, b := range blocks {
			if b.y0 > atY {
				first = append(first, b)
			} else {
				second = append(second, b)
			}
		}
	case gapX > 0:
		for _, b := range blocks {
			if b.x1 < atX {
				first = append(first, b)
			} else {
				second = append(second, b)
			}
		}
	}
	// A cut parts the blocks in two, each with fewer blocks than before.
	// Blocks of a broken file (extents upside down, or not numbers) can
	// all fall on one side of it; those are read as blocks that lie over
	// each other, or the parting would never end.
	if len(first) == 0 || len(second) == 0 {
		// Blocks that lie over each other: from the top, then from the left.
		sorted := append([]*block(nil), blocks...)
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i].y1 != sorted[j].y1 {
				return sorted[i].y1 > sorted[j].y1
			}
			return sorted[i].x0 < sorted[j].x0
		})
		return sorted
	}
	return append(inOrder(first), inOrder(second)...)
}

var (
	// listMark is a piece that is the mark of a list item and nothing else.
	listMark    = regexp.MustCompile(`^\s*([•◦▪■●○‣·∙*–—-]|\(?\d{1,3}[.)]|\(?[a-zA-Z][.)]|\(?[ivxIVX]{1,5}[.)])\s*$`)
	bulletStart = regexp.MustCompile(`^[•◦▪■●○‣·∙*–—-]\s`)
	numberStart = regexp.MustCompile(`^(\(?\d{1,3}[.)]|\(?[a-zA-Z][.)]|\(?[ivxIVX]{1,5}[.)]|§)\s`)
	sentenceEnd = regexp.MustCompile(`[.:;!?…]["'”’»)]*$`)
)

// paragraphs cuts a block where a paragraph ends: at more space than
// between its other lines, at a line that ends before the next one's
// first word would have fitted, at an indented first line, and at a list
// item.
func (b *block) paragraphs() []Paragraph {
	lines := b.lines
	leading := math.Inf(1)
	for i := 1; i < len(lines); i++ {
		if d := lines[i-1].y - lines[i].y; d > 0 && d < leading {
			leading = d
		}
	}
	var out []Paragraph
	var cur []piece
	flush := func() {
		if len(cur) == 0 {
			return
		}
		p := Paragraph{Text: joinLines(cur), Bold: true, Italic: true}
		for _, l := range cur {
			p.Size = math.Max(p.Size, l.size)
			p.Bold = p.Bold && l.bold
			p.Italic = p.Italic && l.italic
		}
		if p.Text != "" {
			out = append(out, p)
		}
		cur = nil
	}
	for i, l := range lines {
		if i > 0 && b.startsParagraph(i, leading) {
			flush()
		}
		cur = append(cur, l)
	}
	flush()
	return out
}

// What a block beside other blocks is.
const (
	column = iota // lines of text that fill a column
	loose         // short texts with white to their right
	cells         // the cells of a table's column, a line each
)

// columnKind says what a block that stands beside others is, and for a
// loose one how far its lines could have run. The cells of a table are a
// few words each. Lines of running text fill their column: most of them
// end near its right edge, or the next column starts right after it.
func (b *block) columnKind() (int, float64) {
	words, full := 0, 0
	em := b.lines[0].size
	for _, l := range b.lines {
		words = max(words, len(strings.Fields(l.text)))
		if l.x1 >= b.x0+0.85*(b.x1-b.x0) {
			full++
		}
	}
	if words <= 3 {
		return cells, 0
	}
	to := b.right
	if b.room > 0 {
		to = b.room - em
	}
	if to-b.x1 > 3.5*em && !(len(b.lines) >= 3 && full*5 >= len(b.lines)*3) {
		return loose, to
	}
	return column, 0
}

func (b *block) startsParagraph(i int, leading float64) bool {
	prev, l := b.lines[i-1], b.lines[i]
	em := l.size
	if prev.y-l.y > 1.35*leading {
		return true
	}
	// The rows of a table stand further apart than the lines of a text.
	if prev.cell && l.cell && prev.y-l.y > 1.55*em {
		return true
	}
	if bulletStart.MatchString(l.text) {
		return true
	}
	// Whether the line before ends early: the first word of this line
	// would have fitted after it.
	first := l.text
	if sp := strings.IndexByte(first, ' '); sp > 0 {
		first = first[:sp]
	}
	perChar := em * 0.5
	if l.chars > 0 && l.x1 > l.x0 {
		perChar = (l.x1 - l.x0) / float64(l.chars)
	}
	leftAligned := math.Abs(prev.x0-l.x0) < em
	// How far the line before could have run: to the block's right edge,
	// or to the white before the column to its right.
	edge := b.x1
	if prev.cell || l.cell {
		how, to := b.columnKind()
		if how == cells {
			return true
		}
		if how == loose {
			edge = to
		}
	}
	short := leftAligned && edge-prev.x1 > float64(utf8.RuneCountInString(first)+1)*perChar+0.5*em
	if short {
		return true
	}
	if numberStart.MatchString(l.text) && sentenceEnd.MatchString(prev.text) {
		return true
	}
	// A first line set in from the margin, the lines around it not.
	if l.x0-b.x0 > 0.8*em && l.x0-prev.x0 > 0.8*em && !bulletStart.MatchString(prev.text) && !numberStart.MatchString(prev.text) {
		if i+1 < len(b.lines) && l.x0-b.lines[i+1].x0 > 0.5*em {
			return true
		}
	}
	return false
}

// joinLines makes one text of a paragraph's lines: a word divided at the
// end of a line is whole again, and lines of a script written without
// spaces join without one.
func joinLines(lines []piece) string {
	var b strings.Builder
	for i, l := range lines {
		text := l.text
		if i == 0 {
			b.WriteString(text)
			continue
		}
		prev := b.String()
		last, _ := utf8.DecodeLastRuneInString(prev)
		next, _ := utf8.DecodeRuneInString(text)
		switch {
		case last == '­':
			b.Reset()
			b.WriteString(strings.TrimSuffix(prev, "­"))
		case (last == '-' || last == '‐') && unicode.IsLower(next) && endsInLetterHyphen(prev):
			b.Reset()
			b.WriteString(prev[:len(prev)-utf8.RuneLen(last)])
		case last == '-' || last == '‐' || last == '/':
			// A compound divided at its hyphen: no space after it.
		case spaceless(last) && spaceless(next):
		default:
			b.WriteByte(' ')
		}
		b.WriteString(text)
	}
	return strings.ReplaceAll(b.String(), "­", "")
}

func endsInLetterHyphen(s string) bool {
	r := []rune(s)
	return len(r) >= 2 && unicode.IsLetter(r[len(r)-2])
}

// spaceless reports whether a character is of a script written without
// spaces between words.
func spaceless(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) ||
		(r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}
