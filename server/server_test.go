package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmdb"
)

var ctx = context.Background()

const pw = "a long enough password"

// env is a server on SQLite files in a temporary folder.
type env struct {
	t   *testing.T
	srv *Server
	hs  *httptest.Server
	st  Stores

	mu  sync.Mutex
	now time.Time
}

func (e *env) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func openStores(t *testing.T) Stores {
	t.Helper()
	dir := t.TempDir()
	var st Stores
	var err error
	if st.Auth, err = authdb.Open(filepath.Join(dir, "accounts.db")); err != nil {
		t.Fatal(err)
	}
	if st.Projects, err = projectdb.Open(filepath.Join(dir, "projects.db")); err != nil {
		t.Fatal(err)
	}
	if st.Memories, err = tmdb.Open(filepath.Join(dir, "memories.db")); err != nil {
		t.Fatal(err)
	}
	if st.Termbases, err = termdb.Open(filepath.Join(dir, "termbases.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		st.Auth.Close()
		st.Projects.Close()
		st.Memories.Close()
		st.Termbases.Close()
	})
	return st
}

func newEnv(t *testing.T, configure ...func(*Config)) *env {
	t.Helper()
	e := &env{t: t, st: openStores(t), now: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)}
	e.hs = httptest.NewUnstartedServer(nil)
	cfg := Config{Stores: e.st, PublicURL: "https://il.example", Version: "test", Now: e.clock, EventPoll: 20 * time.Millisecond}
	for _, c := range configure {
		c(&cfg)
	}
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.srv = srv
	e.hs.Config.Handler = srv
	e.hs.Start()
	t.Cleanup(func() {
		srv.Close()
		e.hs.Close()
	})
	return e
}

type reply struct {
	status int
	header http.Header
	body   []byte
}

func (r reply) err() apiError {
	var a apiError
	json.Unmarshal(r.body, &a)
	return a
}

// do sends a request; body is JSON unless it is []byte. Extra headers
// come in pairs.
func (e *env) do(method, path, token string, body any, headers ...string) reply {
	e.t.Helper()
	var rd io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		js, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(js)
		ct = "application/json"
	}
	req, err := http.NewRequest(method, e.hs.URL+path, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return reply{res.StatusCode, res.Header, b}
}

// call sends a request, checks the status and decodes the answer.
func (e *env) call(method, path, token string, body any, want int, out any, headers ...string) reply {
	e.t.Helper()
	r := e.do(method, path, token, body, headers...)
	if r.status != want {
		e.t.Fatalf("%s %s: %d %s, want %d", method, path, r.status, r.body, want)
	}
	if out != nil {
		if err := json.Unmarshal(r.body, out); err != nil {
			e.t.Fatalf("%s %s: %v in %s", method, path, err, r.body)
		}
	}
	return r
}

// user makes an account and signs it in.
func (e *env) user(name string, admin bool, grants ...authdb.Grant) (authdb.User, string) {
	e.t.Helper()
	u, err := e.st.Auth.CreateUser(ctx, authdb.NewUser{Username: name, Name: strings.ToUpper(name[:1]) + name[1:], Password: pw, Admin: admin}, "test")
	if err != nil {
		e.t.Fatal(err)
	}
	for _, g := range grants {
		g.UserID = u.ID
		if err := e.st.Auth.SetGrant(ctx, g, "test"); err != nil {
			e.t.Fatal(err)
		}
	}
	var res signinResult
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: name, Password: pw, Device: "test"}, 200, &res)
	return res.User, res.Token
}

