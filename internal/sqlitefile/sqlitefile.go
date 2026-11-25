// Package sqlitefile reads the tables of an SQLite 3 database from the
// bytes of its file, which is what a Trados Studio translation memory
// (.sdltm) is. It reads only: whole tables, row by row. Indexes are not
// looked at, and changes still in a write-ahead log (the -wal file beside
// the database) are not seen.
//
// It needs no SQLite library, so it runs where none can, in the browser
// client's WebAssembly core among them. It follows the published
// description of the file format (https://www.sqlite.org/fileformat.html).
package sqlitefile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

// ErrFormat means the data is not an SQLite 3 database.
var ErrFormat = errors.New("not an SQLite 3 database")

var magic = []byte("SQLite format 3\x00")

// Is reports whether data starts as an SQLite 3 database does.
func Is(data []byte) bool { return len(data) >= 100 && bytes.Equal(data[:16], magic) }

// DB is an open database.
type DB struct {
	data     []byte
	pageSize int
	usable   int
	pages    int
	encoding int // 1 UTF-8, 2 UTF-16le, 3 UTF-16be
	tables   map[string]table
	names    []string
}

type table struct {
	name    string
	root    int
	columns []string
	rowid   int // column that is an alias of the rowid, or -1
}

// Row is one row of a table: values by column name. A value is nil,
// int64, float64, string or []byte.
type Row map[string]any

