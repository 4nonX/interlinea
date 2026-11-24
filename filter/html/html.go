// Package html reads and writes web pages (.html, .htm).
//
// Extract returns the text of a page: each run of text between block
// elements (paragraphs, headings, list items, table cells, options) is a
// block, cut into sentences. Inline elements inside it (b, em, a, span)
// become paired tags, and line breaks, pictures and form fields standalone
// tags. The page's title, its description and keywords, and the
// attributes people read (alt, title, placeholder, aria-label, the label
// of a button) are blocks of their own.
//
// Scripts and styles are not text. An element with translate="no" is left
// alone, with everything in it; pictures drawn in the page (svg), frames
// and embedded objects are kept as they are.
//
// Merge writes the translations back and copies every other byte of the
// page unchanged, in the page's own encoding: a character the encoding
// does not have is written as a numeric reference. The page's language
// and direction (lang and dir on the html element) become the target's.
package html

import (
	"bytes"
	"errors"
	"fmt"
	stdhtml "html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/para"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/segment"
)

// ErrNotHTML is returned for files that are not web pages.
var ErrNotHTML = errors.New("html: not a web page")

// Extract returns the translatable units of a page. lang is the source
// language (a BCP 47 tag); it selects the sentence rules.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	d.ext.Format = "html"
	d.ext.SourceLang = lang
	return &d.ext, nil
}

// Merge returns a copy of the page with the units replaced by their
// translations. lang must be the language used for Extract; targetLang
// becomes the page's language.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	d.render(&out, tr, 0, len(d.src), nil)
	page := out.String()
	if targetLang != "" {
		page = setLanguage(page, targetLang)
	}
	return d.encode(page)
}

type document struct {
	src   string // the page as UTF-8
	enc   encoding.Encoding
	bom   []byte
	seg   *segment.Segmenter
	ext   filter.Extraction
	edits []*edit // by start; an attribute's lies inside its tag
	n     int     // blocks so far
}

// edit is a piece of the page that Merge writes anew: a run of text with
// its inline elements, or the value of an attribute.
type edit struct {
	start, end int
	para       para.Para
	// ranges holds the byte range of each tag of a run, by its ID in the
	// paragraph; attr is true for an attribute's value, quote its quote.
	ranges map[string][2]int
	attr   bool
	quote  byte
}

