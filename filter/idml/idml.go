// Package idml reads and writes InDesign documents saved as IDML (.idml).
//
// Extract returns the text of the document's stories, in the order their
// frames stand on the pages, followed by stories no frame on a page
// shows. A paragraph is a block, cut into sentences. Text in another
// character format than most of its paragraph becomes a pair of tags;
// links are pairs too; forced line breaks, special characters (page
// numbers, markers), anchored objects, tables and footnote references are
// standalone tags. The cells of a table and the text of a footnote follow
// the paragraph they are anchored in.
//
// Merge writes the translations back and copies every other byte of the
// package unchanged. A paragraph translated into a language of the other
// direction is set to run that way.
//
// Notes and tracked changes are kept as they are. The language InDesign
// checks the spelling in is not changed. A paragraph whose text is
// wrapped in XML tags of the document's structure is kept as it is, with
// a warning.
package idml

import (
	"archive/zip"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/para"
	"github.com/4nonX/interlinea/i18n"
	"github.com/4nonX/interlinea/internal/xmlraw"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/segment"
)

// ErrNotIDML is returned for files that are not IDML documents.
var ErrNotIDML = errors.New("idml: not an InDesign IDML document")

// Extract returns the translatable units of an IDML document. lang is the
// source language (a BCP 47 tag); it selects the sentence rules.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	d.ext.Format = "idml"
	d.ext.SourceLang = lang
	return &d.ext, nil
}

// Merge returns a copy of the document with the units replaced by their
// translations. lang must be the language used for Extract.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	turn := targetLang != "" && langtag.RTL(lang) != langtag.RTL(targetLang)
	changed := map[string][]byte{}
	for _, s := range d.stories {
		any := false
		for _, r := range s.ranges {
			if r.translated(d, tr) {
				any = true
				break
			}
		}
		if !any {
			continue
		}
		var out strings.Builder
		s.render(&out, d, tr, 0, len(s.data), nil, turn && langtag.RTL(targetLang), turn)
		changed[s.file] = []byte(out.String())
	}
	return docx.RewritePackage(d.zr, changed)
}

type document struct {
	zr      *zip.Reader
	seg     *segment.Segmenter
	ext     filter.Extraction
	stories []*story
}

type story struct {
	file   string
	short  string
	data   []byte
	ranges []*styleRange // by start; nested ones lie inside an object of another
	n      int           // paragraphs so far
}

// styleRange is a ParagraphStyleRange: one or more paragraphs in one
// paragraph style, rewritten as a whole.
type styleRange struct {
	node *xmlraw.Node
	// start and end bound the character ranges and links of the range.
	start, end int
	formats    []format
	paras      []*paragraph
}

// format is how a stretch of text is set: its CharacterStyleRange's start
// tag and properties.
type format struct{ open, props string }

type paragraph struct {
	para para.Para
	ok   bool // has units
	// all is the paragraph as read, for writing it when it has no units.
	all content.Content
	// base is the format of most of its text; end what ends it (a Br in
	// its format), empty for the last paragraph of a story.
	base   int
	endFmt int
	ended  bool
	// tags holds what each tag is written from, by its ID in the paragraph.
	tags map[string]tagSource
}

type tagSource struct {
	kind   itemKind
	format int    // format tags
	raw    string // special characters, a link's start tag
	rng    [2]int // objects
	outer  bool   // a link around character ranges
}

type itemKind int

const (
	textItem itemKind = iota
	specialItem
	breakItem
	objectItem
	linkOpen
	linkClose
	formatItem
)

type piece struct {
	kind   itemKind
	text   string
	format int
	raw    string
	rng    [2]int
	outer  bool
	node   *xmlraw.Node
}

