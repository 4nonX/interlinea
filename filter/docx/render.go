package docx

import (
	"bytes"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

// render rebuilds the part with translated paragraphs. Bytes outside the
// rewritten paragraphs are copied unchanged.
func (p *part) render(d *document, tr filter.Translations, targetLang string) []byte {
	var out bytes.Buffer
	last := 0
	for _, pa := range p.paras {
		if !pa.hasUnits() {
			continue
		}
		out.Write(p.data[last:pa.start])
		pa.render(&out, d, tr, p.w, targetLang)
		last = pa.end
	}
	out.Write(p.data[last:])
	return out.Bytes()
}

type fmtSpan struct {
	id    string
	owner int
	rpr   []byte
}

type linkSpan struct {
	id    string
	owner int
	end   string
}

type runWriter struct {
	out   *bytes.Buffer
	w     string
	base  []byte
	lang  string
	fmts  []fmtSpan
	links []linkSpan
	cache map[string]string
}

func (pa *para) render(out *bytes.Buffer, d *document, tr filter.Translations, w, targetLang string) {
	rw := &runWriter{out: out, w: w, base: pa.base, lang: targetLang, cache: map[string]string{}}
	out.Write(pa.open)
	out.Write(pa.ppr)
	identity := func(t content.Tag) (content.Tag, bool) { return t, true }
	rw.write(pa.prefix, identity, -1)
	for i, pc := range pa.pieces {
		if pc.unit < 0 {
			rw.write(pc.gap, identity, -1)
			continue
		}
		u := d.ext.Units[pc.unit]
		target, ok := tr[u.ID]
		if !ok || len(target) == 0 {
			target = u.Source
		}
		rw.write(target, func(t content.Tag) (content.Tag, bool) { return unitTag(u.Source, t) }, i)
		rw.end(i)
	}
	rw.write(pa.suffix, identity, -1)
	rw.end(-1)
	for len(rw.links) > 0 {
		rw.closeLink(rw.links[len(rw.links)-1].id)
	}
	out.Write(pa.close)
}

// write renders content. resolve maps a tag to the source tag that holds
// its native code; tags it cannot resolve are dropped.
func (rw *runWriter) write(c content.Content, resolve func(content.Tag) (content.Tag, bool), owner int) {
	for _, r := range c {
		if !r.IsTag() {
			rw.text(r.Text)
			continue
		}
		t, ok := resolve(*r.Tag)
		if !ok {
			continue
		}
		switch t.Kind {
		case content.TagOpen:
			if t.Type == "link" {
				// Native is the hyperlink start tag, optionally followed by
				// the run properties of the link text.
				start, rpr := splitLinkNative(t.Native)
				rw.out.WriteString(start)
				rw.links = append(rw.links, linkSpan{id: t.ID, owner: owner, end: closingTag(start)})
				if rpr != "" {
					rw.fmts = append(rw.fmts, fmtSpan{id: "link:" + t.ID, owner: owner, rpr: []byte(rpr)})
				}
			} else {
				rw.fmts = append(rw.fmts, fmtSpan{id: t.ID, owner: owner, rpr: []byte(t.Native)})
			}
		case content.TagClose:
			if t.Type == "link" {
				rw.closeLink(t.ID)
			} else {
				rw.dropFmt(t.ID)
			}
		case content.TagStandalone:
			rw.out.WriteString(t.Native)
		}
	}
}

// end closes the spans a unit opened but did not close.
func (rw *runWriter) end(owner int) {
	for i := len(rw.links) - 1; i >= 0; i-- {
		if rw.links[i].owner == owner {
			rw.closeLink(rw.links[i].id)
		}
	}
	kept := rw.fmts[:0]
	for _, f := range rw.fmts {
		if f.owner != owner {
			kept = append(kept, f)
		}
	}
	rw.fmts = kept
}

func (rw *runWriter) closeLink(id string) {
	for i := len(rw.links) - 1; i >= 0; i-- {
		if rw.links[i].id != id {
			continue
		}
		for j := len(rw.links) - 1; j >= i; j-- {
			rw.dropFmt("link:" + rw.links[j].id)
		}
		// Links cannot overlap in XML: close inner ones first.
		for j := len(rw.links) - 1; j >= i; j-- {
			rw.out.WriteString(rw.links[j].end)
		}
		rw.links = rw.links[:i]
		return
	}
}

func (rw *runWriter) dropFmt(id string) {
	for i := len(rw.fmts) - 1; i >= 0; i-- {
		if rw.fmts[i].id == id {
			rw.fmts = append(rw.fmts[:i], rw.fmts[i+1:]...)
			return
		}
	}
}

// splitLinkNative separates the hyperlink start tag from the run
// properties that may follow it.
func splitLinkNative(native string) (start, rpr string) {
	if i := strings.Index(native, ">"); i >= 0 {
		return native[:i+1], native[i+1:]
	}
	return native, ""
}

// closingTag derives "</w:hyperlink>" from a raw start tag.
func closingTag(start string) string {
	s := strings.TrimPrefix(start, "<")
	if i := strings.IndexAny(s, " \t\r\n/>"); i >= 0 {
		s = s[:i]
	}
	return "</" + s + ">"
}

func (rw *runWriter) currentRPr() []byte {
	if n := len(rw.fmts); n > 0 {
		return rw.fmts[n-1].rpr
	}
	return rw.base
}

// text writes one run. Tabs, line breaks and special hyphens become their
// Word elements.
func (rw *runWriter) text(s string) {
	if s == "" {
		return
	}
	w := rw.w
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
	rw.out.WriteString(rw.withLang(rw.currentRPr()))
	rw.out.WriteString(b.String())
	rw.out.WriteString("</" + w + ":r>")
}

// withLang returns run properties with the language set to the target, in
// the attribute Word uses for that script.
func (rw *runWriter) withLang(rpr []byte) string {
	if rw.lang == "" {
		return string(rpr)
	}
	if s, ok := rw.cache[string(rpr)]; ok {
		return s
	}
	s := setLang(rpr, rw.w, rw.lang)
	rw.cache[string(rpr)] = s
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
	case "ar", "he", "fa", "ur", "yi", "th", "hi", "bn", "ta", "te":
		return "bidi"
	}
	return "val"
}

