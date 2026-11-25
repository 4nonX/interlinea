package pdf

import (
	"bytes"
	"math"
)

// matrix is a transformation [a b c d e f] as PDF writes it.
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// mul is m applied first, then n.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

// chunk is a piece of text shown in one go, where it is on the page.
type chunk struct {
	text string
	// x, y is where its baseline starts and w how far it runs, in the
	// direction it is written; size is the height of its font.
	x, y, w, size float64
	// angle is the direction it is written in, in steps of five degrees.
	angle        int
	bold, italic bool
}

type textState struct {
	font                       *font
	size, charSpace, wordSpace float64
	scale, leading, rise       float64
}

type graphicsState struct {
	ctm  matrix
	text textState
}

// marked is a stretch of marked content.
type marked struct {
	// actual is the text the stretch stands for, where it says so.
	actual    string
	hasActual bool
	shown     bool
}

// reader interprets the content of pages.
type reader struct {
	f      *file
	byRef  map[int]*font
	chunks []chunk
	// known and unknown count the glyphs whose character the fonts said,
	// and did not say.
	known, unknown int
	ops            int
}

// maxOps is how many operators a file's pages may have together.
const maxOps = 20_000_000

func (r *reader) fontOf(v any) *font {
	if rf, ok := v.(ref); ok {
		if ft, ok := r.byRef[rf.num]; ok {
			return ft
		}
		d := r.f.dict(v)
		if d == nil {
			return nil
		}
		ft := loadFont(r.f, d)
		r.byRef[rf.num] = ft
		return ft
	}
	if d := r.f.dict(v); d != nil {
		return loadFont(r.f, d)
	}
	return nil
}

// run interprets a content stream with its resources.
func (r *reader) run(data []byte, resources dict, ctm matrix, depth int, active map[int]bool) {
	gs := graphicsState{ctm: ctm, text: textState{scale: 1}}
	var stack []graphicsState
	var tm, tlm matrix = identity, identity
	var marks []marked
	fonts := r.f.dict(resources["Font"])
	xobjects := r.f.dict(resources["XObject"])
	properties := r.f.dict(resources["Properties"])
	localFonts := map[name]*font{}
	l := &lexer{b: data}
	var args []any
	num := func(i int) float64 {
		if i < 0 || i >= len(args) {
			return 0
		}
		n, _ := args[i].(float64)
		return n
	}
	show := func(s pstring) {
		ft := gs.text.font
		if ft == nil {
			return
		}
		ts := gs.text
		// Text space to the page.
		trm := matrix{ts.size * ts.scale, 0, 0, ts.size, 0, ts.rise}.mul(tm).mul(gs.ctm)
		var text []byte
		advance := 0.0
		for _, g := range ft.glyphs(s) {
			if g.known {
				r.known++
			} else if g.text == "" {
				r.unknown++
			}
			text = append(text, g.text...)
			w := g.width/1000*ts.size + ts.charSpace
			if g.space {
				w += ts.wordSpace
			}
			advance += w * ts.scale
		}
		along := math.Hypot(tm[0]*gs.ctm[0]+tm[1]*gs.ctm[2], tm[0]*gs.ctm[1]+tm[1]*gs.ctm[3])
		c := chunk{
			text: string(text), x: trm[4], y: trm[5], w: advance * along, size: math.Hypot(trm[2], trm[3]),
			angle: int(math.Round(math.Atan2(trm[1], trm[0])*180/math.Pi/5)) * 5,
			bold:  ft.bold, italic: ft.italic,
		}
		tm = matrix{1, 0, 0, 1, advance, 0}.mul(tm)
		if c.size <= 0 {
			return
		}
		// A stretch that says its text stands for itself, once.
		for i := len(marks) - 1; i >= 0; i-- {
			if marks[i].hasActual {
				if marks[i].shown {
					return
				}
				marks[i].shown = true
				c.text = marks[i].actual
				break
			}
		}
		if privateOnly(c.text) {
			// A sign of a symbol font that no character stands for: in
			// running text that is the mark of a list item.
			c.text = "•"
		}
		if c.text != "" && len(r.chunks) < 4_000_000 {
			r.chunks = append(r.chunks, c)
		}
	}
	newLine := func(tx, ty float64) {
		tlm = matrix{1, 0, 0, 1, tx, ty}.mul(tlm)
		tm = tlm
	}
	for {
		v, err := l.next()
		if err != nil {
			return
		}
		o, isOp := v.(op)
		if !isOp {
			if len(args) < 4096 {
				args = append(args, v)
			}
			continue
		}
		if o == "" {
			return
		}
		r.ops++
		if r.ops > maxOps {
			return
		}
		switch o {
		case "q":
			if len(stack) < 256 {
				stack = append(stack, gs)
			}
		case "Q":
			if n := len(stack); n > 0 {
				gs = stack[n-1]
				stack = stack[:n-1]
			}
		case "cm":
			if len(args) >= 6 {
				gs.ctm = matrix{num(0), num(1), num(2), num(3), num(4), num(5)}.mul(gs.ctm)
			}
		case "BT":
			tm, tlm = identity, identity
		case "Tf":
			if len(args) >= 2 {
				if n, ok := args[0].(name); ok {
					ft, have := localFonts[n]
					if !have {
						ft = r.fontOf(fonts[n])
						localFonts[n] = ft
					}
					gs.text.font = ft
				}
				gs.text.size = num(1)
			}
		case "Td":
			newLine(num(0), num(1))
		case "TD":
			gs.text.leading = -num(1)
			newLine(num(0), num(1))
		case "Tm":
			if len(args) >= 6 {
				tm = matrix{num(0), num(1), num(2), num(3), num(4), num(5)}
				tlm = tm
			}
		case "T*":
			newLine(0, -gs.text.leading)
		case "TL":
			gs.text.leading = num(0)
		case "Tc":
			gs.text.charSpace = num(0)
		case "Tw":
			gs.text.wordSpace = num(0)
		case "Tz":
			gs.text.scale = num(0) / 100
		case "Ts":
			gs.text.rise = num(0)
		case "Tj":
			if len(args) >= 1 {
				if s, ok := args[0].(pstring); ok {
					show(s)
				}
			}
		case "'":
			newLine(0, -gs.text.leading)
			if len(args) >= 1 {
				if s, ok := args[0].(pstring); ok {
					show(s)
				}
			}
		case "\"":
			if len(args) >= 3 {
				gs.text.wordSpace, gs.text.charSpace = num(0), num(1)
				newLine(0, -gs.text.leading)
				if s, ok := args[2].(pstring); ok {
					show(s)
				}
			}
		case "TJ":
			if len(args) >= 1 {
				list, _ := args[0].([]any)
				for _, e := range list {
					switch x := e.(type) {
					case pstring:
						show(x)
					case float64:
						tm = matrix{1, 0, 0, 1, -x / 1000 * gs.text.size * gs.text.scale, 0}.mul(tm)
					}
				}
			}
		case "Do":
			if len(args) >= 1 && depth < 12 {
				if n, ok := args[0].(name); ok {
					r.form(xobjects[n], resources, gs.ctm, depth, active)
				}
			}
		case "BMC", "BDC":
			m := marked{}
			if o == "BDC" && len(args) >= 2 {
				var props dict
				switch x := args[1].(type) {
				case dict:
					props = x
				case name:
					props = r.f.dict(properties[x])
				}
				if s, ok := r.f.resolve(props["ActualText"]).(pstring); ok {
					m.actual, m.hasActual = textString(s), true
				}
			}
			if len(marks) < 256 {
				marks = append(marks, m)
			}
		case "EMC":
			if n := len(marks); n > 0 {
				marks = marks[:n-1]
			}
		case "BI":
			// An image in the stream: its data runs to EI.
			if at := bytes.Index(l.b[l.p:], []byte("ID")); at >= 0 {
				l.p += at + 2
			}
			for {
				at := bytes.Index(l.b[l.p:], []byte("EI"))
				if at < 0 {
					l.p = len(l.b)
					break
				}
				end := l.p + at + 2
				before := at == 0 || isSpace(l.b[l.p+at-1])
				l.p = end
				if before && (end >= len(l.b) || isSpace(l.b[end])) {
					break
				}
			}
		}
		args = args[:0]
	}
}

