// Package scc reads and writes Scenarist closed captions (.scc): CEA-608
// line 21 captions as hexadecimal byte pairs with their time codes, the
// form in which US broadcast and streaming captions are exchanged.
//
// Captions come in three modes, and each is read and written:
//
//   - Pop-on: a caption is loaded into the memory not shown and shown at
//     End of Caption, until the screen is erased or the next is shown.
//   - Paint-on: a caption is painted straight onto the screen, from its
//     first address to the next erase.
//   - Roll-up: lines are written on the bottom row of a window of two,
//     three or four rows and roll up at each carriage return; a line is
//     shown from its first character until it leaves the window.
//
// Each caption (each line of roll-up captions) is one unit. Rows are kept
// apart by tags; colour and italics codes in a row (mid-row codes) are
// tags too. Special and extended characters are read as the letters they
// stand for.
//
// On merge, a pop-on or paint-on caption is written again with its rows
// placed from the caption's bottom row up, aligned as the source's rows
// were (centred, left or right), and a row longer than the 32 columns a
// caption has wrapped at a space. A roll-up line keeps its codes
// (addresses, tab offsets, carriage returns) and gets the translated text
// in place of its own; a line longer than the row has columns from where it
// starts goes on after a carriage return of its own. The time codes move so
// each caption still appears when it did: a pop-on load starts earlier
// (taking in an erase sent while the caption before is still shown), and
// later lines move only where a longer text reaches into them. Letters
// CEA-608 has no code for are refused, naming them. Everything else in the
// file stays as it was.
package scc

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/i18n"
)

// Format is the name stored with a project's files.
const Format = "scc"

// Columns is how many characters a caption row holds.
const Columns = 32

const header = "Scenarist_SCC V1.0"

// word is a byte pair without its parity bits.
type word struct{ hi, lo byte }

func (w word) control() bool { return w.hi >= 0x10 && w.hi <= 0x1F }

// line is a line of the file: a time code and the pairs sent from it on,
// one a frame.
type line struct {
	text  string // as written, for a line left alone
	frame int64  // the frame its first pair is sent in
	drop  bool   // drop-frame time code (;)
	words []word
	upper bool // hexadecimal in capitals
}

type file struct {
	src   string
	lines []*line
	caps  []*caption
	// channel2 is set where the captions are on channel 2 (CC2).
	channel2 bool
}

// at is a pair's place: its line and its index in the line.
type at struct{ line, word int }

// cell is a character of a row, or a mid-row code (which takes a cell).
type cell struct {
	r    rune
	code word // mid-row code, where r is 0
	// ext marks a special or extended character, which an extended
	// character does not take the place of.
	ext bool
}

// row is a row of the screen as a decoder keeps it: 32 columns, written
// at a cursor that addresses and tab offsets move. A character overwrites
// what the column had; past the last column the last is overwritten.
type row struct {
	n    int  // 1 to 15
	attr byte // the attribute of its last address: 0-15, colour or italics with underline in bit 0; 16+ for an indent
	grid [Columns]cell
	used [Columns]bool
	cur  int
	// over is text sent past the last column. A decoder writes it over the
	// last column; it is kept, so the translator sees what the row says
	// (and the translation is laid out to fit).
	over  []cell
	atEnd bool
	// pend is an address back on the row that has text: whether it
	// overwrites or the text goes on is known once the row is written.
	pend *restate
}

// restate is an address of the row it is on, behind the cursor. Encoders
// send one as they please, to restate the row; taken as an overwrite, it
// would cover the text it restates. So the text goes on after the cursor
// as long as the row has room for it, and is written over the row from
// the address only where it would not fit (as pycaption reads them).
type restate struct {
	col        int
	grid       [Columns]cell
	used       [Columns]bool
	cur        int
	over       []cell
	ops        []op
	appended   int
	overflowed bool
}

// op is a write to a row: a cell, a backspace (back), a tab (tab > 0),
// or an address to a column (to > 0, the column plus one).
type op struct {
	c    cell
	back bool
	tab  int
	to   int
}

func (r *row) put(c cell) {
	if r.pend != nil {
		r.pend.ops = append(r.pend.ops, op{c: c})
		r.pend.appended++
		if r.pend.cur+r.pend.appended > Columns {
			r.pend.overflowed = true
		}
	}
	if r.atEnd {
		r.over = append(r.over, c)
		return
	}
	r.grid[r.cur], r.used[r.cur] = c, true
	if r.cur < Columns-1 {
		r.cur++
	} else {
		r.atEnd = true
	}
}

// back is a backspace: the column before the cursor is cleared.
func (r *row) back() {
	if r.pend != nil {
		r.pend.ops = append(r.pend.ops, op{back: true})
		r.pend.appended--
	}
	if len(r.over) > 0 {
		r.over = r.over[:len(r.over)-1]
		return
	}
	if r.atEnd {
		r.atEnd = false
	} else if r.cur > 0 {
		r.cur--
	}
	r.grid[r.cur], r.used[r.cur] = cell{}, false
}

// tab moves the cursor on (a tab offset).
func (r *row) tab(n int) {
	if r.pend != nil {
		r.pend.ops = append(r.pend.ops, op{tab: n})
		r.pend.appended += n
	}
	r.moveTo(r.cur + n)
}

// address places the cursor at a column of the row: an address back on
// a row with text is held until it is known whether it overwrites.
func (r *row) address(col int) {
	if p := r.pend; p != nil && !p.overflowed && col >= p.col {
		// An address on from the held one: the text goes on as far from
		// where it went on as the address is from the held one.
		if at := col + p.cur - p.col; at >= r.cur && at < Columns {
			p.ops = append(p.ops, op{to: col + 1})
			p.appended += at - r.cur
			r.moveTo(at)
			return
		}
	}
	r.settle()
	if col < r.cur && !r.empty() {
		r.pend = &restate{col: col, grid: r.grid, used: r.used, cur: r.cur, over: append([]cell(nil), r.over...)}
		return
	}
	r.moveTo(col)
}

// settle decides a held address: where the text written since it did not
// fit after the cursor, it is written over the row from the address.
func (r *row) settle() {
	p := r.pend
	if p == nil {
		return
	}
	r.pend = nil
	if !p.overflowed {
		return
	}
	r.grid, r.used, r.over = p.grid, p.used, p.over
	r.moveTo(p.col)
	for _, o := range p.ops {
		switch {
		case o.back:
			r.back()
		case o.tab > 0:
			r.moveTo(r.cur + o.tab)
		case o.to > 0:
			r.moveTo(o.to - 1)
		default:
			r.put(o.c)
		}
	}
}

