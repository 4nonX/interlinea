// Package ebustl reads and writes EBU STL subtitles (EBU Tech 3264), the
// binary format European broadcasters exchange subtitles in, for teletext
// and for subtitles burnt into the picture (open subtitles).
//
// A file is a General Subtitle Information block (GSI, 1024 bytes) and Text
// and Timing Information blocks (TTI, 128 bytes each). A subtitle is one
// TTI block, or several (extension blocks) when its text is longer than one
// block holds. Each subtitle is one unit. Its rows are kept apart by
// tags: the row break (CR/LF, 8Ah) with the control codes around it, such
// as teletext's double height and boxing. Control codes inside a row are
// tags too: italics, underline and boxing as pairs, teletext colours on
// their own. Those at the start and end of the text are kept out of sight.
//
// On merge, each translated subtitle's text is written in the character
// set of the file (Latin, ISO 6937), or in the first of the code tables
// Latin/Cyrillic, Latin/Arabic, Latin/Greek and Latin/Hebrew that holds the
// translation; the blocks of a subtitle are written again as many as its
// text needs, a subtitle with more or fewer rows moves up or down so its
// last row stays where it was, and the GSI block gets the number of blocks,
// the longest row, the code table and the language of the translation.
// Comments (CF 1) and user data blocks (EBN FEh) stay as they are.
//
// The original programme and episode titles of the GSI block are units
// too, at most 32 characters each; their translations are written to the
// translated programme and episode titles, in the GSI block's code page
// (437, 850, 860, 863 or 865). A title in letters the code page lacks is
// left out, as the format can't hold it.
package ebustl

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"golang.org/x/text/encoding/charmap"
)

// Format is the name stored with a project's files.
const Format = "stl"

const (
	gsiSize  = 1024
	ttiSize  = 128
	textSize = 112

	crlf   = 0x8A
	unused = 0x8F
)

// block is a TTI block.
type block []byte

func (b block) sn() int          { return int(b[1]) | int(b[2])<<8 }
func (b block) ebn() byte        { return b[3] }
func (b block) comment() bool    { return b[15] == 1 }
func (b block) text() []byte     { return b[16:] }
func (b block) vp() int          { return int(b[13]) }
func (b block) timeIn() string   { return timecode(b[5:9]) }
func (b block) timeOut() string  { return timecode(b[9:13]) }
func (b block) userData() bool   { return b[3] == 0xFE }
func (b block) lastOfSet() bool  { return b[3] == 0xFF }
func timecode(t []byte) string   { return fmt.Sprintf("%02d:%02d:%02d:%02d", t[0], t[1], t[2], t[3]) }
func (b block) clone() block     { return append(block(nil), b...) }
func (b block) setVP(v int)      { b[13] = byte(v) }
func (b block) setEBN(n byte)    { b[3] = n }
func (b block) setText(t []byte) { copy(b[16:], t) }

// subtitle is a subtitle of the file: its blocks, which follow each other.
type subtitle struct {
	// first and count are its blocks' place in the list of blocks.
	first, count int
	// raw is its text: the text fields of its blocks, without the unused
	// space that ends it.
	raw []byte
	id  string
	// translatable is false for comments and subtitles without text.
	translatable bool
}

type file struct {
	gsi    []byte
	blocks []block
	subs   []*subtitle
	table  byte // the character code table, 0 to 4
	tele   bool // teletext, where a row of double height takes two
}

func parse(data []byte) (*file, error) {
	if len(data) < gsiSize || !bytes.HasPrefix(data[3:11], []byte("STL")) {
		return nil, fmt.Errorf("the file is not EBU STL: it does not start with a General Subtitle Information block")
	}
	if (len(data)-gsiSize)%ttiSize != 0 {
		return nil, fmt.Errorf("the file is not EBU STL: its subtitle blocks are not 128 bytes each")
	}
	f := &file{gsi: append([]byte(nil), data[:gsiSize]...)}
	cct := strings.TrimSpace(string(f.gsi[12:14]))
	switch cct {
	case "", "00":
		f.table = 0
	case "01", "02", "03", "04":
		f.table = cct[1] - '0'
	default:
		return nil, fmt.Errorf("the file uses the character code table %q, which EBU STL does not have", cct)
	}
	f.tele = f.gsi[11] == '1' || f.gsi[11] == '2'
	for at := gsiSize; at < len(data); at += ttiSize {
		f.blocks = append(f.blocks, block(append([]byte(nil), data[at:at+ttiSize]...)))
	}
	// The blocks of a subtitle run up to the one marked last (EBN FFh),
	// past user data blocks.
	var cur *subtitle
	for i, b := range f.blocks {
		if b.userData() {
			continue
		}
		if cur == nil || cur.first+cur.count != i {
			cur = &subtitle{first: i}
		}
		cur.count++
		cur.raw = append(cur.raw, b.text()...)
		if b.lastOfSet() || b.ebn() > 0xEF || i == len(f.blocks)-1 {
			f.close(cur)
			cur = nil
		}
	}
	if cur != nil {
		f.close(cur)
	}
	return f, nil
}