func TestSetupAndSignIn(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.SetupToken = "setup-secret" })
	var info serverInfo
	e.call("GET", "/api/server", "", nil, 200, &info)
	if !info.Setup || info.Version != "test" || info.Provider != "" {
		t.Fatalf("server info %+v", info)
	}
	if r := e.do("POST", "/api/setup", "", setupRequest{SetupToken: "wrong", Username: "anna", Password: pw}); r.status != 403 {
		t.Fatalf("wrong setup token: %d", r.status)
	}
	if r := e.do("POST", "/api/setup", "", setupRequest{SetupToken: "setup-secret", Username: "anna"}); r.status != 422 {
		t.Fatalf("setup without a password: %d", r.status)
	}
	var first signinResult
	e.call("POST", "/api/setup", "", setupRequest{SetupToken: "setup-secret", Username: "Anna", Name: "Anna Weber", Password: pw, Device: "Firefox"}, 200, &first)
	if !first.User.Admin || first.User.Username != "anna" || !strings.HasPrefix(first.Token, "il_") {
		t.Fatalf("first administrator %+v", first)
	}
	if r := e.do("POST", "/api/setup", "", setupRequest{SetupToken: "setup-secret", Username: "bob", Password: pw}); r.status != 409 {
		t.Fatalf("second setup: %d", r.status)
	}
	e.call("GET", "/api/server", "", nil, 200, &info)
	if info.Setup {
		t.Fatal("still in setup")
	}

	var me meResult
	e.call("GET", "/api/me", first.Token, nil, 200, &me)
	if me.User.Name != "Anna Weber" || me.Grants == nil || me.Identities == nil {
		t.Fatalf("me %+v", me)
	}
	// Wrong passwords, a missing token, a bad one.
	r := e.do("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: "not it at all"})
	if r.status != 401 || r.err().Code != "credentials" {
		t.Fatalf("wrong password: %d %s", r.status, r.body)
	}
	if r := e.do("GET", "/api/me", "", nil); r.status != 401 || r.header.Get("WWW-Authenticate") == "" {
		t.Fatalf("no token: %d", r.status)
	}
	if r := e.do("GET", "/api/me", "il_nonsense", nil); r.status != 401 {
		t.Fatalf("bad token: %d", r.status)
	}
	// Signing out ends the token.
	var other signinResult
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: pw, Device: "Laptop"}, 200, &other)
	var tokens []struct {
		authdb.Token
		Current bool `json:"current"`
	}
	e.call("GET", "/api/me/tokens", other.Token, nil, 200, &tokens)
	if len(tokens) != 2 {
		t.Fatalf("tokens %+v", tokens)
	}
	e.call("POST", "/api/auth/signout", other.Token, nil, 204, nil)
	if r := e.do("GET", "/api/me", other.Token, nil); r.status != 401 {
		t.Fatalf("after signing out: %d", r.status)
	}
	e.call("GET", "/api/me", first.Token, nil, 200, nil)
}

func TestSignInRateLimits(t *testing.T) {
	e := newEnv(t)
	e.user("anna", false)
	wrong := passwordSignin{Username: "anna", Password: "wrong wrong wrong"}
	for i := 0; i < 10; i++ {
		if r := e.do("POST", "/api/auth/password", "", wrong); r.status != 401 {
			t.Fatalf("attempt %d: %d", i, r.status)
		}
	}
	// The account is locked for a while, even with the right password.
	r := e.do("POST", "/api/auth/password", "", passwordSignin{Username: "ANNA", Password: pw})
	if r.status != 429 || r.header.Get("Retry-After") == "" {
		t.Fatalf("eleventh attempt: %d %s", r.status, r.body)
	}
	e.advance(15 * time.Minute)
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: pw}, 200, nil)
	// One address gets 20 attempts a minute, whoever they are for.
	e.advance(time.Minute)
	for i := 0; i < 20; i++ {
		e.do("POST", "/api/auth/password", "", passwordSignin{Username: fmt.Sprintf("nobody%d", i), Password: "x"})
	}
	if r := e.do("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: pw}); r.status != 429 {
		t.Fatalf("21st attempt from one address: %d", r.status)
	}
	e.advance(time.Minute)
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: pw}, 200, nil)
}

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(2, time.Minute, func() time.Time { return now })
	l.add("a")
	now = now.Add(20 * time.Second)
	l.add("a")
	if w := l.wait("a"); w != 40*time.Second {
		t.Fatalf("wait %v", w)
	}
	if l.wait("b") != 0 {
		t.Fatal("another key waits")
	}
	now = now.Add(40 * time.Second)
	if l.wait("a") != 0 {
		t.Fatal("the window passed")
	}
	l.add("a")
	l.reset("a")
	if l.wait("a") != 0 || len(l.seen) != 0 {
		t.Fatalf("after reset %v", l.seen)
	}
	l.add("c")
	now = now.Add(2 * time.Minute)
	l.wait("d")
	if len(l.seen) != 0 {
		t.Fatalf("old keys kept: %v", l.seen)
	}
}

