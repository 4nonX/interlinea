// Package sfnt reads TrueType and OpenType fonts (also from collections)
// as far as a PDF needs them: the glyph of each character, how wide it is,
// the font's metrics and names, substitutions for joining scripts, and a
// copy of the font with only the glyphs used (a subset) to embed.
package sfnt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Font is one font of a font file.
type Font struct {
	data   []byte
	tables map[string][]byte

	// Name is the PostScript name, without spaces.
	Name string
	// Family is the family name, such as "Noto Sans".
	Family     string
	UnitsPerEm int
	// Ascent and Descent (negative) are the line's extent above and below
	// the baseline, CapHeight the height of capitals, in font units.
	Ascent, Descent, LineGap, CapHeight int
	// Bounding box of all glyphs, in font units.
	XMin, YMin, XMax, YMax int
	ItalicAngle            float64
	Bold, Italic, Fixed    bool
	NumGlyphs              int
	// CFF tells the outlines are PostScript (CFF), not TrueType.
	CFF bool

	cmap     map[rune]uint16
	advances []uint16
	gsub     *gsub
	cff      *cffFont
}

// ErrNotFont is returned for data that is not a font.
var ErrNotFont = errors.New("sfnt: not a TrueType or OpenType font")

// Parse reads the first font of a font file or collection.
func Parse(data []byte) (*Font, error) { return ParseIndex(data, 0) }

// Count tells how many fonts a file holds: more than one in a collection.
func Count(data []byte) int {
	if len(data) >= 12 && string(data[:4]) == "ttcf" {
		return int(binary.BigEndian.Uint32(data[8:12]))
	}
	if len(data) >= 4 {
		return 1
	}
	return 0
}

// ParseIndex reads font i of a font file or collection.
func ParseIndex(data []byte, i int) (*Font, error) {
	if len(data) < 12 {
		return nil, ErrNotFont
	}
	off := 0
	if string(data[:4]) == "ttcf" {
		n := int(binary.BigEndian.Uint32(data[8:12]))
		if i < 0 || i >= n || 12+4*(i+1) > len(data) {
			return nil, fmt.Errorf("sfnt: no font %d in the collection", i)
		}
		off = int(binary.BigEndian.Uint32(data[12+4*i:]))
	}
	if off+12 > len(data) {
		return nil, ErrNotFont
	}
	switch v := binary.BigEndian.Uint32(data[off:]); v {
	case 0x00010000, 0x4F54544F, 0x74727565: // TrueType, "OTTO", "true"
	default:
		return nil, ErrNotFont
	}
	n := int(binary.BigEndian.Uint16(data[off+4:]))
	f := &Font{data: data, tables: map[string][]byte{}}
	for k := 0; k < n; k++ {
		rec := off + 12 + 16*k
		if rec+16 > len(data) {
			return nil, ErrNotFont
		}
		tag := string(data[rec : rec+4])
		start := int(binary.BigEndian.Uint32(data[rec+8:]))
		length := int(binary.BigEndian.Uint32(data[rec+12:]))
		if start < 0 || length < 0 || start+length > len(data) {
			return nil, fmt.Errorf("sfnt: table %s lies outside the file", tag)
		}
		f.tables[tag] = data[start : start+length]
	}
	if err := f.read(); err != nil {
		return nil, err
	}
	return f, nil
}

type reader struct {
	b   []byte
	bad bool
}

func (r *reader) u16(at int) int {
	if at < 0 || at+2 > len(r.b) {
		r.bad = true
		return 0
	}
	return int(binary.BigEndian.Uint16(r.b[at:]))
}

func (r *reader) i16(at int) int { return int(int16(r.u16(at))) }

func (r *reader) u32(at int) int {
	if at < 0 || at+4 > len(r.b) {
		r.bad = true
		return 0
	}
	return int(binary.BigEndian.Uint32(r.b[at:]))
}

