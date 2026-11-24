package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	parag "github.com/4nonX/interlinea/filter/para"
)

// dialect is the markup a paragraph is written in.
type dialect uint8

const (
	wml   dialect = iota // WordprocessingML <w:p>: body, notes, headers, text boxes
	dml                  // DrawingML <a:p>: charts, SmartArt
	label                // a chart's cached category or series name <c:v>
)

// para is one paragraph (or chart label) that may contain units, with its
// byte range in the part and the pieces it is rebuilt from.
type para struct {
	dialect    dialect
	ns         string // prefix of the paragraph's namespace ("w", "a")
	start, end int
	open       []byte // raw start tag
	close      []byte // raw end tag
	ppr        []byte // raw paragraph properties, if any
	base       []byte // run properties of most of the text
	// prefix and suffix hold tags at the paragraph edges that are not shown
	// to translators (a footnote number, an anchor, a link around everything).
	prefix, suffix content.Content
	pieces         []piece
	// ranges holds, for placeholders, the byte ranges their markup comes
	// from, so text inside them (a text box in a drawing) can be rewritten.
	ranges map[string][]span
	// mirror marks a copy of other content (the fallback copy of a text box,
	// SmartArt's cached drawing): it gets no units of its own but the
	// translations of the paragraph with the same source.
	mirror bool
	sig    string
}

// piece is a unit (by index into the extraction) or the whitespace and tags
// between units.
type piece struct {
	unit int // -1 for a gap; -2 for a mirror unit not matched (yet)
	gap  content.Content
	src  content.Content   // the unit's source with this paragraph's markup
	ids  map[string]string // unit tag ID -> paragraph tag ID
}

func (p *para) hasUnits() bool {
	for _, pc := range p.pieces {
		if pc.unit >= 0 {
			return true
		}
	}
	return false
}

// context while walking a part.
type walkCtx struct {
	tableDepth int
	table      int
	row, col   int
	note       string
	skip       bool
}

type frame struct {
	space, local string
	fallback     bool
	textbox      bool
	strCache     bool
}

