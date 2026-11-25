package sfnt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// A CFF font (PostScript outlines, as OpenType fonts with "OTTO" and CJK
// fonts have them), read as far as subsetting needs it.
type cffFont struct {
	data       []byte
	header     []byte
	names      []byte // the Name INDEX as it is
	topRaw     []dictEntry
	strings    []byte // the String INDEX as it is
	gsubrs     []byte // the Global Subr INDEX as it is
	charsets   []byte // the charset as it is
	fdselect   []byte // the FDSelect as it is, CID-keyed only
	charstr    [][]byte
	cidKeyed   bool
	cids       []uint16 // glyph to CID, CID-keyed only
	privates   []cffPrivate
	fdArrayRaw [][]dictEntry // font DICTs, CID-keyed only
}

type cffPrivate struct {
	dict  []dictEntry
	subrs []byte // the local Subr INDEX as it is, or nil
}

type dictEntry struct {
	op       int // 0..21, or 1200+n for 12 n
	operands []byte
	values   []float64
}

func readIndex(b []byte, at int) (items [][]byte, end int, err error) {
	if at+2 > len(b) {
		return nil, 0, errors.New("cff: INDEX outside the table")
	}
	count := int(binary.BigEndian.Uint16(b[at:]))
	if count == 0 {
		return nil, at + 2, nil
	}
	if at+3 > len(b) {
		return nil, 0, errors.New("cff: INDEX outside the table")
	}
	os := int(b[at+2])
	if os < 1 || os > 4 {
		return nil, 0, errors.New("cff: bad INDEX offset size")
	}
	offs := at + 3
	base := offs + (count+1)*os - 1
	off := func(i int) int {
		v := 0
		for k := 0; k < os; k++ {
			v = v<<8 | int(b[offs+i*os+k])
		}
		return v
	}
	if offs+(count+1)*os > len(b) {
		return nil, 0, errors.New("cff: INDEX outside the table")
	}
	items = make([][]byte, count)
	for i := 0; i < count; i++ {
		s, e := base+off(i), base+off(i+1)
		if s > e || e > len(b) {
			return nil, 0, errors.New("cff: INDEX item outside the table")
		}
		items[i] = b[s:e]
	}
	return items, base + off(count), nil
}

func writeIndex(items [][]byte) []byte {
	var out bytes.Buffer
	binary.Write(&out, binary.BigEndian, uint16(len(items)))
	if len(items) == 0 {
		return out.Bytes()
	}
	total := 1
	for _, it := range items {
		total += len(it)
	}
	os := 1
	for total >= 1<<(8*os) {
		os++
	}
	out.WriteByte(byte(os))
	off := 1
	put := func(v int) {
		for k := os - 1; k >= 0; k-- {
			out.WriteByte(byte(v >> (8 * k)))
		}
	}
	put(off)
	for _, it := range items {
		off += len(it)
		put(off)
	}
	for _, it := range items {
		out.Write(it)
	}
	return out.Bytes()
}

func parseDict(b []byte) ([]dictEntry, error) {
	var out []dictEntry
	start := 0
	var vals []float64
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c <= 21:
			op := int(c)
			i++
			if c == 12 {
				if i >= len(b) {
					return nil, errors.New("cff: DICT cut short")
				}
				op = 1200 + int(b[i])
				i++
			}
			opEnd := i - 1
			if op >= 1200 {
				opEnd--
			}
			out = append(out, dictEntry{op: op, operands: b[start:opEnd], values: vals})
			start, vals = i, nil
		case c == 28:
			if i+3 > len(b) {
				return nil, errors.New("cff: DICT cut short")
			}
			vals = append(vals, float64(int16(binary.BigEndian.Uint16(b[i+1:]))))
			i += 3
		case c == 29:
			if i+5 > len(b) {
				return nil, errors.New("cff: DICT cut short")
			}
			vals = append(vals, float64(int32(binary.BigEndian.Uint32(b[i+1:]))))
			i += 5
		case c == 30:
			i++
			for i < len(b) {
				x := b[i]
				i++
				if x&0x0f == 0x0f || x>>4 == 0x0f {
					break
				}
			}
			vals = append(vals, 0) // reals are only copied, never used
		case c >= 32 && c <= 246:
			vals = append(vals, float64(int(c)-139))
			i++
		case c >= 247 && c <= 250:
			if i+2 > len(b) {
				return nil, errors.New("cff: DICT cut short")
			}
			vals = append(vals, float64((int(c)-247)*256+int(b[i+1])+108))
			i += 2
		case c >= 251 && c <= 254:
			if i+2 > len(b) {
				return nil, errors.New("cff: DICT cut short")
			}
			vals = append(vals, float64(-(int(c)-251)*256-int(b[i+1])-108))
			i += 2
		default:
			return nil, fmt.Errorf("cff: bad DICT byte %d", c)
		}
	}
	return out, nil
}

func find(d []dictEntry, op int) []float64 {
	for _, e := range d {
		if e.op == op {
			return e.values
		}
	}
	return nil
}

