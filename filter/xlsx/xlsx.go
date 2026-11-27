// Package xlsx reads and writes Excel workbooks (.xlsx).
//
// Extract returns the text of the cells, sheet by sheet in the order of
// the workbook and cell by cell along the rows, then each sheet's
// comments and the text boxes, shapes and chart titles on it. A text
// that several cells share is read once, where it first appears.
// Formatting that changes inside a cell becomes paired tags and a line
// break a standalone tag. Merge writes the translations back and copies
// every other byte of the package unchanged; a sheet translated into a
// language of the other direction is turned around.
//
// Numbers, dates and the results of formulas are not text and are left
// alone. Sheet names are kept: formulas, charts and defined names refer
// to sheets by name.
package xlsx

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"path"
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

// ErrNotWorkbook is returned for files that are not Excel workbooks.
var ErrNotWorkbook = errors.New("xlsx: not an Excel workbook")

// Extract returns the translatable units of a workbook. lang is the source
// language (a BCP 47 tag); it selects the sentence rules.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	w, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	w.ext.Format = "xlsx"
	w.ext.SourceLang = lang
	return &w.ext, nil
}

// Merge returns a copy of the workbook with the units replaced by their
// translations. lang must be the language used for Extract; targetLang is
// written to the text of text boxes and charts.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	w, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	changed := map[string][]byte{}
	for _, p := range w.parts {
		var edits []xmlraw.Edit
		for _, s := range p.items {
			if !s.para.Translated(&w.ext, tr) {
				continue // untouched: every byte stays, phonetic guides included
			}
			edits = append(edits, xmlraw.Edit{Span: s.node.Whole(), With: s.render(s.para.Build(&w.ext, tr))})
		}
		if p.sheet && langtag.RTL(lang) != langtag.RTL(targetLang) {
			edits = append(edits, turn(p, langtag.RTL(targetLang))...)
		}
		if len(edits) == 0 {
			continue
		}
		out, err := xmlraw.Apply(p.data, edits)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %s: %w", p.file, err)
		}
		changed[p.file] = out
	}
	for name, b := range w.drawings.Render(tr, targetLang) {
		changed[name] = b
	}
	return docx.RewritePackage(w.zr, changed)
}

type workbook struct {
	zr       *zip.Reader
	seg      *segment.Segmenter
	ext      filter.Extraction
	parts    []*part
	drawings *docx.Parts
}

// part is a file of the package with text items: the shared strings, a
// sheet with its inline strings, or a sheet's comments.
type part struct {
	file  string
	data  []byte
	sheet bool
	views []*xmlraw.Node // a sheet's sheetView elements
	items []*item
}

// item is a text of the workbook: a shared string (si), a cell's own text
// (is) or the text of a comment.
type item struct {
	node *xmlraw.Node
	base string // the run properties of most of the text
	para para.Para
}

// noProps stands for a run without properties among runs that have some.
const noProps = "-"

type sheetRef struct {
	name, file string
	index      int
}

