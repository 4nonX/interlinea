package xliff

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/internal/xmlraw"
	"github.com/4nonX/interlinea/langtag"
)

// Trados Studio exports a file for bilingual review in Word (Batch tasks,
// Export for bilingual review) as a Word document with one table: Segment
// ID, Segment status, Source segment, Target segment. The first cell
// holds the segment number and its translation unit's ID in the character
// styles SegmentID and TransUnitID; the status is a word and the match
// ("Draft (77%)"). Tags are text in the character style Tag, <1> </1>
// <2/>, and locked content is in the style LockedContent. Word comments
// in a row are the segment's comments. Studio updates the file from the
// review by the segment IDs and the hashes it keeps in the document's
// custom XML part, so only target cells (and the status shown) change.

// FormatSDLReview is a Trados Studio bilingual review document.
const FormatSDLReview = "sdlreview"

const reviewPart = "word/document.xml"

var reviewTagRE = regexp.MustCompile(`<\s*(/?)\s*(\d+)\s*(/?)\s*>`)

// reviewTag is a code of a source cell.
type reviewTag struct {
	tag content.Tag
	// text is how the cell writes it; locked is true for locked content.
	text   string
	locked bool
}

type reviewSegment struct {
	unit         filter.Unit
	target       *xmlraw.Node // the target cell
	status       *xmlraw.Node // the status cell
	statusSuffix string
	lang         string // the target paragraph's language as written
	tags         []reviewTag
	// mq holds the tags of a memoQ table's segment.
	mq *rtfSegment
}

type reviewFile struct {
	zr   *zip.Reader
	data []byte // the main part
	w    string // the WordprocessingML prefix as written
	// The styles' IDs; mqStyle is memoQ's style of tags.
	tagStyle, lockedStyle, segStyle, tuStyle, mqStyle string
	segments                                          []*reviewSegment
	byID                                              map[string]*reviewSegment
	ext                                               filter.Extraction
}

// docxStyles maps the names of a document's styles to their IDs.
func docxStyles(zr *zip.Reader) map[string]string {
	out := map[string]string{}
	b, err := docx.ReadPart(zr, "word/styles.xml")
	if err != nil {
		return out
	}
	root, err := xmlraw.Parse(b)
	if err != nil {
		return out
	}
	for _, s := range root.Children("style") {
		id, _ := s.AttrAny("styleId")
		if n := s.Child("name"); n != nil {
			if v, ok := n.AttrAny("val"); ok {
				out[v] = id
			}
		}
		if id != "" {
			if _, ok := out[id]; !ok {
				out[id] = id
			}
		}
	}
	return out
}

func styleID(styles map[string]string, name string) string {
	if id, ok := styles[name]; ok && id != "" {
		return id
	}
	return name
}

// runStyle is a run's character style ID.
func runStyle(r *xmlraw.Node) string {
	if p := r.Child("rPr"); p != nil {
		if s := p.Child("rStyle"); s != nil {
			v, _ := s.AttrAny("val")
			return v
		}
	}
	return ""
}

// paraLang is the language a paragraph's mark has (pPr/rPr/lang).
func paraLang(p *xmlraw.Node) string {
	if pp := p.Child("pPr"); pp != nil {
		if rp := pp.Child("rPr"); rp != nil {
			if l := rp.Child("lang"); l != nil {
				v, _ := l.AttrAny("val")
				return v
			}
		}
	}
	return ""
}

// docxRun is text of a cell in one character style.
type docxRun struct {
	text  string
	style string
	lang  string
}

