// Package rtf reads RTF files as the tables of text that bilingual review
// files are: memoQ's two-column RTF and Déjà Vu's RTF external view hold
// one segment per table row. Every token keeps its place in the file, so a
// filter can rewrite the text of one cell and leave every other byte as it
// was, whether the file was written by the CAT tool or saved again by Word.
package rtf

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// ErrNotRTF is returned for data that does not start as an RTF file.
var ErrNotRTF = errors.New("rtf: not an RTF file")

// Kind is the kind of a token.
type Kind uint8

const (
	// Text is a run of characters, Open and Close are braces, Word a
	// control word (\b0), Symbol a control symbol (\~, \', \{), Binary the
	// bytes after \binN.
	Text Kind = iota + 1
	Open
	Close
	Word
	Symbol
	Binary
)

// Token is a piece of the file with its place: [Off, End) in the data.
type Token struct {
	Kind Kind
	Off  int
	End  int
	// Name is the control word ("par") or the symbol ("~", "'", "{").
	Name string
	// Param is a control word's number, or the byte of \'hh.
	Param    int
	HasParam bool
	// Raw is a text token's characters, without line ends.
	Raw string
}

// Tokenize cuts RTF data into tokens.
func Tokenize(data []byte) ([]Token, error) {
	if !bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n\ufeff"), []byte(`{\rtf`)) {
		return nil, ErrNotRTF
	}
	var toks []Token
	i := 0
	for i < len(data) {
		c := data[i]
		switch c {
		case '{':
			toks = append(toks, Token{Kind: Open, Off: i, End: i + 1})
			i++
		case '}':
			toks = append(toks, Token{Kind: Close, Off: i, End: i + 1})
			i++
		case '\\':
			t, next := control(data, i)
			if t.Kind == Word && t.Name == "bin" && t.HasParam && t.Param > 0 {
				end := next + t.Param
				if end > len(data) {
					end = len(data)
				}
				toks = append(toks, t, Token{Kind: Binary, Off: next, End: end})
				i = end
				continue
			}
			toks = append(toks, t)
			i = next
		case '\r', '\n':
			i++
		default:
			start := i
			var b strings.Builder
			for i < len(data) && data[i] != '{' && data[i] != '}' && data[i] != '\\' {
				if data[i] != '\r' && data[i] != '\n' {
					b.WriteByte(data[i])
				}
				i++
			}
			toks = append(toks, Token{Kind: Text, Off: start, End: i, Raw: b.String()})
		}
	}
	return toks, nil
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

// control reads the control word or symbol at data[i] (a backslash).
func control(data []byte, i int) (Token, int) {
	j := i + 1
	if j >= len(data) {
		return Token{Kind: Symbol, Off: i, End: j, Name: ""}, j
	}
	if !isLetter(data[j]) {
		if data[j] == '\'' {
			if j+2 < len(data) {
				hi, ok1 := hexVal(data[j+1])
				lo, ok2 := hexVal(data[j+2])
				if ok1 && ok2 {
					return Token{Kind: Symbol, Off: i, End: j + 3, Name: "'", Param: hi<<4 | lo, HasParam: true}, j + 3
				}
			}
			return Token{Kind: Symbol, Off: i, End: j + 1, Name: "'"}, j + 1
		}
		// A backslash before a line end is a paragraph mark.
		if data[j] == '\r' || data[j] == '\n' {
			end := j + 1
			if data[j] == '\r' && end < len(data) && data[end] == '\n' {
				end++
			}
			return Token{Kind: Word, Off: i, End: end, Name: "par"}, end
		}
		return Token{Kind: Symbol, Off: i, End: j + 1, Name: string(data[j])}, j + 1
	}
	k := j
	for k < len(data) && isLetter(data[k]) && k-j < 32 {
		k++
	}
	t := Token{Kind: Word, Off: i, Name: string(data[j:k])}
	p := k
	neg := false
	if p < len(data) && data[p] == '-' && p+1 < len(data) && isDigit(data[p+1]) {
		neg = true
		p++
	}
	q := p
	n := 0
	for q < len(data) && isDigit(data[q]) && q-p < 10 {
		n = n*10 + int(data[q]-'0')
		q++
	}
	if q > p {
		t.HasParam = true
		if neg {
			n = -n
		}
		t.Param = n
		k = q
	}
	if k < len(data) && data[k] == ' ' {
		k++
	}
	t.End = k
	return t, k
}

