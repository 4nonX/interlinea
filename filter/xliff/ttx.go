package xliff

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/xmlraw"
	"github.com/4nonX/interlinea/langtag"
	segmenter "github.com/4nonX/interlinea/segment"
)

// Trados TagEditor (Trados 2007 and earlier) writes its bilingual files
// (.ttx) as XML: <TRADOStag> with the languages in
// FrontMatter/UserSettings (SourceLanguage, TargetLanguage) and the
// document in Body/Raw. A segment is a <Tu> with a <Tuv> for the source
// and, once translated, one for the target; MatchPercent is the match it
// came from. Text not yet in a <Tu> has not been segmented; TagEditor
// segments it when it is translated, and so does this filter, wrapping
// each translated sentence in a <Tu> of its own.
//
// The document's codes are <ut> elements: Type="start" and "end" open and
// close, others stand alone, and Style="external" marks structure between
// paragraphs (never inside a segment). <df> elements are formatting shown
// in TagEditor and may hold several paragraphs.

// FormatTTX is a Trados TagEditor bilingual file.
const FormatTTX = "ttx"

type ttxSegment struct {
	unit filter.Unit
	// A segment of the file: its <Tu>, source and target <Tuv>.
	tu, src, tgt *xmlraw.Node
	// A sentence not yet in a <Tu>: the bytes it covers, and its source as
	// it is written there.
	span   xmlraw.Span
	srcRaw string
	tags   tagTable
}

type ttxFile struct {
	doc      *xmlraw.Document
	settings *xmlraw.Node
	segments []*ttxSegment
	byID     map[string]*ttxSegment
	ext      filter.Extraction
	srcTag   string // the languages as the file writes them ("EN-US")
	tgtTag   string
	seg      *segmenter.Segmenter
}

func ttxLang(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if n, err := langtag.Normalize(s); err == nil {
		return n
	}
	return s
}

func readTTX(doc *xmlraw.Document) (*ttxFile, error) {
	f := &ttxFile{doc: doc, byID: map[string]*ttxSegment{}}
	f.ext.Format = FormatTTX
	var body *xmlraw.Node
	for _, k := range doc.Root.Elements() {
		switch k.Local {
		case "FrontMatter":
			f.settings = k.Child("UserSettings")
		case "Body":
			body = k.Child("Raw")
		}
	}
	if f.settings != nil {
		f.srcTag = strings.TrimSpace(f.settings.Get("SourceLanguage"))
		f.tgtTag = strings.TrimSpace(f.settings.Get("TargetLanguage"))
	}
	f.ext.SourceLang, f.ext.TargetLang = ttxLang(f.srcTag), ttxLang(f.tgtTag)
	if f.ext.SourceLang == "" {
		return nil, errNoLanguage
	}
	if body == nil {
		return nil, fmt.Errorf("xliff: the .ttx file has no body")
	}
	f.seg = segmenter.New(nil, f.ext.SourceLang)
	f.ext.Blocks = append(f.ext.Blocks, filter.Block{ID: "b1", Part: filter.PartBody, File: f.settings.Get("SourceDocumentPath")})
	if err := f.walk(body.Kids); err != nil {
		return nil, err
	}
	unsegmented := 0
	for _, s := range f.segments {
		if s.tu == nil {
			unsegmented++
		}
	}
	if unsegmented > 0 {
		f.ext.Warnings = append(f.ext.Warnings, fmt.Sprintf("%d sentences were not yet segmented in the file; they are segmented here and written back as segments of their own.", unsegmented))
	}
	return f, nil
}

// external reports whether a node is document structure, outside any
// segment: an external <ut>, a <Tu>, or a <df> holding one of these.
func ttxExternal(n *xmlraw.Node) bool {
	if !n.IsElement() {
		return false
	}
	switch n.Local {
	case "ut":
		return n.Get("Style") == "external"
	case "Tu":
		return true
	case "df":
		for _, k := range n.Kids {
			if ttxExternal(k) {
				return true
			}
		}
	}
	return false
}