const (
	nsDML   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsChart = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	nsMC    = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

func (d *document) parsePart(file string, kind filter.Part, data []byte, cache bool) (*part, error) {
	toks, err := tokenize(data)
	if err != nil {
		return nil, err
	}
	p := &part{file: file, kind: kind, data: data, cache: cache}
	// Prefixes are looked up on every start tag, since drawings declare
	// their namespaces locally.
	prefixes := map[string]string{}
	ctx := walkCtx{table: -1}
	var stack []frame
	paraIndex, labelIndex := 0, 0
	for i := 0; i < len(toks); i++ {
		switch t := toks[i].Tok.(type) {
		case xml.StartElement:
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" {
					prefixes[a.Value] = a.Name.Local
				}
			}
			f := frame{space: t.Name.Space, local: t.Name.Local}
			if n := len(stack); n > 0 {
				f.fallback, f.textbox, f.strCache = stack[n-1].fallback, stack[n-1].textbox, stack[n-1].strCache
			}
			w, a, c, mc := prefixes[nsMain], prefixes[nsDML], prefixes[nsChart], prefixes[nsMC]
			switch {
			case mc != "" && t.Name.Space == mc && t.Name.Local == "Fallback":
				f.fallback = true
			case w != "" && t.Name.Space == w && t.Name.Local == "txbxContent":
				f.textbox = true
			case c != "" && t.Name.Space == c && t.Name.Local == "strCache":
				f.strCache = true
			}
			if w != "" && t.Name.Space == w {
				switch t.Name.Local {
				case "tbl":
					ctx.tableDepth++
					if ctx.tableDepth == 1 {
						ctx.table++
						ctx.row = -1
					}
				case "tr":
					if ctx.tableDepth == 1 {
						ctx.row++
						ctx.col = -1
					}
				case "tc":
					if ctx.tableDepth == 1 {
						ctx.col++
					}
				case "footnote", "endnote", "comment":
					ctx.note, ctx.skip = "", false
					for _, at := range t.Attr {
						switch at.Name.Local {
						case "id":
							ctx.note = at.Value
						case "type":
							// separators and continuation notices are layout, not text
							ctx.skip = at.Value != "normal"
						}
					}
				}
			}
			// A table on a slide: its cells hold DrawingML paragraphs.
			if a != "" && t.Name.Space == a {
				switch t.Name.Local {
				case "tbl":
					ctx.tableDepth++
					if ctx.tableDepth == 1 {
						ctx.table++
						ctx.row = -1
					}
				case "tr":
					if ctx.tableDepth == 1 {
						ctx.row++
						ctx.col = -1
					}
				case "tc":
					if ctx.tableDepth == 1 {
						ctx.col++
					}
				}
			}
			isW := w != "" && t.Name.Space == w && t.Name.Local == "p"
			isA := a != "" && t.Name.Space == a && t.Name.Local == "p"
			isLabel := c != "" && t.Name.Space == c && t.Name.Local == "v" && f.strCache && !d.noLabels
			if (isW || isA || isLabel) && !ctx.skip {
				n, _, err := build(data, toks, i)
				if err != nil {
					return nil, err
				}
				mirror := f.fallback || cache
				dia, prefix := wml, w
				if isA {
					dia, prefix = dml, a
				}
				switch {
				case isLabel:
					if !mirror {
						labelIndex++
						d.addLabel(p, n, fmt.Sprintf("%s/v%d", shortName(file), labelIndex))
					}
				default:
					id := ""
					if !mirror {
						paraIndex++
						id = fmt.Sprintf("%s/p%d", shortName(file), paraIndex)
					}
					if err := d.addParagraph(p, n, id, dia, prefix, ctx, f.textbox, mirror); err != nil {
						return nil, err
					}
				}
			}
			stack = append(stack, f)
		case xml.EndElement:
			if n := len(stack); n > 0 {
				stack = stack[:n-1]
			}
			if w := prefixes[nsMain]; w != "" && t.Name.Space == w {
				switch t.Name.Local {
				case "tbl":
					ctx.tableDepth--
				case "footnote", "endnote", "comment":
					ctx.note, ctx.skip = "", false
				}
			}
			if a := prefixes[nsDML]; a != "" && t.Name.Space == a && t.Name.Local == "tbl" {
				ctx.tableDepth--
			}
		}
	}
	return p, nil
}

// addLabel adds a chart's cached category or series name as a unit.
func (d *document) addLabel(p *part, n *node, id string) {
	text := n.InnerText()
	if strings.TrimSpace(text) == "" || len(n.End) == 0 {
		return
	}
	if _, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
		return // a number stored as text
	}
	block := filter.Block{ID: id, Part: p.kind, File: p.file, Style: "label"}
	src := content.Text(text)
	pa := &para{dialect: label, start: n.Off, end: n.Off + len(n.Raw), open: n.Start, close: n.End}
	pa.pieces = []piece{{unit: len(d.ext.Units), src: src, ids: map[string]string{}}}
	d.ext.Units = append(d.ext.Units, filter.Unit{ID: id + "/s1", Block: id, Source: src})
	d.ext.Blocks = append(d.ext.Blocks, block)
	d.labels = append(d.labels, labelRef{part: p, unit: len(d.ext.Units) - 1})
	p.paras = append(p.paras, pa)
}

