package glossary

import (
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/term"
)

// Run is a piece of text in one style; every format sets runs the same way.
type Run struct {
	Text   string
	Bold   bool
	Italic bool
	// Small and Grey for what is said about a term rather than the term.
	Small, Grey bool
	// Strike for a forbidden term.
	Strike bool
	// Lang of the text, if it is in one of the termbase's languages.
	Lang string
}

// Block is a paragraph of an article.
type Block struct {
	Runs []Run
	// Lang of the paragraph's terms, which sets its direction.
	Lang string
	RTL  bool
	// Indent: 0 for the headword's paragraph, 1 for what follows it.
	Indent int
	// Head marks the paragraph that starts an article.
	Head bool
}

// Kinds are the termbase's fields by name, to show values by their kind.
func kinds(tb *term.Termbase) map[string]term.Kind {
	m := map[string]term.Kind{}
	for _, f := range tb.Fields {
		m[strings.ToLower(f.Name)] = f.Kind
	}
	return m
}

// Show writes a value as its kind reads: yes or no rather than true.
func (g *Glossary) Show(field, value string) string {
	switch g.kinds[strings.ToLower(field)] {
	case term.KindBoolean:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes":
			return "yes"
		case "false", "0", "no":
			return "no"
		}
	}
	return value
}

// gramRuns sets a term's own fields as a dictionary does: its grammar
// abbreviated in small italics ("f", "v", "adj pl"), then its other
// fields, such as a context, in brackets.
func (g *Glossary) gramRuns(vs []term.Value) []Run {
	var out []Run
	if gr, _ := g.grammar(vs); gr != "" {
		out = append(out, Run{Text: " " + gr, Small: true, Italic: true, Grey: true})
	}
	var rest []string
	for _, n := range g.TermFields {
		if grammarField(n) == notGrammar {
			rest = append(rest, n)
		}
	}
	if f := g.fieldRuns(vs, rest); len(f) > 0 {
		out = append(out, Run{Text: " (", Small: true})
		out = append(out, f...)
		out = append(out, Run{Text: ")", Small: true})
	}
	return out
}

// Legend explains the grammar abbreviations used, or is empty.
func (g *Glossary) Legend() string {
	ab := g.Abbreviations()
	if len(ab) == 0 {
		return ""
	}
	return "Abbreviations: " + strings.Join(ab, " · ")
}

func (g *Glossary) fieldRuns(vs []term.Value, names []string) []Run {
	var out []Run
	for _, n := range names {
		v := Value(vs, n)
		if v == "" {
			continue
		}
		v = g.Show(n, v)
		if len(out) > 0 {
			out = append(out, Run{Text: "  ", Small: true})
		}
		if g.Options.Labels {
			out = append(out, Run{Text: n + " ", Small: true, Grey: true, Italic: true})
		}
		out = append(out, Run{Text: v, Small: true})
	}
	return out
}

// termRuns sets terms one after the other with their status and fields.
func (g *Glossary) termRuns(terms []term.Term, lang string, bold bool) []Run {
	var out []Run
	for i, t := range terms {
		if i > 0 {
			out = append(out, Run{Text: "; "})
		}
		out = append(out, Run{Text: t.Text, Bold: bold, Lang: lang, Strike: g.Options.Status && t.Status == term.StatusForbidden})
		if g.Options.Status {
			if m := StatusMark(t.Status); m != "" {
				out = append(out, Run{Text: " " + m, Small: true, Grey: true, Italic: true})
			}
		}
		out = append(out, g.gramRuns(t.Fields)...)
	}
	return out
}

