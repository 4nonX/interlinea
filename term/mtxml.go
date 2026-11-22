package term

import (
	"fmt"
	"strconv"
	"strings"
)

// ReadMultiTerm reads MultiTerm XML (<mtf>, as MultiTerm exports it, usually
// UTF-16). A descriptive field named Status sets the term status.
func ReadMultiTerm(data []byte) (*Termbase, Report, error) {
	var rep Report
	var entries []Entry
	root, err := eachElement(data, map[string]bool{"conceptGrp": true}, func(n *xnode) error {
		e := Entry{}
		for i := range n.Nodes {
			c := &n.Nodes[i]
			switch c.XMLName.Local {
			case "concept":
				e.ID = c.text()
			case "transacGrp":
				mtTransac(c, &e)
			case "descripGrp":
				for _, v := range mtValues(c) {
					if v.Field == "Entry ID" {
						e.ID = v.Value
						continue
					}
					e.Fields = append(e.Fields, v)
				}
			case "languageGrp":
				l := Language{}
				for j := range c.Nodes {
					cc := &c.Nodes[j]
					switch cc.XMLName.Local {
					case "language":
						l.Lang = mtLang(cc.attr("lang"), cc.attr("type"))
					case "descripGrp":
						l.Fields = append(l.Fields, mtValues(cc)...)
					case "termGrp":
						t := Term{}
						for k := range cc.Nodes {
							x := &cc.Nodes[k]
							switch x.XMLName.Local {
							case "term":
								t.Text = x.text()
							case "descripGrp":
								for _, v := range mtValues(x) {
									if strings.EqualFold(v.Field, "Status") || strings.EqualFold(v.Field, "Term status") {
										t.Status = NormalizeStatus(v.Value)
										continue
									}
									t.Fields = append(t.Fields, v)
								}
							}
						}
						l.Terms = append(l.Terms, t)
					}
				}
				e.Languages = append(e.Languages, l)
			}
		}
		if e.Clean() {
			entries = append(entries, e)
			rep.Entries++
			rep.Terms += e.TermCount()
		} else {
			rep.Skipped++
			rep.warn("Entry %q has no terms and was skipped.", e.ID)
		}
		return nil
	})
	if err != nil {
		return nil, rep, fmt.Errorf("multiterm: %w", err)
	}
	if root != "mtf" {
		return nil, rep, fmt.Errorf("multiterm: root element <%s> is not <mtf>", root)
	}
	return &Termbase{Definition: InferDefinition("", entries), Entries: entries}, rep, nil
}

// mtLang turns MultiTerm's lang ("DE", "EN-GB") or language name into a tag.
func mtLang(code, name string) string {
	if code != "" {
		if c, ok := LangFromName(code); ok {
			return c
		}
		return NormalizeLang(code)
	}
	if c, ok := LangFromName(name); ok {
		return c
	}
	return name
}

// mtValues flattens a descripGrp, including nested groups (sub-fields).
func mtValues(n *xnode) []Value {
	var out []Value
	for i := range n.Nodes {
		c := &n.Nodes[i]
		switch c.XMLName.Local {
		case "descrip":
			out = append(out, Value{Field: c.attr("type"), Value: c.text()})
		case "descripGrp":
			out = append(out, mtValues(c)...)
		case "xref":
			out = append(out, Value{Field: "Cross reference", Value: c.text()})
		}
	}
	return out
}

func mtTransac(n *xnode, e *Entry) {
	var kind, who string
	for i := range n.Nodes {
		c := &n.Nodes[i]
		switch c.XMLName.Local {
		case "transac":
			kind, who = c.attr("type"), c.text()
		case "date":
			d := parseDate(c.text())
			switch kind {
			case "origination":
				e.CreatedAt = d
			case "modification":
				e.ChangedAt = d
			}
		}
	}
	switch kind {
	case "origination":
		e.CreatedBy = who
	case "modification":
		e.ChangedBy = who
	}
}

