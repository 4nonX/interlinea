package server

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/internal/sqlitedb"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
)

// Data written by a release must open in every later one. Each release
// leaves its data folder in testdata/releases/<version>, with a list of
// what is in it; TestUpgrade opens a copy of each with the code as it is
// now, which runs the migrations, and checks through the API that the
// data is all there and can be worked on.
//
// When a version is released, its folder is written once and kept:
//
//	go test ./server -run TestKeepData -keep-data 0.1.0
var keepData = flag.String("keep-data", "", "write the data folder of this version to testdata/releases")

var dataFiles = []string{"accounts.db", "projects.db", "memories.db", "termbases.db"}

// kept says what a kept data folder holds, for the check after an upgrade.
type kept struct {
	Version  string   `json:"version"`
	Accounts []string `json:"accounts"`
	// Roles defined beside the five that come with the server.
	Roles []string `json:"roles"`
	// Grants as "user kind resource role".
	Grants  []string `json:"grants"`
	Project string   `json:"project"`
	Files   []string `json:"files"`
	// Segments of the first file in en-GB, as "id status target".
	Segments []string `json:"segments"`
	// Memories as "name units".
	Memories []string `json:"memories"`
	// A search of the main memory and the translation it finds.
	MemoryQuery, MemoryTarget string
	// Termbases as "name kind entries".
	Termbases []string `json:"termbases"`
	// An entry of the master with its history, and one that was deleted.
	Entry        string   `json:"entry"`
	EntryHistory []string `json:"entryHistory"`
	Deleted      string   `json:"deleted"`
	// The merge request, as "title state", and its items as "state".
	MergeRequest string   `json:"mergeRequest"`
	MergeItems   []string `json:"mergeItems"`
	MinorFields  []string `json:"minorFields"`
}

