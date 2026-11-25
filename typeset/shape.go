package typeset

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// item is the smallest piece of a line: a character with the marks on it
// (or a ligature of two), set in one font.
type item struct {
	face   *Face
	glyphs []uint16
	width  float64 // in points, at the span's size
	text   string
	span   int
	space  bool
	// A line may break after it.
	breakAfter bool
	// Always break after it (a line break in the text).
	newline bool
	class   bidiClass
	level   int
}

type bidiClass int

const (
	bidiNeutral bidiClass = iota
	bidiL
	bidiR
	bidiNumber
)

func classOf(r rune) bidiClass {
	switch {
	case r >= 0x0590 && r <= 0x08FF, r >= 0xFB1D && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF, r >= 0x10800 && r <= 0x10FFF:
		if r >= 0x0660 && r <= 0x0669 || r >= 0x06F0 && r <= 0x06F9 {
			return bidiNumber
		}
		if unicode.Is(unicode.Mn, r) {
			return bidiNeutral
		}
		return bidiR
	case r >= '0' && r <= '9':
		return bidiNumber
	case unicode.IsLetter(r):
		return bidiL
	}
	return bidiNeutral
}

// RTL tells a language is written from right to left.
func RTL(lang string) bool {
	p := strings.ToLower(lang)
	if i := strings.IndexAny(p, "-_"); i >= 0 {
		p = p[:i]
	}
	switch p {
	case "ar", "he", "iw", "fa", "ur", "ps", "sd", "ug", "yi", "dv", "ckb", "syr":
		return true
	}
	return false
}

var mirrors = map[rune]rune{'(': ')', ')': '(', '[': ']', ']': '[', '{': '}', '}': '{', '<': '>', '>': '<', '«': '»', '»': '«', '‹': '›', '›': '‹'}

// Arabic joining: how a letter joins its neighbours.
type joining int

const (
	joinNone  joining = iota // does not join (hamza, Latin, space)
	joinRight                // joins the letter before it only (alef, dal, reh, waw)
	joinDual                 // joins both sides
	joinCause                // tatweel
	joinTransparent
)

func joiningOf(r rune) joining {
	switch {
	case r == 0x0640:
		return joinCause
	case unicode.Is(unicode.Mn, r) && r >= 0x0610 && r <= 0x08FF:
		return joinTransparent
	case r == 0x0621, r == 0x0674, r == 0x06DD:
		return joinNone
	case r >= 0x0622 && r <= 0x0625, r == 0x0627, r == 0x0629, r >= 0x062F && r <= 0x0632, r == 0x0648,
		r >= 0x0671 && r <= 0x0673, r >= 0x0675 && r <= 0x0677, r >= 0x0688 && r <= 0x0699, r == 0x06C0,
		r >= 0x06C3 && r <= 0x06CB, r == 0x06CD, r == 0x06CF, r == 0x06D2, r == 0x06D3, r == 0x06D5, r == 0x06EE, r == 0x06EF:
		return joinRight
	case r >= 0x0620 && r <= 0x064A, r >= 0x066E && r <= 0x06D3, r >= 0x06FA && r <= 0x06FC, r == 0x06FF, r >= 0x0750 && r <= 0x077F, r >= 0x08A0 && r <= 0x08C7:
		return joinDual
	}
	return joinNone
}

// The forms of a letter, isolated first, in the presentation forms, for
// fonts without substitutions.
var arabicForms = func() map[rune][]rune {
	m := map[rune][]rune{}
	add := func(base rune, first rune, n int) {
		f := make([]rune, n)
		for i := range f {
			f[i] = first + rune(i)
		}
		m[base] = f
	}
	seq := []struct {
		r rune
		n int
	}{{0x621, 1}, {0x622, 2}, {0x623, 2}, {0x624, 2}, {0x625, 2}, {0x626, 4}, {0x627, 2}, {0x628, 4}, {0x629, 2},
		{0x62A, 4}, {0x62B, 4}, {0x62C, 4}, {0x62D, 4}, {0x62E, 4}, {0x62F, 2}, {0x630, 2}, {0x631, 2}, {0x632, 2},
		{0x633, 4}, {0x634, 4}, {0x635, 4}, {0x636, 4}, {0x637, 4}, {0x638, 4}, {0x639, 4}, {0x63A, 4}, {0x641, 4},
		{0x642, 4}, {0x643, 4}, {0x644, 4}, {0x645, 4}, {0x646, 4}, {0x647, 4}, {0x648, 2}, {0x649, 2}, {0x64A, 4}}
	at := rune(0xFE80)
	for _, s := range seq {
		add(s.r, at, s.n)
		at += rune(s.n)
	}
	// Persian and Urdu letters, in the forms A block.
	add(0x067E, 0xFB56, 4) // peh
	add(0x0686, 0xFB7A, 4) // tcheh
	add(0x0698, 0xFB8A, 2) // jeh
	add(0x06A9, 0xFB8E, 4) // keheh
	add(0x06AF, 0xFB92, 4) // gaf
	add(0x06CC, 0xFBFC, 4) // farsi yeh
	return m
}()