// putExtended writes an extended character, which takes the place of the
// basic character before it (sent for decoders without extended ones).
func (r *row) putExtended(c cell) {
	if n := len(r.over); n > 0 {
		if pc := r.over[n-1]; pc.r != 0 && !pc.ext {
			r.over = r.over[:n-1]
		}
		c.ext = true
		r.over = append(r.over, c)
		return
	}
	if r.cur > 0 {
		prev := r.cur - 1
		if r.cur == Columns-1 && r.used[r.cur] {
			prev = r.cur
		}
		if pc := r.grid[prev]; r.used[prev] && pc.r != 0 && !pc.ext {
			r.back()
		}
	}
	c.ext = true
	r.put(c)
}

// clearFrom clears the row from the cursor on (Delete to End of Row).
func (r *row) clearFrom() {
	for i := r.cur; i < Columns; i++ {
		r.grid[i], r.used[i] = cell{}, false
	}
}

func (r *row) moveTo(col int) {
	// Text past the last column stays part of the row when the cursor moves.
	r.cur, r.atEnd = min(max(col, 0), Columns-1), false
}

// span is the row's text: its first column with a character, and the
// cells from there to the last, columns between them as spaces.
func (r *row) span() (start int, cells []cell) {
	r.settle()
	first, last := -1, -1
	for i := 0; i < Columns; i++ {
		if r.used[i] && !(r.grid[i].r == ' ' || r.grid[i].r == 0 && r.grid[i].code == (word{})) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 && len(r.over) == 0 {
		return 0, nil
	}
	if len(r.over) > 0 {
		// The text goes on past the last column: its spaces before count.
		if first < 0 {
			first = Columns - 1
		}
		last = Columns - 1
	}
	for i := first; i <= last; i++ {
		if r.used[i] {
			cells = append(cells, r.grid[i])
		} else {
			cells = append(cells, cell{r: ' '})
		}
	}
	cells = append(cells, r.over...)
	for len(cells) > 0 && cells[len(cells)-1].r == ' ' {
		cells = cells[:len(cells)-1]
	}
	return first, cells
}

func (r *row) empty() bool {
	_, cs := r.span()
	for _, c := range cs {
		if c.r != 0 && c.r != ' ' {
			return false
		}
	}
	return true
}

// The caption modes.
const (
	popOn = iota + 1
	paintOn
	rollUp
	textMode
)

type caption struct {
	id   string
	mode int
	rows []*row
	// show and hide are frames; hide -1 where it is not known.
	show, hide int64
	// Pop-on: load is the first pair of the load (Resume Caption Loading
	// or Erase Non-displayed Memory), eoc its End of Caption.
	load, eoc at
	// Pop-on and paint-on: the first and last pair of the text, with the
	// addresses of its rows.
	first, last at
	// Roll-up: the pairs of the line's text. They are replaced; the codes
	// between them (addresses, tab offsets) stay where they are.
	chars []at
	// Roll-up: how many carriage returns the line has seen; it leaves the
	// window when they reach the window's rows.
	rolled int
	// once: the source sends the codes in its text once, not twice; the
	// translation does too, so it takes no longer to send than it must.
	once bool
}

func (c *caption) hasText() bool {
	for _, r := range c.rows {
		if !r.empty() {
			return true
		}
	}
	return false
}

// The miscellaneous control codes (first byte 14h on channel 1).
const (
	rcl = 0x20
	bs  = 0x21
	der = 0x24
	ru2 = 0x25
	ru3 = 0x26
	ru4 = 0x27
	rdc = 0x29
	tr  = 0x2A
	rtd = 0x2B
	edm = 0x2C
	cr  = 0x2D
	enm = 0x2E
	eoc = 0x2F
)

func parity(b byte) byte {
	b &= 0x7F
	n := 0
	for x := b; x != 0; x >>= 1 {
		n += int(x & 1)
	}
	if n%2 == 0 {
		b |= 0x80
	}
	return b
}

func parse(data []byte) (*file, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("the file is not a Scenarist caption file: it is not text")
	}
	src := string(data)
	f := &file{src: src}
	lines := strings.Split(src, "\n")
	first := strings.TrimSpace(strings.TrimPrefix(lines[0], "\ufeff"))
	if first != header {
		return nil, fmt.Errorf("the file is not a Scenarist caption file: it does not start with %q", header)
	}
	for no, l := range lines[1:] {
		t := strings.TrimRight(l, "\r")
		if strings.TrimSpace(t) == "" {
			continue
		}
		tc, rest, ok := strings.Cut(strings.TrimSpace(t), "\t")
		if !ok {
			tc, rest, ok = strings.Cut(strings.TrimSpace(t), " ")
		}
		if !ok {
			return nil, fmt.Errorf("line %d has a time code and no captions", no+2)
		}
		frame, drop, err := parseTimecode(tc)
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", no+2, err)
		}
		if n := len(f.lines); n > 0 && frame < f.lines[n-1].frame {
			return nil, fmt.Errorf("line %d: the time code %s is before the one of the line above it", no+2, tc)
		}
		ln := &line{text: l, frame: frame, drop: drop}
		for _, h := range strings.Fields(rest) {
			if len(h) == 2 {
				// A pair cut short (the last character of a line): its
				// first byte.
				h += "80"
			}
			if len(h) != 4 {
				return nil, fmt.Errorf("line %d: %q is not a byte pair", no+2, h)
			}
			v, err := strconv.ParseUint(h, 16, 16)
			if err != nil {
				return nil, fmt.Errorf("line %d: %q is not a byte pair", no+2, h)
			}
			if strings.ToUpper(h) == h && strings.ToLower(h) != h {
				ln.upper = true
			}
			ln.words = append(ln.words, word{byte(v>>8) & 0x7F, byte(v) & 0x7F})
		}
		f.lines = append(f.lines, ln)
	}
	return f, f.read()
}

// parseTimecode reads HH:MM:SS:FF, or HH:MM:SS;FF in drop-frame time, as
// a frame count at 29.97 frames per second.
func parseTimecode(tc string) (int64, bool, error) {
	drop := strings.ContainsAny(tc, ";.")
	parts := strings.FieldsFunc(tc, func(r rune) bool { return r == ':' || r == ';' || r == '.' })
	if len(parts) != 4 {
		return 0, false, fmt.Errorf("%q is not a time code", tc)
	}
	var v [4]int64
	for i, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return 0, false, fmt.Errorf("%q is not a time code", tc)
		}
		v[i] = n
	}
	if v[1] > 59 || v[2] > 59 || v[3] > 29 {
		return 0, false, fmt.Errorf("%q is not a time code", tc)
	}
	frames := ((v[0]*60+v[1])*60+v[2])*30 + v[3]
	if drop {
		minutes := v[0]*60 + v[1]
		frames -= 2 * (minutes - minutes/10)
	}
	return frames, drop, nil
}