// describe reads, through the API, what kept lists.
func describe(t *testing.T, e *env, admin string) kept {
	t.Helper()
	var k kept
	var users []authdb.User
	e.call("GET", "/api/users", admin, nil, 200, &users)
	for _, u := range users {
		k.Accounts = append(k.Accounts, u.Username)
		var gs []authdb.Grant
		e.call("GET", fmt.Sprintf("/api/users/%d/grants", u.ID), admin, nil, 200, &gs)
		for _, g := range gs {
			k.Grants = append(k.Grants, fmt.Sprintf("%s %s %s %s", u.Username, g.Kind, g.Resource, g.Role))
		}
	}
	sort.Strings(k.Accounts)
	sort.Strings(k.Grants)
	var roles []authdb.RoleDef
	e.call("GET", "/api/roles", admin, nil, 200, &roles)
	for _, r := range roles {
		if !r.BuiltIn {
			var ps []string
			for _, p := range r.Permissions {
				ps = append(ps, string(p))
			}
			k.Roles = append(k.Roles, r.Name+" "+strings.Join(ps, "+"))
		}
	}
	var projects []projectView
	e.call("GET", "/api/projects", admin, nil, 200, &projects)
	if len(projects) != 1 {
		t.Fatalf("projects: %+v", projects)
	}
	p := projects[0]
	k.Project = p.Settings.Name
	for _, f := range p.Settings.Files {
		k.Files = append(k.Files, f.Name)
	}
	var segments []projectdb.StoredSegment
	e.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=%s&lang=en-GB", p.ID, k.Files[0]), admin, nil, 200, &segments)
	for _, s := range segments {
		k.Segments = append(k.Segments, strings.TrimSpace(fmt.Sprintf("%s %s %s", s.ID, s.Status, s.Target.Plain())))
	}
	var memories []struct {
		Name  string `json:"name"`
		Units int    `json:"units"`
	}
	e.call("GET", "/api/memories", admin, nil, 200, &memories)
	for _, m := range memories {
		k.Memories = append(k.Memories, fmt.Sprintf("%s %d", m.Name, m.Units))
	}
	sort.Strings(k.Memories)
	k.MemoryQuery = "Mieter"
	var hits []concordanceHit
	e.call("POST", "/api/memories/Weber%20DE-EN/concordance", admin, concordanceRequest{Query: k.MemoryQuery}, 200, &hits)
	if len(hits) > 0 {
		k.MemoryTarget = hits[0].Unit.Target.Plain()
	}
	var termbases []termbaseSummary
	e.call("GET", "/api/termbases", admin, nil, 200, &termbases)
	for _, tb := range termbases {
		k.Termbases = append(k.Termbases, fmt.Sprintf("%s %s %d", tb.Name, tb.Kind, tb.Entries))
	}
	sort.Strings(k.Termbases)
	var found struct {
		Entries []termdb.Stored `json:"entries"`
	}
	e.call("GET", "/api/termbases/tb-weber/entries?text=Kaution", admin, nil, 200, &found)
	if len(found.Entries) != 1 {
		t.Fatalf("the entry Kaution: %+v", found)
	}
	entry := found.Entries[0]
	k.Entry = fmt.Sprintf("%s v%d %s", entry.ID, entry.Version, termsOf(entry.Entry))
	var history []termdb.Version
	e.call("GET", "/api/termbases/tb-weber/entries/"+entry.ID+"/history", admin, nil, 200, &history)
	for _, v := range history {
		k.EntryHistory = append(k.EntryHistory, fmt.Sprintf("v%d %s: %s", v.Version, v.By, termsOf(v.Entry)))
	}
	var deleted []termdb.Deletion
	e.call("GET", "/api/termbases/tb-weber/deleted", admin, nil, 200, &deleted)
	if len(deleted) == 1 {
		k.Deleted = termsOf(deleted[0].Entry)
	}
	var mrs []termdb.MergeRequest
	e.call("GET", "/api/termbases/tb-weber/merge-requests", admin, nil, 200, &mrs)
	if len(mrs) != 1 {
		t.Fatalf("merge requests: %+v", mrs)
	}
	k.MergeRequest = mrs[0].Title + " " + mrs[0].State
	var items struct {
		Items []termdb.Item `json:"items"`
	}
	e.call("POST", fmt.Sprintf("/api/merge-requests/%d/items", mrs[0].ID), admin, itemQuery{}, 200, &items)
	for _, it := range items.Items {
		k.MergeItems = append(k.MergeItems, it.Kind+" "+it.State+" "+termsOf(it.Entry))
	}
	sort.Strings(k.MergeItems)
	var rules termdb.ReviewRules
	e.call("GET", "/api/termbases/tb-weber/review-rules", admin, nil, 200, &rules)
	k.MinorFields = rules.MinorFields
	return k
}

func termsOf(e term.Entry) string {
	var parts []string
	for _, l := range e.Languages {
		for _, tm := range l.Terms {
			parts = append(parts, l.Lang+"="+tm.Text+"/"+string(tm.Status))
		}
	}
	return strings.Join(parts, " ")
}

func signIn(e *env, name string) string {
	var res signinResult
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: name, Password: pw, Device: "test"}, 200, &res)
	return res.Token
}