// WriteMultiTerm writes MultiTerm XML in UTF-16, as MultiTerm itself does.
// Term status is written as a descriptive field named Status.
func WriteMultiTerm(tb *Termbase) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-16"?>` + "\n<mtf>\n")
	for i, e := range tb.Entries {
		b.WriteString("<conceptGrp>")
		id := strconv.Itoa(i + 1)
		if _, err := strconv.Atoi(e.ID); err == nil {
			id = e.ID
		}
		b.WriteString("<concept>" + esc(id) + "</concept>")
		mtWriteTransac(&b, "origination", e.CreatedBy, e.CreatedAt.UTC().Format("2006-01-02T15:04:05"), !e.CreatedAt.IsZero())
		mtWriteTransac(&b, "modification", e.ChangedBy, e.ChangedAt.UTC().Format("2006-01-02T15:04:05"), !e.ChangedAt.IsZero())
		if e.ID != "" && e.ID != id {
			b.WriteString(`<descripGrp><descrip type="Entry ID">` + esc(e.ID) + "</descrip></descripGrp>")
		}
		mtWriteValues(&b, e.Fields)
		for _, l := range e.Languages {
			b.WriteString("<languageGrp>")
			b.WriteString(`<language type="` + escAttr(LangName(l.Lang)) + `" lang="` + escAttr(strings.ToUpper(l.Lang)) + `"/>`)
			mtWriteValues(&b, l.Fields)
			for _, t := range l.Terms {
				b.WriteString("<termGrp><term>" + esc(t.Text) + "</term>")
				if t.Status != "" {
					b.WriteString(`<descripGrp><descrip type="Status">` + esc(t.Status) + "</descrip></descripGrp>")
				}
				mtWriteValues(&b, t.Fields)
				b.WriteString("</termGrp>")
			}
			b.WriteString("</languageGrp>")
		}
		b.WriteString("</conceptGrp>\n")
	}
	b.WriteString("</mtf>\n")
	return utf16LE(b.String())
}

func mtWriteTransac(b *strings.Builder, kind, who, date string, hasDate bool) {
	if who == "" && !hasDate {
		return
	}
	b.WriteString(`<transacGrp><transac type="` + kind + `">` + esc(who) + "</transac>")
	if hasDate {
		b.WriteString("<date>" + date + "</date>")
	}
	b.WriteString("</transacGrp>")
}

func mtWriteValues(b *strings.Builder, vs []Value) {
	for _, v := range vs {
		b.WriteString(`<descripGrp><descrip type="` + escAttr(v.Field) + `">` + esc(v.Value) + "</descrip></descripGrp>")
	}
}

// ReadXDT reads the languages and name from a MultiTerm termbase
// definition (.xdt). Field definitions vary between MultiTerm versions and
// are taken from the entries instead.
func ReadXDT(data []byte) (Definition, error) {
	var d Definition
	var texts, locales []string
	root, err := eachElement(data, map[string]bool{"Name": true, "Languages": true}, func(n *xnode) error {
		switch n.XMLName.Local {
		case "Name":
			if d.Name == "" {
				d.Name = n.text()
			}
		case "Languages":
			var walk func(*xnode)
			walk = func(x *xnode) {
				for i := range x.Nodes {
					c := &x.Nodes[i]
					switch c.XMLName.Local {
					case "ItemText":
						texts = append(texts, c.text())
					case "ItemLocale":
						locales = append(locales, c.text())
					default:
						walk(c)
					}
				}
			}
			walk(n)
		}
		return nil
	})
	if err != nil {
		return d, fmt.Errorf("xdt: %w", err)
	}
	if root != "TermbaseDefinition" && root != "termbaseDefinition" {
		return d, fmt.Errorf("xdt: root element <%s> is not a termbase definition", root)
	}
	for i, loc := range locales {
		name := ""
		if i < len(texts) {
			name = texts[i]
		}
		d.Languages = append(d.Languages, mtLang(loc, name))
	}
	return d, nil
}
