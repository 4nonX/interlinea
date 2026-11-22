package tmx

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
)

// Writer writes a TMX 1.4b document.
type Writer struct {
	w      *bufio.Writer
	closed bool
	err    error
}

// NewWriter writes the XML declaration, the root element and the header.
// Empty header fields get TMX-conformant defaults; SrcLang is required.
func NewWriter(w io.Writer, h Header) (*Writer, error) {
	if h.SrcLang == "" {
		return nil, errors.New("tmx: header srclang is required")
	}
	def := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	tw := &Writer{w: bufio.NewWriter(w)}
	tw.str(xml.Header)
	tw.str(`<tmx version="1.4">` + "\n")
	tw.str("  <header")
	tw.attr("creationtool", def(h.CreationTool, "Interlinea"))
	tw.attr("creationtoolversion", def(h.CreationToolVersion, "0"))
	tw.attr("segtype", def(h.SegType, "sentence"))
	tw.attr("o-tmf", def(h.OTMF, "Interlinea"))
	tw.attr("adminlang", def(h.AdminLang, "en-US"))
	tw.attr("srclang", h.SrcLang)
	tw.attr("datatype", def(h.DataType, "plaintext"))
	tw.optAttr("creationdate", formatDate(h.CreationDate))
	tw.optAttr("creationid", h.CreationID)
	if len(h.Props) == 0 && len(h.Notes) == 0 {
		tw.str("/>\n")
	} else {
		tw.str(">\n")
		tw.notesProps("    ", h.Notes, h.Props)
		tw.str("  </header>\n")
	}
	tw.str("  <body>\n")
	return tw, tw.err
}

func (t *Writer) str(s string) {
	if t.err == nil {
		_, t.err = t.w.WriteString(s)
	}
}

func (t *Writer) escaped(s string) {
	if t.err == nil {
		t.err = xml.EscapeText(t.w, []byte(s))
	}
}

func (t *Writer) attr(name, value string) {
	t.str(" " + name + `="`)
	t.escaped(value)
	t.str(`"`)
}

func (t *Writer) optAttr(name, value string) {
	if value != "" {
		t.attr(name, value)
	}
}

func (t *Writer) notesProps(indent string, notes []string, props []Prop) {
	for _, n := range notes {
		t.str(indent + "<note>")
		t.escaped(n)
		t.str("</note>\n")
	}
	for _, p := range props {
		t.str(indent + "<prop")
		t.attr("type", p.Type)
		t.str(">")
		t.escaped(p.Value)
		t.str("</prop>\n")
	}
}

// Write writes one unit. Units need at least one variant.
func (t *Writer) Write(u *Unit) error {
	if t.closed {
		return errors.New("tmx: write after close")
	}
	if len(u.Variants) == 0 {
		return fmt.Errorf("tmx: unit %q has no variants", u.ID)
	}
	t.str("    <tu")
	t.optAttr("tuid", u.ID)
	t.optAttr("srclang", u.SrcLang)
	t.optAttr("creationdate", formatDate(u.CreationDate))
	t.optAttr("creationid", u.CreationID)
	t.optAttr("changedate", formatDate(u.ChangeDate))
	t.optAttr("changeid", u.ChangeID)
	if u.UsageCount > 0 {
		t.attr("usagecount", strconv.Itoa(u.UsageCount))
	}
	t.optAttr("lastusagedate", formatDate(u.LastUsage))
	t.str(">\n")
	t.notesProps("      ", u.Notes, u.Props)
	for i := range u.Variants {
		v := &u.Variants[i]
		if v.Lang == "" {
			return fmt.Errorf("tmx: unit %q has a variant without language", u.ID)
		}
		t.str("      <tuv")
		t.attr("xml:lang", v.Lang)
		t.optAttr("creationdate", formatDate(v.CreationDate))
		t.optAttr("creationid", v.CreationID)
		t.optAttr("changedate", formatDate(v.ChangeDate))
		t.optAttr("changeid", v.ChangeID)
		t.str(">\n")
		t.notesProps("        ", v.Notes, v.Props)
		t.str("        <seg>")
		t.seg(v.Content)
		t.str("</seg>\n")
		t.str("      </tuv>\n")
	}
	t.str("    </tu>\n")
	return t.err
}

// seg writes inline content. Tag pairs inside the segment become bpt/ept,
// unpaired open or close tags become it, standalone tags become ph. TMX
// requires numeric i and x values, so IDs are renumbered per segment.
func (t *Writer) seg(c content.Content) {
	opens := map[string]bool{}
	closes := map[string]bool{}
	for _, r := range c {
		if !r.IsTag() {
			continue
		}
		switch r.Tag.Kind {
		case content.TagOpen:
			opens[r.Tag.ID] = true
		case content.TagClose:
			closes[r.Tag.ID] = true
		}
	}
	pairNum := map[string]int{}
	nextI, nextX := 0, 0
	for _, r := range c {
		if !r.IsTag() {
			t.escaped(r.Text)
			continue
		}
		tg := r.Tag
		typ := tg.Type
		switch {
		case tg.Kind == content.TagOpen && closes[tg.ID]:
			nextI++
			pairNum[tg.ID] = nextI
			t.element("bpt", "i", nextI, typ, "", tg.Native)
		case tg.Kind == content.TagClose && opens[tg.ID]:
			n, ok := pairNum[tg.ID]
			if !ok {
				// Close before open in the same segment: treat as isolated.
				nextX++
				t.element("it", "x", nextX, typ, "end", tg.Native)
				continue
			}
			t.element("ept", "i", n, "", "", tg.Native)
		case tg.Kind == content.TagOpen:
			nextX++
			t.element("it", "x", nextX, typ, "begin", tg.Native)
		case tg.Kind == content.TagClose:
			nextX++
			t.element("it", "x", nextX, typ, "end", tg.Native)
		default:
			nextX++
			t.element("ph", "x", nextX, typ, "", tg.Native)
		}
	}
}

func (t *Writer) element(name, numAttr string, num int, typ, pos, native string) {
	t.str("<" + name)
	if pos != "" {
		t.attr("pos", pos)
	}
	t.attr(numAttr, strconv.Itoa(num))
	if typ != "" {
		t.attr("type", typ)
	}
	t.str(">")
	t.escaped(native)
	t.str("</" + name + ">")
}

// Close finishes the document and flushes it. It does not close the
// underlying writer.
func (t *Writer) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	t.str("  </body>\n</tmx>\n")
	if t.err == nil {
		t.err = t.w.Flush()
	}
	return t.err
}

// SegString renders content the way Write does, which helps in tests and logs.
func SegString(c content.Content) (string, error) {
	var b strings.Builder
	tw := &Writer{w: bufio.NewWriter(&b)}
	tw.seg(c)
	if tw.err == nil {
		tw.err = tw.w.Flush()
	}
	return b.String(), tw.err
}
