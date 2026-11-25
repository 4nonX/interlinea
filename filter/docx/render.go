package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/4nonX/interlinea/langtag"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

// render rebuilds the part with translated paragraphs. Bytes outside the
// rewritten paragraphs are copied unchanged.
func (p *part) render(d *document, tr filter.Translations, targetLang string) []byte {
	var out bytes.Buffer
	p.writeRange(&out, d, tr, targetLang, 0, len(p.data))
	return out.Bytes()
}

// writeRange copies data[s:e], replacing every paragraph inside it that has
// units with its rebuilt version. Paragraphs nested in another one (a text
// box in a drawing) are rebuilt when the outer paragraph writes the markup
// of the drawing through writeRange.
func (p *part) writeRange(out *bytes.Buffer, d *document, tr filter.Translations, lang string, s, e int) {
	pos := s
	for _, pa := range p.paras {
		if pa.start < pos || pa.end > e || !pa.hasUnits() {
			continue
		}
		out.Write(p.data[pos:pa.start])
		pa.render(out, p, d, tr, lang)
		pos = pa.end
	}
	out.Write(p.data[pos:e])
}

func (p *part) hasUnits() bool {
	for _, pa := range p.paras {
		if pa.hasUnits() {
			return true
		}
	}
	return false
}

type fmtSpan struct {
	key string
	rpr []byte
}

type wrapSpan struct {
	key   string
	owner int
	end   string
}

type runWriter struct {
	out   *bytes.Buffer
	part  *part
	doc   *document
	tr    filter.Translations
	pa    *para
	lang  string
	fmts  []fmtSpan
	wraps []wrapSpan
	cache map[string]string
	// dir is "rtl" or "ltr" when the paragraph has a translation, whose
	// direction its runs then take; empty leaves them as they are.
	dir string
}

func (pa *para) render(out *bytes.Buffer, p *part, d *document, tr filter.Translations, lang string) {
	if pa.dialect == label {
		u := d.ext.Units[pa.pieces[0].unit]
		text := u.Source.Plain()
		if t, ok := tr[u.ID]; ok && len(t) > 0 {
			text = t.Plain()
		}
		out.Write(pa.open)
		out.WriteString(escapeText(text))
		out.Write(pa.close)
		return
	}
	rw := &runWriter{out: out, part: p, doc: d, tr: tr, pa: pa, lang: lang, cache: map[string]string{}}
	out.Write(pa.open)
	ppr := pa.ppr
	if lang != "" && pa.dialect != dml && pa.translated(d, tr) {
		// A translated paragraph reads in its language's direction: an
		// Arabic translation of an English paragraph from the right, an
		// English one of an Arabic paragraph from the left.
		rw.dir = "ltr"
		if langtag.RTL(lang) {
			rw.dir = "rtl"
		}
		ppr = []byte(setParaDirection(ppr, pa.ns, rw.dir == "rtl"))
	}
	if lang != "" && pa.dialect == dml && pa.translated(d, tr) {
		ppr = []byte(setParaDirectionDML(ppr, pa.ns, langtag.RTL(lang)))
	}
	out.Write(ppr)
	// The edges and the gaps use the paragraph's own tag IDs.
	all := append(append(content.Content{}, pa.prefix...), pa.suffix...)
	for _, pc := range pa.pieces {
		if pc.unit == -1 {
			all = append(all, pc.gap...)
		}
	}
	paraLevel := tagSource{src: all}
	rw.write(pa.prefix, paraLevel, -1)
	for i := 0; i < len(pa.pieces); i++ {
		pc := pa.pieces[i]
		switch {
		case pc.unit == -1:
			rw.write(pc.gap, paraLevel, -1)
			continue
		case pc.unit < 0: // a mirror unit without a twin keeps its source
			rw.write(pc.src, tagSource{src: pc.src, ids: pc.ids}, i)
			rw.end(i)
			continue
		}
		owner := i
		target, ok := tr[d.ext.Units[pc.unit].ID]
		if !ok || len(target) == 0 || filter.IsJoined(target) {
			rw.write(pc.src, tagSource{src: pc.src, ids: pc.ids}, owner)
			rw.end(owner)
			continue
		}
		// The units joined to this one are translated with it: their
		// tags count on from its own, and the tags that stood between
		// them follow the translation.
		src, ids := pc.src, pc.ids
		var between content.Content
		for {
			k := i + 1
			var gap content.Content
			for k < len(pa.pieces) && pa.pieces[k].unit == -1 {
				gap = append(gap, pa.pieces[k].gap...)
				k++
			}
			if k >= len(pa.pieces) || pa.pieces[k].unit < 0 || !filter.IsJoined(tr[d.ext.Units[pa.pieces[k].unit].ID]) {
				break
			}
			more, back := filter.JoinTags(src, pa.pieces[k].src)
			joined := map[string]string{}
			for id, pid := range ids {
				joined[id] = pid
			}
			for id, own := range back {
				joined[id] = pa.pieces[k].ids[own]
			}
			src, ids = append(src.Clone(), more...), joined
			for _, r := range gap {
				if r.IsTag() {
					between = between.AppendTag(*r.Tag)
				}
			}
			i = k
		}
		rw.write(target, tagSource{src: src, ids: ids}, owner)
		rw.end(owner)
		rw.write(between, paraLevel, -1)
	}
	rw.write(pa.suffix, paraLevel, -1)
	rw.end(-1)
	for len(rw.wraps) > 0 {
		rw.closeWrap(rw.wraps[len(rw.wraps)-1].key)
	}
	out.Write(pa.close)
}

