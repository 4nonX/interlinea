// Package textfmt reads the text formats of software and subtitles:
// Java properties, JSON, YAML, gettext PO, .NET RESX, and SubRip, WebVTT,
// YouTube (.sbv), Advanced SubStation Alpha (.ass, .ssa) and MicroDVD
// (.sub) subtitles. Each string of a file is one unit; it is not cut into
// sentences, as a string is what the program shows in one place. The
// placeholders a program fills in (%s, {0}, {{name}}, ${name}) and the
// markup inside a string are tags, so they can't be mistyped or lost.
//
// A file is read into slots: the places a translation goes, each with the
// range of bytes it replaces and the way its text is written in the
// file's syntax. Everything outside the slots stays byte for byte.
package textfmt

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"golang.org/x/text/encoding/charmap"
)

// The formats, by the names stored with a project's files.
const (
	Properties = "properties"
	JSON       = "json"
	YAML       = "yaml"
	PO         = "po"
	RESX       = "resx"
	SRT        = "srt"
	VTT        = "vtt"
	SBV        = "sbv"
	ASS        = "ass"
	MicroDVD   = "sub"
)

// slot is a place a translation goes.
type slot struct {
	// start and end are the bytes of the file the translation replaces.
	start, end int
	id         string
	text       string
	// key is the string's name in the file and notes what the file says
	// about it for translators.
	key   string
	notes []string
	// write renders a text in the file's syntax for this place.
	write func(string) string
	// markup is set where a string holds no program placeholders but
	// subtitle markup: its tags are pairs, and its lines are kept apart
	// by tags.
	markup bool
	// tags, if set, finds the subtitle markup in place of subtitleTag.
	tags *regexp.Regexp
	// timing is when a subtitle is shown.
	timing *filter.Timing
	// also are further places that get the same translation (a plural's
	// other forms).
	also []*slot
}

type doc struct {
	src   string
	enc   encoding
	slots []*slot
}

type encoding int

const (
	utf8Plain encoding = iota
	utf8BOM
	utf16LE
	utf16BE
	latin1
	// windows1252 is the legacy encoding of subtitles made on Windows.
	windows1252
)

// decode reads the file as UTF-8, as UTF-16 with a byte order mark, or,
// for the formats that allow it, as ISO 8859-1 or, for subtitles, as
// Windows-1252.
func decode(data []byte, allowLatin1 bool) (string, encoding, error) {
	return decodeAs(data, allowLatin1, false)
}

func decodeAs(data []byte, allowLatin1, allow1252 bool) (string, encoding, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		if !utf8.Valid(data[3:]) {
			return "", 0, fmt.Errorf("the file is not valid UTF-8")
		}
		return string(data[3:]), utf8BOM, nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		body := data[2:]
		if len(body)%2 != 0 {
			return "", 0, fmt.Errorf("the UTF-16 text has an odd number of bytes")
		}
		u := make([]uint16, len(body)/2)
		for i := range u {
			if le {
				u[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
			} else {
				u[i] = uint16(body[2*i])<<8 | uint16(body[2*i+1])
			}
		}
		if le {
			return string(utf16.Decode(u)), utf16LE, nil
		}
		return string(utf16.Decode(u)), utf16BE, nil
	}
	if utf8.Valid(data) {
		return string(data), utf8Plain, nil
	}
	if allow1252 {
		s, err := charmap.Windows1252.NewDecoder().String(string(data))
		if err != nil {
			return "", 0, fmt.Errorf("the file is not UTF-8, UTF-16 or Windows-1252")
		}
		return s, windows1252, nil
	}
	if !allowLatin1 {
		return "", 0, fmt.Errorf("the file is not UTF-8 or UTF-16")
	}
	var b strings.Builder
	for _, c := range data {
		b.WriteRune(rune(c))
	}
	return b.String(), latin1, nil
}

func encode(s string, enc encoding) []byte {
	switch enc {
	case utf8BOM:
		return append([]byte{0xEF, 0xBB, 0xBF}, s...)
	case utf16LE, utf16BE:
		u := utf16.Encode([]rune(s))
		out := make([]byte, 2, 2+2*len(u))
		if enc == utf16LE {
			out[0], out[1] = 0xFF, 0xFE
		} else {
			out[0], out[1] = 0xFE, 0xFF
		}
		for _, x := range u {
			if enc == utf16LE {
				out = append(out, byte(x), byte(x>>8))
			} else {
				out = append(out, byte(x>>8), byte(x))
			}
		}
		return out
	case windows1252:
		// Where the translation has letters the file's encoding lacks,
		// the file is written in UTF-8, which subtitle players read.
		if b, err := charmap.Windows1252.NewEncoder().String(s); err == nil {
			return []byte(b)
		}
		return append([]byte{0xEF, 0xBB, 0xBF}, s...)
	case latin1:
		out := make([]byte, 0, len(s))
		for _, r := range s {
			if r > 0xFF {
				r = '?'
			}
			out = append(out, byte(r))
		}
		return out
	}
	return []byte(s)
}

