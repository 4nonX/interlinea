package glossary

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/typeset"
)

// Result is a file an export made.
type Result struct {
	Data      []byte
	Extension string
	MediaType string
	// Summary says what it holds.
	Summary string
	// Missing are characters no font had (PDF only): they show as boxes.
	Missing []rune
}

// Problem is an option that cannot be followed, and which.
type Problem struct {
	Field   string
	Message string
}

func (p *Problem) Error() string { return p.Message }

// Check tells the first option that does not fit the termbase.
func Check(tb *term.Termbase, o term.ExportOptions) error {
	if field, msg := o.Problem(); msg != "" {
		return &Problem{field, msg}
	}
	has := func(l string) bool {
		for _, x := range tb.Languages {
			if strings.EqualFold(x, l) {
				return true
			}
		}
		return false
	}
	for _, l := range o.Languages {
		if !has(l) {
			return &Problem{"languages", fmt.Sprintf("%s has no language %s.", tb.Name, l)}
		}
	}
	if o.SortBy != "" {
		if !has(o.SortBy) {
			return &Problem{"sortBy", fmt.Sprintf("%s has no language %s to sort by.", tb.Name, o.SortBy)}
		}
		if len(o.Languages) > 0 {
			in := false
			for _, l := range o.Languages {
				in = in || strings.EqualFold(l, o.SortBy)
			}
			if !in {
				return &Problem{"sortBy", "Sort by one of the languages exported."}
			}
		}
	}
	if o.Glossary() && len(tb.Languages) == 0 {
		return &Problem{"languages", fmt.Sprintf("%s has no languages yet.", tb.Name)}
	}
	return nil
}

// Export makes the file the options ask for. fonts is used for PDF; a nil
// set takes no fonts, so a PDF cannot be made.
func Export(tb *term.Termbase, o term.ExportOptions, fonts *typeset.FontSet, made time.Time) (*Result, error) {
	if err := Check(tb, o); err != nil {
		return nil, err
	}
	if !o.Glossary() {
		sel := term.Select(tb, o)
		f := term.Format(o.Format)
		data, err := term.Write(sel, f)
		if err != nil {
			return nil, err
		}
		n := len(sel.Entries)
		return &Result{Data: data, Extension: f.Extension(), MediaType: mediaType(o.Format), Summary: fmt.Sprintf("%d %s", n, plural(n, "entry", "entries"))}, nil
	}
	g := Build(tb, o)
	r := &Result{Summary: g.Summary(), MediaType: mediaType(o.Format)}
	switch o.Format {
	case "pdf":
		if fonts == nil {
			return nil, errors.New("glossary: no fonts to make a PDF with")
		}
		r.Data, r.Extension = PDF(g, fonts, made), ".pdf"
		r.Missing = fonts.Missing()
	case "docx":
		r.Data, r.Extension = Word(g, made), ".docx"
	case "html":
		r.Data, r.Extension = HTML(g, made), ".html"
	case "txt":
		r.Data, r.Extension = Text(g, made), ".txt"
	}
	return r, nil
}

func mediaType(format string) string {
	switch format {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "html":
		return "text/html; charset=utf-8"
	case "txt":
		return "text/plain; charset=utf-8"
	case "csv":
		return "text/csv; charset=utf-8"
	case "tsv":
		return "text/tab-separated-values; charset=utf-8"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	return "application/xml"
}

// Presets are the ready-made exports every termbase offers, besides those
// kept with it.
func Presets() []term.ExportPreset {
	return []term.ExportPreset{
		{Name: "Printable dictionary", Options: term.ExportOptions{Format: "pdf", Layout: "dictionary", Letters: true, Labels: true, Status: true, AllFields: true}},
		{Name: "Glossary table", Options: term.ExportOptions{Format: "pdf", Layout: "table", Letters: true, Status: true, Fields: []string{}}},
		{Name: "Word dictionary", Options: term.ExportOptions{Format: "docx", Layout: "dictionary", Letters: true, Labels: true, Status: true, AllFields: true}},
		{Name: "Web page", Options: term.ExportOptions{Format: "html", Layout: "dictionary", Letters: true, Labels: true, Status: true, AllFields: true}},
		{Name: "Approved terms only", Options: term.ExportOptions{Format: "pdf", Layout: "table", Letters: true, Statuses: []string{term.StatusApproved}, Fields: []string{}}},
		{Name: "Everything as TBX", Options: term.ExportOptions{Format: "tbx", AllFields: true}},
	}
}
