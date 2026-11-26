package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/term"
)

// A project's managers choose its memories and termbases, but only among
// those they may use themselves: a project is no way into a memory or a
// termbase they have no grant on.
func TestProjectSettingsNeedAccessToWhatTheyUse(t *testing.T) {
	te := newTeam(t)
	p := te.createLease()
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	// Pete manages this one project and nothing else.
	_, pete := te.user("pete", false, authdb.Grant{Kind: authdb.KindProject, Resource: strconv.FormatInt(p.ID, 10), Role: authdb.RoleManage})
	secret, err := te.st.Memories.Create(ctx, tm2("Geheim", "en-GB"))
	if err != nil {
		t.Fatal(err)
	}
	secret.Add(ctx, tmUnit("Geheime Klausel.", "Secret clause."), 0)
	if _, err := te.st.Termbases.CreateBy(ctx, term.Definition{Name: "Geheim-TB", Languages: []string{"de-DE", "en-GB"}}, "test"); err != nil {
		t.Fatal(err)
	}

	settings := func() (project.Settings, int) {
		var got projectView
		te.call("GET", base, pete, nil, 200, &got)
		return got.Settings, got.Version
	}

	// A memory he may not read.
	set, v := settings()
	set.Memories["en-GB"] = append(set.Memories["en-GB"], project.MemorySetting{Memory: "Geheim", Lookup: true, Concordance: true})
	if r := te.do("PUT", fmt.Sprintf("%s/settings?version=%d", base, v), pete, set); r.status != 403 {
		t.Fatalf("a memory he may not read: %d %s", r.status, r.body)
	}
	// Read on it lets him look it up, not store into it.
	if err := te.st.Auth.SetGrant(ctx, authdb.Grant{UserID: mustUser(t, te.env, "pete").ID, Kind: authdb.KindMemory, Resource: "Geheim", Role: authdb.RoleRead}, "test"); err != nil {
		t.Fatal(err)
	}
	te.call("PUT", fmt.Sprintf("%s/settings?version=%d", base, v), pete, set, 200, nil)
	set, v = settings()
	for i, m := range set.Memories["en-GB"] {
		if m.Memory == "Geheim" {
			set.Memories["en-GB"][i].Update = true
		}
	}
	if r := te.do("PUT", fmt.Sprintf("%s/settings?version=%d", base, v), pete, set); r.status != 403 {
		t.Fatalf("storing into a memory he may only read: %d %s", r.status, r.body)
	}

	// A termbase he has no grant on, as a master or as the project's own.
	set, v = settings()
	set.Terminology.Masters = append(set.Terminology.Masters, "Geheim-TB")
	if r := te.do("PUT", fmt.Sprintf("%s/settings?version=%d", base, v), pete, set); r.status != 403 {
		t.Fatalf("a master he may not read: %d %s", r.status, r.body)
	}
	set, v = settings()
	set.Terminology.Termbase = "Geheim-TB"
	if r := te.do("PUT", fmt.Sprintf("%s/settings?version=%d", base, v), pete, set); r.status != 403 {
		t.Fatalf("another termbase as the project's own: %d %s", r.status, r.body)
	}
	if r := te.do("DELETE", "/api/termbases/Geheim-TB", pete, nil); r.status < 400 {
		t.Fatalf("he deleted a termbase he had no grant on: %d", r.status)
	}

	// What the project used before he came still works for him: he changes
	// the deadline without a grant on the project's memories.
	set, v = settings()
	set.Description = "Neue Frist"
	te.call("PUT", fmt.Sprintf("%s/settings?version=%d", base, v), pete, set, 200, nil)

	// Those who make projects choose among all memories and masters, as
	// the catalog offers them, but take no other termbase as a project's
	// own: Mia manages all projects and nothing else.
	_, mia := te.user("mia", false, authdb.Grant{Kind: authdb.KindProject, Resource: "*", Role: authdb.RoleManage})
	var cat struct {
		Memories []catalogMemory `json:"memories"`
	}
	te.call("GET", "/api/catalog", mia, nil, 200, &cat)
	offered := false
	for _, m := range cat.Memories {
		offered = offered || m.Name == "Geheim"
	}
	if !offered {
		t.Fatalf("the catalog leaves out a memory: %+v", cat.Memories)
	}
	var got projectView
	te.call("GET", base, mia, nil, 200, &got)
	set = got.Settings
	for i, m := range set.Memories["en-GB"] {
		if m.Memory == "Geheim" {
			set.Memories["en-GB"][i].Update = true
		}
	}
	set.Terminology.Masters = append(set.Terminology.Masters, "Geheim-TB")
	te.call("PUT", fmt.Sprintf("%s/settings?version=%d", base, got.Version), mia, set, 200, &got)
	set = got.Settings
	set.Terminology.Termbase = "Geheim-TB"
	if r := te.do("PUT", fmt.Sprintf("%s/settings?version=%d", base, got.Version), mia, set); r.status != 403 {
		t.Fatalf("a master as a project's own termbase: %d %s", r.status, r.body)
	}
}

// A translation service's secret header goes to its own host only: a
// redirect elsewhere is not followed.
func TestProviderRedirectsStayOnTheHost(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("X-Key") != "")
	}))
	defer other.Close()
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/moved" {
			w.Write([]byte("here"))
			return
		}
		if r.URL.Query().Get("to") == "self" {
			http.Redirect(w, r, "/moved", http.StatusFound)
			return
		}
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer service.Close()
	c := &http.Client{CheckRedirect: sameHost}
	req, _ := http.NewRequest("GET", service.URL, nil)
	req.Header.Set("X-Key", "secret")
	if _, err := c.Do(req); err == nil || leaked.Load() {
		t.Fatalf("followed to another host: %v, leaked %v", err, leaked.Load())
	}
	req, _ = http.NewRequest("GET", service.URL+"?to=self", nil)
	req.Header.Set("X-Key", "secret")
	if res, err := c.Do(req); err != nil || res.StatusCode != 200 {
		t.Fatalf("a redirect on the same host: %v", err)
	}
}