func parse(data []byte, lang string) (*document, error) {
	zr, err := docx.OpenPackage(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotIDML, err)
	}
	d := &document{zr: zr, seg: segment.New(nil, lang)}
	mapData, err := docx.ReadPart(zr, "designmap.xml")
	if err != nil {
		return nil, ErrNotIDML
	}
	root, err := xmlraw.Parse(mapData)
	if err != nil || root.Local != "Document" {
		return nil, ErrNotIDML
	}
	src := func(local string) []string {
		var out []string
		for _, k := range root.Elements() {
			if k.Local == local && k.Get("src") != "" {
				out = append(out, k.Get("src"))
			}
		}
		return out
	}
	storyFiles := src("Story")
	byID := map[string]string{}
	for _, f := range storyFiles {
		id := strings.TrimSuffix(path.Base(f), path.Ext(f))
		byID[strings.TrimPrefix(id, "Story_")] = f
	}
	// Stories in the order their frames stand on the pages, then on the
	// master pages, then the rest as the document lists them.
	var order []string
	seen := map[string]bool{}
	add := func(f string) {
		if f != "" && !seen[f] {
			seen[f] = true
			order = append(order, f)
		}
	}
	for _, kind := range []string{"Spread", "MasterSpread"} {
		for _, f := range src(kind) {
			b, err := docx.ReadPart(zr, f)
			if err != nil {
				continue
			}
			sp, err := xmlraw.Parse(b)
			if err != nil {
				continue
			}
			sp.Walk(func(n *xmlraw.Node) bool {
				if n.Local == "TextFrame" || n.Local == "TextPath" {
					add(byID[n.Get("ParentStory")])
				}
				return true
			})
		}
	}
	for _, f := range storyFiles {
		add(f)
	}
	for _, f := range order {
		b, err := docx.ReadPart(zr, f)
		if err != nil {
			continue // listed and not there: nothing to translate
		}
		root, err := xmlraw.Parse(b)
		if err != nil {
			d.ext.Warnings = append(d.ext.Warnings, fmt.Sprintf("%s could not be read and is kept unchanged: %v", f, err))
			continue
		}
		s := &story{file: f, short: strings.TrimSuffix(path.Base(f), path.Ext(f)), data: b}
		d.scan(s, root, scope{part: filter.PartBody, table: -1})
		sort.SliceStable(s.ranges, func(i, j int) bool { return s.ranges[i].start < s.ranges[j].start })
		d.stories = append(d.stories, s)
	}
	return d, nil
}

// scope is where a paragraph is.
type scope struct {
	part  filter.Part
	cell  *filter.Cell
	table int
}

// scan finds the paragraph style ranges under n.
func (d *document) scan(s *story, n *xmlraw.Node, sc scope) {
	tables := 0
	for _, k := range n.Elements() {
		switch k.Local {
		case "ParagraphStyleRange":
			d.styleRange(s, k, sc)
		case "Note":
			// An editor's note, not part of the text.
		case "Change":
			// Tracked changes are kept as they are.
		case "Footnote":
			d.scan(s, k, scope{part: filter.PartFootnote, table: -1})
		case "Table":
			inner := sc
			inner.table = tables
			tables++
			d.scan(s, k, inner)
		case "Cell":
			inner := sc
			// A cell is named by its column and row: "1:0".
			if col, row, ok := strings.Cut(k.Get("Name"), ":"); ok {
				c, err1 := strconv.Atoi(col)
				r, err2 := strconv.Atoi(row)
				if err1 == nil && err2 == nil {
					inner.cell = &filter.Cell{Table: sc.table, Row: r, Col: c}
				}
			}
			d.scan(s, k, inner)
		default:
			d.scan(s, k, sc)
		}
	}
}

