package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm/tmdb"
)

func TestMemories(t *testing.T) {
	e := newEnv(t)
	_, admin := e.user("anna", true)
	ben, benTok := e.user("ben", false)
	_, carl := e.user("carl", false, authdb.Grant{Kind: authdb.KindMemory, Resource: "*", Role: authdb.RoleManage})

	if r := e.do("POST", "/api/memories", benTok, map[string]string{"name": "X", "sourceLang": "de", "targetLang": "en"}); r.status != 403 {
		t.Fatalf("a translator makes a memory: %d", r.status)
	}
	if r := e.do("POST", "/api/memories", carl, map[string]string{"name": "X"}); r.status != 422 {
		t.Fatalf("memory without languages: %d", r.status)
	}
	var m memoryView
	e.call("POST", "/api/memories", carl, map[string]string{"name": "Weber DE-EN", "sourceLang": "de-DE", "targetLang": "en-GB"}, 201, &m)
	if m.Role != "manage" {
		t.Fatalf("made %+v", m)
	}
	// A memory can be kept for a client.
	var kept memoryView
	e.call("POST", "/api/memories", carl, map[string]string{"name": "Hofmann DE-EN", "sourceLang": "de-DE", "targetLang": "en-GB", "client": " Hofmann GmbH "}, 201, &kept)
	if kept.Client != "Hofmann GmbH" {
		t.Fatalf("made for %q", kept.Client)
	}
	e.call("GET", "/api/memories/Hofmann%20DE-EN", carl, nil, 200, &kept)
	var all []memoryView
	e.call("GET", "/api/memories", carl, nil, 200, &all)
	if kept.Client != "Hofmann GmbH" || len(all) != 2 || all[0].Client != "Hofmann GmbH" || all[1].Client != "" {
		t.Fatalf("clients: %+v, %+v", kept, all)
	}
	e.call("DELETE", "/api/memories/Hofmann%20DE-EN", carl, nil, 204, nil)
	if r := e.do("POST", "/api/memories", carl, map[string]string{"name": "weber de-en", "sourceLang": "de-DE", "targetLang": "en-GB"}); r.status != 409 {
		t.Fatalf("same name: %d", r.status)
	}
	base := "/api/memories/Weber%20DE-EN"
	// Ben may read and add, after the manager lets him.
	if r := e.do("GET", base, benTok, nil); r.status != 404 {
		t.Fatalf("Ben sees the memory: %d", r.status)
	}
	e.call("PUT", "/api/grants", carl, authdb.Grant{UserID: ben.ID, Kind: authdb.KindMemory, Resource: "Weber DE-EN", Role: authdb.RoleRead}, 204, nil)
	if r := e.do("POST", base+"/units", benTok, unitInput{Source: content.Text("Miete"), Target: content.Text("rent")}); r.status != 403 {
		t.Fatalf("a reader adds: %d", r.status)
	}
	e.call("PUT", "/api/grants", carl, authdb.Grant{UserID: ben.ID, Kind: authdb.KindMemory, Resource: "Weber DE-EN", Role: authdb.RoleEdit}, 204, nil)
	var u unitView
	e.call("POST", base+"/units", benTok, unitInput{Source: content.Text("Der Mieter zahlt die Miete."), Target: content.Text("The tenant pays the rent."), Attributes: map[string]string{"client": "Weber"}}, 201, &u)
	if u.ID == 0 || u.CreatedBy != "ben" || u.Attributes["client"] != "Weber" || u.Version != 1 {
		t.Fatalf("unit %+v", u)
	}
	if r := e.do("POST", base+"/units", benTok, unitInput{Source: content.Text("Leer"), Target: content.Text(" ")}); r.status != 422 || r.err().Problems[0].Field != "target" {
		t.Fatalf("empty target: %d %s", r.status, r.body)
	}
	if r := e.do("POST", base+"/units?policy=maybe", benTok, unitInput{Source: content.Text("a"), Target: content.Text("b")}); r.status != 422 {
		t.Fatalf("bad policy: %d", r.status)
	}
	var ms []matchView
	e.call("POST", base+"/lookup", benTok, lookupRequest{Source: content.Text("Der Mieter zahlt Miete.")}, 200, &ms)
	if len(ms) != 1 || ms[0].Score < 70 || ms[0].Score >= 100 {
		t.Fatalf("fuzzy match %+v", ms)
	}
	var hits []concordanceHit
	e.call("POST", base+"/concordance", benTok, concordanceRequest{Query: "tenant", InTarget: true}, 200, &hits)
	if len(hits) != 1 {
		t.Fatalf("concordance %+v", hits)
	}
	// Updating needs the version.
	path := fmt.Sprintf("%s/units/%d", base, u.ID)
	var up unitView
	e.call("PUT", path+"?version=1", benTok, unitInput{Source: u.Source, Target: content.Text("The tenant pays rent.")}, 200, &up)
	if up.Version != 2 || up.Target.Plain() != "The tenant pays rent." {
		t.Fatalf("updated %+v", up)
	}
	if r := e.do("PUT", path+"?version=1", benTok, unitInput{Source: u.Source, Target: content.Text("x")}); r.status != 409 {
		t.Fatalf("stale update: %d", r.status)
	}
	e.call("GET", path, benTok, nil, 200, &up)
	// TMX out and back in.
	r := e.call("GET", base+"/export", benTok, nil, 200, nil)
	if !strings.Contains(string(r.body), "<tmx") || !strings.Contains(string(r.body), "The tenant pays rent.") || !strings.Contains(r.header.Get("Content-Disposition"), "Weber DE-EN.tmx") {
		t.Fatalf("export %s %v", r.body, r.header)
	}
	var other memoryView
	e.call("POST", "/api/memories", carl, map[string]string{"name": "Copy", "sourceLang": "de-DE", "targetLang": "en-GB"}, 201, &other)
	var st struct{ Read, Stored, Skipped int }
	e.call("POST", "/api/memories/Copy/import", carl, r.body, 200, &st, "Content-Type", "application/xml")
	if st.Read != 1 || st.Stored != 1 {
		t.Fatalf("import %+v", st)
	}
	if r := e.do("POST", "/api/memories/Copy/import", carl, []byte("not xml"), "Content-Type", "application/xml"); r.status != 422 {
		t.Fatalf("bad TMX: %d", r.status)
	}
	var changes struct {
		Changes []tmdb.Change `json:"changes"`
	}
	e.call("GET", base+"/changes", benTok, nil, 200, &changes)
	if len(changes.Changes) < 3 {
		t.Fatalf("changes %+v", changes)
	}
	var list []memoryView
	e.call("GET", "/api/memories", benTok, nil, 200, &list)
	if len(list) != 1 || list[0].Units != 1 || list[0].Role != "edit" {
		t.Fatalf("Ben's memories %+v", list)
	}
	e.call("DELETE", path, benTok, nil, 204, nil)
	if r := e.do("DELETE", base, benTok, nil); r.status != 403 {
		t.Fatalf("an editor deletes the memory: %d", r.status)
	}
	e.call("DELETE", base, admin, nil, 204, nil)
	if gs, _ := e.st.Auth.GrantsOn(ctx, authdb.KindMemory, "Weber DE-EN"); len(gs) != 0 {
		t.Fatalf("grants left %+v", gs)
	}
}