func (d *document) addParagraph(p *part, n *node, id string, dia dialect, prefix string, ctx walkCtx, textbox, mirror bool) error {
	if len(n.End) == 0 {
		return nil // <w:p/>: empty paragraph
	}
	pa := &para{dialect: dia, ns: prefix, start: n.Off, end: n.Off + len(n.Raw), open: n.Start, close: n.End, mirror: mirror}
	block := filter.Block{ID: id, Part: p.kind, File: p.file}
	if textbox {
		block.Frame = "textbox"
	}
	if ctx.tableDepth > 0 {
		block.Cell = &filter.Cell{Table: ctx.table, Row: ctx.row, Col: ctx.col}
	}
	switch p.kind {
	case filter.PartFootnote, filter.PartEndnote, filter.PartComment:
		block.Note = ctx.note
	}
	if ppr := n.Child("pPr"); ppr != nil && ppr.Prefix == prefix {
		pa.ppr = ppr.Raw
		if dia == dml {
			if v, ok := ppr.AttrAny("algn"); ok {
				block.Align = map[string]string{"ctr": "center", "r": "right", "just": "both", "l": "left"}[v]
			}
		} else {
			if s := ppr.Child("pStyle"); s != nil {
				block.Style, _ = s.AttrAny("val")
			}
			if o := ppr.Child("outlineLvl"); o != nil {
				if v, _ := o.AttrAny("val"); v != "" {
					if lvl, err := strconv.Atoi(v); err == nil && lvl < 9 {
						block.Heading = lvl + 1
					}
				}
			}
			if ppr.Child("numPr") != nil {
				block.List = true
			}
			if jc := ppr.Child("jc"); jc != nil {
				block.Align, _ = jc.AttrAny("val")
			}
		}
	}
	if block.Style != "" {
		lvl, name, numbered := d.heading(block.Style)
		block.StyleName = name
		if block.Heading == 0 {
			block.Heading = lvl
		}
		block.List = block.List || (numbered && lvl == 0)
	}

	ip := &itemParser{w: prefix, keepRevisions: d.opts.KeepRevisions}
	if dia == dml {
		ip.dmlItems(n)
	} else {
		ip.walk(n.Kids, nil, true)
		ip.flushField()
	}
	c, base, ranges := toContent(ip.items, dia == dml, d.styleName)
	pa.base, pa.ranges = base, ranges
	c, pa.prefix, pa.suffix = parag.StripEdges(c)
	if !parag.HasText(c) {
		return nil // nothing to translate; the paragraph is copied as it is
	}
	block.Revisions = ip.revisions
	for _, r := range pa.prefix {
		if r.Tag.Kind != content.TagClose {
			block.Lead = append(block.Lead, r.Tag.Type)
		}
	}
	for _, r := range pa.suffix {
		if r.Tag.Kind == content.TagStandalone {
			block.Trail = append(block.Trail, r.Tag.Type)
		}
	}
	var sig []string
	n2 := 0
	for _, pc := range d.splitParagraph(c) {
		if pc.unit < 0 {
			pa.pieces = append(pa.pieces, pc)
			continue
		}
		src, ids := parag.Renumber(pc.gap)
		sig = append(sig, src.String())
		if mirror {
			pa.pieces = append(pa.pieces, piece{unit: -2, src: src, ids: ids})
			continue
		}
		n2++
		pa.pieces = append(pa.pieces, piece{unit: len(d.ext.Units), src: src, ids: ids})
		d.ext.Units = append(d.ext.Units, filter.Unit{ID: fmt.Sprintf("%s/s%d", block.ID, n2), Block: block.ID, Source: src})
	}
	pa.sig = strings.Join(sig, "\x00")
	if !mirror {
		d.ext.Blocks = append(d.ext.Blocks, block)
		if _, seen := d.sigs[pa.sig]; !seen {
			d.sigs[pa.sig] = pa
		}
	}
	p.paras = append(p.paras, pa)
	return nil
}

// resolveMirrors gives mirror paragraphs the units of the paragraph with
// the same source.
func (d *document) resolveMirrors() {
	for _, p := range d.parts {
		for _, pa := range p.paras {
			if !pa.mirror {
				continue
			}
			twin, ok := d.sigs[pa.sig]
			if !ok {
				continue // left as it is
			}
			var units []int
			for _, pc := range twin.pieces {
				if pc.unit >= 0 {
					units = append(units, pc.unit)
				}
			}
			k := 0
			for i := range pa.pieces {
				if pa.pieces[i].unit == -2 && k < len(units) {
					pa.pieces[i].unit = units[k]
					k++
				}
			}
		}
	}
}

