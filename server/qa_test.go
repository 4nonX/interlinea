package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/qa"
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