// cellRuns reads a table cell's text by character style. Text deleted
// with tracked changes is left out, inserted text is in; paragraphs are
// separated by line breaks.
func cellRuns(tc *xmlraw.Node) []docxRun {
	var out []docxRun
	add := func(text, style, lang string) {
		if text == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].style == style {
			out[n-1].text += text
			return
		}
		out = append(out, docxRun{text: text, style: style, lang: lang})
	}
	var walk func(n *xmlraw.Node)
	walk = func(n *xmlraw.Node) {
		for _, k := range n.Kids {
			if !k.IsElement() {
				continue
			}
			switch k.Local {
			case "del", "moveFrom", "rPr", "pPr", "tcPr", "commentReference", "instrText", "delText":
				continue
			case "r":
				style := runStyle(k)
				lang := ""
				if rp := k.Child("rPr"); rp != nil {
					if l := rp.Child("lang"); l != nil {
						lang, _ = l.AttrAny("val")
					}
				}
				for _, x := range k.Kids {
					switch x.Local {
					case "t":
						add(x.InnerText(), style, lang)
					case "tab", "ptab":
						add("\t", style, lang)
					case "br", "cr":
						add("\n", style, lang)
					case "noBreakHyphen":
						add("\u2011", style, lang)
					case "softHyphen":
						add("\u00ad", style, lang)
					case "sym":
						if c, ok := x.AttrAny("char"); ok {
							if v, err := strconv.ParseUint(c, 16, 32); err == nil {
								r := rune(v)
								if r >= 0xf000 {
									r -= 0xf000
								}
								add(string(r), style, lang)
							}
						}
					}
				}
			case "p":
				if len(out) > 0 {
					add("\n", "", "")
				}
				walk(k)
			default:
				walk(k)
			}
		}
	}
	walk(tc)
	return out
}

// cellText is a cell's text, all styles alike.
func cellText(tc *xmlraw.Node) string {
	var b strings.Builder
	for _, r := range cellRuns(tc) {
		b.WriteString(r.text)
	}
	return b.String()
}