func entry(de, en string) term.Entry {
	return term.Entry{Languages: []term.Language{
		{Lang: "de-DE", Terms: []term.Term{{Text: de}}},
		{Lang: "en-GB", Terms: []term.Term{{Text: en}}},
	}}
}

func TestTermbases(t *testing.T) {
	e := newEnv(t)
	_, tbm := e.user("tess", false, authdb.Grant{Kind: authdb.KindTermbase, Resource: "*", Role: authdb.RoleManage})
	ben, benTok := e.user("ben", false)

	if r := e.do("POST", "/api/termbases", benTok, newTermbase{Definition: term.Definition{Name: "X", Languages: []string{"de"}}}); r.status != 403 {
		t.Fatalf("a translator makes a termbase: %d", r.status)
	}
	if r := e.do("POST", "/api/termbases", tbm, newTermbase{Definition: term.Definition{Name: " "}}); r.status != 422 {
		t.Fatalf("termbase without a name: %d", r.status)
	}
	// A master may start without languages; the first entry brings them.
	var bare termbaseDetail
	e.call("POST", "/api/termbases", tbm, newTermbase{Definition: term.Definition{Name: "Bare"}}, 201, &bare)
	if len(bare.Definition.Languages) != 0 {
		t.Fatalf("languages of a termbase made without any: %v", bare.Definition.Languages)
	}
	e.call("POST", "/api/termbases/Bare/entries", tbm, map[string]any{"entry": term.Entry{Languages: []term.Language{{Lang: "nl", Terms: []term.Term{{Text: "huur"}}}}}}, 201, nil)
	e.call("GET", "/api/termbases/Bare", tbm, nil, 200, &bare)
	if len(bare.Definition.Languages) != 1 || bare.Definition.Languages[0] != "nl" {
		t.Fatalf("languages after the first entry: %v", bare.Definition.Languages)
	}
	e.call("DELETE", "/api/termbases/Bare", tbm, nil, 204, nil)
	var tb termbaseDetail
	e.call("POST", "/api/termbases", tbm, newTermbase{Definition: term.Definition{Name: "Recht", Languages: []string{"de-DE", "en-GB"}}}, 201, &tb)
	if tb.Kind != "master" || tb.Role != "manage" {
		t.Fatalf("termbase %+v", tb)
	}
	base := "/api/termbases/Recht"
	e.call("PUT", "/api/grants", tbm, authdb.Grant{UserID: ben.ID, Kind: authdb.KindTermbase, Resource: "Recht", Role: authdb.RoleEdit}, 204, nil)
	var s termdb.Stored
	forged := entry("Mieter", "tenant")
	forged.CreatedBy, forged.CreatedAt = "someone else", time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	e.call("POST", base+"/entries", benTok, entryWrite{Entry: forged, Summary: "new"}, 201, &s)
	if s.ID == "" || s.Version != 1 || s.CreatedBy != "ben" || s.CreatedAt.Year() == 2001 {
		t.Fatalf("entry %+v", s)
	}
	if r := e.do("POST", base+"/entries", benTok, entryWrite{Entry: term.Entry{}}); r.status != 422 {
		t.Fatalf("entry without terms: %d %s", r.status, r.body)
	}
	ep := base + "/entries/" + s.ID
	up := entry("Mieter", "lessee")
	e.call("PUT", ep+"?version=1", benTok, entryWrite{Entry: up}, 200, &s)
	if r := e.do("PUT", ep+"?version=1", benTok, entryWrite{Entry: up}); r.status != 409 {
		t.Fatalf("stale entry: %d", r.status)
	}
	var hist []termdb.Version
	e.call("GET", ep+"/history", benTok, nil, 200, &hist)
	if len(hist) != 2 {
		t.Fatalf("history %+v", hist)
	}
	e.call("POST", ep+"/revert", benTok, map[string]int{"version": 1}, 200, &s)
	if s.Languages[1].Terms[0].Text != "tenant" {
		t.Fatalf("reverted %+v", s)
	}
	var found []termdb.Stored
	e.call("GET", base+"/lookup?text=MIETER&lang=de-AT", benTok, nil, 200, &found)
	if len(found) != 1 {
		t.Fatalf("lookup %+v", found)
	}
	e.call("POST", base+"/entries", benTok, entryWrite{Entry: entry("Mieter", "renter")}, 201, nil)
	var dups [][]string
	e.call("GET", base+"/duplicates?lang=de-DE", benTok, nil, 200, &dups)
	if len(dups) != 1 || len(dups[0]) != 2 {
		t.Fatalf("duplicates %+v", dups)
	}
	e.call("POST", base+"/merge", benTok, map[string]any{"keep": dups[0][0], "others": dups[0][1:]}, 200, &s)
	// Export and import.
	r := e.call("GET", base+"/export?format=tbx", benTok, nil, 200, nil)
	if !strings.Contains(string(r.body), "<tbx") || !strings.Contains(string(r.body), "renter") {
		t.Fatalf("export %s", r.body)
	}
	csv := e.call("GET", base+"/export?format=csv", benTok, nil, 200, nil)
	if !strings.HasPrefix(csv.header.Get("Content-Type"), "text/csv") {
		t.Fatalf("csv %v", csv.header)
	}
	if r := e.do("GET", base+"/export?format=pdf", benTok, nil); r.status != 422 {
		t.Fatalf("unknown format: %d", r.status)
	}
	e.call("POST", "/api/termbases", tbm, newTermbase{Definition: term.Definition{Name: "Copy", Languages: []string{"de-DE", "en-GB"}}}, 201, nil)
	var rep struct {
		Added    int  `json:"added"`
		Imported bool `json:"imported"`
	}
	e.call("POST", "/api/termbases/Copy/import?dryRun=1", tbm, r.body, 200, &rep, "Content-Type", "application/xml")
	if rep.Added != 1 || rep.Imported {
		t.Fatalf("dry run %+v", rep)
	}
	e.call("POST", "/api/termbases/Copy/import", tbm, r.body, 200, &rep, "Content-Type", "application/xml")
	if rep.Added != 1 || !rep.Imported {
		t.Fatalf("import %+v", rep)
	}
	if r := e.do("POST", "/api/termbases/Copy/import?onMatch=maybe", tbm, r.body); r.status != 422 {
		t.Fatalf("bad policy: %d", r.status)
	}
	// Chosen entries go out alone, and a field is renamed everywhere.
	e.call("POST", base+"/entries", benTok, map[string]any{"entry": term.Entry{Languages: []term.Language{{Lang: "de-DE", Terms: []term.Term{{Text: "Hausordnung"}}}}}}, 201, nil)
	var all struct{ Entries []termdb.Stored }
	e.call("GET", base+"/entries", benTok, nil, 200, &all)
	if len(all.Entries) < 2 {
		t.Fatalf("entries to choose from: %d", len(all.Entries))
	}
	part := e.call("POST", base+"/export?format=csv", benTok, map[string]any{"entries": []string{all.Entries[0].ID}}, 200, nil)
	if lines := strings.Count(strings.TrimSpace(string(part.body)), "\n"); lines != 1 {
		t.Fatalf("one entry and the header expected:\n%s", part.body)
	}
	if r := e.do("POST", base+"/export?format=csv", benTok, map[string]any{"entries": []string{}}); r.status != 422 {
		t.Fatalf("an export of no entries: %d", r.status)
	}
	var def termbaseDetail
	e.call("GET", base, tbm, nil, 200, &def)
	if len(def.Definition.Fields) > 0 {
		old := def.Definition.Fields[0].Name
		if r := e.do("PUT", base+"/fields/"+url.PathEscape(old), benTok, map[string]string{"name": "Renamed"}); r.status != 403 {
			t.Fatalf("an editor renames a field: %d", r.status)
		}
		e.call("PUT", base+"/fields/"+url.PathEscape(old), tbm, map[string]string{"name": "Renamed"}, 200, nil)
		e.call("GET", base, tbm, nil, 200, &def)
		if def.Definition.Fields[0].Name != "Renamed" {
			t.Fatalf("after renaming: %+v", def.Definition.Fields)
		}
		if r := e.do("PUT", base+"/fields/Renamed", tbm, map[string]string{"name": ""}); r.status != 422 {
			t.Fatalf("renaming to nothing: %d", r.status)
		}
		e.call("PUT", base+"/fields/Renamed", tbm, map[string]string{"name": old}, 200, nil)
	}

	// A spreadsheet is inspected first, then imported with the columns,
	// languages and default status the person chose.
	sheet := []byte("Deutsch,Englisch,Bemerkung\nKaution,deposit; security deposit,aus dem Vertrag\nNebenkosten,service charge,\n,,nur eine Bemerkung\n")
	var insp struct {
		Format  string
		Tabular bool
		Total   int
		Rows    [][]string
		Mapping *term.Mapping
		Read    term.Report
	}
	e.call("POST", "/api/termfiles/inspect?filename=liste.csv", benTok, sheet, 200, &insp, "Content-Type", "text/csv")
	if insp.Format != "csv" || !insp.Tabular || insp.Total != 4 || len(insp.Rows) != 4 || insp.Mapping == nil || len(insp.Mapping.Columns) < 3 {
		t.Fatalf("inspection %+v", insp)
	}
	mapping := term.Mapping{Header: true, Separator: ";", Columns: []term.Column{
		{Index: 0, Role: term.RoleTerm, Lang: "de"}, {Index: 1, Role: term.RoleTerm, Lang: "en"},
		{Index: 2, Role: term.RoleField, Field: "Bemerkung", Level: term.LevelEntry},
	}}
	mj, _ := json.Marshal(mapping)
	var read struct {
		Languages []string
		Fields    []term.Field
		Read      term.Report
		Dropped   int
		Sample    []string
	}
	e.call("POST", "/api/termfiles/inspect?filename=liste.csv&mapping="+url.QueryEscape(string(mj)), benTok, sheet, 200, &read, "Content-Type", "text/csv")
	if strings.Join(read.Languages, " ") != "de en" || len(read.Fields) != 1 || read.Read.Entries != 2 || read.Read.Skipped != 1 {
		t.Fatalf("read with a mapping %+v", read)
	}
	if len(read.Sample) != 2 || read.Sample[1] != "Nebenkosten, service charge" {
		t.Fatalf("sample %v", read.Sample)
	}
	e.call("POST", "/api/termfiles/inspect?filename=liste.csv&mapping="+url.QueryEscape(string(mj))+"&languages="+url.QueryEscape(`{"de":""}`), benTok, sheet, 200, &read, "Content-Type", "text/csv")
	if strings.Join(read.Languages, " ") != "de en" || read.Dropped != 0 || read.Sample[0] != "deposit, security deposit" {
		t.Fatalf("read with German left out %+v", read)
	}
	if r := e.do("POST", "/api/termfiles/inspect?filename=liste.csv&mapping=nonsense", benTok, sheet); r.status != 422 {
		t.Fatalf("a mapping that is not JSON: %d", r.status)
	}
	if r := e.do("POST", "/api/termfiles/inspect?filename=x.xml", benTok, []byte("<html></html>")); r.status != 422 {
		t.Fatalf("a file that is no termbase: %d", r.status)
	}
	if r := e.do("POST", "/api/termfiles/inspect?filename=liste.csv", "", sheet); r.status != 401 {
		t.Fatalf("inspecting without signing in: %d", r.status)
	}
	var planned struct {
		Added, Dropped int
		Imported       bool
		Plan           []struct{ Kind, Entry, Existing string }
	}
	opts := "&mapping=" + url.QueryEscape(string(mj)) + "&languages=" + url.QueryEscape(`{"de":"de-DE","en":"en-GB"}`) + "&status=proposed"
	e.call("POST", "/api/termbases/Copy/import?dryRun=1&filename=liste.csv"+opts, tbm, sheet, 200, &planned, "Content-Type", "text/csv")
	if planned.Added != 2 || planned.Imported || len(planned.Plan) != 2 || planned.Plan[0].Kind != "add" || planned.Plan[0].Entry != "Kaution, deposit, security deposit" {
		t.Fatalf("plan %+v", planned)
	}
	e.call("POST", "/api/termbases/Copy/import?filename=liste.csv"+opts, tbm, sheet, 200, &planned, "Content-Type", "text/csv")
	if planned.Added != 2 || !planned.Imported {
		t.Fatalf("import with options %+v", planned)
	}
	var kept struct{ Entries []termdb.Stored }
	e.call("GET", "/api/termbases/Copy/entries?text=Kaution", tbm, nil, 200, &kept)
	if len(kept.Entries) != 1 || kept.Entries[0].Languages[0].Lang != "de-DE" || kept.Entries[0].Languages[1].Lang != "en-GB" ||
		kept.Entries[0].Languages[1].Terms[1].Status != term.StatusProposed || len(kept.Entries[0].Fields) != 1 {
		t.Fatalf("imported entry %+v", kept.Entries)
	}
	// The same file again meets the entries that are there now; leaving
	// German out drops nothing, leaving both out drops everything.
	e.call("POST", "/api/termbases/Copy/import?dryRun=1&filename=liste.csv"+opts, tbm, sheet, 200, &planned, "Content-Type", "text/csv")
	if planned.Added != 0 || len(planned.Plan) != 2 || planned.Plan[0].Existing == "" {
		t.Fatalf("plan against existing entries %+v", planned)
	}
	none := "&mapping=" + url.QueryEscape(string(mj)) + "&languages=" + url.QueryEscape(`{"de":"","en":""}`)
	planned.Plan = nil
	e.call("POST", "/api/termbases/Copy/import?dryRun=1&filename=liste.csv"+none, tbm, sheet, 200, &planned, "Content-Type", "text/csv")
	if planned.Dropped != 2 || len(planned.Plan) != 0 {
		t.Fatalf("every language left out %+v", planned)
	}
	for _, bad := range []string{"&languages=x", "&languages=" + url.QueryEscape(`{"de":"not a tag!"}`), "&status=forbidden"} {
		if r := e.do("POST", "/api/termbases/Copy/import?dryRun=1&filename=liste.csv"+bad, tbm, sheet); r.status != 422 {
			t.Fatalf("%s: %d", bad, r.status)
		}
	}
	// A MultiTerm termbase file goes in as it is.
	sdltb, err := os.ReadFile("../term/testdata/orchard.sdltb")
	if err != nil {
		t.Fatal(err)
	}
	var orchard struct {
		Format    string
		Name      string
		Languages []string
		Read      term.Report
	}
	e.call("POST", "/api/termfiles/inspect?filename=orchard.sdltb", benTok, sdltb, 200, &orchard)
	if orchard.Format != "sdltb" || orchard.Name != "Orchard" || orchard.Read.Entries != 5 || len(orchard.Languages) != 5 {
		t.Fatalf("termbase file %+v", orchard)
	}

	// Definition and structure belong to managers.
	if r := e.do("PUT", base+"/definition", benTok, term.Definition{Languages: []string{"de-DE", "en-GB", "fr-FR"}}); r.status != 403 {
		t.Fatalf("an editor changes the definition: %d", r.status)
	}
	e.call("PUT", base+"/definition", tbm, term.Definition{Languages: []string{"de-DE", "en-GB", "fr-FR"}}, 200, &tb)
	if len(tb.Definition.Languages) != 3 {
		t.Fatalf("definition %+v", tb.Definition)
	}
	e.call("DELETE", base+"/languages/fr-FR", tbm, nil, 200, nil)
	var changes struct {
		Changes []termdb.Change `json:"changes"`
	}
	e.call("GET", base+"/changes", benTok, nil, 200, &changes)
	if len(changes.Changes) == 0 {
		t.Fatal("no changes")
	}
	e.call("DELETE", ep, benTok, nil, 204, nil)
	var deleted []termdb.Deletion
	e.call("GET", base+"/deleted", benTok, nil, 200, &deleted)
	if len(deleted) == 0 {
		t.Fatalf("deleted %+v", deleted)
	}
	var list []termbaseSummary
	e.call("GET", "/api/termbases", benTok, nil, 200, &list)
	if len(list) != 1 || list[0].Role != "edit" {
		t.Fatalf("Ben's termbases %+v", list)
	}
	e.call("DELETE", "/api/termbases/Copy", tbm, nil, 204, nil)
}