func parse(data []byte, lang string) (*workbook, error) {
	zr, err := docx.OpenPackage(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	w := &workbook{zr: zr, seg: segment.New(nil, lang)}
	rels, err := docx.Rels(zr, "")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	main := ""
	for _, r := range rels {
		if strings.HasSuffix(r.Type, "/officeDocument") {
			main = r.Target
		}
	}
	if main == "" {
		return nil, ErrNotWorkbook
	}
	mainData, err := docx.ReadPart(zr, main)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	root, err := xmlraw.Parse(mainData)
	if err != nil || root.Local != "workbook" {
		return nil, ErrNotWorkbook
	}
	wrels, err := docx.Rels(zr, main)
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	target := map[string]string{}
	sstFile := ""
	for _, r := range wrels {
		switch {
		case strings.HasSuffix(r.Type, "/worksheet"):
			target[r.ID] = r.Target
		case strings.HasSuffix(r.Type, "/sharedStrings"):
			sstFile = r.Target
		}
	}
	var sheets []sheetRef
	if list := root.Child("sheets"); list != nil {
		for _, s := range list.Children("sheet") {
			id := ""
			for _, a := range s.Attrs {
				if a.Name.Space != "" && a.Name.Space != "xmlns" && a.Name.Local == "id" {
					id = a.Value
				}
			}
			if f := target[id]; f != "" { // chart sheets have no cells
				sheets = append(sheets, sheetRef{name: s.Get("name"), file: f, index: len(sheets)})
			}
		}
	}

	// The shared strings, each cut into units when a cell first shows it.
	var shared []*item
	sst := &part{file: sstFile}
	if sstFile != "" {
		b, err := docx.ReadPart(zr, sstFile)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %w", err)
		}
		sroot, err := xmlraw.Parse(b)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %s: %w", sstFile, err)
		}
		sst.data = b
		for _, si := range sroot.Children("si") {
			shared = append(shared, &item{node: si})
		}
	}
	used := make([]bool, len(shared))
	w.parts = append(w.parts, sst)

	var drawn []docx.PartRef
	type later struct {
		at    int // units before this sheet's drawings
		parts []docx.PartRef
	}
	var drawingsAt []later
	for _, sh := range sheets {
		b, err := docx.ReadPart(zr, sh.file)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %w", err)
		}
		p := &part{file: sh.file, data: b, sheet: true}
		toks, err := xmlraw.Tokenize(b)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %s: %w", sh.file, err)
		}
		short := strings.TrimSuffix(path.Base(sh.file), path.Ext(sh.file))
		for i := 0; i < len(toks); i++ {
			se, ok := startOf(toks[i])
			if !ok {
				continue
			}
			switch se.local {
			case "sheetView":
				n, _, err := xmlraw.Build(b, toks, i)
				if err != nil {
					return nil, fmt.Errorf("xlsx: %s: %w", sh.file, err)
				}
				p.views = append(p.views, n)
			case "c":
				ref, typ := se.attr("r"), se.attr("t")
				if typ != "s" && typ != "inlineStr" {
					continue
				}
				n, next, err := xmlraw.Build(b, toks, i)
				if err != nil {
					return nil, fmt.Errorf("xlsx: %s: %w", sh.file, err)
				}
				i = next - 1
				block := filter.Block{ID: short + "/" + ref, Part: filter.PartBody, File: sh.file}
				if row, col, ok := cellRef(ref); ok {
					block.Cell = &filter.Cell{Table: sh.index, Row: row, Col: col}
				}
				if typ == "inlineStr" {
					if is := n.Child("is"); is != nil {
						it := &item{node: is}
						if w.add(it, block) {
							p.items = append(p.items, it)
						}
					}
					continue
				}
				v := n.Child("v")
				if v == nil {
					continue
				}
				idx, err := strconv.Atoi(strings.TrimSpace(v.InnerText()))
				if err != nil || idx < 0 || idx >= len(shared) || used[idx] {
					continue
				}
				used[idx] = true
				if w.add(shared[idx], block) {
					sst.items = append(sst.items, shared[idx])
				}
			}
		}
		w.parts = append(w.parts, p)

		srels, _ := docx.Rels(zr, sh.file)
		var mine []docx.PartRef
		for _, r := range srels {
			switch {
			case strings.HasSuffix(r.Type, "/comments"):
				cp, err := w.comments(r.Target)
				if err != nil {
					w.ext.Warnings = append(w.ext.Warnings, fmt.Sprintf("%s could not be read and is kept unchanged: %v", r.Target, err))
					continue
				}
				w.parts = append(w.parts, cp)
			case strings.HasSuffix(r.Type, "/drawing"):
				mine = append(mine, docx.PartRef{Name: r.Target, Kind: filter.PartBody})
				drels, _ := docx.Rels(zr, r.Target)
				for _, dr := range drels {
					if strings.HasSuffix(dr.Type, "/chart") {
						mine = append(mine, docx.PartRef{Name: dr.Target, Kind: filter.PartChart})
					}
				}
			}
		}
		if len(mine) > 0 {
			drawingsAt = append(drawingsAt, later{at: len(w.ext.Units), parts: mine})
			drawn = append(drawn, mine...)
		}
	}

	// Text boxes, shapes and chart titles are DrawingML, read as in Word
	// and PowerPoint. Their units go after the cells and comments of the
	// sheet they are on.
	w.drawings = docx.ExtractParts(zr, lang, drawn)
	dex := w.drawings.Extraction()
	w.ext.Warnings = append(w.ext.Warnings, dex.Warnings...)
	if len(dex.Units) > 0 {
		fileOf := map[string]string{}
		for _, b := range dex.Blocks {
			fileOf[b.ID] = b.File
		}
		var units []filter.Unit
		var blocks []filter.Block
		pos := 0
		for _, l := range drawingsAt {
			units = append(units, w.ext.Units[pos:l.at]...)
			pos = l.at
			names := map[string]bool{}
			for _, p := range l.parts {
				names[p.Name] = true
			}
			for _, u := range dex.Units {
				if names[fileOf[u.Block]] {
					units = append(units, u)
				}
			}
		}
		units = append(units, w.ext.Units[pos:]...)
		// Units moved: every text item finds its own again by ID.
		at := map[string]int{}
		for i, u := range units {
			at[u.ID] = i
		}
		for _, p := range w.parts {
			for _, it := range p.items {
				if it.para.Units > 0 {
					it.para.First = at[w.ext.Units[it.para.First].ID]
				}
			}
		}
		// Blocks follow their units' order.
		byID := map[string]filter.Block{}
		for _, b := range w.ext.Blocks {
			byID[b.ID] = b
		}
		for _, b := range dex.Blocks {
			byID[b.ID] = b
		}
		seen := map[string]bool{}
		for _, u := range units {
			if !seen[u.Block] {
				seen[u.Block] = true
				blocks = append(blocks, byID[u.Block])
			}
		}
		w.ext.Units, w.ext.Blocks = units, blocks
	}
	return w, nil
}