// tagSource finds the source tag (with its markup) for a tag in a
// translation, and the paragraph-level ID its byte ranges are stored under.
type tagSource struct {
	src content.Content
	ids map[string]string // nil: IDs are already paragraph-level
}

func (ts tagSource) find(kind content.TagKind, id string) (content.Tag, string, bool) {
	for _, r := range ts.src {
		if r.IsTag() && r.Tag.ID == id && r.Tag.Kind == kind {
			pid := id
			if ts.ids != nil {
				pid = ts.ids[id]
			}
			return *r.Tag, pid, true
		}
	}
	return content.Tag{}, "", false
}

func isWrapper(typ string) bool {
	return typ == "link" || typ == "control" || typ == "insertion"
}

func (rw *runWriter) write(c content.Content, ts tagSource, owner int) {
	key := func(id string) string { return fmt.Sprintf("%d:%s", owner, id) }
	for _, r := range c {
		if !r.IsTag() {
			rw.text(r.Text)
			continue
		}
		t, pid, ok := ts.find(r.Tag.Kind, r.Tag.ID)
		if !ok {
			continue // a tag the source does not have
		}
		switch t.Kind {
		case content.TagOpen:
			if !isWrapper(t.Type) {
				rw.fmts = append(rw.fmts, fmtSpan{key: key(t.ID), rpr: []byte(t.Native)})
				continue
			}
			start, rpr := splitWrapNative(t.Native)
			end := ""
			if cl, _, ok := ts.find(content.TagClose, t.ID); ok {
				end = cl.Native
			}
			rw.out.WriteString(start)
			rw.wraps = append(rw.wraps, wrapSpan{key: key(t.ID), owner: owner, end: end})
			if rpr != "" {
				rw.fmts = append(rw.fmts, fmtSpan{key: "wrap:" + key(t.ID), rpr: []byte(rpr)})
			}
		case content.TagClose:
			if isWrapper(t.Type) {
				rw.closeWrap(key(t.ID))
			} else {
				rw.dropFmt(key(t.ID))
			}
		case content.TagStandalone:
			if spans, ok := rw.pa.ranges[pid]; ok && len(spans) > 0 {
				for _, s := range spans {
					rw.part.writeRange(rw.out, rw.doc, rw.tr, rw.lang, s[0], s[1])
				}
			} else {
				rw.out.WriteString(t.Native)
			}
		}
	}
}