// styleRange reads a ParagraphStyleRange into paragraphs and units.
func (d *document) styleRange(s *story, n *xmlraw.Node, sc scope) {
	r := &styleRange{node: n}
	var pieces []piece
	var objects []*xmlraw.Node
	whole := true
	first := true
	inRange := func(csr *xmlraw.Node, f int) {
		var walk func(kids []*xmlraw.Node)
		walk = func(kids []*xmlraw.Node) {
			for _, c := range kids {
				if !c.IsElement() {
					continue
				}
				switch c.Local {
				case "Properties":
				case "Content":
					for _, t := range c.Kids {
						switch {
						case t.CharData:
							// A forced line break is a character of its own.
							for i, line := range strings.Split(t.Text, " ") {
								if i > 0 {
									pieces = append(pieces, piece{kind: specialItem, format: f, raw: " ", text: "lb"})
								}
								if line != "" {
									pieces = append(pieces, piece{kind: textItem, format: f, text: line})
								}
							}
						case !t.IsElement():
							pieces = append(pieces, piece{kind: specialItem, format: f, raw: string(t.Raw)})
						}
					}
				case "Br":
					pieces = append(pieces, piece{kind: breakItem, format: f})
				case "HyperlinkTextSource":
					pieces = append(pieces, piece{kind: linkOpen, format: f, raw: openTag(c)})
					walk(c.Kids)
					pieces = append(pieces, piece{kind: linkClose, format: f})
				default:
					pieces = append(pieces, piece{kind: objectItem, format: f, rng: c.Whole(), text: c.Local, node: c})
					objects = append(objects, c)
				}
			}
		}
		walk(csr.Kids)
	}
	addRange := func(csr *xmlraw.Node) {
		f := format{open: openTag(csr)}
		if p := csr.Child("Properties"); p != nil {
			f.props = string(p.Raw)
		}
		r.formats = append(r.formats, f)
		inRange(csr, len(r.formats)-1)
	}
	for _, k := range n.Elements() {
		switch k.Local {
		case "Properties":
			continue
		case "CharacterStyleRange":
			addRange(k)
		case "HyperlinkTextSource":
			// A link around whole character ranges.
			pieces = append(pieces, piece{kind: linkOpen, raw: openTag(k), outer: true})
			for _, c := range k.Elements() {
				if c.Local == "CharacterStyleRange" {
					addRange(c)
				} else if c.Local != "Properties" {
					whole = false
				}
			}
			pieces = append(pieces, piece{kind: linkClose, outer: true})
		default:
			whole = false
		}
		if first {
			r.start, first = k.Off, false
		}
		r.end = k.Off + len(k.Raw)
	}
	if !whole {
		d.ext.Warnings = append(d.ext.Warnings, fmt.Sprintf("A paragraph of %s has text inside tags of the document's structure and is kept unchanged.", s.file))
		d.scan(s, n, sc)
		return
	}
	if first {
		return // no text at all
	}

	// Paragraphs end at a Br.
	var cur []piece
	flush := func(ended bool, endFmt int) {
		p := d.paragraph(r, cur)
		p.ended, p.endFmt = ended, endFmt
		cur = nil
		if p.ok {
			s.n++
			style := styleName(n.Get("AppliedParagraphStyle"))
			block := filter.Block{ID: s.short + "/p" + strconv.Itoa(s.n), Part: sc.part, File: s.file, Style: style, StyleName: style, Cell: sc.cell}
			p.para.Add(&d.ext, block)
		}
		r.paras = append(r.paras, p)
	}
	for _, pc := range pieces {
		if pc.kind == breakItem {
			flush(true, pc.format)
			continue
		}
		cur = append(cur, pc)
	}
	if len(cur) > 0 {
		flush(false, 0)
	}
	s.ranges = append(s.ranges, r)
	// What is anchored in the text follows it: table cells, footnotes.
	for _, o := range objects {
		if o.Local == "Note" || o.Local == "Change" {
			continue
		}
		wrap := &xmlraw.Node{Kids: []*xmlraw.Node{o}}
		d.scan(s, wrap, sc)
	}
}

