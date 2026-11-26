package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/qa"
	"github.com/4nonX/interlinea/spell"
)

func dictionaryArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"en_GB.aff": "SET UTF-8\nLANG en_GB\n",
		"en_GB.dic": "9\nThe\ntenant\npays\nthe\nrent\nlease\ndeposit\nis\nthree\n",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestDictionariesAndQA(t *testing.T) {
	te := newTeam(t)
	arch := dictionaryArchive(t)
	if r := te.do("POST", "/api/dictionaries", te.ben, arch, "Content-Type", "application/octet-stream"); r.status != 403 {
		t.Fatalf("a translator installs a dictionary: %d", r.status)
	}
	var ds []projectdb.Dictionary
	te.call("POST", "/api/dictionaries", te.pm, arch, 200, &ds, "Content-Type", "application/octet-stream")
	if len(ds) != 1 || ds[0].Name != "en_GB" || ds[0].Lang != "en-GB" || ds[0].Words != 9 {
		t.Fatalf("installed %+v", ds)
	}
	te.call("GET", "/api/dictionaries", te.ben, nil, 200, &ds)
	if len(ds) != 1 {
		t.Fatalf("listed %+v", ds)
	}
	var sr spellResult
	te.call("POST", "/api/dictionaries/en_GB/check", te.ben, spellRequest{Words: []string{"tenant", "tennant"}}, 200, &sr)
	if len(sr.Correct) != 2 || !sr.Correct[0] || sr.Correct[1] {
		t.Fatalf("checked %+v", sr)
	}
	var sug []string
	te.call("GET", "/api/dictionaries/en_GB/suggest?word=tennant", te.ben, nil, 200, &sug)
	if len(sug) == 0 || sug[0] != "tenant" {
		t.Fatalf("suggestions %v", sug)
	}
	var kinds []struct{ Kind qa.Kind }
	te.call("GET", "/api/qa/kinds", te.ben, nil, 200, &kinds)
	if len(kinds) != len(qa.Kinds) {
		t.Fatalf("kinds %v", kinds)
	}

	// A project spells English with it and checks its terms.
	req := leaseRequest()
	req.Settings.QA.Dictionaries = map[string]string{"en-GB": "en_GB"}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	if _, err := te.st.Termbases.Termbase(ctx, "tb-weber"); err != nil {
		t.Fatal(err)
	}
	tb, _ := te.st.Termbases.Termbase(ctx, "tb-weber")
	if _, err := tb.Add(ctx, entry("Mieter", "tenant"), "test", ""); err != nil {
		t.Fatal(err)
	}
	var issues []qa.Issue
	te.call("POST", base+"/qa", te.pm, qaRequest{File: "Mietvertrag.txt", Lang: "en-GB", Segments: []string{"s2"},
		Targets: map[string]content.Content{"s2": content.Text("The lessee pays the rent 3 times.")}}, 200, &issues)
	found := map[qa.Kind]bool{}
	for _, i := range issues {
		found[i.Kind] = true
		if i.Segment != "s2" {
			t.Fatalf("issue on another segment: %+v", i)
		}
	}
	if !found[qa.Spelling] || !found[qa.TermMissing] || !found[qa.NumbersExtra] {
		t.Fatalf("issues %+v", issues)
	}
	// With the term and no number, only the unknown word is left.
	te.call("POST", base+"/qa", te.pm, qaRequest{File: "Mietvertrag.txt", Lang: "en-GB", Segments: []string{"s2"},
		Targets: map[string]content.Content{"s2": content.Text("The tenant pays the rent.")}}, 200, &issues)
	if len(issues) != 0 {
		t.Fatalf("issues after fixing %+v", issues)
	}
	// The dictionary is in use, so it stays.
	if r := te.do("DELETE", "/api/dictionaries/en_GB", te.pm, nil); r.status != 409 || r.err().Code != "inUse" {
		t.Fatalf("deleting a dictionary in use: %d %s", r.status, r.body)
	}

	// The report.
	r := te.download(base+"/qa/report?file=Mietvertrag.txt&lang=en-GB&format=csv", te.pm)
	if !strings.HasPrefix(r.header.Get("Content-Type"), "text/csv") || !bytes.Contains(r.body, []byte("Empty translation")) {
		t.Fatalf("report %s %s", r.header.Get("Content-Type"), r.body)
	}
	if r := te.do("GET", base+"/qa/report?file=Mietvertrag.txt&lang=en-GB&format=pdf", te.pm, nil); r.status != 422 {
		t.Fatalf("pdf report: %d", r.status)
	}

	// Terms in a segment, with their place in UTF-16 too.
	var hits []termHit
	te.call("POST", base+"/recognize", te.pm, recognizeRequest{Lang: "en-GB", Text: "Für den Mieter gilt"}, 200, &hits)
	if len(hits) != 1 || hits[0].Term.Text != "Mieter" || hits[0].Start16 != 8 || hits[0].End16 != 14 || hits[0].Start != 9 || len(hits[0].Targets) == 0 || hits[0].Targets[0].Text != "tenant" {
		t.Fatalf("hits %+v", hits)
	}

	// The analysis counts the project's words per language.
	var reps []analysis.Report
	te.call("POST", base+"/analysis", te.pm, analysisRequest{}, 200, &reps)
	if len(reps) != 2 || reps[0].TargetLang != "en-GB" || reps[0].Total.Words == 0 || len(reps[0].Files) != 1 {
		t.Fatalf("analysis %+v", reps)
	}
	te.call("POST", base+"/analysis", te.pm, analysisRequest{Lang: "fr-fr"}, 200, &reps)
	if len(reps) != 1 || reps[0].TargetLang != "fr-FR" {
		t.Fatalf("one language %+v", reps)
	}
	// Not for those who can't see the project.
	if r := te.do("POST", base+"/qa", te.eve, qaRequest{File: "Mietvertrag.txt", Lang: "en-GB"}); r.status != 404 {
		t.Fatalf("someone else checks: %d", r.status)
	}
}