// Is reports whether the format is one of this package's.
func Is(format string) bool {
	switch format {
	case Properties, JSON, YAML, PO, RESX, SRT, VTT, SBV, ASS, MicroDVD:
		return true
	}
	return false
}

// isSubtitles reports whether the format is one of subtitles.
func isSubtitles(format string) bool {
	switch format {
	case SRT, VTT, SBV, ASS, MicroDVD:
		return true
	}
	return false
}

func parse(format string, data []byte) (*doc, error) {
	if format == MicroDVD && bytes.HasPrefix(data, []byte{0, 0, 1, 0xBA}) {
		return nil, fmt.Errorf("the file is VobSub: its subtitles are pictures, with no text to translate")
	}
	src, enc, err := decodeAs(data, format == Properties || format == PO, isSubtitles(format))
	if err != nil {
		return nil, err
	}
	d := &doc{src: src, enc: enc}
	switch format {
	case Properties:
		d.slots, err = parseProperties(src, enc == latin1)
	case JSON:
		d.slots, err = parseJSON(src)
	case YAML:
		d.slots, err = parseYAML(src)
	case PO:
		d.slots, err = parsePO(src)
	case RESX:
		d.slots, err = parseRESX(src)
	case SRT, VTT:
		d.slots, err = parseSubtitles(src, format == VTT)
	case SBV:
		d.slots, err = parseSBV(src)
	case ASS:
		d.slots, err = parseASS(src)
	case MicroDVD:
		d.slots, err = parseMicroDVD(src)
	default:
		return nil, fmt.Errorf("textfmt: %q is not a text format", format)
	}
	if err != nil {
		return nil, err
	}
	for i, s := range d.slots {
		if s.id == "" {
			s.id = fmt.Sprintf("t%d", i+1)
		}
	}
	return d, nil
}

// placeholder finds what a program fills in or reads as a command:
// printf and Python formats, positional and named braces, shell and
// template variables, markup, and escaped line breaks written out.
var placeholder = regexp.MustCompile(`%(?:\d+\$)?[-+ 0#]*\d*(?:\.\d+)?(?:hh|h|ll|l|z|j|t|L)?[sdiuoxXfFeEgGcp@]|%\([A-Za-z_][A-Za-z0-9_]*\)[sdif]|\{\{[^{}\n]+\}\}|[$%#]\{[^{}\n]+\}|\{[A-Za-z0-9_.]*\}|</?[A-Za-z][^<>\n]*>|\\[nrt]`)

// subtitleTag finds the markup of a subtitle: <i>, </b>, <c.yellow>, <00:01.000>.
var subtitleTag = regexp.MustCompile(`</?[A-Za-z0-9][^<>\n]*>|\{\\[^{}\n]*\}`)

// assToggle is an override block that only turns italics, bold,
// underline or strikeout on or off: {\i1}, {\b0}, {\u}.
var assToggle = regexp.MustCompile(`^\{\\([ibus])([01]?)\}$`)

