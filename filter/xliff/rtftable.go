package xliff

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/rtf"
	"github.com/4nonX/interlinea/internal/xmlraw"
	"github.com/4nonX/interlinea/langtag"
)

// Bilingual tables in RTF: memoQ's two-column RTF and Déjà Vu's RTF
// external view keep each segment in a table row, with the source in one
// cell and the translation in the next. They come back from Word as Word
// saved them, so they are read as RTF of any writer, and only the cells
// that change are written again.

// rtfTag is a code of a source cell: how it is written, and the
// formatting its group starts with, so the translation writes it alike.
type rtfTag struct {
	tag    content.Tag
	text   string
	prefix string
}

// rtfSegment is one row of a bilingual RTF table.
type rtfSegment struct {
	unit         filter.Unit
	row          int
	target       int // cell index of the translation
	status       int // cell index of the status, -1 for none
	statusSuffix string
	tags         []rtfTag
}

// rtfTable is what memoQ and Déjà Vu tables share.
type rtfTable struct {
	doc      *rtf.Doc
	segments []*rtfSegment
	byID     map[string]*rtfSegment
	ext      filter.Extraction
	// tagStyle is the number of the character style tags have (-1 for
	// none); defaultPrefix starts a tag's group when no source tag shows
	// how.
	tagStyle      int
	defaultPrefix string
	// status writes a state as the table's status column says it.
	status func(filter.State) string
}

// tagOf finds the n-th source tag written as text (from 0).
func (s *rtfSegment) tagOf(text string, n int) (rtfTag, bool) {
	for _, t := range s.tags {
		if t.text == text {
			if n == 0 {
				return t, true
			}
			n--
		}
	}
	return rtfTag{}, false
}

// tagByContent finds a source tag by kind and ID.
func (s *rtfSegment) tagByContent(t content.Tag) (rtfTag, bool) {
	for _, x := range s.tags {
		if x.tag.Kind == t.Kind && x.tag.ID == t.ID {
			return x, true
		}
	}
	return rtfTag{}, false
}

// rtfLang is the language of a header cell's name or, failing that, of
// the cell's \lang.
func rtfLang(name string, lcid int) string {
	if t, ok := langtag.ByName(strings.TrimSpace(name)); ok {
		return t
	}
	if t, ok := langtag.FromLCID(lcid); ok {
		return t
	}
	return ""
}

// cellLang is the language most of a column's text has.
func cellLang(doc *rtf.Doc, rows []int, col int) int {
	n := map[int]int{}
	for _, r := range rows {
		cells := doc.Rows[r].Cells
		if col >= len(cells) {
			continue
		}
		c := cells[col]
		for _, run := range c.Runs {
			if strings.TrimSpace(run.Text) != "" && run.Lang != 0 && run.Lang != 1024 {
				n[run.Lang] += len(run.Text)
			}
		}
		if c.Lang != 0 && c.Lang != 1024 {
			n[c.Lang]++
		}
	}
	best, most := 0, 0
	for l, k := range n {
		if k > most || k == most && l < best {
			best, most = l, k
		}
	}
	return best
}

// merge writes translations and states into the table's cells.
func (f *rtfTable) merge(updates map[string]Update) ([]byte, error) {
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
			return nil, fmt.Errorf("segment %q: the segments of a bilingual table cannot be joined", id)
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
		cells := f.doc.Rows[sg.row].Cells
		if changed {
			inner, err := f.render(sg, u.Target)
			if err != nil {
				return nil, fmt.Errorf("segment %q: %w", id, err)
			}
			c := cells[sg.target]
			edits = append(edits, xmlraw.Edit{Span: xmlraw.Span{c.Start, c.End}, With: append(f.doc.ClearCell(c), inner...)})
		}
		if sg.status >= 0 && f.status != nil && (restate || changed) {
			text := f.status(st)
			if !changed && sg.statusSuffix != "" {
				text += " " + sg.statusSuffix
			}
			c := cells[sg.status]
			edits = append(edits, xmlraw.Edit{Span: xmlraw.Span{c.Start, c.End}, With: append(f.doc.ClearCell(c), []byte(`{\uc1 `+rtf.Escape(text)+`}`)...)})
		}
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].Span[0] < edits[j].Span[0] })
	out, err := xmlraw.Apply(f.doc.Data, edits)
	if err != nil {
		return nil, err
	}
	// The table must still read as it did, with as many rows and cells.
	back, err := rtf.Read(out)
	if err != nil || len(back.Rows) != len(f.doc.Rows) {
		return nil, fmt.Errorf("xliff: the merged RTF file does not keep the table's rows")
	}
	for i := range back.Rows {
		if len(back.Rows[i].Cells) != len(f.doc.Rows[i].Cells) {
			return nil, fmt.Errorf("xliff: the merged RTF file does not keep the cells of row %d", i+1)
		}
	}
	return out, nil
}

