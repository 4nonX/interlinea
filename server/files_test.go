package server

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/project"
)

// segmentsOf are the segments an app sends for an extraction.
func segmentsOf(ex *filter.Extraction) []project.Segment {
	out := make([]project.Segment, len(ex.Units))
	for i, u := range ex.Units {
		out[i] = project.Segment{ID: u.ID, Source: u.Source, Target: u.Target, Status: project.StatusNew}
	}
	return out
}

func wantProblem(t *testing.T, err error, part string) {
	t.Helper()
	var fp *fileProblem
	if !errors.As(err, &fp) || !strings.Contains(fp.msg, part) {
		t.Fatalf("got %v, want a problem with %q", err, part)
	}
}

func TestCheckDocx(t *testing.T) {
	data := docxtest.Document(docxtest.Parts{Lang: "de-DE", Body: docxtest.P("", docxtest.R("Der Mieter zahlt "), docxtest.B("pünktlich"), docxtest.R(" die Miete. Die Kaution beträgt drei Monatsmieten.")) +
		docxtest.P("", docxtest.R("Der Vermieter haftet nicht."))})
	ex, err := docx.Extract(data, "de-DE", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 3 {
		t.Fatalf("units %+v", ex.Units)
	}
	up := func(segs []project.Segment) fileUpload {
		return fileUpload{Name: "Mietvertrag.docx", Role: project.Translatable, Format: "docx", Data: data, Segments: map[string][]project.Segment{"en-GB": segs}}
	}

	// The file's own segments pass, with a translation kept.
	honest := segmentsOf(ex)
	honest[0].Target = content.Text("The tenant pays the rent on time.")
	f := up(honest)
	if err := checkFile(f, "de-DE"); err != nil {
		t.Fatal(err)
	}
	if f.Segments["en-GB"][0].Target.Plain() != "The tenant pays the rent on time." {
		t.Fatalf("translation lost: %+v", f.Segments["en-GB"][0])
	}

	// Sources without their tags get the file's back.
	plain := segmentsOf(ex)
	plain[0].Source = content.Text(ex.Units[0].Source.Plain())
	f = up(plain)
	if err := checkFile(f, "de-DE"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Segments["en-GB"][0].Source, ex.Units[0].Source) {
		t.Fatalf("source %+v, want the file's %+v", f.Segments["en-GB"][0].Source, ex.Units[0].Source)
	}

	// Text the file doesn't have is refused.
	forged := segmentsOf(ex)
	forged[2].Source = content.Text("Der Vermieter haftet für alles.")
	wantProblem(t, checkFile(up(forged), "de-DE"), "Segment 3 of Mietvertrag.docx (en-GB) is not the file's")

	// So are missing, extra and renamed segments.
	wantProblem(t, checkFile(up(segmentsOf(ex)[:2]), "de-DE"), "has 3 segments, not 2")
	wantProblem(t, checkFile(up(append(segmentsOf(ex), project.Segment{ID: "x", Source: content.Text("Mehr.")})), "de-DE"), "has 3 segments, not 4")
	renamed := segmentsOf(ex)
	renamed[1].ID = "document/p9/s1"
	wantProblem(t, checkFile(up(renamed), "de-DE"), "Segment 2")

	// And a file that is not a Word document, or not sent as one.
	bad := up(segmentsOf(ex))
	bad.Data = []byte("PK not really")
	wantProblem(t, checkFile(bad, "de-DE"), "could not be read")
	asText := up(segmentsOf(ex))
	asText.Format = "txt"
	wantProblem(t, checkFile(asText, "de-DE"), `its name says "docx"`)
}

func TestCheckDocxRevisions(t *testing.T) {
	data := docxtest.Document(docxtest.Parts{Lang: "de-DE", Body: `<w:p><w:r><w:t xml:space="preserve">Die Miete ist </w:t></w:r><w:ins w:id="1" w:author="A" w:date="2026-01-01T00:00:00Z"><w:r><w:t>monatlich </w:t></w:r></w:ins><w:r><w:t>fällig.</w:t></w:r></w:p>`})
	kept, err := docx.Extract(data, "de-DE", docx.Options{KeepRevisions: true})
	if err != nil {
		t.Fatal(err)
	}
	f := fileUpload{Name: "Nachtrag.docx", Role: project.Translatable, Format: "docx", Data: data, Segments: map[string][]project.Segment{"en-GB": segmentsOf(kept)}}
	if err := checkFile(f, "de-DE"); err != nil {
		t.Fatalf("segments read with revisions kept: %v", err)
	}
	accepted, err := docx.Extract(data, "de-DE", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f.Segments["en-GB"] = segmentsOf(accepted)
	if err := checkFile(f, "de-DE"); err != nil {
		t.Fatalf("segments read with revisions accepted: %v", err)
	}
}

const lockedXliff = `<?xml version="1.0"?><xliff xmlns:sdl="http://sdl.com/FileTypes/SdlXliff/1.0" xmlns="urn:oasis:names:tc:xliff:document:1.2" version="1.2"><file source-language="de-DE" target-language="en-GB"><body>` +
	`<trans-unit id="u"><source>Die Miete ist fällig. Kanzlei Weber</source><seg-source><mrk mtype="seg" mid="1">Die Miete ist fällig.</mrk> <mrk mtype="seg" mid="2">Kanzlei Weber</mrk></seg-source>` +
	`<target><mrk mtype="seg" mid="1">The rent is due.</mrk> <mrk mtype="seg" mid="2">Kanzlei Weber</mrk></target>` +
	`<sdl:seg-defs><sdl:seg id="1" conf="Translated"/><sdl:seg id="2" conf="ApprovedSignOff" locked="true"/></sdl:seg-defs></trans-unit></body></file></xliff>`

func TestCheckXliff(t *testing.T) {
	ex, err := xliff.Extract([]byte(lockedXliff))
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 2 || !ex.Units[1].Locked {
		t.Fatalf("units %+v", ex.Units)
	}
	segs := segmentsOf(ex)
	for i := range segs {
		segs[i].Locked = false // an app that unlocks segments
	}
	f := fileUpload{Name: "Brief.sdlxliff", Role: project.Translatable, TargetLang: "en-GB", Format: "xliff", Data: []byte(lockedXliff), Segments: map[string][]project.Segment{"en-GB": segs}}
	if err := checkFile(f, "de-DE"); err != nil {
		t.Fatal(err)
	}
	for i, u := range ex.Units {
		if u.Locked && !f.Segments["en-GB"][i].Locked {
			t.Fatalf("segment %s is locked in the file", u.ID)
		}
	}
	forged := segmentsOf(ex)
	forged[0].Source = content.Text("Die Miete entfällt.")
	f.Segments["en-GB"] = forged
	wantProblem(t, checkFile(f, "de-DE"), "Segment 1 of Brief.sdlxliff")
}

func TestCheckText(t *testing.T) {
	data := []byte("Der Schlüssel liegt beim Hausmeister.\n\nDie Heizung wird im Oktober\neingeschaltet.\n")
	up := func(texts ...string) fileUpload {
		return fileUpload{Name: "Hinweise.txt", Role: project.Translatable, Format: "txt", Data: data, Segments: map[string][]project.Segment{"en-GB": segs(texts...)}}
	}
	// A line is a paragraph: the file has three segments, and the app's must be those.
	if err := checkFile(up("Der Schlüssel liegt beim Hausmeister.", "Die Heizung wird im Oktober", "eingeschaltet."), "de-DE"); err != nil {
		t.Fatal(err)
	}
	wantProblem(t, checkFile(up("Die Heizung wird im Oktober", "Der Schlüssel liegt beim Hausmeister.", "eingeschaltet."), "de-DE"), "Segment 1 of Hinweise.txt")
	wantProblem(t, checkFile(up("Der Schlüssel liegt beim Nachbarn.", "Die Heizung wird im Oktober", "eingeschaltet."), "de-DE"), "Segment 1")
	wantProblem(t, checkFile(up("Der Schlüssel liegt beim Hausmeister.", "Die Heizung wird im Oktober eingeschaltet."), "de-DE"), "has 3 segments, not 2")
	// Reference files are not segmented, so nothing is checked.
	ref := fileUpload{Name: "Plan.pdf", Role: project.Reference, Format: "pdf", Data: []byte("%PDF")}
	if err := checkFile(ref, "de-DE"); err != nil {
		t.Fatal(err)
	}
}

func TestForgedFileRefused(t *testing.T) {
	te := newTeam(t)
	req := leaseRequest()
	req.Files[0].Segments["fr-FR"] = segs("Mietvertrag.", "Der Mieter zahlt nichts.", "Die Kaution beträgt drei Monatsmieten.", "Der Vermieter haftet nicht.")
	r := te.do("POST", "/api/projects", te.pm, req)
	if r.status != 422 || r.err().Code != "badFile" || !strings.Contains(r.err().Message, "Segment 2 of Mietvertrag.txt (fr-FR)") {
		t.Fatalf("forged segments: %d %s", r.status, r.body)
	}
	p := te.createLease()
	nf := fileUpload{Name: "Nachtrag.txt", Role: project.Translatable, Format: "txt", Data: []byte("Der Mieter zahlt die Miete.\n"),
		Segments: map[string][]project.Segment{"en-GB": segs("Der Mieter zahlt keine Miete.")}}
	if r := te.do("POST", fmt.Sprintf("/api/projects/%d/files?version=1", p.ID), te.pm, nf); r.status != 422 || r.err().Code != "badFile" {
		t.Fatalf("forged added file: %d %s", r.status, r.body)
	}
}

// checkFile prepares a file sent with its segments, into en-GB.
func checkFile(f fileUpload, sourceLang string) error {
	return prepareFile(&f, sourceLang, []string{"en-GB"})
}