// wrapper is markup around a stretch of text that must stay around its
// translation: a link, a content control, a tracked insertion.
type wrapper struct {
	typ, desc   string
	open, close string
}

// item is a stretch of text with one set of run properties, or a
// placeholder tag, in paragraph order.
type item struct {
	text   string
	rpr    []byte
	isText bool
	tag    content.Tag // standalone placeholder: Type, Desc and Native set
	ranges []span
	wraps  []*wrapper // enclosing wrappers, outermost first
}

type itemParser struct {
	w             string
	items         []item
	keepRevisions bool
	revisions     bool
	// field collects the runs of a complex field (begin to end) so the whole
	// field, code and result, travels as one placeholder.
	field      bytes.Buffer
	fieldSpans []span
	fieldDepth int
	fieldWraps []*wrapper
}

var instrRe = regexp.MustCompile(`<[a-zA-Z0-9]*:?instrText[^>]*>([^<]*)<`)

func placeholderDesc(typ, native string) string {
	switch typ {
	case "tab":
		return "Tab"
	case "lb":
		return "Line break"
	case "pagebreak":
		return "Page break"
	case "columnbreak":
		return "Column break"
	case "field":
		var code []string
		for _, m := range instrRe.FindAllStringSubmatch(native, -1) {
			code = append(code, m[1])
		}
		if c := strings.Fields(strings.Join(code, "")); len(c) > 0 {
			return "Field " + c[0]
		}
		if i := strings.Index(native, `instr="`); i >= 0 {
			if f := strings.Fields(native[i+7:]); len(f) > 0 {
				return "Field " + strings.Trim(f[0], `"`)
			}
		}
		return "Field"
	case "footnote":
		return "Footnote reference"
	case "note-mark":
		return "Note number"
	case "image":
		return "Image or drawing"
	case "symbol":
		return "Symbol"
	case "anchor":
		return "Bookmark or comment range"
	case "comment":
		return "Comment mark"
	case "control":
		return "Content control"
	case "math":
		return "Equation"
	case "deletion":
		return "Tracked deletion"
	case "end":
		return "Paragraph end"
	}
	return "Inline code"
}

func (ip *itemParser) placeholder(typ string, native []byte, ranges []span, wraps []*wrapper) {
	n := string(native)
	ip.items = append(ip.items, item{tag: content.Tag{Kind: content.TagStandalone, Type: typ, Desc: placeholderDesc(typ, n), Native: n}, ranges: ranges, wraps: wraps})
}

func (ip *itemParser) text(s string, rpr []byte, wraps []*wrapper) {
	if s == "" {
		return
	}
	if n := len(ip.items); n > 0 {
		last := &ip.items[n-1]
		if last.isText && sameWraps(last.wraps, wraps) && bytes.Equal(last.rpr, rpr) {
			last.text += s
			return
		}
	}
	ip.items = append(ip.items, item{text: s, rpr: rpr, isText: true, wraps: wraps})
}

func sameWraps(a, b []*wrapper) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func withWrap(ws []*wrapper, w *wrapper) []*wrapper {
	out := make([]*wrapper, len(ws), len(ws)+1)
	copy(out, ws)
	return append(out, w)
}

func (ip *itemParser) flushField() {
	if ip.field.Len() == 0 {
		return
	}
	ip.placeholder("field", bytes.Clone(ip.field.Bytes()), ip.fieldSpans, ip.fieldWraps)
	ip.field.Reset()
	ip.fieldSpans = nil
	ip.fieldDepth = 0
	ip.fieldWraps = nil
}