// add cuts an item into units under the block. It reports false for an
// item without text.
func (w *workbook) add(it *item, block filter.Block) bool {
	c, base := itemContent(it.node)
	p, ok := para.Split(w.seg, c)
	if !ok {
		return false
	}
	it.base = base
	it.para = p
	it.para.Add(&w.ext, block)
	return true
}

// comments reads the comments of a sheet.
func (w *workbook) comments(file string) (*part, error) {
	b, err := docx.ReadPart(w.zr, file)
	if err != nil {
		return nil, err
	}
	root, err := xmlraw.Parse(b)
	if err != nil {
		return nil, err
	}
	p := &part{file: file, data: b}
	short := strings.TrimSuffix(path.Base(file), path.Ext(file))
	if list := root.Child("commentList"); list != nil {
		for _, c := range list.Children("comment") {
			text := c.Child("text")
			if text == nil {
				continue
			}
			ref := c.Get("ref")
			it := &item{node: text}
			if w.add(it, filter.Block{ID: short + "/" + ref, Part: filter.PartComment, File: file, Note: ref}) {
				p.items = append(p.items, it)
			}
		}
	}
	return p, nil
}

type start struct {
	local string
	attrs map[string]string
}

func (s start) attr(name string) string { return s.attrs[name] }

func startOf(t xmlraw.Token) (start, bool) {
	se, ok := t.Tok.(xml.StartElement)
	if !ok {
		return start{}, false
	}
	s := start{local: se.Name.Local, attrs: map[string]string{}}
	for _, a := range se.Attr {
		if a.Name.Space == "" {
			s.attrs[a.Name.Local] = a.Value
		}
	}
	return s, true
}

