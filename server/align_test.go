package server

import (
	"fmt"
	"testing"

	"github.com/4nonX/interlinea/align"
	"github.com/4nonX/interlinea/content"
)

func TestAlignFiles(t *testing.T) {
	te := newTeam(t)
	de := "Mietvertrag\n\nDer Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats. Die Kaution beträgt 2.400,00 Euro.\n\nDas Mietverhältnis beginnt am 1. März 2026 und läuft auf unbestimmte Zeit.\n"
	en := "<html><body><h1>Lease</h1><p>The tenant pays the rent by the third working day of each month. The deposit is EUR 2,400.00.</p><p>The tenancy begins on 1 March 2026 and runs for an indefinite period.</p></body></html>"
	req := alignRequest{
		Source: alignedFile{Name: "Mietvertrag.txt", Lang: "de-DE", Data: []byte(de)},
		Target: alignedFile{Name: "Lease.html", Lang: "en-GB", Data: []byte(en)},
	}
	var al align.Result
	te.call("POST", "/api/align", te.ben, req, 200, &al)
	if len(al.Source) != 4 || len(al.Target) != 4 || len(al.Pairs) != 4 {
		t.Fatalf("aligned %+v", al)
	}
	for i, want := range [][2]string{
		{"Mietvertrag", "Lease"},
		{"Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats.", "The tenant pays the rent by the third working day of each month."},
		{"Die Kaution beträgt 2.400,00 Euro.", "The deposit is EUR 2,400.00."},
		{"Das Mietverhältnis beginnt am 1. März 2026 und läuft auf unbestimmte Zeit.", "The tenancy begins on 1 March 2026 and runs for an indefinite period."},
	} {
		p := al.Pairs[i]
		if p.Source.Plain() != want[0] || p.Target.Plain() != want[1] || p.Score < 50 {
			t.Errorf("pair %d: %q = %q (%d)", i, p.Source.Plain(), p.Target.Plain(), p.Score)
		}
	}

	// Sent again with a pair fixed by hand, that pair stays and the rest is
	// paired around it.
	again := req
	again.Fixed = []align.Fixed{{Source: []int{1, 2}, Target: []int{1}}}
	var re align.Result
	te.call("POST", "/api/align", te.ben, again, 200, &re)
	if len(re.Pairs) != 4 || !re.Pairs[1].Fixed || len(re.Pairs[1].SourceSegments) != 2 || re.Pairs[0].Fixed || len(re.Pairs[2].SourceSegments) != 0 || re.Pairs[2].TargetSegments[0] != 2 {
		t.Fatalf("around a fixed pair: %+v", re.Pairs)
	}
	again.Fixed = []align.Fixed{{Source: []int{9}, Target: []int{1}}}
	if r := te.do("POST", "/api/align", te.ben, again); r.status != 422 || r.err().Problems[0].Field != "fixed" {
		t.Fatalf("a fixed pair of a segment that is not there: %d %s", r.status, r.body)
	}

	// Sent with the order a review reads the translation in, the segments
	// are paired as read; the answer names them by their documents' positions.
	moved := req
	moved.Fixed = nil
	moved.Target.Data = []byte("<html><body><h1>Lease</h1><p>The tenancy begins on 1 March 2026 and runs for an indefinite period.</p><p>The tenant pays the rent by the third working day of each month. The deposit is EUR 2,400.00.</p></body></html>")
	moved.Order = align.Order{Target: []int{0, 2, 3, 1}}
	var mo align.Result
	te.call("POST", "/api/align", te.ben, moved, 200, &mo)
	if got := fmt.Sprint(mo.Pairs[1].TargetSegments, mo.Pairs[2].TargetSegments, mo.Pairs[3].TargetSegments); got != "[2] [3] [1]" {
		t.Fatalf("in the order read: %s %+v", got, mo.Pairs)
	}
	if mo.Target[1].Plain() != "The tenancy begins on 1 March 2026 and runs for an indefinite period." {
		t.Errorf("the segments stay in the document's order: %q", mo.Target[1].Plain())
	}
	moved.Order = align.Order{Target: []int{0, 1, 1, 3}}
	if r := te.do("POST", "/api/align", te.ben, moved); r.status != 422 || r.err().Problems[0].Field != "order" {
		t.Fatalf("an order naming a segment twice: %d %s", r.status, r.body)
	}

	// What can't be aligned says which file and why.
	for field, change := range map[string]func(*alignRequest){
		"source.name": func(r *alignRequest) { r.Source.Name = " " },
		"target.lang": func(r *alignRequest) { r.Target.Lang = "not a tag" },
		"source.data": func(r *alignRequest) { r.Source.Data = []byte("\n \n") },
		"target.data": func(r *alignRequest) { r.Target.Name = "Lease.docx" },
	} {
		bad := req
		change(&bad)
		if r := te.do("POST", "/api/align", te.ben, bad); r.status != 422 || r.err().Problems[0].Field != field {
			t.Errorf("%s: %d %s", field, r.status, r.body)
		}
	}
	bad := req
	bad.Target.Name = "Lease.sdlxliff"
	if r := te.do("POST", "/api/align", te.ben, bad); r.status != 422 || r.err().Message != "Lease.sdlxliff could not be read: a bilingual file holds both languages already: its translations are taken from it as they are." {
		t.Errorf("a bilingual file: %d %s", r.status, r.body)
	}

	// The pairs go into a memory together.
	var units struct {
		Units []unitInput `json:"units"`
	}
	for _, p := range al.Pairs {
		units.Units = append(units.Units, unitInput{Source: p.Source, Target: p.Target, Attributes: map[string]string{"file": "Mietvertrag.txt"}})
	}
	if r := te.do("POST", "/api/memories/Weber%20DE-EN/units/batch", te.ben, units); r.status != 404 {
		t.Fatalf("a translator without the memory adds to it: %d", r.status)
	}
	var before, after struct {
		Units int `json:"units"`
	}
	te.call("GET", "/api/memories/Weber%20DE-EN", te.pm, nil, 200, &before)
	var added struct {
		Added int `json:"added"`
	}
	te.call("POST", "/api/memories/Weber%20DE-EN/units/batch", te.pm, units, 201, &added)
	te.call("GET", "/api/memories/Weber%20DE-EN", te.pm, nil, 200, &after)
	if added.Added != 4 || after.Units != before.Units+4 {
		t.Fatalf("added %d, %d units before and %d after", added.Added, before.Units, after.Units)
	}
	// One unit that can't be stored and none is.
	units.Units[2].Target = content.Text(" ")
	if r := te.do("POST", "/api/memories/Weber%20DE-EN/units/batch", te.pm, units); r.status != 422 || r.err().Problems[0].Field != "units[2].target" {
		t.Fatalf("an empty translation: %d %s", r.status, r.body)
	}
	units.Units = nil
	if r := te.do("POST", "/api/memories/Weber%20DE-EN/units/batch", te.pm, units); r.status != 422 {
		t.Fatalf("no units: %d", r.status)
	}
	te.call("GET", "/api/memories/Weber%20DE-EN", te.pm, nil, 200, &after)
	if after.Units != before.Units+4 {
		t.Fatal(fmt.Sprint("units after the refused batches: ", after.Units))
	}
}