// Blocks lays out an article as a dictionary has it: the headword with the
// rest of its language, then each other language on a line of its own.
func (g *Glossary) Blocks(a Article) []Block {
	var out []Block
	sortRTL := langtag.RTL(g.Sort)
	for k, e := range a.Entries {
		head := Block{Lang: g.Sort, RTL: sortRTL, Head: k == 0}
		if k == 0 {
			head.Runs = append(head.Runs, Run{Text: a.Headword, Bold: true, Lang: g.Sort, Strike: g.Options.Status && a.Status == term.StatusForbidden})
			if g.Options.Status {
				if m := StatusMark(a.Status); m != "" {
					head.Runs = append(head.Runs, Run{Text: " " + m, Small: true, Grey: true, Italic: true})
				}
			}
		}
		if len(a.Entries) > 1 {
			if k > 0 {
				out = append(out, head)
				head = Block{Lang: g.Sort, RTL: sortRTL, Indent: 1}
			}
			head.Runs = append(head.Runs, Run{Text: "  " + strconv.Itoa(k+1) + " ", Bold: true, Small: true})
		}
		sl, _ := LanguageOf(e, g.Sort)
		// The headword's own fields, then its synonyms.
		var rest []term.Term
		own := -1
		for i, t := range sl.Terms {
			if own < 0 && t.Text == a.Headword {
				own = i
				continue
			}
			rest = append(rest, t)
		}
		if own >= 0 {
			head.Runs = append(head.Runs, g.gramRuns(sl.Terms[own].Fields)...)
		}
		if len(rest) > 0 {
			head.Runs = append(head.Runs, Run{Text: ", "})
			head.Runs = append(head.Runs, g.termRuns(rest, g.Sort, false)...)
		}
		if f := g.fieldRuns(sl.Fields, g.LangFields); len(f) > 0 {
			head.Runs = append(head.Runs, Run{Text: "   "})
			head.Runs = append(head.Runs, f...)
		}
		if f := g.fieldRuns(e.Fields, g.EntryFields); len(f) > 0 {
			head.Runs = append(head.Runs, Run{Text: "   "})
			head.Runs = append(head.Runs, f...)
		}
		out = append(out, head)
		for _, lang := range g.Languages[1:] {
			l, ok := LanguageOf(e, lang)
			if !ok || len(l.Terms) == 0 {
				continue
			}
			b := Block{Lang: lang, RTL: langtag.RTL(lang), Indent: 1}
			b.Runs = append(b.Runs, Run{Text: langLabel(lang) + "  ", Small: true, Grey: true, Bold: true})
			b.Runs = append(b.Runs, g.termRuns(l.Terms, lang, false)...)
			if f := g.fieldRuns(l.Fields, g.LangFields); len(f) > 0 {
				b.Runs = append(b.Runs, Run{Text: "   "})
				b.Runs = append(b.Runs, f...)
			}
			out = append(out, b)
		}
	}
	return out
}

// Cells lays out an entry as a table row: a cell for each language, its
// terms one under the other, and a last cell for the entry's own fields
// when any are shown.
func (g *Glossary) Cells(e term.Entry) [][]Block {
	var out [][]Block
	for _, lang := range g.Languages {
		var cell []Block
		l, _ := LanguageOf(e, lang)
		rtl := langtag.RTL(lang)
		for i, t := range l.Terms {
			b := Block{Lang: lang, RTL: rtl, Head: i == 0}
			b.Runs = g.termRuns([]term.Term{t}, lang, i == 0 && strings.EqualFold(lang, g.Sort))
			cell = append(cell, b)
		}
		if f := g.fieldRuns(l.Fields, g.LangFields); len(f) > 0 {
			cell = append(cell, Block{Lang: lang, RTL: rtl, Runs: f, Indent: 1})
		}
		out = append(out, cell)
	}
	if len(g.EntryFields) > 0 {
		var cell []Block
		for _, n := range g.EntryFields {
			if f := g.fieldRuns(e.Fields, []string{n}); len(f) > 0 {
				cell = append(cell, Block{Runs: f})
			}
		}
		out = append(out, cell)
	}
	return out
}

// Headers are the table's column headings.
func (g *Glossary) Headers() []string {
	var out []string
	for _, l := range g.Languages {
		out = append(out, term.LangName(l))
	}
	if len(g.EntryFields) > 0 {
		out = append(out, strings.Join(g.EntryFields, ", "))
	}
	return out
}

// Table tells the glossary is laid out as a table.
func (g *Glossary) Table() bool { return g.Options.Layout == "table" }

// JoinRuns joins runs into plain text.
func JoinRuns(rs []Run) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// langLabel is the short name of a language before its terms: "EN-GB".
func langLabel(code string) string { return strings.ToUpper(code) }