// paragraph turns the pieces between two paragraph ends into content.
func (d *document) paragraph(r *styleRange, pieces []piece) *paragraph {
	p := &paragraph{tags: map[string]tagSource{}}
	weight := map[int]int{}
	best := -1
	for _, pc := range pieces {
		if pc.kind == textItem {
			weight[pc.format] += len(pc.text)
		}
	}
	for _, pc := range pieces {
		if pc.kind == textItem && weight[pc.format] > best {
			best, p.base = weight[pc.format], pc.format
		}
	}
	if best < 0 && len(pieces) > 0 {
		p.base = pieces[0].format
	}
	c := content.Content{}
	id := 0
	next := func() string { id++; return strconv.Itoa(id) }
	openFmt, fmtID := -1, ""
	closeFmt := func() {
		if openFmt >= 0 {
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: fmtID, Type: "format"})
			openFmt = -1
		}
	}
	var links []string
	for _, pc := range pieces {
		// Text in another format than most of the paragraph is a pair.
		if pc.kind == textItem || pc.kind == specialItem || pc.kind == objectItem {
			if pc.format != p.base && !sameFormat(r, pc.format, p.base) {
				if openFmt < 0 || !sameFormat(r, openFmt, pc.format) {
					closeFmt()
					openFmt, fmtID = pc.format, next()
					typ, desc := describe(r.formats[p.base], r.formats[pc.format])
					p.tags[fmtID] = tagSource{kind: formatItem, format: pc.format}
					c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: fmtID, Type: typ, Desc: desc, Native: r.formats[pc.format].open})
				}
			} else {
				closeFmt()
			}
		}
		switch pc.kind {
		case textItem:
			c = c.Append(pc.text)
		case specialItem:
			tid := next()
			typ, desc := "special", i18n.Fill(i18n.N("Special character {code}"), "code", pc.raw)
			if pc.text == "lb" {
				typ, desc = "lb", i18n.N("Forced line break")
			}
			p.tags[tid] = tagSource{kind: specialItem, raw: pc.raw}
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: tid, Type: typ, Desc: desc, Native: pc.raw})
		case objectItem:
			tid := next()
			typ := "object"
			switch pc.text {
			case "Footnote":
				typ = "footnote"
			case "Table":
				typ = "table"
			}
			p.tags[tid] = tagSource{kind: objectItem, rng: pc.rng}
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: tid, Type: typ, Desc: pc.text})
		case linkOpen:
			closeFmt()
			tid := next()
			links = append(links, tid)
			p.tags[tid] = tagSource{kind: linkOpen, raw: pc.raw, outer: pc.outer}
			c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: tid, Type: "link", Desc: i18n.N("Link"), Native: pc.raw})
		case linkClose:
			closeFmt()
			if n := len(links); n > 0 {
				c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: links[n-1], Type: "link", Desc: i18n.N("Link")})
				links = links[:n-1]
			} else {
				// The end of a link that began in an earlier paragraph.
				tid := next()
				p.tags[tid] = tagSource{kind: linkClose, outer: pc.outer}
				c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: tid, Type: "link", Desc: i18n.N("End of a link")})
			}
		}
	}
	closeFmt()
	// A link that ends in a later paragraph: its start stands alone here.
	for _, tid := range links {
		for i := range c {
			if c[i].IsTag() && c[i].Tag.ID == tid {
				c[i].Tag.Kind = content.TagStandalone
				c[i].Tag.Desc = i18n.N("Start of a link")
			}
		}
	}
	p.all = c
	p.para, p.ok = para.Split(d.seg, c)
	return p
}

func sameFormat(r *styleRange, a, b int) bool {
	return a == b || r.formats[a] == r.formats[b]
}

// openTag is an element's start tag, opened when the element was empty.
func openTag(n *xmlraw.Node) string {
	s := string(n.Start)
	if n.SelfClosing() {
		s = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(s, ">")), "/") + ">"
	}
	return s
}

// styleName is a style's name as InDesign shows it.
func styleName(ref string) string {
	name := strings.TrimPrefix(ref, "ParagraphStyle/")
	name = strings.TrimPrefix(name, "CharacterStyle/")
	name = strings.TrimPrefix(name, "$ID/")
	if dec, err := url.PathUnescape(name); err == nil {
		name = dec
	}
	return name
}

