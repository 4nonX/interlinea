package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// The objects of a PDF file as Go values: nil, bool, float64, pstring (a
// string's bytes), name, []any, dict, *stream and ref.
type (
	name    string
	pstring string
	ref     struct{ num, gen int }
	dict    map[name]any
	// op is a keyword that is not a value: an operator of a content
	// stream, or obj, endobj, stream, R in a file.
	op string
)

type stream struct {
	dict dict
	raw  []byte
}

var errSyntax = errors.New("syntax")

func isSpace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// lexer reads objects from bytes.
type lexer struct {
	b []byte
	p int
	// refs is whether "12 0 R" is read as a reference, as in a file; a
	// content stream has none.
	refs bool
	// depth is how deep in arrays and dictionaries the reader is.
	depth int
}

// maxDepth is how deep arrays and dictionaries may be nested.
const maxDepth = 256

func (l *lexer) skip() {
	for l.p < len(l.b) {
		c := l.b[l.p]
		if isSpace(c) {
			l.p++
		} else if c == '%' {
			for l.p < len(l.b) && l.b[l.p] != '\n' && l.b[l.p] != '\r' {
				l.p++
			}
		} else {
			return
		}
	}
}

// next reads one object or keyword. At the end it returns op("").
func (l *lexer) next() (any, error) {
	l.skip()
	if l.p >= len(l.b) {
		return op(""), nil
	}
	c := l.b[l.p]
	switch {
	case c == '/':
		l.p++
		return l.name(), nil
	case c == '(':
		l.p++
		return l.literal(), nil
	case c == '<':
		if l.p+1 < len(l.b) && l.b[l.p+1] == '<' {
			l.p += 2
			return l.dict()
		}
		l.p++
		return l.hex(), nil
	case c == '[':
		l.p++
		if l.depth >= maxDepth {
			return nil, errSyntax
		}
		l.depth++
		defer func() { l.depth-- }()
		var out []any
		for {
			l.skip()
			if l.p >= len(l.b) {
				return out, nil
			}
			if l.b[l.p] == ']' {
				l.p++
				return out, nil
			}
			v, err := l.next()
			if err != nil {
				return nil, err
			}
			if o, ok := v.(op); ok {
				if o == "" {
					return out, nil
				}
				continue // a stray keyword in an array
			}
			out = append(out, v)
		}
	case c == ']' || c == '>' || c == ')' || c == '{' || c == '}':
		// A delimiter out of place: step over it.
		l.p++
		if c == '>' && l.p < len(l.b) && l.b[l.p] == '>' {
			l.p++
			return op(">>"), nil
		}
		return op(string(c)), nil
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return l.number()
	}
	start := l.p
	for l.p < len(l.b) && !isSpace(l.b[l.p]) && !isDelim(l.b[l.p]) {
		l.p++
	}
	word := string(l.b[start:l.p])
	switch word {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	return op(word), nil
}

func (l *lexer) number() (any, error) {
	start := l.p
	for l.p < len(l.b) {
		c := l.b[l.p]
		if c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') {
			l.p++
		} else {
			break
		}
	}
	text := string(l.b[start:l.p])
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		// "--5", "5-", "." and the like: what a lenient reader makes of them.
		clean := make([]byte, 0, len(text))
		for i := 0; i < len(text); i++ {
			if text[i] == '-' || text[i] == '+' {
				if len(clean) == 0 && text[i] == '-' {
					clean = append(clean, '-')
				}
				continue
			}
			clean = append(clean, text[i])
		}
		f, _ = strconv.ParseFloat(string(clean), 64)
	}
	if !l.refs || f != float64(int(f)) || f < 0 {
		return f, nil
	}
	// "12 0 R"?
	save := l.p
	l.skip()
	g := l.p
	for l.p < len(l.b) && l.b[l.p] >= '0' && l.b[l.p] <= '9' {
		l.p++
	}
	if l.p > g && l.p-g < 6 {
		gen, _ := strconv.Atoi(string(l.b[g:l.p]))
		l.skip()
		if l.p < len(l.b) && l.b[l.p] == 'R' && (l.p+1 == len(l.b) || isSpace(l.b[l.p+1]) || isDelim(l.b[l.p+1])) {
			l.p++
			return ref{int(f), gen}, nil
		}
	}
	l.p = save
	return f, nil
}