// walk reads the segments among nodes: each <Tu>, and the sentences of
// each run of text between structure.
func (f *ttxFile) walk(nodes []*xmlraw.Node) error {
	var run []*xmlraw.Node
	flush := func() error {
		err := f.sentences(run)
		run = run[:0]
		return err
	}
	for _, n := range nodes {
		switch {
		case n.Local == "Tu":
			if err := flush(); err != nil {
				return err
			}
			if err := f.tu(n); err != nil {
				return err
			}
		case ttxExternal(n):
			if err := flush(); err != nil {
				return err
			}
			if n.Local == "df" {
				if err := f.walk(n.Kids); err != nil {
					return err
				}
			}
		case n.CharData || n.Local == "ut" || n.Local == "df":
			run = append(run, n)
		default:
			// Comments and other markup end a run.
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func (f *ttxFile) add(s *ttxSegment) error {
	s.unit.ID = "b1/" + strconv.Itoa(len(f.segments)+1)
	s.unit.Block = "b1"
	s.tags.add(s.unit.Source)
	s.tags.add(s.unit.Target)
	f.byID[s.unit.ID] = s
	f.segments = append(f.segments, s)
	f.ext.Units = append(f.ext.Units, s.unit)
	return nil
}

func (f *ttxFile) tu(n *xmlraw.Node) error {
	var tuvs []*xmlraw.Node
	for _, k := range n.Elements() {
		if k.Local == "Tuv" {
			tuvs = append(tuvs, k)
		}
	}
	if len(tuvs) == 0 {
		return nil
	}
	s := &ttxSegment{tu: n, tags: tagTable{}}
	for _, v := range tuvs {
		lang := v.Get("Lang")
		switch {
		case s.src == nil && strings.EqualFold(lang, f.srcTag):
			s.src = v
		case s.tgt == nil && !strings.EqualFold(lang, f.srcTag):
			s.tgt = v
			if f.tgtTag == "" {
				f.tgtTag = strings.TrimSpace(lang)
				f.ext.TargetLang = ttxLang(f.tgtTag)
			}
		}
	}
	if s.src == nil {
		s.src = tuvs[0]
		if s.tgt == s.src {
			s.tgt = nil
		}
	}
	s.unit.Source = ttxContent(s.src.Kids)
	if s.tgt != nil {
		s.unit.Target = ttxContent(s.tgt.Kids)
	}
	hasText := s.tgt != nil && strings.TrimSpace(s.unit.Target.Plain()) != ""
	if hasText {
		s.unit.State = filter.StateTranslated
	} else {
		s.unit.State = filter.StateNew
	}
	s.unit.Match, _ = strconv.Atoi(n.Get("MatchPercent"))
	switch o := strings.ToLower(n.Get("Origin")); {
	case o == "mt" || strings.Contains(o, "machine"):
		s.unit.Origin = "mt"
	case o != "" && o != "manual" && s.unit.Match > 0:
		s.unit.Origin = "tm"
	}
	return f.add(s)
}

// ttxContent reads the inside of a <Tuv> or of a run: text, <ut> codes
// (start and end paired, the rest standing alone) and <df> formatting as
// a pair of tags around its content.
func ttxContent(nodes []*xmlraw.Node) content.Content {
	c := content.Content{}
	var opens []string
	n := 0
	var read func(nodes []*xmlraw.Node)
	read = func(nodes []*xmlraw.Node) {
		for _, k := range nodes {
			switch {
			case k.CharData:
				c = c.Append(k.Text)
			case k.Local == "ut":
				n++
				desc := k.Get("DisplayText")
				if desc == "" {
					desc = truncate(k.InnerText(), 80)
				}
				t := content.Tag{Kind: content.TagStandalone, ID: "u" + strconv.Itoa(n), Desc: desc, Native: string(k.Raw)}
				switch k.Get("Type") {
				case "start":
					t.Kind = content.TagOpen
					opens = append(opens, t.ID)
				case "end":
					if len(opens) > 0 {
						t.Kind, t.ID = content.TagClose, opens[len(opens)-1]
						opens = opens[:len(opens)-1]
					}
				}
				c = c.AppendTag(t)
			case k.Local == "df":
				n++
				id := "d" + strconv.Itoa(n)
				typ := ""
				switch {
				case k.Get("Bold") == "on":
					typ = "bold"
				case k.Get("Italic") == "on":
					typ = "italic"
				case k.Get("Underline") == "on":
					typ = "underline"
				}
				c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: id, Type: typ, Native: string(k.Start)})
				read(k.Kids)
				c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: id, Type: typ, Native: string(k.End)})
			case k.IsElement():
				n++
				c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: "e" + strconv.Itoa(n), Native: string(k.Raw)})
			}
		}
	}
	read(nodes)
	return c
}

