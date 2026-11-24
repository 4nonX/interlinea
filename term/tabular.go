package term

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Role is what a spreadsheet column holds.
type Role string

const (
	RoleTerm   Role = "term"   // terms of a language; synonyms split by the separator
	RoleStatus Role = "status" // status of the terms of a language
	RoleField  Role = "field"  // a field at entry, language or term level
	RoleID     Role = "id"     // entry ID
	RoleIgnore Role = "ignore"
)

// Column maps one spreadsheet column.
type Column struct {
	Index  int    `json:"index"`
	Header string `json:"header,omitempty"`
	Role   Role   `json:"role"`
	Lang   string `json:"lang,omitempty"`
	Field  string `json:"field,omitempty"`
	Level  Level  `json:"level,omitempty"`
}

// Mapping says how to read a spreadsheet, as memoQ's and MultiTerm
// Convert's column mapping do.
type Mapping struct {
	Columns []Column `json:"columns"`
	// Header is true when the first row holds column names.
	Header bool `json:"header"`
	// Separator splits several terms (synonyms) in one cell; default ";".
	Separator string `json:"separator,omitempty"`
}

// Sheet is a spreadsheet read for mapping: its rows and a guessed mapping.
type Sheet struct {
	Rows    [][]string `json:"rows"`
	Mapping Mapping    `json:"mapping"`
	// Delimiter is the CSV delimiter that was detected (",", ";" or tab).
	Delimiter string `json:"delimiter,omitempty"`
}

// ReadCSV reads delimited text. delim 0 detects comma, semicolon or tab.
func ReadCSV(data []byte, delim rune) ([][]string, rune, error) {
	text := ToUTF8(data)
	if delim == 0 {
		delim = detectDelimiter(text)
	}
	r := csv.NewReader(bytes.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, delim, fmt.Errorf("csv: %w", err)
	}
	return rows, delim, nil
}

func detectDelimiter(text []byte) rune {
	lines := strings.SplitN(string(text), "\n", 6)
	best, bestN := ',', -1
	for _, d := range []rune{'\t', ';', ','} {
		n, consistent := -1, true
		for _, l := range lines[:max(1, len(lines)-1)] {
			c := strings.Count(l, string(d))
			if n < 0 {
				n = c
			} else if c != n {
				consistent = false
			}
		}
		if n > 0 && consistent && n > bestN {
			best, bestN = d, n
		}
	}
	if bestN < 0 {
		for _, d := range []rune{'\t', ';', ','} {
			if strings.ContainsRune(lines[0], d) {
				return d
			}
		}
	}
	return best
}

var (
	reFieldLang = regexp.MustCompile(`^(.+?)\s*[\(\[]\s*([A-Za-z]{2,3}(?:[-_][A-Za-z0-9]{2,8})?|[^\)\]]+)\s*[\)\]]$`) // "Definition (de)", "Note [German]"
	reLangField = regexp.MustCompile(`^([A-Za-z]{2,3}(?:[-_][A-Za-z0-9]{2,8})?)[\s_:]+(.+)$`)                         // "de-DE Status", "en_Note", "de: Definition"
	reFieldSfx  = regexp.MustCompile(`^(.+?)[\s_]+([A-Za-z]{2,3}(?:[-_][A-Za-z0-9]{2,8})?)$`)                         // "Definition_de", "Status EN"
)

var idHeaders = map[string]bool{"id": true, "entry id": true, "entry": true, "entry number": true, "concept": true, "concept id": true, "nr": true, "nr.": true, "no": true, "no.": true, "eintrag": true, "eintragsnummer": true, "#": true}

func isStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "status", "term status", "termstatus", "usage status", "administrative status", "administrativestatus", "freigabe":
		return true
	}
	return false
}