func TestLooseDictionary(t *testing.T) {
	te := newTeam(t)
	loose := looseDictionary{Name: "Hausliste", Lang: "de-AT", Aff: []byte("SET UTF-8\n"), Dic: []byte("2\nMieter\nKaution\n")}
	if r := te.do("POST", "/api/dictionaries", te.ben, loose); r.status != 403 {
		t.Fatalf("a translator installs a dictionary: %d", r.status)
	}
	var ds []projectdb.Dictionary
	te.call("POST", "/api/dictionaries", te.pm, loose, 200, &ds)
	if len(ds) != 1 || ds[0].Name != "Hausliste" || ds[0].Lang != "de-AT" || ds[0].Words != 2 {
		t.Fatalf("installed %+v", ds)
	}
	var sr spellResult
	te.call("POST", "/api/dictionaries/Hausliste/check", te.ben, spellRequest{Words: []string{"Mieter", "Meiter"}}, 200, &sr)
	if len(sr.Correct) != 2 || !sr.Correct[0] || sr.Correct[1] {
		t.Fatalf("checked %+v", sr)
	}
	// Sent again with other words, it replaces the one of its name at once.
	loose.Dic = []byte("1\nVermieter\n")
	te.call("POST", "/api/dictionaries", te.pm, loose, 200, &ds)
	te.call("POST", "/api/dictionaries/Hausliste/check", te.ben, spellRequest{Words: []string{"Mieter", "Vermieter"}}, 200, &sr)
	if sr.Correct[0] || !sr.Correct[1] {
		t.Fatalf("after replacing %+v", sr)
	}
	// Without a language, the files or the name must say it.
	loose.Lang = ""
	if r := te.do("POST", "/api/dictionaries", te.pm, loose); r.status != 422 || r.err().Problems[0].Field != "lang" {
		t.Fatalf("no language: %d %s", r.status, r.body)
	}
	loose.Name = "fr_FR"
	te.call("POST", "/api/dictionaries", te.pm, loose, 200, &ds)
	if ds[0].Lang != "fr-FR" {
		t.Fatalf("language from the name: %+v", ds)
	}
	for _, bad := range []looseDictionary{{Name: "x", Aff: []byte("SET UTF-8\n")}, {Name: "a/b", Aff: []byte("x"), Dic: []byte("x")}, {Name: "x", Lang: "not a tag", Aff: []byte("x"), Dic: []byte("x")}} {
		if r := te.do("POST", "/api/dictionaries", te.pm, bad); r.status != 422 {
			t.Fatalf("%+v: %d %s", bad, r.status, r.body)
		}
	}
}