func (ip *itemParser) walk(kids []*node, wraps []*wrapper, top bool) {
	for _, k := range kids {
		if !k.IsElement() {
			continue // whitespace between elements carries no meaning here
		}
		if ip.fieldDepth > 0 {
			ip.field.Write(k.Raw)
			ip.fieldSpans = append(ip.fieldSpans, k.Whole())
			b, e := countFieldChars(k)
			ip.fieldDepth += b - e
			if ip.fieldDepth <= 0 {
				ip.flushField()
			}
			continue
		}
		if k.Prefix != ip.w {
			ip.placeholder(otherType(k), k.Raw, []span{k.Whole()}, wraps)
			continue
		}
		switch k.Local {
		case "pPr":
			if !top {
				ip.placeholder("inline", k.Raw, []span{k.Whole()}, wraps)
			}
		case "r":
			ip.run(k, wraps)
		case "hyperlink":
			ip.walk(k.Kids, withWrap(wraps, &wrapper{typ: "link", desc: "Link", open: string(k.Start), close: string(k.End)}), false)
		case "sdt":
			c := k.Child("sdtContent")
			if c == nil || len(c.End) == 0 {
				ip.placeholder("control", k.Raw, []span{k.Whole()}, wraps)
				continue
			}
			// The control's properties stay with its start; its content is text.
			open := string(k.Raw[:c.Off-k.Off+len(c.Start)])
			ip.walk(c.Kids, withWrap(wraps, &wrapper{typ: "control", desc: "Content control", open: open, close: string(c.End) + string(k.End)}), false)
		case "ins", "moveTo":
			ip.revisions = true
			if ip.keepRevisions {
				ip.walk(k.Kids, withWrap(wraps, &wrapper{typ: "insertion", desc: "Tracked insertion", open: string(k.Start), close: string(k.End)}), false)
			} else {
				ip.walk(k.Kids, wraps, false)
			}
		case "del", "moveFrom":
			ip.revisions = true
			if ip.keepRevisions {
				ip.placeholder("deletion", k.Raw, []span{k.Whole()}, wraps)
			}
		case "smartTag", "customXml", "dir", "bdo":
			ip.walk(k.Kids, wraps, false)
		case "proofErr":
			// spelling marks are dropped
		case "bookmarkStart", "bookmarkEnd", "commentRangeStart", "commentRangeEnd",
			"permStart", "permEnd", "moveFromRangeStart", "moveFromRangeEnd", "moveToRangeStart", "moveToRangeEnd":
			ip.placeholder("anchor", k.Raw, []span{k.Whole()}, wraps)
		case "fldSimple":
			ip.placeholder("field", k.Raw, []span{k.Whole()}, wraps)
		default:
			ip.placeholder("inline", k.Raw, []span{k.Whole()}, wraps)
		}
	}
}

func otherType(k *node) string {
	switch k.Local {
	case "oMath", "oMathPara":
		return "math"
	case "AlternateContent":
		return "image"
	}
	return "inline"
}

// countFieldChars counts field begin and end markers in an element.
func countFieldChars(n *node) (begins, ends int) {
	if n.Local == "fldChar" {
		switch v, _ := n.AttrAny("fldCharType"); v {
		case "begin":
			return 1, 0
		case "end":
			return 0, 1
		}
	}
	for _, k := range n.Kids {
		b, e := countFieldChars(k)
		begins += b
		ends += e
	}
	return begins, ends
}