var metaCharset = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_:.\-]+)`)

func parse(data []byte, lang string) (*document, error) {
	d := &document{seg: segment.New(nil, lang)}
	if err := d.decode(data); err != nil {
		return nil, err
	}
	if strings.IndexByte(d.src, 0) >= 0 {
		return nil, ErrNotHTML
	}
	d.walk(tokenize(d.src))
	sort.SliceStable(d.edits, func(i, j int) bool { return d.edits[i].start < d.edits[j].start })
	return d, nil
}

// decode reads the page in its encoding: the one a byte order mark or a
// meta element names, else UTF-8, else Windows-1252, which is what a
// browser takes a page for that says nothing and is not UTF-8.
func (d *document) decode(data []byte) error {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		d.bom, data = data[:3], data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		d.bom, d.enc = data[:2], unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
		data = data[2:]
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		d.bom, d.enc = data[:2], unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
		data = data[2:]
	default:
		head := data
		if len(head) > 4096 {
			head = head[:4096]
		}
		if m := metaCharset.FindSubmatch(head); m != nil {
			if e, err := htmlindex.Get(string(m[1])); err == nil {
				if name, _ := htmlindex.Name(e); name != "utf-8" {
					d.enc = e
				}
			}
		}
		if d.enc == nil && !utf8.Valid(data) {
			d.enc = charmap.Windows1252
		}
	}
	if d.enc == nil {
		if !utf8.Valid(data) {
			return fmt.Errorf("%w: the page says it is UTF-8 and is not", ErrNotHTML)
		}
		d.src = string(data)
		return nil
	}
	b, err := d.enc.NewDecoder().Bytes(data)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotHTML, err)
	}
	d.src = string(b)
	return nil
}

// encode writes the page in the encoding it came in.
func (d *document) encode(page string) ([]byte, error) {
	if d.enc == nil {
		return append(append([]byte{}, d.bom...), page...), nil
	}
	b, err := encoding.HTMLEscapeUnsupported(d.enc.NewEncoder()).Bytes([]byte(page))
	if err != nil {
		return nil, fmt.Errorf("html: %w", err)
	}
	return append(append([]byte{}, d.bom...), b...), nil
}

// Elements that sit in a line of text.
var inline = set("a abbr acronym b bdi bdo big cite code data dfn em font i kbd label mark nobr picture q rp rt ruby s samp small span strike strong sub sup time tt u var ins del")

// Elements that stand in a line of text as one thing.
var placeholder = set("br img wbr input")

// Elements kept as they are, with everything in them.
var opaque = set("svg math iframe object embed audio video canvas map template noscript")

// Elements without an end tag.
var void = set("area base br col embed hr img input link meta param source track wbr")

// Elements an open paragraph ends at.
var closesP = set("address article aside blockquote details div dl fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header hr main menu nav ol p pre section table ul")

func set(names string) map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields(names) {
		m[n] = true
	}
	return m
}

// open is an element that has started and not ended.
type open struct {
	name string
	// skip: nothing in it is translated. capture: it is one placeholder of
	// the run around it, from at. inline: it is a pair of tags of the run.
	skip, capture, inline bool
	at                    int
	pre                   bool
}

// item is a piece of a run of text.
type item struct {
	start, end int
	text       bool
	// For tags: the element's name, and whether it opens, closes or stands alone.
	name string
	kind content.TagKind
}

type walker struct {
	d     *document
	stack []open
	run   []item
	// Where the run is: the block element around it, and its table cell.
	table, row, col int
	tableDepth      int
}

func (d *document) walk(toks []token) {
	w := &walker{d: d, table: -1}
	for _, t := range toks {
		switch t.kind {
		case textToken:
			if !w.skipping() {
				w.run = append(w.run, item{start: t.start, end: t.end, text: true})
			}
		case otherToken:
			if !w.skipping() && len(w.run) > 0 {
				w.run = append(w.run, item{start: t.start, end: t.end, name: "comment", kind: content.TagStandalone})
			}
		case startToken:
			w.start(t)
		case endToken:
			w.end(t)
		}
	}
	w.flush()
}

func (w *walker) skipping() bool {
	for _, o := range w.stack {
		if o.skip {
			return true
		}
	}
	return false
}

func (w *walker) pre() bool {
	for _, o := range w.stack {
		if o.pre {
			return true
		}
	}
	return false
}

// block is the nearest element around the run that is not inline.
func (w *walker) block() string {
	for i := len(w.stack) - 1; i >= 0; i-- {
		if !w.stack[i].inline {
			return w.stack[i].name
		}
	}
	return ""
}

func (w *walker) has(name string) int {
	for i := len(w.stack) - 1; i >= 0; i-- {
		if w.stack[i].name == name {
			return i
		}
	}
	return -1
}

func (w *walker) start(t token) {
	skipping := w.skipping()
	no := false
	if v, ok := t.attr("translate"); ok {
		no = strings.EqualFold(v, "no")
	}
	if !skipping && !no {
		w.attributes(t)
	}
	if skipping {
		if !void[t.name] {
			w.stack = append(w.stack, open{name: t.name})
		}
		return
	}
	// What HTML ends without an end tag.
	switch {
	case closesP[t.name]:
		w.implied("p", "")
	case t.name == "li":
		w.implied("li", "ul ol menu")
	case t.name == "dt" || t.name == "dd":
		w.implied("dt", "dl")
		w.implied("dd", "dl")
	case t.name == "option" || t.name == "optgroup":
		w.implied("option", "select datalist")
	case t.name == "tr":
		w.implied("td", "table")
		w.implied("th", "table")
		w.implied("tr", "table")
	case t.name == "td" || t.name == "th":
		w.implied("td", "table")
		w.implied("th", "table")
	}
	switch {
	case opaque[t.name], no && (inline[t.name] || placeholder[t.name]):
		// One placeholder in the run around it.
		if void[t.name] {
			w.run = append(w.run, item{start: t.start, end: t.end, name: t.name, kind: content.TagStandalone})
			return
		}
		w.stack = append(w.stack, open{name: t.name, skip: true, capture: true, at: t.start})
	case no, t.name == "script", t.name == "style":
		w.flush()
		if !void[t.name] {
			w.stack = append(w.stack, open{name: t.name, skip: true})
		}
	case placeholder[t.name]:
		w.run = append(w.run, item{start: t.start, end: t.end, name: t.name, kind: content.TagStandalone})
	case inline[t.name]:
		w.run = append(w.run, item{start: t.start, end: t.end, name: t.name, kind: content.TagOpen})
		w.stack = append(w.stack, open{name: t.name, inline: true})
	default:
		w.flush()
		switch t.name {
		case "table":
			w.tableDepth++
			if w.tableDepth == 1 {
				w.table++
				w.row = -1
			}
		case "tr":
			if w.tableDepth == 1 {
				w.row++
				w.col = -1
			}
		case "td", "th":
			if w.tableDepth == 1 {
				w.col++
			}
		}
		if !void[t.name] {
			w.stack = append(w.stack, open{name: t.name, pre: t.name == "pre" || t.name == "textarea"})
		}
	}
}

// implied ends an open element of the name, as a start tag that cannot be
// inside it does; within stops the search at the element that holds such
// elements.
func (w *walker) implied(name, within string) {
	stop := set(within)
	for i := len(w.stack) - 1; i >= 0; i-- {
		o := w.stack[i]
		if stop[o.name] || o.skip {
			return
		}
		if o.name == name {
			w.flush()
			w.stack = w.stack[:i]
			return
		}
	}
}

func (w *walker) end(t token) {
	at := w.has(t.name)
	if at < 0 {
		// An end tag for nothing that is open: kept in its place.
		if !w.skipping() {
			if inline[t.name] && len(w.run) > 0 {
				w.run = append(w.run, item{start: t.start, end: t.end, name: t.name, kind: content.TagStandalone})
			} else if !inline[t.name] {
				w.flush()
			}
		}
		return
	}
	o := w.stack[at]
	// Skipped content ends only with the element that started it.
	for i := 0; i < at; i++ {
		if w.stack[i].skip {
			w.stack = w.stack[:at]
			return
		}
	}
	switch {
	case o.capture:
		w.stack = w.stack[:at]
		w.run = append(w.run, item{start: o.at, end: t.end, name: o.name, kind: content.TagStandalone})
	case o.skip:
		w.stack = w.stack[:at]
	case o.inline:
		w.stack = w.stack[:at]
		w.run = append(w.run, item{start: t.start, end: t.end, name: t.name, kind: content.TagClose})
	default:
		// The run ends while its element is still the one around it.
		w.flush()
		w.stack = w.stack[:at]
		if t.name == "table" && w.tableDepth > 0 {
			w.tableDepth--
		}
	}
}

// Attributes people read, on any element.
var readable = set("title alt placeholder aria-label aria-description aria-placeholder summary")

// The names and properties of meta elements whose content people read.
var metaText = set("description keywords twitter:title twitter:description og:title og:description og:site_name og:image:alt")

// attributes adds a tag's readable attributes as blocks.
func (w *walker) attributes(t token) {
	for _, a := range t.attrs {
		if a.vEnd == a.valStart {
			continue
		}
		ok := readable[a.name]
		switch {
		case a.name == "label" && (t.name == "optgroup" || t.name == "option" || t.name == "track" || t.name == "menu"):
			ok = true
		case a.name == "value" && t.name == "input":
			typ, _ := t.attr("type")
			switch strings.ToLower(typ) {
			case "button", "submit", "reset":
				ok = true
			}
		case a.name == "content" && t.name == "meta":
			name, _ := t.attr("name")
			prop, _ := t.attr("property")
			ok = metaText[strings.ToLower(name)] || metaText[strings.ToLower(prop)]
		}
		if !ok {
			continue
		}
		p, has := para.Split(w.d.seg, content.Text(a.value))
		if !has {
			continue
		}
		e := &edit{start: a.valStart, end: a.vEnd, attr: true, quote: '"'}
		if q := w.d.src[a.valStart]; q == '\'' {
			e.quote = q
		}
		part := filter.PartAttribute
		if t.name == "meta" {
			part = filter.PartMeta
		}
		w.d.n++
		e.para = p
		e.para.Add(&w.d.ext, filter.Block{ID: "b" + strconv.Itoa(w.d.n), Part: part, Style: t.name, StyleName: a.name})
		w.d.edits = append(w.d.edits, e)
	}
}

// flush ends the run of text and makes it a block if it has any.
func (w *walker) flush() {
	run := w.run
	w.run = nil
	if len(run) == 0 {
		return
	}
	d := w.d
	pre := w.pre()
	// Space around the run is the page's layout, not the text's.
	if !pre {
		for len(run) > 0 && run[0].text {
			s := d.src[run[0].start:run[0].end]
			t := strings.TrimLeft(s, " \t\n\r\f")
			run[0].start += len(s) - len(t)
			if t != "" {
				break
			}
			run = run[1:]
		}
		for n := len(run); n > 0 && run[n-1].text; n = len(run) {
			s := d.src[run[n-1].start:run[n-1].end]
			t := strings.TrimRight(s, " \t\n\r\f")
			run[n-1].end -= len(s) - len(t)
			if t != "" {
				break
			}
			run = run[:n-1]
		}
	}
	if len(run) == 0 {
		return
	}
	// Tags that open and close inside the run are pairs; one whose other
	// half is outside it stands alone.
	pair := make([]int, len(run))
	var opened []int
	for i, it := range run {
		pair[i] = -1
		switch {
		case it.text:
		case it.kind == content.TagOpen:
			opened = append(opened, i)
		case it.kind == content.TagClose:
			for k := len(opened) - 1; k >= 0; k-- {
				if run[opened[k]].name == it.name {
					pair[i], pair[opened[k]] = opened[k], i
					opened = opened[:k]
					break
				}
			}
		}
	}
	c := content.Content{}
	ranges := map[string][2]int{}
	ids := make([]string, len(run))
	next := 0
	for i, it := range run {
		if it.text {
			s := stdhtml.UnescapeString(d.src[it.start:it.end])
			if !pre {
				s = collapse(s)
			}
			c = c.Append(s)
			continue
		}
		raw := d.src[it.start:it.end]
		kind := it.kind
		if kind != content.TagStandalone && pair[i] < 0 {
			kind = content.TagStandalone
		}
		id := ""
		if kind == content.TagClose {
			id = ids[pair[i]]
		} else {
			next++
			id = strconv.Itoa(next)
		}
		ids[i] = id
		typ, desc := describe(it.name, raw)
		tag := content.Tag{Kind: kind, ID: id, Type: typ, Desc: desc}
		key := id
		if kind == content.TagClose {
			key = "/" + id
		} else {
			tag.Native = raw
		}
		ranges[key] = [2]int{it.start, it.end}
		c = c.AppendTag(tag)
	}
	p, ok := para.Split(d.seg, c)
	if !ok {
		return
	}
	d.n++
	name := w.block()
	block := filter.Block{ID: "b" + strconv.Itoa(d.n), Part: filter.PartBody, Style: name}
	switch {
	case name == "title":
		block.Part = filter.PartMeta
	case len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6':
		block.Heading = int(name[1] - '0')
	case name == "li" || name == "dt" || name == "dd":
		block.List = true
	}
	if w.tableDepth > 0 && w.row >= 0 && w.col >= 0 {
		block.Cell = &filter.Cell{Table: w.table, Row: w.row, Col: w.col}
	}
	e := &edit{start: run[0].start, end: run[len(run)-1].end, ranges: ranges}
	e.para = p
	e.para.Add(&d.ext, block)
	d.edits = append(d.edits, e)
}

// collapse turns each run of white space into one space, as a browser
// shows it. A no-break space is not white space.
func collapse(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for i := 0; i < len(s); i++ {
		if isSpace(s[i]) {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteByte(s[i])
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}

// describe names a tag for the translator.
func describe(name, raw string) (typ, desc string) {
	switch name {
	case "b", "strong":
		typ = "bold"
	case "i", "em", "cite", "dfn", "var":
		typ = "italic"
	case "u", "ins":
		typ = "underline"
	case "s", "strike", "del":
		typ = "strike"
	case "sub":
		typ = "subscript"
	case "sup":
		typ = "superscript"
	case "a":
		typ = "link"
	case "br":
		typ = "lb"
	case "img":
		typ = "image"
	case "input":
		typ = "field"
	case "comment":
		typ = "comment"
	case "code", "kbd", "samp", "tt":
		typ = "code"
	default:
		typ = "format"
		if opaque[name] {
			typ = "object"
		}
	}
	desc = raw
	if i := strings.IndexByte(desc, '>'); i >= 0 && !strings.HasPrefix(desc, "<!--") {
		desc = desc[:i+1]
	}
	if r := []rune(desc); len(r) > 80 {
		desc = string(r[:79]) + "…"
	}
	return typ, desc
}

// render writes src[s:e] with every edit inside it made. self is the edit
// being written by its own rules, whose range is s to e.
func (d *document) render(out *strings.Builder, tr filter.Translations, s, e int, self *edit) {
	pos := s
	i := sort.Search(len(d.edits), func(i int) bool { return d.edits[i].start >= s })
	for ; i < len(d.edits); i++ {
		ed := d.edits[i]
		if ed.start >= e {
			break
		}
		if ed == self || ed.start < pos || ed.end > e {
			continue // itself, or inside an edit already written
		}
		out.WriteString(d.src[pos:ed.start])
		d.write(out, tr, ed)
		pos = ed.end
	}
	out.WriteString(d.src[pos:e])
}

// write writes one edit: its translation, or what it was.
func (d *document) write(out *strings.Builder, tr filter.Translations, ed *edit) {
	if !ed.para.Translated(&d.ext, tr) {
		// The text stays; an attribute inside one of its tags may not.
		d.render(out, tr, ed.start, ed.end, ed)
		return
	}
	c := ed.para.Build(&d.ext, tr)
	if ed.attr {
		out.WriteByte(ed.quote)
		out.WriteString(escapeAttr(c.Plain(), ed.quote))
		out.WriteByte(ed.quote)
		return
	}
	for _, r := range c {
		if !r.IsTag() {
			out.WriteString(escapeText(r.Text))
			continue
		}
		key := r.Tag.ID
		if r.Tag.Kind == content.TagClose {
			key = "/" + key
		}
		if rg, ok := ed.ranges[key]; ok {
			d.render(out, tr, rg[0], rg[1], nil)
		}
	}
}

func escapeText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", " ", "&nbsp;").Replace(s)
}

func escapeAttr(s string, quote byte) string {
	q := "&quot;"
	if quote == '\'' {
		q = "&#39;"
	}
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", string(quote), q, " ", "&nbsp;").Replace(s)
}

// setLanguage makes the page's language and direction the target's: lang
// on the html element, and dir when the direction is from the right or
// the page says another.
func setLanguage(page, lang string) string {
	for _, t := range tokenize(page) {
		if t.kind != startToken || t.name != "html" {
			if t.kind == startToken && t.name != "html" {
				break // the html element comes first or not at all
			}
			continue
		}
		tag := page[t.start:t.end]
		set := func(tag, name, value string) string {
			tok, _ := startTag(tag, 0)
			for _, a := range tok.attrs {
				if a.name == name {
					if a.vEnd == a.valStart {
						return tag[:a.vEnd] + `="` + escapeAttr(value, '"') + `"` + tag[a.vEnd:]
					}
					return tag[:a.valStart] + `"` + escapeAttr(value, '"') + `"` + tag[a.vEnd:]
				}
			}
			end := strings.LastIndexByte(tag, '>')
			if end < 0 {
				return tag
			}
			return tag[:end] + ` ` + name + `="` + escapeAttr(value, '"') + `"` + tag[end:]
		}
		tag = set(tag, "lang", lang)
		tok, _ := startTag(tag, 0)
		if _, has := tok.attr("xml:lang"); has {
			tag = set(tag, "xml:lang", lang)
		}
		_, hasDir := tok.attr("dir")
		switch {
		case langtag.RTL(lang):
			tag = set(tag, "dir", "rtl")
		case hasDir:
			tag = set(tag, "dir", "ltr")
		}
		return page[:t.start] + tag + page[t.end:]
	}
	return page
}