const (
	formIsolated = iota
	formFinal
	formInitial
	formMedial
)

var formFeature = [...]string{"isol", "fina", "init", "medi"}

// arabicFormsOf works out the form of each character of a run of text:
// isolated, final, initial or medial, as its neighbours join it; -1 for
// characters that take no form.
func arabicFormsOf(rs []rune) []int {
	forms := make([]int, len(rs))
	// The letter next to i, skipping marks, or -1.
	near := func(i, step int) int {
		for k := i + step; k >= 0 && k < len(rs); k += step {
			if joiningOf(rs[k]) != joinTransparent {
				return k
			}
		}
		return -1
	}
	for i, r := range rs {
		j := joiningOf(r)
		if j == joinTransparent || j == joinNone {
			forms[i] = -1
			continue
		}
		p, n := near(i, -1), near(i, 1)
		joinsPrev := (j == joinRight || j == joinDual || j == joinCause) && p >= 0 && (joiningOf(rs[p]) == joinDual || joiningOf(rs[p]) == joinCause)
		joinsNext := (j == joinDual || j == joinCause) && n >= 0 && (joiningOf(rs[n]) == joinRight || joiningOf(rs[n]) == joinDual || joiningOf(rs[n]) == joinCause)
		switch {
		case joinsPrev && joinsNext:
			forms[i] = formMedial
		case joinsPrev:
			forms[i] = formFinal
		case joinsNext:
			forms[i] = formInitial
		default:
			forms[i] = formIsolated
		}
	}
	return forms
}

// shapeSpan turns the text of one span into items.
func (s *FontSet) shapeSpan(text string, st Style, size float64, span int) []item {
	rs := []rune(text)
	forms := arabicFormsOf(rs)
	var out []item
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\n' {
			out = append(out, item{text: "\n", span: span, newline: true, space: true, breakAfter: true})
			continue
		}
		if r == '\r' {
			continue
		}
		if r == '\t' {
			r = ' '
		}
		f := s.face(r, st)
		if f == nil {
			s.mu.Lock()
			s.missing[r] = true
			s.mu.Unlock()
			// The first font draws its .notdef box in place of it.
			if len(s.faces) == 0 {
				continue
			}
			f = s.faces[0]
		}
		it := item{face: f, span: span, text: string(r), class: classOf(r)}
		g := f.Font.Glyph(r)
		// Arabic letters take their joined form.
		if fm := forms[i]; fm >= 0 && r >= 0x0600 && r <= 0x08FF {
			// The presentation forms are whole glyphs; a font's own
			// substitutions may split letters into parts placed by rules
			// this does not apply, so they are the second choice.
			if alt, ok := arabicForms[r]; ok && fm < len(alt) && f.Font.Has(alt[fm]) {
				g = f.Font.Glyph(alt[fm])
			} else if fm != formIsolated {
				if ng, ok := f.Font.Substitute("arab", formFeature[fm], []uint16{g}); ok && len(ng) == 1 {
					g = ng[0]
				}
			}
			// Lam with alef is one glyph.
			if r == 0x0644 && i+1 < len(rs) && (rs[i+1] == 0x0622 || rs[i+1] == 0x0623 || rs[i+1] == 0x0625 || rs[i+1] == 0x0627) {
				na := rs[i+1]
				ga := f.Font.Glyph(na)
				if lig, ok := lamAlef(f, na, fm == formMedial || fm == formFinal); ok {
					it.glyphs = []uint16{lig}
					it.text = string([]rune{r, na})
					i++
				} else if lig, ok := f.Font.Substitute("arab", "rlig", []uint16{g, ga}); ok && len(lig) == 1 {
					it.glyphs = []uint16{lig[0]}
					it.text = string([]rune{r, na})
					i++
				}
			}
		}
		if it.glyphs == nil {
			it.glyphs = []uint16{g}
		}
		// Marks go with the character before them.
		for i+1 < len(rs) && unicode.In(rs[i+1], unicode.Mn, unicode.Me) {
			i++
			mg := f.Font.Glyph(rs[i])
			if mg == 0 {
				continue
			}
			it.glyphs = append(it.glyphs, mg)
			it.text += string(rs[i])
		}
		if r == ' ' || r == 0x3000 {
			it.space = true
			it.breakAfter = true
		}
		if r == '-' || r == '/' || r == 0x2010 || r == 0x2013 {
			it.breakAfter = true
		}
		if cjk(r) {
			it.breakAfter = true
			if i > 0 && len(out) > 0 && !cjk(rs[i-1]) && !out[len(out)-1].space {
				out[len(out)-1].breakAfter = true
			}
		}
		units := 0
		for _, gg := range it.glyphs {
			units += f.Font.Advance(gg)
		}
		it.width = float64(units) * size / float64(f.Font.UnitsPerEm)
		for k, gg := range it.glyphs {
			if k == 0 {
				f.use(gg, it.text)
			} else {
				f.use(gg, string(firstRune(it.text, k)))
			}
		}
		out = append(out, it)
	}
	return out
}

