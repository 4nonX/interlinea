package glossary

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/filter/pdf"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/typeset"
	"github.com/4nonX/interlinea/typeset/fonts"
)

var made = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func entry(id string, fields []term.Value, langs ...term.Language) term.Entry {
	return term.Entry{ID: id, Fields: fields, Languages: langs, ChangedAt: made}
}

func lang(code string, terms ...term.Term) term.Language {
	return term.Language{Lang: code, Terms: terms}
}

func sample() *term.Termbase {
	return &term.Termbase{
		Definition: term.Definition{Name: "Kanzlei Weber", Languages: []string{"de-DE", "en-GB", "ar"}, Fields: []term.Field{
			{Name: "Subject", Level: term.LevelEntry, Kind: term.KindText},
			{Name: "Checked", Level: term.LevelEntry, Kind: term.KindBoolean},
			{Name: "Gender", Level: term.LevelTerm, Kind: term.KindPicklist, Values: []string{"f", "m", "n"}},
		}},
		Entries: []term.Entry{
			entry("1", []term.Value{{Field: "Subject", Value: "Tenancy"}, {Field: "Checked", Value: "true"}},
				lang("de-DE", term.Term{Text: "Kaution", Status: "approved", Fields: []term.Value{{Field: "Gender", Value: "f"}}}, term.Term{Text: "Mietsicherheit", Status: "proposed"}),
				lang("en-GB", term.Term{Text: "security deposit"}, term.Term{Text: "deposit", Status: "forbidden"}),
				lang("ar", term.Term{Text: "وديعة"})),
			entry("2", []term.Value{{Field: "Subject", Value: "Courts"}},
				lang("de-DE", term.Term{Text: "Ärger"}), lang("en-GB", term.Term{Text: "trouble"})),
			entry("3", nil, lang("de-DE", term.Term{Text: "Abmahnung"}), lang("en-GB", term.Term{Text: "warning letter"})),
			entry("4", nil, lang("de-DE", term.Term{Text: "Bank"}), lang("en-GB", term.Term{Text: "bench"})),
			entry("5", nil, lang("de-DE", term.Term{Text: "Bank"}), lang("en-GB", term.Term{Text: "bank"})),
			entry("6", nil, lang("en-GB", term.Term{Text: "only English"})),
		},
	}
}

func headwords(g *Glossary) (out []string) {
	for _, s := range g.Sections {
		for _, a := range s.Articles {
			out = append(out, a.Headword)
		}
	}
	return out
}

func TestBuildSortsAndGroups(t *testing.T) {
	tb := sample()
	g := Build(tb, term.ExportOptions{Format: "pdf", Letters: true})
	if got := strings.Join(headwords(g), "|"); got != "Abmahnung|Ärger|Bank|Bank|Kaution" {
		t.Fatalf("headwords in German order: %s", got)
	}
	var letters []string
	for _, s := range g.Sections {
		letters = append(letters, s.Letter)
	}
	if strings.Join(letters, "") != "ABK" {
		t.Errorf("Ä goes under A in German: %v", letters)
	}
	if g.WithoutHeadword != 1 || g.Entries != 5 {
		t.Errorf("an entry without German is left out: %d, %d", g.WithoutHeadword, g.Entries)
	}
	g = Build(tb, term.ExportOptions{Format: "pdf", GroupHomonyms: true, PerTerm: true})
	if got := strings.Join(headwords(g), "|"); got != "Abmahnung|Ärger|Bank|Kaution|Mietsicherheit" {
		t.Errorf("synonyms are headwords, homonyms one: %s", got)
	}
	for _, a := range g.Sections[0].Articles {
		if a.Headword == "Bank" && len(a.Entries) != 2 {
			t.Errorf("both Bank entries under one headword: %d", len(a.Entries))
		}
	}
	g = Build(tb, term.ExportOptions{Format: "pdf", SortBy: "en-GB", Languages: []string{"de-DE", "en-GB"}})
	if g.Languages[0] != "en-GB" || headwords(g)[0] != "bank" {
		t.Errorf("sorted by English, which goes first: %v %v", g.Languages, headwords(g))
	}
}