// render writes a translation as the text of a cell: in a group of its
// own, each tag in a group with its source tag's formatting.
func (f *rtfTable) render(sg *rtfSegment, c content.Content) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`{\uc1 `)
	for _, r := range c {
		if r.Tag == nil {
			b.WriteString(rtf.Escape(r.Text))
			continue
		}
		t, ok := sg.tagByContent(*r.Tag)
		if !ok {
			return nil, fmt.Errorf("%w: the source has no %s tag %q", ErrTags, r.Tag.Kind, r.Tag.ID)
		}
		prefix := t.prefix
		if prefix == "" {
			prefix = f.defaultPrefix
		}
		if f.tagStyle < 0 && prefix == "" {
			b.WriteString(rtf.Escape(t.text))
			continue
		}
		b.WriteString("{")
		b.WriteString(prefix)
		if prefix != "" && !strings.HasSuffix(prefix, " ") {
			b.WriteString(" ")
		}
		b.WriteString(rtf.Escape(t.text))
		b.WriteString("}")
	}
	b.WriteString("}")
	return []byte(b.String()), nil
}

// tagPattern finds tags written as text: memoQ's [1] [2} {3] and Déjà
// Vu's {00108}.
type tagPattern struct {
	re *regexp.Regexp
	// kind tells the kind of a match, and its number.
	kind func(m string) (content.TagKind, string)
}

// cellContent reads a cell's runs: text, and the tags the pattern finds in
// runs of the tag style (any run when tagStyle is -1). With source set,
// only tags of the source are read in a translation, and a tag typed
// without the style counts as well, as Word users write them.
func cellContent(c rtf.Cell, tagStyle int, pat tagPattern, source *rtfSegment) (content.Content, []rtfTag) {
	out := content.Content{}
	var tags []rtfTag
	var open []int // indexes in tags of open tags without their close
	used := map[string]int{}
	for _, run := range c.Runs {
		styled := tagStyle < 0 || run.Style == tagStyle
		if !styled && source == nil {
			out = out.Append(run.Text)
			continue
		}
		text := run.Text
		for text != "" {
			loc := pat.re.FindStringIndex(text)
			if loc == nil {
				out = out.Append(text)
				break
			}
			out = out.Append(text[:loc[0]])
			m := text[loc[0]:loc[1]]
			text = text[loc[1]:]
			if source != nil {
				// The same code can be written alike more than once
				// (memoQ's [4] [4]): the n-th in the translation is the
				// n-th in the source.
				if t, ok := source.tagOf(m, used[m]); ok {
					used[m]++
					out = out.AppendTag(t.tag)
				} else {
					out = out.Append(m)
				}
				continue
			}
			kind, num := pat.kind(m)
			t := content.Tag{Kind: kind, ID: num, Native: m, Desc: m}
			switch kind {
			case content.TagOpen:
				open = append(open, len(tags))
			case content.TagClose:
				if len(open) > 0 {
					o := open[len(open)-1]
					open = open[:len(open)-1]
					t.ID = tags[o].tag.ID
				} else {
					t.Kind = content.TagStandalone
				}
			}
			if dup := findTag(tags, t); dup {
				t.ID = t.ID + "." + strconv.Itoa(len(tags)+1)
			}
			tags = append(tags, rtfTag{tag: t, text: m, prefix: run.Prefix})
			out = out.AppendTag(t)
		}
	}
	// An open tag without its close stands alone.
	for _, o := range open {
		old := tags[o].tag
		tags[o].tag.Kind = content.TagStandalone
		for i := range out {
			if out[i].Tag != nil && out[i].Tag.Kind == old.Kind && out[i].Tag.ID == old.ID {
				t := tags[o].tag
				out[i].Tag = &t
			}
		}
	}
	return out.Normalize(), tags
}

func findTag(tags []rtfTag, t content.Tag) bool {
	for _, x := range tags {
		if x.tag.Kind == t.Kind && x.tag.ID == t.ID {
			return true
		}
	}
	return false
}
