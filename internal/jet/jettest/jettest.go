// Package jettest writes small Access databases of the Jet 4 format for
// tests: enough of the format for tables of numbers, yes/no values, text
// and memo fields, with long fields on pages of their own, rows that were
// deleted or moved, and encrypted pages. What it writes is read the same
// by mdbtools, which the tests of package jet were checked against.
package jettest

import (
	"crypto/rc4"
	"encoding/binary"
	"unicode/utf16"
)

const pageSize = 4096

// Column types, as in package jet.
const (
	Bool  = 1
	Int16 = 3
	Int32 = 4
	Text  = 10
	OLE   = 11
	Memo  = 12
)

// Column is a column of a table to write.
type Column struct {
	Name string
	Type int
}

// Table is a table to write. A row holds a value for each column: int for
// the numbers, bool, string, or nil for an empty field.
type Table struct {
	Name    string
	Columns []Column
	Rows    [][]any
	// Deleted and Moved name rows, by their position in Rows, to write as
	// a deleted row and as a row that outgrew its page and left a pointer
	// behind.
	Deleted map[int]bool
	Moved   map[int]bool
	// Swapped writes the marks of deleted and moved rows the other way
	// round. The descriptions of the format disagree on which is which,
	// so a reader has to cope with both.
	Swapped bool
}

type writer struct {
	pages [][]byte
	// open data page of each owner
	open map[uint32]int
	// the pages of each owner, for its usage map
	owned map[uint32][]int
}

func (w *writer) alloc(kind byte) int {
	p := make([]byte, pageSize)
	p[0], p[1] = kind, 1
	w.pages = append(w.pages, p)
	return len(w.pages) - 1
}

const lval = 0x4C41564C // "LVAL": the owner of pages that hold long fields

func (w *writer) dataPage(owner uint32) int {
	n := w.alloc(0x01)
	binary.LittleEndian.PutUint32(w.pages[n][4:], owner)
	w.open[owner] = n
	w.owned[owner] = append(w.owned[owner], n)
	return n
}

// put stores a row on a data page of the owner and returns its place as a
// pointer: the page in the upper three bytes, the row in the lowest.
func (w *writer) put(owner uint32, row []byte, flags uint16, fresh bool) uint32 {
	n, ok := w.open[owner]
	if !ok || fresh {
		n = w.dataPage(owner)
	}
	for {
		p := w.pages[n]
		rows := int(binary.LittleEndian.Uint16(p[12:]))
		low := pageSize
		if rows > 0 {
			low = int(binary.LittleEndian.Uint16(p[14+2*(rows-1):]) & 0x1FFF)
		}
		if low-len(row) >= 14+2*(rows+1) && rows < 250 {
			start := low - len(row)
			copy(p[start:], row)
			binary.LittleEndian.PutUint16(p[14+2*rows:], uint16(start)|flags)
			binary.LittleEndian.PutUint16(p[12:], uint16(rows+1))
			binary.LittleEndian.PutUint16(p[2:], uint16(start-(14+2*(rows+1))))
			return uint32(n)<<8 | uint32(rows)
		}
		n = w.dataPage(owner)
	}
}

func encodeText(s string) []byte {
	units := utf16.Encode([]rune(s))
	narrow := len(units) > 0
	for _, u := range units {
		if u == 0 || u > 0xFF {
			narrow = false
		}
	}
	if narrow {
		out := []byte{0xFF, 0xFE}
		for _, u := range units {
			out = append(out, byte(u))
		}
		return out
	}
	out := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2*i:], u)
	}
	return out
}

// long writes the field of a memo column: short text in the row, longer
// text on one other page, still longer text as a chain of pieces.
func (w *writer) long(data []byte) []byte {
	head := make([]byte, 12)
	head[0], head[1], head[2] = byte(len(data)), byte(len(data)>>8), byte(len(data)>>16)
	const inRow, piece = 64, 4000
	switch {
	case len(data) <= inRow:
		head[3] = 0x80
		return append(head, data...)
	case len(data) <= piece:
		head[3] = 0x40
		binary.LittleEndian.PutUint32(head[4:], w.put(lval, data, 0, false))
		return head
	}
	var next uint32
	for end := len(data); end > 0; {
		start := end - piece
		if rest := len(data) % piece; end == len(data) && rest != 0 {
			start = end - rest
		}
		part := make([]byte, 4, 4+end-start)
		binary.LittleEndian.PutUint32(part, next)
		next = w.put(lval, append(part, data[start:end]...), 0, false)
		end = start
	}
	binary.LittleEndian.PutUint32(head[4:], next)
	return head
}