// GuessMapping reads the header row the way people write it: a language
// name or code makes a term column ("German", "de-DE", "EN"); a field with
// a language ("Definition (de)", "en Note", "Status_DE") is a language-level
// field or that language's status; "Term ..." after the language marks a
// term-level field ("de-DE Term Gender"); other names are entry fields. A
// first row without any language is taken as data, not a header.
func GuessMapping(rows [][]string) Mapping {
	m := Mapping{Separator: ";"}
	if len(rows) == 0 {
		return m
	}
	header := rows[0]
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	cols := make([]Column, width)
	langs := 0
	lastLang := ""
	for i := range cols {
		h := ""
		if i < len(header) {
			h = strings.TrimSpace(header[i])
		}
		c := Column{Index: i, Header: h, Role: RoleField, Field: h, Level: LevelEntry}
		switch {
		case h == "":
			c.Role, c.Field = RoleIgnore, ""
		case idHeaders[strings.ToLower(h)]:
			c.Role, c.Field = RoleID, ""
		default:
			if l, ok := LangFromName(h); ok {
				c.Role, c.Lang, c.Field = RoleTerm, l, ""
				langs++
				lastLang = l
				break
			}
			lang, field := splitLangField(h)
			switch {
			case lang != "" && isStatus(field):
				c.Role, c.Lang, c.Field = RoleStatus, lang, ""
			case lang != "" && strings.HasPrefix(strings.ToLower(field), "term "):
				c.Lang, c.Field, c.Level = lang, strings.TrimSpace(field[5:]), LevelTerm
			case lang != "":
				c.Lang, c.Field, c.Level = lang, field, LevelLanguage
			case isStatus(h) && lastLang != "":
				c.Role, c.Lang, c.Field = RoleStatus, lastLang, ""
			}
		}
		cols[i] = c
	}
	m.Columns = cols
	m.Header = langs > 0
	m.Separator = guessSeparator(rows, cols, m.Header)
	if !m.Header {
		// No header: the first two columns are the terms of two languages
		// the user has to name.
		for i := range m.Columns {
			m.Columns[i] = Column{Index: i, Role: RoleIgnore}
			if i < 2 {
				m.Columns[i].Role = RoleTerm
			}
		}
	}
	return m
}

// guessSeparator picks the synonym separator of the term columns: ";"
// unless no term cell uses it and some use "|".
func guessSeparator(rows [][]string, cols []Column, header bool) string {
	semi, bar := 0, 0
	start := 0
	if header {
		start = 1
	}
	for _, r := range rows[start:] {
		for _, c := range cols {
			if c.Role != RoleTerm || c.Index >= len(r) {
				continue
			}
			semi += strings.Count(r[c.Index], ";")
			bar += strings.Count(r[c.Index], "|")
		}
	}
	if semi == 0 && bar > 0 {
		return "|"
	}
	return ";"
}

func splitLangField(h string) (lang, field string) {
	if m := reFieldLang.FindStringSubmatch(h); m != nil {
		if l, ok := LangFromName(m[2]); ok {
			return l, strings.TrimSpace(m[1])
		}
	}
	if m := reLangField.FindStringSubmatch(h); m != nil {
		if l, ok := LangFromName(m[1]); ok {
			return l, strings.TrimSpace(m[2])
		}
	}
	if m := reFieldSfx.FindStringSubmatch(h); m != nil {
		if l, ok := LangFromName(m[2]); ok {
			return l, strings.TrimSpace(m[1])
		}
	}
	// A language name before or after a field: "German Definition",
	// "Status Englisch".
	parts := strings.Fields(h)
	for i := len(parts) - 1; i > 0; i-- {
		if l, ok := LangFromName(strings.Join(parts[:i], " ")); ok {
			return l, strings.Join(parts[i:], " ")
		}
	}
	for i := 1; i < len(parts); i++ {
		if l, ok := LangFromName(strings.Join(parts[i:], " ")); ok {
			return l, strings.Join(parts[:i], " ")
		}
	}
	return "", ""
}