func timecode(frame int64, drop bool) string {
	if frame < 0 {
		frame = 0
	}
	if drop {
		// Frames 0 and 1 of each minute but every tenth are not numbered.
		d, m := frame/17982, frame%17982
		if m > 1 {
			frame += 18*d + 2*((m-2)/1798)
		} else {
			frame += 18 * d
		}
	}
	f := frame % 30
	s := frame / 30 % 60
	mi := frame / 1800 % 60
	h := frame / 108000
	sep := ":"
	if drop {
		sep = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d", h, mi, s, sep, f)
}

// ms is the time of a frame in milliseconds, at 29.97 frames per second.
func ms(frame int64) int64 { return frame * 1001 / 30 }

// read follows the codes through the file and finds its captions: those
// loaded and shown at once (pop-on), those painted straight onto the
// screen (paint-on), and the lines of roll-up captions.
func (f *file) read() error {
	mode, depth := 0, 0
	// Pop-on: the caption being loaded, in the memory not shown.
	var load *caption
	hidden := map[int]*row{}
	// Paint-on: the caption painted since the screen was last erased.
	var paint *caption
	shown := map[int]*row{}
	// Roll-up: the line being written and those in the window above it.
	var roll *caption
	var window []*caption
	base := 15 // the roll-up window's bottom row
	var r *row // the row written to
	// Captions on the screen, to be hidden when it is erased.
	var screen []*caption

	hideAll := func(frame int64) {
		for _, c := range screen {
			if c.hide < 0 {
				c.hide = frame
			}
		}
		screen = nil
		for _, c := range window {
			if c.hide < 0 {
				c.hide = frame
			}
		}
		window = nil
	}
	add := func(c *caption) {
		c.id = "c" + strconv.Itoa(len(f.caps)+1)
		f.caps = append(f.caps, c)
	}
	// endRoll ends the roll-up line being written.
	endRoll := func() {
		if roll != nil && roll.hasText() {
			window = append(window, roll)
		}
		roll, r = nil, nil
	}
	startRoll := func(frame int64) {
		roll = &caption{mode: rollUp, show: frame, hide: -1}
		r = &row{n: base}
		roll.rows = []*row{r}
	}
	// endPaint ends the paint-on caption: what follows is another.
	endPaint := func() {
		paint, shown, r = nil, map[int]*row{}, nil
	}
	rowsOf := func(m map[int]*row) []*row {
		var out []*row
		for n := 1; n <= 15; n++ {
			if rw := m[n]; rw != nil && !rw.empty() {
				out = append(out, rw)
			}
		}
		return out
	}

	var prev word
	var prevPos at
	prevControl, prevText := false, false
	chanSet := false
	for li, ln := range f.lines {
		for wi, w := range ln.words {
			pos := at{li, wi}
			frame := ln.frame + int64(wi)
			// text marks a pair as part of the caption's text.
			text := func() {
				switch mode {
				case popOn:
					if load != nil {
						if load.first.line < 0 {
							load.first = pos
						}
						load.last = pos
					}
				case paintOn:
					if paint == nil {
						paint = &caption{mode: paintOn, first: pos, show: frame, hide: -1, load: at{-1, -1}, eoc: at{-1, -1}}
						add(paint)
						screen = append(screen, paint)
					}
					paint.last = pos
				case rollUp:
					if roll == nil {
						startRoll(frame)
					}
					if len(roll.chars) == 0 {
						roll.show = frame
						add(roll)
					}
					roll.chars = append(roll.chars, pos)
				}
			}
			if w.control() {
				ch2 := w.hi >= 0x18
				if !chanSet {
					f.channel2, chanSet = ch2, true
				}
				if ch2 != f.channel2 {
					prevControl = false
					continue
				}
				// A control code is sent twice; the second is not acted on,
				// but it belongs where the first does.
				if prevControl && w == prev {
					prevControl = false
					if prevText {
						switch {
						case mode == rollUp && roll != nil && len(roll.chars) > 0 && roll.chars[len(roll.chars)-1] == prevPos:
							roll.chars = append(roll.chars, pos)
						case mode == popOn && load != nil && load.last == prevPos:
							load.last = pos
						case mode == paintOn && paint != nil && paint.last == prevPos:
							paint.last = pos
						}
					}
					continue
				}
				prev, prevControl, prevPos, prevText = w, true, pos, false
				w.hi &^= 0x08
				switch {
				case w.hi == 0x15 && w.lo >= 0x20 && w.lo <= 0x2F:
					// Field 2 (CC3, CC4): not these captions.
				case w.hi == 0x14 && w.lo >= 0x20 && w.lo <= 0x2F:
					switch w.lo {
					case rcl:
						if mode == rollUp {
							endRoll()
						}
						mode = popOn
						if load == nil {
							load = &caption{mode: popOn, load: pos, first: at{-1, -1}, eoc: at{-1, -1}, hide: -1}
						}
						r = nil
					case enm:
						if mode == popOn && load == nil {
							load = &caption{mode: popOn, load: pos, first: at{-1, -1}, eoc: at{-1, -1}, hide: -1}
						}
						hidden = map[int]*row{}
						if mode == popOn {
							r = nil
						}
					case ru2, ru3, ru4:
						if mode != rollUp {
							// Roll-up from another mode starts on a clear screen.
							hideAll(frame)
							endPaint()
							load, hidden = nil, map[int]*row{}
						}
						mode, depth = rollUp, int(w.lo-ru2)+2
						// A smaller window drops the rows above it.
						for len(window) >= depth {
							if window[0].hide < 0 {
								window[0].hide = frame
							}
							window = window[1:]
						}
					case rdc:
						if mode == rollUp {
							endRoll()
						}
						if mode != paintOn {
							// The screen's memory has a cursor of its own.
							r = nil
						}
						mode = paintOn
					case tr, rtd:
						if mode == rollUp {
							endRoll()
						}
						mode = textMode
					case edm:
						if mode == rollUp {
							if roll != nil && roll.hasText() {
								roll.hide = frame
							}
							roll, r = nil, nil
						}
						hideAll(frame)
						if mode == paintOn {
							endPaint()
						}
					case eoc:
						if mode != popOn && load == nil {
							// End of Caption also switches to pop-on, showing
							// what the memory not shown has.
							mode = popOn
						}
						hideAll(frame)
						endPaint()
						if load != nil {
							load.eoc = pos
							load.show = frame
							load.rows = rowsOf(hidden)
							if load.first.line >= 0 && len(load.rows) > 0 {
								add(load)
								screen = append(screen, load)
							}
						}
						load, hidden, r = nil, map[int]*row{}, nil
					case cr:
						if mode == rollUp {
							// The window rolls up a row: the line written goes
							// up, the top one out of the window.
							endRoll()
							keep := window[:0]
							for _, c := range window {
								c.rolled++
								if c.rolled >= depth {
									if c.hide < 0 {
										c.hide = frame
									}
									continue
								}
								keep = append(keep, c)
							}
							window = keep
						}
					case bs, der:
						if r != nil && w.lo == bs {
							r.back()
						}
						if r != nil && w.lo == der {
							r.clearFrom()
						}
						// An edit of a caption's text is part of it: the
						// translation is written as it is to be shown.
						if mode == rollUp && roll != nil || mode == popOn && load != nil && load.first.line >= 0 || mode == paintOn && paint != nil {
							text()
							prevText = true
						}
					}
				case mode == textMode || mode == 0:
				case w.lo >= 0x40 && w.hi >= 0x10 && w.hi <= 0x17:
					// A preamble address code: a row, and where in it.
					n := pacRow(w)
					if n == 0 {
						continue
					}
					a := (w.lo - 0x40) & 0x1F
					col := 0
					if a >= 0x10 {
						col = int(a-0x10) / 2 * 4
					}
					switch mode {
					case popOn, paintOn:
						m := hidden
						if mode == paintOn {
							m = shown
						}
						if mode == popOn && load == nil {
							continue
						}
						r = m[n]
						if r == nil {
							r = &row{n: n}
							m[n] = r
						}
						r.attr = a
						r.address(col)
						text()
						prevText = true
						if mode == paintOn {
							paint.rows = rowsOf(shown)
							if len(paint.rows) == 0 {
								// The row has no text yet: it is kept to be
								// listed once it has.
								paint.rows = nil
							}
						}
					case rollUp:
						// The base row; the line goes on where it says.
						if roll == nil {
							startRoll(frame)
						}
						base = n
						roll.rows[0].n = n
						roll.rows[0].attr = a
						roll.rows[0].address(col)
						r = roll.rows[0]
					}
				case w.hi == 0x17 && w.lo >= 0x21 && w.lo <= 0x23:
					// Tab offset: on in the row.
					if r != nil {
						r.tab(int(w.lo - 0x20))
					}
					if mode != rollUp {
						text()
						prevText = true
					}
				case w.hi == 0x11 && w.lo >= 0x20 && w.lo <= 0x2F:
					// A mid-row code: colour or italics from here on.
					f.cellAt(&r, mode, roll, &paint, shown, load, hidden, startRoll, frame)
					if r != nil {
						r.put(cell{code: word{0x11, w.lo}})
					}
					text()
					prevText = true
				case w.hi == 0x11 && w.lo >= 0x30 && w.lo <= 0x3F:
					f.cellAt(&r, mode, roll, &paint, shown, load, hidden, startRoll, frame)
					if r != nil {
						r.put(cell{r: special[w.lo-0x30], ext: true})
					}
					text()
					prevText = true
				case (w.hi == 0x12 || w.hi == 0x13) && w.lo >= 0x20 && w.lo <= 0x3F:
					// An extended character takes the place of the one before.
					f.cellAt(&r, mode, roll, &paint, shown, load, hidden, startRoll, frame)
					if r != nil {
						r.putExtended(cell{r: extended[w.hi-0x12][w.lo-0x20]})
					}
					text()
					prevText = true
				case w.hi == 0x10 && w.lo >= 0x20 && w.lo <= 0x2F, w.hi == 0x17 && w.lo >= 0x2D && w.lo <= 0x2F:
					// Background colours, and black text: with the text.
					if mode != rollUp {
						text()
						prevText = true
					}
				}
				if mode == paintOn && paint != nil {
					paint.rows = rowsOf(shown)
				}
				continue
			}
			prevControl = false
			if mode == 0 || mode == textMode || (w.hi < 0x20 && w.hi != 0) {
				continue
			}
			if w.hi < 0x20 && w.lo < 0x20 {
				// Padding.
				continue
			}
			f.cellAt(&r, mode, roll, &paint, shown, load, hidden, startRoll, frame)
			if r == nil {
				continue
			}
			for _, b := range []byte{w.hi, w.lo} {
				if b >= 0x20 {
					r.put(cell{r: basic(b)})
				}
			}
			text()
			if mode == paintOn && paint != nil {
				paint.rows = rowsOf(shown)
			}
		}
	}
	endRoll()
	for _, c := range f.caps {
		c.once = f.sentOnce(c)
	}
	// Captions with no text (a roll-up line of spaces) are not captions.
	kept := f.caps[:0]
	for _, c := range f.caps {
		if c.hasText() {
			c.id = "c" + strconv.Itoa(len(kept)+1)
			kept = append(kept, c)
		}
	}
	f.caps = kept
	return nil
}

// sentOnce reports whether the codes of a caption's text (addresses, tab
// offsets, mid-row codes, special characters) are sent once each.
func (f *file) sentOnce(c *caption) bool {
	var ps []at
	switch c.mode {
	case rollUp:
		ps = c.chars
	default:
		for p := c.first; p.line >= 0; p = f.next(p) {
			ps = append(ps, p)
			if p == c.last {
				break
			}
		}
	}
	codes, doubled := 0, 0
	for i, p := range ps {
		w := f.word(p)
		if !w.control() {
			continue
		}
		if i > 0 && f.word(ps[i-1]) == w {
			doubled++
			continue
		}
		codes++
	}
	return codes > 0 && doubled == 0
}

// cellAt makes sure there is a row to write a character in: text written
// without an address goes to the bottom row (pop-on, paint-on) or the
// roll-up line.
func (f *file) cellAt(r **row, mode int, roll *caption, paint **caption, shown map[int]*row, load *caption, hidden map[int]*row, startRoll func(int64), frame int64) {
	if *r != nil {
		return
	}
	switch mode {
	case popOn:
		if load == nil {
			return
		}
		rw := hidden[15]
		if rw == nil {
			rw = &row{n: 15}
			hidden[15] = rw
		}
		*r = rw
	case paintOn:
		rw := shown[15]
		if rw == nil {
			rw = &row{n: 15}
			shown[15] = rw
		}
		*r = rw
	case rollUp:
		if roll == nil {
			startRoll(frame)
			return
		}
		*r = roll.rows[0]
	}
}

// pacRow is the row a preamble address code places text in; 0 if it is
// none.
func pacRow(w word) int {
	high := w.lo >= 0x60
	switch w.hi {
	case 0x11:
		return map[bool]int{false: 1, true: 2}[high]
	case 0x12:
		return map[bool]int{false: 3, true: 4}[high]
	case 0x15:
		return map[bool]int{false: 5, true: 6}[high]
	case 0x16:
		return map[bool]int{false: 7, true: 8}[high]
	case 0x17:
		return map[bool]int{false: 9, true: 10}[high]
	case 0x10:
		if !high {
			return 11
		}
	case 0x13:
		return map[bool]int{false: 12, true: 13}[high]
	case 0x14:
		return map[bool]int{false: 14, true: 15}[high]
	}
	return 0
}

// pac is the preamble address code of a row, with an attribute (0-15) or
// an indent (16 + columns/4*2, with underline in bit 0).
func pac(n int, attr byte) word {
	hi := [16]byte{0, 0x11, 0x11, 0x12, 0x12, 0x15, 0x15, 0x16, 0x16, 0x17, 0x17, 0x10, 0x13, 0x13, 0x14, 0x14}[n]
	high := [16]bool{false, false, true, false, true, false, true, false, true, false, true, false, false, true, false, true}[n]
	lo := byte(0x40) + attr
	if high {
		lo += 0x20
	}
	return word{hi, lo}
}

// codeName says what a mid-row code does.
func codeName(lo byte) string {
	names := [...]string{i18n.N("white"), i18n.N("green"), i18n.N("blue"), i18n.N("cyan"), i18n.N("red"), i18n.N("yellow"), i18n.N("magenta"), i18n.N("italics")}
	s := names[(lo-0x20)/2]
	if lo%2 == 1 {
		s += ", " + i18n.N("underlined")
	}
	return s
}

// unit is a caption as content: its rows with breaks between them, the
// mid-row codes as tags.
func (c *caption) unit() content.Content {
	var out content.Content
	n := 0
	for i, r := range c.rows {
		if i > 0 {
			n++
			out = out.AppendTag(content.Tag{Kind: content.TagStandalone, ID: "br" + strconv.Itoa(n), Type: "break", Desc: i18n.N("new row")})
		}
		text := strings.Builder{}
		flush := func() {
			out = out.Append(text.String())
			text.Reset()
		}
		_, cells := r.span()
		for _, ce := range cells {
			if ce.r != 0 {
				text.WriteRune(ce.r)
				continue
			}
			flush()
			n++
			out = out.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strconv.Itoa(n), Type: "code", Desc: codeName(ce.code.lo), Native: fmt.Sprintf("%02x%02x", parity(ce.code.hi), parity(ce.code.lo))})
		}
		flush()
	}
	if out == nil {
		out = content.Content{}
	}
	return out
}