// end closes the spans a unit opened but did not close.
func (rw *runWriter) end(owner int) {
	for i := len(rw.wraps) - 1; i >= 0; i-- {
		if i < len(rw.wraps) && rw.wraps[i].owner == owner {
			rw.closeWrap(rw.wraps[i].key)
		}
	}
	prefix := fmt.Sprintf("%d:", owner)
	kept := rw.fmts[:0]
	for _, f := range rw.fmts {
		if !strings.HasPrefix(f.key, prefix) {
			kept = append(kept, f)
		}
	}
	rw.fmts = kept
}

func (rw *runWriter) closeWrap(key string) {
	for i := len(rw.wraps) - 1; i >= 0; i-- {
		if rw.wraps[i].key != key {
			continue
		}
		// Wrappers cannot overlap in XML: close inner ones first.
		for j := len(rw.wraps) - 1; j >= i; j-- {
			rw.dropFmt("wrap:" + rw.wraps[j].key)
			rw.out.WriteString(rw.wraps[j].end)
		}
		rw.wraps = rw.wraps[:i]
		return
	}
}

func (rw *runWriter) dropFmt(key string) {
	for i := len(rw.fmts) - 1; i >= 0; i-- {
		if rw.fmts[i].key == key {
			rw.fmts = append(rw.fmts[:i], rw.fmts[i+1:]...)
			return
		}
	}
}

// splitWrapNative separates a wrapper's start markup from the run
// properties of its text that may follow it (links carry their style).
// Wrapper markup proper never contains run properties: a content control's
// start holds only control properties.
func splitWrapNative(native string) (start, rpr string) {
	i := strings.Index(native, ">")
	if i < 0 || i == len(native)-1 {
		return native, ""
	}
	rest := native[i+1:]
	if strings.HasPrefix(rest, "<") {
		name := rest[1:]
		if j := strings.IndexAny(name, " />"); j >= 0 {
			name = name[:j]
		}
		if strings.HasSuffix(name, ":rPr") || name == "rPr" {
			return native[:i+1], rest
		}
	}
	return native, ""
}

func (rw *runWriter) currentRPr() []byte {
	if n := len(rw.fmts); n > 0 {
		return rw.fmts[n-1].rpr
	}
	return rw.pa.base
}

// text writes one run in the paragraph's markup.
func (rw *runWriter) text(s string) {
	if s == "" {
		return
	}
	if rw.pa.dialect == dml {
		rw.dmlText(s)
		return
	}
	w := rw.pa.ns
	var b strings.Builder
	var t strings.Builder
	flush := func() {
		if t.Len() == 0 {
			return
		}
		b.WriteString("<" + w + `:t xml:space="preserve">`)
		b.WriteString(escapeText(t.String()))
		b.WriteString("</" + w + ":t>")
		t.Reset()
	}
	for _, r := range s {
		switch r {
		case '\t':
			flush()
			b.WriteString("<" + w + ":tab/>")
		case '\n':
			flush()
			b.WriteString("<" + w + ":br/>")
		case '‑':
			flush()
			b.WriteString("<" + w + ":noBreakHyphen/>")
		case '­':
			flush()
			b.WriteString("<" + w + ":softHyphen/>")
		default:
			t.WriteRune(r)
		}
	}
	flush()
	rw.out.WriteString("<" + w + ":r>")
	rw.out.WriteString(rw.withLang(rw.currentRPr(), w, false))
	rw.out.WriteString(b.String())
	rw.out.WriteString("</" + w + ":r>")
}

func (rw *runWriter) dmlText(s string) {
	a := rw.pa.ns
	rpr := rw.withLang(rw.currentRPr(), a, true)
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			rw.out.WriteString("<" + a + ":br/>")
		}
		if line == "" {
			continue
		}
		rw.out.WriteString("<" + a + ":r>" + rpr + "<" + a + ":t>" + escapeText(line) + "</" + a + ":t></" + a + ":r>")
	}
}

// withLang returns run properties with the language set to the target.
func (rw *runWriter) withLang(rpr []byte, prefix string, isDML bool) string {
	if rw.lang == "" {
		return string(rpr)
	}
	k := fmt.Sprintf("%t|%s", isDML, rpr)
	if s, ok := rw.cache[k]; ok {
		return s
	}
	var s string
	if isDML {
		s = setLangDML(rpr, prefix, rw.lang)
	} else {
		s = setLang(rpr, prefix, rw.lang, rw.dir)
	}
	rw.cache[k] = s
	return s
}