// cellRef turns "B12" into row 11, column 1.
func cellRef(ref string) (row, col int, ok bool) {
	i := 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		col = col*26 + int(ref[i]-'A'+1)
		i++
	}
	if i == 0 || i == len(ref) {
		return 0, 0, false
	}
	n, err := strconv.Atoi(ref[i:])
	if err != nil || n < 1 {
		return 0, 0, false
	}
	return n - 1, col - 1, true
}

// itemContent is the text of an item as content: runs whose properties
// differ from those of most of the text are wrapped in a pair of tags, and
// a line break is a tag of its own.
func itemContent(n *xmlraw.Node) (content.Content, string) {
	type run struct{ props, text string }
	var runs []run
	rich := false
	for _, k := range n.Kids {
		if !k.IsElement() {
			continue
		}
		switch k.Local {
		case "t":
			runs = append(runs, run{noProps, k.InnerText()})
		case "r":
			rich = true
			props := noProps
			if p := k.Child("rPr"); p != nil {
				props = string(p.Raw)
			}
			text := ""
			if t := k.Child("t"); t != nil {
				text = t.InnerText()
			}
			runs = append(runs, run{props, text})
		}
	}
	base := noProps
	if rich {
		weight := map[string]int{}
		best := -1
		for _, r := range runs {
			weight[r.props] += len(r.text)
		}
		for _, r := range runs {
			if weight[r.props] > best {
				best, base = weight[r.props], r.props
			}
		}
	}
	c := content.Content{}
	id := 0
	next := func() string { id++; return strconv.Itoa(id) }
	text := func(s string) {
		for i, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
			if i > 0 {
				c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: next(), Type: "lb", Desc: i18n.N("Line break"), Native: "\n"})
			}
			c = c.Append(line)
		}
	}
	for i := 0; i < len(runs); i++ {
		r := runs[i]
		if r.props == base {
			text(r.text)
			continue
		}
		typ, desc := describe(base, r.props)
		tid := next()
		c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: tid, Type: typ, Desc: desc, Native: r.props})
		text(r.text)
		for i+1 < len(runs) && runs[i+1].props == r.props {
			i++
			text(runs[i].text)
		}
		c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: tid, Type: typ, Desc: desc})
	}
	return c, base
}

// render writes the item with the given content.
func (it *item) render(c content.Content) []byte {
	q := func(local string) string {
		if it.node.Prefix != "" {
			return it.node.Prefix + ":" + local
		}
		return local
	}
	type run struct{ props, text string }
	var runs []run
	var open []string
	emit := func(s string) {
		props := it.base
		if len(open) > 0 {
			props = open[len(open)-1]
		}
		if n := len(runs); n > 0 && runs[n-1].props == props {
			runs[n-1].text += s
			return
		}
		runs = append(runs, run{props, s})
	}
	for _, r := range c {
		if !r.IsTag() {
			emit(r.Text)
			continue
		}
		switch r.Tag.Kind {
		case content.TagStandalone:
			emit("\n")
		case content.TagOpen:
			open = append(open, r.Tag.Native)
		case content.TagClose:
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	t := func(s string) string {
		space := ""
		if s != strings.TrimSpace(s) || strings.Contains(s, "\n") {
			space = ` xml:space="preserve"`
		}
		return "<" + q("t") + space + ">" + xmlraw.EscapeText(s) + "</" + q("t") + ">"
	}
	var b strings.Builder
	start := string(it.node.Start)
	if it.node.SelfClosing() {
		start = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(start, ">")), "/") + ">"
	}
	b.WriteString(start)
	plain := true
	for _, r := range runs {
		if r.props != noProps {
			plain = false
		}
	}
	if plain {
		all := ""
		for _, r := range runs {
			all += r.text
		}
		b.WriteString(t(all))
	} else {
		for _, r := range runs {
			if r.text == "" {
				continue
			}
			b.WriteString("<" + q("r") + ">")
			if r.props != noProps {
				b.WriteString(r.props)
			}
			b.WriteString(t(r.text) + "</" + q("r") + ">")
		}
	}
	b.WriteString("</" + it.node.Name() + ">")
	return []byte(b.String())
}

