package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

// para is one paragraph that may contain units, with its byte range in the
// part and the pieces it is rebuilt from.
type para struct {
	start, end int    // byte range of the w:p element
	open       []byte // raw start tag
	close      []byte // raw end tag
	ppr        []byte // raw paragraph properties, if any
	base       []byte // run properties of most of the text
	// prefix and suffix hold tags at the paragraph edges that are not shown
	// to translators (a footnote number, an anchor, a link around everything).
	prefix, suffix content.Content
	pieces         []piece
}

// piece is either a unit (by index into the extraction) or the whitespace
// and tags between units.
type piece struct {
	unit int // -1 for a gap
	gap  content.Content
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
}

func (d *document) parsePart(file string, kind filter.Part, data []byte) (*part, error) {
	toks, err := tokenize(data)
	if err != nil {
		return nil, err
	}
	p := &part{file: file, kind: kind, data: data}
	// Find the prefix bound to the main namespace on the root element.
	for _, t := range toks {
		if st, ok := t.tok.(xml.StartElement); ok {
			for _, a := range st.Attr {
				if a.Name.Space == "xmlns" && a.Value == nsMain {
					p.w = a.Name.Local
				}
			}
			break
		}
	}
	if p.w == "" {
		return p, nil // not a WordprocessingML part; nothing to extract
	}
	ctx := walkCtx{table: -1}
	var noteSkip bool
	paraIndex := 0
	for i := 0; i < len(toks); {
		switch t := toks[i].tok.(type) {
		case xml.StartElement:
			if t.Name.Space != p.w {
				i++
				continue
			}
			switch t.Name.Local {
			case "p":
				n, next, err := build(data, toks, i)
				if err != nil {
					return nil, err
				}
				if !noteSkip {
					paraIndex++
					if err := d.addParagraph(p, n, toks[i].start, toks[next-1].end, paraIndex, ctx); err != nil {
						return nil, err
					}
				}
				i = next
				continue
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
			case "footnote", "endnote":
				ctx.note = ""
				noteSkip = false
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "id":
						ctx.note = a.Value
					case "type":
						// separator and continuation notice notes are layout, not text
						noteSkip = a.Value != "normal"
					}
				}
			}
			// A self-closing element produces its end token right after.
		case xml.EndElement:
			if t.Name.Space == p.w {
				switch t.Name.Local {
				case "tbl":
					ctx.tableDepth--
				case "footnote", "endnote":
					ctx.note = ""
					noteSkip = false
				}
			}
		}
		i++
	}
	return p, nil
}

