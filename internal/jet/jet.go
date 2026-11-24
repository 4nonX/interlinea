// Package jet reads tables from a Microsoft Access database of the Jet 4
// format (.mdb), which is what a Trados MultiTerm termbase (.sdltb) is. It
// reads only: whole tables, row by row, with text, numbers and memo
// fields. Indexes, queries and relationships are not looked at.
//
// The format is not published. This follows the descriptions the mdbtools
// and Jackcess projects worked out.
package jet

import (
	"bytes"
	"crypto/rc4"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

const pageSize = 4096

// ErrFormat means the data is not a Jet 4 database this package reads.
var ErrFormat = errors.New("not an Access database of the Jet 4 format")

// Column types, as the table definition numbers them.
const (
	Bool     = 1
	Byte     = 2
	Int16    = 3
	Int32    = 4
	Money    = 5
	Float32  = 6
	Float64  = 7
	DateTime = 8
	Binary   = 9
	Text     = 10
	OLE      = 11
	Memo     = 12
	GUID     = 15
	Numeric  = 16
)

// DB is an open database.
type DB struct {
	data []byte
	// key is the database's page key; zero when its pages are not encrypted.
	key    uint32
	tables map[string]int // lower-case name to definition page
	names  []string
	pages  map[int][]byte // decrypted pages
}

// Column describes a column of a table.
type Column struct {
	Name string
	Type int
	// position of the column in a row
	num, varNum int
	fixedOffset int
	size        int
	fixed       bool
	compressed  bool
}

// Table is a table of the database.
type Table struct {
	db      *DB
	Name    string
	Columns []Column
	defPage int
	varCols int
}

// IsDatabase reports whether data starts as a Jet database does.
func IsDatabase(data []byte) bool {
	return len(data) > 0x20 && bytes.HasPrefix(data[4:], []byte("Standard Jet DB\x00"))
}

// Open reads the catalog of a database held in memory.
func Open(data []byte) (*DB, error) {
	if !IsDatabase(data) || len(data) < 3*pageSize {
		return nil, ErrFormat
	}
	if v := binary.LittleEndian.Uint32(data[0x14:]); v != 1 {
		if v == 0 {
			return nil, fmt.Errorf("%w: it is of the older Jet 3 format (Access 97)", ErrFormat)
		}
		return nil, ErrFormat
	}
	db := &DB{data: data, tables: map[string]int{}, pages: map[int][]byte{}}
	// Part of the first page is scrambled with a fixed key; it holds the
	// key the other pages are encrypted with, if they are.
	head := make([]byte, 128)
	copy(head, data[0x18:0x18+128])
	c, _ := rc4.NewCipher([]byte{0xC7, 0xDA, 0x39, 0x6B})
	c.XORKeyStream(head, head)
	db.key = binary.LittleEndian.Uint32(head[0x3E-0x18:])

	// The catalog is the table whose definition is on page 2.
	catalog, err := db.table("MSysObjects", 2)
	if err != nil {
		return nil, fmt.Errorf("reading the catalog: %w", err)
	}
	err = catalog.Rows(func(r Row) error {
		typ, _ := r["Type"].(int64)
		name, _ := r["Name"].(string)
		id, _ := r["Id"].(int64)
		flags, _ := r["Flags"].(int64)
		// Type 1 is a table of the database itself; system tables are
		// flagged.
		if typ&0x7f != 1 || name == "" || flags&0x80000002 != 0 {
			return nil
		}
		db.tables[strings.ToLower(name)] = int(id & 0x00FFFFFF)
		db.names = append(db.names, name)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading the catalog: %w", err)
	}
	return db, nil
}

// Tables lists the database's own tables.
func (db *DB) Tables() []string { return append([]string(nil), db.names...) }

// Table opens a table by name, whatever its case.
func (db *DB) Table(name string) (*Table, error) {
	pg, ok := db.tables[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("no table %q", name)
	}
	return db.table(name, pg)
}

func (db *DB) page(n int) ([]byte, error) {
	if p, ok := db.pages[n]; ok {
		return p, nil
	}
	if n < 0 || (n+1)*pageSize > len(db.data) {
		return nil, fmt.Errorf("page %d is beyond the end of the file", n)
	}
	p := db.data[n*pageSize : (n+1)*pageSize]
	if db.key != 0 && n > 0 {
		var k [4]byte
		binary.LittleEndian.PutUint32(k[:], db.key^uint32(n))
		c, _ := rc4.NewCipher(k[:])
		out := make([]byte, pageSize)
		c.XORKeyStream(out, p)
		p = out
		db.pages[n] = p
	}
	return p, nil
}

func u16(b []byte, at int) int { return int(binary.LittleEndian.Uint16(b[at:])) }
func u32(b []byte, at int) int { return int(binary.LittleEndian.Uint32(b[at:])) }

// table reads a table definition, which may run over several pages.
func (db *DB) table(name string, defPage int) (*Table, error) {
	var def []byte
	seen := map[int]bool{}
	for pg := defPage; pg != 0; {
		if seen[pg] {
			return nil, errors.New("the table definition loops")
		}
		seen[pg] = true
		p, err := db.page(pg)
		if err != nil {
			return nil, err
		}
		if p[0] != 0x02 {
			return nil, fmt.Errorf("page %d is not a table definition", pg)
		}
		if def == nil {
			def = append(def, p...)
		} else {
			def = append(def, p[8:]...)
		}
		pg = u32(p, 4)
	}
	if len(def) < 63 {
		return nil, errors.New("the table definition is cut short")
	}
	t := &Table{db: db, Name: name, defPage: defPage}
	t.varCols = u16(def, 43)
	cols := u16(def, 45)
	realIdx := u32(def, 51)
	at := 63 + realIdx*12
	if cols > 4096 || at+cols*25 > len(def) {
		return nil, errors.New("the table definition is damaged")
	}
	for i := 0; i < cols; i++ {
		c := def[at+i*25:]
		t.Columns = append(t.Columns, Column{
			Type:        int(c[0]),
			num:         u16(c, 5),
			varNum:      u16(c, 7),
			fixed:       c[15]&0x01 != 0,
			compressed:  c[16]&0x01 != 0,
			fixedOffset: u16(c, 21),
			size:        u16(c, 23),
		})
	}
	at += cols * 25
	for i := range t.Columns {
		if at+2 > len(def) {
			return nil, errors.New("the column names are cut short")
		}
		n := u16(def, at)
		at += 2
		if at+n > len(def) {
			return nil, errors.New("the column names are cut short")
		}
		t.Columns[i].Name = ucs2(def[at : at+n])
		at += n
	}
	return t, nil
}

// Row is a row's values by column name: string for text and memo, int64
// for whole numbers, float64 for the others, bool, []byte for binary data,
// and nil for an empty field.
type Row map[string]any

// rowAt returns the bytes of a row on a data page and its two flags.
func rowAt(p []byte, row int) (data []byte, flagA, flagB bool, err error) {
	n := u16(p, 12)
	if row < 0 || row >= n || 14+2*n > pageSize {
		return nil, false, false, fmt.Errorf("no row %d on the page", row)
	}
	start := u16(p, 14+2*row)
	end := pageSize
	if row > 0 {
		end = u16(p, 14+2*(row-1)) & 0x1FFF
	}
	flagA, flagB = start&0x8000 != 0, start&0x4000 != 0
	start &= 0x1FFF
	if start > end || end > pageSize {
		return nil, false, false, fmt.Errorf("row %d lies outside its page", row)
	}
	return p[start:end], flagA, flagB, nil
}

// Rows calls fn for each row of the table.
func (t *Table) Rows(fn func(Row) error) error {
	db := t.db
	total := len(db.data) / pageSize
	type place struct{ pg, row int }
	var places []place
	for pg := 1; pg < total; pg++ {
		p, err := db.page(pg)
		if err != nil {
			return err
		}
		if p[0] != 0x01 || u32(p, 4) != t.defPage || 14+2*u16(p, 12) > pageSize {
			continue
		}
		for row := 0; row < u16(p, 12); row++ {
			places = append(places, place{pg, row})
		}
	}
	// A row that outgrew its page was moved: its old place holds four
	// bytes that say where it went. The row is read through that pointer,
	// in its old position, and not a second time where it now lies. The
	// two flags of a place are told apart by this, since the descriptions
	// of the format disagree on which of them marks a deleted row and
	// which a pointer: a flagged place of four bytes that leads to a row
	// of this table is a pointer, and with both flags a pointer whose row
	// was deleted.
	follow := map[place]place{}
	moved := map[place]bool{}
	for _, pl := range places {
		p, _ := db.page(pl.pg)
		data, flagA, flagB, err := rowAt(p, pl.row)
		if err != nil || !(flagA || flagB) || len(data) != 4 {
			continue
		}
		ptr := u32(data, 0)
		to := place{ptr >> 8, ptr & 0xFF}
		tp, err := db.page(to.pg)
		if err != nil || to.pg == 0 || tp[0] != 0x01 || u32(tp, 4) != t.defPage || to.row >= u16(tp, 12) {
			continue
		}
		moved[to] = true
		if flagA != flagB {
			follow[pl] = to
		}
	}
	for _, pl := range places {
		if moved[pl] {
			continue
		}
		p, _ := db.page(pl.pg)
		data, flagA, flagB, err := rowAt(p, pl.row)
		if err != nil {
			return fmt.Errorf("table %s, page %d: %w", t.Name, pl.pg, err)
		}
		if to, ok := follow[pl]; ok {
			tp, _ := db.page(to.pg)
			if data, _, _, err = rowAt(tp, to.row); err != nil {
				return fmt.Errorf("table %s, page %d: %w", t.Name, to.pg, err)
			}
		} else if flagA || flagB {
			continue // deleted
		}
		if len(data) == 0 {
			continue
		}
		r, err := t.crack(data)
		if err != nil {
			return fmt.Errorf("table %s, page %d, row %d: %w", t.Name, pl.pg, pl.row, err)
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

// crack takes a row apart into its columns.
func (t *Table) crack(row []byte) (Row, error) {
	if len(row) < 2 {
		return nil, errors.New("the row is cut short")
	}
	rowCols := u16(row, 0)
	maskLen := (rowCols + 7) / 8
	end := len(row)
	if maskLen+2 > end {
		return nil, errors.New("the row is cut short")
	}
	mask := row[end-maskLen:]
	rowVar := 0
	var offsets []int
	if t.varCols > 0 {
		if end-maskLen-2 < 2 {
			return nil, errors.New("the row is cut short")
		}
		rowVar = u16(row, end-maskLen-2)
		if end-maskLen-4-2*rowVar < 2 {
			return nil, errors.New("the row's field table is cut short")
		}
		for i := 0; i <= rowVar; i++ {
			offsets = append(offsets, u16(row, end-maskLen-4-2*i))
		}
	}
	rowFixed := rowCols - rowVar
	fixedSeen := 0
	out := Row{}
	for _, c := range t.Columns {
		null := c.num/8 >= len(mask) || mask[c.num/8]&(1<<(c.num%8)) == 0
		if c.Type == Bool {
			// A yes/no column has no bytes of its own: its bit is its value.
			if c.fixed && fixedSeen < rowFixed {
				fixedSeen++
			}
			out[c.Name] = !null
			continue
		}
		var field []byte
		switch {
		case c.fixed && fixedSeen < rowFixed:
			fixedSeen++
			start := 2 + c.fixedOffset
			if start+c.size > len(row) {
				return nil, fmt.Errorf("column %s lies outside the row", c.Name)
			}
			field = row[start : start+c.size]
		case !c.fixed && c.varNum < rowVar:
			start, stop := offsets[c.varNum], offsets[c.varNum+1]
			if start > stop || stop > len(row) {
				return nil, fmt.Errorf("column %s lies outside the row", c.Name)
			}
			field = row[start:stop]
		default:
			null = true
		}
		if null {
			out[c.Name] = nil
			continue
		}
		v, err := t.db.value(c, field)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", c.Name, err)
		}
		out[c.Name] = v
	}
	return out, nil
}

func (db *DB) value(c Column, f []byte) (any, error) {
	need := func(n int) error {
		if len(f) < n {
			return errors.New("the field is cut short")
		}
		return nil
	}
	switch c.Type {
	case Byte:
		if err := need(1); err != nil {
			return nil, err
		}
		return int64(f[0]), nil
	case Int16:
		if err := need(2); err != nil {
			return nil, err
		}
		return int64(int16(binary.LittleEndian.Uint16(f))), nil
	case Int32:
		if err := need(4); err != nil {
			return nil, err
		}
		return int64(int32(binary.LittleEndian.Uint32(f))), nil
	case Float32:
		if err := need(4); err != nil {
			return nil, err
		}
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(f))), nil
	case Float64, DateTime:
		if err := need(8); err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(f)), nil
	case Money:
		if err := need(8); err != nil {
			return nil, err
		}
		return float64(int64(binary.LittleEndian.Uint64(f))) / 10000, nil
	case Text:
		return text(f), nil
	case Memo, OLE:
		b, err := db.long(f)
		if err != nil {
			return nil, err
		}
		if c.Type == Memo {
			return text(b), nil
		}
		return b, nil
	}
	return append([]byte(nil), f...), nil
}

// long reads a memo or long binary field: its bytes are in the row, on one
// other page, or in a chain of pieces.
func (db *DB) long(f []byte) ([]byte, error) {
	if len(f) < 12 {
		return nil, errors.New("the long field is cut short")
	}
	length := int(f[0]) | int(f[1])<<8 | int(f[2])<<16
	kind := f[3]
	if kind&0x80 != 0 {
		if 12+length > len(f) {
			return nil, errors.New("the long field is cut short")
		}
		return f[12 : 12+length], nil
	}
	ptr := u32(f, 4)
	piece := func(ptr int) ([]byte, error) {
		p, err := db.page(ptr >> 8)
		if err != nil {
			return nil, err
		}
		if p[0] != 0x01 {
			return nil, errors.New("a long field points to a page that holds no data")
		}
		data, _, _, err := rowAt(p, ptr&0xFF)
		return data, err
	}
	if kind&0x40 != 0 {
		data, err := piece(ptr)
		if err != nil {
			return nil, err
		}
		if length > len(data) {
			return nil, errors.New("a long field is shorter than it says")
		}
		return data[:length], nil
	}
	out := make([]byte, 0, length)
	for hops := 0; ptr != 0 && len(out) < length; hops++ {
		if hops > len(db.data)/pageSize {
			return nil, errors.New("a long field loops")
		}
		data, err := piece(ptr)
		if err != nil {
			return nil, err
		}
		if len(data) < 4 {
			return nil, errors.New("a piece of a long field is cut short")
		}
		ptr = u32(data, 0)
		out = append(out, data[4:]...)
	}
	if len(out) < length {
		return nil, errors.New("a long field is shorter than it says")
	}
	return out[:length], nil
}

// text decodes a text field: UCS-2, or after the mark FF FE one byte for
// each character with zero bytes switching between the two.
func text(f []byte) string {
	if len(f) >= 2 && f[0] == 0xFF && f[1] == 0xFE {
		f = f[2:]
		var units []uint16
		compressed := true
		for i := 0; i < len(f); {
			switch {
			case f[i] == 0:
				compressed = !compressed
				i++
			case compressed:
				units = append(units, uint16(f[i]))
				i++
			case i+1 < len(f):
				units = append(units, binary.LittleEndian.Uint16(f[i:]))
				i += 2
			default:
				i = len(f)
			}
		}
		return string(utf16.Decode(units))
	}
	return ucs2(f)
}

func ucs2(b []byte) string {
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(units))
}