func (ip *itemParser) run(r *node, wraps []*wrapper) {
	if b, e := countFieldChars(r); b > 0 || e > 0 || r.Child("instrText") != nil {
		if b > 0 {
			ip.fieldWraps = wraps
			ip.field.Write(r.Raw)
			ip.fieldSpans = append(ip.fieldSpans, r.Whole())
			ip.fieldDepth = b - e
			if ip.fieldDepth <= 0 {
				ip.flushField()
			}
			return
		}
		// The end of a field that began in an earlier paragraph.
		ip.placeholder("field", r.Raw, []span{r.Whole()}, wraps)
		return
	}
	var rpr []byte
	var rprSpan []span
	if p := r.Child("rPr"); p != nil {
		rpr = p.Raw
		rprSpan = []span{p.Whole()}
	}
	wrap := func(c *node) ([]byte, []span) {
		var b bytes.Buffer
		b.Write(r.Start)
		b.Write(rpr)
		b.Write(c.Raw)
		b.Write(r.End)
		spans := append([]span{r.StartTag()}, rprSpan...)
		spans = append(spans, c.Whole(), r.EndTag())
		return b.Bytes(), spans
	}
	for _, c := range r.Kids {
		if !c.IsElement() || c.Local == "rPr" {
			continue
		}
		if c.Prefix != ip.w {
			nat, sp := wrap(c)
			ip.placeholder(otherType(c), nat, sp, wraps)
			continue
		}
		typ := ""
		switch c.Local {
		case "t":
			ip.text(c.InnerText(), rpr, wraps)
		case "noBreakHyphen":
			ip.text("‑", rpr, wraps)
		case "softHyphen":
			ip.text("­", rpr, wraps)
		case "tab", "ptab":
			typ = "tab"
		case "br":
			typ = "lb"
			if v, _ := c.AttrAny("type"); v == "page" || v == "column" {
				typ = v + "break"
			}
		case "cr":
			typ = "lb"
		case "footnoteReference", "endnoteReference":
			typ = "footnote"
		case "footnoteRef", "endnoteRef", "annotationRef":
			typ = "note-mark"
		case "drawing", "pict", "object":
			typ = "image"
		case "sym":
			typ = "symbol"
		case "commentReference":
			typ = "comment"
		case "delText", "lastRenderedPageBreak":
			// not part of the visible text
		default:
			typ = "inline"
		}
		if typ != "" {
			nat, sp := wrap(c)
			ip.placeholder(typ, nat, sp, wraps)
		}
	}
}

// dmlItems reads a DrawingML paragraph (charts, SmartArt).
func (ip *itemParser) dmlItems(p *node) {
	for _, k := range p.Kids {
		if !k.IsElement() {
			continue
		}
		if k.Prefix != ip.w {
			ip.placeholder(otherType(k), k.Raw, []span{k.Whole()}, nil)
			continue
		}
		switch k.Local {
		case "pPr":
		case "r":
			var rpr []byte
			if p := k.Child("rPr"); p != nil {
				rpr = p.Raw
			}
			if t := k.Child("t"); t != nil {
				ip.text(t.InnerText(), rpr, nil)
			}
		case "br":
			ip.placeholder("lb", k.Raw, []span{k.Whole()}, nil)
		case "fld":
			nat := string(k.Raw)
			desc := "Field"
			if v, ok := k.AttrAny("type"); ok {
				desc = "Field " + v
			}
			ip.items = append(ip.items, item{tag: content.Tag{Kind: content.TagStandalone, Type: "field", Desc: desc, Native: nat}, ranges: []span{k.Whole()}})
		case "endParaRPr":
			ip.placeholder("end", k.Raw, []span{k.Whole()}, nil)
		default:
			ip.placeholder("inline", k.Raw, []span{k.Whole()}, nil)
		}
	}
}