// turn sets the direction of a sheet's views: columns run from the right
// for a right-to-left language.
func turn(p *part, rtl bool) []xmlraw.Edit {
	var edits []xmlraw.Edit
	for _, v := range p.views {
		was := v.Get("rightToLeft") == "1" || v.Get("rightToLeft") == "true"
		if was == rtl {
			continue
		}
		val := "0"
		if rtl {
			val = "1"
		}
		edits = append(edits, xmlraw.Edit{Span: v.StartTag(), With: xmlraw.SetAttr(v.Start, "rightToLeft", val)})
	}
	return edits
}

// describe names how a run's properties differ from those of most of the
// text: a tag type and a description such as "Bold, Colour #C00000".
func describe(base, props string) (typ, desc string) {
	a, b := propSet(base), propSet(props)
	type change struct{ typ, desc string }
	var changes []change
	flag := func(key, typ, on, off string) {
		switch {
		case b[key] != "" && a[key] == "":
			changes = append(changes, change{typ, on})
		case b[key] == "" && a[key] != "":
			changes = append(changes, change{"format", off})
		}
	}
	flag("b", "bold", i18n.N("Bold"), i18n.N("Not bold"))
	flag("i", "italic", i18n.N("Italic"), i18n.N("Not italic"))
	flag("u", "underline", i18n.N("Underline"), i18n.N("Not underlined"))
	flag("strike", "strike", i18n.N("Strikethrough"), i18n.N("Not struck through"))
	if a["vertAlign"] != b["vertAlign"] {
		switch b["vertAlign"] {
		case "superscript":
			changes = append(changes, change{"superscript", i18n.N("Superscript")})
		case "subscript":
			changes = append(changes, change{"subscript", i18n.N("Subscript")})
		default:
			changes = append(changes, change{"format", i18n.N("Normal position")})
		}
	}
	if a["color"] != b["color"] {
		if v := b["color"]; v == "" {
			changes = append(changes, change{"color", i18n.N("Automatic colour")})
		} else {
			changes = append(changes, change{"color", i18n.Fill(i18n.N("Colour {value}"), "value", v)})
		}
	}
	if a["rFont"] != b["rFont"] && b["rFont"] != "" {
		changes = append(changes, change{"font", i18n.Fill(i18n.N("Font {value}"), "value", b["rFont"])})
	}
	if a["sz"] != b["sz"] && b["sz"] != "" {
		changes = append(changes, change{"size", i18n.Fill(i18n.N("Size {n} pt"), "n", b["sz"])})
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

// propSet reads run properties into the values describe compares.
func propSet(props string) map[string]string {
	out := map[string]string{}
	if props == noProps || props == "" {
		return out
	}
	n, err := xmlraw.ParseFragment([]byte(props))
	if err != nil {
		return out
	}
	for _, k := range n.Elements() {
		val, has := k.Attr("val")
		switch k.Local {
		case "b", "i", "strike":
			if !has || (val != "0" && val != "false") {
				out[k.Local] = "1"
			}
		case "u":
			if val != "none" {
				out["u"] = "1"
			}
		case "color":
			switch {
			case k.Get("rgb") != "":
				rgb := strings.ToUpper(k.Get("rgb"))
				if len(rgb) == 8 {
					rgb = rgb[2:] // the first two digits are the opacity
				}
				out["color"] = "#" + rgb
			case k.Get("theme") != "":
				out["color"] = "theme " + k.Get("theme")
			case k.Get("indexed") != "":
				out["color"] = "indexed " + k.Get("indexed")
			}
		case "rFont", "sz", "vertAlign":
			out[k.Local] = val
		}
	}
	return out
}