// skipped are destinations whose text is not part of the document.
var skipped = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true, "pict": true,
	"header": true, "headerl": true, "headerr": true, "headerf": true,
	"footer": true, "footerl": true, "footerr": true, "footerf": true,
	"footnote": true, "annotation": true, "listtable": true, "listoverridetable": true,
	"revtbl": true, "rsidtbl": true, "filetbl": true, "object": true, "xe": true, "tc": true,
	"txe": true, "fldinst": true, "datafield": true, "themedata": true, "colorschememapping": true,
	"latentstyles": true, "datastore": true, "xmlnstbl": true, "generator": true, "pgdsctbl": true,
	"nonshppict": true, "shpinst": true, "mmathPr": true, "userprops": true, "docvar": true,
	"bkmkstart": true, "bkmkend": true, "atnid": true, "atnauthor": true, "atndate": true,
}

// symbols are control words that stand for a character.
var symbols = map[string]string{
	"tab": "\t", "line": "\n", "par": "\n", "sect": "\n", "page": "\n", "column": "\n",
	"emdash": "\u2014", "endash": "\u2013", "emspace": "\u2003", "enspace": "\u2002",
	"qmspace": "\u2005", "bullet": "\u2022", "lquote": "\u2018", "rquote": "\u2019",
	"ldblquote": "\u201c", "rdblquote": "\u201d", "zwj": "\u200d", "zwnj": "\u200c",
	"zwbo": "\u200b", "zwnbo": "\ufeff", "ltrmark": "\u200e", "rtlmark": "\u200f",
}

// Codepage returns the decoder of a Windows code page (nil for one this
// package does not know, and for code pages decoded as Latin-1).
func Codepage(cp int) encoding.Encoding {
	switch cp {
	case 437:
		return charmap.CodePage437
	case 850:
		return charmap.CodePage850
	case 852:
		return charmap.CodePage852
	case 866:
		return charmap.CodePage866
	case 874:
		return charmap.Windows874
	case 932:
		return japanese.ShiftJIS
	case 936:
		return simplifiedchinese.GBK
	case 949:
		return korean.EUCKR
	case 950:
		return traditionalchinese.Big5
	case 1250:
		return charmap.Windows1250
	case 1251:
		return charmap.Windows1251
	case 1252:
		return charmap.Windows1252
	case 1253:
		return charmap.Windows1253
	case 1254:
		return charmap.Windows1254
	case 1255:
		return charmap.Windows1255
	case 1256:
		return charmap.Windows1256
	case 1257:
		return charmap.Windows1257
	case 1258:
		return charmap.Windows1258
	case 10000:
		return charmap.Macintosh
	}
	return nil
}

// charsetCodepage maps a font's \fcharset to its code page (0 for the
// document's own).
func charsetCodepage(cs int) int {
	switch cs {
	case 77:
		return 10000
	case 128:
		return 932
	case 129:
		return 949
	case 134:
		return 936
	case 136:
		return 950
	case 161:
		return 1253
	case 162:
		return 1254
	case 163:
		return 1258
	case 177:
		return 1255
	case 178:
		return 1256
	case 186:
		return 1257
	case 204:
		return 1251
	case 222:
		return 874
	case 238:
		return 1250
	case 254:
		return 437
	}
	return 0
}

// leadByte reports whether b starts a character of two bytes in a
// double-byte code page.
func leadByte(cp int, b byte) bool {
	switch cp {
	case 932:
		return b >= 0x81 && b <= 0x9f || b >= 0xe0 && b <= 0xfc
	case 936, 949, 950:
		return b >= 0x81 && b <= 0xfe
	}
	return false
}

