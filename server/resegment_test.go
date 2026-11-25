package server

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
)

// A translator cuts a document's segments differently, and the document
// is written with what they translated.
func TestSplitAndJoinSegments(t *testing.T) {
	te := newTeam(t)
	lease := docxtest.Document(docxtest.Parts{Lang: "de-DE", Body: docxtest.P("", docxtest.R("Der Mieter zahlt "), docxtest.B("pünktlich"), docxtest.R(" die Miete. Die Kaution beträgt drei Monatsmieten.")) +
		docxtest.P("", docxtest.R("Der Vermieter haftet nicht."))})
	req := leaseRequest()
	req.Files = []fileUpload{
		{Name: "Mietvertrag.docx", Role: project.Translatable, Data: lease},
		{Name: "Hinweise.txt", Role: project.Translatable, Data: []byte("Der Schlüssel liegt beim Hausmeister. Die Heizung läuft.\n")},
		{Name: "Brief.sdlxliff", Role: project.Translatable, Data: tradosFile(t)},
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
	for _, f := range []string{"Hinweise.txt", "Brief.sdlxliff"} {
		var segs []projectdb.StoredSegment
		te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=%s&lang=en-GB", p.ID, f), te.pm, nil, 200, &segs)
		if r := te.do("POST", at("join", f, segs[0].ID, segs[0].Version), te.pm, nil); r.status != 422 || r.err().Code != "notHere" || !strings.Contains(r.err().Message, "its own segments") {
			t.Fatalf("%s: %d %s", f, r.status, r.body)
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
	te.call("POST", at("join", "Mietvertrag.docx", first, 1), te.pm, nil, 200, &got)
	if len(got) != 2 || got[0].ID != first || len(got[0].Joined) != 2 || got[0].Version != 2 || got[0].Source.Plain() != "Der Mieter zahlt pünktlich die Miete. Die Kaution beträgt drei Monatsmieten." {
		t.Fatalf("joined %+v", got)
	}
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
	// Separated again, the sentences stand apart with the translation on the first.
	te.call("POST", at("separate", "Mietvertrag.docx", first, 3), te.pm, nil, 200, &got)
	if len(got) != 4 || got[1].ID != second || got[1].Status != project.StatusNew || got[0].Status != project.StatusDraft {
		t.Fatalf("separated %+v", got)
	}

	// A project sent with segments cut differently is taken as the app has it.
	te.call("POST", at("join", "Mietvertrag.docx", first, got[0].Version), te.pm, nil, 200, &got)
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
	if len(kept) != 3 || len(kept[0].Joined) != 2 || kept[1].Split == nil || kept[2].Target.Plain() != "is not liable." {
		t.Fatalf("sent with its cuts %+v", kept)
	}
	// One whose parts are not the file's is refused.
	sent[2].Source = content.Text("haftet doch.")
	again.Settings.Name = "Mietvertrag Hofmann 3"
	if r := te.do("POST", "/api/projects", te.pm, again); r.status != 422 {
		t.Fatalf("parts that are not the file's: %d %s", r.status, r.body)
	}
}