func (l *lexer) name() name {
	var out []byte
	for l.p < len(l.b) && !isSpace(l.b[l.p]) && !isDelim(l.b[l.p]) {
		c := l.b[l.p]
		if c == '#' && l.p+2 < len(l.b) {
			if v, err := strconv.ParseUint(string(l.b[l.p+1:l.p+3]), 16, 8); err == nil {
				out = append(out, byte(v))
				l.p += 3
				continue
			}
		}
		out = append(out, c)
		l.p++
	}
	return name(out)
}

func (l *lexer) literal() pstring {
	var out []byte
	depth := 1
	for l.p < len(l.b) {
		c := l.b[l.p]
		l.p++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return pstring(out)
			}
			out = append(out, c)
		case '\\':
			if l.p >= len(l.b) {
				return pstring(out)
			}
			e := l.b[l.p]
			l.p++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if l.p < len(l.b) && l.b[l.p] == '\n' {
					l.p++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for i := 0; i < 2 && l.p < len(l.b) && l.b[l.p] >= '0' && l.b[l.p] <= '7'; i++ {
						v = v*8 + int(l.b[l.p]-'0')
						l.p++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}
	return pstring(out)
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func (l *lexer) hex() pstring {
	var out []byte
	half := -1
	for l.p < len(l.b) {
		c := l.b[l.p]
		l.p++
		if c == '>' {
			break
		}
		v := unhex(c)
		if v < 0 {
			continue
		}
		if half < 0 {
			half = v
		} else {
			out = append(out, byte(half<<4|v))
			half = -1
		}
	}
	if half >= 0 {
		out = append(out, byte(half<<4))
	}
	return pstring(out)
}

func (l *lexer) dict() (any, error) {
	if l.depth >= maxDepth {
		return nil, errSyntax
	}
	l.depth++
	defer func() { l.depth-- }()
	d := dict{}
	for {
		l.skip()
		if l.p >= len(l.b) {
			return d, nil
		}
		if l.b[l.p] == '>' {
			l.p++
			if l.p < len(l.b) && l.b[l.p] == '>' {
				l.p++
			}
			return d, nil
		}
		k, err := l.next()
		if err != nil {
			return nil, err
		}
		key, ok := k.(name)
		if !ok {
			if o, isOp := k.(op); isOp && (o == "" || o == ">>") {
				return d, nil
			}
			continue // not a key: skipped
		}
		l.skip()
		if l.p < len(l.b) && l.b[l.p] == '>' {
			continue // a key without a value
		}
		v, err := l.next()
		if err != nil {
			return nil, err
		}
		if o, isOp := v.(op); isOp {
			if o == "" {
				return d, nil
			}
			continue
		}
		d[key] = v
	}
}

// file is a PDF file with its objects found.
type file struct {
	data    []byte
	offsets map[int]int    // object number -> where it starts
	packed  map[int][2]int // object number -> the object stream it is in, and its place there
	cache   map[int]any
	loading map[int]bool
	trailer dict
	// unpacked object streams: the objects of each, by place.
	streams map[int][]any
}

func open(data []byte) (*file, error) {
	if !bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-")) {
		return nil, errors.New("it is not a PDF file")
	}
	f := &file{data: data, offsets: map[int]int{}, packed: map[int][2]int{}, cache: map[int]any{}, loading: map[int]bool{}, streams: map[int][]any{}}
	if !f.readTables() || f.root() == nil {
		// The table of objects is missing or broken: the objects are
		// found by reading the whole file.
		f.offsets, f.packed, f.cache, f.trailer = map[int]int{}, map[int][2]int{}, map[int]any{}, nil
		f.scan()
	}
	if f.trailer == nil || f.root() == nil {
		return nil, errors.New("its list of objects cannot be read")
	}
	return f, nil
}

func (f *file) root() dict {
	d, _ := f.resolve(f.trailer["Root"]).(dict)
	return d
}

// readTables follows the cross-reference tables and streams from the end
// of the file.
func (f *file) readTables() bool {
	at := bytes.LastIndex(f.data, []byte("startxref"))
	if at < 0 {
		return false
	}
	l := &lexer{b: f.data, p: at + len("startxref")}
	v, _ := l.next()
	n, ok := v.(float64)
	if !ok {
		return false
	}
	seen := map[int]bool{}
	next := []int{int(n)}
	for len(next) > 0 {
		off := next[0]
		next = next[1:]
		if off <= 0 || off >= len(f.data) || seen[off] {
			continue
		}
		seen[off] = true
		tr, ok := f.readTable(off)
		if !ok {
			return false
		}
		if f.trailer == nil {
			f.trailer = tr
		} else {
			for k, v := range tr {
				if _, has := f.trailer[k]; !has {
					f.trailer[k] = v
				}
			}
		}
		if x, ok := tr["XRefStm"].(float64); ok {
			next = append(next, int(x))
		}
		if p, ok := tr["Prev"].(float64); ok {
			next = append(next, int(p))
		}
	}
	return f.trailer != nil
}

func (f *file) readTable(off int) (dict, bool) {
	l := &lexer{b: f.data, p: off, refs: true}
	l.skip()
	if bytes.HasPrefix(f.data[l.p:], []byte("xref")) {
		l.p += 4
		for {
			v, err := l.next()
			if err != nil {
				return nil, false
			}
			if o, ok := v.(op); ok {
				if o != "trailer" {
					return nil, false
				}
				t, err := l.next()
				d, ok := t.(dict)
				return d, err == nil && ok
			}
			first, ok1 := v.(float64)
			c, _ := l.next()
			count, ok2 := c.(float64)
			if !ok1 || !ok2 || count < 0 || count > 1e7 {
				return nil, false
			}
			for i := 0; i < int(count); i++ {
				a, _ := l.next()
				_, _ = l.next()
				kind, _ := l.next()
				pos, ok := a.(float64)
				if !ok {
					return nil, false
				}
				num := int(first) + i
				if kind == op("n") && pos > 0 {
					if _, has := f.offsets[num]; !has {
						if _, has := f.packed[num]; !has {
							f.offsets[num] = int(pos)
						}
					}
				}
			}
		}
	}
	// A cross-reference stream.
	obj, ok := f.readObjectAt(off)
	if !ok {
		return nil, false
	}
	s, ok := obj.(*stream)
	if !ok || s.dict["Type"] != name("XRef") {
		return nil, false
	}
	data, err := decodeStream(f, s)
	if err != nil {
		return nil, false
	}
	var w [3]int
	ws, _ := s.dict["W"].([]any)
	if len(ws) < 3 {
		return nil, false
	}
	for i := range w {
		n, _ := ws[i].(float64)
		w[i] = int(n)
		if w[i] < 0 || w[i] > 8 {
			return nil, false
		}
	}
	size, _ := s.dict["Size"].(float64)
	index, _ := s.dict["Index"].([]any)
	if len(index) == 0 {
		index = []any{0.0, size}
	}
	row := w[0] + w[1] + w[2]
	if row == 0 {
		return nil, false
	}
	p := 0
	field := func(n int) int {
		v := 0
		for i := 0; i < n; i++ {
			v = v<<8 | int(data[p])
			p++
		}
		return v
	}
	for i := 0; i+1 < len(index); i += 2 {
		first, _ := index[i].(float64)
		count, _ := index[i+1].(float64)
		for j := 0; j < int(count) && p+row <= len(data); j++ {
			kind := 1
			if w[0] > 0 {
				kind = field(w[0])
			}
			a, b := field(w[1]), field(w[2])
			num := int(first) + j
			_, has1 := f.offsets[num]
			_, has2 := f.packed[num]
			if has1 || has2 {
				continue
			}
			switch kind {
			case 1:
				f.offsets[num] = a
			case 2:
				f.packed[num] = [2]int{a, b}
			}
		}
	}
	return s.dict, true
}

// readObjectAt reads the object that starts ("12 0 obj") at an offset.
func (f *file) readObjectAt(off int) (any, bool) {
	if off < 0 || off >= len(f.data) {
		return nil, false
	}
	l := &lexer{b: f.data, p: off}
	if _, ok := mustNumber(l); !ok {
		return nil, false
	}
	if _, ok := mustNumber(l); !ok {
		return nil, false
	}
	if v, _ := l.next(); v != op("obj") {
		return nil, false
	}
	l.refs = true
	v, err := l.next()
	if err != nil {
		return nil, false
	}
	d, isDict := v.(dict)
	if !isDict {
		return v, true
	}
	l.skip()
	if !bytes.HasPrefix(f.data[l.p:], []byte("stream")) {
		return d, true
	}
	p := l.p + len("stream")
	if p < len(f.data) && f.data[p] == '\r' {
		p++
	}
	if p < len(f.data) && f.data[p] == '\n' {
		p++
	}
	// The length the dictionary says, if what follows it is the stream's end.
	if n, ok := f.resolve(d["Length"]).(float64); ok && n >= 0 && p+int(n) <= len(f.data) {
		end := p + int(n)
		rest := f.data[end:min(len(f.data), end+32)]
		if bytes.HasPrefix(bytes.TrimLeft(rest, "\r\n \t"), []byte("endstream")) {
			return &stream{d, f.data[p:end]}, true
		}
	}
	end := bytes.Index(f.data[p:], []byte("endstream"))
	if end < 0 {
		return &stream{d, f.data[p:]}, true
	}
	raw := f.data[p : p+end]
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	raw = bytes.TrimSuffix(raw, []byte("\r"))
	return &stream{d, raw}, true
}

func mustNumber(l *lexer) (int, bool) {
	v, err := l.next()
	n, ok := v.(float64)
	return int(n), err == nil && ok
}

// scan finds every "n g obj" in the file, and the trailer among them.
func (f *file) scan() {
	b := f.data
	var streams []int
	for i := 0; i+3 < len(b); i++ {
		if b[i] != 'o' || b[i+1] != 'b' || b[i+2] != 'j' || (i > 0 && !isSpace(b[i-1])) {
			continue
		}
		// Backwards: generation, object number.
		p := i - 1
		for p >= 0 && isSpace(b[p]) {
			p--
		}
		g := p
		for p >= 0 && b[p] >= '0' && b[p] <= '9' {
			p--
		}
		if p == g || p < 0 || !isSpace(b[p]) {
			continue
		}
		for p >= 0 && isSpace(b[p]) {
			p--
		}
		n := p
		for p >= 0 && b[p] >= '0' && b[p] <= '9' {
			p--
		}
		if p == n || n-p > 9 {
			continue
		}
		num, _ := strconv.Atoi(string(b[p+1 : n+1]))
		f.offsets[num] = p + 1 // a later object of the number replaces an earlier one
	}
	for num := range f.offsets {
		if s, ok := f.object(num).(*stream); ok {
			switch s.dict["Type"] {
			case name("XRef"):
				if f.trailer == nil || s.dict["Root"] != nil {
					f.trailer = s.dict
				}
			case name("ObjStm"):
				streams = append(streams, num)
			}
		}
	}
	for _, num := range streams {
		for i, inner := range f.objectStreamNumbers(num) {
			if _, direct := f.offsets[inner]; !direct {
				f.packed[inner] = [2]int{num, i}
			}
		}
	}
	// Trailer dictionaries, the last one first.
	for at := len(b); ; {
		at = bytes.LastIndex(b[:at], []byte("trailer"))
		if at < 0 {
			break
		}
		l := &lexer{b: b, p: at + len("trailer"), refs: true}
		if v, err := l.next(); err == nil {
			if d, ok := v.(dict); ok && d["Root"] != nil {
				if f.trailer == nil || f.trailer["Root"] == nil {
					f.trailer = d
				}
				if d["Encrypt"] != nil {
					f.trailer["Encrypt"] = d["Encrypt"]
				}
				break
			}
		}
	}
	if f.trailer == nil {
		// No trailer at all: the catalog is the object that says it is.
		for num := range f.offsets {
			if d, ok := f.object(num).(dict); ok && d["Type"] == name("Catalog") {
				f.trailer = dict{"Root": ref{num, 0}}
			}
		}
	}
}

// object returns the object of a number, or nil.
func (f *file) object(num int) any {
	if v, ok := f.cache[num]; ok {
		return v
	}
	if f.loading[num] {
		return nil
	}
	f.loading[num] = true
	defer delete(f.loading, num)
	var v any
	if off, ok := f.offsets[num]; ok {
		v, _ = f.readObjectAt(off)
	} else if at, ok := f.packed[num]; ok {
		objs := f.objectStream(at[0])
		if at[1] < len(objs) {
			v = objs[at[1]]
		}
	}
	f.cache[num] = v
	return v
}

func (f *file) objectStreamHeader(num int) (data []byte, numbers, offsets []int, first int) {
	s, ok := f.object(num).(*stream)
	if !ok {
		return
	}
	data, err := decodeStream(f, s)
	if err != nil {
		return nil, nil, nil, 0
	}
	n, _ := f.resolve(s.dict["N"]).(float64)
	fo, _ := f.resolve(s.dict["First"]).(float64)
	first = int(fo)
	if n < 0 || n > 1e6 || first < 0 || first > len(data) {
		return nil, nil, nil, 0
	}
	l := &lexer{b: data[:first]}
	for i := 0; i < int(n); i++ {
		a, ok1 := mustNumber(l)
		b, ok2 := mustNumber(l)
		if !ok1 || !ok2 {
			break
		}
		numbers = append(numbers, a)
		offsets = append(offsets, b)
	}
	return data, numbers, offsets, first
}

func (f *file) objectStreamNumbers(num int) []int {
	_, numbers, _, _ := f.objectStreamHeader(num)
	return numbers
}

func (f *file) objectStream(num int) []any {
	if objs, ok := f.streams[num]; ok {
		return objs
	}
	f.streams[num] = nil
	data, _, offsets, first := f.objectStreamHeader(num)
	objs := make([]any, len(offsets))
	for i, off := range offsets {
		if first+off >= len(data) {
			continue
		}
		l := &lexer{b: data, p: first + off, refs: true}
		if v, err := l.next(); err == nil {
			if _, isOp := v.(op); !isOp {
				objs[i] = v
			}
		}
	}
	f.streams[num] = objs
	return objs
}

// resolve follows references to the object they name.
func (f *file) resolve(v any) any {
	for i := 0; i < 32; i++ {
		r, ok := v.(ref)
		if !ok {
			return v
		}
		v = f.object(r.num)
	}
	return nil
}

func (f *file) dict(v any) dict {
	switch x := f.resolve(v).(type) {
	case dict:
		return x
	case *stream:
		return x.dict
	}
	return nil
}

func (f *file) array(v any) []any {
	a, _ := f.resolve(v).([]any)
	return a
}

func (f *file) number(v any) (float64, bool) {
	n, ok := f.resolve(v).(float64)
	return n, ok
}

func (f *file) name(v any) name {
	n, _ := f.resolve(v).(name)
	return n
}

func (f *file) String() string {
	return fmt.Sprintf("pdf file, %d objects", len(f.offsets)+len(f.packed))
}