func parseCFF(b []byte) (*cffFont, error) {
	if len(b) < 4 {
		return nil, errors.New("cff: table cut short")
	}
	c := &cffFont{data: b, header: b[:4]}
	hdr := int(b[2])
	_, end, err := readIndex(b, hdr)
	if err != nil {
		return nil, err
	}
	c.names = b[hdr:end]
	tops, end2, err := readIndex(b, end)
	if err != nil || len(tops) == 0 {
		return nil, errors.New("cff: no Top DICT")
	}
	_, end3, err := readIndex(b, end2)
	if err != nil {
		return nil, err
	}
	c.strings = b[end2:end3]
	_, end4, err := readIndex(b, end3)
	if err != nil {
		return nil, err
	}
	c.gsubrs = b[end3:end4]
	if c.topRaw, err = parseDict(tops[0]); err != nil {
		return nil, err
	}
	cs := find(c.topRaw, 17)
	if len(cs) == 0 {
		return nil, errors.New("cff: no CharStrings")
	}
	if c.charstr, _, err = readIndex(b, int(cs[0])); err != nil {
		return nil, err
	}
	n := len(c.charstr)
	c.cidKeyed = find(c.topRaw, 1230) != nil
	if v := find(c.topRaw, 15); len(v) == 1 && v[0] > 2 {
		at := int(v[0])
		size, cids, err := readCharset(b, at, n)
		if err != nil {
			return nil, err
		}
		c.charsets = b[at : at+size]
		c.cids = cids
	} else if c.cidKeyed {
		return nil, errors.New("cff: a CID-keyed font without a charset")
	}
	readPrivate := func(d []dictEntry) (cffPrivate, error) {
		v := find(d, 18)
		if len(v) != 2 {
			return cffPrivate{}, nil
		}
		size, off := int(v[0]), int(v[1])
		if off < 0 || off+size > len(b) {
			return cffPrivate{}, errors.New("cff: Private DICT outside the table")
		}
		pd, err := parseDict(b[off : off+size])
		if err != nil {
			return cffPrivate{}, err
		}
		p := cffPrivate{dict: pd}
		if s := find(pd, 19); len(s) == 1 {
			_, e, err := readIndex(b, off+int(s[0]))
			if err != nil {
				return cffPrivate{}, err
			}
			p.subrs = b[off+int(s[0]) : e]
		}
		return p, nil
	}
	if c.cidKeyed {
		fda := find(c.topRaw, 1236)
		fds := find(c.topRaw, 1237)
		if len(fda) != 1 || len(fds) != 1 {
			return nil, errors.New("cff: a CID-keyed font without FDArray or FDSelect")
		}
		dicts, _, err := readIndex(b, int(fda[0]))
		if err != nil {
			return nil, err
		}
		for _, raw := range dicts {
			d, err := parseDict(raw)
			if err != nil {
				return nil, err
			}
			p, err := readPrivate(d)
			if err != nil {
				return nil, err
			}
			c.fdArrayRaw = append(c.fdArrayRaw, d)
			c.privates = append(c.privates, p)
		}
		at := int(fds[0])
		size, err := fdSelectSize(b, at, n)
		if err != nil {
			return nil, err
		}
		c.fdselect = b[at : at+size]
	} else {
		p, err := readPrivate(c.topRaw)
		if err != nil {
			return nil, err
		}
		c.privates = []cffPrivate{p}
	}
	return c, nil
}

func readCharset(b []byte, at, n int) (int, []uint16, error) {
	if at >= len(b) {
		return 0, nil, errors.New("cff: charset outside the table")
	}
	cids := make([]uint16, n)
	g := 1
	p := at + 1
	get := func(k int) int {
		if p+k > len(b) {
			return -1
		}
		v := 0
		for i := 0; i < k; i++ {
			v = v<<8 | int(b[p+i])
		}
		p += k
		return v
	}
	switch b[at] {
	case 0:
		for ; g < n; g++ {
			v := get(2)
			if v < 0 {
				return 0, nil, errors.New("cff: charset cut short")
			}
			cids[g] = uint16(v)
		}
	case 1, 2:
		wide := 1
		if b[at] == 2 {
			wide = 2
		}
		for g < n {
			first, left := get(2), get(wide)
			if first < 0 || left < 0 {
				return 0, nil, errors.New("cff: charset cut short")
			}
			for k := 0; k <= left && g < n; k++ {
				cids[g] = uint16(first + k)
				g++
			}
		}
	default:
		return 0, nil, errors.New("cff: unknown charset format")
	}
	return p - at, cids, nil
}

func fdSelectSize(b []byte, at, n int) (int, error) {
	if at >= len(b) {
		return 0, errors.New("cff: FDSelect outside the table")
	}
	switch b[at] {
	case 0:
		return 1 + n, nil
	case 3:
		if at+3 > len(b) {
			return 0, errors.New("cff: FDSelect cut short")
		}
		ranges := int(binary.BigEndian.Uint16(b[at+1:]))
		return 3 + 3*ranges + 2, nil
	}
	return 0, errors.New("cff: unknown FDSelect format")
}

