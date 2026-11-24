package server

import (
	"strings"
	"testing"

	"github.com/4nonX/interlinea/filter/docx/docxtest"
)

func TestInspect(t *testing.T) {
	te := newTeam(t)
	german := "Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats auf das Konto des Vermieters. Die Kaution beträgt drei Monatsmieten und wird bei Beginn des Mietverhältnisses fällig. Der Vermieter hat die Kaution von seinem Vermögen getrennt anzulegen, und die Zinsen stehen dem Mieter zu."
	lease := docxtest.Document(docxtest.Parts{Lang: "de-DE", Body: docxtest.P("", docxtest.R(german))})
	var got inspection
	te.call("POST", "/api/files/inspect?name=Mietvertrag.docx", te.ben, lease, 200, &got, "Content-Type", "application/octet-stream")
	if got.Format != "docx" || !got.Translatable || got.Segments != 3 || got.Words < 40 || got.Detection == nil || got.Detection.Lang != "de-DE" {
		t.Fatalf("Word document: %+v %+v", got, got.Detection)
	}
	te.call("POST", "/api/files/inspect?name=Brief.sdlxliff", te.ben, tradosFile(t), 200, &got, "Content-Type", "application/octet-stream")
	if got.Format != "xliff" || got.Bilingual == nil || got.Bilingual.Format != "sdlxliff" || got.Bilingual.TargetLang != "en-GB" || got.Bilingual.Translated == 0 {
		t.Fatalf("bilingual: %+v %+v", got, got.Bilingual)
	}
	te.call("POST", "/api/files/inspect?name=Hofmann.sdlppx", te.ben, tradosPackage(t, tradosFile(t)), 200, &got, "Content-Type", "application/octet-stream")
	if got.Format != "package" || got.Package == nil || len(got.Package.Files) != 1 || got.Package.Files[0].Lang != "en-GB" || got.Package.SourceLang != "de-DE" {
		t.Fatalf("package: %+v %+v", got, got.Package)
	}
	te.call("POST", "/api/files/inspect?name=Hinweise.txt", te.ben, []byte(german), 200, &got, "Content-Type", "application/octet-stream")
	if got.Format != "txt" || got.Segments != 3 || got.Detection.Lang != "de-DE" {
		t.Fatalf("text: %+v", got)
	}
	// The formats of software, subtitles, XML, Markdown and OpenDocument are read by their filters.
	for name, body := range map[string]string{
		"app.properties": "greeting=Guten Tag\nbye = Auf Wiedersehen\n",
		"app.json":       `{"greeting": "Guten Tag", "bye": "Auf Wiedersehen", "n": 3}`,
		"app.yml":        "de:\n  greeting: Guten Tag\n  bye: Auf Wiedersehen\n",
		"app.po":         "msgid \"Guten Tag\"\nmsgstr \"\"\n\nmsgid \"Auf Wiedersehen\"\nmsgstr \"\"\n",
		"App.resx":       `<root><data name="a"><value>Guten Tag</value></data><data name="b"><value>Auf Wiedersehen</value></data></root>`,
		"film.srt":       "1\n00:00:01,000 --> 00:00:02,000\nGuten Tag\n\n2\n00:00:03,000 --> 00:00:04,000\nAuf Wiedersehen\n",
		"film.vtt":       "WEBVTT\n\n00:01.000 --> 00:02.000\nGuten Tag\n\n00:03.000 --> 00:04.000\nAuf Wiedersehen\n",
		"data.xml":       `<doc><p>Guten Tag</p><p>Auf Wiedersehen</p></doc>`,
		"README.md":      "# Guten Tag\n\nAuf Wiedersehen\n",
	} {
		te.call("POST", "/api/files/inspect?name="+name, te.ben, []byte(body), 200, &got, "Content-Type", "application/octet-stream")
		if !got.Translatable || got.Segments != 2 || got.Words != 4 || got.Problem != "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	te.call("POST", "/api/files/inspect?name=broken.json", te.ben, []byte(`{"a": `), 200, &got, "Content-Type", "application/octet-stream")
	if got.Translatable || !strings.Contains(got.Problem, "not valid JSON") {
		t.Errorf("broken JSON: %+v", got)
	}
	te.call("POST", "/api/files/inspect?name=Plan.pdf", te.ben, []byte("%PDF-1.7"), 200, &got, "Content-Type", "application/octet-stream")
	if got.Format != "other" || got.Translatable {
		t.Fatalf("reference file: %+v", got)
	}
	te.call("POST", "/api/files/inspect?name=Kaputt.docx", te.ben, []byte("not a zip"), 200, &got, "Content-Type", "application/octet-stream")
	if got.Translatable || !strings.Contains(got.Problem, "could not be read") {
		t.Fatalf("broken file: %+v", got)
	}
	if r := te.do("POST", "/api/files/inspect", te.ben, []byte("x"), "Content-Type", "application/octet-stream"); r.status != 422 {
		t.Fatalf("no name: %d", r.status)
	}
	if r := te.do("POST", "/api/files/inspect?name=a.txt", "", []byte("x")); r.status != 401 {
		t.Fatalf("not signed in: %d", r.status)
	}
}
