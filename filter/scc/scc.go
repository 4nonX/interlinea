// Package scc reads and writes Scenarist closed captions (.scc): CEA-608
// line 21 captions as hexadecimal byte pairs with their time codes, the
// form in which US broadcast and streaming captions are exchanged.
//
// A pop-on caption is one unit: the text loaded into the hidden caption
// memory and shown at End of Caption, until the screen is erased or the
// next caption is shown. Its rows are kept apart by tags; colour and
// italics codes inside a row (mid-row codes) are tags too. Special and
// extended characters are read as the letters they stand for.
//
// On merge, a translated caption is loaded again: its rows are placed from
// the caption's bottom row up, aligned as the source's rows were (centred,
// left or right), a row longer than the 32 columns a caption has is
// wrapped at a space, and the time code of the line it is loaded in moves
// earlier so it still appears when it did (later lines move only where a
// longer load reaches into them). Letters CEA-608 has no code for are
// refused. Everything else in the file stays as it was.
//
// Roll-up and paint-on captions, written while a programme is broadcast,
// are not translated: such a file is refused with that reason.
package scc

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
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
}

type row struct {
	n     int  // 1 to 15
	col   int  // where it starts
	attr  byte // the PAC's attribute: 0-15, colour or italics with underline in bit 0; 16+ for an indent
	cells []cell
}