// toContent turns items into tagged content. It returns the run properties
// most of the text uses, and the byte ranges of placeholder markup by tag ID.
func toContent(items []item, isDML bool, styleName func(string) string) (content.Content, []byte, map[string][]span) {
	counts := map[string]int{}
	rprOf := map[string][]byte{}
	keyOf := make([]string, len(items))
	for i, it := range items {
		if !it.isText {
			continue
		}
		k := propKey(it.rpr, isDML)
		keyOf[i] = k
		counts[k] += len([]rune(it.text))
		if _, ok := rprOf[k]; !ok {
			rprOf[k] = it.rpr
		}
	}
	best, bestN := "", -1
	for i, it := range items {
		if it.isText && counts[keyOf[i]] > bestN {
			best, bestN = keyOf[i], counts[keyOf[i]]
		}
	}
	base := rprOf[best]

	// Text inside a link usually has one character style (Hyperlink). That
	// style travels with the link tag instead of adding a second tag pair.
	linkKey := map[*wrapper]string{}
	linkRPr := map[*wrapper][]byte{}
	mixed := map[*wrapper]bool{}
	for i, it := range items {
		if !it.isText || len(it.wraps) == 0 {
			continue
		}
		l := it.wraps[len(it.wraps)-1]
		if l.typ != "link" {
			continue
		}
		if k, ok := linkKey[l]; ok && k != keyOf[i] {
			mixed[l] = true
		}
		if _, ok := linkKey[l]; !ok {
			linkKey[l], linkRPr[l] = keyOf[i], it.rpr
		}
	}

	var c content.Content
	ranges := map[string][]span{}
	next := 0
	newID := func() string { next++; return strconv.Itoa(next) }

	type open struct {
		w       *wrapper
		id      string
		baseKey string
		baseRPr []byte
	}
	var opened []open
	fmtID, fmtKey, fmtType, fmtDesc := "", best, "", ""
	spanBase := func() (string, []byte) {
		if n := len(opened); n > 0 {
			return opened[n-1].baseKey, opened[n-1].baseRPr
		}
		return best, base
	}
	closeFmt := func() {
		if fmtID != "" {
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: fmtID, Type: fmtType, Desc: fmtDesc})
			fmtID = ""
		}
		fmtKey, _ = spanBase()
	}
	setWraps := func(ws []*wrapper) {
		k := 0
		for k < len(opened) && k < len(ws) && opened[k].w == ws[k] {
			k++
		}
		if k == len(opened) && k == len(ws) {
			return
		}
		closeFmt()
		for len(opened) > k {
			o := opened[len(opened)-1]
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: o.id, Type: o.w.typ, Desc: o.w.desc, Native: o.w.close})
			opened = opened[:len(opened)-1]
		}
		for _, w := range ws[k:] {
			bk, br := spanBase()
			o := open{w: w, id: newID(), baseKey: bk, baseRPr: br}
			native := w.open
			if lk, ok := linkKey[w]; ok && !mixed[w] && lk != bk {
				native += string(linkRPr[w])
				o.baseKey, o.baseRPr = lk, linkRPr[w]
			}
			opened = append(opened, o)
			c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: o.id, Type: w.typ, Desc: w.desc, Native: native})
		}
		fmtKey, _ = spanBase()
	}
	for i, it := range items {
		setWraps(it.wraps)
		if !it.isText {
			t := it.tag
			t.ID = newID()
			ranges[t.ID] = it.ranges
			c = c.AppendTag(t)
			continue
		}
		if k := keyOf[i]; k != fmtKey {
			closeFmt()
			if bk, br := spanBase(); k != bk {
				fmtID, fmtKey = newID(), k
				fmtType, fmtDesc = describe(br, it.rpr, isDML, styleName)
				c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: fmtID, Type: fmtType, Desc: fmtDesc, Native: string(it.rpr)})
			}
		}
		c = c.Append(it.text)
	}
	setWraps(nil)
	closeFmt()
	return c, base, ranges
}

// splitParagraph cuts paragraph content into units and gaps. A sentence
// break inside a pair of tags is not used, so tags never cross units.
func (d *document) splitParagraph(c content.Content) []piece {
	var out []piece
	for _, part := range parag.Cut(c, parag.Breaks(d.seg, c)) {
		lead, mid, trail := parag.TrimSpace(part)
		if !parag.HasText(mid) {
			out = appendGap(out, part)
			continue
		}
		out = appendGap(out, lead)
		out = append(out, piece{unit: 0, gap: mid})
		out = appendGap(out, trail)
	}
	return out
}

func appendGap(ps []piece, g content.Content) []piece {
	if len(g) == 0 {
		return ps
	}
	if n := len(ps); n > 0 && ps[n-1].unit == -1 {
		ps[n-1].gap = append(ps[n-1].gap, g...).Normalize()
		return ps
	}
	return append(ps, piece{unit: -1, gap: g.Clone()})
}