func TestOwnPassword(t *testing.T) {
	e := newEnv(t)
	_, tok := e.user("ben", false)
	r := e.do("PUT", "/api/me/password", tok, map[string]string{"current": "wrong", "new": "another long password"})
	if r.status != 422 || r.err().Problems[0].Field != "current" {
		t.Fatalf("wrong current password: %d %s", r.status, r.body)
	}
	if r := e.do("PUT", "/api/me/password", tok, map[string]string{"current": pw, "new": "short"}); r.status != 422 {
		t.Fatalf("short password: %d", r.status)
	}
	var res signinResult
	e.call("PUT", "/api/me/password", tok, map[string]string{"current": pw, "new": "another long password"}, 200, &res)
	if r := e.do("GET", "/api/me", tok, nil); r.status != 401 {
		t.Fatal("the old token still works")
	}
	e.call("GET", "/api/me", res.Token, nil, 200, nil)
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: "ben", Password: "another long password"}, 200, nil)
}

func TestAdministration(t *testing.T) {
	e := newEnv(t)
	anna, admin := e.user("anna", true)
	_, ben := e.user("ben", false)

	if r := e.do("GET", "/api/users", ben, nil); r.status != 403 {
		t.Fatalf("users for a non-administrator: %d", r.status)
	}
	var carl authdb.User
	e.call("POST", "/api/users", admin, newUserRequest{Username: "carl", Name: "Carl Roth", Email: "carl@example.com", Password: pw}, 201, &carl)
	if r := e.do("POST", "/api/users", admin, newUserRequest{Username: "CARL", Password: pw}); r.status != 409 || r.err().Code != "exists" {
		t.Fatalf("taken username: %d %s", r.status, r.body)
	}
	if r := e.do("POST", "/api/users", admin, newUserRequest{Username: "dora!", Password: pw}); r.status != 422 || r.err().Problems[0].Field != "username" {
		t.Fatalf("bad username: %d %s", r.status, r.body)
	}
	var us []authdb.User
	e.call("GET", "/api/users", admin, nil, 200, &us)
	if len(us) != 3 {
		t.Fatalf("users %+v", us)
	}
	// Changes need the version they were made from.
	name := "Carl Roth-Berg"
	if r := e.do("PATCH", fmt.Sprintf("/api/users/%d", carl.ID), admin, userPatch{Name: &name}); r.status != 428 {
		t.Fatalf("change without a version: %d", r.status)
	}
	var changed authdb.User
	e.call("PATCH", fmt.Sprintf("/api/users/%d?version=1", carl.ID), admin, userPatch{Name: &name}, 200, &changed)
	if changed.Name != name || changed.Version != 2 {
		t.Fatalf("changed %+v", changed)
	}
	if r := e.do("PATCH", fmt.Sprintf("/api/users/%d", carl.ID), admin, userPatch{Name: &name}, "If-Match", `"1"`); r.status != 409 || r.err().Code != "conflict" {
		t.Fatalf("stale change: %d %s", r.status, r.body)
	}
	// Disabling signs out everywhere.
	var carlSignin signinResult
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: "carl", Password: pw}, 200, &carlSignin)
	yes := true
	e.call("PATCH", fmt.Sprintf("/api/users/%d", carl.ID), admin, userPatch{Disabled: &yes}, 200, nil, "If-Match", "2")
	if r := e.do("GET", "/api/me", carlSignin.Token, nil); r.status != 401 {
		t.Fatalf("disabled account still signed in: %d", r.status)
	}
	if r := e.do("POST", "/api/auth/password", "", passwordSignin{Username: "carl", Password: pw}); r.status != 403 {
		t.Fatalf("disabled account signs in: %d", r.status)
	}
	// The last administrator stays.
	no := false
	if r := e.do("PATCH", fmt.Sprintf("/api/users/%d", anna.ID), admin, userPatch{Admin: &no}, "If-Match", "1"); r.status != 409 || r.err().Code != "lastAdmin" {
		t.Fatalf("demoting the last administrator: %d %s", r.status, r.body)
	}
	// Passwords and identities.
	e.call("PUT", fmt.Sprintf("/api/users/%d/password", carl.ID), admin, map[string]string{"password": "a fresh long password"}, 204, nil)
	e.call("POST", fmt.Sprintf("/api/users/%d/identities", carl.ID), admin, map[string]string{"issuer": "https://login.example", "subject": "c-1"}, 204, nil)
	var ids []authdb.Identity
	e.call("GET", fmt.Sprintf("/api/users/%d/identities", carl.ID), admin, nil, 200, &ids)
	if len(ids) != 1 || ids[0].Subject != "c-1" {
		t.Fatalf("identities %+v", ids)
	}
	e.call("DELETE", fmt.Sprintf("/api/users/%d/identities?issuer=%s&subject=c-1", carl.ID, "https%3A%2F%2Flogin.example"), admin, nil, 204, nil)
	e.call("GET", fmt.Sprintf("/api/users/%d/tokens", carl.ID), admin, nil, 200, nil)
	e.call("DELETE", fmt.Sprintf("/api/users/%d/tokens", carl.ID), admin, nil, 204, nil)
	e.call("DELETE", fmt.Sprintf("/api/users/%d", carl.ID), admin, nil, 204, nil)
	if r := e.do("GET", fmt.Sprintf("/api/users/%d", carl.ID), admin, nil); r.status != 404 {
		t.Fatalf("deleted account: %d", r.status)
	}
	if r := e.do("GET", "/api/users/abc", admin, nil); r.status != 404 {
		t.Fatalf("bad id: %d", r.status)
	}
	// Everyone signed in sees who work can go to.
	var people []person
	e.call("GET", "/api/people", ben, nil, 200, &people)
	if len(people) != 2 || people[0].Username != "anna" {
		t.Fatalf("people %+v", people)
	}
}

