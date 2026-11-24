package term

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// TBX data categories and the field names Interlinea shows for them.
// Reading maps the categories to these names; writing maps them back, so
// a TBX file survives a round trip. Fields without a category are written
// as notes in the form "[Name] value" and read back as fields.
var tbxCategories = []struct {
	elem, typ, name string
}{
	{"descrip", "definition", "Definition"},
	{"descrip", "subjectField", "Domain"},
	{"descrip", "context", "Context"},
	{"descrip", "explanation", "Explanation"},
	{"descrip", "example", "Example"},
	{"termNote", "partOfSpeech", "Part of speech"},
	{"termNote", "grammaticalGender", "Gender"},
	{"termNote", "grammaticalNumber", "Number"},
	{"termNote", "termType", "Term type"},
	{"termNote", "geographicalUsage", "Geographical usage"},
	{"termNote", "termLocation", "Term location"},
	{"termNote", "usageNote", "Usage note"},
	{"admin", "source", "Source"},
	{"admin", "customerSubset", "Client"},
	{"admin", "projectSubset", "Project"},
	{"admin", "responsibility", "Responsible"},
	{"admin", "entrySource", "Entry source"},
}

func fieldFromTBX(elem, typ string) string {
	for _, c := range tbxCategories {
		if c.typ == typ && (c.elem == elem || elem == "") {
			return c.name
		}
	}
	for _, c := range tbxCategories {
		if strings.EqualFold(c.typ, typ) {
			return c.name
		}
	}
	if typ == "" {
		return strings.ToUpper(elem[:1]) + elem[1:]
	}
	return typ
}

func tbxFromField(name string) (elem, typ string, ok bool) {
	for _, c := range tbxCategories {
		if strings.EqualFold(c.name, name) || strings.EqualFold(c.typ, name) {
			return c.elem, c.typ, true
		}
	}
	switch strings.ToLower(name) {
	case "note", "notes", "comment", "anmerkung", "notiz":
		return "note", "", true
	case "subject", "subject field", "fachgebiet", "sachgebiet":
		return "descrip", "subjectField", true
	case "definition", "definiton", "begriffsbestimmung":
		return "descrip", "definition", true
	}
	return "", "", false
}

var bracketNote = regexp.MustCompile(`(?s)^\[([^\]]{1,60})\] (.*)$`)

// ReadTBX reads TBX version 3 (ISO 30042:2019, <tbx>) and version 2
// (ISO 30042:2008, <martif>), in any dialect. Unknown data categories are
// kept as fields named after their type.
func ReadTBX(data []byte) (*Termbase, Report, error) {
	var rep Report
	var entries []Entry
	root, err := eachElement(data, map[string]bool{"conceptEntry": true, "termEntry": true}, func(n *xnode) error {
		e, ok := tbxEntry(n, &rep)
		if ok {
			entries = append(entries, e)
			rep.Entries++
			rep.Terms += e.TermCount()
		} else {
			rep.Skipped++
			rep.warn("Entry %q has no terms and was skipped.", n.attr("id"))
		}
		return nil
	})
	if err != nil {
		return nil, rep, fmt.Errorf("tbx: %w", err)
	}
	if root != "tbx" && root != "martif" {
		return nil, rep, fmt.Errorf("tbx: root element <%s> is not <tbx> or <martif>", root)
	}
	tb := &Termbase{Definition: InferDefinition("", entries), Entries: entries}
	return tb, rep, nil
}

func tbxEntry(n *xnode, rep *Report) (Entry, bool) {
	e := Entry{ID: n.attr("id")}
	for i := range n.Nodes {
		c := &n.Nodes[i]
		switch c.XMLName.Local {
		case "langSec", "langSet":
			l := Language{Lang: c.lang()}
			for j := range c.Nodes {
				cc := &c.Nodes[j]
				switch cc.XMLName.Local {
				case "termSec", "tig", "ntig":
					if t, ok := tbxTerm(cc); ok {
						l.Terms = append(l.Terms, t)
					}
				default:
					l.Fields = append(l.Fields, tbxValues(cc)...)
				}
			}
			e.Languages = append(e.Languages, l)
		case "transacGrp":
			tbxTransac(c, &e)
		default:
			e.Fields = append(e.Fields, tbxValues(c)...)
		}
	}
	return e, e.Clean()
}