// FromRows turns spreadsheet rows into entries with a mapping. Each row is
// one entry, as in memoQ and MultiTerm Convert.
func FromRows(rows [][]string, m Mapping) (*Termbase, Report, error) {
	var rep Report
	sep := m.Separator
	if sep == "" {
		sep = ";"
	}
	hasTerm := false
	for _, c := range m.Columns {
		if c.Role == RoleTerm {
			if c.Lang == "" {
				return nil, rep, fmt.Errorf("column %d holds terms but has no language", c.Index+1)
			}
			hasTerm = true
		}
	}
	if !hasTerm {
		return nil, rep, fmt.Errorf("no column holds terms")
	}
	start := 0
	if m.Header {
		start = 1
	}
	var entries []Entry
	split := func(s string) []string {
		var out []string
		for _, p := range splitCell(s, sep) {
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	// Status and term fields line up with the terms, so empty places count.
	aligned := func(s string) []string { return splitCell(s, sep) }
	for r := start; r < len(rows); r++ {
		row := rows[r]
		cell := func(i int) string {
			if i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		e := Entry{}
		get := func(lang string) *Language {
			if l := e.Lang(lang); l != nil {
				return l
			}
			e.Languages = append(e.Languages, Language{Lang: lang})
			return &e.Languages[len(e.Languages)-1]
		}
		// Terms first, so status and term fields can attach to them.
		for _, c := range m.Columns {
			if c.Role == RoleTerm {
				l := get(c.Lang)
				for _, t := range split(cell(c.Index)) {
					l.Terms = append(l.Terms, Term{Text: t})
				}
			}
		}
		for _, c := range m.Columns {
			v := cell(c.Index)
			if v == "" {
				continue
			}
			switch c.Role {
			case RoleID:
				e.ID = v
			case RoleStatus:
				l := get(c.Lang)
				vals := aligned(v)
				for i := range l.Terms {
					// One status for several terms applies to all of them.
					k := i
					if len(vals) == 1 {
						k = 0
					}
					if k < len(vals) {
						l.Terms[i].Status = NormalizeStatus(vals[k])
					}
				}
			case RoleField:
				switch c.Level {
				case LevelLanguage:
					l := get(c.Lang)
					l.Fields = append(l.Fields, Value{Field: c.Field, Value: v})
				case LevelTerm:
					l := get(c.Lang)
					vals := aligned(v)
					for i := range l.Terms {
						if i < len(vals) {
							l.Terms[i].Fields = append(l.Terms[i].Fields, Value{Field: c.Field, Value: vals[i]})
						}
					}
				default:
					e.Fields = append(e.Fields, Value{Field: c.Field, Value: v})
				}
			}
		}
		if e.Clean() {
			entries = append(entries, e)
			rep.Entries++
			rep.Terms += e.TermCount()
		} else if strings.TrimSpace(strings.Join(row, "")) != "" {
			rep.Skipped++
			rep.warn("Row %d has no terms and was skipped.", r+1)
		}
	}
	return &Termbase{Definition: InferDefinition("", entries), Entries: entries}, rep, nil
}

// ToRows lays a termbase out as one row per entry: ID, entry fields, then
// for each language its terms, their status, language fields and term
// fields. Several terms share a cell, separated by "; ", and their status
// and term fields follow the same order. GuessMapping reads this back.
func ToRows(tb *Termbase) [][]string {
	type col struct {
		header string
		get    func(e *Entry) string
	}
	var cols []col
	cols = append(cols, col{"ID", func(e *Entry) string { return e.ID }})
	entryFields, langFields, termFields := orderedFields(tb)
	for _, f := range entryFields {
		cols = append(cols, col{f, func(e *Entry) string { return joinValues(e.Fields, f) }})
	}
	langs := tb.Languages
	if len(langs) == 0 {
		langs = InferDefinition("", tb.Entries).Languages
	}
	for _, lg := range langs {
		cols = append(cols, col{lg, func(e *Entry) string {
			l := e.Lang(lg)
			if l == nil {
				return ""
			}
			var ts []string
			for _, t := range l.Terms {
				ts = append(ts, t.Text)
			}
			return joinCell(ts)
		}})
		cols = append(cols, col{lg + " Status", func(e *Entry) string {
			l := e.Lang(lg)
			if l == nil {
				return ""
			}
			var ss []string
			has := false
			for _, t := range l.Terms {
				s := t.Status
				if s == "" {
					s = StatusProposed
				} else {
					has = true
				}
				ss = append(ss, s)
			}
			if !has {
				return ""
			}
			return joinCell(ss)
		}})
		for _, f := range langFields[lg] {
			cols = append(cols, col{lg + " " + f, func(e *Entry) string {
				if l := e.Lang(lg); l != nil {
					return joinValues(l.Fields, f)
				}
				return ""
			}})
		}
		for _, f := range termFields[lg] {
			cols = append(cols, col{lg + " Term " + f, func(e *Entry) string {
				l := e.Lang(lg)
				if l == nil {
					return ""
				}
				var vs []string
				has := false
				for _, t := range l.Terms {
					v := firstValue(t.Fields, f)
					if v != "" {
						has = true
					}
					vs = append(vs, v)
				}
				if !has {
					return ""
				}
				return joinCell(vs)
			}})
		}
	}
	rows := [][]string{make([]string, len(cols))}
	for i, c := range cols {
		rows[0][i] = c.header
	}
	for i := range tb.Entries {
		row := make([]string, len(cols))
		for j, c := range cols {
			row[j] = c.get(&tb.Entries[i])
		}
		rows = append(rows, row)
	}
	return rows
}

func orderedFields(tb *Termbase) (entry []string, lang, term map[string][]string) {
	lang, term = map[string][]string{}, map[string][]string{}
	seen := map[string]bool{}
	add := func(list *[]string, key, name string) {
		if !seen[key] {
			seen[key] = true
			*list = append(*list, name)
		}
	}
	for _, e := range tb.Entries {
		for _, v := range e.Fields {
			add(&entry, "e|"+v.Field, v.Field)
		}
		for _, l := range e.Languages {
			ll := lang[l.Lang]
			for _, v := range l.Fields {
				add(&ll, "l|"+l.Lang+"|"+v.Field, v.Field)
			}
			lang[l.Lang] = ll
			tl := term[l.Lang]
			for _, t := range l.Terms {
				for _, v := range t.Fields {
					add(&tl, "t|"+l.Lang+"|"+v.Field, v.Field)
				}
			}
			term[l.Lang] = tl
		}
	}
	return entry, lang, term
}

func joinValues(vs []Value, field string) string {
	var out []string
	for _, v := range vs {
		if v.Field == field {
			out = append(out, v.Value)
		}
	}
	return strings.Join(out, "; ")
}

func firstValue(vs []Value, field string) string {
	for _, v := range vs {
		if v.Field == field {
			return v.Value
		}
	}
	return ""
}

// WriteCSV writes rows as UTF-8 CSV with a byte order mark (so Excel opens
// umlauts correctly) or as tab-separated text.
func WriteCSV(rows [][]string, delim rune) []byte {
	var buf bytes.Buffer
	if delim != '\t' {
		buf.Write([]byte{0xEF, 0xBB, 0xBF})
	}
	w := csv.NewWriter(&buf)
	w.Comma = delim
	w.UseCRLF = true
	for _, r := range rows {
		clean := make([]string, len(r))
		for i, v := range r {
			if !utf8.ValidString(v) {
				v = strings.ToValidUTF8(v, "")
			}
			clean[i] = v
		}
		_ = w.Write(clean)
	}
	w.Flush()
	return buf.Bytes()
}

// joinCell writes the values of an entry's terms into one cell, separated
// by "; ". A semicolon inside a value is written "\;", so a term or a
// definition with a semicolon in it comes back as one value.
func joinCell(values []string) string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), ";", `\;`)
	}
	return strings.Join(out, "; ")
}

// splitCell splits a cell at the separator, trims the parts and reads
// "\;" as a semicolon that belongs to the value.
func splitCell(s, sep string) []string {
	if !strings.Contains(s, `\`) {
		parts := strings.Split(s, sep)
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\\' && i+1 < len(s) && (s[i+1] == ';' || s[i+1] == '\\'):
			cur.WriteByte(s[i+1])
			i += 2
		case strings.HasPrefix(s[i:], sep):
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
			i += len(sep)
		default:
			cur.WriteByte(s[i])
			i++
		}
	}
	return append(parts, strings.TrimSpace(cur.String()))
}