// int5 writes a number as a DICT operand of five bytes, so offsets can be
// worked out before the parts they point at are placed.
func int5(v int) []byte {
	out := []byte{29, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(out[1:], uint32(int32(v)))
	return out
}

func writeDict(d []dictEntry, skip map[int]bool, extra [][2]any) []byte {
	var out bytes.Buffer
	opBytes := func(op int) []byte {
		if op >= 1200 {
			return []byte{12, byte(op - 1200)}
		}
		return []byte{byte(op)}
	}
	// ROS comes first in a CID-keyed font's Top DICT.
	for _, e := range d {
		if e.op == 1230 {
			out.Write(e.operands)
			out.Write(opBytes(e.op))
		}
	}
	for _, e := range d {
		if skip[e.op] || e.op == 1230 {
			continue
		}
		out.Write(e.operands)
		out.Write(opBytes(e.op))
	}
	for _, x := range extra {
		for _, v := range x[1].([]int) {
			out.Write(int5(v))
		}
		out.Write(opBytes(x[0].(int)))
	}
	return out.Bytes()
}

// subset writes the CFF data again with only the glyphs kept drawn; the
// others are empty, so glyph numbers and CIDs stay as they are.
func (c *cffFont) subset(keep map[uint16]bool) []byte {
	strs := make([][]byte, len(c.charstr))
	for g := range c.charstr {
		if g == 0 || keep[uint16(g)] {
			strs[g] = c.charstr[g]
		} else {
			strs[g] = []byte{14} // endchar
		}
	}
	charstrings := writeIndex(strs)
	skip := map[int]bool{15: true, 16: true, 17: true, 18: true, 1236: true, 1237: true}

	// Private DICTs, each followed by its Subrs.
	privates := make([][]byte, len(c.privates))
	for i, p := range c.privates {
		pskip := map[int]bool{19: true}
		if p.subrs == nil {
			privates[i] = writeDict(p.dict, pskip, nil)
			continue
		}
		// The Subrs offset is from the Private DICT's start: its own size.
		probe := writeDict(p.dict, pskip, [][2]any{{19, []int{0}}})
		privates[i] = writeDict(p.dict, pskip, [][2]any{{19, []int{len(probe)}}})
	}

	topExtra := func(charset, charstr, priv, fdarray, fdsel int) [][2]any {
		x := [][2]any{}
		if c.charsets != nil {
			x = append(x, [2]any{15, []int{charset}})
		}
		x = append(x, [2]any{17, []int{charstr}})
		if c.cidKeyed {
			x = append(x, [2]any{1236, []int{fdarray}}, [2]any{1237, []int{fdsel}})
		} else if len(privates) == 1 && len(c.privates[0].dict) > 0 {
			x = append(x, [2]any{18, []int{len(privates[0]), priv}})
		}
		return x
	}
	topLen := len(writeDict(c.topRaw, skip, topExtra(0, 0, 0, 0, 0)))
	topIndexLen := len(writeIndex([][]byte{make([]byte, topLen)}))
	at := len(c.header) + len(c.names) + topIndexLen + len(c.strings) + len(c.gsubrs)
	charsetAt := at
	at += len(c.charsets)
	fdselAt := at
	at += len(c.fdselect)
	charstrAt := at
	at += len(charstrings)

	var fdarray []byte
	fdarrayAt := at
	privAt := 0
	if c.cidKeyed {
		// Font DICTs point at Private DICTs placed after the FDArray.
		fdSkip := map[int]bool{18: true}
		sizes := make([][]byte, len(c.fdArrayRaw))
		for i, d := range c.fdArrayRaw {
			sizes[i] = writeDict(d, fdSkip, [][2]any{{18, []int{0, 0}}})
		}
		fdLen := len(writeIndex(sizes))
		pAt := fdarrayAt + fdLen
		dicts := make([][]byte, len(c.fdArrayRaw))
		for i, d := range c.fdArrayRaw {
			dicts[i] = writeDict(d, fdSkip, [][2]any{{18, []int{len(privates[i]), pAt}}})
			pAt += len(privates[i]) + len(c.privates[i].subrs)
		}
		fdarray = writeIndex(dicts)
	} else {
		privAt = at
	}
	top := writeDict(c.topRaw, skip, topExtra(charsetAt, charstrAt, privAt, fdarrayAt, fdselAt))

	var out bytes.Buffer
	out.Write([]byte{c.header[0], c.header[1], 4, c.header[3]})
	out.Write(c.names)
	out.Write(writeIndex([][]byte{top}))
	out.Write(c.strings)
	out.Write(c.gsubrs)
	out.Write(c.charsets)
	out.Write(c.fdselect)
	out.Write(charstrings)
	out.Write(fdarray)
	for i := range privates {
		out.Write(privates[i])
		out.Write(c.privates[i].subrs)
	}
	return out.Bytes()
}