func (f *Font) read() error {
	head, ok := f.tables["head"]
	if !ok || len(head) < 54 {
		return errors.New("sfnt: no head table")
	}
	h := reader{b: head}
	f.UnitsPerEm = h.u16(18)
	if f.UnitsPerEm == 0 {
		f.UnitsPerEm = 1000
	}
	f.XMin, f.YMin, f.XMax, f.YMax = h.i16(36), h.i16(38), h.i16(40), h.i16(42)
	style := h.u16(44)
	f.Bold = style&1 != 0
	f.Italic = style&2 != 0

	maxp := reader{b: f.tables["maxp"]}
	f.NumGlyphs = maxp.u16(4)
	if maxp.bad || f.NumGlyphs == 0 {
		return errors.New("sfnt: no maxp table")
	}

	hhea := reader{b: f.tables["hhea"]}
	f.Ascent, f.Descent, f.LineGap = hhea.i16(4), hhea.i16(6), hhea.i16(8)
	nh := hhea.u16(34)
	if hhea.bad || nh == 0 {
		return errors.New("sfnt: no hhea table")
	}
	hmtx := reader{b: f.tables["hmtx"]}
	f.advances = make([]uint16, f.NumGlyphs)
	last := 0
	for g := 0; g < f.NumGlyphs; g++ {
		if g < nh {
			last = hmtx.u16(4 * g)
		}
		f.advances[g] = uint16(last)
	}

	if os2, ok := f.tables["OS/2"]; ok {
		o := reader{b: os2}
		if o.u16(4) >= 700 {
			f.Bold = true
		}
		sel := o.u16(62)
		if sel&1 != 0 {
			f.Italic = true
		}
		if sel&32 != 0 {
			f.Bold = true
		}
		// Typographic metrics, when the font says to use them, else the
		// Windows ones fall back to hhea's.
		if a, d := o.i16(68), o.i16(70); !o.bad && sel&128 != 0 && a != 0 {
			f.Ascent, f.Descent, f.LineGap = a, d, o.i16(72)
		}
		if o.u16(0) >= 2 {
			f.CapHeight = o.i16(88)
		}
	}
	if f.CapHeight == 0 {
		f.CapHeight = f.Ascent * 7 / 10
	}
	if post, ok := f.tables["post"]; ok && len(post) >= 16 {
		p := reader{b: post}
		f.ItalicAngle = float64(p.i16(4)) + float64(p.u16(6))/65536
		f.Fixed = p.u32(12) != 0
	}
	f.readNames()
	if err := f.readCmap(); err != nil {
		return err
	}
	if c, ok := f.tables["CFF "]; ok {
		f.CFF = true
		cf, err := parseCFF(c)
		if err != nil {
			return err
		}
		f.cff = cf
	} else if _, ok := f.tables["glyf"]; !ok {
		return errors.New("sfnt: the font has neither TrueType nor CFF outlines")
	} else if _, ok := f.tables["loca"]; !ok {
		return errors.New("sfnt: no loca table")
	}
	f.gsub = parseGSUB(f.tables["GSUB"])
	return nil
}

func (f *Font) readNames() {
	name := reader{b: f.tables["name"]}
	count, strOff := name.u16(2), name.u16(4)
	get := func(id int) string {
		best := ""
		for i := 0; i < count; i++ {
			rec := 6 + 12*i
			pid, eid, nid := name.u16(rec), name.u16(rec+2), name.u16(rec+6)
			length, off := name.u16(rec+8), name.u16(rec+10)
			if nid != id || name.bad {
				continue
			}
			start := strOff + off
			if start+length > len(name.b) {
				continue
			}
			raw := name.b[start : start+length]
			switch {
			case pid == 3 || pid == 0:
				u := make([]uint16, len(raw)/2)
				for k := range u {
					u[k] = binary.BigEndian.Uint16(raw[2*k:])
				}
				s := string(utf16.Decode(u))
				if pid == 3 && (eid == 1 || eid == 10) {
					return s
				}
				best = s
			case pid == 1 && best == "":
				best = string(raw)
			}
		}
		return best
	}
	f.Name = strings.Map(func(r rune) rune {
		if r <= ' ' || r > '~' || strings.ContainsRune("[](){}<>/%", r) {
			return -1
		}
		return r
	}, get(6))
	f.Family = get(16)
	if f.Family == "" {
		f.Family = get(1)
	}
	if f.Name == "" {
		f.Name = strings.ReplaceAll(f.Family, " ", "")
	}
	if f.Name == "" {
		f.Name = "Font"
	}
}