// Extract returns the captions of a Scenarist file as units.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	ex := &filter.Extraction{Units: []filter.Unit{}, Blocks: []filter.Block{}, Format: Format, SourceLang: lang}
	for _, c := range f.caps {
		src := c.unit()
		if strings.TrimSpace(src.Plain()) == "" {
			continue
		}
		drop := len(f.lines) > 0 && f.lines[0].drop
		key := timecode(c.show, drop) + " -->"
		hide := c.show
		if c.hide >= 0 {
			hide = c.hide
			key += " " + timecode(c.hide, drop)
		}
		notes := []string{key}
		switch c.mode {
		case rollUp:
			notes = append(notes, "A line of roll-up captions: it is shown as it is written and rolls up with the lines after it.")
		case paintOn:
			notes = append(notes, "Paint-on captions: shown as they are written.")
		}
		ex.Blocks = append(ex.Blocks, filter.Block{ID: c.id, Part: filter.PartBody})
		ex.Units = append(ex.Units, filter.Unit{ID: c.id, Block: c.id, Source: src, Key: key, Notes: notes,
			Timing: &filter.Timing{Start: ms(c.show), End: ms(hide), LineLimit: Columns}})
	}
	return ex, nil
}

// change is what a translated caption does to the file: the pairs it
// takes out, and the pairs it puts in before the first of them.
type change struct {
	at     at
	remove []at
	words  []word
	// from is where each of words was in the file, for the pairs kept as
	// they were (a pop-on load's codes); none for the pairs written.
	from []at
	// anchor is the pair of words that must be sent in frame, so the
	// caption shows when it did.
	anchor int
	frame  int64
	// erase: an erase sent while the caption before is still shown may be
	// taken into these pairs, at its frame (a pop-on load).
	erase bool
}