func (d *document) addParagraph(p *part, n *node, start, end, index int, ctx walkCtx) error {
	pa := &para{start: start, end: end, open: n.start, close: n.end}
	if len(n.end) == 0 {
		return nil // <w:p/>: empty paragraph
	}
	block := filter.Block{
		ID:   fmt.Sprintf("%s/p%d", shortName(p.file), index),
		Part: p.kind,
		File: p.file,
	}
	if ctx.tableDepth > 0 {
		block.Cell = &filter.Cell{Table: ctx.table, Row: ctx.row, Col: ctx.col}
	}
	if p.kind == filter.PartFootnote || p.kind == filter.PartEndnote {
		block.Note = ctx.note
	}
	if ppr := n.child("pPr"); ppr != nil && ppr.prefix == p.w {
		pa.ppr = ppr.raw
		if s := ppr.child("pStyle"); s != nil {
			block.Style, _ = s.attr("val")
		}
		if o := ppr.child("outlineLvl"); o != nil {
			if v, _ := o.attr("val"); v != "" {
				if lvl, err := strconv.Atoi(v); err == nil && lvl < 9 {
					block.Heading = lvl + 1
				}
			}
		}
		if ppr.child("numPr") != nil {
			block.List = true
		}
		if jc := ppr.child("jc"); jc != nil {
			block.Align, _ = jc.attr("val")
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

	ip := &itemParser{w: p.w}
	ip.walk(n.kids, nil, true)
	ip.flushField()
	c, base := ip.toContent()
	pa.base = base
	c, pa.prefix, pa.suffix = stripEdges(c)
	if !hasText(c) {
		return nil // nothing to translate; the paragraph is copied as it is
	}
	for _, pc := range d.splitParagraph(c) {
		if pc.unit < 0 {
			pa.pieces = append(pa.pieces, pc)
			continue
		}
		u := filter.Unit{
			ID:     fmt.Sprintf("%s/s%d", block.ID, len(unitPieces(pa.pieces))+1),
			Block:  block.ID,
			Source: renumber(pc.gap),
		}
		pa.pieces = append(pa.pieces, piece{unit: len(d.ext.Units)})
		d.ext.Units = append(d.ext.Units, u)
	}
	d.ext.Blocks = append(d.ext.Blocks, block)
	p.paras = append(p.paras, pa)
	return nil
}

func unitPieces(ps []piece) []piece {
	var out []piece
	for _, p := range ps {
		if p.unit >= 0 {
			out = append(out, p)
		}
	}
	return out
}

// item is a stretch of text with one set of run properties, or a
// placeholder tag, in paragraph order.
type item struct {
	text   string
	rpr    []byte
	isText bool
	tag    content.Tag // standalone placeholder: Type and Native set
	link   *node       // enclosing hyperlink, if any
}

type itemParser struct {
	w     string
	items []item
	// field collects the runs of a complex field (begin to end) so the whole
	// field, code and result, travels as one placeholder.
	field      bytes.Buffer
	fieldDepth int
	fieldLink  *node
}

func (ip *itemParser) placeholder(typ string, native []byte, link *node) {
	ip.items = append(ip.items, item{tag: content.Tag{Kind: content.TagStandalone, Type: typ, Native: string(native)}, link: link})
}

func (ip *itemParser) text(s string, rpr []byte, link *node) {
	if s == "" {
		return
	}
	if n := len(ip.items); n > 0 {
		last := &ip.items[n-1]
		if last.isText && last.link == link && bytes.Equal(last.rpr, rpr) {
			last.text += s
			return
		}
	}
	ip.items = append(ip.items, item{text: s, rpr: rpr, isText: true, link: link})
}

func (ip *itemParser) flushField() {
	if ip.field.Len() == 0 {
		return
	}
	ip.placeholder("field", bytes.Clone(ip.field.Bytes()), ip.fieldLink)
	ip.field.Reset()
	ip.fieldDepth = 0
	ip.fieldLink = nil
}

func (ip *itemParser) walk(kids []*node, link *node, top bool) {
	for _, k := range kids {
		if k.isText() {
			continue // whitespace between elements carries no meaning here
		}
		if ip.fieldDepth > 0 {
			ip.field.Write(k.raw)
			b, e := countFieldChars(k)
			ip.fieldDepth += b - e
			if ip.fieldDepth <= 0 {
				ip.flushField()
			}
			continue
		}
		if k.prefix != ip.w {
			ip.placeholder(otherType(k), k.raw, link)
			continue
		}
		switch k.local {
		case "pPr":
			if !top {
				ip.placeholder("inline", k.raw, link)
			}
		case "r":
			ip.run(k, link)
		case "hyperlink":
			ip.walk(k.kids, k, false)
		case "ins", "smartTag", "customXml", "moveTo", "dir", "bdo":
			ip.walk(k.kids, link, false)
		case "del", "moveFrom", "proofErr":
			// deleted text and spelling marks are dropped
		case "bookmarkStart", "bookmarkEnd", "commentRangeStart", "commentRangeEnd",
			"permStart", "permEnd", "moveFromRangeStart", "moveFromRangeEnd", "moveToRangeStart", "moveToRangeEnd":
			ip.placeholder("anchor", k.raw, link)
		case "fldSimple":
			ip.placeholder("field", k.raw, link)
		case "sdt":
			ip.placeholder("control", k.raw, link)
		default:
			ip.placeholder("inline", k.raw, link)
		}
	}
}

func otherType(k *node) string {
	switch k.local {
	case "oMath", "oMathPara":
		return "math"
	case "AlternateContent":
		return "image"
	}
	return "inline"
}

// countFieldChars counts field begin and end markers in an element.
func countFieldChars(n *node) (begins, ends int) {
	if n.local == "fldChar" {
		switch v, _ := n.attr("fldCharType"); v {
		case "begin":
			return 1, 0
		case "end":
			return 0, 1
		}
	}
	for _, k := range n.kids {
		b, e := countFieldChars(k)
		begins += b
		ends += e
	}
	return begins, ends
}

func (ip *itemParser) run(r *node, link *node) {
	if b, e := countFieldChars(r); b > 0 || e > 0 || r.child("instrText") != nil {
		if b > 0 {
			ip.fieldLink = link
			ip.field.Write(r.raw)
			ip.fieldDepth = b - e
			if ip.fieldDepth <= 0 {
				ip.flushField()
			}
			return
		}
		// The end of a field that began in an earlier paragraph.
		ip.placeholder("field", r.raw, link)
		return
	}
	var rpr []byte
	if p := r.child("rPr"); p != nil {
		rpr = p.raw
	}
	wrap := func(inner []byte) []byte {
		var b bytes.Buffer
		b.Write(r.start)
		b.Write(rpr)
		b.Write(inner)
		b.Write(r.end)
		return b.Bytes()
	}
	for _, c := range r.kids {
		if c.isText() || c.local == "rPr" {
			continue
		}
		if c.prefix != ip.w {
			ip.placeholder(otherType(c), wrap(c.raw), link)
			continue
		}
		switch c.local {
		case "t":
			ip.text(c.innerText(), rpr, link)
		case "noBreakHyphen":
			ip.text("‑", rpr, link)
		case "softHyphen":
			ip.text("­", rpr, link)
		case "tab", "ptab":
			ip.placeholder("tab", wrap(c.raw), link)
		case "br":
			typ := "lb"
			if v, _ := c.attr("type"); v == "page" || v == "column" {
				typ = v + "break"
			}
			ip.placeholder(typ, wrap(c.raw), link)
		case "cr":
			ip.placeholder("lb", wrap(c.raw), link)
		case "footnoteReference", "endnoteReference":
			ip.placeholder("footnote", wrap(c.raw), link)
		case "footnoteRef", "endnoteRef":
			ip.placeholder("note-mark", wrap(c.raw), link)
		case "drawing", "pict", "object":
			ip.placeholder("image", wrap(c.raw), link)
		case "sym":
			ip.placeholder("symbol", wrap(c.raw), link)
		case "commentReference", "annotationRef":
			ip.placeholder("comment", wrap(c.raw), link)
		case "delText", "lastRenderedPageBreak":
			// not part of the visible text
		default:
			ip.placeholder("inline", wrap(c.raw), link)
		}
	}
}

// rprKey normalizes run properties for comparison: the language and
// proofing marks do not count as a formatting change.
func rprKey(rpr []byte) string {
	if len(rpr) == 0 {
		return ""
	}
	n, err := parseFragment(rpr)
	if err != nil {
		return string(rpr)
	}
	var b strings.Builder
	for _, k := range n.kids {
		if k.isText() {
			continue
		}
		switch k.local {
		case "lang", "noProof", "rPrChange":
			continue
		}
		b.Write(k.raw)
	}
	return b.String()
}

// toContent turns the items into tagged content. It returns the run
// properties most of the text uses; text with other properties is wrapped
// in paired tags whose native code is the run properties to restore.
func (ip *itemParser) toContent() (content.Content, []byte) {
	counts := map[string]int{}
	rprOf := map[string][]byte{}
	keyOf := make([]string, len(ip.items))
	best, bestN := "", -1
	for i, it := range ip.items {
		if !it.isText {
			continue
		}
		k := rprKey(it.rpr)
		keyOf[i] = k
		counts[k] += len([]rune(it.text))
		if _, ok := rprOf[k]; !ok {
			rprOf[k] = it.rpr
		}
	}
	// Deterministic choice: most characters, then first seen.
	for i, it := range ip.items {
		if !it.isText {
			continue
		}
		if counts[keyOf[i]] > bestN {
			best, bestN = keyOf[i], counts[keyOf[i]]
		}
	}
	base := rprOf[best]

	// Text inside a link usually has one character style (Hyperlink). That
	// style travels with the link tag instead of adding a second tag pair.
	linkKey := map[*node]string{}
	linkRPr := map[*node][]byte{}
	mixed := map[*node]bool{}
	for i, it := range ip.items {
		if !it.isText || it.link == nil {
			continue
		}
		if k, ok := linkKey[it.link]; ok && k != keyOf[i] {
			mixed[it.link] = true
		}
		if _, ok := linkKey[it.link]; !ok {
			linkKey[it.link], linkRPr[it.link] = keyOf[i], it.rpr
		}
	}
	for l := range mixed {
		delete(linkKey, l)
		delete(linkRPr, l)
	}

	var c content.Content
	next := 0
	newID := func() string { next++; return strconv.Itoa(next) }
	var curLink *node
	linkID, fmtID, fmtKey := "", "", best
	// spanBase is the formatting text has without a tag: the paragraph's,
	// or the link's own when the link carries it.
	spanBase := best
	closeFmt := func() {
		if fmtID != "" {
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: fmtID})
			fmtID = ""
		}
		fmtKey = spanBase
	}
	setLink := func(l *node) {
		if l == curLink {
			return
		}
		closeFmt()
		if curLink != nil {
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: linkID, Type: "link"})
		}
		curLink, linkID, spanBase = l, "", best
		if l != nil {
			linkID = newID()
			native := string(l.start)
			if k, ok := linkKey[l]; ok && k != best {
				native += string(linkRPr[l])
				spanBase = k
			}
			c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: linkID, Type: "link", Native: native})
		}
		fmtKey = spanBase
	}
	for i, it := range ip.items {
		setLink(it.link)
		if !it.isText {
			t := it.tag
			t.ID = newID()
			c = c.AppendTag(t)
			continue
		}
		if k := keyOf[i]; k != fmtKey {
			closeFmt()
			if k != spanBase {
				fmtID, fmtKey = newID(), k
				c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: fmtID, Type: formatType(base, it.rpr), Native: string(it.rpr)})
			}
		}
		c = c.Append(it.text)
	}
	setLink(nil)
	closeFmt()
	// Close tags carry the same type as their open tags.
	types := map[string]string{}
	for _, r := range c {
		if r.IsTag() && r.Tag.Kind == content.TagOpen {
			types[r.Tag.ID] = r.Tag.Type
		}
	}
	for i := range c {
		if c[i].IsTag() && c[i].Tag.Kind == content.TagClose {
			c[i].Tag.Type = types[c[i].Tag.ID]
		}
	}
	return c, base
}