// tableCells returns a row's cells, in cells kept in content controls as well.
func tableCells(tr *xmlraw.Node) []*xmlraw.Node {
	var out []*xmlraw.Node
	for _, k := range tr.Elements() {
		switch k.Local {
		case "tc":
			out = append(out, k)
		case "sdt", "customXml":
			inner := k
			if c := k.Child("sdtContent"); c != nil {
				inner = c
			}
			for _, x := range inner.Elements() {
				if x.Local == "tc" {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

// reviewTable finds the table of segments: the one whose rows start with
// a segment number in the SegmentID style.
func reviewTable(root *xmlraw.Node, segStyle, tuStyle string) *xmlraw.Node {
	body := root.Child("body")
	if body == nil {
		return nil
	}
	for _, t := range body.Children("tbl") {
		for _, tr := range t.Children("tr") {
			cells := tableCells(tr)
			if len(cells) < 4 {
				continue
			}
			seg, tu := false, false
			for _, r := range cellRuns(cells[0]) {
				seg = seg || r.style == segStyle
				tu = tu || r.style == tuStyle
			}
			if seg && tu {
				return t
			}
		}
	}
	return nil
}

// IsReviewDOCX reports whether data is a bilingual table in a Word
// document: a Trados Studio review document or a memoQ two-column DOCX.
func IsReviewDOCX(data []byte) bool { return docxTableFormat(data) != "" }

// docxTableFormat tells which bilingual table a Word document is, "" for
// none.
func docxTableFormat(data []byte) string {
	if len(data) < 4 || !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return ""
	}
	zr, err := docx.OpenPackage(data)
	if err != nil {
		return ""
	}
	b, err := docx.ReadPart(zr, reviewPart)
	if err != nil || !bytes.Contains(b, []byte("tbl")) {
		return ""
	}
	styles := docxStyles(zr)
	seg, tu := styleID(styles, "SegmentID"), styleID(styles, "TransUnitID")
	sdl := bytes.Contains(b, []byte(`"`+seg+`"`)) && bytes.Contains(b, []byte(`"`+tu+`"`))
	_, mqStyled := styles["mqInternal"]
	mq := mqStyled || bytes.Contains(b, []byte("MQ"))
	if !sdl && !mq {
		return ""
	}
	root, err := xmlraw.Parse(b)
	if err != nil {
		return ""
	}
	if sdl && reviewTable(root, seg, tu) != nil {
		return FormatSDLReview
	}
	if t, _ := mqDOCXTable(root, mqStyled); t != nil {
		return FormatMQDOCX
	}
	return ""
}

// reviewState reads a status cell ("Translated (100%)").
func reviewState(status string, hasText bool) (filter.State, int, string) {
	s := strings.TrimSpace(status)
	match := 0
	suffix := ""
	if m := mqPercent.FindStringSubmatch(s); m != nil {
		match, _ = strconv.Atoi(m[1])
		suffix = strings.TrimSpace(m[0])
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	}
	var st filter.State
	switch strings.ToLower(s) {
	case "draft", "entwurf":
		st = filter.StateDraft
	case "translated", "\u00fcbersetzt":
		st = filter.StateTranslated
	case "translation approved", "\u00fcbersetzung genehmigt":
		st = filter.StateReviewed
	case "signed off", "abgezeichnet":
		st = filter.StateFinal
	case "translation rejected", "sign-off rejected", "\u00fcbersetzung abgelehnt", "abzeichnung abgelehnt":
		st = filter.StateRejected
	default:
		st = filter.StateNew
	}
	if st == filter.StateNew && hasText {
		st = filter.StateDraft
	}
	return st, match, suffix
}

// reviewStatus is Studio's word for a state.
func reviewStatus(s filter.State) string {
	switch s {
	case filter.StateDraft:
		return "Draft"
	case filter.StateTranslated:
		return "Translated"
	case filter.StateReviewed:
		return "Translation Approved"
	case filter.StateFinal:
		return "Signed Off"
	case filter.StateRejected:
		return "Translation Rejected"
	}
	return "Not Translated"
}

// reviewComments reads the comments part: each comment's text by its ID.
func reviewComments(zr *zip.Reader) map[string]string {
	out := map[string]string{}
	b, err := docx.ReadPart(zr, "word/comments.xml")
	if err != nil {
		return out
	}
	root, err := xmlraw.Parse(b)
	if err != nil {
		return out
	}
	for _, c := range root.Children("comment") {
		id, _ := c.AttrAny("id")
		var parts []string
		for _, p := range c.Children("p") {
			if t := strings.TrimSpace(cellText(&xmlraw.Node{Kids: []*xmlraw.Node{p}})); t != "" {
				parts = append(parts, t)
			}
		}
		text := strings.Join(parts, "\n")
		if author, ok := c.AttrAny("author"); ok && author != "" && text != "" {
			text = author + ": " + text
		}
		if text != "" {
			out[id] = text
		}
	}
	return out
}

func readReviewDOCX(data []byte) (*reviewFile, error) {
	zr, err := docx.OpenPackage(data)
	if err != nil {
		return nil, fmt.Errorf("xliff: the review document is not a Word file: %w", err)
	}
	main, err := docx.ReadPart(zr, reviewPart)
	if err != nil {
		return nil, fmt.Errorf("xliff: the review document has no text: %w", err)
	}
	root, err := xmlraw.Parse(main)
	if err != nil {
		return nil, fmt.Errorf("xliff: the review document's text cannot be read: %w", err)
	}
	styles := docxStyles(zr)
	f := &reviewFile{zr: zr, data: main, w: root.Prefix, byID: map[string]*reviewSegment{},
		tagStyle: styleID(styles, "Tag"), lockedStyle: styleID(styles, "LockedContent"),
		segStyle: styleID(styles, "SegmentID"), tuStyle: styleID(styles, "TransUnitID")}
	f.ext.Format = FormatSDLReview
	table := reviewTable(root, f.segStyle, f.tuStyle)
	if table == nil {
		_, mqStyled := styles["mqInternal"]
		if t, header := mqDOCXTable(root, mqStyled); t != nil {
			f.mqStyle = styleID(styles, "mqInternal")
			return f.readMQ(t, header)
		}
		return nil, errNotReview
	}
	comments := reviewComments(zr)
	f.ext.Blocks = append(f.ext.Blocks, filter.Block{ID: "b1", Part: filter.PartBody})
	var srcLang, tgtLang string
	for _, tr := range table.Children("tr") {
		cells := tableCells(tr)
		if len(cells) < 4 {
			continue
		}
		segID, tuID := "", ""
		for _, r := range cellRuns(cells[0]) {
			switch r.style {
			case f.segStyle:
				segID += r.text
			case f.tuStyle:
				tuID += r.text
			}
		}
		segID, tuID = strings.TrimSpace(segID), strings.TrimSpace(tuID)
		if segID == "" {
			continue
		}
		sg := &reviewSegment{target: cells[3], status: cells[1]}
		for _, p := range cells[3].Children("p") {
			if sg.lang = paraLang(p); sg.lang != "" {
				break
			}
		}
		if srcLang == "" {
			for _, p := range cells[2].Children("p") {
				if srcLang = paraLang(p); srcLang != "" {
					break
				}
			}
			if srcLang == "" {
				for _, r := range cellRuns(cells[2]) {
					if r.lang != "" {
						srcLang = r.lang
						break
					}
				}
			}
		}
		if tgtLang == "" {
			tgtLang = sg.lang
		}
		src := f.content(cellRuns(cells[2]), sg, true)
		tgt := f.content(cellRuns(cells[3]), sg, false)
		id := segID
		if tuID != "" {
			id = tuID + "/" + segID
		}
		sg.unit = filter.Unit{ID: id, Block: "b1", Key: segID, Source: src}
		hasText := strings.TrimSpace(tgt.Plain()) != "" || tgt.HasTags()
		if hasText {
			sg.unit.Target = tgt
		}
		var match int
		sg.unit.State, match, sg.statusSuffix = reviewState(cellText(cells[1]), hasText)
		if hasText {
			sg.unit.Match = match
		}
		var notes []string
		tr.Walk(func(n *xmlraw.Node) bool {
			if n.Local == "commentRangeStart" {
				cid, _ := n.AttrAny("id")
				if t, ok := comments[cid]; ok {
					notes = append(notes, t)
				}
			}
			return true
		})
		sg.unit.Notes = notes
		if _, dup := f.byID[id]; dup {
			return nil, fmt.Errorf("xliff: two rows of the review document are segment %s", segID)
		}
		f.byID[id] = sg
		f.segments = append(f.segments, sg)
		f.ext.Units = append(f.ext.Units, sg.unit)
	}
	if len(f.segments) == 0 {
		return nil, fmt.Errorf("xliff: the review document has no segments")
	}
	f.ext.SourceLang, f.ext.TargetLang = ttxLang(srcLang), ttxLang(tgtLang)
	// A language Trados does not know is written as a private tag
	// ("zh-x-hmn-SDL"), which names no language Interlinea can take.
	for _, l := range []*string{&f.ext.SourceLang, &f.ext.TargetLang} {
		if _, err := langtag.Normalize(*l); err != nil || strings.Contains(strings.ToLower(*l), "-x-") {
			*l = ""
		}
	}
	if f.ext.SourceLang == "" || f.ext.TargetLang == "" {
		f.ext.Warnings = append(f.ext.Warnings, noLanguagesNote)
	}
	return f, nil
}

// content reads a cell's runs. In the source each tag and each piece of
// locked content becomes a tag of the segment; in the target, tags are
// those of the source, whether typed in the Tag style or not.
func (f *reviewFile) content(runs []docxRun, sg *reviewSegment, source bool) content.Content {
	out := content.Content{}
	locked := 0
	for _, r := range runs {
		switch {
		case r.style == f.lockedStyle:
			if source {
				locked++
				t := content.Tag{Kind: content.TagStandalone, ID: "locked" + strconv.Itoa(locked), Type: "locked", Desc: truncate(r.text, 80), Native: r.text}
				sg.tags = append(sg.tags, reviewTag{tag: t, text: r.text, locked: true})
				out = out.AppendTag(t)
				continue
			}
			if t, ok := sg.lockedTag(r.text); ok {
				out = out.AppendTag(t.tag)
				continue
			}
			out = out.Append(r.text)
		case r.style == f.tagStyle || !source:
			text := r.text
			for text != "" {
				loc := reviewTagRE.FindStringSubmatchIndex(text)
				if loc == nil {
					out = out.Append(text)
					break
				}
				out = out.Append(text[:loc[0]])
				m := text[loc[0]:loc[1]]
				closing := loc[3] > loc[2]
				num := text[loc[4]:loc[5]]
				alone := loc[7] > loc[6]
				text = text[loc[1]:]
				kind := content.TagOpen
				switch {
				case closing:
					kind = content.TagClose
				case alone:
					kind = content.TagStandalone
				}
				if source {
					t := content.Tag{Kind: kind, ID: num, Desc: m, Native: m}
					sg.tags = append(sg.tags, reviewTag{tag: t, text: m})
					out = out.AppendTag(t)
					continue
				}
				if t, ok := sg.tagByContent(content.Tag{Kind: kind, ID: num}); ok {
					out = out.AppendTag(t.tag)
				} else {
					out = out.Append(m)
				}
			}
		default:
			out = out.Append(r.text)
		}
	}
	return out.Normalize()
}

func (s *reviewSegment) lockedTag(text string) (reviewTag, bool) {
	for _, t := range s.tags {
		if t.locked && t.text == text {
			return t, true
		}
	}
	return reviewTag{}, false
}

func (s *reviewSegment) tagByContent(t content.Tag) (reviewTag, bool) {
	for _, x := range s.tags {
		if x.tag.Kind == t.Kind && x.tag.ID == t.ID {
			return x, true
		}
	}
	return reviewTag{}, false
}

// el writes an element name with the document's prefix.
func (f *reviewFile) el(local string) string {
	if f.w == "" {
		return local
	}
	return f.w + ":" + local
}

// run writes a run of text or of a tag.
func (f *reviewFile) run(text, style, lang string) string {
	var b strings.Builder
	b.WriteString("<" + f.el("r") + ">")
	if style != "" || lang != "" {
		b.WriteString("<" + f.el("rPr") + ">")
		if style != "" {
			b.WriteString("<" + f.el("rStyle") + " " + f.el("val") + `="` + xmlraw.EscapeAttr(style) + `"/>`)
		}
		if lang != "" {
			b.WriteString("<" + f.el("lang") + " " + f.el("val") + `="` + xmlraw.EscapeAttr(lang) + `"/>`)
		}
		b.WriteString("</" + f.el("rPr") + ">")
	}
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteString("<" + f.el("br") + "/>")
		}
		for j, part := range strings.Split(line, "\t") {
			if j > 0 {
				b.WriteString("<" + f.el("tab") + "/>")
			}
			if part != "" {
				b.WriteString("<" + f.el("t") + ` xml:space="preserve">` + xmlraw.EscapeText(part) + "</" + f.el("t") + ">")
			}
		}
	}
	b.WriteString("</" + f.el("r") + ">")
	return b.String()
}

// cellEdits replaces a cell's paragraphs with one paragraph of runs. The
// first paragraph's start tag and properties stay.
func (f *reviewFile) cellEdits(tc *xmlraw.Node, runs string) []xmlraw.Edit {
	paras := tc.Children("p")
	if len(paras) == 0 {
		end := tc.EndTag()
		return []xmlraw.Edit{{Span: xmlraw.Span{end[0], end[0]}, With: []byte("<" + f.el("p") + ">" + runs + "</" + f.el("p") + ">")}}
	}
	p := paras[0]
	start := string(p.Start)
	if p.SelfClosing() {
		start = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(start, "/>")), "/") + ">"
	}
	props := ""
	if pp := p.Child("pPr"); pp != nil {
		props = string(pp.Raw)
	}
	edits := []xmlraw.Edit{{Span: p.Whole(), With: []byte(start + props + runs + "</" + f.el("p") + ">")}}
	for _, q := range paras[1:] {
		edits = append(edits, xmlraw.Edit{Span: q.Whole()})
	}
	return edits
}