// Open reads the schema of the database in data. data is kept, not copied.
func Open(data []byte) (*DB, error) {
	if !Is(data) {
		return nil, ErrFormat
	}
	ps := int(binary.BigEndian.Uint16(data[16:]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps&(ps-1) != 0 {
		return nil, fmt.Errorf("sqlite: page size %d: %w", ps, ErrFormat)
	}
	reserved := int(data[20])
	db := &DB{data: data, pageSize: ps, usable: ps - reserved, pages: len(data) / ps, tables: map[string]table{}}
	if db.usable < 480 {
		return nil, fmt.Errorf("sqlite: usable page size %d: %w", db.usable, ErrFormat)
	}
	switch enc := int(binary.BigEndian.Uint32(data[56:])); enc {
	case 0, 1:
		db.encoding = 1
	case 2, 3:
		db.encoding = enc
	default:
		return nil, fmt.Errorf("sqlite: text encoding %d: %w", enc, ErrFormat)
	}
	schema := table{name: "sqlite_schema", root: 1, columns: []string{"type", "name", "tbl_name", "rootpage", "sql"}, rowid: -1}
	err := db.scan(schema, func(r Row) error {
		if t, _ := r["type"].(string); t != "table" {
			return nil
		}
		name, _ := r["name"].(string)
		root, _ := r["rootpage"].(int64)
		sql, _ := r["sql"].(string)
		cols, rowid, withoutRowid := columns(sql)
		if name == "" || root <= 0 || withoutRowid {
			// A table without rowid is an index b-tree; none of the
			// files read here has one.
			return nil
		}
		db.tables[strings.ToLower(name)] = table{name: name, root: int(root), columns: cols, rowid: rowid}
		db.names = append(db.names, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Tables names the database's tables, in the order of its schema.
func (db *DB) Tables() []string { return append([]string(nil), db.names...) }

// Columns names the columns of a table, or nil when there is no such table.
func (db *DB) Columns(name string) []string {
	t, ok := db.tables[strings.ToLower(name)]
	if !ok {
		return nil
	}
	return append([]string(nil), t.columns...)
}

// ErrNoTable means the database has no table of that name.
var ErrNoTable = errors.New("sqlite: no such table")

// Rows calls fn with every row of the table, in rowid order. An error
// from fn stops the scan and is returned.
func (db *DB) Rows(name string, fn func(Row) error) error {
	t, ok := db.tables[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoTable, name)
	}
	return db.scan(t, fn)
}

func (db *DB) page(n int) ([]byte, error) {
	if n < 1 || n > db.pages {
		return nil, fmt.Errorf("sqlite: page %d of %d: %w", n, db.pages, ErrFormat)
	}
	return db.data[(n-1)*db.pageSize : n*db.pageSize], nil
}

// scan walks the table b-tree rooted at t.root.
func (db *DB) scan(t table, fn func(Row) error) error {
	seen := map[int]bool{}
	var walk func(n int) error
	walk = func(n int) error {
		if seen[n] {
			return fmt.Errorf("sqlite: page %d is reached twice: %w", n, ErrFormat)
		}
		seen[n] = true
		p, err := db.page(n)
		if err != nil {
			return err
		}
		h := 0
		if n == 1 {
			h = 100
		}
		if len(p) < h+12 {
			return ErrFormat
		}
		kind := p[h]
		cells := int(binary.BigEndian.Uint16(p[h+3:]))
		switch kind {
		case 0x0d: // table leaf
			ptrs := h + 8
			for i := 0; i < cells; i++ {
				at := int(binary.BigEndian.Uint16(p[ptrs+2*i:]))
				rowid, payload, err := db.leafCell(p, at)
				if err != nil {
					return fmt.Errorf("sqlite: page %d cell %d: %w", n, i, err)
				}
				row, err := db.record(payload, t, rowid)
				if err != nil {
					return fmt.Errorf("sqlite: table %s row %d: %w", t.name, rowid, err)
				}
				if err := fn(row); err != nil {
					return err
				}
			}
			return nil
		case 0x05: // table interior
			ptrs := h + 12
			for i := 0; i < cells; i++ {
				at := int(binary.BigEndian.Uint16(p[ptrs+2*i:]))
				if at+4 > len(p) {
					return ErrFormat
				}
				if err := walk(int(binary.BigEndian.Uint32(p[at:]))); err != nil {
					return err
				}
			}
			return walk(int(binary.BigEndian.Uint32(p[h+8:])))
		default:
			return fmt.Errorf("sqlite: page %d of table %s is of kind %#x: %w", n, t.name, kind, ErrFormat)
		}
	}
	return walk(t.root)
}

// leafCell returns the rowid and the whole payload of a table leaf cell,
// following its overflow pages.
func (db *DB) leafCell(p []byte, at int) (int64, []byte, error) {
	if at >= len(p) {
		return 0, nil, ErrFormat
	}
	size, n := varint(p[at:])
	if n == 0 {
		return 0, nil, ErrFormat
	}
	at += n
	rowid, n := varint(p[at:])
	if n == 0 {
		return 0, nil, ErrFormat
	}
	at += n
	total := int(size)
	if total < 0 || total > len(db.data) {
		return 0, nil, ErrFormat
	}
	u := db.usable
	x := u - 35
	local := total
	if total > x {
		m := (u-12)*32/255 - 23
		k := m + (total-m)%(u-4)
		if k <= x {
			local = k
		} else {
			local = m
		}
	}
	if at+local > len(p) {
		return 0, nil, ErrFormat
	}
	if local == total {
		return int64(rowid), p[at : at+total], nil
	}
	out := make([]byte, 0, total)
	out = append(out, p[at:at+local]...)
	if at+local+4 > len(p) {
		return 0, nil, ErrFormat
	}
	next := int(binary.BigEndian.Uint32(p[at+local:]))
	for len(out) < total {
		if next == 0 {
			return 0, nil, fmt.Errorf("overflow chain ends early: %w", ErrFormat)
		}
		op, err := db.page(next)
		if err != nil {
			return 0, nil, err
		}
		take := min(total-len(out), u-4)
		out = append(out, op[4:4+take]...)
		next = int(binary.BigEndian.Uint32(op))
	}
	return int64(rowid), out, nil
}

// record decodes a row's payload into its columns.
func (db *DB) record(b []byte, t table, rowid int64) (Row, error) {
	hsize, n := varint(b)
	if n == 0 || int(hsize) > len(b) || hsize < uint64(n) {
		return nil, ErrFormat
	}
	header := b[n:hsize]
	body := b[hsize:]
	row := make(Row, len(t.columns))
	for i := 0; len(header) > 0; i++ {
		st, k := varint(header)
		if k == 0 {
			return nil, ErrFormat
		}
		header = header[k:]
		v, size, err := db.value(st, body)
		if err != nil {
			return nil, err
		}
		body = body[size:]
		if i < len(t.columns) {
			row[t.columns[i]] = v
		}
	}
	for _, c := range t.columns {
		if _, ok := row[c]; !ok {
			// A column added later than the row was written.
			row[c] = nil
		}
	}
	if t.rowid >= 0 {
		row[t.columns[t.rowid]] = rowid
	}
	return row, nil
}

func (db *DB) value(st uint64, b []byte) (any, int, error) {
	need := func(n int) error {
		if len(b) < n {
			return ErrFormat
		}
		return nil
	}
	signed := func(n int) (any, int, error) {
		if err := need(n); err != nil {
			return nil, 0, err
		}
		var v int64
		for i := 0; i < n; i++ {
			v = v<<8 | int64(b[i])
		}
		shift := 64 - 8*n
		return v << shift >> shift, n, nil
	}
	switch {
	case st == 0:
		return nil, 0, nil
	case st >= 1 && st <= 4:
		return signed(int(st))
	case st == 5:
		return signed(6)
	case st == 6:
		return signed(8)
	case st == 7:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(b)), 8, nil
	case st == 8:
		return int64(0), 0, nil
	case st == 9:
		return int64(1), 0, nil
	case st >= 12 && st%2 == 0:
		n := int((st - 12) / 2)
		if err := need(n); err != nil {
			return nil, 0, err
		}
		return append(make([]byte, 0, n), b[:n]...), n, nil
	case st >= 13:
		n := int((st - 13) / 2)
		if err := need(n); err != nil {
			return nil, 0, err
		}
		return db.text(b[:n]), n, nil
	}
	return nil, 0, fmt.Errorf("serial type %d: %w", st, ErrFormat)
}

func (db *DB) text(b []byte) string {
	if db.encoding == 1 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		if db.encoding == 2 {
			u[i] = binary.LittleEndian.Uint16(b[2*i:])
		} else {
			u[i] = binary.BigEndian.Uint16(b[2*i:])
		}
	}
	return string(utf16.Decode(u))
}

// varint reads SQLite's big-endian variable-length integer: up to nine
// bytes, seven bits from each of the first eight and all eight of the
// ninth. n is 0 when b ends first.
func varint(b []byte) (v uint64, n int) {
	for i := 0; i < 9; i++ {
		if i >= len(b) {
			return 0, 0
		}
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return v, 9
}

// columns reads the column names of a CREATE TABLE statement, which
// column (if any) is an alias of the rowid, and whether the table is
// WITHOUT ROWID.
func columns(sql string) (names []string, rowid int, withoutRowid bool) {
	rowid = -1
	sql = stripComments(sql)
	open := strings.IndexByte(sql, '(')
	if open < 0 {
		return nil, -1, false
	}
	depth := 0
	end := -1
	var quote byte
	for i := open; i < len(sql) && end < 0; i++ {
		c := sql[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`' || c == '[':
			quote = c
			if c == '[' {
				quote = ']'
			}
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
	}
	if end < 0 {
		return nil, -1, false
	}
	withoutRowid = strings.Contains(strings.ToUpper(sql[end:]), "WITHOUT ROWID")
	for _, def := range splitTop(sql[open+1 : end]) {
		def = strings.TrimSpace(def)
		if def == "" {
			continue
		}
		word := strings.ToUpper(firstWord(def))
		switch word {
		case "CONSTRAINT", "PRIMARY", "UNIQUE", "CHECK", "FOREIGN":
			continue
		}
		name := unquote(firstToken(def))
		rest := strings.ToUpper(strings.Join(strings.Fields(def[len(firstToken(def)):]), " "))
		if strings.HasPrefix(rest, "INTEGER") && strings.Contains(rest, "PRIMARY KEY") && !strings.Contains(rest, "PRIMARY KEY DESC") {
			rowid = len(names)
		}
		names = append(names, name)
	}
	return names, rowid, withoutRowid
}

// stripComments replaces SQL comments (-- to the end of the line, and
// /* ... */) outside quotes with a space. Trados's schema has one with a
// comma in it.
func stripComments(sql string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '[':
			quote = ']'
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
			if i < len(sql) {
				b.WriteByte('\n')
			}
			continue
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			i += end + 3
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// splitTop splits at commas outside parentheses and quotes.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '[':
			quote = ']'
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func firstToken(def string) string {
	def = strings.TrimLeft(def, " \t\r\n")
	if def == "" {
		return ""
	}
	if c := def[0]; c == '"' || c == '`' || c == '[' || c == '\'' {
		close := c
		if c == '[' {
			close = ']'
		}
		for i := 1; i < len(def); i++ {
			if def[i] != close {
				continue
			}
			if close != ']' && i+1 < len(def) && def[i+1] == close {
				i++ // a doubled quote stands for itself
				continue
			}
			return def[:i+1]
		}
		return def
	}
	if i := strings.IndexAny(def, " \t\r\n("); i >= 0 {
		return def[:i]
	}
	return def
}

func firstWord(def string) string {
	t := firstToken(def)
	if t != "" && (t[0] == '"' || t[0] == '`' || t[0] == '[' || t[0] == '\'') {
		return ""
	}
	return t
}

func unquote(s string) string {
	if len(s) >= 2 {
		switch s[0] {
		case '"', '`', '\'':
			if s[len(s)-1] == s[0] {
				return strings.ReplaceAll(s[1:len(s)-1], string(s[0])+string(s[0]), string(s[0]))
			}
		case '[':
			if s[len(s)-1] == ']' {
				return s[1 : len(s)-1]
			}
		}
	}
	return s
}