// formatType names the difference between two run properties for display.
func formatType(base, rpr []byte) string {
	set := func(b []byte) map[string]string {
		out := map[string]string{}
		if len(b) == 0 {
			return out
		}
		n, err := parseFragment(b)
		if err != nil {
			return out
		}
		for _, k := range n.kids {
			if k.isText() || k.local == "lang" || k.local == "noProof" {
				continue
			}
			v, _ := k.attr("val")
			out[k.local] = v
		}
		return out
	}
	a, b := set(base), set(rpr)
	var added []string
	for k, v := range b {
		if av, ok := a[k]; !ok || av != v {
			added = append(added, k+"="+v)
		}
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return "format"
		}
	}
	only := func(names ...string) bool {
		if len(added) == 0 {
			return false
		}
		for _, x := range added {
			ok := false
			for _, n := range names {
				if x == n {
					ok = true
				}
			}
			if !ok {
				return false
			}
		}
		return true
	}
	switch {
	case only("b=", "bCs=", "b=1", "b=true", "bCs=1", "bCs=true"):
		return "bold"
	case only("i=", "iCs=", "i=1", "i=true", "iCs=1", "iCs=true"):
		return "italic"
	case len(added) == 1 && strings.HasPrefix(added[0], "u=") && added[0] != "u=none":
		return "underline"
	case only("vertAlign=superscript"):
		return "superscript"
	case only("vertAlign=subscript"):
		return "subscript"
	}
	return "format"
}