func (f *file) close(s *subtitle) {
	if i := bytes.IndexByte(s.raw, unused); i >= 0 {
		s.raw = s.raw[:i]
	}
	s.id = "t" + strconv.Itoa(len(f.subs)+1)
	first := f.blocks[s.first]
	s.translatable = !first.comment() && hasText(f.table, s.raw)
	f.subs = append(f.subs, s)
}

// isControl reports whether a byte of a text field is a control code:
// teletext's (00h to 1Fh) and those of open subtitles (80h to 8Fh).
func isControl(c byte) bool { return c < 0x20 || (c >= 0x80 && c <= 0x9F) }

func hasText(table byte, raw []byte) bool {
	for _, c := range raw {
		if !isControl(c) && c != ' ' {
			return true
		}
	}
	return false
}

// token is a piece of a subtitle's text: text, or control codes.
type token struct {
	text  string
	codes []byte
	// brk is set for the codes between two rows.
	brk bool
}

// tokens cuts a subtitle's text into its edges, kept out of sight, and
// what is between them.
func tokens(table byte, raw []byte) (lead []byte, body []token, trail []byte, err error) {
	var all []token
	for i := 0; i < len(raw); {
		if isControl(raw[i]) {
			j := i
			for j < len(raw) && isControl(raw[j]) {
				j++
			}
			all = append(all, token{codes: raw[i:j], brk: bytes.IndexByte(raw[i:j], crlf) >= 0})
			i = j
			continue
		}
		j := i
		for j < len(raw) && !isControl(raw[j]) {
			j++
		}
		s, err := decodeText(table, raw[i:j])
		if err != nil {
			return nil, nil, nil, err
		}
		all = append(all, token{text: s})
		i = j
	}
	// The codes before the first text and after the last are the edges;
	// so are spaces, which teletext puts after its codes.
	for len(all) > 0 && (all[0].codes != nil || strings.TrimSpace(all[0].text) == "") {
		if all[0].codes != nil {
			lead = append(lead, all[0].codes...)
		} else {
			b, _ := encodeText(table, all[0].text)
			lead = append(lead, b...)
		}
		all = all[1:]
	}
	var tail []byte
	for len(all) > 0 && (all[len(all)-1].codes != nil || strings.TrimSpace(all[len(all)-1].text) == "") {
		t := all[len(all)-1]
		var b []byte
		if t.codes != nil {
			b = t.codes
		} else {
			b, _ = encodeText(table, t.text)
		}
		tail = append(append([]byte(nil), b...), tail...)
		all = all[:len(all)-1]
	}
	// The spaces at a row's edges belong with the break between rows.
	for i := range all {
		if !all[i].brk {
			continue
		}
		if i > 0 && all[i-1].codes == nil {
			t := strings.TrimRight(all[i-1].text, " ")
			pad := strings.Repeat(" ", len(all[i-1].text)-len(t))
			all[i-1].text = t
			all[i].codes = append([]byte(pad), all[i].codes...)
		}
		if i+1 < len(all) && all[i+1].codes == nil {
			t := strings.TrimLeft(all[i+1].text, " ")
			pad := strings.Repeat(" ", len(all[i+1].text)-len(t))
			all[i+1].text = t
			all[i].codes = append(append([]byte(nil), all[i].codes...), pad...)
		}
	}
	return lead, all, tail, nil
}

// codeName says what control codes do, for a translator.
func codeName(codes []byte) string {
	var names []string
	for _, c := range codes {
		switch {
		case c == crlf:
			names = append(names, "new row")
		case c == 0x80:
			names = append(names, "italics on")
		case c == 0x81:
			names = append(names, "italics off")
		case c == 0x82:
			names = append(names, "underline on")
		case c == 0x83:
			names = append(names, "underline off")
		case c == 0x84:
			names = append(names, "boxing on")
		case c == 0x85:
			names = append(names, "boxing off")
		case c <= 0x07:
			names = append(names, [...]string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}[c])
		case c == 0x0A:
			names = append(names, "end box")
		case c == 0x0B:
			names = append(names, "start box")
		case c == 0x0C:
			names = append(names, "normal height")
		case c == 0x0D:
			names = append(names, "double height")
		case c == ' ':
		default:
			names = append(names, fmt.Sprintf("code %02Xh", c))
		}
	}
	return strings.Join(names, ", ")
}