// Merge returns the file with its captions translated.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	var changes []change
	for _, c := range f.caps {
		t, ok := tr[c.id]
		if !ok || len(t) == 0 || t.Equal(c.unit()) {
			// A caption translated as it was (a name, a number, symbols)
			// is left as the source sends it.
			continue
		}
		ch, err := f.changeFor(c, t)
		if err != nil {
			drop := len(f.lines) > 0 && f.lines[0].drop
			return nil, fmt.Errorf("caption at %s: %v", timecode(c.show, drop), err)
		}
		changes = append(changes, ch)
	}
	if len(changes) == 0 {
		return data, nil
	}
	changes = nest(changes)
	removed := map[at]bool{}
	starts := map[at][]change{}
	for _, ch := range changes {
		for _, p := range ch.remove {
			removed[p] = true
		}
		if len(ch.words) > 0 {
			starts[ch.at] = append(starts[ch.at], ch)
		}
	}
	type out struct {
		words   []word
		want    int64 // the frame it should start in
		changed bool
		keep    *line
		// anchor is the index of the pair that must come in its frame;
		// -1 for none. erase: an erase line before may be taken in.
		anchor int
		erase  bool
		// slack is how far the source's line already reached back into the
		// one before it (sent while that was still being sent): the
		// written line may too, so it keeps the source's timing.
		slack int64
	}
	// The lines with their new pairs. A line is cut where a translated
	// caption starts and where what it had after one goes on, so each part
	// keeps its own time: a caption's at its anchor, the rest where it was.
	var outs []*out
	prevEnd := int64(-1)
	for i, ln := range f.lines {
		o := &out{want: ln.frame, keep: ln, anchor: -1, slack: max(0, prevEnd-ln.frame)}
		prevEnd = ln.frame + int64(len(ln.words))
		cut := func(want int64) {
			if len(o.words) > 0 {
				// A part of the line: written with its own time code.
				o.changed = true
				outs = append(outs, o)
			}
			o = &out{want: want, keep: ln, anchor: -1, changed: true}
		}
		afterChange := false
		for j, w := range ln.words {
			if chs := starts[at{i, j}]; len(chs) > 0 {
				cut(0)
				o.anchor = chs[0].anchor
				o.want = chs[0].frame - int64(o.anchor)
				o.erase = chs[0].erase
				for _, ch := range chs {
					o.words = append(o.words, ch.words...)
				}
				afterChange = true
			}
			if removed[at{i, j}] {
				o.changed = true
				continue
			}
			if afterChange {
				cut(ln.frame + int64(j))
				afterChange = false
			}
			o.words = append(o.words, w)
		}
		if len(o.words) > 0 || len(outs) == 0 || outs[len(outs)-1].keep != ln {
			outs = append(outs, o)
		}
	}
	// A load that must start before an erase sent while the caption before
	// is still shown takes the erase in, at its frame: the caption before
	// is erased when it was, and this one shown when it was.
	for i, o := range outs {
		if o.anchor < 0 || !o.erase {
			continue
		}
		for k := i - 1; k >= 0; k-- {
			prev := outs[k]
			if len(prev.words) == 0 {
				continue
			}
			if prev.want+int64(len(prev.words)) <= o.want || !eraseOnly(prev.words, true) {
				break
			}
			show := o.want + int64(o.anchor)
			start := show - int64(o.anchor+len(prev.words))
			p := int(prev.want - start)
			if p < 0 || p > o.anchor {
				break
			}
			// Not between the two of a doubled code.
			for p > 0 && p < len(o.words) && o.words[p] == o.words[p-1] && o.words[p].control() {
				p++
			}
			if p > o.anchor {
				break
			}
			// Erasing the memory a caption loads into only before its text.
			if !eraseOnly(prev.words, false) && p > firstText(o.words) {
				break
			}
			o.words = append(o.words[:p], append(append([]word(nil), prev.words...), o.words[p:]...)...)
			o.anchor += len(prev.words)
			o.want = show - int64(o.anchor)
			prev.words = nil
		}
	}
	// Lines are sent one after another: one starts when the one before has
	// been sent at the earliest.
	var b strings.Builder
	nl := "\n"
	if strings.Contains(f.src, "\r\n") {
		nl = "\r\n"
	}
	b.WriteString(strings.TrimSuffix(strings.SplitN(f.src, "\n", 2)[0], "\r") + nl)
	end, last := int64(-1), int64(0)
	for _, o := range outs {
		if len(o.words) == 0 {
			continue
		}
		start := max(o.want, 0)
		if start < end-o.slack {
			start = end - o.slack
		}
		// Time codes go on, never back.
		start = max(start, last)
		last = start
		end = start + int64(len(o.words))
		b.WriteString(nl)
		if !o.changed && start == o.keep.frame {
			b.WriteString(strings.TrimRight(o.keep.text, "\r"))
			b.WriteString(nl)
			continue
		}
		b.WriteString(timecode(start, o.keep.drop))
		b.WriteString("\t")
		for k, w := range o.words {
			if k > 0 {
				b.WriteString(" ")
			}
			h := fmt.Sprintf("%02x%02x", parity(w.hi), parity(w.lo))
			if o.keep.upper {
				h = strings.ToUpper(h)
			}
			b.WriteString(h)
		}
		b.WriteString(nl)
	}
	return []byte(b.String()), nil
}