func decodeBytes(cp int, b []byte) string {
	if enc := Codepage(cp); enc != nil {
		if s, err := enc.NewDecoder().Bytes(b); err == nil {
			return string(s)
		}
	}
	// Latin-1, which is what most RTF writers mean by default.
	var sb strings.Builder
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

// Run is text of one cell with the formatting a filter tells apart.
type Run struct {
	Text string
	// Style is the character style's number (\csN), -1 for none.
	Style int
	// Lang is the language's number (\langN), 0 for none.
	Lang int
	// Group is the token index of the group the run's text is in (its
	// "{"), -1 at the top level; Prefix the control words between that
	// brace and the text, as written.
	Group  int
	Prefix string
}

// Cell is a table cell: its runs, and the bytes it covers. [Start, End)
// starts after the previous cell or row mark and ends at this cell's \cell.
type Cell struct {
	Runs []Run
	// Lang is the language in force at the cell's end (\langN), which an
	// empty cell has as well.
	Lang       int
	Start, End int
	// First and Last are the indexes of its first token and of its \cell.
	First, Last int
}

// Text is the cell's text.
func (c Cell) Text() string {
	var b strings.Builder
	for _, r := range c.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// Row is a table row.
type Row struct {
	Cells []Cell
}

// Doc is an RTF file read as table rows.
type Doc struct {
	Data   []byte
	Tokens []Token
	Rows   []Row
	// Styles maps each character style's name to its number.
	Styles map[string]int
	// Codepage is the document's (\ansicpg), 1252 when it names none.
	Codepage int
}

type state struct {
	skip  bool
	uc    int
	cp    int
	style int
	lang  int
	group int
	// intbl: the paragraph is in a table; text outside tables is not kept.
	intbl bool
}

// Read reads the table rows of an RTF file. Text outside tables is not
// kept: the files this is for keep their segments in a table.
func Read(data []byte) (*Doc, error) {
	toks, err := Tokenize(data)
	if err != nil {
		return nil, err
	}
	d := &Doc{Data: data, Tokens: toks, Styles: map[string]int{}, Codepage: 1252}
	fonts := map[int]int{} // font number to code page
	d.readTables(fonts)
	d.readStyles()
	d.readRows(fonts)
	return d, nil
}

// readTables reads the font table and the document code page.
func (d *Doc) readTables(fonts map[int]int) {
	depth := 0
	inFonts := -1
	font, charset, cpg := -1, -1, 0
	for i, t := range d.Tokens {
		switch t.Kind {
		case Open:
			depth++
		case Close:
			if inFonts >= 0 && depth == inFonts+1 && font >= 0 {
				cp := cpg
				if cp == 0 && charset >= 0 {
					cp = charsetCodepage(charset)
				}
				fonts[font] = cp
				font, charset, cpg = -1, -1, 0
			}
			if depth == inFonts {
				return
			}
			depth--
		case Text:
			// Entries may be written without braces: \f0\fswiss Arial;
			if inFonts >= 0 && depth == inFonts && font >= 0 && strings.Contains(t.Raw, ";") {
				cp := cpg
				if cp == 0 && charset >= 0 {
					cp = charsetCodepage(charset)
				}
				fonts[font] = cp
				font, charset, cpg = -1, -1, 0
			}
		case Word:
			switch t.Name {
			case "ansicpg":
				if depth == 1 && t.HasParam && t.Param > 0 {
					d.Codepage = t.Param
				}
			case "fonttbl":
				if inFonts < 0 && i > 0 && d.Tokens[i-1].Kind == Open {
					inFonts = depth
				}
			case "f":
				if inFonts >= 0 && depth > inFonts && font < 0 {
					font = t.Param
				}
			case "fcharset":
				if inFonts >= 0 {
					charset = t.Param
				}
			case "cpg":
				if inFonts >= 0 {
					cpg = t.Param
				}
			case "pard", "trowd", "sectd":
				if inFonts < 0 {
					return
				}
			}
		}
	}
}

// readStyles reads the names of the character styles.
func (d *Doc) readStyles() {
	depth, in := 0, -1
	entry, cs := false, -1
	var name strings.Builder
	for i, t := range d.Tokens {
		switch t.Kind {
		case Open:
			depth++
			if in >= 0 && depth == in+1 {
				entry, cs = true, -1
				name.Reset()
			}
		case Close:
			if in >= 0 && depth == in+1 && entry {
				if cs >= 0 {
					n := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name.String()), ";"))
					if n != "" {
						d.Styles[n] = cs
					}
				}
				entry = false
			}
			if depth == in {
				return
			}
			depth--
		case Word:
			if t.Name == "stylesheet" && in < 0 && i > 0 && d.Tokens[i-1].Kind == Open {
				in = depth
			}
			if in >= 0 && entry && depth == in+1 && t.Name == "cs" {
				cs = t.Param
			}
		case Text:
			if in >= 0 && entry && depth == in+1 {
				name.WriteString(t.Raw)
			}
		}
	}
}

