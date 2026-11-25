package term

import (
	"fmt"
	"strings"
	"time"
)

// ExportOptions say what an export holds and, for a glossary, how it is
// laid out. They are kept with a termbase as named presets.
type ExportOptions struct {
	// Format is an exchange format (tbx, tbx2, multiterm, csv, tsv, xlsx)
	// or a glossary to read: pdf, docx, html or txt.
	Format string `json:"format"`
	// Title of a glossary; the termbase's name if empty.
	Title string `json:"title,omitempty"`
	// Languages exported, in their order; all if empty.
	Languages []string `json:"languages,omitempty"`
	// SortBy is the language a glossary is sorted by and its entries
	// headed with; the first language if empty.
	SortBy string `json:"sortBy,omitempty"`
	// Fields exported by name: all if left out, none if empty.
	Fields []string `json:"fields,omitempty"`
	// AllFields exports every field (what a left-out Fields says, made
	// plain for a preset).
	AllFields bool `json:"allFields,omitempty"`
	// Statuses of the terms exported (approved, proposed, forbidden); all
	// if empty.
	Statuses []string `json:"statuses,omitempty"`
	// Entries exported by ID, such as those a search shows; all if empty.
	Entries []string `json:"entries,omitempty"`
	// Only entries with this value in this field (an entry, language or
	// term field), such as a subject field.
	FilterField string `json:"filterField,omitempty"`
	FilterValue string `json:"filterValue,omitempty"`
	// Only entries changed on or after this day (YYYY-MM-DD).
	ChangedSince string `json:"changedSince,omitempty"`

	// Layout of a glossary: "dictionary" (entries one after the other, in
	// two columns) or "table" (a column per language).
	Layout string `json:"layout,omitempty"`
	// PerTerm makes every term of the sort language a headword of its own,
	// so a synonym is found under its own letter.
	PerTerm bool `json:"perTerm,omitempty"`
	// GroupHomonyms puts entries with the same headword under it, numbered.
	GroupHomonyms bool `json:"groupHomonyms,omitempty"`
	// Letters puts each initial letter before its headwords.
	Letters bool `json:"letters,omitempty"`
	// Labels names the fields before their values.
	Labels bool `json:"labels,omitempty"`
	// Status marks terms that are proposed or forbidden.
	Status bool `json:"status,omitempty"`
	// Page size of a PDF or Word glossary: "a4" (default) or "letter".
	Page string `json:"page,omitempty"`
}

// Glossary tells the format is a glossary to read, not an exchange format.
func (o ExportOptions) Glossary() bool {
	switch o.Format {
	case "pdf", "docx", "html", "txt":
		return true
	}
	return false
}

// ExportPreset is export options kept under a name.
type ExportPreset struct {
	Name    string        `json:"name"`
	Options ExportOptions `json:"options"`
}

// WantsField tells whether a field is exported.
func (o ExportOptions) WantsField(name string) bool {
	if o.AllFields || o.Fields == nil {
		return true
	}
	for _, f := range o.Fields {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}

// WantsStatus tells whether terms of a status are exported.
func (o ExportOptions) WantsStatus(status string) bool {
	if len(o.Statuses) == 0 {
		return true
	}
	if status == "" {
		status = StatusApproved
	}
	for _, s := range o.Statuses {
		if s == status {
			return true
		}
	}
	return false
}

// Select gives the termbase with what the options leave out taken out:
// languages, fields, terms of other statuses, and entries the filter does
// not match. Entries with no term left are left out.
func Select(tb *Termbase, o ExportOptions) *Termbase {
	out := &Termbase{Definition: tb.Definition}
	langs := o.Languages
	if len(langs) == 0 {
		langs = tb.Languages
	}
	keepLang := map[string]bool{}
	for _, l := range langs {
		keepLang[strings.ToLower(l)] = true
	}
	out.Languages = nil
	for _, l := range langs {
		out.Languages = append(out.Languages, l)
	}
	out.Fields = nil
	for _, f := range tb.Fields {
		if o.WantsField(f.Name) {
			out.Fields = append(out.Fields, f)
		}
	}
	var ids map[string]bool
	if len(o.Entries) > 0 {
		ids = map[string]bool{}
		for _, id := range o.Entries {
			ids[id] = true
		}
	}
	var since time.Time
	if o.ChangedSince != "" {
		since, _ = time.Parse("2006-01-02", o.ChangedSince)
	}
	fields := func(vs []Value) []Value {
		var kept []Value
		for _, v := range vs {
			if o.WantsField(v.Field) {
				kept = append(kept, v)
			}
		}
		return kept
	}
	for _, e := range tb.Entries {
		if ids != nil && !ids[e.ID] {
			continue
		}
		if !since.IsZero() {
			t := e.ChangedAt
			if t.IsZero() {
				t = e.CreatedAt
			}
			if t.Before(since) {
				continue
			}
		}
		if o.FilterField != "" && !entryHas(e, o.FilterField, o.FilterValue) {
			continue
		}
		ne := e
		ne.Fields = fields(e.Fields)
		ne.Languages = nil
		terms := 0
		for _, l := range e.Languages {
			if !keepLang[strings.ToLower(l.Lang)] {
				continue
			}
			nl := Language{Lang: l.Lang, Fields: fields(l.Fields)}
			for _, t := range l.Terms {
				if !o.WantsStatus(t.Status) {
					continue
				}
				nt := t
				nt.Fields = fields(t.Fields)
				nl.Terms = append(nl.Terms, nt)
			}
			if len(nl.Terms) > 0 {
				terms += len(nl.Terms)
				ne.Languages = append(ne.Languages, nl)
			}
		}
		if terms == 0 {
			continue
		}
		out.Entries = append(out.Entries, ne)
	}
	return out
}

// entryHas tells whether an entry has a value in a field, at any level.
func entryHas(e Entry, field, value string) bool {
	match := func(vs []Value) bool {
		for _, v := range vs {
			if strings.EqualFold(v.Field, field) && (value == "" || strings.EqualFold(strings.TrimSpace(v.Value), strings.TrimSpace(value))) {
				return true
			}
		}
		return false
	}
	if match(e.Fields) {
		return true
	}
	for _, l := range e.Languages {
		if match(l.Fields) {
			return true
		}
		for _, t := range l.Terms {
			if match(t.Fields) {
				return true
			}
		}
	}
	return false
}

// Problem tells what is wrong with options on their own (not measured
// against a termbase) and which option it is, or "".
func (o ExportOptions) Problem() (field, message string) {
	switch o.Format {
	case "pdf", "docx", "html", "txt", "tbx", "tbx2", "multiterm", "csv", "tsv", "xlsx":
	case "":
		return "format", "Choose what to export to."
	default:
		return "format", fmt.Sprintf("%q is not a format to export to.", o.Format)
	}
	switch o.Layout {
	case "", "dictionary", "table":
	default:
		return "layout", "The layout is dictionary or table."
	}
	switch o.Page {
	case "", "a4", "letter":
	default:
		return "page", "The page is A4 or letter."
	}
	for _, s := range o.Statuses {
		if s != StatusApproved && s != StatusProposed && s != StatusForbidden {
			return "statuses", fmt.Sprintf("%q is not a status: approved, proposed or forbidden.", s)
		}
	}
	if o.ChangedSince != "" {
		if _, err := time.Parse("2006-01-02", o.ChangedSince); err != nil {
			return "changedSince", "Give the day as YYYY-MM-DD."
		}
	}
	return "", ""
}
