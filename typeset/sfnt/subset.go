package sfnt

import (
	"bytes"
	"encoding/binary"
	"sort"
)

// Subset gives the font with only the glyphs used drawn, to embed in a
// PDF: a TrueType font file (for FontFile2), or for CFF outlines the CFF
// data (for FontFile3 /CIDFontType0C). Glyph numbers stay as they are, so
// the glyphs a page names are found in the subset.
func (f *Font) Subset(used map[uint16]bool) []byte {
	keep := map[uint16]bool{0: true}
	for g := range used {
		if int(g) < f.NumGlyphs {
			keep[g] = true
		}
	}
	if f.cff != nil {
		return f.cff.subset(keep)
	}
	return f.subsetTrueType(keep)
}

func (f *Font) glyphData(g int) []byte {
	loca := reader{b: f.tables["loca"]}
	glyf := f.tables["glyf"]
	head := reader{b: f.tables["head"]}
	long := head.u16(50) == 1
	var s, e int
	if long {
		s, e = loca.u32(4*g), loca.u32(4*g+4)
	} else {
		s, e = 2*loca.u16(2*g), 2*loca.u16(2*g+2)
	}
	if loca.bad || s > e || e > len(glyf) {
		return nil
	}
	return glyf[s:e]
}

func (f *Font) subsetTrueType(keep map[uint16]bool) []byte {
	// Composite glyphs bring their parts.
	queue := make([]uint16, 0, len(keep))
	for g := range keep {
		queue = append(queue, g)
	}
	for len(queue) > 0 {
		g := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		d := f.glyphData(int(g))
		if len(d) < 10 || int16(binary.BigEndian.Uint16(d)) >= 0 {
			continue
		}
		r := reader{b: d}
		at := 10
		for {
			flags, comp := r.u16(at), r.u16(at+2)
			if r.bad {
				break
			}
			if !keep[uint16(comp)] && comp < f.NumGlyphs {
				keep[uint16(comp)] = true
				queue = append(queue, uint16(comp))
			}
			at += 4
			if flags&1 != 0 {
				at += 4
			} else {
				at += 2
			}
			switch {
			case flags&8 != 0:
				at += 2
			case flags&0x40 != 0:
				at += 4
			case flags&0x80 != 0:
				at += 8
			}
			if flags&0x20 == 0 {
				break
			}
		}
	}

	var glyf bytes.Buffer
	loca := make([]byte, 4*(f.NumGlyphs+1))
	for g := 0; g < f.NumGlyphs; g++ {
		binary.BigEndian.PutUint32(loca[4*g:], uint32(glyf.Len()))
		if keep[uint16(g)] {
			glyf.Write(f.glyphData(g))
			for glyf.Len()%4 != 0 {
				glyf.WriteByte(0)
			}
		}
	}
	binary.BigEndian.PutUint32(loca[4*f.NumGlyphs:], uint32(glyf.Len()))

	head := append([]byte(nil), f.tables["head"]...)
	binary.BigEndian.PutUint16(head[50:], 1) // long offsets
	binary.BigEndian.PutUint32(head[8:], 0)  // checksum adjustment, set below
	tables := map[string][]byte{
		"head": head,
		"hhea": f.tables["hhea"],
		"maxp": f.tables["maxp"],
		"hmtx": f.tables["hmtx"],
		"loca": loca,
		"glyf": glyf.Bytes(),
	}
	for _, t := range []string{"cvt ", "fpgm", "prep"} {
		if b, ok := f.tables[t]; ok {
			tables[t] = b
		}
	}
	out := writeSFNT(tables)
	// The whole font's checksum makes head's adjustment.
	sum := checksum(out)
	headAt := bytes.Index(out[:12+16*len(tables)], []byte("head"))
	off := binary.BigEndian.Uint32(out[headAt+8:])
	binary.BigEndian.PutUint32(out[off+8:], 0xB1B0AFBA-sum)
	return out
}

func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}

func writeSFNT(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	entry, pow := 0, 1
	for pow*2 <= n {
		pow *= 2
		entry++
	}
	var out bytes.Buffer
	binary.Write(&out, binary.BigEndian, []uint16{1, 0, uint16(n), uint16(pow * 16), uint16(entry), uint16(n*16 - pow*16)})
	off := 12 + 16*n
	for _, t := range tags {
		b := tables[t]
		out.WriteString(t)
		binary.Write(&out, binary.BigEndian, []uint32{checksum(b), uint32(off), uint32(len(b))})
		off += (len(b) + 3) &^ 3
	}
	for _, t := range tags {
		b := tables[t]
		out.Write(b)
		for k := len(b); k%4 != 0; k++ {
			out.WriteByte(0)
		}
	}
	return out.Bytes()
}