// form draws a form: content with resources of its own, used by pages.
func (r *reader) form(v any, parent dict, ctm matrix, depth int, active map[int]bool) {
	s, ok := r.f.resolve(v).(*stream)
	if !ok || r.f.name(s.dict["Subtype"]) != "Form" {
		return
	}
	if rf, ok := v.(ref); ok {
		if active[rf.num] {
			return // a form that draws itself
		}
		active[rf.num] = true
		defer delete(active, rf.num)
	}
	data, err := decodeStream(r.f, s)
	if err != nil {
		return
	}
	if m := r.f.array(s.dict["Matrix"]); len(m) == 6 {
		var fm matrix
		for i := range fm {
			fm[i], _ = r.f.number(m[i])
		}
		ctm = fm.mul(ctm)
	}
	resources := r.f.dict(s.dict["Resources"])
	if resources == nil {
		resources = parent
	}
	r.run(data, resources, ctm, depth+1, active)
}

// page is a page of the file, with what it inherits from the page tree.
type page struct {
	d         dict
	resources dict
	rotate    int
}

func (f *file) pages() []page {
	var out []page
	seen := map[int]bool{}
	var walk func(v any, resources dict, rotate int, depth int)
	walk = func(v any, resources dict, rotate int, depth int) {
		if rf, ok := v.(ref); ok {
			if seen[rf.num] {
				return
			}
			seen[rf.num] = true
		}
		d := f.dict(v)
		if d == nil || depth > 64 || len(out) >= 100_000 {
			return
		}
		if res := f.dict(d["Resources"]); res != nil {
			resources = res
		}
		if rot, ok := f.number(d["Rotate"]); ok {
			rotate = int(rot)
		}
		kids := f.array(d["Kids"])
		if kids == nil && f.name(d["Type"]) != "Pages" {
			out = append(out, page{d, resources, rotate})
			return
		}
		for _, k := range kids {
			walk(k, resources, rotate, depth+1)
		}
	}
	walk(f.root()["Pages"], nil, 0, 0)
	return out
}

// read interprets a page and returns the text shown on it.
func (r *reader) read(p page) []chunk {
	var data []byte
	switch c := r.f.resolve(p.d["Contents"]).(type) {
	case *stream:
		data, _ = decodeStream(r.f, c)
	case []any:
		for _, v := range c {
			if s, ok := r.f.resolve(v).(*stream); ok {
				if part, err := decodeStream(r.f, s); err == nil {
					data = append(data, part...)
					data = append(data, '\n')
				}
			}
		}
	}
	// A page shown turned is read as it is shown.
	ctm := identity
	switch ((p.rotate % 360) + 360) % 360 {
	case 90:
		ctm = matrix{0, -1, 1, 0, 0, 0}
	case 180:
		ctm = matrix{-1, 0, 0, -1, 0, 0}
	case 270:
		ctm = matrix{0, 1, -1, 0, 0, 0}
	}
	r.chunks = nil
	r.run(data, p.resources, ctm, 0, map[int]bool{})
	return r.chunks
}

// privateOnly reports whether a text is nothing but characters of the
// private use area, where symbol fonts keep their signs.
func privateOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0xE000 || r > 0xF8FF {
			return false
		}
	}
	return true
}