// changeFor is what a translated caption does to the file.
func (f *file) changeFor(c *caption, t content.Content) (change, error) {
	switch c.mode {
	case popOn:
		ws, err := f.compose(c, t)
		if err != nil {
			return change{}, err
		}
		// The codes before the text (load, erase) and after it (erase,
		// end of caption) are kept as they were.
		before, bFrom := f.spanAt(c.load, c.first, false)
		after, aFrom := f.spanAt(c.last, c.eoc, true)
		var all []word
		var from []at
		all = append(all, before...)
		from = append(from, bFrom...)
		all = append(all, ws...)
		for range ws {
			from = append(from, at{-1, -1})
		}
		// What was sent between the text in other modes (a paint-on caption)
		// is kept, after the translation.
		mid, mFrom := f.otherModes(c)
		all = append(all, mid...)
		from = append(from, mFrom...)
		n := len(all)
		all = append(all, after...)
		from = append(from, aFrom...)
		eocIdx := len(all) - 1
		for i := n; i < len(all); i++ {
			if all[i].hi&^0x08 == 0x14 && all[i].lo == eoc {
				eocIdx = i
				break
			}
		}
		var remove []at
		for p := c.load; ; p = f.next(p) {
			remove = append(remove, p)
			if p == c.eoc {
				break
			}
		}
		// The second of the doubled End of Caption.
		if q := f.next(c.eoc); q.line >= 0 && f.word(q) == f.word(c.eoc) {
			remove = append(remove, q)
			all = append(all[:eocIdx+1], append([]word{f.word(q)}, all[eocIdx+1:]...)...)
			from = append(from[:eocIdx+1], append([]at{q}, from[eocIdx+1:]...)...)
		}
		return change{at: c.load, remove: remove, words: all, from: from, anchor: eocIdx, frame: f.frameOf(c.eoc), erase: true}, nil
	case paintOn:
		ws, err := f.compose(c, t)
		if err != nil {
			return change{}, err
		}
		var remove []at
		for p := c.first; p.line >= 0; p = f.next(p) {
			remove = append(remove, p)
			if p == c.last {
				break
			}
		}
		return change{at: c.first, remove: remove, words: ws, anchor: firstText(ws), frame: f.frameOf(c.first)}, nil
	default:
		ws, moved, err := f.composeRoll(c, t)
		if err != nil {
			return change{}, err
		}
		from, remove := c.chars[0], c.chars
		if moved {
			// The translation brings its own address: the source's, right
			// before the text, goes, so the text is not sent later.
			for p := f.prev(from); p.line >= 0 && isAddress(f.word(p)); p = f.prev(p) {
				from = p
				remove = append([]at{p}, remove...)
			}
		}
		return change{at: from, remove: remove, words: ws, anchor: firstText(ws), frame: f.frameOf(c.chars[0])}, nil
	}
}

// firstText is the index of the first pair with text: codes the
// translation needs before it (an address) are sent ahead of the frame
// the text was, so it shows when the source's did.
func firstText(ws []word) int {
	for i, w := range ws {
		if !w.control() {
			return i
		}
	}
	return 0
}

// otherModes is what a pop-on caption's text has between its first and
// last pair that was not sent to load it: pairs sent after a switch to
// another mode, up to and with the switch back, and erases of the screen.
func (f *file) otherModes(c *caption) ([]word, []at) {
	var ws []word
	var where []at
	loading, kept := true, false
	var prev word
	for p := c.first; p.line >= 0 && p != c.last; p = f.next(p) {
		w := f.word(p)
		if w.control() && (w.hi >= 0x18) != f.channel2 {
			continue
		}
		if w.control() && w == prev {
			// The second of a doubled code goes where the first did.
			if kept {
				ws, where = append(ws, w), append(where, p)
			}
			prev = word{}
			continue
		}
		prev = word{}
		if w.control() {
			prev = w
		}
		// An erase of the screen while loading is not part of the load.
		kept = !loading || w.hi&^0x08 == 0x14 && w.lo == edm
		if w.hi&^0x08 == 0x14 {
			switch w.lo {
			case rdc, ru2, ru3, ru4, tr, rtd:
				loading, kept = false, true
			case rcl:
				loading = true
			}
		}
		if kept {
			ws, where = append(ws, w), append(where, p)
		}
	}
	return ws, where
}