// firstRune gives the k-th character of s.
func firstRune(s string, k int) rune {
	for i, r := range []rune(s) {
		if i == k {
			return r
		}
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

func lamAlef(f *Face, alef rune, joined bool) (uint16, bool) {
	base := map[rune]rune{0x0622: 0xFEF5, 0x0623: 0xFEF7, 0x0625: 0xFEF9, 0x0627: 0xFEFB}[alef]
	if joined {
		base++
	}
	if base != 0 && f.Font.Has(base) {
		return f.Font.Glyph(base), true
	}
	return 0, false
}

func cjk(r rune) bool {
	return r >= 0x2E80 && r <= 0x9FFF || r >= 0xAC00 && r <= 0xD7AF || r >= 0xF900 && r <= 0xFAFF || r >= 0xFF00 && r <= 0xFFEF || r >= 0x20000 && r <= 0x3FFFF
}

// reorder puts a line's items in the order they are drawn, left to right,
// by the bidirectional levels of the paragraph (a simplified form of the
// Unicode bidirectional algorithm, enough for terms and short notes).
func reorder(items []item, rtl bool) []item {
	base := 0
	if rtl {
		base = 1
	}
	n := len(items)
	for i := range items {
		switch items[i].class {
		case bidiL:
			items[i].level = 0
			if rtl {
				items[i].level = 2
			}
		case bidiR:
			items[i].level = 1
		case bidiNumber:
			// Numbers read left to right, also in right-to-left text.
			if rtl {
				items[i].level = 2
			} else {
				items[i].level = 0
				// After right-to-left letters they stay in that run.
				for k := i - 1; k >= 0; k-- {
					if items[k].class == bidiR {
						items[i].level = 2
						break
					}
					if items[k].class == bidiL {
						break
					}
				}
			}
		default:
			items[i].level = -1
		}
	}
	// Neutrals between two of the same direction take it, others the
	// paragraph's.
	dir := func(l int) int {
		if l < 0 {
			return -1
		}
		return l % 2
	}
	for i := 0; i < n; i++ {
		if items[i].level >= 0 {
			continue
		}
		j := i
		for j < n && items[j].level < 0 {
			j++
		}
		before, after := base, base
		if i > 0 {
			before = dir(items[i-1].level)
		}
		if j < n {
			after = dir(items[j].level)
		}
		lv := base
		if before == after {
			lv = before
			if lv == 0 && rtl {
				lv = 2
			}
		}
		for k := i; k < j; k++ {
			items[k].level = lv
		}
		i = j - 1
	}
	maxL := 0
	for _, it := range items {
		if it.level > maxL {
			maxL = it.level
		}
	}
	out := append([]item(nil), items...)
	for l := maxL; l >= 1; l-- {
		for i := 0; i < n; {
			if out[i].level < l {
				i++
				continue
			}
			j := i
			for j < n && out[j].level >= l {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				out[a], out[b] = out[b], out[a]
			}
			i = j
		}
	}
	// Brackets face the other way in right-to-left runs.
	for i := range out {
		if out[i].level%2 == 1 {
			r, _ := utf8.DecodeRuneInString(out[i].text)
			if m, ok := mirrors[r]; ok && out[i].face != nil && out[i].face.Font.Has(m) {
				g := out[i].face.Font.Glyph(m)
				out[i].glyphs = []uint16{g}
				out[i].face.use(g, string(m))
			}
		}
	}
	return out
}