func fixedSize(typ int) int {
	switch typ {
	case Int16:
		return 2
	case Int32:
		return 4
	}
	return 0
}

func isFixed(typ int) bool { return typ == Bool || typ == Int16 || typ == Int32 }

func (w *writer) row(t Table, values []any) []byte {
	cols := len(t.Columns)
	mask := make([]byte, (cols+7)/8)
	row := make([]byte, 2)
	binary.LittleEndian.PutUint16(row, uint16(cols))
	for i, c := range t.Columns {
		v := values[i]
		switch c.Type {
		case Bool:
			if b, _ := v.(bool); b {
				mask[i/8] |= 1 << (i % 8)
			}
		case Int16, Int32:
			field := make([]byte, fixedSize(c.Type))
			if n, ok := v.(int); ok {
				mask[i/8] |= 1 << (i % 8)
				if c.Type == Int16 {
					binary.LittleEndian.PutUint16(field, uint16(int16(n)))
				} else {
					binary.LittleEndian.PutUint32(field, uint32(int32(n)))
				}
			}
			row = append(row, field...)
		}
	}
	var offsets []int
	for i, c := range t.Columns {
		if isFixed(c.Type) {
			continue
		}
		offsets = append(offsets, len(row))
		s, ok := values[i].(string)
		if !ok {
			continue
		}
		mask[i/8] |= 1 << (i % 8)
		if c.Type == Text {
			row = append(row, encodeText(s)...)
		} else {
			row = append(row, w.long(encodeText(s))...)
		}
	}
	if len(offsets) > 0 {
		offsets = append(offsets, len(row))
		for i := len(offsets) - 1; i >= 0; i-- {
			row = binary.LittleEndian.AppendUint16(row, uint16(offsets[i]))
		}
		row = binary.LittleEndian.AppendUint16(row, uint16(len(offsets)-1))
	}
	return append(row, mask...)
}

// definition writes a table's definition page.
func (w *writer) definition(page int, t Table, system bool, usedMap, freeMap uint32) {
	p := w.pages[page]
	def := make([]byte, 63)
	def[0], def[1] = 0x02, 0x01
	rows := 0
	for i := range t.Rows {
		if !t.Deleted[i] {
			rows++
		}
	}
	binary.LittleEndian.PutUint32(def[16:], uint32(rows))
	def[40] = 0x4E
	if system {
		def[40] = 0x53
	}
	vars := 0
	for _, c := range t.Columns {
		if !isFixed(c.Type) {
			vars++
		}
	}
	binary.LittleEndian.PutUint16(def[41:], uint16(len(t.Columns)))
	binary.LittleEndian.PutUint16(def[43:], uint16(vars))
	binary.LittleEndian.PutUint16(def[45:], uint16(len(t.Columns)))
	binary.LittleEndian.PutUint32(def[55:], usedMap)
	binary.LittleEndian.PutUint32(def[59:], freeMap)
	varNum, offset := 0, 0
	for i, c := range t.Columns {
		col := make([]byte, 25)
		col[0] = byte(c.Type)
		binary.LittleEndian.PutUint16(col[5:], uint16(i))
		binary.LittleEndian.PutUint16(col[9:], uint16(i))
		col[15] = 0x02
		if isFixed(c.Type) {
			col[15] |= 0x01
			binary.LittleEndian.PutUint16(col[21:], uint16(offset))
			binary.LittleEndian.PutUint16(col[23:], uint16(fixedSize(c.Type)))
			offset += fixedSize(c.Type)
		} else {
			binary.LittleEndian.PutUint16(col[7:], uint16(varNum))
			varNum++
			if c.Type == Text {
				col[11], col[12] = 0x09, 0x04
				col[16] = 0x01
				binary.LittleEndian.PutUint16(col[23:], 510)
			}
			if c.Type == Memo {
				col[11], col[12] = 0x09, 0x04
				col[16] = 0x01
			}
		}
		def = append(def, col...)
	}
	for _, c := range t.Columns {
		name := utf16.Encode([]rune(c.Name))
		def = binary.LittleEndian.AppendUint16(def, uint16(2*len(name)))
		for _, u := range name {
			def = binary.LittleEndian.AppendUint16(def, u)
		}
	}
	binary.LittleEndian.PutUint32(def[8:], uint32(len(def)-8))
	if len(def) > pageSize {
		panic("jettest: the table has too many columns for one definition page")
	}
	copy(p, def)
}

