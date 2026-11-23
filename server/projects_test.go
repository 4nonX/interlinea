package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm"
)

func segs(texts ...string) []project.Segment {
	out := make([]project.Segment, len(texts))
	for i, s := range texts {
		out[i] = project.Segment{ID: fmt.Sprintf("s%d", i+1), Source: content.Text(s), Status: project.StatusNew}
	}
	return out
}

// team is a server with a project manager, two translators, a reviewer
// and someone with no part in the work, and a master termbase and memory.
type team struct {
	*env
	admin, pm, ben, carl, dora, eve string
	benID, carlID, doraID, eveID    int64
}

func newTeam(t *testing.T) *team {
	e := newEnv(t)
	w := &team{env: e}
	_, w.admin = e.user("anna", true)
	_, w.pm = e.user("paula", false,
		authdb.Grant{Kind: authdb.KindProject, Resource: "*", Role: authdb.RoleManage},
		authdb.Grant{Kind: authdb.KindMemory, Resource: "*", Role: authdb.RoleManage},
		authdb.Grant{Kind: authdb.KindTermbase, Resource: "tb-weber", Role: authdb.RoleManage})
	var u authdb.User
	u, w.ben = e.user("ben", false)
	w.benID = u.ID
	u, w.carl = e.user("carl", false)
	w.carlID = u.ID
	u, w.dora = e.user("dora", false)
	w.doraID = u.ID
	u, w.eve = e.user("eve", false)
	w.eveID = u.ID
	if _, err := e.st.Termbases.CreateBy(ctx, term.Definition{Name: "tb-weber", Languages: []string{"de-DE", "en-GB", "fr-FR"}}, "test"); err != nil {
		t.Fatal(err)
	}
	m, err := e.st.Memories.Create(ctx, tm2("Weber DE-EN", "en-GB"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(ctx, tmUnit("Der Mieter zahlt die Miete.", "The tenant pays the rent."), 0); err != nil {
		t.Fatal(err)
	}
	return w
}

func tm2(name, target string) tm.Info {
	return tm.Info{Name: name, SourceLang: "de-DE", TargetLang: target}
}

func tmUnit(src, tgt string) tm.Unit {
	return tm.Unit{Source: content.Text(src), Target: content.Text(tgt), CreatedBy: "test"}
}

func leaseRequest() createProjectRequest {
	s := project.Defaults()
	s.Name = "Mietvertrag Hofmann"
	s.Client = "Kanzlei Weber"
	s.SourceLang = "de-DE"
	s.TargetLangs = []string{"en-GB", "fr-FR"}
	s.Memories = map[string][]project.MemorySetting{
		"en-GB": {
			{Memory: "Mietvertrag Hofmann (en-GB)", Lookup: true, Concordance: true, Update: true},
			{Memory: "Weber DE-EN", Lookup: true, Concordance: true, Penalty: 1},
		},
		"fr-FR": {{Memory: "Mietvertrag Hofmann (fr-FR)", Lookup: true, Update: true}},
	}
	s.Terminology = project.Terminology{Mode: project.Layered, Masters: []string{"tb-weber"}, Location: project.Server}
	lease := segs("Mietvertrag.", "Der Mieter zahlt die Miete.", "Die Kaution beträgt drei Monatsmieten.", "Der Vermieter haftet nicht.")
	return createProjectRequest{
		Settings: s,
		Files: []fileUpload{
			{Name: "Mietvertrag.txt", Role: project.Translatable, Format: "txt", Data: []byte("Mietvertrag.\n\nDer Mieter zahlt die Miete. Die Kaution beträgt drei Monatsmieten.\n\nDer Vermieter haftet nicht.\n"), Segments: map[string][]project.Segment{"en-GB": lease, "fr-FR": lease}},
			{Name: "Styleguide.pdf", Role: project.Reference, Format: "pdf", Data: []byte("%PDF-1.7 guide")},
		},
		Memories: []tm.Info{tm2("Mietvertrag Hofmann (en-GB)", "en-GB"), tm2("Mietvertrag Hofmann (fr-FR)", "fr-FR")},
		Termbase: &term.Definition{Name: "Mietvertrag Hofmann"},
	}
}

func (te *team) createLease() projectView {
	var p projectView
	te.call("POST", "/api/projects", te.pm, leaseRequest(), 201, &p)
	return p
}

func segPath(id int64, file, lang, seg string, version int) string {
	return fmt.Sprintf("/api/projects/%d/segments?file=%s&lang=%s&segment=%s&version=%d", id, url.QueryEscape(file), lang, seg, version)
}

func TestCreateProject(t *testing.T) {
	te := newTeam(t)
	if r := te.do("POST", "/api/projects", te.ben, leaseRequest()); r.status != 403 {
		t.Fatalf("a translator makes a project: %d", r.status)
	}
	p := te.createLease()
	if p.ID == 0 || p.Settings.Terminology.Termbase != "Mietvertrag Hofmann" || len(p.Settings.Files) != 2 || !p.Access.manage() {
		t.Fatalf("created %+v", p)
	}
	// Its memories and its termbase were made, layered over the master.
	for _, n := range []string{"Mietvertrag Hofmann (en-GB)", "Mietvertrag Hofmann (fr-FR)"} {
		if _, err := te.st.Memories.Memory(ctx, n); err != nil {
			t.Fatalf("memory %s: %v", n, err)
		}
	}
	var tb termbaseDetail
	te.call("GET", "/api/termbases/Mietvertrag%20Hofmann", te.pm, nil, 200, &tb)
	if tb.Kind != "project" || len(tb.Upstreams) != 1 || tb.Upstreams[0] != "tb-weber" || tb.Role != "manage" || len(tb.Definition.Languages) != 3 {
		t.Fatalf("project termbase %+v", tb)
	}
	// The same project again: the name is taken, and nothing is left over.
	again := leaseRequest()
	again.Memories = nil
	again.Termbase = nil
	again.Settings.Memories = map[string][]project.MemorySetting{}
	again.Settings.Terminology = project.Terminology{Mode: project.Own, Location: project.Local}
	if r := te.do("POST", "/api/projects", te.pm, again); r.status != 409 || r.err().Code != "exists" {
		t.Fatalf("same name: %d %s", r.status, r.body)
	}
	// A setup with problems names them, and makes nothing.
	bad := leaseRequest()
	bad.Settings.Name = "Other"
	bad.Memories = []tm.Info{tm2("Mietvertrag Hofmann (en-GB)", "en-GB")}
	if r := te.do("POST", "/api/projects", te.pm, bad); r.status != 409 || !strings.Contains(r.err().Message, "memory called") {
		t.Fatalf("taken memory: %d %s", r.status, r.body)
	}
	bad.Memories = []tm.Info{tm2("New EN", "en-GB")}
	bad.Settings.Memories = map[string][]project.MemorySetting{"en-GB": {{Memory: "Missing", Lookup: true}}}
	bad.Termbase = &term.Definition{Name: "Other TB"}
	r := te.do("POST", "/api/projects", te.pm, bad)
	if r.status != 422 || r.err().Problems[0].Field != "memories.en-GB[0].memory" {
		t.Fatalf("missing memory: %d %s", r.status, r.body)
	}
	if _, err := te.st.Memories.Memory(ctx, "New EN"); err == nil {
		t.Fatal("a memory was left over")
	}
	if _, err := te.st.Termbases.Termbase(ctx, "Other TB"); err == nil {
		t.Fatal("a termbase was left over")
	}
	// A project that fails after its memories were made removes them.
	clash := leaseRequest()
	clash.Settings.Name = "Mietvertrag Hofmann" // taken: the project store refuses
	clash.Memories = []tm.Info{tm2("Clash (en-GB)", "en-GB"), tm2("Clash (fr-FR)", "fr-FR")}
	clash.Settings.Memories = map[string][]project.MemorySetting{
		"en-GB": {{Memory: "Clash (en-GB)", Lookup: true, Update: true}},
		"fr-FR": {{Memory: "Clash (fr-FR)", Lookup: true, Update: true}},
	}
	clash.Termbase = &term.Definition{Name: "Clash TB"}
	if r := te.do("POST", "/api/projects", te.pm, clash); r.status != 409 {
		t.Fatalf("clash: %d %s", r.status, r.body)
	}
	if _, err := te.st.Memories.Memory(ctx, "Clash (en-GB)"); err == nil {
		t.Fatal("memories of a refused project kept")
	}
	if _, err := te.st.Termbases.Termbase(ctx, "Clash TB"); err == nil {
		t.Fatal("termbase of a refused project kept")
	}
	// A project termbase needs a mode and a location that have one.
	direct := leaseRequest()
	direct.Settings.Terminology = project.Terminology{Mode: project.Direct, Masters: []string{"tb-weber"}, WriteTo: "tb-weber"}
	if r := te.do("POST", "/api/projects", te.pm, direct); r.status != 422 || r.err().Problems[0].Field != "termbase" {
		t.Fatalf("termbase in direct mode: %d %s", r.status, r.body)
	}
}

func TestProjectAccessAndWork(t *testing.T) {
	te := newTeam(t)
	p := te.createLease()
	id := p.ID
	base := fmt.Sprintf("/api/projects/%d", id)

	// Without a grant or work, the project doesn't exist for you.
	if r := te.do("GET", base, te.ben, nil); r.status != 404 {
		t.Fatalf("someone else's project: %d", r.status)
	}
	var list []json.RawMessage
	te.call("GET", "/api/projects", te.ben, nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("Ben's list %s", list)
	}
	// Give out work: Ben translates English s1-s2, Carl the rest and
	// French; Dora reviews English.
	var benWork projectdb.Assignment
	te.call("POST", base+"/assignments", te.pm, projectdb.Assignment{File: "Mietvertrag.txt", TargetLang: "en-GB", FromSegment: "s1", ToSegment: "s2", UserID: te.benID, Task: projectdb.TaskTranslate, Due: "2026-12-01T12:00:00Z"}, 201, &benWork)
	if benWork.UserName != "Ben" {
		t.Fatalf("assigned %+v", benWork)
	}
	te.call("POST", base+"/assignments", te.pm, projectdb.Assignment{File: "Mietvertrag.txt", TargetLang: "en-GB", FromSegment: "s3", ToSegment: "s4", UserID: te.carlID, Task: projectdb.TaskTranslate}, 201, nil)
	te.call("POST", base+"/assignments", te.pm, projectdb.Assignment{TargetLang: "fr-FR", UserID: te.carlID, Task: projectdb.TaskTranslate}, 201, nil)
	te.call("POST", base+"/assignments", te.pm, projectdb.Assignment{TargetLang: "en-GB", UserID: te.doraID, Task: projectdb.TaskReview}, 201, nil)
	if r := te.do("POST", base+"/assignments", te.pm, projectdb.Assignment{File: "Mietvertrag.txt", TargetLang: "en-GB", FromSegment: "s2", ToSegment: "s3", UserID: te.eveID, Task: projectdb.TaskTranslate}); r.status != 422 {
		t.Fatalf("overlapping work: %d %s", r.status, r.body)
	}
	if r := te.do("POST", base+"/assignments", te.pm, projectdb.Assignment{UserID: 999, Task: projectdb.TaskReview}); r.status != 422 || r.err().Problems[0].Field != "user" {
		t.Fatalf("work for nobody: %d %s", r.status, r.body)
	}
	if r := te.do("POST", base+"/assignments", te.ben, projectdb.Assignment{UserID: te.eveID, Task: projectdb.TaskReview}); r.status != 403 {
		t.Fatalf("a translator gives out work: %d", r.status)
	}

	// Work makes the project visible.
	var got projectView
	te.call("GET", base, te.ben, nil, 200, &got)
	if !got.Access.Assigned || got.Access.edit() {
		t.Fatalf("Ben's access %+v", got.Access)
	}
	te.call("GET", "/api/projects", te.ben, nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("Ben's list %s", list)
	}
	var mine []projectdb.Assignment
	te.call("GET", "/api/me/assignments", te.ben, nil, 200, &mine)
	if len(mine) != 1 || mine[0].ID != benWork.ID {
		t.Fatalf("Ben's work %+v", mine)
	}
	if r := te.do("GET", base, te.eve, nil); r.status != 404 {
		t.Fatalf("Eve sees the project: %d", r.status)
	}

	// Files and segments.
	r := te.call("GET", base+"/files/content?name=styleguide.pdf", te.ben, nil, 200, nil)
	if string(r.body) != "%PDF-1.7 guide" || r.header.Get("Content-Type") != "application/pdf" || !strings.Contains(r.header.Get("Content-Disposition"), "Styleguide.pdf") {
		t.Fatalf("file %q %v", r.body, r.header)
	}
	var stored []projectdb.StoredSegment
	te.call("GET", base+"/segments?file=Mietvertrag.txt&lang=en-GB", te.ben, nil, 200, &stored)
	if len(stored) != 4 || stored[1].Version != 1 {
		t.Fatalf("segments %+v", stored)
	}

	// Ben translates his segments, not Carl's.
	var saved projectdb.StoredSegment
	te.call("PUT", segPath(id, "Mietvertrag.txt", "en-GB", "s1", 1), te.ben, project.Edit{Target: content.Text("Lease."), Status: project.StatusConfirmed}, 200, &saved)
	if saved.Version != 2 || saved.ChangedBy != "ben" {
		t.Fatalf("saved %+v", saved)
	}
	if r := te.do("PUT", segPath(id, "Mietvertrag.txt", "en-GB", "s3", 1), te.ben, project.Edit{Target: content.Text("Deposit."), Status: project.StatusDraft}); r.status != 403 {
		t.Fatalf("Ben saves Carl's segment: %d", r.status)
	}
	if r := te.do("PUT", segPath(id, "Mietvertrag.txt", "en-GB", "s1", 1), te.ben, project.Edit{Target: content.Text("Tenancy."), Status: project.StatusDraft}); r.status != 409 || r.err().Code != "conflict" {
		t.Fatalf("stale save: %d %s", r.status, r.body)
	}
	// The reviewer corrects any English segment.
	te.call("PUT", segPath(id, "Mietvertrag.txt", "en-GB", "s1", 2), te.dora, project.Edit{Target: content.Text("Tenancy agreement."), Status: project.StatusConfirmed}, 200, nil)
	if r := te.do("PUT", segPath(id, "Mietvertrag.txt", "fr-FR", "s1", 1), te.dora, project.Edit{Target: content.Text("Bail."), Status: project.StatusDraft}); r.status != 403 {
		t.Fatalf("the English reviewer saves French: %d", r.status)
	}
	// Confirmed translations went into the project memory, not the master
	// (which the project only looks up in).
	pm, _ := te.st.Memories.Memory(ctx, "Mietvertrag Hofmann (en-GB)")
	if n, _ := pm.Count(ctx); n != 2 {
		t.Fatalf("project memory has %d units", n)
	}
	master, _ := te.st.Memories.Memory(ctx, "Weber DE-EN")
	if n, _ := master.Count(ctx); n != 1 {
		t.Fatalf("master memory has %d units", n)
	}
	// Lookups and concordance through the project need no memory grants.
	var ms []matchView
	te.call("POST", base+"/lookup", te.ben, lookupRequest{Lang: "en-GB", Source: content.Text("Der Mieter zahlt die Miete.")}, 200, &ms)
	if len(ms) != 1 || ms[0].Score != 99 || ms[0].Memory != "Weber DE-EN" || ms[0].Unit.Target.Plain() != "The tenant pays the rent." {
		t.Fatalf("lookup %+v", ms)
	}
	var hits []concordanceHit
	te.call("POST", base+"/concordance", te.ben, concordanceRequest{Lang: "en-GB", Query: "Mietvertrag"}, 200, &hits)
	if len(hits) != 2 {
		t.Fatalf("concordance %+v", hits)
	}
	// The memories the project looks up in come whole, as TMX.
	tmx := te.call("GET", base+"/memories/weber%20de-en/export", te.ben, nil, 200, nil)
	if !strings.Contains(string(tmx.body), "The tenant pays the rent.") {
		t.Fatalf("memory for the project %s", tmx.body)
	}
	// Or only what matches the project's segments.
	m, _ := te.st.Memories.Memory(ctx, "Weber DE-EN")
	if _, err := m.Add(ctx, tmUnit("Ganz etwas anderes ohne jeden Bezug.", "Something else entirely."), 0); err != nil {
		t.Fatal(err)
	}
	ex := te.call("GET", base+"/memories/Weber%20DE-EN/extract?lang=en-gb", te.ben, nil, 200, nil)
	if !strings.Contains(string(ex.body), "The tenant pays the rent.") || strings.Contains(string(ex.body), "Something else") || ex.header.Get("Interlinea-Units") != "1" {
		t.Fatalf("extract %s %v", ex.body, ex.header)
	}
	if r := te.do("GET", base+"/memories/Weber%20DE-EN/extract?lang=fr-FR", te.ben, nil); r.status != 404 {
		t.Fatalf("extract for a language that doesn't use it: %d", r.status)
	}
	if r := te.do("GET", base+"/memories/Elsewhere/export", te.ben, nil); r.status != 404 {
		t.Fatalf("a memory the project doesn't use: %d", r.status)
	}
	if r := te.do("POST", "/api/memories/Weber%20DE-EN/lookup", te.ben, lookupRequest{Source: content.Text("Miete")}); r.status != 404 {
		t.Fatalf("direct lookup without a grant: %d", r.status)
	}

	// Ben reports his work done; he can't change anything else of it.
	benWork.Done = true
	benWork.ToSegment = "s4"
	var done projectdb.Assignment
	te.call("PUT", fmt.Sprintf("%s/assignments/%d?version=1", base, benWork.ID), te.ben, benWork, 200, &done)
	if !done.Done || done.ToSegment != "s2" {
		t.Fatalf("done %+v", done)
	}
	if r := te.do("PUT", fmt.Sprintf("%s/assignments/%d?version=2", base, benWork.ID), te.carl, done); r.status != 403 {
		t.Fatalf("Carl changes Ben's work: %d", r.status)
	}
	// The manager moves Ben's work to Eve, then takes it back.
	done.UserID = te.eveID
	var moved projectdb.Assignment
	te.call("PUT", fmt.Sprintf("%s/assignments/%d?version=2", base, benWork.ID), te.pm, done, 200, &moved)
	if moved.UserName != "Eve" {
		t.Fatalf("moved %+v", moved)
	}
	te.call("DELETE", fmt.Sprintf("%s/assignments/%d", base, benWork.ID), te.pm, nil, 204, nil)
	if r := te.do("GET", base, te.ben, nil); r.status != 404 {
		t.Fatalf("Ben still sees the project: %d", r.status)
	}
	if r := te.do("DELETE", fmt.Sprintf("%s/assignments/%d", base, benWork.ID), te.pm, nil); r.status != 404 {
		t.Fatalf("second removal: %d", r.status)
	}

	// Presence.
	var pr projectdb.Presence
	te.call("PUT", base+"/presence", te.carl, map[string]string{"file": "Mietvertrag.txt", "lang": "fr-FR", "segment": "s2"}, 200, &pr)
	var present []projectdb.Presence
	te.call("GET", base+"/presence?file=Mietvertrag.txt&lang=fr-FR", te.dora, nil, 200, &present)
	if len(present) != 1 || present[0].UserName != "Carl" || present[0].Segment != "s2" {
		t.Fatalf("presence %+v", present)
	}
	te.call("DELETE", base+"/presence", te.carl, nil, 204, nil)
	te.call("GET", base+"/presence", te.dora, nil, 200, &present)
	if len(present) != 0 {
		t.Fatalf("after leaving %+v", present)
	}

	// The change log.
	var log struct {
		Changes []projectdb.Change `json:"changes"`
		More    bool               `json:"more"`
	}
	te.call("GET", base+"/changes?since=0&limit=3", te.dora, nil, 200, &log)
	if len(log.Changes) != 3 || !log.More || log.Changes[0].Kind != projectdb.ChangeProjectCreated {
		t.Fatalf("log %+v", log)
	}
}

func TestProjectSettingsFilesAndPretranslation(t *testing.T) {
	te := newTeam(t)
	p := te.createLease()
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	if _, err := te.st.Auth.User(ctx, te.carlID); err != nil {
		t.Fatal(err)
	}
	// Editors save, but don't change settings.
	te.call("PUT", "/api/grants", te.pm, authdb.Grant{UserID: te.carlID, Kind: authdb.KindProject, Resource: fmt.Sprint(p.ID), Role: authdb.RoleEdit}, 204, nil)
	te.call("PUT", segPath(p.ID, "Mietvertrag.txt", "fr-FR", "s1", 1), te.carl, project.Edit{Target: content.Text("Bail."), Status: project.StatusDraft}, 200, nil)
	set := p.Settings
	set.Due = "2026-12-04T16:00:00Z"
	if r := te.do("PUT", base+"/settings?version=1", te.carl, set); r.status != 403 {
		t.Fatalf("an editor changes settings: %d", r.status)
	}
	var got projectView
	te.call("PUT", base+"/settings?version=1", te.pm, set, 200, &got)
	if got.Version != 2 || got.Settings.Due == "" {
		t.Fatalf("settings %+v", got)
	}
	set.Memories["en-GB"] = append(set.Memories["en-GB"], project.MemorySetting{Memory: "Missing", Lookup: true})
	if r := te.do("PUT", base+"/settings?version=2", te.pm, set); r.status != 422 {
		t.Fatalf("settings with problems: %d %s", r.status, r.body)
	}
	// Files.
	nf := fileUpload{Name: "Nachtrag.txt", Role: project.Translatable, Format: "txt", Data: []byte("Der Mieter zahlt die Miete.\n"),
		Segments: map[string][]project.Segment{"en-GB": segs("Der Mieter zahlt die Miete."), "fr-FR": segs("Der Mieter zahlt die Miete.")}}
	te.call("POST", base+"/files?version=2", te.pm, nf, 200, &got)
	if len(got.Settings.Files) != 3 {
		t.Fatalf("files %+v", got.Settings.Files)
	}
	// Pre-translation from the memories fills the 99% match.
	var filled []struct {
		Lang   string `json:"lang"`
		Filled int    `json:"filled"`
	}
	te.call("POST", base+"/pretranslate", te.pm, map[string]any{"lang": "en-GB", "prepare": project.Prepare{Pretranslate: true, MinScore: 90}}, 200, &filled)
	if len(filled) != 1 || filled[0].Filled != 2 {
		t.Fatalf("filled %+v", filled)
	}
	var stored []projectdb.StoredSegment
	te.call("GET", base+"/segments?file=Nachtrag.txt&lang=en-GB", te.pm, nil, 200, &stored)
	if stored[0].Target.Plain() != "The tenant pays the rent." || stored[0].Status != project.StatusDraft {
		t.Fatalf("pre-translated %+v", stored[0])
	}
	if r := te.do("POST", base+"/pretranslate", te.pm, map[string]any{"prepare": project.Prepare{MinScore: 10}}); r.status != 422 {
		t.Fatalf("bad minimum: %d", r.status)
	}
	te.call("DELETE", base+"/files?name=nachtrag.txt&version=3", te.pm, nil, 200, &got)
	if len(got.Settings.Files) != 2 {
		t.Fatalf("after removal %+v", got.Settings.Files)
	}
	// Memories and termbases a project uses aren't deleted under it.
	r := te.do("DELETE", "/api/memories/Mietvertrag%20Hofmann%20(en-GB)", te.pm, nil)
	if r.status != 409 || r.err().Code != "inUse" || !strings.Contains(r.err().Message, "Mietvertrag Hofmann") {
		t.Fatalf("deleting a used memory: %d %s", r.status, r.body)
	}
	if r := te.do("DELETE", "/api/termbases/tb-weber", te.pm, nil); r.status != 409 {
		t.Fatalf("deleting a used termbase: %d %s", r.status, r.body)
	}
	// Deleting the project takes its grants; its memories stay.
	if r := te.do("DELETE", base, te.carl, nil); r.status != 403 {
		t.Fatalf("an editor deletes the project: %d", r.status)
	}
	te.call("DELETE", base, te.pm, nil, 204, nil)
	if gs, _ := te.st.Auth.GrantsOn(ctx, authdb.KindProject, fmt.Sprint(p.ID)); len(gs) != 0 {
		t.Fatalf("grants left %+v", gs)
	}
	te.call("DELETE", "/api/memories/Mietvertrag%20Hofmann%20(en-GB)", te.pm, nil, 204, nil)
}

func TestProjectTerminology(t *testing.T) {
	te := newTeam(t)
	master, _ := te.st.Termbases.Termbase(ctx, "tb-weber")
	if _, err := master.Add(ctx, term.Entry{Languages: []term.Language{
		{Lang: "de-DE", Terms: []term.Term{{Text: "Mieter"}}},
		{Lang: "en-GB", Terms: []term.Term{{Text: "tenant"}}},
	}}, "test", ""); err != nil {
		t.Fatal(err)
	}
	p := te.createLease()
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	te.call("POST", base+"/assignments", te.pm, projectdb.Assignment{TargetLang: "en-GB", UserID: te.benID, Task: projectdb.TaskTranslate}, 201, nil)
	// A translator adds a term found while translating: it goes into the
	// project termbase, not the master.
	e := term.Entry{Languages: []term.Language{
		{Lang: "de-DE", Terms: []term.Term{{Text: "Kaution"}}},
		{Lang: "en-GB", Terms: []term.Term{{Text: "deposit"}}},
	}}
	var added struct {
		Termbase string        `json:"termbase"`
		Entry    termdb.Stored `json:"entry"`
	}
	te.call("POST", base+"/terms", te.ben, e, 201, &added)
	if added.Termbase != "Mietvertrag Hofmann" || added.Entry.ID == "" {
		t.Fatalf("added %+v", added)
	}
	if r := te.do("POST", base+"/terms", te.eve, e); r.status != 404 {
		t.Fatalf("Eve adds a term: %d", r.status)
	}
	var tbs []termbaseView
	te.call("GET", base+"/terminology", te.ben, nil, 200, &tbs)
	if len(tbs) != 1 || tbs[0].Name != "Mietvertrag Hofmann" || len(tbs[0].Entries) != 2 {
		t.Fatalf("terminology %+v", tbs)
	}
	if tbs[0].Entries[0].Termbase != "Mietvertrag Hofmann" || tbs[0].Entries[1].Termbase != "tb-weber" {
		t.Fatalf("layers %+v", tbs[0].Entries)
	}
	// Through the project, Ben edits the project termbase directly too.
	var search struct {
		Entries []termdb.Stored `json:"entries"`
		Total   int             `json:"total"`
	}
	te.call("GET", "/api/termbases/Mietvertrag%20Hofmann/entries?text=kau", te.ben, nil, 200, &search)
	if search.Total != 1 {
		t.Fatalf("search %+v", search)
	}
	if r := te.do("POST", "/api/termbases/tb-weber/entries", te.ben, entryWrite{Entry: e}); r.status != 404 {
		t.Fatalf("Ben writes into the master: %d", r.status)
	}
	// Renaming the master renames it in the project.
	var renamed termbaseDetail
	te.call("PUT", "/api/termbases/tb-weber/name", te.pm, map[string]string{"name": "Weber Recht"}, 200, &renamed)
	if renamed.Name != "Weber Recht" {
		t.Fatalf("renamed %+v", renamed)
	}
	var got projectView
	te.call("GET", base, te.pm, nil, 200, &got)
	if got.Settings.Terminology.Masters[0] != "Weber Recht" {
		t.Fatalf("project after the rename %+v", got.Settings.Terminology)
	}
	if r, _ := te.st.Auth.RoleOn(ctx, mustUser(t, te.env, "paula"), authdb.KindTermbase, "Weber Recht"); r != authdb.RoleManage {
		t.Fatalf("the manager's grant after the rename: %q", r)
	}
}

func mustUser(t *testing.T, e *env, name string) authdb.User {
	t.Helper()
	u, err := e.st.Auth.UserByName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestTemplatesAndCatalog(t *testing.T) {
	te := newTeam(t)
	tpl := project.TemplateFrom("Weber leases", leaseRequest().Settings)
	tpl.Memories = map[string][]project.MemorySetting{"en-GB": {{Memory: "Weber DE-EN", Lookup: true}}}
	if r := te.do("PUT", "/api/templates?version=0", te.ben, tpl); r.status != 403 {
		t.Fatalf("a translator keeps a template: %d", r.status)
	}
	var st projectdb.StoredTemplate
	te.call("PUT", "/api/templates?version=0", te.pm, tpl, 200, &st)
	if st.Version != 1 {
		t.Fatalf("template %+v", st)
	}
	var ts []projectdb.StoredTemplate
	te.call("GET", "/api/templates", te.ben, nil, 200, &ts)
	if len(ts) != 1 {
		t.Fatalf("templates %+v", ts)
	}
	// A template that uses the master keeps it from being deleted.
	if r := te.do("DELETE", "/api/memories/Weber%20DE-EN", te.pm, nil); r.status != 409 || !strings.Contains(r.err().Message, "template Weber leases") {
		t.Fatalf("deleting a memory a template uses: %d %s", r.status, r.body)
	}
	te.call("DELETE", "/api/templates?name=Weber%20leases", te.pm, nil, 204, nil)

	var cat struct {
		Memories  []catalogMemory   `json:"memories"`
		Termbases []catalogTermbase `json:"termbases"`
	}
	te.call("GET", "/api/catalog", te.pm, nil, 200, &cat)
	if len(cat.Memories) != 1 || cat.Memories[0].Units != 1 || len(cat.Termbases) != 1 || cat.Termbases[0].Role != "manage" || cat.Termbases[0].Kind != "master" {
		t.Fatalf("project manager's catalog %+v", cat)
	}
	te.call("GET", "/api/catalog", te.ben, nil, 200, &cat)
	if len(cat.Memories) != 0 || len(cat.Termbases) != 0 {
		t.Fatalf("translator's catalog %+v", cat)
	}
}