func TestGrammar(t *testing.T) {
	g := &Glossary{TermFields: []string{"Part of speech", "Gender", "Number", "Context"}}
	cases := []struct {
		vs   []term.Value
		want string
	}{
		{[]term.Value{{Field: "Gender", Value: "feminine"}, {Field: "Part of speech", Value: "noun"}}, "f"},
		{[]term.Value{{Field: "Part of speech", Value: "verb"}}, "v"},
		{[]term.Value{{Field: "Gender", Value: "n"}, {Field: "Number", Value: "plural"}}, "nt pl"},
		{[]term.Value{{Field: "Part of speech", Value: "Adjektiv"}}, "adj"},
		{[]term.Value{{Field: "Part of speech", Value: "idiom"}}, "idiom"},
	}
	for _, c := range cases {
		if got, _ := g.grammar(c.vs); got != c.want {
			t.Errorf("%v: %q, want %q", c.vs, got, c.want)
		}
	}
	tb := sample()
	tb.Entries[0].Languages[1].Terms[0].Fields = []term.Value{{Field: "Gender", Value: "noun"}}
	gl := Build(tb, term.ExportOptions{Format: "pdf"})
	if l := gl.Legend(); l != "Abbreviations: f feminine" {
		t.Errorf("legend: %q", l)
	}
}

func TestSelect(t *testing.T) {
	sel := term.Select(sample(), term.ExportOptions{Statuses: []string{"approved"}, Fields: []string{"Subject"}, FilterField: "Subject", FilterValue: "tenancy"})
	if len(sel.Entries) != 1 {
		t.Fatalf("the filter keeps one entry: %d", len(sel.Entries))
	}
	e := sel.Entries[0]
	if len(e.Fields) != 1 || e.Fields[0].Field != "Subject" {
		t.Errorf("only the field asked for: %v", e.Fields)
	}
	for _, l := range e.Languages {
		for _, tm := range l.Terms {
			if tm.Status != "approved" && tm.Status != "" {
				t.Errorf("only approved terms: %v", tm)
			}
			if len(tm.Fields) != 0 {
				t.Errorf("term fields left out: %v", tm.Fields)
			}
		}
	}
}

func TestBlocks(t *testing.T) {
	g := Build(sample(), term.ExportOptions{Format: "txt", Labels: true, Status: true})
	var kaution Article
	for _, a := range g.Sections[0].Articles {
		if a.Headword == "Kaution" {
			kaution = a
		}
	}
	bs := g.Blocks(kaution)
	head := JoinRuns(bs[0].Runs)
	for _, want := range []string{"Kaution f", "Mietsicherheit proposed", "Subject Tenancy", "Checked yes"} {
		if !strings.Contains(head, want) {
			t.Errorf("headword paragraph lacks %q: %s", want, head)
		}
	}
	if len(bs) != 3 || !strings.Contains(JoinRuns(bs[1].Runs), "deposit forbidden") || !bs[2].RTL {
		t.Errorf("a line each for English and Arabic, right to left: %#v", bs)
	}
}

func fontSet(t *testing.T) *typeset.FontSet {
	fs, err := typeset.NewFontSet(fonts.All(), false)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestPDF(t *testing.T) {
	for _, layout := range []string{"dictionary", "table"} {
		r, err := Export(sample(), term.ExportOptions{Format: "pdf", Layout: layout, Letters: true, Labels: true, Status: true}, fontSet(t), made)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Missing) != 0 {
			t.Errorf("%s: characters without a font: %q", layout, string(r.Missing))
		}
		doc, err := pdf.Read(r.Data)
		if err != nil {
			t.Fatalf("%s: our own reader reads it: %v", layout, err)
		}
		var text strings.Builder
		for _, p := range doc.Paragraphs {
			text.WriteString(p.Text + "\n")
		}
		s := text.String()
		for _, want := range []string{"Kanzlei Weber", "Abmahnung", "Ärger", "security deposit", "Mietsicherheit", "Kaution"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: the PDF's text lacks %q:\n%s", layout, want, s)
			}
		}
		if !bytes.Contains(r.Data, []byte("/FontFile2")) || !bytes.Contains(r.Data, []byte("/ToUnicode")) {
			t.Errorf("%s: fonts embedded with their text", layout)
		}
	}
}

func TestPDFManyPages(t *testing.T) {
	tb := &term.Termbase{Definition: term.Definition{Name: "Long", Languages: []string{"en-GB", "de-DE"}}}
	for i := 0; i < 400; i++ {
		w := string(rune('a'+i%26)) + strings.Repeat("x", i%7) + string(rune('a'+(i/26)%26))
		tb.Entries = append(tb.Entries, entry(w, nil, lang("en-GB", term.Term{Text: w}), lang("de-DE", term.Term{Text: strings.ToUpper(w) + " " + strings.Repeat("lange Erklärung ", i%9)})))
	}
	for _, layout := range []string{"dictionary", "table"} {
		r, err := Export(tb, term.ExportOptions{Format: "pdf", Layout: layout, Letters: true}, fontSet(t), made)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := pdf.Read(r.Data)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Pages < 3 {
			t.Errorf("%s: 400 entries take pages: %d", layout, doc.Pages)
		}
	}
}