// sentences segments a run of text not yet in a <Tu>. A sentence ends
// only inside or between the run's own text nodes, so that it can be
// wrapped in a <Tu> without cutting through formatting; space around it
// stays outside.
func (f *ttxFile) sentences(run []*xmlraw.Node) error {
	if len(run) == 0 {
		return nil
	}
	// The plain text of the run, and where each top-level text node is in it.
	type piece struct {
		node       *xmlraw.Node
		start, end int // in plain
	}
	var plain strings.Builder
	pieces := make([]piece, len(run))
	for i, n := range run {
		pieces[i].node = n
		pieces[i].start = plain.Len()
		plain.WriteString(ttxPlain(n))
		pieces[i].end = plain.Len()
	}
	text := plain.String()
	if !hasWord(text) {
		return nil
	}
	// Cut points: the segmenter's breaks that fall in a text node or
	// between nodes.
	cuts := []int{0}
	for _, b := range f.seg.Breaks(text) {
		if b <= 0 || b >= len(text) {
			continue
		}
		for _, p := range pieces {
			if b > p.start && b < p.end && !p.node.CharData {
				b = -1
				break
			}
		}
		if b > 0 {
			cuts = append(cuts, b)
		}
	}
	cuts = append(cuts, len(text))
	for i := 0; i+1 < len(cuts); i++ {
		from, to := cuts[i], cuts[i+1]
		// Space at the edges stays outside the segment.
		for from < to && unicode.IsSpace(rune(text[from])) {
			from++
		}
		for to > from && unicode.IsSpace(rune(text[to-1])) {
			to--
		}
		if !hasWord(text[from:to]) {
			continue
		}
		// The codes that close the run go with its last sentence, as
		// TagEditor puts them, with the space between them.
		last := !hasWord(text[to:])
		if last {
			for _, p := range pieces {
				if p.start == p.end && p.start > to {
					to = p.start
				}
			}
		}
		// The nodes the sentence covers, text nodes cut where it starts
		// and ends.
		var nodes []*xmlraw.Node
		var raw strings.Builder
		start, end := -1, -1
		for _, p := range pieces {
			// A node is the sentence's when its text overlaps it; a code
			// without text, when it stands within it.
			if p.start == p.end {
				if p.start < from || p.start > to || (p.start == to && !last) {
					continue
				}
			} else if p.end <= from || p.start >= to {
				continue
			}
			n := p.node
			if !n.CharData {
				if start < 0 {
					start = n.Off
				}
				end = n.Off + len(n.Raw)
				raw.Write(n.Raw)
				nodes = append(nodes, n)
				continue
			}
			a, b := max(from, p.start)-p.start, min(to, p.end)-p.start
			offs := rawOffsets(n)
			if start < 0 {
				start = n.Off + offs[a]
			}
			end = n.Off + offs[b]
			part := n.Text[a:b]
			raw.WriteString(xmlraw.EscapeText(part))
			nodes = append(nodes, &xmlraw.Node{CharData: true, Text: part})
		}
		s := &ttxSegment{span: xmlraw.Span{start, end}, srcRaw: raw.String(), tags: tagTable{}}
		s.unit.Source = ttxContent(nodes)
		s.unit.State = filter.StateNew
		if err := f.add(s); err != nil {
			return err
		}
	}
	return nil
}

// ttxPlain is the text a node adds to a sentence: a <ut>'s markup is a
// code, not text.
func ttxPlain(n *xmlraw.Node) string {
	switch {
	case n.CharData:
		return n.Text
	case n.Local == "ut":
		return ""
	}
	var b strings.Builder
	for _, k := range n.Kids {
		b.WriteString(ttxPlain(k))
	}
	return b.String()
}

