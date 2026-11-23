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