func TestGrants(t *testing.T) {
	e := newEnv(t)
	_, admin := e.user("anna", true)
	ben, benTok := e.user("ben", false)
	carl, _ := e.user("carl", false)
	if _, err := e.st.Memories.Create(ctx, tmInfo("Weber DE-EN")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Memories.Create(ctx, tmInfo("Other DE-EN")); err != nil {
		t.Fatal(err)
	}
	// Administrators give any role.
	e.call("PUT", "/api/grants", admin, authdb.Grant{UserID: ben.ID, Kind: authdb.KindMemory, Resource: "Weber DE-EN", Role: authdb.RoleManage}, 204, nil)
	if r := e.do("PUT", "/api/grants", admin, authdb.Grant{UserID: ben.ID, Kind: authdb.KindMemory, Resource: "Missing", Role: authdb.RoleRead}); r.status != 404 {
		t.Fatalf("grant on a missing memory: %d", r.status)
	}
	if r := e.do("PUT", "/api/grants", admin, authdb.Grant{UserID: ben.ID, Kind: authdb.KindProject, Resource: "abc", Role: authdb.RoleRead}); r.status != 422 {
		t.Fatalf("grant on a project by name: %d", r.status)
	}
	// Managers decide who may use what they manage, nothing else.
	e.call("PUT", "/api/grants", benTok, authdb.Grant{UserID: carl.ID, Kind: authdb.KindMemory, Resource: "Weber DE-EN", Role: authdb.RoleEdit}, 204, nil)
	if r := e.do("PUT", "/api/grants", benTok, authdb.Grant{UserID: carl.ID, Kind: authdb.KindMemory, Resource: "Other DE-EN", Role: authdb.RoleRead}); r.status != 403 {
		t.Fatalf("grant on something not managed: %d", r.status)
	}
	if r := e.do("PUT", "/api/grants", benTok, authdb.Grant{UserID: carl.ID, Kind: authdb.KindMemory, Resource: "*", Role: authdb.RoleRead}); r.status != 403 {
		t.Fatalf("grant on all memories by a manager: %d", r.status)
	}
	var on []grantView
	e.call("GET", "/api/grants?kind=memory&resource=Weber%20DE-EN", benTok, nil, 200, &on)
	if len(on) != 2 || on[1].Username != "carl" || on[1].Role != authdb.RoleEdit {
		t.Fatalf("grants on the memory %+v", on)
	}
	e.call("DELETE", fmt.Sprintf("/api/grants?userId=%d&kind=memory&resource=Weber%%20DE-EN", carl.ID), benTok, nil, 204, nil)
	if r := e.do("DELETE", fmt.Sprintf("/api/grants?userId=%d&kind=memory&resource=Weber%%20DE-EN", carl.ID), benTok, nil); r.status != 404 {
		t.Fatalf("second revoke: %d", r.status)
	}
	var gs []authdb.Grant
	e.call("GET", fmt.Sprintf("/api/users/%d/grants", ben.ID), admin, nil, 200, &gs)
	if len(gs) != 1 {
		t.Fatalf("Ben's grants %+v", gs)
	}
}

func TestRoutesNeedSignIn(t *testing.T) {
	e := newEnv(t)
	for _, route := range []string{
		"GET /api/me", "GET /api/people", "GET /api/users", "GET /api/catalog", "GET /api/projects", "POST /api/projects",
		"GET /api/projects/1", "PUT /api/projects/1/segments", "GET /api/memories", "POST /api/memories/x/lookup",
		"GET /api/termbases", "GET /api/termbases/x/entries", "GET /api/merge-requests/1", "GET /api/events", "GET /api/templates",
	} {
		method, path, _ := strings.Cut(route, " ")
		if r := e.do(method, path, "", nil); r.status != 401 {
			t.Errorf("%s without signing in: %d", route, r.status)
		}
	}
	if r := e.do("GET", "/api/nothing", "", nil); r.status != 404 || r.err().Code != "notFound" {
		t.Fatalf("unknown route: %d %s", r.status, r.body)
	}
	if r := e.do("GET", "/healthz", "", nil); r.status != 200 {
		t.Fatalf("health: %d", r.status)
	}
}

func TestRequestChecks(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.Origins = []string{"http://localhost:5173"} })
	if r := e.do("POST", "/api/auth/password", "", []byte(`{"username": "a"`), "Content-Type", "application/json"); r.status != 400 {
		t.Fatalf("broken JSON: %d", r.status)
	}
	if r := e.do("POST", "/api/auth/password", "", []byte(`{"username": "a", "extra": 1}`), "Content-Type", "application/json"); r.status != 400 {
		t.Fatalf("unknown field: %d", r.status)
	}
	if r := e.do("POST", "/api/auth/password", "", []byte(`username=a`), "Content-Type", "application/x-www-form-urlencoded"); r.status != 415 {
		t.Fatalf("a form: %d", r.status)
	}
	big := bytes.Repeat([]byte("a"), maxBody+10)
	if r := e.do("POST", "/api/auth/password", "", append(append([]byte(`{"username": "`), big...), '"', '}'), "Content-Type", "application/json"); r.status != 413 {
		t.Fatalf("too large: %d", r.status)
	}
	// Allowed web apps get CORS answers; others don't.
	r := e.do("OPTIONS", "/api/projects", "", nil, "Origin", "http://localhost:5173", "Access-Control-Request-Method", "POST")
	if r.status != 204 || r.header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" || !strings.Contains(r.header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("preflight: %d %v", r.status, r.header)
	}
	if r := e.do("GET", "/api/server", "", nil, "Origin", "https://evil.example"); r.header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS for another origin")
	}
	if r := e.do("GET", "/api/server", "", nil); r.header.Get("Cache-Control") != "no-store" || r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers %v", r.header)
	}
}