// stripEdges moves tags at the paragraph edges out of the translatable
// content: standalone tags before the first or after the last text, and a
// pair of tags that encloses everything.
func stripEdges(c content.Content) (body, prefix, suffix content.Content) {
	body = c
	prefix, suffix = content.Content{}, content.Content{}
	for changed := true; changed; {
		changed = false
		for len(body) > 0 && body[0].IsTag() && body[0].Tag.Kind == content.TagStandalone {
			prefix = append(prefix, body[0])
			body = body[1:]
			changed = true
		}
		for len(body) > 0 && body[len(body)-1].IsTag() && body[len(body)-1].Tag.Kind == content.TagStandalone {
			suffix = append(content.Content{body[len(body)-1]}, suffix...)
			body = body[:len(body)-1]
			changed = true
		}
		if len(body) >= 2 && body[0].IsTag() && body[0].Tag.Kind == content.TagOpen {
			last := body[len(body)-1]
			if last.IsTag() && last.Tag.Kind == content.TagClose && last.Tag.ID == body[0].Tag.ID {
				prefix = append(prefix, body[0])
				suffix = append(content.Content{last}, suffix...)
				body = body[1 : len(body)-1]
				changed = true
			}
		}
	}
	return body, prefix, suffix
}

func hasText(c content.Content) bool {
	for _, r := range c {
		if !r.IsTag() && strings.TrimFunc(r.Text, unicode.IsSpace) != "" {
			return true
		}
	}
	return false
}