// readRows reads the document's text as rows of cells.
func (d *Doc) readRows(fonts map[int]int) {
	st := state{uc: 1, cp: d.Codepage, style: -1, group: -1}
	var stack []state
	var cell Cell
	var row Row
	cell.Start, cell.First = 0, 0
	skipChars := 0 // fallback characters still to skip after \u
	var pending []byte
	pendingCP := 0
	flush := func() {
		if len(pending) > 0 {
			s := decodeBytes(pendingCP, pending)
			pending = pending[:0]
			d.addText(&cell, st, s)
		}
	}
	add := func(s string) {
		flush()
		d.addText(&cell, st, s)
	}
	for i := 0; i < len(d.Tokens); i++ {
		t := d.Tokens[i]
		switch t.Kind {
		case Open:
			flush()
			stack = append(stack, st)
			st.group = i
			// A group that starts with \* is a destination to skip unless
			// this reader knows it.
			if i+1 < len(d.Tokens) && d.Tokens[i+1].Kind == Symbol && d.Tokens[i+1].Name == "*" {
				st.skip = true
			}
			continue
		case Close:
			flush()
			if len(stack) > 0 {
				st = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			skipChars = 0
			continue
		case Binary:
			continue
		}
		if st.skip {
			continue
		}
		switch t.Kind {
		case Text:
			s := t.Raw
			if skipChars > 0 {
				n := skipChars
				if n > len(s) {
					n = len(s)
				}
				s = s[n:]
				skipChars -= n
			}
			if s == "" {
				continue
			}
			if len(pending) == 1 && leadByte(pendingCP, pending[0]) {
				pending = append(pending, s[0])
				s = s[1:]
				flush()
			}
			if s != "" {
				add(decodeBytes(st.cp, []byte(s)))
			}
		case Symbol:
			if skipChars > 0 {
				skipChars--
				continue
			}
			switch t.Name {
			case "'":
				if !t.HasParam {
					continue
				}
				if len(pending) > 0 && pendingCP != st.cp {
					flush()
				}
				pendingCP = st.cp
				pending = append(pending, byte(t.Param))
				// A character is complete unless this byte leads a
				// double-byte one.
				if !(len(pending) == 1 && leadByte(st.cp, pending[0])) {
					flush()
				}
			case "~":
				add("\u00a0")
			case "_":
				add("\u2011")
			case "-":
				add("\u00ad")
			case "\\", "{", "}":
				add(t.Name)
			case "*":
			}
		case Word:
			if skipped[t.Name] {
				st.skip = true
				continue
			}
			switch t.Name {
			case "u":
				if !t.HasParam {
					continue
				}
				n := t.Param
				if n < 0 {
					n += 65536
				}
				flush()
				// A surrogate pair is two \u words.
				if utf16.IsSurrogate(rune(n)) && n < 0xdc00 {
					lowTok, ok := d.nextLow(i)
					if ok {
						r := utf16.DecodeRune(rune(n), rune(lowTok))
						d.addText(&cell, st, string(r))
						skipChars = st.uc
						// The fallback of the high half comes before the low \u.
						i = d.lowIndex(i)
						continue
					}
				}
				if utf8.ValidRune(rune(n)) {
					d.addText(&cell, st, string(rune(n)))
				}
				skipChars = st.uc
			case "uc":
				st.uc = t.Param
			case "f":
				if cp, ok := fonts[t.Param]; ok && cp != 0 {
					st.cp = cp
				} else {
					st.cp = d.Codepage
				}
			case "intbl":
				st.intbl = true
			case "pard":
				st.intbl = false
			case "itap":
				st.intbl = t.Param > 0
			case "cs":
				st.style = t.Param
			case "plain":
				st.style = -1
				st.lang = 0
				st.cp = d.Codepage
			case "lang":
				st.lang = t.Param
			case "cell":
				flush()
				cell.End = t.Off
				cell.Last = i
				cell.Lang = st.lang
				row.Cells = append(row.Cells, cell)
				cell = Cell{Start: t.End, First: i + 1}
			case "row":
				flush()
				if len(row.Cells) > 0 {
					d.Rows = append(d.Rows, row)
				}
				row = Row{}
				cell = Cell{Start: t.End, First: i + 1}
			case "nestcell":
				add(" ")
			case "nestrow":
				add("\n")
			default:
				if s, ok := symbols[t.Name]; ok {
					if skipChars > 0 {
						skipChars--
						continue
					}
					add(s)
				}
			}
		}
	}
}

// nextLow returns the value of the \u word with a low surrogate that
// follows the one at i, after its fallback characters.
func (d *Doc) nextLow(i int) (int, bool) {
	for j := i + 1; j < len(d.Tokens) && j < i+6; j++ {
		t := d.Tokens[j]
		if t.Kind == Word && t.Name == "u" && t.HasParam {
			n := t.Param
			if n < 0 {
				n += 65536
			}
			if n >= 0xdc00 && n <= 0xdfff {
				return n, true
			}
			return 0, false
		}
		if t.Kind == Open || t.Kind == Close {
			return 0, false
		}
	}
	return 0, false
}

// lowIndex returns the index of the low \u after the high one at i.
func (d *Doc) lowIndex(i int) int {
	for j := i + 1; j < len(d.Tokens) && j < i+6; j++ {
		t := d.Tokens[j]
		if t.Kind == Word && t.Name == "u" {
			return j
		}
	}
	return i
}

func (d *Doc) addText(c *Cell, st state, s string) {
	if s == "" || !st.intbl {
		return
	}
	if n := len(c.Runs); n > 0 {
		last := &c.Runs[n-1]
		if last.Style == st.style && last.Lang == st.lang && last.Group == st.group {
			last.Text += s
			return
		}
	}
	prefix := ""
	if st.group >= 0 {
		var b strings.Builder
		for j := st.group + 1; j < len(d.Tokens); j++ {
			t := d.Tokens[j]
			if t.Kind != Word && !(t.Kind == Symbol && t.Name == "*") {
				break
			}
			b.Write(d.Data[t.Off:t.End])
		}
		prefix = b.String()
	}
	c.Runs = append(c.Runs, Run{Text: s, Style: st.style, Lang: st.lang, Group: st.group, Prefix: prefix})
}

// textual reports whether a token adds text to a cell: characters,
// character symbols and the control words that stand for characters.
func textual(t Token) bool {
	switch t.Kind {
	case Text:
		return true
	case Symbol:
		switch t.Name {
		case "'", "~", "_", "-", "\\", "{", "}":
			return true
		}
	case Word:
		if t.Name == "u" {
			return true
		}
		_, ok := symbols[t.Name]
		return ok
	}
	return false
}

// ClearCell returns the bytes of a cell without its text: every group and
// formatting word stays, so what is written in it next keeps the cell's
// formatting. Destinations in the cell (bookmarks, pictures, a field's
// instructions) stay as they are; a field's shown text goes with the rest.
func (d *Doc) ClearCell(c Cell) []byte {
	var b bytes.Buffer
	last := c.Start
	depth := 0
	keep := -1 // depth of a destination kept whole
	for j := c.First; j < c.Last && j < len(d.Tokens); j++ {
		t := d.Tokens[j]
		gap := d.Data[last:t.Off]
		last = t.End
		switch t.Kind {
		case Open:
			depth++
			if keep < 0 && j+1 < len(d.Tokens) {
				n := d.Tokens[j+1]
				if n.Kind == Symbol && n.Name == "*" || n.Kind == Word && skipped[n.Name] {
					keep = depth
				}
			}
		case Close:
			if keep == depth {
				keep = -1
			}
			depth--
		default:
			if keep < 0 && textual(t) {
				// A control word goes with its delimiting space; line
				// ends between tokens are kept.
				b.Write(gap)
				continue
			}
		}
		b.Write(gap)
		b.Write(d.Data[t.Off:t.End])
	}
	b.Write(d.Data[last:c.End])
	return b.Bytes()
}

// Escape writes text as RTF: braces and backslashes escaped, line breaks
// as \line, tabs as \tab, characters outside ASCII as \u with a "?" for
// readers that do not know it. The text must be written where \uc is 1,
// as it is inside a group that starts with \uc1.
func Escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '{' || r == '}':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\line `)
		case r == '\t':
			b.WriteString(`\tab `)
		case r == '\u00a0':
			b.WriteString(`\~`)
		case r == '\u00ad':
			b.WriteString(`\-`)
		case r == '\u2011':
			b.WriteString(`\_`)
		case r < 0x20:
		case r < 0x80:
			b.WriteRune(r)
		case r > 0xffff:
			hi, lo := utf16.EncodeRune(r)
			b.WriteString(uword(int(hi)))
			b.WriteString(uword(int(lo)))
		default:
			b.WriteString(uword(int(r)))
		}
	}
	return b.String()
}

func uword(n int) string {
	if n > 32767 {
		n -= 65536
	}
	return `\u` + strconv.Itoa(n) + `?`
}