// describe names how a character format differs from the paragraph's.
func describe(base, f format) (typ, desc string) {
	a, err1 := xmlraw.ParseFragment([]byte(base.open + "</CharacterStyleRange>"))
	b, err2 := xmlraw.ParseFragment([]byte(f.open + "</CharacterStyleRange>"))
	if err1 != nil || err2 != nil {
		return "format", i18n.N("Formatting")
	}
	type change struct{ typ, desc string }
	var changes []change
	if s := b.Get("AppliedCharacterStyle"); s != a.Get("AppliedCharacterStyle") && s != "" {
		if name := styleName(s); name != "[No character style]" {
			changes = append(changes, change{"style", i18n.Fill(i18n.N("Character style {name}"), "name", name)})
		}
	}
	if s := b.Get("FontStyle"); s != a.Get("FontStyle") && s != "" {
		switch strings.ToLower(s) {
		case "bold":
			changes = append(changes, change{"bold", i18n.N("Bold")})
		case "italic", "oblique":
			changes = append(changes, change{"italic", i18n.N("Italic")})
		default:
			changes = append(changes, change{"format", s})
		}
	}
	if b.Get("Underline") == "true" && a.Get("Underline") != "true" {
		changes = append(changes, change{"underline", i18n.N("Underline")})
	}
	if b.Get("StrikeThru") == "true" && a.Get("StrikeThru") != "true" {
		changes = append(changes, change{"strike", i18n.N("Strikethrough")})
	}
	if p := b.Get("Position"); p != a.Get("Position") {
		switch p {
		case "Superscript", "OTSuperscript":
			changes = append(changes, change{"superscript", i18n.N("Superscript")})
		case "Subscript", "OTSubscript":
			changes = append(changes, change{"subscript", i18n.N("Subscript")})
		}
	}
	if s := b.Get("PointSize"); s != a.Get("PointSize") && s != "" {
		changes = append(changes, change{"size", i18n.Fill(i18n.N("Size {n} pt"), "n", s)})
	}
	if s := b.Get("FillColor"); s != a.Get("FillColor") && s != "" {
		changes = append(changes, change{"color", i18n.Fill(i18n.N("Colour {value}"), "value", strings.TrimPrefix(s, "Color/"))})
	}
	switch len(changes) {
	case 0:
		return "format", i18n.N("Formatting")
	case 1:
		return changes[0].typ, changes[0].desc
	}
	descs := make([]string, len(changes))
	for i, c := range changes {
		descs[i] = c.desc
	}
	return "format", strings.Join(descs, ", ")
}

func (r *styleRange) translated(d *document, tr filter.Translations) bool {
	for _, p := range r.paras {
		if p.ok && p.para.Translated(&d.ext, tr) {
			return true
		}
	}
	return false
}

// render writes data[s:e] with every style range inside it written anew
// where it has a translation. self is the range being written by its own
// rules.
func (s *story) render(out *strings.Builder, d *document, tr filter.Translations, from, to int, self *styleRange, rtl, turn bool) {
	pos := from
	for _, r := range s.ranges {
		if r == self || r.start < pos || r.end > to || r.start < from {
			continue
		}
		if !r.translated(d, tr) {
			continue // as it is; a table or footnote in it is looked at on its own
		}
		// The paragraph style range's own start tag carries the direction.
		tagStart, tagEnd := r.node.Off, r.node.Off+len(r.node.Start)
		if turn && tagStart >= pos {
			out.Write(s.data[pos:tagStart])
			dir := "LeftToRightDirection"
			if rtl {
				dir = "RightToLeftDirection"
			}
			out.Write(xmlraw.SetAttr(r.node.Start, "ParagraphDirection", dir))
			pos = tagEnd
		}
		out.Write(s.data[pos:r.start])
		r.write(out, s, d, tr, rtl, turn)
		pos = r.end
	}
	out.Write(s.data[pos:to])
}