func TestMergeRequests(t *testing.T) {
	e := newEnv(t)
	_, tbm := e.user("tess", false, authdb.Grant{Kind: authdb.KindTermbase, Resource: "*", Role: authdb.RoleManage})
	ben, benTok := e.user("ben", false)
	rita, ritaTok := e.user("rita", false)
	e.call("POST", "/api/termbases", tbm, newTermbase{Definition: term.Definition{Name: "Recht", Languages: []string{"de-DE", "en-GB"}}}, 201, nil)
	var hof termbaseDetail
	e.call("POST", "/api/termbases", tbm, newTermbase{Definition: term.Definition{Name: "Hofmann"}, Upstreams: []string{"Recht"}}, 201, &hof)
	if len(hof.Definition.Languages) != 2 {
		t.Fatalf("a layered termbase has its master's languages: %+v", hof.Definition)
	}
	for _, g := range []authdb.Grant{
		{UserID: ben.ID, Kind: authdb.KindTermbase, Resource: "Hofmann", Role: authdb.RoleEdit},
		{UserID: ben.ID, Kind: authdb.KindTermbase, Resource: "Recht", Role: authdb.RoleSuggest},
		{UserID: rita.ID, Kind: authdb.KindTermbase, Resource: "Recht", Role: authdb.RoleApprove},
	} {
		e.call("PUT", "/api/grants", tbm, g, 204, nil)
	}
	e.call("POST", "/api/termbases/Hofmann/entries", benTok, entryWrite{Entry: entry("Kaution", "deposit")}, 201, nil)
	e.call("POST", "/api/termbases/Hofmann/entries", benTok, entryWrite{Entry: entry("Nebenkosten", "service charges")}, 201, nil)
	var layered []termdb.Layered
	e.call("GET", "/api/termbases/Hofmann/layered", benTok, nil, 200, &layered)
	if len(layered) != 2 {
		t.Fatalf("layered %+v", layered)
	}
	// Ben submits the project's terms to the master.
	var mr termdb.MergeRequest
	e.call("POST", "/api/termbases/Hofmann/merge-requests", benTok, map[string]string{"master": "Recht", "title": "Hofmann terms"}, 201, &mr)
	if mr.ID == 0 || mr.State != "open" {
		t.Fatalf("merge request %+v", mr)
	}
	mp := fmt.Sprintf("/api/merge-requests/%d", mr.ID)
	var items struct {
		Items []termdb.Item `json:"items"`
		Total int           `json:"total"`
	}
	e.call("GET", mp+"/items?state=waiting", ritaTok, nil, 200, &items)
	if items.Total != 2 {
		t.Fatalf("items %+v", items)
	}
	// Ben can't approve his own; Rita can.
	if r := e.do("POST", mp+"/decide", benTok, map[string]any{"filter": termdb.Filter{IDs: []int64{items.Items[0].ID}}, "decision": termdb.Decision{State: termdb.StateApproved}}); r.status != 403 {
		t.Fatalf("Ben approves: %d", r.status)
	}
	if r := e.do("POST", mp+"/decide", ritaTok, map[string]any{"decision": termdb.Decision{State: "fine"}}); r.status != 422 {
		t.Fatalf("unknown decision: %d", r.status)
	}
	// Reviewers are assigned by the master's managers, and must approve.
	if r := e.do("PUT", mp+"/reviewers", tbm, termdb.Assignment{Reviewer: "ben"}); r.status != 422 {
		t.Fatalf("Ben as reviewer: %d %s", r.status, r.body)
	}
	e.call("PUT", mp+"/reviewers", tbm, termdb.Assignment{Reviewer: "rita", Scope: termdb.Filter{Text: "kaution"}}, 204, nil)
	var revs []termdb.Assignment
	e.call("GET", mp+"/reviewers", benTok, nil, 200, &revs)
	if len(revs) != 1 || revs[0].Reviewer != "rita" {
		t.Fatalf("reviewers %+v", revs)
	}
	e.call("POST", mp+"/items", ritaTok, itemQuery{Reviewer: "rita"}, 200, &items)
	if items.Total != 1 {
		t.Fatalf("Rita's items %+v", items)
	}
	var res termdb.DecideResult
	e.call("POST", mp+"/decide", ritaTok, map[string]any{"filter": termdb.Filter{IDs: []int64{items.Items[0].ID}}, "decision": termdb.Decision{State: termdb.StateApproved}}, 200, &res)
	if res.Decided != 1 {
		t.Fatalf("decided %+v", res)
	}
	var sample []termdb.Item
	e.call("POST", mp+"/sample", ritaTok, map[string]any{"n": 5, "seed": 1}, 200, &sample)
	if len(sample) != 1 {
		t.Fatalf("sample %+v", sample)
	}
	var prog termdb.Progress
	e.call("GET", mp+"/progress", benTok, nil, 200, &prog)
	if prog.Total != 2 || prog.States[termdb.StateApproved] != 1 {
		t.Fatalf("progress %+v", prog)
	}
	var found []termdb.Stored
	e.call("GET", "/api/termbases/Recht/lookup?text=Kaution&lang=de", ritaTok, nil, 200, &found)
	if len(found) != 1 {
		t.Fatalf("approved entry in the master: %+v", found)
	}
	e.call("POST", mp+"/close", ritaTok, nil, 200, &mr)
	if mr.State != "closed" {
		t.Fatalf("closed %+v", mr)
	}
	if r := e.do("POST", mp+"/decide", ritaTok, map[string]any{"decision": termdb.Decision{State: termdb.StateRejected}}); r.status != 409 {
		t.Fatalf("deciding in a closed request: %d %s", r.status, r.body)
	}
	var mrs []termdb.MergeRequest
	e.call("GET", "/api/termbases/Hofmann/merge-requests", benTok, nil, 200, &mrs)
	if len(mrs) != 1 {
		t.Fatalf("requests %+v", mrs)
	}
	_, eve := e.user("eve", false)
	if r := e.do("GET", mp, eve, nil); r.status != 404 {
		t.Fatalf("Eve sees the request: %d", r.status)
	}
}