func tbxTerm(n *xnode) (Term, bool) {
	var t Term
	var walk func(n *xnode)
	walk = func(n *xnode) {
		for i := range n.Nodes {
			c := &n.Nodes[i]
			switch c.XMLName.Local {
			case "term":
				t.Text = c.text()
			case "termGrp":
				walk(c)
			case "transacGrp":
			case "termNote":
				typ := c.attr("type")
				if typ == "administrativeStatus" || typ == "normativeAuthorization" {
					t.Status = NormalizeStatus(c.text())
					continue
				}
				t.Fields = append(t.Fields, tbxValues(c)...)
			default:
				t.Fields = append(t.Fields, tbxValues(c)...)
			}
		}
	}
	walk(n)
	return t, strings.TrimSpace(t.Text) != ""
}

// tbxValues reads a data category element (descrip, admin, note, termNote,
// xref, ref) or a group of them.
func tbxValues(n *xnode) []Value {
	switch n.XMLName.Local {
	case "descripGrp", "adminGrp", "termNoteGrp", "noteGrp":
		var out []Value
		for i := range n.Nodes {
			out = append(out, tbxValues(&n.Nodes[i])...)
		}
		return out
	case "note":
		t := n.text()
		if m := bracketNote.FindStringSubmatch(t); m != nil {
			return []Value{{Field: m[1], Value: m[2]}}
		}
		return []Value{{Field: "Note", Value: t}}
	case "xref":
		v := n.attr("target")
		if t := n.text(); t != "" {
			v = t + " (" + v + ")"
		}
		return []Value{{Field: "Cross reference", Value: v}}
	case "descrip", "admin", "termNote", "ref":
		return []Value{{Field: fieldFromTBX(n.XMLName.Local, n.attr("type")), Value: n.text()}}
	}
	// An element of the tool's own (<ShortInfo> in an .ibex glossary): its
	// text is kept as a field named after the element, not dropped.
	if len(n.Nodes) == 0 {
		if t := n.text(); t != "" {
			return []Value{{Field: n.XMLName.Local, Value: t}}
		}
	}
	return nil
}

func tbxTransac(n *xnode, e *Entry) {
	var kind, who string
	var when time.Time
	for i := range n.Nodes {
		c := &n.Nodes[i]
		switch c.XMLName.Local {
		case "transac":
			kind = c.text()
		case "date":
			when = parseDate(c.text())
		case "transacNote":
			who = c.text()
		}
	}
	switch kind {
	case "origination", "creation", "input":
		e.CreatedAt, e.CreatedBy = when, who
	case "modification":
		e.ChangedAt, e.ChangedBy = when, who
	}
}