// write writes the character ranges of a paragraph style range.
func (r *styleRange) write(out *strings.Builder, s *story, d *document, tr filter.Translations, rtl, turn bool) {
	w := &writer{out: out, r: r, s: s, d: d, tr: tr, rtl: rtl, turn: turn, cur: -1}
	for _, p := range r.paras {
		c := p.all
		if p.ok {
			c = p.para.Build(&d.ext, tr).Normalize()
		}
		w.paragraph(p, c)
	}
	w.closeRange()
	w.closeOuter()
}

type writer struct {
	out  *strings.Builder
	r    *styleRange
	s    *story
	d    *document
	tr   filter.Translations
	rtl  bool
	turn bool
	// cur is the format of the open character range, -1 for none; link
	// counts the links open inside it, outer those around ranges.
	cur   int
	link  int
	outer int
}

func (w *writer) closeRange() {
	if w.cur >= 0 {
		for ; w.link > 0; w.link-- {
			w.out.WriteString("</HyperlinkTextSource>")
		}
		w.out.WriteString("</CharacterStyleRange>")
		w.cur = -1
	}
}

func (w *writer) closeOuter() {
	for ; w.outer > 0; w.outer-- {
		w.out.WriteString("</HyperlinkTextSource>")
	}
}

// in makes f the format of the open character range. Inside a link that
// lies in a range the format stays, since the link cannot span ranges.
func (w *writer) in(f int) {
	if w.cur >= 0 && (w.link > 0 || sameFormat(w.r, w.cur, f)) {
		return
	}
	w.closeRange()
	w.out.WriteString(w.r.formats[f].open)
	w.out.WriteString(w.r.formats[f].props)
	w.cur = f
}

func (w *writer) openLink(src tagSource, format int) {
	if src.outer {
		w.closeRange()
		w.out.WriteString(src.raw)
		w.outer++
		return
	}
	w.in(format)
	w.out.WriteString(src.raw)
	w.link++
}

func (w *writer) closeLink(src tagSource) {
	if src.outer {
		w.closeRange()
		if w.outer > 0 {
			w.out.WriteString("</HyperlinkTextSource>")
			w.outer--
		}
		return
	}
	if w.link > 0 {
		w.out.WriteString("</HyperlinkTextSource>")
		w.link--
	}
}

func (w *writer) paragraph(p *paragraph, c content.Content) {
	var formats []int // the format pairs open, innermost last
	now := func() int {
		if n := len(formats); n > 0 {
			return formats[n-1]
		}
		return p.base
	}
	var kinds []itemKind // what each open pair is, to close it
	for _, run := range c {
		if !run.IsTag() {
			if run.Text == "" {
				continue
			}
			w.in(now())
			w.out.WriteString("<Content>" + xmlraw.EscapeText(run.Text) + "</Content>")
			continue
		}
		src, ok := p.tags[run.Tag.ID]
		if !ok {
			continue
		}
		switch run.Tag.Kind {
		case content.TagStandalone:
			switch src.kind {
			case specialItem:
				w.in(now())
				w.out.WriteString("<Content>" + src.raw + "</Content>")
			case objectItem:
				w.in(now())
				w.s.render(w.out, w.d, w.tr, src.rng[0], src.rng[1], nil, w.rtl, w.turn)
			case linkOpen:
				w.openLink(src, now())
			case linkClose:
				w.closeLink(src)
			}
		case content.TagOpen:
			kinds = append(kinds, src.kind)
			if src.kind == formatItem {
				formats = append(formats, src.format)
			} else {
				w.openLink(src, now())
			}
		case content.TagClose:
			if len(kinds) == 0 {
				continue
			}
			kinds = kinds[:len(kinds)-1]
			if src.kind == formatItem {
				if n := len(formats); n > 0 {
					formats = formats[:n-1]
				}
			} else {
				w.closeLink(src)
			}
		}
	}
	if p.ended {
		w.in(p.endFmt)
		w.out.WriteString("<Br/>")
	}
}