func TestConfig(t *testing.T) {
	st := openStores(t)
	for _, c := range []Config{
		{},
		{Stores: st, PublicURL: "ftp://x"},
		{Stores: st, Origins: []string{"http://a.example/path"}},
		{Stores: st, OIDC: &OIDC{Name: "x"}},
	} {
		if _, err := New(c); err == nil {
			t.Errorf("%+v accepted", c)
		}
	}
}

func tmInfo(name string) tm.Info {
	return tm.Info{Name: name, SourceLang: "de-DE", TargetLang: "en-GB"}
}

func TestWebApp(t *testing.T) {
	web := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>Interlinea</title>")},
		"assets/app-1.js":      {Data: []byte("console.log(1)")},
		"core/interlinea.wasm": {Data: []byte("\x00asm")},
	}
	e := newEnv(t, func(c *Config) { c.Web = web })
	r := e.do("GET", "/", "", nil)
	if r.status != 200 || !strings.Contains(string(r.body), "<title>Interlinea") || r.header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("page: %d %v", r.status, r.header)
	}
	// The app's own addresses get the page, so reloading works.
	if r := e.do("GET", "/projects/12/editor", "", nil); r.status != 200 || !strings.Contains(string(r.body), "Interlinea") {
		t.Fatalf("app address: %d", r.status)
	}
	if r := e.do("GET", "/assets/app-1.js", "", nil); r.status != 200 || !strings.Contains(r.header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %v", r.status, r.header)
	}
	if r := e.do("GET", "/core/interlinea.wasm", "", nil); r.status != 200 || r.header.Get("Content-Type") != "application/wasm" {
		t.Fatalf("wasm: %d %v", r.status, r.header)
	}
	// The API stays the API.
	if r := e.do("GET", "/api/unknown", "", nil); r.status != 404 || r.err().Code != "notFound" {
		t.Fatalf("API under the web app: %d", r.status)
	}
}