func parseDate(s string) time.Time {
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04:05Z", "2006-01-02", "20060102T150405Z"} {
		if t, err := time.Parse(f, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// TBXOptions controls TBX output.
type TBXOptions struct {
	// Version is 3 (ISO 30042:2019, the default) or 2 (ISO 30042:2008).
	Version int
}

func tbxStatus(s string) string {
	switch s {
	case StatusApproved:
		return "preferredTerm-admn-sts"
	case StatusForbidden:
		return "deprecatedTerm-admn-sts"
	case StatusProposed:
		return "admittedTerm-admn-sts"
	}
	return ""
}

// WriteTBX writes the termbase as TBX-Basic. Fields with a TBX-Basic data
// category use it; others are written as notes "[Name] value", which
// ReadTBX turns back into fields.
func WriteTBX(tb *Termbase, opts TBXOptions) []byte {
	v3 := opts.Version != 2
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	srcLang := "en"
	if len(tb.Languages) > 0 {
		srcLang = tb.Languages[0]
	}
	title := tb.Name
	if title == "" {
		title = "Termbase"
	}
	if v3 {
		b.WriteString(`<?xml-model href="https://raw.githubusercontent.com/LTAC-Global/TBX-Basic_dialect/master/DCA/TBXcoreStructV03_TBX-Basic_integrated.rng" type="application/xml" schematypens="http://relaxng.org/ns/structure/1.0"?>` + "\n")
		b.WriteString(`<tbx type="TBX-Basic" style="dca" xml:lang="` + escAttr(srcLang) + `" xmlns="urn:iso:std:iso:30042:ed-2">` + "\n")
		b.WriteString("<tbxHeader><fileDesc><titleStmt><title>" + esc(title) + "</title></titleStmt><sourceDesc><p>Interlinea</p></sourceDesc></fileDesc></tbxHeader>\n<text><body>\n")
	} else {
		b.WriteString(`<martif type="TBX-Basic" xml:lang="` + escAttr(srcLang) + `">` + "\n")
		b.WriteString("<martifHeader><fileDesc><titleStmt><title>" + esc(title) + "</title></titleStmt><sourceDesc><p>Interlinea</p></sourceDesc></fileDesc></martifHeader>\n<text><body>\n")
	}
	entryEl, langEl, termEl := "conceptEntry", "langSec", "termSec"
	if !v3 {
		entryEl, langEl, termEl = "termEntry", "langSet", "tig"
	}
	// IDs must be unique XML names; entries without one get c1, c2 and so
	// on, skipping names other entries already use.
	used := map[string]bool{}
	for _, e := range tb.Entries {
		if validXMLID(e.ID) {
			used[e.ID] = true
		}
	}
	written := map[string]bool{}
	next := 1
	for _, e := range tb.Entries {
		id := e.ID
		if id == "" || !validXMLID(id) || written[id] {
			for used[fmt.Sprintf("c%d", next)] || written[fmt.Sprintf("c%d", next)] {
				next++
			}
			id = fmt.Sprintf("c%d", next)
			next++
		}
		written[id] = true
		b.WriteString("<" + entryEl + ` id="` + escAttr(id) + `">`)
		writeTBXValues(&b, e.Fields, "descrip")
		writeTBXTransac(&b, "origination", e.CreatedAt, e.CreatedBy)
		writeTBXTransac(&b, "modification", e.ChangedAt, e.ChangedBy)
		for _, l := range e.Languages {
			b.WriteString("<" + langEl + ` xml:lang="` + escAttr(l.Lang) + `">`)
			writeTBXValues(&b, l.Fields, "descrip")
			for _, t := range l.Terms {
				b.WriteString("<" + termEl + "><term>" + esc(t.Text) + "</term>")
				if s := tbxStatus(t.Status); s != "" {
					b.WriteString(`<termNote type="administrativeStatus">` + s + "</termNote>")
				}
				writeTBXValues(&b, t.Fields, "termNote")
				b.WriteString("</" + termEl + ">")
			}
			b.WriteString("</" + langEl + ">")
		}
		b.WriteString("</" + entryEl + ">\n")
	}
	if v3 {
		b.WriteString("</body></text>\n</tbx>\n")
	} else {
		b.WriteString("</body></text>\n</martif>\n")
	}
	return []byte(b.String())
}

// TBX-Basic orders elements: descrip and admin before notes. Values are
// written in that order, keeping their relative order otherwise.
func writeTBXValues(b *strings.Builder, vs []Value, _ string) {
	type out struct {
		rank int
		xml  string
	}
	var items []out
	for _, v := range vs {
		elem, typ, ok := tbxFromField(v.Field)
		switch {
		case !ok:
			items = append(items, out{3, "<note>" + esc("["+v.Field+"] "+v.Value) + "</note>"})
		case elem == "note":
			items = append(items, out{3, "<note>" + esc(v.Value) + "</note>"})
		default:
			rank := map[string]int{"termNote": 0, "descrip": 1, "admin": 2}[elem]
			items = append(items, out{rank, "<" + elem + ` type="` + typ + `">` + esc(v.Value) + "</" + elem + ">"})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].rank < items[j].rank })
	for _, it := range items {
		b.WriteString(it.xml)
	}
}

func writeTBXTransac(b *strings.Builder, kind string, when time.Time, who string) {
	if when.IsZero() && who == "" {
		return
	}
	b.WriteString(`<transacGrp><transac type="transactionType">` + kind + "</transac>")
	if !when.IsZero() {
		b.WriteString("<date>" + when.UTC().Format("2006-01-02") + "</date>")
	}
	if who != "" {
		b.WriteString(`<transacNote type="responsibility">` + esc(who) + "</transacNote>")
	}
	b.WriteString("</transacGrp>")
}

func validXMLID(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && (r == '-' || r == '.' || r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}
