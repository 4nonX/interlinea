package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/auth/oidc"
)

// idp is an identity provider for the tests: it signs in whoever the test
// says, as its login page would.
type idp struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu    sync.Mutex
	codes map[string]idpCode
}

type idpCode struct {
	challenge, redirect string
	claims              map[string]any
}

func newIDP(t *testing.T) *idp {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &idp{t: t, key: k, codes: map[string]idpCode{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": p.srv.URL, "authorization_endpoint": p.srv.URL + "/authorize", "token_endpoint": p.srv.URL + "/token",
			"jwks_uri": p.srv.URL + "/keys", "code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		b := func(x *big.Int) string { return base64.RawURLEncoding.EncodeToString(x.Bytes()) }
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig", "n": b(k.N), "e": b(big.NewInt(int64(k.E)))}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		p.mu.Lock()
		c, ok := p.codes[r.Form.Get("code")]
		delete(p.codes, r.Form.Get("code"))
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge || r.Form.Get("redirect_uri") != c.redirect {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id_token": p.sign(c.claims), "token_type": "Bearer"})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *idp) sign(claims map[string]any) string {
	hb, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	cb, _ := json.Marshal(claims)
	signed := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		p.t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// login is the person signing in at the provider: it returns the
// address the provider sends the browser back to.
func (p *idp) login(flowURL, subject, username string, extra map[string]any) string {
	p.t.Helper()
	u, err := url.Parse(flowURL)
	if err != nil {
		p.t.Fatal(err)
	}
	q := u.Query()
	now := time.Now().Unix()
	claims := map[string]any{"iss": p.srv.URL, "sub": subject, "aud": "interlinea", "exp": now + 300, "iat": now,
		"nonce": q.Get("nonce"), "preferred_username": username, "name": "Eva Brandt", "email": username + "@agentur.example", "email_verified": true}
	for k, v := range extra {
		claims[k] = v
	}
	code := "c-" + q.Get("state")[:10]
	p.mu.Lock()
	p.codes[code] = idpCode{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), claims: claims}
	p.mu.Unlock()
	return q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&code=" + code
}

func oidcEnv(t *testing.T, create bool) (*env, *idp) {
	t.Helper()
	p := newIDP(t)
	var e *env
	e = newEnv(t, func(c *Config) {
		c.Origins = []string{"http://localhost:5173"}
		prov, err := oidc.Discover(ctx, oidc.Config{Issuer: p.srv.URL, ClientID: "interlinea", RedirectURL: "https://il.example/api/auth/oidc/callback"})
		if err != nil {
			t.Fatal(err)
		}
		c.OIDC = &OIDC{Name: "Agentur", Provider: prov, CreateUsers: create}
	})
	return e, p
}

// signInThrough runs a provider sign-in and returns where the browser
// ends up.
func (e *env) signInThrough(p *idp, back, subject, username string) *url.URL {
	e.t.Helper()
	var start map[string]string
	e.call("POST", "/api/auth/oidc/start", "", oidcStartRequest{Return: back, Device: "Desktop"}, 200, &start)
	return e.followCallback(p.login(start["url"], subject, username, nil))
}

func (e *env) followCallback(cb string) *url.URL {
	e.t.Helper()
	u, _ := url.Parse(cb)
	r := e.do("GET", u.Path+"?"+u.RawQuery, "", nil)
	if r.status != http.StatusSeeOther {
		e.t.Fatalf("callback: %d %s", r.status, r.body)
	}
	to, err := url.Parse(r.header.Get("Location"))
	if err != nil {
		e.t.Fatal(err)
	}
	return to
}

func fragment(u *url.URL, key string) string {
	v, _ := url.ParseQuery(u.Fragment)
	return v.Get(key)
}

func TestProviderSignIn(t *testing.T) {
	e, p := oidcEnv(t, false)
	var info serverInfo
	e.call("GET", "/api/server", "", nil, 200, &info)
	if info.Provider != "Agentur" {
		t.Fatalf("info %+v", info)
	}
	// Return addresses: the server, allowed web apps, apps on this
	// computer; nothing else.
	for _, bad := range []string{"https://evil.example/", "http://192.168.1.5:8000/", "javascript:alert(1)", "https://il.example/#x", "http://user@localhost/"} {
		if r := e.do("POST", "/api/auth/oidc/start", "", oidcStartRequest{Return: bad}); r.status != 422 {
			t.Errorf("return %q: %d", bad, r.status)
		}
	}
	// Nobody is linked to this subject yet.
	to := e.signInThrough(p, "http://127.0.0.1:43123/done", "eva-1", "eva")
	if to.Host != "127.0.0.1:43123" || !strings.Contains(fragment(to, "interlinea-error"), "No account") {
		t.Fatalf("unlinked: %v", to)
	}
	// Eva links her provider sign-in to her account.
	eva, evaTok := e.user("eva", false)
	var start map[string]string
	e.call("POST", "/api/me/identities", evaTok, oidcStartRequest{Return: "http://localhost:5173/settings"}, 200, &start)
	to = e.followCallback(p.login(start["url"], "eva-1", "eva", nil))
	link := fragment(to, "interlinea-link")
	if link == "" || to.Path != "/settings" {
		t.Fatalf("link: %v", to)
	}
	// The provider's answer alone links nothing: Eva's account confirms.
	if _, err := e.st.Auth.UserByIdentity(ctx, p.srv.URL, "eva-1"); err == nil {
		t.Fatal("linked before Eva confirmed")
	}
	_, otherTok := e.user("otto", false)
	if r := e.do("POST", "/api/me/identities/confirm", otherTok, map[string]string{"code": link}); r.status != 403 {
		t.Fatalf("another account confirms Eva's link: %d %s", r.status, r.body)
	}
	// That attempt used the code up; Eva links again and confirms.
	e.call("POST", "/api/me/identities", evaTok, oidcStartRequest{Return: "http://localhost:5173/settings"}, 200, &start)
	to = e.followCallback(p.login(start["url"], "eva-1", "eva", nil))
	var linked map[string]string
	e.call("POST", "/api/me/identities/confirm", evaTok, map[string]string{"code": fragment(to, "interlinea-link")}, 200, &linked)
	if linked["linked"] != "Agentur" {
		t.Fatalf("linked: %v", linked)
	}
	if r := e.do("POST", "/api/me/identities/confirm", evaTok, map[string]string{"code": fragment(to, "interlinea-link")}); r.status != 404 {
		t.Fatalf("a code confirmed twice: %d", r.status)
	}
	// Now the provider signs her in: the app exchanges the code once.
	to = e.signInThrough(p, "https://il.example/", "eva-1", "eva")
	code := fragment(to, "interlinea-code")
	if code == "" || to.Host != "il.example" {
		t.Fatalf("signed in: %v", to)
	}
	var res signinResult
	e.call("POST", "/api/auth/exchange", "", map[string]string{"code": code}, 200, &res)
	if res.User.ID != eva.ID || !strings.HasPrefix(res.Token, "il_") {
		t.Fatalf("exchange %+v", res)
	}
	if r := e.do("POST", "/api/auth/exchange", "", map[string]string{"code": code}); r.status != 401 {
		t.Fatalf("second exchange: %d", r.status)
	}
	var tokens []authdb.Token
	e.call("GET", "/api/me/tokens", res.Token, nil, 200, &tokens)
	found := false
	for _, tk := range tokens {
		found = found || tk.Name == "Desktop"
	}
	if !found {
		t.Fatalf("tokens %+v", tokens)
	}
	// A code runs out.
	to = e.signInThrough(p, "https://il.example/", "eva-1", "eva")
	e.advance(exchangeTTL + time.Second)
	if r := e.do("POST", "/api/auth/exchange", "", map[string]string{"code": fragment(to, "interlinea-code")}); r.status != 401 {
		t.Fatalf("expired code: %d", r.status)
	}
	// A callback is used once, and a stranger's state is refused.
	e.call("POST", "/api/auth/oidc/start", "", oidcStartRequest{Return: "https://il.example/"}, 200, &start)
	cb := p.login(start["url"], "eva-1", "eva", nil)
	e.followCallback(cb)
	u, _ := url.Parse(cb)
	if r := e.do("GET", u.Path+"?"+u.RawQuery, "", nil); r.status != 400 || !strings.Contains(string(r.body), "expired") {
		t.Fatalf("callback again: %d %s", r.status, r.body)
	}
	// The provider refusing comes back to the app.
	e.call("POST", "/api/auth/oidc/start", "", oidcStartRequest{Return: "https://il.example/"}, 200, &start)
	su, _ := url.Parse(start["url"])
	to = e.followCallback("https://il.example/api/auth/oidc/callback?state=" + url.QueryEscape(su.Query().Get("state")) + "&error=access_denied&error_description=No+licence")
	if !strings.Contains(fragment(to, "interlinea-error"), "No licence") {
		t.Fatalf("refused: %v", to)
	}
	// A disabled account isn't signed in.
	yes := true
	e.st.Auth.UpdateUser(ctx, eva.ID, authdb.UserChange{Disabled: &yes}, eva.Version, "test")
	to = e.signInThrough(p, "https://il.example/", "eva-1", "eva")
	if !strings.Contains(fragment(to, "interlinea-error"), "disabled") {
		t.Fatalf("disabled: %v", to)
	}
	// Unlinking.
	no := false
	cur, _ := e.st.Auth.User(ctx, eva.ID)
	e.st.Auth.UpdateUser(ctx, eva.ID, authdb.UserChange{Disabled: &no}, cur.Version, "test")
	_, evaTok = func() (authdb.User, string) {
		var r signinResult
		e.call("POST", "/api/auth/password", "", passwordSignin{Username: "eva", Password: pw}, 200, &r)
		return r.User, r.Token
	}()
	e.call("DELETE", "/api/me/identities?issuer="+url.QueryEscape(p.srv.URL)+"&subject=eva-1", evaTok, nil, 204, nil)
	to = e.signInThrough(p, "https://il.example/", "eva-1", "eva")
	if fragment(to, "interlinea-error") == "" {
		t.Fatalf("after unlinking: %v", to)
	}
}

func TestProviderMakesAccounts(t *testing.T) {
	e, p := oidcEnv(t, true)
	e.user("eva", false) // the username is taken: the new account gets eva2
	to := e.signInThrough(p, "https://il.example/", "new-7", "eva")
	var res signinResult
	e.call("POST", "/api/auth/exchange", "", map[string]string{"code": fragment(to, "interlinea-code")}, 200, &res)
	if res.User.Username != "eva2" || res.User.Name != "Eva Brandt" || res.User.Email != "eva@agentur.example" || res.User.HasPassword || res.User.Admin {
		t.Fatalf("made %+v", res.User)
	}
	ids, _ := e.st.Auth.Identities(ctx, res.User.ID)
	if len(ids) != 1 || ids[0].Subject != "new-7" {
		t.Fatalf("identities %+v", ids)
	}
	// The next sign-in finds the same account.
	to = e.signInThrough(p, "https://il.example/", "new-7", "eva")
	var again signinResult
	e.call("POST", "/api/auth/exchange", "", map[string]string{"code": fragment(to, "interlinea-code")}, 200, &again)
	if again.User.ID != res.User.ID {
		t.Fatalf("second sign-in %+v", again.User)
	}
	// An unverified address is not taken over.
	var start map[string]string
	e.call("POST", "/api/auth/oidc/start", "", oidcStartRequest{Return: "https://il.example/"}, 200, &start)
	to = e.followCallback(p.login(start["url"], "new-8", "", map[string]any{"email": "boss@agentur.example", "email_verified": false}))
	var unverified signinResult
	e.call("POST", "/api/auth/exchange", "", map[string]string{"code": fragment(to, "interlinea-code")}, 200, &unverified)
	if unverified.User.Email != "" || unverified.User.Username != "user" {
		t.Fatalf("unverified %+v", unverified.User)
	}
	// The admin links an identity by subject alone (the provider's issuer).
	_, admin := e.user("anna", true)
	carl, _ := e.user("carl", false)
	e.call("POST", fmt.Sprintf("/api/users/%d/identities", carl.ID), admin, map[string]string{"subject": "carl-1"}, 204, nil)
	to = e.signInThrough(p, "https://il.example/", "carl-1", "carl.roth")
	var linked signinResult
	e.call("POST", "/api/auth/exchange", "", map[string]string{"code": fragment(to, "interlinea-code")}, 200, &linked)
	if linked.User.ID != carl.ID {
		t.Fatalf("linked by an administrator %+v", linked.User)
	}
}

func TestReturnURLs(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.Origins = []string{"http://localhost:5173"} })
	for raw, want := range map[string]bool{
		"https://il.example/after":     true,
		"http://localhost:5173/x":      true,
		"http://localhost:9999/x":      true,
		"http://127.0.0.1:8123/cb":     true,
		"http://[::1]:8123/cb":         true,
		"https://localhost:9999/x":     false,
		"https://il.example.evil.com/": false,
		"http://10.0.0.1/":             false,
		"/relative":                    false,
	} {
		if _, ok := e.srv.returnURL(raw); ok != want {
			t.Errorf("%s: %v, want %v", raw, ok, want)
		}
	}
	if r := e.do("POST", "/api/auth/oidc/start", "", oidcStartRequest{Return: "https://il.example/"}); r.status != 404 {
		t.Fatalf("start without a provider: %d", r.status)
	}
}