// The dictionaries that come with Interlinea are listed and used like
// installed ones, can't be deleted, and give way to an installed one of
// the same name.
func TestIncludedDictionaries(t *testing.T) {
	fsys := fstest.MapFS{
		"en_GB.zip":      {Data: dictionaryArchive(t)},
		spell.ShelfIndex: {Data: []byte(`{"source":"test","commit":"1","dictionaries":[{"name":"en_GB","lang":"en-GB","words":9,"file":"en_GB.zip"}]}`)},
	}
	shelf, err := spell.OpenShelf(fsys)
	if err != nil {
		t.Fatal(err)
	}
	te := newTeam(t, func(c *Config) { c.Dictionaries = shelf })
	var ds []dictionaryView
	ds = nil
	te.call("GET", "/api/dictionaries", te.ben, nil, 200, &ds)
	if len(ds) != 1 || ds[0].Name != "en_GB" || ds[0].Lang != "en-GB" || ds[0].Words != 9 || !ds[0].Included {
		t.Fatalf("listed %+v", ds)
	}
	var sr spellResult
	te.call("POST", "/api/dictionaries/EN_gb/check", te.ben, spellRequest{Words: []string{"tenant", "tennant"}}, 200, &sr)
	if !sr.Correct[0] || sr.Correct[1] {
		t.Fatalf("checked %+v", sr)
	}
	if r := te.do("DELETE", "/api/dictionaries/en_GB", te.pm, nil); r.status != 409 || r.err().Code != "included" {
		t.Fatalf("deleting an included dictionary: %d %s", r.status, r.body)
	}

	// A project spells with it.
	req := leaseRequest()
	req.Settings.QA.Dictionaries = map[string]string{"en-GB": "en_GB"}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	var issues []qa.Issue
	te.call("POST", base+"/qa", te.pm, qaRequest{File: "Mietvertrag.txt", Lang: "en-GB", Segments: []string{"s2"},
		Targets: map[string]content.Content{"s2": content.Text("The tennant pays the rent.")}}, 200, &issues)
	if len(issues) == 0 || issues[0].Kind != qa.Spelling {
		t.Fatalf("issues %+v", issues)
	}

	// One installed under its name takes its place...
	own := looseDictionary{Name: "en_GB", Lang: "en-GB", Aff: []byte("SET UTF-8\n"), Dic: []byte("3\nThe\ntennant\npays\n")}
	te.call("POST", "/api/dictionaries", te.pm, own, 200, nil)
	ds = nil
	te.call("GET", "/api/dictionaries", te.ben, nil, 200, &ds)
	if len(ds) != 1 || ds[0].Included || ds[0].Words != 3 {
		t.Fatalf("listed with an own one %+v", ds)
	}
	te.call("POST", "/api/dictionaries/en_GB/check", te.ben, spellRequest{Words: []string{"tenant", "tennant"}}, 200, &sr)
	if sr.Correct[0] || !sr.Correct[1] {
		t.Fatalf("checked with the own one %+v", sr)
	}
	// ...and can be deleted though the project spells with the name: the
	// included one is used again.
	te.call("DELETE", "/api/dictionaries/en_GB", te.pm, nil, 204, nil)
	ds = nil
	te.call("GET", "/api/dictionaries", te.ben, nil, 200, &ds)
	if len(ds) != 1 || !ds[0].Included {
		t.Fatalf("listed after deleting the own one %+v", ds)
	}
	te.call("POST", "/api/dictionaries/en_GB/check", te.ben, spellRequest{Words: []string{"tenant", "tennant"}}, 200, &sr)
	if !sr.Correct[0] || sr.Correct[1] {
		t.Fatalf("checked after deleting the own one %+v", sr)
	}
	if r := te.do("GET", "/api/dictionaries/fr_FR/suggest?word=x", te.ben, nil); r.status != 404 {
		t.Fatalf("a dictionary that is nowhere: %d", r.status)
	}
}

// Subtitles keep their times on the server, and are checked against the
// project's limits with them.
func TestSubtitleChecks(t *testing.T) {
	te := newTeam(t)
	req := leaseRequest()
	req.Settings.QA.Subtitles = qa.SubtitleLimits{LineLength: 30, Lines: 2, CPS: 15}
	req.Files = append(req.Files, fileUpload{Name: "film.srt", Role: project.Translatable, Format: "srt",
		Data: []byte("1\n00:00:01,000 --> 00:00:03,000\nGuten Abend.\nWillkommen.\n\n2\n00:00:04,000 --> 00:00:05,000\nAuf Wiedersehen.\n")})
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	var segs []project.Segment
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=film.srt&lang=en-GB", p.ID), te.pm, nil, 200, &segs)
	if len(segs) != 2 || segs[0].Timing == nil || segs[0].Timing.Start != 1000 || segs[0].Timing.End != 3000 {
		t.Fatalf("segments %+v", segs)
	}
	var issues []qa.Issue
	te.call("POST", fmt.Sprintf("/api/projects/%d/qa", p.ID), te.pm, qaRequest{File: "film.srt", Lang: "en-GB", Segments: []string{segs[1].ID},
		Targets: map[string]content.Content{segs[1].ID: content.Text("Goodbye, and thank you all for watching tonight.")}}, 200, &issues)
	found := map[qa.Kind]string{}
	for _, i := range issues {
		found[i.Kind] = i.Message
	}
	if found[qa.LineLength] != "Line 1 has 48 characters; at most 30 are allowed." || !strings.Contains(found[qa.ReadingSpeed], "48.0 characters per second") {
		t.Errorf("issues %+v", issues)
	}
	// The project's limits are part of its settings.
	if p.Settings.QA.Subtitles.LineLength != 30 {
		t.Errorf("settings %+v", p.Settings.QA)
	}
	bad := leaseRequest()
	bad.Settings.Name = "Zu viel"
	bad.Settings.QA.Subtitles.Lines = 11
	bad.Memories, bad.Settings.Memories = nil, map[string][]project.MemorySetting{}
	bad.Termbase.Name = "Zu viel"
	if r := te.do("POST", "/api/projects", te.pm, bad); r.status != 422 || r.err().Problems[0].Field != "qa.subtitles.lines" {
		t.Errorf("a limit out of range: %d %s", r.status, r.body)
	}
}