func TestWord(t *testing.T) {
	for _, layout := range []string{"dictionary", "table"} {
		r, err := Export(sample(), term.ExportOptions{Format: "docx", Layout: layout, Letters: true, Labels: true, Status: true}, nil, made)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(r.Data), int64(len(r.Data)))
		if err != nil {
			t.Fatal(err)
		}
		var doc string
		for _, f := range zr.File {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			d := xml.NewDecoder(bytes.NewReader(b))
			for {
				if _, err := d.Token(); err != nil {
					if err != io.EOF {
						t.Errorf("%s %s is not well-formed: %v", layout, f.Name, err)
					}
					break
				}
			}
			if f.Name == "word/document.xml" {
				doc = string(b)
			}
			if f.Name == "word/header1.xml" && !strings.Contains(string(b), `STYLEREF "Headword" \l`) {
				t.Errorf("the running head names the last headword of the page")
			}
		}
		for _, want := range []string{"Kaution", "Ärger", `w:rStyle w:val="Headword"`, "<w:bidi/>"} {
			if !strings.Contains(doc, want) {
				t.Errorf("%s: the document lacks %q", layout, want)
			}
		}
	}
}

func TestHTMLAndText(t *testing.T) {
	r, err := Export(sample(), term.ExportOptions{Format: "html", Letters: true, Status: true}, nil, made)
	if err != nil {
		t.Fatal(err)
	}
	h := string(r.Data)
	for _, want := range []string{`<html lang="de-DE">`, `href="#l0"`, "<b>Kaution</b>", `dir="rtl"`, "<s>deposit</s>", `id="find"`} {
		if !strings.Contains(h, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	r, err = Export(sample(), term.ExportOptions{Format: "txt", Letters: true, Labels: true}, nil, made)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Data), "Kaution f, Mietsicherheit   Subject Tenancy  Checked yes\n    EN-GB  security deposit; deposit") {
		t.Errorf("plain text:\n%s", r.Data)
	}
}

func TestExchangeTakesOptions(t *testing.T) {
	r, err := Export(sample(), term.ExportOptions{Format: "csv", Languages: []string{"de-DE", "en-GB"}, Fields: []string{}}, nil, made)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(r.Data), "Subject") || strings.Contains(string(r.Data), "وديعة") {
		t.Errorf("fields and languages left out:\n%s", r.Data)
	}
	if _, err := Export(sample(), term.ExportOptions{Format: "pdf", SortBy: "fr-FR"}, nil, made); err == nil || !strings.Contains(err.Error(), "fr-FR") {
		t.Errorf("a language the termbase lacks is refused: %v", err)
	}
}

