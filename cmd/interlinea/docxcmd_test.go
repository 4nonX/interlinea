package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
)

func TestExtractAndMerge(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "lease.docx")
	if err := os.WriteFile(src, docxtest.Hofmann(), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := exec(t, "", "extract", "-lang", "de-DE", src)
	if code != 0 || !strings.Contains(out, "document/p2/s1\tZwischen {1}Herrn Jonas Hofmann{/1}") {
		t.Fatalf("extract: %d %q %q", code, out, errOut)
	}
	code, out, _ = exec(t, "", "extract", "-lang", "de-DE", "-json", src)
	var ex filter.Extraction
	if code != 0 || json.Unmarshal([]byte(out), &ex) != nil || len(ex.Units) != 38 {
		t.Fatalf("extract -json: %d units", len(ex.Units))
	}
	trFile := filepath.Join(dir, "tr.json")
	os.WriteFile(trFile, []byte(`{"document/p1/s1":"Residential lease agreement","document/p20/s1":[{"text":"The right to reduce the rent"},{"tag":{"kind":"standalone","id":"1"}},{"text":" remains unaffected."}]}`), 0o644)
	dst := filepath.Join(dir, "out.docx")
	code, out, errOut = exec(t, "", "merge", "-lang", "de-DE", "-target", "en-GB", "-o", dst, src, trFile)
	if code != 0 || !strings.Contains(out, "2 translated units") {
		t.Fatalf("merge: %d %q %q", code, out, errOut)
	}
	data, _ := os.ReadFile(dst)
	got, err := docx.Extract(data, "en-GB", docx.Options{})
	if err != nil || got.Units[0].Source.Plain() != "Residential lease agreement" {
		t.Fatalf("merged document: %v %v", err, got)
	}
	if code, _, _ := exec(t, "", "merge", "-lang", "de-DE", src, trFile); code != 2 {
		t.Error("merge without -o and -target should be a usage error")
	}
	os.WriteFile(trFile, []byte(`{"x":[{"tag":{"kind":"bogus","id":"1"}}]}`), 0o644)
	if code, _, _ := exec(t, "", "merge", "-lang", "de-DE", "-target", "en", "-o", dst, src, trFile); code != 1 {
		t.Error("invalid translations should fail")
	}
}

// extract and merge read every document format, by the file's name.
func TestExtractAndMergeOtherFormats(t *testing.T) {
	dir := t.TempDir()
	deck := filepath.Join(dir, "Folien.pptx")
	page := filepath.Join(dir, "Seite.html")
	os.WriteFile(deck, docxtest.Deck(), 0o644)
	os.WriteFile(page, []byte(`<p>Die Kaution ist <b>fällig</b>.</p>`), 0o644)
	code, out, errOut := exec(t, "", "extract", "-lang", "de-DE", deck)
	if code != 0 || !strings.Contains(out, "slide2/p1/s1\tMietrecht 2027") || !strings.Contains(out, "notesSlide1/p1/s1\tAuf die Tabelle hinweisen.") {
		t.Fatalf("extract pptx: %d %q %q", code, out, errOut)
	}
	code, out, errOut = exec(t, "", "extract", "-lang", "de-DE", page)
	if code != 0 || out != "b1/s1\tDie Kaution ist {1}fällig{/1}.\n" {
		t.Fatalf("extract html: %d %q %q", code, out, errOut)
	}
	trFile := filepath.Join(dir, "tr.json")
	os.WriteFile(trFile, []byte(`{"b1/s1":[{"text":"The deposit is "},{"tag":{"kind":"open","id":"1"}},{"text":"due"},{"tag":{"kind":"close","id":"1"}},{"text":"."}]}`), 0o644)
	dst := filepath.Join(dir, "page.html")
	if code, out, errOut = exec(t, "", "merge", "-lang", "de-DE", "-target", "en-GB", "-o", dst, page, trFile); code != 0 {
		t.Fatalf("merge html: %d %q %q", code, out, errOut)
	}
	if got, _ := os.ReadFile(dst); string(got) != `<p>The deposit is <b>due</b>.</p>` {
		t.Errorf("merged page: %s", got)
	}
	other := filepath.Join(dir, "Plan.pdf")
	os.WriteFile(other, []byte("%PDF"), 0o644)
	if code, _, errOut = exec(t, "", "extract", other); code == 0 || !strings.Contains(errOut, "is not a Word, PowerPoint, Excel, HTML or InDesign IDML file") {
		t.Errorf("extract pdf: %d %q", code, errOut)
	}
}
