// Package markdown reads Markdown: headings, paragraphs, list items,
// quotes and table cells are translated, cut into sentences; emphasis,
// links, images, code in a line and HTML in a line are tags. Code blocks,
// front matter, link definitions and HTML blocks stay as they are, as does
// everything else outside the text.
//
// A paragraph written over several lines comes back on one line, which
// Markdown reads the same; a line break made on purpose (two blanks or a
// backslash at the end of a line) is a tag and stays.
package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/para"
	"github.com/4nonX/interlinea/segment"
)

// Format is the name stored with a project's files.
const Format = "md"

type edit struct {
	start, end int
	para       para.Para
}

type doc struct {
	src   string
	bom   bool
	seg   *segment.Segmenter
	ext   filter.Extraction
	edits []*edit
}

type line struct {
	start, end int
	text       string
}

func linesOf(src string) []line {
	var out []line
	at := 0
	for at < len(src) {
		nl := strings.IndexByte(src[at:], '\n')
		end, next := len(src), len(src)
		if nl >= 0 {
			end, next = at+nl, at+nl+1
		}
		stop := end
		if stop > at && src[stop-1] == '\r' {
			stop--
		}
		out = append(out, line{at, stop, src[at:stop]})
		at = next
	}
	return out
}

var (
	fence     = regexp.MustCompile("^ {0,3}(```+|~~~+)")
	heading   = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+|$)`)
	closing   = regexp.MustCompile(`[ \t]+#+[ \t]*$`)
	rule      = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	underline = regexp.MustCompile(`^ {0,3}(?:=+|-+)[ \t]*$`)
	container = regexp.MustCompile(`^(?:[ \t]*(?:>[ \t]?|(?:[-*+]|\d{1,9}[.)])(?:[ \t]+\[[ xX]\])?[ \t]+))*[ \t]*`)
	marker    = regexp.MustCompile(`^(?:[ \t]*(?:>[ \t]?|(?:[-*+]|\d{1,9}[.)])(?:[ \t]+\[[ xX]\])?[ \t]+))+`)
	linkDef   = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:[ \t]*\S`)
	htmlLine  = regexp.MustCompile(`^ {0,3}</?[A-Za-z!][^>]*>[ \t]*$`)
	tableRule = regexp.MustCompile(`^[ \t]*\|?[ \t]*:?-{1,}:?[ \t]*(?:\|[ \t]*:?-{1,}:?[ \t]*)*\|?[ \t]*$`)
	hardBreak = regexp.MustCompile(`(?: {2,}|\\)$`)
)

func parse(data []byte, lang string) (*doc, error) {
	d := &doc{seg: segment.New(nil, lang)}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		d.bom, data = true, data[3:]
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("the file is not UTF-8")
	}
	d.src = string(data)
	d.ext = filter.Extraction{Units: []filter.Unit{}, Blocks: []filter.Block{}, Format: Format, SourceLang: lang}
	ls := linesOf(d.src)
	i := 0
	// Front matter: from --- on the first line to the next --- or ....
	if len(ls) > 0 && strings.TrimRight(ls[0].text, " \t") == "---" {
		for j := 1; j < len(ls); j++ {
			if t := strings.TrimRight(ls[j].text, " \t"); t == "---" || t == "..." {
				i = j + 1
				break
			}
		}
	}
	blank := true
	for i < len(ls) {
		text := ls[i].text
		switch {
		case strings.TrimSpace(text) == "":
			blank = true
			i++
			continue
		case fence.MatchString(text):
			// A code block: up to the line that closes it, or the end.
			mark := strings.TrimLeft(text, " ")[:3]
			i++
			for i < len(ls) && !strings.HasPrefix(strings.TrimLeft(ls[i].text, " "), mark) {
				i++
			}
			i++
			blank = false
			continue
		case blank && (strings.HasPrefix(text, "    ") || strings.HasPrefix(text, "\t")) && !marker.MatchString(strings.TrimLeft(text, " \t")):
			// Indented code, after an empty line.
			i++
			continue
		case rule.MatchString(text), linkDef.MatchString(text), htmlLine.MatchString(text):
			blank = false
			i++
			continue
		}
		blank = false
		if heading.MatchString(text) {
			pre := len(heading.FindString(text))
			end := len(text)
			if m := closing.FindStringIndex(text); m != nil && m[0] >= pre {
				end = m[0]
			}
			level := strings.Count(strings.TrimSpace(text[:pre]), "#")
			d.add(ls[i].start+pre, ls[i].start+end, text[pre:end], level, false)
			i++
			continue
		}
		// A table: its rows are lines with |, one of them the line of dashes.
		isRule := func(k int) bool {
			return k < len(ls) && strings.Contains(ls[k].text, "|") && strings.Contains(ls[k].text, "-") && tableRule.MatchString(ls[k].text)
		}
		if strings.Contains(text, "|") && (isRule(i) || isRule(i+1) || inTable(ls, i)) {
			if !isRule(i) {
				d.row(ls[i])
			}
			i++
			continue
		}
		// A paragraph: this line and those that go on with it.
		pre := len(container.FindString(text))
		start := ls[i].start + pre
		isList := strings.ContainsAny(strings.TrimSpace(text[:pre]), "-*+.)")
		parts := []string{text[pre:]}
		j := i + 1
		for j < len(ls) {
			t := ls[j].text
			if strings.TrimSpace(t) == "" || fence.MatchString(t) || heading.MatchString(t) || rule.MatchString(t) || htmlLine.MatchString(t) {
				break
			}
			// A new list item or quote ends the paragraph; a quote's next
			// line, with its own >, goes on with it.
			if m := strings.TrimSpace(marker.FindString(t)); m != "" && !(strings.Trim(m, "> ") == "" && strings.Contains(text[:pre], ">")) {
				break
			}
			if underline.MatchString(t) {
				// A heading written with a line under it: the line stays.
				break
			}
			parts = append(parts, strings.TrimLeft(strings.TrimLeft(t, " \t>"), " \t"))
			j++
		}
		level := 0
		if j < len(ls) && underline.MatchString(ls[j].text) && !isList {
			level = 2
			if strings.Contains(ls[j].text, "=") {
				level = 1
			}
		}
		d.add(start, ls[j-1].end, joinLines(parts), level, isList)
		if level > 0 {
			j++
		}
		i = j
	}
	return d, nil
}

func inTable(ls []line, i int) bool {
	for j := i - 1; j >= 0 && strings.Contains(ls[j].text, "|"); j-- {
		if tableRule.MatchString(ls[j].text) && strings.Contains(ls[j].text, "-") {
			return true
		}
	}
	return false
}

// joinLines makes one line of a paragraph's lines. A line break made on
// purpose stays, as the mark the inline reader makes a tag of.
func joinLines(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i == len(parts)-1 {
			b.WriteString(strings.TrimRight(p, " \t"))
			break
		}
		if hardBreak.MatchString(p) {
			b.WriteString(strings.TrimRight(strings.TrimSuffix(strings.TrimRight(p, " "), `\`), " ") + hardMark)
		} else {
			b.WriteString(strings.TrimRight(p, " \t") + " ")
		}
	}
	return b.String()
}

const hardMark = "\x00br\x00"

// row reads the cells of a table row.
func (d *doc) row(l line) {
	at := 0
	for at <= len(l.text) {
		end := at
		for end < len(l.text) && !(l.text[end] == '|' && (end == 0 || l.text[end-1] != '\\')) {
			end++
		}
		cell := l.text[at:end]
		if t := strings.TrimSpace(cell); t != "" {
			lead := len(cell) - len(strings.TrimLeft(cell, " \t"))
			d.add(l.start+at+lead, l.start+at+lead+len(t), t, 0, false)
		}
		at = end + 1
	}
}

func (d *doc) add(start, end int, text string, level int, list bool) {
	p, ok := para.Split(d.seg, inline(text))
	if !ok {
		return
	}
	e := &edit{start: start, end: end, para: p}
	e.para.Add(&d.ext, filter.Block{ID: "b" + strconv.Itoa(len(d.edits)+1), Part: filter.PartBody, Heading: level, List: list})
	d.edits = append(d.edits, e)
}

var inlineMark = regexp.MustCompile("``[^\n]*?``|`[^`\n]+`|!?\\[|\\]\\([^()\\s]*(?:\\s+\"[^\"]*\")?\\)|\\]\\[[^\\]]*\\]|<[A-Za-z/!][^<>\n]*>|\\*\\*|__|~~|\\*|_|\x00br\x00|\\\\[\\\\`*_{}\\[\\]()#+.!|~-]")

// inline reads the text of a paragraph: its emphasis, links, images, code
// and HTML are tags that carry what was written.
func inline(text string) content.Content {
	var c content.Content
	n := 0
	next := func() string { n++; return strconv.Itoa(n) }
	// What is open: the mark and its tag's number.
	type opened struct{ mark, id string }
	var open []opened
	find := func(mark string) int {
		for i := len(open) - 1; i >= 0; i-- {
			if open[i].mark == mark {
				return i
			}
		}
		return -1
	}
	at := 0
	for _, m := range inlineMark.FindAllStringIndex(text, -1) {
		lit := text[m[0]:m[1]]
		switch {
		case strings.HasPrefix(lit, `\`):
			// An escaped character is text as written.
			continue
		case lit == "*" || lit == "_":
			// A single mark is emphasis only where it touches a word.
			left := m[0] > 0 && !isSpace(text[m[0]-1])
			right := m[1] < len(text) && !isSpace(text[m[1]])
			if find(lit) < 0 && !right || find(lit) >= 0 && !left {
				continue
			}
			if lit == "_" && find(lit) < 0 && left && isWord(text[m[0]-1]) {
				// snake_case is not emphasis.
				continue
			}
		}
		c = c.Append(text[at:m[0]])
		at = m[1]
		switch {
		case lit == hardMark:
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: next(), Type: "break", Desc: "line break", Native: "  \n"})
		case strings.HasPrefix(lit, "`"):
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: next(), Type: "code", Desc: lit, Native: lit})
		case strings.HasPrefix(lit, "<"):
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: next(), Type: "html", Desc: lit, Native: lit})
		case lit == "[" || lit == "![":
			id := next()
			open = append(open, opened{"[", id})
			kind := "link"
			if lit == "![" {
				kind = "image"
			}
			c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: id, Type: kind, Native: lit})
		case strings.HasPrefix(lit, "]"):
			if i := find("["); i >= 0 {
				c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: open[i].id, Type: "link", Desc: lit, Native: lit})
				open = open[:i]
			} else {
				c = c.Append(lit)
			}
		default:
			if i := find(lit); i >= 0 {
				c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: open[i].id, Type: emphasis(lit), Native: lit})
				open = open[:i]
			} else {
				id := next()
				open = append(open, opened{lit, id})
				c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: id, Type: emphasis(lit), Native: lit})
			}
		}
	}
	c = c.Append(text[at:])
	// A mark that nothing closes was text after all.
	if len(open) > 0 {
		left := map[string]bool{}
		for _, o := range open {
			left[o.id] = true
		}
		var fixed content.Content
		for _, r := range c {
			if r.Tag != nil && r.Tag.Kind == content.TagOpen && left[r.Tag.ID] {
				fixed = fixed.Append(r.Tag.Native)
			} else if r.Tag != nil {
				fixed = fixed.AppendTag(*r.Tag)
			} else {
				fixed = fixed.Append(r.Text)
			}
		}
		c = fixed
	}
	if c == nil {
		c = content.Content{}
	}
	return c
}

func emphasis(mark string) string {
	switch mark {
	case "**", "__":
		return "bold"
	case "~~":
		return "strikethrough"
	}
	return "italic"
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' }

func isWord(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= 0x80
}

// Extract returns the text of a Markdown file as units.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	return &d.ext, nil
}

// Merge returns the file with its units replaced by their translations.
// lang must be the language used for Extract.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	if d.bom {
		out.WriteString("\xEF\xBB\xBF")
	}
	at := 0
	for _, e := range d.edits {
		out.WriteString(d.src[at:e.start])
		at = e.end
		// A paragraph nothing of which is translated stays as it is written.
		if !e.para.Translated(&d.ext, tr) {
			out.WriteString(d.src[e.start:e.end])
			continue
		}
		for _, r := range e.para.Build(&d.ext, tr) {
			if r.Tag == nil {
				out.WriteString(r.Text)
			} else {
				out.WriteString(r.Tag.Native)
			}
		}
	}
	out.WriteString(d.src[at:])
	return []byte(out.String()), nil
}