// tagged turns a string into content: its placeholders are tags that
// carry what they stand for.
func tagged(text string, markup bool, tags *regexp.Regexp) content.Content {
	re := placeholder
	if markup {
		re = subtitleTag
	}
	if tags != nil {
		re = tags
	}
	var c content.Content
	at, n := 0, 0
	// The pairs of a subtitle's markup: an opening tag and the closing one
	// that follows it share a number.
	open := map[string][]string{}
	for _, m := range re.FindAllStringIndex(text, -1) {
		c = c.Append(text[at:m[0]])
		at = m[1]
		lit := text[m[0]:m[1]]
		t := content.Tag{Kind: content.TagStandalone, Type: "placeholder", Desc: lit, Native: lit}
		if m := assToggle.FindStringSubmatch(lit); m != nil && re == assTags {
			// {\i1} ... {\i0}: italics (bold, underline, strikeout) on and off.
			name := "ass-" + m[1]
			if m[2] == "1" {
				n++
				t.Kind, t.ID, t.Type = content.TagOpen, strconv.Itoa(n), name
				open[name] = append(open[name], t.ID)
			} else if ids := open[name]; len(ids) > 0 {
				t.Kind, t.ID, t.Type = content.TagClose, ids[len(ids)-1], name
				open[name] = ids[:len(ids)-1]
			}
		} else if markup && strings.HasPrefix(lit, "<") {
			name := strings.ToLower(strings.FieldsFunc(strings.Trim(lit, "</>"), func(r rune) bool { return r == ' ' || r == '.' })[0])
			if strings.HasPrefix(lit, "</") {
				if ids := open[name]; len(ids) > 0 {
					t.Kind, t.ID, t.Type = content.TagClose, ids[len(ids)-1], name
					open[name] = ids[:len(ids)-1]
				}
			} else if !strings.ContainsAny(name, ":0123456789") || len(name) == 1 {
				n++
				t.Kind, t.ID, t.Type = content.TagOpen, strconv.Itoa(n), name
				open[name] = append(open[name], t.ID)
			}
		}
		if t.ID == "" {
			n++
			t.ID = strconv.Itoa(n)
		}
		c = c.AppendTag(t)
	}
	c = c.Append(text[at:])
	// A pair left open is a tag on its own.
	for i, r := range c {
		if r.Tag != nil && r.Tag.Kind == content.TagOpen && len(open[r.Tag.Type]) > 0 && contains(open[r.Tag.Type], r.Tag.ID) {
			c[i].Tag.Kind = content.TagStandalone
		}
	}
	if c == nil {
		c = content.Content{}
	}
	return c
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

// lineBreaks makes the line breaks of a text tags, so a subtitle keeps
// its lines and a translator sees where they are.
func lineBreaks(c content.Content) content.Content {
	var out content.Content
	n := len(c.Tags())
	for _, r := range c {
		if r.Tag != nil {
			out = out.AppendTag(*r.Tag)
			continue
		}
		for i, line := range strings.Split(r.Text, "\n") {
			if i > 0 {
				n++
				out = out.AppendTag(content.Tag{Kind: content.TagStandalone, ID: "br" + strconv.Itoa(n), Type: "break", Desc: "line break", Native: "\n"})
			}
			out = out.Append(line)
		}
	}
	return out
}

// plain renders a translation as the string it stands for: its text,
// and for each tag what the source's tag of that number carries. A tag
// the source does not have is left out.
func plain(target, source content.Content) string {
	native := map[string]string{}
	for _, t := range source.Tags() {
		native[t.Kind.String()+":"+t.ID] = t.Native
	}
	var b strings.Builder
	for _, r := range target {
		if r.Tag == nil {
			b.WriteString(r.Text)
		} else {
			b.WriteString(native[r.Tag.Kind.String()+":"+r.Tag.ID])
		}
	}
	return b.String()
}

func (s *slot) content() content.Content {
	c := tagged(s.text, s.markup, s.tags)
	if s.markup {
		c = lineBreaks(c)
	}
	return c
}

// Extract returns the strings of a file as units.
func Extract(format string, data []byte, lang string) (*filter.Extraction, error) {
	d, err := parse(format, data)
	if err != nil {
		return nil, err
	}
	ex := &filter.Extraction{Units: []filter.Unit{}, Blocks: []filter.Block{}, Format: format, SourceLang: lang}
	for _, s := range d.slots {
		if strings.TrimSpace(s.text) == "" {
			continue
		}
		ex.Blocks = append(ex.Blocks, filter.Block{ID: s.id, Part: filter.PartBody})
		// The string's name in the file, or a subtitle's times, is the
		// first thing a translator is told about it.
		notes := s.notes
		if s.key != "" {
			notes = append([]string{s.key}, notes...)
		}
		ex.Units = append(ex.Units, filter.Unit{ID: s.id, Block: s.id, Source: s.content(), Key: s.key, Notes: notes, Timing: s.timing})
	}
	return ex, nil
}

// Merge returns the file with its strings replaced by their translations.
// A string without one keeps its text. targetLang is written where the
// format names the file's language.
func Merge(format string, data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	d, err := parse(format, data)
	if err != nil {
		return nil, err
	}
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	for _, s := range d.slots {
		t, ok := tr[s.id]
		if !ok || len(t) == 0 || strings.TrimSpace(s.text) == "" {
			continue
		}
		said := plain(t, s.content())
		edits = append(edits, edit{s.start, s.end, s.write(said)})
		for _, o := range s.also {
			edits = append(edits, edit{o.start, o.end, o.write(said)})
		}
	}
	// By position; the slots of a file never overlap.
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && edits[j].start < edits[j-1].start; j-- {
			edits[j], edits[j-1] = edits[j-1], edits[j]
		}
	}
	var out strings.Builder
	at := 0
	for _, e := range edits {
		if e.start < at {
			return nil, fmt.Errorf("textfmt: the places of two strings overlap")
		}
		out.WriteString(d.src[at:e.start])
		out.WriteString(e.text)
		at = e.end
	}
	out.WriteString(d.src[at:])
	res := out.String()
	if format == PO && targetLang != "" {
		res = setPOLanguage(res, targetLang)
	}
	return encode(res, d.enc), nil
}

// line is a line of a file: where it starts and ends, without its line
// ending, and its text.
type line struct {
	start, end int
	text       string
}

func linesOf(src string) []line {
	var out []line
	at := 0
	for at <= len(src) {
		nl := strings.IndexByte(src[at:], '\n')
		end := len(src)
		next := len(src) + 1
		if nl >= 0 {
			end = at + nl
			next = end + 1
		}
		stop := end
		if stop > at && src[stop-1] == '\r' {
			stop--
		}
		if at < len(src) || nl >= 0 {
			out = append(out, line{at, stop, src[at:stop]})
		}
		at = next
	}
	return out
}

// newline is the line ending the file uses.
func newline(src string) string {
	if strings.Contains(src, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