// eraseOnly reports whether a line only erases the caption shown or, with
// loading, the memory captions load into.
func eraseOnly(ws []word, loading bool) bool {
	for _, w := range ws {
		if w.hi&^0x08 != 0x14 || w.lo != edm && !(loading && w.lo == enm) {
			return false
		}
	}
	return len(ws) > 0
}

func (f *file) word(p at) word { return f.lines[p.line].words[p.word] }

func (f *file) frameOf(p at) int64 { return f.lines[p.line].frame + int64(p.word) }

func (f *file) prev(p at) at {
	if p.word > 0 {
		return at{p.line, p.word - 1}
	}
	for l := p.line - 1; l >= 0; l-- {
		if n := len(f.lines[l].words); n > 0 {
			return at{l, n - 1}
		}
	}
	return at{-1, -1}
}

// isAddress reports whether a pair is a preamble address code or a tab
// offset, on either channel.
func isAddress(w word) bool {
	hi := w.hi &^ 0x08
	if hi == 0x17 && w.lo >= 0x21 && w.lo <= 0x23 {
		return true
	}
	return w.lo >= 0x40 && w.lo <= 0x7f && pacRow(word{hi, w.lo}) > 0
}

func (f *file) next(p at) at {
	if p.word+1 < len(f.lines[p.line].words) {
		return at{p.line, p.word + 1}
	}
	for l := p.line + 1; l < len(f.lines); l++ {
		if len(f.lines[l].words) > 0 {
			return at{l, 0}
		}
	}
	return at{-1, -1}
}

// spanAt is the words from one place to another, with the first and
// without the last or, with last, without the first and with the last;
// and where each is.
func (f *file) spanAt(from, to at, last bool) ([]word, []at) {
	var out []word
	var where []at
	p := from
	if last {
		p = f.next(p)
	}
	for p.line >= 0 && p != to {
		out, where = append(out, f.word(p)), append(where, p)
		p = f.next(p)
	}
	if last && p == to {
		out, where = append(out, f.word(p)), append(where, p)
	}
	return out, where
}

// nest puts a change made within the pairs another keeps as they were (a
// paint-on caption sent while a pop-on one loads) into that one's, where
// its pairs were; a pair kept that another change takes out goes.
func nest(changes []change) []change {
	startAt := map[at]int{}
	removedBy := map[at]int{}
	for i, ch := range changes {
		startAt[ch.at] = i
		for _, p := range ch.remove {
			if _, ok := removedBy[p]; !ok || p == ch.at {
				removedBy[p] = i
			}
		}
	}
	taken := map[int]bool{}
	for i := range changes {
		ch := &changes[i]
		if ch.from == nil || taken[i] {
			continue
		}
		var words []word
		var from []at
		anchor := ch.anchor
		for k, w := range ch.words {
			p := ch.from[k]
			if p.line >= 0 {
				if j, ok := startAt[p]; ok && j != i && !taken[j] {
					taken[j] = true
					if k < ch.anchor {
						anchor += len(changes[j].words)
					}
					words = append(words, changes[j].words...)
					for range changes[j].words {
						from = append(from, at{-1, -1})
					}
				}
				if j, ok := removedBy[p]; ok && j != i {
					if k < ch.anchor {
						anchor--
					}
					continue
				}
			}
			words, from = append(words, w), append(from, p)
		}
		ch.words, ch.from, ch.anchor = words, from, anchor
	}
	var out []change
	for i, ch := range changes {
		if !taken[i] {
			out = append(out, ch)
		} else {
			// Its pairs still go from where they were.
			out = append(out, change{at: ch.at, remove: ch.remove})
		}
	}
	return out
}

// piece is a part of a translated row: text, or a mid-row code.
type piece struct {
	text string
	code *word
}