// pairOf says which pair a control code opens or closes: italics,
// underline or boxing.
func pairOf(codes []byte) (name string, open, ok bool) {
	if len(codes) != 1 || codes[0] < 0x80 || codes[0] > 0x85 {
		return "", false, false
	}
	return [...]string{"italics", "underline", "boxing"}[(codes[0]-0x80)/2], codes[0]%2 == 0, true
}

// unit is a subtitle as content, with the codes of each of its tags.
func unit(table byte, raw []byte) (c content.Content, lead, trail []byte, codes map[string][]byte, err error) {
	lead, toks, trail, err := tokens(table, raw)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	codes = map[string][]byte{}
	open := map[string][]string{}
	n := 0
	for _, t := range toks {
		if t.codes == nil {
			c = c.Append(t.text)
			continue
		}
		n++
		id := strconv.Itoa(n)
		tag := content.Tag{Kind: content.TagStandalone, ID: id, Type: "code", Desc: codeName(t.codes), Native: fmt.Sprintf("% X", t.codes)}
		switch name, on, ok := pairOf(t.codes); {
		case t.brk:
			tag.Type, tag.Desc = "break", "row break ("+codeName(t.codes)+")"
		case ok && on:
			tag.Kind, tag.Type = content.TagOpen, name
			open[name] = append(open[name], id)
		case ok && len(open[name]) > 0:
			tag.Kind, tag.Type, tag.ID = content.TagClose, name, open[name][len(open[name])-1]
			open[name] = open[name][:len(open[name])-1]
			n--
			id = tag.ID
			codes["close:"+id] = t.codes
			c = c.AppendTag(tag)
			continue
		}
		codes[tag.Kind.String()+":"+id] = t.codes
		c = c.AppendTag(tag)
	}
	// A pair left open is a tag on its own.
	for i, r := range c {
		if r.Tag != nil && r.Tag.Kind == content.TagOpen && contains(open[r.Tag.Type], r.Tag.ID) {
			codes["standalone:"+r.Tag.ID] = codes["open:"+r.Tag.ID]
			c[i].Tag.Kind = content.TagStandalone
		}
	}
	if c == nil {
		c = content.Content{}
	}
	return c, lead, trail, codes, nil
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

// The titles of the GSI block: where the original is, where the
// translation goes.
var titles = []struct {
	id, name   string
	from, into int
}{
	{"programme-title", "Programme title", 16, 80},
	{"episode-title", "Episode title", 48, 112},
}

const titleSize = 32

// codePage is the code page of the GSI block's text.
func (f *file) codePage() *charmap.Charmap {
	switch string(f.gsi[0:3]) {
	case "437":
		return charmap.CodePage437
	case "860":
		return charmap.CodePage860
	case "863":
		return charmap.CodePage863
	case "865":
		return charmap.CodePage865
	}
	return charmap.CodePage850
}

func (f *file) title(at int) string {
	s, _ := f.codePage().NewDecoder().Bytes(bytes.TrimRight(f.gsi[at:at+titleSize], " \x00"))
	return strings.TrimSpace(string(s))
}

// fps is the file's frames per second.
func (f *file) fps() int64 {
	if string(f.gsi[3:11]) == "STL30.01" {
		return 30
	}
	return 25
}

// ms is a time code in milliseconds.
func (f *file) ms(t []byte) int64 {
	return ((int64(t[0])*60+int64(t[1]))*60+int64(t[2]))*1000 + int64(t[3])*1000/f.fps()
}

// Extract returns the subtitles of an EBU STL file as units.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	ex := &filter.Extraction{Units: []filter.Unit{}, Blocks: []filter.Block{}, Format: Format, SourceLang: lang}
	for _, t := range titles {
		if text := f.title(t.from); text != "" {
			ex.Blocks = append(ex.Blocks, filter.Block{ID: t.id, Part: filter.PartMeta})
			ex.Units = append(ex.Units, filter.Unit{ID: t.id, Block: t.id, Source: content.Text(text), Key: t.name, MaxLength: titleSize,
				Notes: []string{t.name + " of the file's header. Its translation goes into the translated " + strings.ToLower(t.name) + "."}})
		}
	}
	for _, s := range f.subs {
		if !s.translatable {
			continue
		}
		c, _, _, _, err := unit(f.table, s.raw)
		if err != nil {
			return nil, fmt.Errorf("subtitle %d: %v", f.blocks[s.first].sn(), err)
		}
		b := f.blocks[s.first]
		key := b.timeIn() + " --> " + b.timeOut()
		ex.Blocks = append(ex.Blocks, filter.Block{ID: s.id, Part: filter.PartBody})
		tm := &filter.Timing{Start: f.ms(b[5:9]), End: f.ms(b[9:13])}
		ex.Units = append(ex.Units, filter.Unit{ID: s.id, Block: s.id, Source: c, Key: key, Notes: []string{key}, Timing: tm})
	}
	return ex, nil
}