func hasWord(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// rawOffsets maps each byte offset of a text node's decoded text to the
// offset in its raw bytes: entities and character references are one
// unit, CDATA sections are their content.
func rawOffsets(n *xmlraw.Node) []int {
	raw := n.Raw
	out := make([]int, 0, len(n.Text)+1)
	i := 0
	for i < len(raw) {
		switch {
		case strings.HasPrefix(string(raw[i:]), "<![CDATA["):
			i += len("<![CDATA[")
			end := strings.Index(string(raw[i:]), "]]>")
			if end < 0 {
				end = len(raw) - i
			}
			for j := 0; j < end; j++ {
				out = append(out, i+j)
			}
			i += end + len("]]>")
		case raw[i] == '&':
			semi := strings.IndexByte(string(raw[i:]), ';')
			if semi < 0 {
				out = append(out, i)
				i++
				continue
			}
			decoded := decodeEntity(string(raw[i : i+semi+1]))
			for range len(decoded) {
				out = append(out, i)
			}
			i += semi + 1
		case raw[i] == '\r':
			// XML reads CR LF and a lone CR as LF.
			out = append(out, i)
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
			i++
		default:
			out = append(out, i)
			i++
		}
	}
	for len(out) < len(n.Text) {
		out = append(out, len(raw))
	}
	return append(out[:len(n.Text)], len(raw))
}

func decodeEntity(e string) string {
	switch e {
	case "&lt;":
		return "<"
	case "&gt;":
		return ">"
	case "&amp;":
		return "&"
	case "&quot;":
		return "\""
	case "&apos;":
		return "'"
	}
	if strings.HasPrefix(e, "&#") {
		num := strings.TrimSuffix(strings.TrimPrefix(e, "&#"), ";")
		base := 10
		if strings.HasPrefix(num, "x") || strings.HasPrefix(num, "X") {
			num, base = num[1:], 16
		}
		if v, err := strconv.ParseInt(num, base, 32); err == nil {
			return string(rune(v))
		}
	}
	return e
}

func (f *ttxFile) merge(updates map[string]Update, targetLang string) ([]byte, error) {
	w := &writer{starts: map[*xmlraw.Node][]byte{}, replaced: map[*xmlraw.Node]bool{}}
	tgtTag := f.tgtTag
	if tgtTag == "" && targetLang != "" {
		// The file was not made for a language yet: it is now, written as
		// TagEditor writes languages.
		tgtTag = strings.ToUpper(targetLang)
		if f.settings != nil {
			w.setAttr(f.settings, "TargetLanguage", tgtTag)
		}
	}
	for id, u := range updates {
		s, ok := f.byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoSegment, id)
		}
		if u.State != "" && !u.State.Valid() {
			return nil, fmt.Errorf("%w %q for segment %q", ErrBadState, u.State, id)
		}
		if u.Target == nil || sameContent(u.Target, s.unit.Target) || strings.TrimSpace(u.Target.Plain()) == "" && len(u.Target.Tags()) == 0 {
			// TagEditor keeps no state of its own, only the translation.
			continue
		}
		if tgtTag == "" {
			return nil, fmt.Errorf("xliff: the .ttx file names no target language")
		}
		inner, err := render(u.Target, s.tags)
		if err != nil {
			return nil, fmt.Errorf("segment %q: %w", id, err)
		}
		switch {
		case s.tgt != nil:
			w.fill(s.tgt, inner)
		case s.tu != nil:
			at := s.src.Whole()[1]
			w.edits = append(w.edits, xmlraw.Edit{Span: xmlraw.Span{at, at}, With: []byte(`<Tuv Lang="` + xmlraw.EscapeAttr(tgtTag) + `">` + inner + "</Tuv>")})
		default:
			srcTag := f.srcTag
			w.edits = append(w.edits, xmlraw.Edit{Span: s.span, With: []byte(`<Tu Origin="manual" MatchPercent="0"><Tuv Lang="` + xmlraw.EscapeAttr(srcTag) + `">` + s.srcRaw + `</Tuv><Tuv Lang="` + xmlraw.EscapeAttr(tgtTag) + `">` + inner + "</Tuv></Tu>")})
		}
	}
	for n, b := range w.starts {
		if !w.replaced[n] {
			w.edits = append(w.edits, xmlraw.Edit{Span: n.StartTag(), With: b})
		}
	}
	out, err := xmlraw.Apply(f.doc.Data, w.edits)
	if err != nil {
		return nil, err
	}
	if _, err := xmlraw.Parse(out); err != nil {
		return nil, fmt.Errorf("xliff: the merged file is not well-formed: %w", err)
	}
	return f.doc.Encode(out), nil
}