func langAttr(lang string) string {
	l := strings.ToLower(lang)
	if i := strings.IndexAny(l, "-_"); i >= 0 {
		l = l[:i]
	}
	switch l {
	case "ja", "zh", "ko":
		return "eastAsia"
	case "ar", "he", "fa", "ur", "yi", "ps", "sd", "ug", "dv", "th", "lo", "km", "my", "hi", "bn", "ta", "te", "kn", "ml", "mr", "gu",
		"pa", "ne", "si", "bo":
		// Word keeps the language of right-to-left and complex scripts here.
		return "bidi"
	}
	return "val"
}

// rPr children that must follow w:lang in the schema order.
var afterLang = map[string]bool{"eastAsianLayout": true, "specVanish": true, "oMath": true, "rPrChange": true}

func attrName(a xml.Attr) string {
	if a.Name.Space != "" {
		return a.Name.Space + ":" + a.Name.Local
	}
	return a.Name.Local
}

// setLang sets the language of WordprocessingML run properties in the
// attribute Word uses for the target's script.
func setLang(rpr []byte, w, lang, dir string) string {
	which := langAttr(lang)
	rtl := ""
	if dir == "rtl" {
		rtl = "<" + w + ":rtl/>"
	}
	newLang := func(old *node) string {
		var b strings.Builder
		b.WriteString("<" + w + ":lang")
		seen := false
		if old != nil {
			for _, a := range old.Attrs {
				v := a.Value
				if a.Name.Local == which {
					v, seen = lang, true
				}
				b.WriteString(" " + attrName(a) + `="` + escapeAttr(v) + `"`)
			}
		}
		if !seen {
			b.WriteString(" " + w + ":" + which + `="` + escapeAttr(lang) + `"`)
		}
		b.WriteString("/>")
		return b.String()
	}
	if len(rpr) == 0 {
		return "<" + w + ":rPr>" + rtl + newLang(nil) + "</" + w + ":rPr>"
	}
	n, err := parseFragment(rpr)
	if err != nil {
		return string(rpr)
	}
	var inner strings.Builder
	placed := false
	// w:rtl stands before w:cs, w:em and w:lang in the schema's order.
	rtlPlaced := rtl == ""
	placeRTL := func() {
		if !rtlPlaced {
			inner.WriteString(rtl)
			rtlPlaced = true
		}
	}
	for _, k := range n.Kids {
		if !k.IsElement() {
			continue
		}
		if k.Local == "rtl" && dir != "" {
			// Set anew for the translation's direction, or dropped.
			placeRTL()
			continue
		}
		if k.Local == "cs" || k.Local == "em" {
			placeRTL()
		}
		if k.Local == "lang" {
			placeRTL()
			inner.WriteString(newLang(k))
			placed = true
			continue
		}
		if !placed && afterLang[k.Local] {
			placeRTL()
			inner.WriteString(newLang(nil))
			placed = true
		}
		inner.Write(k.Raw)
	}
	if !placed {
		placeRTL()
		inner.WriteString(newLang(nil))
	}
	open := string(n.Start)
	if len(n.End) == 0 { // <w:rPr/>
		open = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(open, ">")), "/") + ">"
	}
	return open + inner.String() + "</" + n.Prefix + ":" + n.Local + ">"
}

// setLangDML sets the lang attribute of DrawingML run properties.
func setLangDML(rpr []byte, a, lang string) string {
	if len(rpr) == 0 {
		return "<" + a + `:rPr lang="` + escapeAttr(lang) + `"/>`
	}
	n, err := parseFragment(rpr)
	if err != nil {
		return string(rpr)
	}
	var b strings.Builder
	b.WriteString("<" + n.Prefix + ":" + n.Local)
	seen := false
	for _, at := range n.Attrs {
		v := at.Value
		if at.Name.Space == "" && at.Name.Local == "lang" {
			v, seen = lang, true
		}
		b.WriteString(" " + attrName(at) + `="` + escapeAttr(v) + `"`)
	}
	if !seen {
		b.WriteString(` lang="` + escapeAttr(lang) + `"`)
	}
	if len(n.End) == 0 {
		b.WriteString("/>")
		return b.String()
	}
	b.WriteString(">")
	for _, k := range n.Kids {
		b.Write(k.Raw)
	}
	b.WriteString("</" + n.Prefix + ":" + n.Local + ">")
	return b.String()
}