// rowsOf reads a translation into rows of text and mid-row codes: a new
// row at each break of the source and each line break typed in.
func (c *caption) translatedRows(t content.Content) [][]piece {
	src := c.unit()
	codes := map[string]word{}
	breaks := map[string]bool{}
	for _, tg := range src.Tags() {
		if tg.Type == "break" {
			breaks[tg.ID] = true
			continue
		}
		v, _ := strconv.ParseUint(tg.Native, 16, 16)
		codes[tg.ID] = word{byte(v>>8) & 0x7F, byte(v) & 0x7F}
	}
	rows := [][]piece{{}}
	for _, r := range t {
		if r.Tag != nil {
			if breaks[r.Tag.ID] || r.Tag.Type == "break" {
				rows = append(rows, nil)
			} else if w, ok := codes[r.Tag.ID]; ok && r.Tag.Kind == content.TagStandalone {
				rows[len(rows)-1] = append(rows[len(rows)-1], piece{code: &w})
			}
			continue
		}
		for i, part := range strings.Split(r.Text, "\n") {
			if i > 0 {
				rows = append(rows, nil)
			}
			rows[len(rows)-1] = append(rows[len(rows)-1], piece{text: part})
		}
	}
	var out [][]piece
	for _, r := range rows {
		if r = trimPieces(r); len(r) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// encodeRow writes a row's text and codes as pairs: two basic characters
// a pair, each special or extended character and mid-row code a pair of
// its own, sent twice. width is the columns the row takes.
func encodeRow(ln []piece, once bool) (out []word, width int, err error) {
	pend := byte(0)
	char := func(b byte) {
		if pend == 0 {
			pend = b
			return
		}
		out = append(out, word{pend, b})
		pend = 0
	}
	code := func(w word) {
		if pend != 0 {
			out = append(out, word{pend, 0})
			pend = 0
		}
		out = append(out, w)
		if !once {
			out = append(out, w)
		}
	}
	for _, p := range ln {
		if p.code != nil {
			code(*p.code)
			width++
			continue
		}
		for _, r := range p.text {
			enc, err := encode(r)
			if err != nil {
				return nil, 0, err
			}
			width++
			switch {
			case len(enc) == 1:
				char(enc[0])
			case enc[0] == 0x11:
				code(word{enc[0], enc[1]})
			default:
				// The basic letter first, for decoders without the
				// extended ones; the extended code takes its place.
				char(enc[2])
				code(word{enc[0], enc[1]})
			}
		}
	}
	if pend != 0 {
		out = append(out, word{pend, 0})
	}
	return out, width, nil
}

func (f *file) onChannel(out []word) []word {
	if f.channel2 {
		for i := range out {
			if out[i].control() {
				out[i].hi |= 0x08
			}
		}
	}
	return out
}

// compose writes a pop-on or paint-on caption's translation as pairs: a
// row's address, its text and codes, row by row.
func (f *file) compose(c *caption, t content.Content) ([]word, error) {
	var lines [][]piece
	for _, r := range c.translatedRows(t) {
		// Rows too long for the screen are wrapped at a space.
		lines = append(lines, wrap(r, Columns)...)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("the translation is empty")
	}
	if len(lines) > 15 {
		return nil, fmt.Errorf("the translation needs %d rows; a caption has 15", len(lines))
	}
	bottom := c.rows[len(c.rows)-1].n
	first := bottom - len(lines) + 1
	if first < 1 {
		first = 1
	}
	var out []word
	twice := func(w word) {
		out = append(out, w)
		if !c.once {
			out = append(out, w)
		}
	}
	for i, ln := range lines {
		// The source's row that stands where this one does, counted from
		// the bottom, gives its look and alignment.
		k := len(c.rows) - len(lines) + i
		if k < 0 {
			k = 0
		}
		tmpl := c.rows[k]
		cells, width, err := encodeRow(ln, c.once)
		if err != nil {
			return nil, err
		}
		col := align(tmpl, width)
		n := first + i
		if n > 15 {
			return nil, fmt.Errorf("the translation needs more rows than the screen has below the caption's first")
		}
		styled := tmpl.attr < 0x10 && tmpl.attr&^1 != 0
		if styled && col == 0 {
			col = 1
		}
		pos := col
		if styled {
			pos = col - 1
		}
		under := tmpl.attr & 1
		twice(pac(n, 0x10+byte(pos/4)*2+under))
		if off := pos % 4; off > 0 {
			twice(word{0x17, 0x20 + byte(off)})
		}
		if styled {
			twice(word{0x11, 0x20 + tmpl.attr})
		}
		out = append(out, cells...)
	}
	return f.onChannel(out), nil
}

// composeRoll writes a roll-up line's translation as pairs: its text, and
// where it is longer than the row has columns from where the line starts,
// a carriage return and the rest on the next line.
func (f *file) composeRoll(c *caption, t content.Content) (ws []word, moved bool, err error) {
	rows := c.translatedRows(t)
	if len(rows) == 0 {
		return nil, false, fmt.Errorf("the translation is empty")
	}
	start, _ := c.rows[0].span()
	var out []word
	firstLine := true
	for _, r := range rows {
		width := Columns
		if firstLine {
			// A line that would run past the last column from where the
			// source's starts is addressed further left, as far as it needs
			// to the indent before.
			if w := piecesWidth(r); start+w > Columns {
				// An indent alone, with no tab offset after it: no longer to
				// send than the source's address.
				start, moved = max(0, Columns-w)/4*4, true
				out = append(out, pac(c.rows[0].n, 0x10+byte(start/4)*2))
				if !c.once {
					out = append(out, out[len(out)-1])
				}
			}
			width = Columns - start
		}
		for _, ln := range wrap(r, width) {
			if !firstLine {
				out = append(out, word{0x14, cr})
				if !c.once {
					out = append(out, word{0x14, cr})
				}
			}
			cells, _, err := encodeRow(ln, c.once)
			if err != nil {
				return nil, false, err
			}
			out = append(out, cells...)
			firstLine = false
		}
	}
	return f.onChannel(out), moved, nil
}

func trimPieces(r []piece) []piece {
	for len(r) > 0 && r[0].code == nil && strings.TrimSpace(r[0].text) == "" {
		r = r[1:]
	}
	for len(r) > 0 && r[len(r)-1].code == nil && strings.TrimSpace(r[len(r)-1].text) == "" {
		r = r[:len(r)-1]
	}
	if len(r) > 0 && r[0].code == nil {
		r[0].text = strings.TrimLeft(r[0].text, " ")
	}
	if len(r) > 0 && r[len(r)-1].code == nil {
		r[len(r)-1].text = strings.TrimRight(r[len(r)-1].text, " ")
	}
	return r
}

// piecesWidth is the columns a row of pieces takes.
func piecesWidth(ps []piece) int {
	n := 0
	for _, p := range ps {
		if p.code != nil {
			n++
		} else {
			n += utf8.RuneCountInString(p.text)
		}
	}
	return n
}

// wrap cuts a row longer than a caption's columns at its last space that
// fits, or where it must.
func wrap(r []piece, cols int) [][]piece {
	width := func(ps []piece) int {
		n := 0
		for _, p := range ps {
			if p.code != nil {
				n++
			} else {
				n += utf8.RuneCountInString(p.text)
			}
		}
		return n
	}
	var out [][]piece
	for width(r) > cols {
		// The cells of the row, to find where to cut.
		n := 0
		cut, cutPiece, cutAt := -1, 0, 0
		for i, p := range r {
			if p.code != nil {
				n++
				continue
			}
			for j, ch := range p.text {
				if n >= cols {
					break
				}
				if ch == ' ' {
					cut, cutPiece, cutAt = n, i, j
				}
				n++
			}
		}
		if cut <= 0 {
			// No space: cut at the last column.
			n, cutPiece, cutAt = 0, 0, 0
			for i, p := range r {
				if p.code != nil {
					n++
					continue
				}
				if n+utf8.RuneCountInString(p.text) >= cols {
					runes := []rune(p.text)
					cutPiece, cutAt = i, len(string(runes[:cols-n]))
					break
				}
				n += utf8.RuneCountInString(p.text)
			}
			head := append(append([]piece(nil), r[:cutPiece]...), piece{text: r[cutPiece].text[:cutAt]})
			tail := append([]piece{{text: r[cutPiece].text[cutAt:]}}, r[cutPiece+1:]...)
			out = append(out, head)
			r = trimPieces(tail)
			continue
		}
		head := append(append([]piece(nil), r[:cutPiece]...), piece{text: r[cutPiece].text[:cutAt]})
		tail := append([]piece{{text: r[cutPiece].text[cutAt+1:]}}, r[cutPiece+1:]...)
		out = append(out, trimPieces(head))
		r = trimPieces(tail)
	}
	return append(out, r)
}

// align is where a translated row starts: centred where the source's row
// was centred, at the right edge where it stood there, else where it
// started.
func align(src *row, width int) int {
	start, cells := src.span()
	sw := len(cells)
	center := float64(start) + float64(sw)/2
	switch {
	case center >= 13 && center <= 19 && start > 0:
		return (Columns - width) / 2
	case start+sw >= Columns-1 && start > 4:
		return Columns - width
	}
	if start+width > Columns {
		return max(0, Columns-width)
	}
	return start
}