// TestSamplePages writes sample glossaries to look at, when asked to by
// INTERLINEA_GLOSSARY_OUT (a folder).
func TestSamplePages(t *testing.T) {
	dir := os.Getenv("INTERLINEA_GLOSSARY_OUT")
	if dir == "" {
		t.Skip("set INTERLINEA_GLOSSARY_OUT to write sample glossaries")
	}
	tb := &term.Termbase{Definition: term.Definition{Name: "Kanzlei Weber: Mietrecht", Languages: []string{"de-DE", "en-GB", "fr-FR"}, Fields: []term.Field{
		{Name: "Subject", Level: term.LevelEntry}, {Name: "Definition", Level: term.LevelLanguage}, {Name: "Part of speech", Level: term.LevelTerm}, {Name: "Gender", Level: term.LevelTerm}, {Name: "Number", Level: term.LevelTerm}}}}
	words := [][]string{
		{"Abmahnung", "f", "warning letter", "lettre d'avertissement", "Aufforderung, ein vertragswidriges Verhalten zu unterlassen."},
		{"Abnahme", "f", "acceptance", "réception", ""},
		{"Anfechtung", "f", "avoidance", "annulation", "Beseitigung einer Willenserklärung wegen eines Willensmangels."},
		{"Ärger", "m", "trouble", "ennui", ""},
		{"Auflassung", "f", "conveyance", "accord de transfert", "Einigung über den Übergang des Eigentums an einem Grundstück."},
		{"Bürge", "m", "guarantor", "caution", ""},
		{"Bürgschaft", "f", "guarantee", "cautionnement", "Vertrag, mit dem sich der Bürge verpflichtet, für die Verbindlichkeit eines Dritten einzustehen."},
		{"Eigenbedarf", "m", "personal use", "besoin personnel", "Bedarf des Vermieters an der Wohnung für sich oder Angehörige."},
		{"Erbbaurecht", "n", "heritable building right", "droit de superficie", ""},
		{"Gewährleistung", "f", "warranty", "garantie", ""},
		{"Hausordnung", "f", "house rules", "règlement intérieur", ""},
		{"Kaution", "f", "security deposit", "dépôt de garantie", "Sicherheitsleistung des Mieters, höchstens drei Monatsmieten."},
		{"Kündigung", "f", "notice of termination", "résiliation", "Einseitige Erklärung, die ein Dauerschuldverhältnis beendet."},
		{"kündigen", "verb", "terminate", "résilier", ""},
		{"mietfrei", "adjective", "rent-free", "exempt de loyer", ""},
		{"mieten", "verb", "rent", "louer", ""},
		{"Mangel", "m", "defect", "vice", ""},
		{"Mietminderung", "f", "rent reduction", "diminution du loyer", "Kürzung der Miete, solange die Mietsache einen Mangel hat."},
		{"Mietsache", "f", "leased property", "chose louée", ""},
		{"Nebenkosten", "pl", "service charges", "charges locatives", "Betriebskosten, die der Mieter neben der Grundmiete trägt."},
		{"Räumung", "f", "eviction", "expulsion", ""},
		{"Schönheitsreparaturen", "pl", "cosmetic repairs", "travaux d'embellissement", ""},
		{"Staffelmiete", "f", "graduated rent", "loyer progressif", "Miete, die sich in festgelegten Stufen erhöht."},
		{"Untermiete", "f", "subletting", "sous-location", ""},
		{"Vermieter", "m", "landlord", "bailleur", ""},
		{"Verjährung", "f", "limitation", "prescription", "Ablauf der Frist, nach der ein Anspruch nicht mehr durchgesetzt werden kann."},
		{"Wohnfläche", "f", "living space", "surface habitable", ""},
		{"Zwangsvollstreckung", "f", "enforcement", "exécution forcée", ""},
	}
	for i, w := range words {
		gram := []term.Value{{Field: "Part of speech", Value: "noun"}, {Field: "Gender", Value: w[1]}}
		switch w[1] {
		case "verb", "adjective":
			gram = []term.Value{{Field: "Part of speech", Value: w[1]}}
		case "pl":
			gram = []term.Value{{Field: "Part of speech", Value: "noun"}, {Field: "Number", Value: "plural"}}
		}
		de := term.Language{Lang: "de-DE", Terms: []term.Term{{Text: w[0], Fields: gram}}}
		if w[4] != "" {
			de.Fields = []term.Value{{Field: "Definition", Value: w[4]}}
		}
		if w[0] == "Kaution" {
			de.Terms = append(de.Terms, term.Term{Text: "Mietsicherheit", Status: "proposed"})
		}
		enGram := []term.Value{{Field: "Part of speech", Value: "noun"}}
		if w[1] == "verb" || w[1] == "adjective" {
			enGram = []term.Value{{Field: "Part of speech", Value: w[1]}}
		}
		en := term.Language{Lang: "en-GB", Terms: []term.Term{{Text: w[2], Fields: enGram}}}
		if w[0] == "Kaution" {
			en.Terms = append(en.Terms, term.Term{Text: "deposit", Status: "forbidden"})
		}
		tb.Entries = append(tb.Entries, entry(strconv.Itoa(i), []term.Value{{Field: "Subject", Value: "Tenancy law"}}, de, en, term.Language{Lang: "fr-FR", Terms: []term.Term{{Text: w[3]}}}))
	}
	for _, x := range []struct{ name, format, layout string }{{"dictionary.pdf", "pdf", "dictionary"}, {"table.pdf", "pdf", "table"}, {"dictionary.docx", "docx", "dictionary"}, {"dictionary.html", "html", "dictionary"}} {
		r, err := Export(tb, term.ExportOptions{Format: x.format, Layout: x.layout, Letters: true, Status: true, Labels: false, Fields: []string{"Definition", "Gender", "Part of speech", "Number"}}, fontSet(t), made)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, x.name), r.Data, 0o644)
	}
}