// translated reports whether any of the paragraph's units has a translation.
func (pa *para) translated(d *document, tr filter.Translations) bool {
	for _, pc := range pa.pieces {
		if pc.unit >= 0 {
			if t, ok := tr[d.ext.Units[pc.unit].ID]; ok && len(t) > 0 {
				return true
			}
		}
	}
	return false
}

// pPr children that follow w:bidi in the schema's order.
var afterBidi = map[string]bool{
	"adjustRightInd": true, "snapToGrid": true, "spacing": true, "ind": true, "contextualSpacing": true, "mirrorIndents": true,
	"suppressOverlap": true, "jc": true, "textDirection": true, "textAlignment": true, "textboxTightWrap": true, "outlineLvl": true,
	"divId": true, "cnfStyle": true, "rPr": true, "sectPr": true, "pPrChange": true,
}

// setParaDirection sets or removes w:bidi in a paragraph's properties, so
// the paragraph is laid out from the right for a right-to-left language
// and from the left otherwise. Its alignment needs no change: Word reads
// "left" as the side a paragraph starts on.
func setParaDirection(ppr []byte, w string, rtl bool) string {
	bidi := "<" + w + ":bidi/>"
	if len(ppr) == 0 {
		if !rtl {
			return ""
		}
		return "<" + w + ":pPr>" + bidi + "</" + w + ":pPr>"
	}
	n, err := parseFragment(ppr)
	if err != nil {
		return string(ppr)
	}
	has := false
	for _, k := range n.Kids {
		if k.IsElement() && k.Local == "bidi" {
			has = true
		}
	}
	if !has && !rtl {
		return string(ppr)
	}
	var inner strings.Builder
	placed := !rtl
	for _, k := range n.Kids {
		if !k.IsElement() {
			continue
		}
		if k.Local == "bidi" {
			if !placed {
				inner.WriteString(bidi)
				placed = true
			}
			continue
		}
		if !placed && afterBidi[k.Local] {
			inner.WriteString(bidi)
			placed = true
		}
		inner.Write(k.Raw)
	}
	if !placed {
		inner.WriteString(bidi)
	}
	open := string(n.Start)
	if len(n.End) == 0 { // <w:pPr/>
		open = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(open, ">")), "/") + ">"
	}
	return open + inner.String() + "</" + n.Prefix + ":" + n.Local + ">"
}

// setParaDirectionDML turns a DrawingML paragraph around when its
// translation reads the other way: rtl is set or cleared, and an alignment
// to one side moves to the other, as PowerPoint does when the direction of
// a paragraph is changed. A paragraph that already reads the right way is
// left as it is.
func setParaDirectionDML(ppr []byte, a string, rtl bool) string {
	if len(ppr) == 0 {
		if !rtl {
			return ""
		}
		return "<" + a + `:pPr algn="r" rtl="1"/>`
	}
	n, err := parseFragment(ppr)
	if err != nil {
		return string(ppr)
	}
	was := n.Get("rtl") == "1" || n.Get("rtl") == "true"
	if was == rtl {
		return string(ppr)
	}
	start := n.Start
	switch algn, has := n.Attr("algn"); {
	case algn == "l", !has && rtl:
		start = xmlraw.SetAttr(start, "algn", "r")
	case algn == "r":
		start = xmlraw.SetAttr(start, "algn", "l")
	}
	if rtl {
		start = xmlraw.SetAttr(start, "rtl", "1")
	} else {
		start = xmlraw.SetAttr(start, "rtl", "0")
	}
	return string(start) + string(ppr[len(n.Start):])
}