// render writes a translation as runs: text in the target language, tags
// in the Tag style, locked content in its own.
func (f *reviewFile) render(sg *reviewSegment, c content.Content) (string, error) {
	var b strings.Builder
	for _, r := range c {
		if r.Tag == nil {
			b.WriteString(f.run(r.Text, "", sg.lang))
			continue
		}
		if sg.mq != nil {
			t, ok := sg.mq.tagByContent(*r.Tag)
			if !ok {
				return "", fmt.Errorf("%w: the source has no %s tag %q", ErrTags, r.Tag.Kind, r.Tag.ID)
			}
			b.WriteString(f.run(t.text, f.mqStyle, sg.lang))
			continue
		}
		t, ok := sg.tagByContent(*r.Tag)
		if !ok {
			return "", fmt.Errorf("%w: the source has no %s tag %q", ErrTags, r.Tag.Kind, r.Tag.ID)
		}
		style := f.tagStyle
		if t.locked {
			style = f.lockedStyle
		}
		b.WriteString(f.run(t.text, style, sg.lang))
	}
	return b.String(), nil
}

func (f *reviewFile) merge(data []byte, updates map[string]Update) ([]byte, error) {
	var edits []xmlraw.Edit
	for id, u := range updates {
		sg, ok := f.byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoSegment, id)
		}
		if u.State != "" && !u.State.Valid() {
			return nil, fmt.Errorf("%w %q for segment %q", ErrBadState, u.State, id)
		}
		if len(u.Joined) > 0 {
			return nil, fmt.Errorf("segment %q: the segments of a review document cannot be joined", id)
		}
		changed := u.Target != nil && !sameContent(u.Target, sg.unit.Target)
		st := u.State
		if st == "" {
			st = sg.unit.State
		}
		restate := st != sg.unit.State
		if !changed && !restate {
			continue
		}
		if sg.unit.Locked {
			return nil, fmt.Errorf("%w: %q", ErrLocked, id)
		}
		if changed {
			runs, err := f.render(sg, u.Target)
			if err != nil {
				return nil, fmt.Errorf("segment %q: %w", id, err)
			}
			edits = append(edits, f.cellEdits(sg.target, runs)...)
		}
		if sg.status == nil {
			continue
		}
		text := reviewStatus(st)
		if f.ext.Format == FormatMQDOCX {
			text = mqStatus(st)
		}
		// The match shown stays while the translation is the one it was
		// for.
		if sg.statusSuffix != "" && !changed {
			text += " " + sg.statusSuffix
		}
		props := ""
		for _, p := range sg.status.Children("p") {
			if r := p.Child("r"); r != nil {
				if rp := r.Child("rPr"); rp != nil {
					props = string(rp.Raw)
				}
				break
			}
		}
		run := "<" + f.el("r") + ">" + props + "<" + f.el("t") + ` xml:space="preserve">` + xmlraw.EscapeText(text) + "</" + f.el("t") + "></" + f.el("r") + ">"
		edits = append(edits, f.cellEdits(sg.status, run)...)
	}
	if len(edits) == 0 {
		return data, nil
	}
	out, err := xmlraw.Apply(f.data, edits)
	if err != nil {
		return nil, err
	}
	if _, err := xmlraw.Parse(out); err != nil {
		return nil, fmt.Errorf("xliff: the merged review document is not well-formed: %w", err)
	}
	return docx.RewritePackage(f.zr, map[string][]byte{reviewPart: out})
}