// Merge returns the file with its subtitles translated.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	// The texts, as Unicode first: the code table is chosen for all of them.
	type written struct {
		s     *subtitle
		parts []part
	}
	var out []written
	var all [][]part
	for _, s := range f.subs {
		t, ok := tr[s.id]
		if !ok || len(t) == 0 || !s.translatable {
			continue
		}
		src, lead, trail, codes, err := unit(f.table, s.raw)
		if err != nil {
			return nil, err
		}
		w := written{s, compose(t, src, lead, trail, codes, f.tele)}
		out = append(out, w)
		all = append(all, w.parts)
	}
	_, titled := tr["programme-title"]
	_, titled2 := tr["episode-title"]
	if len(out) == 0 && targetLang == "" && !titled && !titled2 {
		return append([]byte(nil), data...), nil
	}
	table, err := f.chooseTable(all)
	if err != nil {
		return nil, err
	}
	// The subtitles not translated are written again only when the table
	// changes.
	texts := map[*subtitle][]byte{}
	for _, w := range out {
		texts[w.s], _ = render(table, w.parts)
	}
	if table != f.table {
		for _, s := range f.subs {
			if _, ok := texts[s]; ok || !s.translatable {
				continue
			}
			src, lead, trail, codes, err := unit(f.table, s.raw)
			if err != nil {
				return nil, err
			}
			b, err := render(table, compose(src, src, lead, trail, codes, f.tele))
			if err != nil {
				return nil, err
			}
			texts[s] = b
		}
	}
	var blocks []block
	longest := 0
	next := 0
	for _, s := range f.subs {
		for ; next < s.first; next++ {
			blocks = append(blocks, f.blocks[next]) // user data before it
		}
		next = s.first + s.count
		raw, ok := texts[s]
		if !ok {
			for i := s.first; i < s.first+s.count; i++ {
				blocks = append(blocks, f.blocks[i])
			}
			if s.translatable {
				longest = max(longest, rowWidth(f.table, s.raw, f.tele))
			}
			continue
		}
		longest = max(longest, rowWidth(table, raw, f.tele))
		head := f.blocks[s.first].clone()
		// More or fewer rows: the subtitle's last row stays where it was,
		// for subtitles in the lower half of the screen.
		if delta := bytes.Count(raw, []byte{crlf}) - bytes.Count(s.raw, []byte{crlf}); delta != 0 && head.vp() > f.rows()/2 {
			low := 0
			if f.tele {
				low = 1
			}
			head.setVP(max(low, head.vp()-delta))
		}
		chunks := split(raw)
		for i, ch := range chunks {
			b := head.clone()
			if i == len(chunks)-1 {
				b.setEBN(0xFF)
			} else {
				b.setEBN(byte(i))
			}
			field := bytes.Repeat([]byte{unused}, textSize)
			copy(field, ch)
			b.setText(field)
			blocks = append(blocks, b)
		}
	}
	for ; next < len(f.blocks); next++ {
		blocks = append(blocks, f.blocks[next])
	}

	gsi := append([]byte(nil), f.gsi...)
	if len(blocks) > 99999 {
		return nil, fmt.Errorf("the translation needs %d subtitle blocks; an EBU STL file holds at most 99999", len(blocks))
	}
	copy(gsi[238:243], fmt.Sprintf("%05d", len(blocks)))
	if old, err := strconv.Atoi(strings.TrimSpace(string(gsi[251:253]))); err != nil || longest > old {
		if longest > 99 {
			longest = 99
		}
		copy(gsi[251:253], fmt.Sprintf("%02d", longest))
	}
	copy(gsi[12:14], fmt.Sprintf("%02d", table))
	for _, t := range titles {
		tt, ok := tr[t.id]
		if !ok || len(tt) == 0 || f.title(t.from) == "" {
			continue
		}
		b, err := f.codePage().NewEncoder().Bytes([]byte(strings.TrimSpace(tt.Plain())))
		if err != nil || len(b) > titleSize {
			// Letters the code page lacks, or too long: the field can't
			// hold it.
			continue
		}
		field := bytes.Repeat([]byte(" "), titleSize)
		copy(field, b)
		copy(gsi[t.into:t.into+titleSize], field)
	}
	if lc, ok := languageCode(targetLang); ok {
		copy(gsi[14:16], lc)
	}
	var buf bytes.Buffer
	buf.Write(gsi)
	for _, b := range blocks {
		buf.Write(b)
	}
	return buf.Bytes(), nil
}

