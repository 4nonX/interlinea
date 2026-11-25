package server

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
)

// A translator cuts a document's segments differently, and the document
// is written with what they translated.
func TestSplitAndJoinSegments(t *testing.T) {
	te := newTeam(t)
	lease := docxtest.Document(docxtest.Parts{Lang: "de-DE", Body: docxtest.P("", docxtest.R("Der Mieter zahlt "), docxtest.B("pünktlich"), docxtest.R(" die Miete. Die Kaution beträgt drei Monatsmieten.")) +
		docxtest.P("", docxtest.R("Der Vermieter haftet nicht."))})
	plain, err := os.ReadFile("../filter/xliff/testdata/xliff2.xlf")
	if err != nil {
		t.Fatal(err)
	}
	req := leaseRequest()
	req.Files = []fileUpload{
		{Name: "Mietvertrag.docx", Role: project.Translatable, Data: lease},
		{Name: "Hinweise.txt", Role: project.Translatable, Data: []byte("Der Schlüssel liegt beim Hausmeister. Die Heizung läuft.\n\nBitte lüften.\n")},
		{Name: "Brief.sdlxliff", Role: project.Translatable, Data: tradosFile(t)},
		{Name: "Plain.xlf", Role: project.Translatable, Data: plain},
	}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	at := func(what, file, seg string, version int) string {
		return fmt.Sprintf("/api/projects/%d/segments/%s?file=%s&lang=en-GB&segment=%s&version=%d", p.ID, what, file, url.QueryEscape(seg), version)
	}
	var word []projectdb.StoredSegment
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Mietvertrag.docx&lang=en-GB", p.ID), te.pm, nil, 200, &word)
	if len(word) != 3 {
		t.Fatalf("%d segments", len(word))
	}
	first, second, third := word[0].ID, word[1].ID, word[2].ID

	// Not for a stranger, not without a version, not in a file that brings its own segments.
	if r := te.do("POST", at("join", "Mietvertrag.docx", first, 1), te.ben, nil); r.status != 404 {
		t.Fatalf("a stranger joins: %d", r.status)
	}
	if r := te.do("POST", strings.Replace(at("join", "Mietvertrag.docx", first, 1), "&version=1", "", 1), te.pm, nil); r.status != 428 {
		t.Fatalf("without a version: %d", r.status)
	}
	if r := te.do("POST", at("join", "Mietvertrag.docx", first, 7), te.pm, nil); r.status != 409 {
		t.Fatalf("a stale copy: %d", r.status)
	}
	// A file of Trados keeps its segments: the program has its own record of each.
	{
		var segs []projectdb.StoredSegment
		te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Brief.sdlxliff&lang=en-GB", p.ID), te.pm, nil, 200, &segs)
		if segs[0].File == nil || !segs[0].File.Fixed {
			t.Fatalf("a Trados segment is not marked as fixed: %+v", segs[0].File)
		}
		for _, how := range []string{"join", "split"} {
			if r := te.do("POST", at(how, "Brief.sdlxliff", segs[0].ID, segs[0].Version), te.pm, map[string]any{"before": content.Text("x")}); r.status != 422 || r.err().Code != "notHere" || !strings.Contains(r.err().Message, "keeps its own record") {
				t.Fatalf("%s in a Trados file: %d %s", how, r.status, r.body)
			}
		}
	}
	// A text file is cut like any document, and written with what was translated.
	{
		var segs []projectdb.StoredSegment
		te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Hinweise.txt&lang=en-GB", p.ID), te.pm, nil, 200, &segs)
		if len(segs) != 3 {
			t.Fatalf("text segments %+v", segs)
		}
		if r := te.do("POST", at("join", "Hinweise.txt", segs[1].ID, 1), te.pm, nil); r.status != 422 || !strings.Contains(r.err().Message, "different paragraphs") {
			t.Fatalf("two lines of a text joined: %d %s", r.status, r.body)
		}
		var cut []projectdb.StoredSegment
		te.call("POST", at("join", "Hinweise.txt", segs[0].ID, 1), te.pm, nil, 200, &cut)
		te.call("POST", at("split", "Hinweise.txt", cut[1].ID, 1), te.pm, map[string]any{"before": content.Text("Bitte")}, 200, &cut)
		if len(cut) != 3 || len(cut[0].Joined) != 2 || cut[2].Source.Plain() != "lüften." {
			t.Fatalf("text cut %+v", cut)
		}
		te.call("PUT", segPath(p.ID, "Hinweise.txt", "en-GB", cut[0].ID, cut[0].Version), te.pm, project.Edit{Target: content.Text("The heating is on; the key is with the caretaker."), Status: project.StatusConfirmed}, 200, nil)
		te.call("PUT", segPath(p.ID, "Hinweise.txt", "en-GB", url.QueryEscape(cut[2].ID), cut[2].Version), te.pm, project.Edit{Target: content.Text("air the rooms."), Status: project.StatusConfirmed}, 200, nil)
		r := te.download(fmt.Sprintf("/api/projects/%d/files/translated?name=Hinweise.txt&lang=en-GB", p.ID), te.pm)
		if string(r.body) != "The heating is on; the key is with the caretaker.\n\nBitte air the rooms.\n" {
			t.Fatalf("the text: %q", r.body)
		}
	}
	// So is a plain XLIFF file, within its units.
	{
		var segs []projectdb.StoredSegment
		te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Plain.xlf&lang=en-GB", p.ID), te.pm, nil, 200, &segs)
		if len(segs) != 3 || segs[0].File == nil || segs[0].File.Fixed {
			t.Fatalf("xliff segments %+v", segs)
		}
		if r := te.do("POST", at("join", "Plain.xlf", segs[1].ID, segs[1].Version), te.pm, nil); r.status != 422 || !strings.Contains(r.err().Message, "different paragraphs") {
			t.Fatalf("two units of an XLIFF file joined: %d %s", r.status, r.body)
		}
		var cut []projectdb.StoredSegment
		te.call("POST", at("join", "Plain.xlf", segs[0].ID, segs[0].Version), te.pm, nil, 200, &cut)
		if len(cut) != 2 || len(cut[0].Joined) != 2 || cut[0].File != nil {
			t.Fatalf("xliff joined %+v", cut)
		}
		te.call("PUT", segPath(p.ID, "Plain.xlf", "en-GB", url.QueryEscape(cut[0].ID), cut[0].Version), te.pm, project.Edit{Target: content.Text("Both sentences as one."), Status: project.StatusConfirmed}, 200, nil)
		r := te.download(fmt.Sprintf("/api/projects/%d/files/translated?name=Plain.xlf&lang=en-GB", p.ID), te.pm)
		back, err := xliff.Extract(r.body)
		if err != nil {
			t.Fatal(err)
		}
		if back.Units[0].Target.Plain() != "Both sentences as one." || len(back.Units[1].Target) != 0 || back.Units[1].State != filter.StateTranslated || back.Units[2].Target.Plain() != "Draft deposit" {
			t.Fatalf("the XLIFF file: %s", r.body)
		}
	}
	if r := te.do("POST", at("join", "Mietvertrag.docx", second, 1), te.pm, nil); r.status != 422 || !strings.Contains(r.err().Message, "different paragraphs") {
		t.Fatalf("across paragraphs: %d %s", r.status, r.body)
	}
	if r := te.do("POST", at("split", "Mietvertrag.docx", third, 1), te.pm, map[string]any{"before": content.Text("Etwas anderes")}); r.status != 422 || !strings.Contains(r.err().Message, "not in the segment") {
		t.Fatalf("a place that is not there: %d %s", r.status, r.body)
	}

	// Join the two sentences, split the last segment.
	var got []projectdb.StoredSegment
	got = nil
	te.call("POST", at("join", "Mietvertrag.docx", first, 1), te.pm, nil, 200, &got)
	if len(got) != 2 || got[0].ID != first || len(got[0].Joined) != 2 || got[0].Version != 2 || got[0].Source.Plain() != "Der Mieter zahlt pünktlich die Miete. Die Kaution beträgt drei Monatsmieten." {
		t.Fatalf("joined %+v", got)
	}
	got = nil
	te.call("POST", at("split", "Mietvertrag.docx", third, 1), te.pm, map[string]any{"before": content.Text("Der Vermieter ")}, 200, &got)
	if len(got) != 3 || got[1].ID != third+"#1" || got[2].ID != third+"#2" || got[2].Source.Plain() != "haftet nicht." {
		t.Fatalf("split %+v", got)
	}
	joined := content.Text("The deposit is three months' rent, and the tenant pays ").AppendTag(content.Tag{Kind: content.TagOpen, ID: "1"}).Append("on time").AppendTag(content.Tag{Kind: content.TagClose, ID: "1"}).Append(".")
	te.call("PUT", segPath(p.ID, "Mietvertrag.docx", "en-GB", first, 2), te.pm, project.Edit{Target: joined, Status: project.StatusConfirmed}, 200, nil)
	te.call("PUT", segPath(p.ID, "Mietvertrag.docx", "en-GB", url.QueryEscape(third+"#2"), 2), te.pm, project.Edit{Target: content.Text("is not liable."), Status: project.StatusConfirmed}, 200, nil)

	r := te.download(fmt.Sprintf("/api/projects/%d/files/translated?name=Mietvertrag.docx&lang=en-GB", p.ID), te.pm)
	back, err := docx.Extract(r.body, "en-GB", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	all := ""
	for _, u := range back.Units {
		all += u.Source.String() + "|"
	}
	if all != "The deposit is three months' rent, and the tenant pays {1}on time{/1}.|Der Vermieter is not liable.|" {
		t.Fatalf("the document: %s", all)
	}
	// The other language is as the file was read.
	var fr []projectdb.StoredSegment
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Mietvertrag.docx&lang=fr-FR", p.ID), te.pm, nil, 200, &fr)
	if len(fr) != 3 || fr[0].ID != first || fr[2].ID != third {
		t.Fatalf("fr %+v", fr)
	}
	// Split where they were joined, the sentences stand apart again, the translation on the first.
	got = nil
	te.call("POST", at("split", "Mietvertrag.docx", first, 3), te.pm, map[string]any{"before": word[0].Source}, 200, &got)
	if len(got) != 4 || got[1].ID != second || got[1].Status != project.StatusNew || got[0].Status != project.StatusDraft || got[0].Joined != nil {
		t.Fatalf("apart again %+v", got)
	}
	// A part of one sentence joined with the next one, and sent as the app has it.
	var parts, grown []projectdb.StoredSegment
	te.call("POST", at("split", "Mietvertrag.docx", first, got[0].Version), te.pm, map[string]any{"before": content.Text("Der Mieter")}, 200, &parts)
	te.call("POST", at("join", "Mietvertrag.docx", parts[1].ID, parts[1].Version), te.pm, nil, 200, &grown)
	if len(grown) != 4 || len(grown[1].Joined) != 2 || grown[1].Split == nil || grown[0].ID != first+"#1" {
		t.Fatalf("a part joined on %+v", grown)
	}
	got = grown

	// A project sent with segments cut differently is taken as the app has it.
	sent := make([]project.Segment, len(got))
	for i := range got {
		sent[i] = got[i].Segment
	}
	again := leaseRequest()
	again.Settings.Name = "Mietvertrag Hofmann 2"
	again.Settings.TargetLangs = []string{"en-GB"}
	again.Settings.Memories = map[string][]project.MemorySetting{}
	again.Memories, again.Termbase = nil, nil
	again.Settings.Terminology = project.Terminology{Mode: project.Direct, Masters: []string{"tb-weber"}, WriteTo: "tb-weber", Location: project.Server}
	again.Files = []fileUpload{{Name: "Mietvertrag.docx", Role: project.Translatable, Data: lease, Segments: map[string][]project.Segment{"en-GB": sent}}}
	var p2 projectView
	te.call("POST", "/api/projects", te.pm, again, 201, &p2)
	var kept []projectdb.StoredSegment
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Mietvertrag.docx&lang=en-GB", p2.ID), te.pm, nil, 200, &kept)
	if len(kept) != 4 || len(kept[0].Joined) != 2 || kept[1].Split == nil || kept[1].Source.Plain() != "zahlt pünktlich die Miete. Die Kaution beträgt drei Monatsmieten." || kept[3].Target.Plain() != "is not liable." {
		t.Fatalf("sent with its cuts %+v", kept)
	}
	// One whose parts are not the file's is refused.
	sent[3].Source = content.Text("haftet doch.")
	again.Settings.Name = "Mietvertrag Hofmann 3"
	if r := te.do("POST", "/api/projects", te.pm, again); r.status != 422 {
		t.Fatalf("parts that are not the file's: %d %s", r.status, r.body)
	}

	// A project's own segmentation rules cut the files the server reads.
	ruled := leaseRequest()
	ruled.Settings.Name = "Mietvertrag Hofmann 4"
	ruled.Settings.TargetLangs = []string{"en-GB"}
	ruled.Settings.Memories = map[string][]project.MemorySetting{}
	ruled.Memories, ruled.Termbase = nil, nil
	ruled.Settings.Terminology = project.Terminology{Mode: project.Direct, Masters: []string{"tb-weber"}, WriteTo: "tb-weber", Location: project.Server}
	ruled.Settings.Segmentation = []project.SegmentationRule{{Break: false, Before: `Miete\.`, After: `\s+Die`}, {Break: true, Before: `Vermieter`, After: `\s`}}
	ruled.Files = []fileUpload{{Name: "Mietvertrag.docx", Role: project.Translatable, Data: lease}}
	var p4 projectView
	te.call("POST", "/api/projects", te.pm, ruled, 201, &p4)
	var cut []projectdb.StoredSegment
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Mietvertrag.docx&lang=en-GB", p4.ID), te.pm, nil, 200, &cut)
	if len(cut) != 3 || len(cut[0].Joined) != 2 || cut[1].Source.Plain() != "Der Vermieter" || cut[2].Source.Plain() != "haftet nicht." {
		t.Fatalf("cut by the project's rules %+v", cut)
	}
	if len(p4.Settings.Segmentation) != 2 {
		t.Fatalf("the rules are kept with the project: %+v", p4.Settings.Segmentation)
	}
	ruled.Settings.Name = "Mietvertrag Hofmann 5"
	ruled.Settings.Segmentation = []project.SegmentationRule{{Break: true, Before: `(`}}
	if r := te.do("POST", "/api/projects", te.pm, ruled); r.status != 422 || r.err().Problems[0].Field != "segmentation[0]" {
		t.Fatalf("a rule that is no regular expression: %d %s", r.status, r.body)
	}
}