// rPr children that must follow w:lang in the schema order.
var afterLang = map[string]bool{"eastAsianLayout": true, "specVanish": true, "oMath": true, "rPrChange": true}

func setLang(rpr []byte, w, lang string) string {
	which := langAttr(lang)
	newLang := func(old *node) string {
		var b strings.Builder
		b.WriteString("<" + w + ":lang")
		seen := false
		if old != nil {
			for _, a := range old.attrs {
				name := a.Name.Local
				if a.Name.Space != "" {
					name = a.Name.Space + ":" + name
				}
				v := a.Value
				if a.Name.Local == which {
					v, seen = lang, true
				}
				b.WriteString(" " + name + `="` + escapeAttr(v) + `"`)
			}
		}
		if !seen {
			b.WriteString(" " + w + ":" + which + `="` + escapeAttr(lang) + `"`)
		}
		b.WriteString("/>")
		return b.String()
	}
	if len(rpr) == 0 {
		return "<" + w + ":rPr>" + newLang(nil) + "</" + w + ":rPr>"
	}
	n, err := parseFragment(rpr)
	if err != nil {
		return string(rpr)
	}
	var inner strings.Builder
	placed := false
	for _, k := range n.kids {
		if k.isText() {
			continue
		}
		if k.local == "lang" {
			inner.WriteString(newLang(k))
			placed = true
			continue
		}
		if !placed && afterLang[k.local] {
			inner.WriteString(newLang(nil))
			placed = true
		}
		inner.Write(k.raw)
	}
	if !placed {
		inner.WriteString(newLang(nil))
	}
	open := string(n.start)
	if len(n.end) == 0 { // <w:rPr/>
		open = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(open, ">")), "/") + ">"
	}
	return open + inner.String() + "</" + n.prefix + ":" + n.local + ">"
}