// splitParagraph cuts paragraph content into units and gaps. A sentence
// break inside a pair of tags is not used, so tags never cross units.
func (d *document) splitParagraph(c content.Content) []piece {
	breaks := d.seg.Breaks(c.Plain())
	// Depth of open pairs at each break: opens before the break minus closes
	// at or before it (close tags at a break end the previous unit).
	var ok []int
	for _, b := range breaks {
		depth, pos := 0, 0
		for _, r := range c {
			if !r.IsTag() {
				pos += len(r.Text)
				continue
			}
			switch r.Tag.Kind {
			case content.TagOpen:
				if pos < b {
					depth++
				}
			case content.TagClose:
				if pos <= b {
					depth--
				}
			}
		}
		if depth == 0 {
			ok = append(ok, b)
		}
	}
	var out []piece
	for _, part := range cut(c, ok) {
		lead, mid, trail := trimSpace(part)
		if !hasText(mid) {
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
	if n := len(ps); n > 0 && ps[n-1].unit < 0 {
		ps[n-1].gap = append(ps[n-1].gap, g...).Normalize()
		return ps
	}
	return append(ps, piece{unit: -1, gap: g.Clone()})
}

// cut splits content at plain-text offsets without losing anything. Close
// tags at a cut stay with the left part; open and standalone tags go right.
func cut(c content.Content, at []int) []content.Content {
	var out []content.Content
	cur := content.Content{}
	pos, k := 0, 0
	flush := func() {
		out = append(out, cur)
		cur = content.Content{}
		k++
	}
	for _, r := range c {
		if r.IsTag() {
			if k < len(at) && pos == at[k] && r.Tag.Kind != content.TagClose {
				flush()
			}
			cur = cur.AppendTag(*r.Tag)
			continue
		}
		s := r.Text
		for s != "" {
			if k < len(at) && pos == at[k] {
				flush()
				continue
			}
			n := len(s)
			if k < len(at) && at[k]-pos < n {
				n = at[k] - pos
			}
			cur = cur.Append(s[:n])
			s = s[n:]
			pos += n
		}
	}
	for k < len(at) && pos >= at[k] {
		flush()
	}
	return append(out, cur)
}

// trimSpace moves whitespace at the start and end of a part into gaps.
func trimSpace(c content.Content) (lead, mid, trail content.Content) {
	mid = c.Clone()
	lead, trail = content.Content{}, content.Content{}
	if len(mid) > 0 && !mid[0].IsTag() {
		t := mid[0].Text
		s := strings.TrimLeftFunc(t, unicode.IsSpace)
		if s != t {
			lead = content.Text(t[:len(t)-len(s)])
			if s == "" {
				mid = mid[1:]
			} else {
				mid[0].Text = s
			}
		}
	}
	if n := len(mid); n > 0 && !mid[n-1].IsTag() {
		t := mid[n-1].Text
		s := strings.TrimRightFunc(t, unicode.IsSpace)
		if s != t {
			trail = content.Text(t[len(s):])
			if s == "" {
				mid = mid[:n-1]
			} else {
				mid[n-1].Text = s
			}
		}
	}
	return lead, mid, trail
}

// renumber gives the tags of a unit IDs from 1 in order of appearance.
func renumber(c content.Content) content.Content {
	ids := map[string]string{}
	out := c.Clone()
	for i := range out {
		if !out[i].IsTag() {
			continue
		}
		t := out[i].Tag
		id, ok := ids[t.ID]
		if !ok {
			id = strconv.Itoa(len(ids) + 1)
			ids[t.ID] = id
		}
		t.ID = id
	}
	return out
}
