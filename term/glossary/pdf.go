package glossary

import (
	"fmt"
	"strings"
	"time"

	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/typeset"
)

const (
	bodySize  = 9.5
	smallSize = 7.8
)

var (
	grey   = [3]float64{0.42, 0.42, 0.42}
	accent = [3]float64{0.13, 0.32, 0.62}
)

func spans(rs []Run) []typeset.Span {
	out := make([]typeset.Span, 0, len(rs))
	for _, r := range rs {
		sp := typeset.Span{Text: r.Text, Style: typeset.Style{Bold: r.Bold, Italic: r.Italic, Lang: r.Lang}, Strike: r.Strike}
		if r.Small {
			sp.Size = smallSize
		}
		if r.Grey {
			sp.Color = grey
		}
		out = append(out, sp)
	}
	return out
}

func (g *Glossary) subtitle(made time.Time) string {
	var names []string
	for _, l := range g.Languages {
		names = append(names, term.LangName(l))
	}
	return fmt.Sprintf("%s · %d %s · %s", strings.Join(names, " · "), g.Entries, plural(g.Entries, "entry", "entries"), Made(made))
}

// PDF writes the glossary as a PDF in the fonts of the set; Missing on the
// set then tells the characters none of them has.
func PDF(g *Glossary, fonts *typeset.FontSet, made time.Time) []byte {
	w, h := typeset.A4Width, typeset.A4Height
	if g.Options.Page == "letter" {
		w, h = typeset.LetterWidth, typeset.LetterHeight
	}
	d := typeset.NewDocument(fonts, w, h)
	d.Title = g.Title
	d.Lang = g.Sort
	d.RTL = langtag.RTL(g.Sort)
	d.Head = func(first, last string) string {
		if first == last {
			return first
		}
		return first + " – " + last
	}
	if !g.Table() {
		d.Columns = 2
		d.ColumnRule = true
		d.Gap = 22
	}
	banner := []*typeset.Para{
		{Spans: []typeset.Span{{Text: g.Title, Style: typeset.Style{Bold: true}}}, Size: 18, SpaceAfter: 2},
		{Spans: []typeset.Span{{Text: g.subtitle(made), Color: grey}}, Size: 8.5, SpaceAfter: 14},
	}
	if l := g.Legend(); l != "" {
		banner[1].SpaceAfter = 3
		banner = append(banner, &typeset.Para{Spans: []typeset.Span{{Text: l, Color: grey, Style: typeset.Style{Italic: true}}}, Size: 8, SpaceAfter: 14})
	}
	d.Banner(banner...)
	for _, s := range g.Sections {
		if s.Letter != "" {
			d.Add(&typeset.Para{Spans: []typeset.Span{{Text: s.Letter, Style: typeset.Style{Bold: true}, Color: accent}}, Size: 22, SpaceBefore: 12, SpaceAfter: 6,
				Leading: 1.1, KeepWithNext: true, Bookmark: s.Letter, RTL: d.RTL})
		}
		if g.Table() {
			d.AddTable(g.pdfTable(s))
			continue
		}
		for _, a := range s.Articles {
			bs := g.Blocks(a)
			for k, b := range bs {
				p := &typeset.Para{Spans: spans(b.Runs), Size: bodySize, RTL: b.RTL, Leading: 1.28}
				if b.Head {
					p.Mark = a.Headword
					p.SpaceBefore = 5
					p.Hanging = 10
				} else {
					p.Indent, p.Hanging = 10, 10
				}
				// A short article stays in one column.
				p.KeepWithNext = k < len(bs)-1 && len(bs) <= 6
				d.Add(p)
			}
		}
	}
	return d.PDF(made)
}

func (g *Glossary) pdfTable(s Section) *typeset.Table {
	t := &typeset.Table{Padding: 4, HeaderFill: [3]float64{0.93, 0.94, 0.96}, LineColor: [3]float64{0.82, 0.83, 0.86}}
	for i, h := range g.Headers() {
		w := 1.0
		if i == len(g.Languages) {
			w = 0.9
		}
		t.Widths = append(t.Widths, w)
		rtl := i < len(g.Languages) && langtag.RTL(g.Languages[i])
		t.Header = append(t.Header, typeset.Cell{{Spans: []typeset.Span{{Text: h, Style: typeset.Style{Bold: true}}}, Size: 8.5, RTL: rtl}})
	}
	for _, a := range s.Articles {
		for _, e := range a.Entries {
			var row []typeset.Cell
			for _, cell := range g.Cells(e) {
				var c typeset.Cell
				for _, b := range cell {
					p := &typeset.Para{Spans: spans(b.Runs), Size: bodySize, RTL: b.RTL, Leading: 1.25}
					if len(c) > 0 {
						p.SpaceBefore = 2
					}
					c = append(c, p)
				}
				row = append(row, c)
			}
			t.Rows = append(t.Rows, row)
			t.Marks = append(t.Marks, a.Headword)
		}
	}
	return t
}

// Text writes the glossary as plain text.
func Text(g *Glossary, made time.Time) []byte {
	var b strings.Builder
	b.WriteString(g.Title + "\n")
	b.WriteString(strings.Repeat("=", max(3, len([]rune(g.Title)))) + "\n")
	b.WriteString(g.subtitle(made) + "\n")
	if l := g.Legend(); l != "" {
		b.WriteString(l + "\n")
	}
	for _, s := range g.Sections {
		if s.Letter != "" {
			b.WriteString("\n" + s.Letter + "\n")
		}
		if g.Table() {
			for _, a := range s.Articles {
				for _, e := range a.Entries {
					b.WriteString("\n")
					hs := g.Headers()
					for i, cell := range g.Cells(e) {
						if len(cell) == 0 {
							continue
						}
						var parts []string
						for _, bl := range cell {
							parts = append(parts, JoinRuns(bl.Runs))
						}
						b.WriteString(hs[i] + ": " + strings.Join(parts, "; ") + "\n")
					}
				}
			}
			continue
		}
		for _, a := range s.Articles {
			b.WriteString("\n")
			for _, bl := range g.Blocks(a) {
				if bl.Indent > 0 {
					b.WriteString("    ")
				}
				b.WriteString(strings.TrimSpace(JoinRuns(bl.Runs)) + "\n")
			}
		}
	}
	return []byte(b.String())
}