type caption struct {
	id string
	// load is the first pair of the load (Resume Caption Loading or Erase
	// Non-displayed Memory), text where the first text pair is, end the
	// last text pair, eoc the End of Caption pair.
	load, text, end, eoc at
	rows                 []*row
	show, hide           int64 // frames; hide -1 where not known
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
		ln := &line{text: l, frame: frame, drop: drop}
		for _, h := range strings.Fields(rest) {
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

// read follows the codes through the file and finds the pop-on captions.
func (f *file) read() error {
	var cur *caption // being loaded
	var shown *caption
	rows := map[int]*row{}
	var r *row
	mode := 0 // 0 none, 1 pop-on, 2 text
	var prev word
	var prevPos at
	prevControl := false
	// Which channel the captions are on: that of the first control code.
	chanSet := false
	for li, ln := range f.lines {
		for wi, w := range ln.words {
			pos := at{li, wi}
			frame := ln.frame + int64(wi)
			if w.control() {
				ch2 := w.hi >= 0x18
				if !chanSet {
					f.channel2, chanSet = ch2, true
				}
				if ch2 != f.channel2 {
					prev, prevControl = w, false
					continue
				}
				// A control code is sent twice; the second is not acted on.
				if prevControl && w == prev {
					prevControl = false
					// The second of a code within the text belongs to it.
					if cur != nil && cur.text.line >= 0 && cur.end == prevPos {
						cur.end = pos
					}
					prevPos = pos
					continue
				}
				prev, prevControl, prevPos = w, true, pos
				w.hi &^= 0x08
				switch {
				case w.hi == 0x14 && w.lo >= 0x20 && w.lo <= 0x2F, w.hi == 0x15 && w.lo >= 0x20 && w.lo <= 0x2F:
					if w.hi == 0x15 {
						// Field 2 (CC3, CC4): not these captions.
						continue
					}
					switch w.lo {
					case rcl:
						mode = 1
						if cur == nil {
							cur = &caption{load: pos, text: at{-1, -1}, eoc: at{-1, -1}}
							rows, r = map[int]*row{}, nil
						}
					case enm:
						if cur == nil {
							cur = &caption{load: pos, text: at{-1, -1}, eoc: at{-1, -1}}
						}
						rows, r = map[int]*row{}, nil
					case ru2, ru3, ru4:
						return fmt.Errorf("the file has roll-up captions, written while a programme is broadcast; Interlinea translates pop-on captions")
					case rdc:
						return fmt.Errorf("the file has paint-on captions, written while a programme is broadcast; Interlinea translates pop-on captions")
					case tr, rtd:
						mode = 2
					case edm:
						if shown != nil && shown.hide < 0 {
							shown.hide = frame
						}
					case eoc:
						if shown != nil && shown.hide < 0 {
							shown.hide = frame
						}
						if cur != nil {
							cur.eoc = pos
							cur.show, cur.hide = frame, -1
							for n := 1; n <= 15; n++ {
								if rw := rows[n]; rw != nil && len(rw.cells) > 0 {
									cur.rows = append(cur.rows, rw)
								}
							}
							if len(cur.rows) > 0 {
								cur.id = "c" + strconv.Itoa(len(f.caps)+1)
								f.caps = append(f.caps, cur)
								shown = cur
							} else {
								shown = nil
							}
						}
						cur, rows, r = nil, map[int]*row{}, nil
					case bs:
						if r != nil && len(r.cells) > 0 {
							r.cells = r.cells[:len(r.cells)-1]
						}
					case der, cr:
					}
				case mode != 1 || cur == nil:
				case w.lo >= 0x40 && (w.hi >= 0x10 && w.hi <= 0x17):
					// A preamble address code: a row, and where in it.
					n := pacRow(w)
					if n == 0 {
						continue
					}
					r = rows[n]
					if r == nil {
						r = &row{n: n}
						rows[n] = r
					}
					a := (w.lo - 0x40) & 0x1F
					r.attr = a
					if a >= 0x10 {
						r.col = int(a-0x10) / 2 * 4
					} else {
						r.col = 0
					}
					r.cells = nil
					f.markText(cur, pos)
				case w.hi == 0x17 && w.lo >= 0x21 && w.lo <= 0x23:
					// Tab offset.
					if r != nil {
						if len(r.cells) == 0 {
							r.col += int(w.lo - 0x20)
						} else {
							for i := 0; i < int(w.lo-0x20); i++ {
								r.cells = append(r.cells, cell{r: ' '})
							}
						}
					}
					f.markText(cur, pos)
				case w.hi == 0x11 && w.lo >= 0x20 && w.lo <= 0x2F:
					// A mid-row code: colour or italics from here on.
					if r != nil {
						r.cells = append(r.cells, cell{code: word{0x11, w.lo}})
					}
					f.markText(cur, pos)
				case w.hi == 0x11 && w.lo >= 0x30 && w.lo <= 0x3F:
					if r != nil {
						r.cells = append(r.cells, cell{r: special[w.lo-0x30]})
					}
					f.markText(cur, pos)
				case (w.hi == 0x12 || w.hi == 0x13) && w.lo >= 0x20 && w.lo <= 0x3F:
					// An extended character takes the place of the one before.
					if r != nil {
						if len(r.cells) > 0 && r.cells[len(r.cells)-1].code == (word{}) {
							r.cells = r.cells[:len(r.cells)-1]
						}
						r.cells = append(r.cells, cell{r: extended[w.hi-0x12][w.lo-0x20]})
					}
					f.markText(cur, pos)
				default:
					// Background colours and the like: kept with the text.
					f.markText(cur, pos)
				}
				continue
			}
			prevControl, prevPos = false, pos
			if mode != 1 || cur == nil || w.hi < 0x20 && w.hi != 0 {
				continue
			}
			for _, b := range []byte{w.hi, w.lo} {
				if b >= 0x20 && r != nil {
					r.cells = append(r.cells, cell{r: basic(b)})
				}
			}
			if r != nil {
				f.markText(cur, pos)
			}
		}
	}
	return nil
}

func hasText(rows map[int]*row) bool {
	for _, r := range rows {
		if len(r.cells) > 0 {
			return true
		}
	}
	return false
}

func (f *file) markText(c *caption, pos at) {
	if c.text.line < 0 {
		c.text = pos
	}
	c.end = pos
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
	names := [...]string{"white", "green", "blue", "cyan", "red", "yellow", "magenta", "italics"}
	s := names[(lo-0x20)/2]
	if lo%2 == 1 {
		s += ", underlined"
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
			out = out.AppendTag(content.Tag{Kind: content.TagStandalone, ID: "br" + strconv.Itoa(n), Type: "break", Desc: "new row"})
		}
		text := strings.Builder{}
		flush := func() {
			out = out.Append(text.String())
			text.Reset()
		}
		cells := trimCells(r.cells)
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

// trimCells drops the spaces at a row's edges.
func trimCells(cs []cell) []cell {
	for len(cs) > 0 && cs[0].r == ' ' {
		cs = cs[1:]
	}
	for len(cs) > 0 && cs[len(cs)-1].r == ' ' {
		cs = cs[:len(cs)-1]
	}
	return cs
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
		drop := f.lines[c.eoc.line].drop
		hide := c.hide
		if hide < 0 {
			hide = c.show
		}
		key := timecode(c.show, drop) + " --> " + timecode(hide, drop)
		if c.hide < 0 {
			key = timecode(c.show, drop) + " -->"
		}
		ex.Blocks = append(ex.Blocks, filter.Block{ID: c.id, Part: filter.PartBody})
		ex.Units = append(ex.Units, filter.Unit{ID: c.id, Block: c.id, Source: src, Key: key, Notes: []string{key},
			Timing: &filter.Timing{Start: ms(c.show), End: ms(hide), LineLimit: Columns}})
	}
	return ex, nil
}

// Merge returns the file with its captions translated.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	// The new words of each translated caption, and where its show
	// frame must be.
	type load struct {
		c     *caption
		words []word
		eocAt int // the End of Caption pair's index in words
	}
	var loads []load
	for _, c := range f.caps {
		t, ok := tr[c.id]
		if !ok || len(t) == 0 {
			continue
		}
		ws, err := f.compose(c, t)
		if err != nil {
			return nil, fmt.Errorf("caption at %s: %v", timecode(c.show, f.lines[c.eoc.line].drop), err)
		}
		// The codes before the text (load, erase) and after it (erase,
		// end of caption) are kept as they were.
		before := f.span(c.load, c.text, false)
		after := f.span(c.end, c.eoc, true)
		var all []word
		all = append(all, before...)
		all = append(all, ws...)
		n := len(all)
		all = append(all, after...)
		eocIdx := n + len(after) - 1
		for i := n; i < len(all); i++ {
			if all[i].hi&^0x08 == 0x14 && all[i].lo == eoc {
				eocIdx = i
				break
			}
		}
		loads = append(loads, load{c, all, eocIdx})
	}
	if len(loads) == 0 {
		return data, nil
	}
	// The lines with their new words: a caption's load goes in the line it
	// starts in, and the pairs it had in later lines are taken out.
	type out struct {
		words   []word
		want    int64 // the frame it should start in
		changed bool
		keep    *line
		// eoc is the index of the End of Caption of a load the line
		// starts with; -1 for none.
		eoc int
	}
	outs := make([]*out, len(f.lines))
	for i, ln := range f.lines {
		outs[i] = &out{words: ln.words, want: ln.frame, keep: ln, eoc: -1}
	}
	// Each line's pairs as indexes still kept.
	removed := map[at]bool{}
	starts := map[int][]load{}
	for _, l := range loads {
		for p := l.c.load; ; p = f.next(p) {
			removed[p] = true
			if p == l.c.eoc {
				break
			}
		}
		// The second of the doubled End of Caption.
		if q := f.next(l.c.eoc); q.line >= 0 && f.word(q) == f.word(l.c.eoc) {
			removed[q] = true
			l.words = append(l.words[:l.eocAt+1], append([]word{f.word(q)}, l.words[l.eocAt+1:]...)...)
		}
		starts[l.c.load.line] = append(starts[l.c.load.line], l)
	}
	for i, ln := range f.lines {
		var ws []word
		changed := false
		for j, w := range ln.words {
			for _, l := range starts[i] {
				if l.c.load.word == j {
					// The load starts here: its words go in now, timed so
					// End of Caption comes in the frame it did.
					if len(ws) == 0 {
						outs[i].want = f.frameOf(l.c.eoc) - int64(l.eocAt)
						outs[i].eoc = l.eocAt
					}
					ws = append(ws, l.words...)
					changed = true
				}
			}
			if removed[at{i, j}] {
				changed = true
				continue
			}
			ws = append(ws, w)
		}
		outs[i].words, outs[i].changed = ws, changed
	}
	// A load that must start before an erase sent while the caption before
	// is still shown takes the erase in, at its frame: the caption before
	// is erased when it was, and this one shown when it was.
	for i, o := range outs {
		if o.eoc < 0 {
			continue
		}
		for k := i - 1; k >= 0; k-- {
			prev := outs[k]
			if len(prev.words) == 0 {
				continue
			}
			if prev.want+int64(len(prev.words)) <= o.want || !eraseOnly(prev.words) {
				break
			}
			show := o.want + int64(o.eoc)
			start := show - int64(o.eoc+len(prev.words))
			p := int(prev.want - start)
			if p < 0 || p > o.eoc {
				break
			}
			// Not between the two of a doubled code.
			for p > 0 && p < len(o.words) && o.words[p] == o.words[p-1] && o.words[p].control() {
				p++
			}
			if p > o.eoc {
				break
			}
			o.words = append(o.words[:p], append(append([]word(nil), prev.words...), o.words[p:]...)...)
			o.eoc += len(prev.words)
			o.want = show - int64(o.eoc)
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
	end := int64(-1)
	for _, o := range outs {
		if len(o.words) == 0 {
			continue
		}
		start := o.want
		if start < end {
			start = end
			o.changed = true
		}
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

// eraseOnly reports whether a line only erases the caption shown.
func eraseOnly(ws []word) bool {
	for _, w := range ws {
		if w.hi&^0x08 != 0x14 || w.lo != edm {
			return false
		}
	}
	return len(ws) > 0
}

func (f *file) word(p at) word { return f.lines[p.line].words[p.word] }

func (f *file) frameOf(p at) int64 { return f.lines[p.line].frame + int64(p.word) }

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

// span is the words from one place to another, without the first or,
// with last, without the first and with the last.
func (f *file) span(from, to at, last bool) []word {
	var out []word
	p := from
	if last {
		p = f.next(p)
	}
	for p.line >= 0 && p != to {
		out = append(out, f.word(p))
		p = f.next(p)
	}
	if last && p == to {
		out = append(out, f.word(p))
	}
	return out
}

// piece is a part of a translated row: text, or a mid-row code.
type piece struct {
	text string
	code *word
}

// compose writes a caption's translation as pairs: a row's address, its
// text and codes, row by row.
func (f *file) compose(c *caption, t content.Content) ([]word, error) {
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
	// Rows too long for the screen are wrapped at a space.
	var lines [][]piece
	for _, r := range rows {
		r = trimPieces(r)
		if len(r) == 0 {
			continue
		}
		lines = append(lines, wrap(r)...)
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
		out = append(out, w, w)
	}
	for i, ln := range lines {
		// The source's row that stands where this one does, counted from
		// the bottom, gives its look and alignment.
		k := len(c.rows) - len(lines) + i
		if k < 0 {
			k = 0
		}
		tmpl := c.rows[k]
		width := 0
		var cellsOut []word
		pend := byte(0)
		flushChar := func(b byte) {
			if pend == 0 {
				pend = b
				return
			}
			cellsOut = append(cellsOut, word{pend, b})
			pend = 0
		}
		padCode := func(w word) {
			if pend != 0 {
				cellsOut = append(cellsOut, word{pend, 0})
				pend = 0
			}
			cellsOut = append(cellsOut, w, w)
		}
		for _, p := range ln {
			if p.code != nil {
				padCode(*p.code)
				width++
				continue
			}
			for _, r := range p.text {
				enc, err := encode(r)
				if err != nil {
					return nil, err
				}
				width++
				switch {
				case len(enc) == 1:
					flushChar(enc[0])
				case enc[0] == 0x11:
					padCode(word{enc[0], enc[1]})
				default:
					// The basic letter first, for decoders without the
					// extended ones; the extended code takes its place.
					flushChar(enc[2])
					padCode(word{enc[0], enc[1]})
				}
			}
		}
		if pend != 0 {
			cellsOut = append(cellsOut, word{pend, 0})
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
		out = append(out, cellsOut...)
	}
	if f.channel2 {
		for i := range out {
			if out[i].control() {
				out[i].hi |= 0x08
			}
		}
	}
	return out, nil
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

// wrap cuts a row longer than a caption's columns at its last space that
// fits, or where it must.
func wrap(r []piece) [][]piece {
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
	for width(r) > Columns {
		// The cells of the row, to find where to cut.
		n := 0
		cut, cutPiece, cutAt := -1, 0, 0
		for i, p := range r {
			if p.code != nil {
				n++
				continue
			}
			for j, ch := range p.text {
				if n >= Columns {
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
				if n+utf8.RuneCountInString(p.text) >= Columns {
					runes := []rune(p.text)
					cutPiece, cutAt = i, len(string(runes[:Columns-n]))
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
	sw := len(trimCells(src.cells))
	lead := len(src.cells) - len(trimLeft(src.cells))
	start := src.col + lead
	if src.attr < 0x10 && src.attr&^1 != 0 {
		start++
	}
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

func trimLeft(cs []cell) []cell {
	for len(cs) > 0 && cs[0].r == ' ' {
		cs = cs[1:]
	}
	return cs
}