func (f *Font) readCmap() error {
	f.cmap = map[rune]uint16{}
	if _, ok := f.tables["cmap"]; !ok {
		// A subset made to embed has none: a PDF names glyphs by number.
		return nil
	}
	c := reader{b: f.tables["cmap"]}
	n := c.u16(2)
	best, bestRank := -1, 0
	for i := 0; i < n; i++ {
		pid, eid, off := c.u16(4+8*i), c.u16(6+8*i), c.u32(8+8*i)
		format := c.u16(off)
		rank := 0
		switch {
		case format == 12 && (pid == 3 && eid == 10 || pid == 0 && (eid == 4 || eid == 6)):
			rank = 3
		case format == 4 && (pid == 3 && eid == 1 || pid == 0):
			rank = 2
		case format == 4 && pid == 3 && eid == 0: // symbol fonts
			rank = 1
		}
		if rank > bestRank {
			best, bestRank = off, rank
		}
	}
	if c.bad || best < 0 {
		return errors.New("sfnt: no Unicode cmap")
	}
	switch c.u16(best) {
	case 4:
		segs := c.u16(best+6) / 2
		ends := best + 14
		starts := ends + 2*segs + 2
		deltas := starts + 2*segs
		ranges := deltas + 2*segs
		for s := 0; s < segs; s++ {
			end, start := c.u16(ends+2*s), c.u16(starts+2*s)
			delta, ro := c.u16(deltas+2*s), c.u16(ranges+2*s)
			if start > end || c.bad {
				continue
			}
			for ch := start; ch <= end && ch != 0xFFFF; ch++ {
				var g int
				if ro == 0 {
					g = (ch + delta) & 0xFFFF
				} else {
					at := ranges + 2*s + ro + 2*(ch-start)
					g = c.u16(at)
					if g != 0 {
						g = (g + delta) & 0xFFFF
					}
				}
				if g != 0 && g < f.NumGlyphs {
					f.cmap[rune(ch)] = uint16(g)
				}
			}
		}
	case 12:
		groups := c.u32(best + 12)
		for k := 0; k < groups && !c.bad; k++ {
			at := best + 16 + 12*k
			start, end, g := c.u32(at), c.u32(at+4), c.u32(at+8)
			if end-start > 0x10FFFF || end < start {
				continue
			}
			for ch := start; ch <= end; ch++ {
				if gg := g + ch - start; gg < f.NumGlyphs && gg != 0 {
					f.cmap[rune(ch)] = uint16(gg)
				}
			}
		}
	}
	if c.bad {
		return errors.New("sfnt: the cmap table is cut short")
	}
	return nil
}

// Glyph is the glyph of a character, or 0 if the font has none.
func (f *Font) Glyph(r rune) uint16 { return f.cmap[r] }

// Has tells whether the font has a glyph for the character.
func (f *Font) Has(r rune) bool { _, ok := f.cmap[r]; return ok }

// Advance is how far a glyph moves the pen, in font units.
func (f *Font) Advance(g uint16) int {
	if int(g) < len(f.advances) {
		return int(f.advances[g])
	}
	return 0
}

// Runes lists the characters the font maps, for a ToUnicode table.
func (f *Font) Runes() map[rune]uint16 { return f.cmap }

// CID is the number a PDF uses for a glyph: the glyph's index, or in a
// CID-keyed CFF font the CID its charset gives it.
func (f *Font) CID(g uint16) uint16 {
	if f.cff != nil && f.cff.cidKeyed && int(g) < len(f.cff.cids) {
		return f.cff.cids[g]
	}
	return g
}

// CIDKeyed tells a CFF font is CID-keyed (as CJK fonts are).
func (f *Font) CIDKeyed() bool { return f.cff != nil && f.cff.cidKeyed }

// Substitute applies the font's substitutions of an OpenType feature
// (such as "init", "medi", "fina", "isol" or "rlig") for a script to a run
// of glyphs, and returns the run. ok reports the feature exists.
func (f *Font) Substitute(script, feature string, glyphs []uint16) ([]uint16, bool) {
	if f.gsub == nil {
		return glyphs, false
	}
	return f.gsub.apply(script, feature, glyphs)
}

// HasFeature tells whether the font has the substitutions of a feature
// for a script.
func (f *Font) HasFeature(script, feature string) bool {
	return f.gsub != nil && len(f.gsub.lookupsFor(script, feature)) > 0
}