// rows is the number of rows the file has, for the vertical position.
func (f *file) rows() int {
	if f.tele {
		return 23
	}
	if n, err := strconv.Atoi(strings.TrimSpace(string(f.gsi[253:255]))); err == nil && n > 0 {
		return n
	}
	return 23
}

// part is a piece of a translated subtitle: text, or control codes.
type part struct {
	text  string
	codes []byte
}

// compose puts a translation together: its text, and for each of its
// tags the codes of the source's tag of that number. A line break typed
// in the translation is a new row, made as the source's first row break
// is, or as a plain CR/LF where it has none.
func compose(t, src content.Content, lead, trail []byte, codes map[string][]byte, tele bool) []part {
	brk := []byte{crlf}
	if tele {
		brk = []byte{crlf, crlf}
	}
	for _, r := range src {
		if r.Tag != nil && r.Tag.Type == "break" {
			brk = codes[r.Tag.Kind.String()+":"+r.Tag.ID]
			break
		}
	}
	parts := []part{{codes: lead}}
	for _, r := range t {
		if r.Tag != nil {
			if c, ok := codes[r.Tag.Kind.String()+":"+r.Tag.ID]; ok {
				parts = append(parts, part{codes: c})
			}
			continue
		}
		for i, row := range strings.Split(r.Text, "\n") {
			if i > 0 {
				parts = append(parts, part{codes: brk})
			}
			parts = append(parts, part{text: row})
		}
	}
	return append(parts, part{codes: trail})
}

// render writes the parts of a subtitle in a code table.
func render(table byte, parts []part) ([]byte, error) {
	var out []byte
	for _, p := range parts {
		if p.codes != nil {
			out = append(out, p.codes...)
			continue
		}
		b, err := encodeText(table, p.text)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// chooseTable is the file's code table if it holds every translation,
// or else the first that does.
func (f *file) chooseTable(all [][]part) (byte, error) {
	fits := func(table byte) bool {
		for _, parts := range all {
			if _, err := render(table, parts); err != nil {
				return false
			}
		}
		return true
	}
	if fits(f.table) {
		return f.table, nil
	}
	for table := byte(0); table <= 4; table++ {
		if fits(table) {
			return table, nil
		}
	}
	for _, parts := range all {
		for _, p := range parts {
			for _, r := range p.text {
				if _, err := encodeText(0, string(r)); err != nil {
					ok := false
					for table := byte(1); table <= 4; table++ {
						if _, err := encodeText(table, string(r)); err == nil {
							ok = true
						}
					}
					if !ok {
						return 0, fmt.Errorf("EBU STL can't hold the character %q: its character sets are Latin, Cyrillic, Arabic, Greek and Hebrew", r)
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("the translations need characters of more than one of EBU STL's character sets (Latin with Cyrillic, Arabic, Greek or Hebrew), which one file can't hold")
}

// split cuts a subtitle's text into the text fields of its blocks,
// keeping an accent with its letter.
func split(raw []byte) [][]byte {
	var out [][]byte
	for len(raw) > textSize {
		n := textSize
		if raw[n-1] >= 0xC1 && raw[n-1] <= 0xCF {
			n--
		}
		out = append(out, raw[:n])
		raw = raw[n:]
	}
	return append(out, raw)
}

// rowWidth is the most characters any row of a subtitle shows. Teletext
// shows its control codes as spaces; an accent and its letter are one.
func rowWidth(table byte, raw []byte, tele bool) int {
	longest := 0
	for _, row := range bytes.Split(raw, []byte{crlf}) {
		n := 0
		for i := 0; i < len(row); i++ {
			c := row[i]
			switch {
			case c >= 0x80 && c <= 0x9F:
			case c < 0x20:
				if tele {
					n++
				}
			case table == 0 && c >= 0xC1 && c <= 0xCF && i+1 < len(row):
				// The accent is shown on the next letter.
			default:
				n++
			}
		}
		longest = max(longest, n)
	}
	return longest
}