// TestKeepData writes the data folder of a version, with one of
// everything the stores keep.
func TestKeepData(t *testing.T) {
	if *keepData == "" {
		t.Skip("writes testdata/releases/<version>; run with -keep-data <version>")
	}
	to := filepath.Join("testdata", "releases", *keepData)
	if _, err := os.Stat(to); err == nil {
		t.Fatalf("%s is there already: a release's data is written once and kept", to)
	}
	dir := t.TempDir()
	e := newEnvIn(t, dir)
	_, admin := e.user("anna", true)
	_, pm := e.user("paula", false,
		authdb.Grant{Kind: authdb.KindProject, Resource: "*", Role: authdb.RoleManage},
		authdb.Grant{Kind: authdb.KindMemory, Resource: "*", Role: authdb.RoleManage},
		authdb.Grant{Kind: authdb.KindTermbase, Resource: "*", Role: authdb.RoleManage})
	ben, benTok := e.user("ben", false)

	// A role of one's own, given on the master.
	e.call("PUT", "/api/roles/Reviewer", admin, map[string]any{"permissions": []string{"read", "suggest", "approve"}}, 200, nil)

	// A master with a field, entries, a changed entry and a deleted one.
	def := term.Definition{Name: "tb-weber", Languages: []string{"de-DE", "en-GB", "fr-FR"}, Fields: []term.Field{{Name: "Note", Level: term.LevelEntry, Kind: term.KindText}}}
	e.call("POST", "/api/termbases", pm, newTermbase{Definition: def}, 201, nil)
	e.call("PUT", "/api/grants", admin, authdb.Grant{UserID: ben.ID, Kind: authdb.KindTermbase, Resource: "tb-weber", Role: "Reviewer"}, 204, nil)
	var kaution, miete termdb.Stored
	e.call("POST", "/api/termbases/tb-weber/entries", pm, entryWrite{Entry: entry("Kaution", "deposit")}, 201, &kaution)
	changed := entry("Kaution", "security deposit")
	changed.ID = kaution.ID
	e.call("PUT", fmt.Sprintf("/api/termbases/tb-weber/entries/%s?version=1", kaution.ID), pm, entryWrite{Entry: changed, Summary: "The usual term"}, 200, nil)
	e.call("POST", "/api/termbases/tb-weber/entries", pm, entryWrite{Entry: entry("Miete", "rent")}, 201, &miete)
	e.call("DELETE", "/api/termbases/tb-weber/entries/"+miete.ID, pm, nil, 204, nil)
	e.call("PUT", "/api/termbases/tb-weber/review-rules", pm, termdb.ReviewRules{MinorFields: []string{"Note"}}, 200, nil)

	// A main memory, and a project with its own memories and termbase.
	m, err := e.st.Memories.Create(ctx, tm2("Weber DE-EN", "en-GB"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(ctx, tmUnit("Der Mieter zahlt die Miete.", "The tenant pays the rent."), 0); err != nil {
		t.Fatal(err)
	}
	var p projectView
	e.call("POST", "/api/projects", pm, leaseRequest(), 201, &p)
	e.call("PUT", "/api/grants", admin, authdb.Grant{UserID: ben.ID, Kind: authdb.KindProject, Resource: fmt.Sprint(p.ID), Role: authdb.RoleEdit}, 204, nil)
	e.call("PUT", segPath(p.ID, "Mietvertrag.txt", "en-GB", "s1", 1), benTok, project.Edit{Target: content.Text("Lease."), Status: project.StatusConfirmed}, 200, nil)
	e.call("PUT", segPath(p.ID, "Mietvertrag.txt", "en-GB", "s2", 1), benTok, project.Edit{Target: content.Text("The tenant pays the rent."), Status: project.StatusDraft}, 200, nil)

	// The project's terms: one new, submitted to the master and approved;
	// one more that waits.
	ptb := "/api/termbases/" + strings.ReplaceAll(p.Settings.Terminology.Termbase, " ", "%20")
	e.call("POST", ptb+"/entries", pm, entryWrite{Entry: entry("Nebenkosten", "service charges")}, 201, nil)
	var mr termdb.MergeRequest
	e.call("POST", ptb+"/merge-requests", pm, map[string]string{"master": "tb-weber", "title": "Hofmann terms"}, 201, &mr)
	e.call("POST", fmt.Sprintf("/api/merge-requests/%d/decide", mr.ID), benTok, map[string]any{"decision": termdb.Decision{State: termdb.StateApproved, Comment: "Fine"}}, 200, nil)

	want := describe(t, e, admin)
	want.Version = *keepData

	// The files as a backup writes them: whole, without a log beside them.
	e.srv.Close()
	e.hs.Close()
	e.st.Auth.Close()
	e.st.Projects.Close()
	e.st.Memories.Close()
	e.st.Termbases.Close()
	if err := os.MkdirAll(to, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range dataFiles {
		db, err := sql.Open("sqlite3", sqlitedb.FileDSN(filepath.ToSlash(filepath.Join(dir, f))))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`VACUUM INTO ?`, filepath.Join(to, f)); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	js, _ := json.MarshalIndent(want, "", "  ")
	if err := os.WriteFile(filepath.Join(to, "kept.json"), append(js, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", to)
}

// TestUpgrade opens the data of every release with the code as it is and
// finds everything in it, then works on it.
func TestUpgrade(t *testing.T) {
	releases, err := os.ReadDir(filepath.Join("testdata", "releases"))
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) == 0 {
		t.Fatal("no kept data: see TestKeepData")
	}
	for _, rel := range releases {
		t.Run(rel.Name(), func(t *testing.T) {
			from := filepath.Join("testdata", "releases", rel.Name())
			var want kept
			js, err := os.ReadFile(filepath.Join(from, "kept.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(js, &want); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for _, f := range dataFiles {
				data, err := os.ReadFile(filepath.Join(from, f))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, f), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// Opening runs the migrations from the release's schema to this one.
			e := newEnvIn(t, dir)
			admin := signIn(e, "anna")
			got := describe(t, e, admin)
			got.Version = want.Version
			a, _ := json.MarshalIndent(want, "", "  ")
			b, _ := json.MarshalIndent(got, "", "  ")
			if string(a) != string(b) {
				t.Fatalf("the data of %s after the upgrade\nwas:\n%s\nis:\n%s", rel.Name(), a, b)
			}

			// And it is worked on as before: a translation, a unit, an
			// entry restored, a deleted one brought back, an account.
			benTok := signIn(e, "ben")
			var projects []projectView
			e.call("GET", "/api/projects", admin, nil, 200, &projects)
			var saved projectdb.StoredSegment
			e.call("PUT", segPath(projects[0].ID, "Mietvertrag.txt", "en-GB", "s2", 2), benTok, project.Edit{Target: content.Text("The tenant pays the rent."), Status: project.StatusConfirmed}, 200, &saved)
			if saved.Version != 3 || saved.Status != project.StatusConfirmed {
				t.Fatalf("saved after the upgrade: %+v", saved)
			}
			pm := signIn(e, "paula")
			e.call("POST", "/api/memories/Weber%20DE-EN/units", pm, unitInput{Source: content.Text("Die Kaution ist fällig."), Target: content.Text("The deposit is due.")}, 201, nil)
			var found struct {
				Entries []termdb.Stored `json:"entries"`
			}
			e.call("GET", "/api/termbases/tb-weber/entries?text=Kaution", admin, nil, 200, &found)
			var back termdb.Stored
			e.call("POST", "/api/termbases/tb-weber/entries/"+found.Entries[0].ID+"/revert", pm, map[string]int{"version": 1}, 200, &back)
			if back.Version != 3 || !strings.Contains(termsOf(back.Entry), "en-GB=deposit/") {
				t.Fatalf("restored after the upgrade: %+v", back)
			}
			var deleted []termdb.Deletion
			e.call("GET", "/api/termbases/tb-weber/deleted", admin, nil, 200, &deleted)
			e.call("POST", "/api/termbases/tb-weber/deleted/"+deleted[0].EntryID+"/restore", pm, nil, 201, nil)
			e.call("POST", "/api/users", admin, map[string]any{"username": "carla", "name": "Carla", "password": pw}, 201, nil)
			e.call("PUT", "/api/roles/Reader%20plus", admin, map[string]any{"permissions": []string{"read", "suggest"}}, 200, nil)
		})
	}
}