// Build writes a database with the tables. A key other than zero encrypts
// its pages, as Access does for a database saved "encoded".
func Build(tables []Table, key uint32) []byte {
	w := &writer{open: map[uint32]int{}, owned: map[uint32][]int{}}
	head := w.alloc(0x00)
	copy(w.pages[head][4:], "Standard Jet DB\x00")
	binary.LittleEndian.PutUint32(w.pages[head][0x14:], 1)
	w.alloc(0x05) // the map of the pages in use, which nothing here reads
	const catalogPage = 2
	w.alloc(0x02)

	catalog := Table{Name: "MSysObjects", Columns: []Column{
		{"Id", Int32}, {"ParentId", Int32}, {"Name", Text}, {"Type", Int16}, {"Flags", Int32}, {"LvProp", OLE},
	}}
	catalog.Rows = append(catalog.Rows, []any{catalogPage, 0, "MSysObjects", 1, -2147483648, nil})
	defs := make([]int, len(tables))
	for i, t := range tables {
		defs[i] = w.alloc(0x02)
		catalog.Rows = append(catalog.Rows, []any{defs[i], 0, t.Name, 1, 0, nil})
	}

	// The maps of each table's pages lie on a data page of their own and
	// are filled in once the pages are known.
	const mapOwner, mapBytes = 1, 69
	maps := map[int]uint32{}
	all := append([]Table{catalog}, tables...)
	pagesOf := append([]int{catalogPage}, defs...)
	free := w.put(mapOwner, make([]byte, mapBytes), 0, false)
	for _, def := range pagesOf {
		maps[def] = w.put(mapOwner, make([]byte, mapBytes), 0, false)
	}

	for i, t := range all {
		owner := uint32(pagesOf[i])
		deleted, pointer := uint16(0x8000), uint16(0x4000)
		if t.Swapped {
			deleted, pointer = pointer, deleted
		}
		for r, values := range t.Rows {
			row := w.row(t, values)
			switch {
			case t.Deleted[r]:
				w.put(owner, row, deleted, false)
			case t.Moved[r]:
				// The pointer stays where the row was; the row itself
				// goes to a page of its own.
				at := w.open[owner]
				to := w.put(owner, row, 0, true)
				ptr := make([]byte, 4)
				binary.LittleEndian.PutUint32(ptr, to)
				if at == 0 {
					at = w.dataPage(owner)
				}
				w.open[owner] = at
				w.put(owner, ptr, pointer, false)
			default:
				w.put(owner, row, 0, false)
			}
		}
	}
	for i, t := range all {
		def := pagesOf[i]
		m := w.pages[maps[def]>>8]
		at, _ := slot(m, int(maps[def]&0xFF))
		for _, pg := range w.owned[uint32(def)] {
			m[at+5+pg/8] |= 1 << (pg % 8)
		}
		w.definition(def, t, i == 0, maps[def], free)
	}

	// The key lies in a part of the first page that is scrambled with a
	// key of its own.
	area := make([]byte, 128)
	binary.LittleEndian.PutUint32(area[0x3E-0x18:], key)
	c, _ := rc4.NewCipher([]byte{0xC7, 0xDA, 0x39, 0x6B})
	c.XORKeyStream(area, area)
	copy(w.pages[head][0x18:], area)

	out := make([]byte, 0, len(w.pages)*pageSize)
	for n, p := range w.pages {
		if key != 0 && n > 0 {
			var k [4]byte
			binary.LittleEndian.PutUint32(k[:], key^uint32(n))
			c, _ := rc4.NewCipher(k[:])
			c.XORKeyStream(p, p)
		}
		out = append(out, p...)
	}
	return out
}

func slot(p []byte, row int) (start, end int) {
	start = int(binary.LittleEndian.Uint16(p[14+2*row:]) & 0x1FFF)
	end = pageSize
	if row > 0 {
		end = int(binary.LittleEndian.Uint16(p[14+2*(row-1):]) & 0x1FFF)
	}
	return start, end
}
