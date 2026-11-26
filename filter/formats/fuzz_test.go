package formats_test

import (
	"testing"

	"github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/filter/formats/formatstest"
)

// fuzzFormats are the document formats the fuzzer reads a file as; its
// first input picks one.
var fuzzFormats = []string{formats.Word, formats.PowerPoint, formats.Excel, formats.HTML, formats.InDesign,
	formats.Properties, formats.JSON, formats.YAML, formats.PO, formats.RESX, formats.SubRip, formats.WebVTT,
	formats.XML, formats.OpenText, formats.OpenSheet, formats.OpenSlides, formats.Markdown, formats.Text,
	formats.YouTube, formats.SubStation, formats.MicroDVD, formats.TimedText, formats.EBUSTL}

// FuzzExtract reads documents in every format, and writes a document it
// reads back without translations.
func FuzzExtract(f *testing.F) {
	index := func(format string) uint8 {
		for i, x := range fuzzFormats {
			if x == format {
				return uint8(i)
			}
		}
		panic(format)
	}
	f.Add(index(formats.Word), docxtest.Document(docxtest.Parts{Body: docxtest.P("Title", docxtest.R("Mietvertrag")) + docxtest.P("", docxtest.R("Der "), docxtest.B("Mieter"), docxtest.R(" zahlt."), docxtest.FootnoteRef(1)), Footnotes: []string{docxtest.P("FootnoteText", docxtest.R("Monatlich."))}}))
	f.Add(index(formats.PowerPoint), formatstest.Deck())
	f.Add(index(formats.Excel), formatstest.Workbook())
	f.Add(index(formats.HTML), formatstest.Page())
	f.Add(index(formats.InDesign), formatstest.Layout())
	f.Add(index(formats.Markdown), []byte("# Mietvertrag\n\nDer **Mieter** zahlt [die Miete](https://example.com).\n\n- eins\n- zwei\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"))
	f.Add(index(formats.JSON), []byte(`{"title": "Mietvertrag", "items": ["Miete", {"note": "Kaution"}]}`))
	f.Add(index(formats.YAML), []byte("title: Mietvertrag\nitems:\n  - Miete\n  - note: Kaution\n"))
	f.Add(index(formats.PO), []byte("msgid \"\"\nmsgstr \"\"\n\"Language: de\\n\"\n\nmsgid \"Rent\"\nmsgstr \"Miete\"\n"))
	f.Add(index(formats.Properties), []byte("title=Mietvertrag\nrent = Miete \\\n  pro Monat\n"))
	f.Add(index(formats.SubRip), []byte("1\n00:00:01,000 --> 00:00:02,000\nDer Mieter zahlt.\n\n"))
	f.Add(index(formats.WebVTT), []byte("WEBVTT\n\n00:01.000 --> 00:02.000\nDer Mieter zahlt.\n"))
	f.Add(index(formats.RESX), []byte(`<?xml version="1.0"?><root><data name="t"><value>Miete</value></data></root>`))
	f.Add(index(formats.XML), []byte(`<?xml version="1.0"?><doc><p>Der <b>Mieter</b> zahlt.</p></doc>`))
	f.Add(index(formats.Text), []byte("Der Mieter zahlt die Miete. Er haftet.\n\nZweiter Absatz.\n"))
	f.Add(index(formats.YouTube), []byte("0:00:01.000,0:00:02.000\nDer Mieter zahlt.\n\n"))
	f.Add(index(formats.SubStation), []byte("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\i1}Der Mieter{\\i0}\\Nzahlt.\n"))
	f.Add(index(formats.MicroDVD), []byte("{1}{1}25\n{25}{50}{y:i}Der Mieter|zahlt.\n"))
	f.Add(index(formats.TimedText), []byte(`<tt xmlns="http://www.w3.org/ns/ttml" xml:lang="de"><body><div><p begin="1s" end="2s">Der <span>Mieter</span><br/>zahlt.</p></div></body></tt>`))
	f.Add(index(formats.EBUSTL), formatstest.Subtitles())
	f.Fuzz(func(t *testing.T, which uint8, data []byte) {
		format := fuzzFormats[int(which)%len(fuzzFormats)]
		if _, err := formats.Extract(format, data, "de-DE"); err == nil {
			formats.Merge(format, data, "de-DE", "en-GB", nil)
		}
	})
}
