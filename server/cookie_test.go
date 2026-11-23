package server

import (
	"net/http"
	"strings"
	"testing"
)

// The web app's sign-in: a cookie scripts can't read, and a header on
// every request.
func TestCookieSignIn(t *testing.T) {
	e := newEnv(t)
	e.user("anna", true)
	var res signinResult
	r := e.call("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: pw, Device: "Chrome"}, 200, &res, askCookie, "cookie")
	if res.Token != "" || strings.Contains(string(r.body), `"token"`) {
		t.Fatalf("a cookie sign-in shows its token: %s", r.body)
	}
	if res.User.Username != "anna" || res.ExpiresAt.IsZero() {
		t.Fatalf("sign-in %+v", res)
	}
	c := (&http.Response{Header: r.header}).Cookies()
	if len(c) != 1 || c[0].Name != sessionCookie || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode || c[0].Path != "/api/" || c[0].MaxAge <= 0 || c[0].Value == "" {
		t.Fatalf("cookie %+v", c)
	}
	if c[0].Secure {
		t.Fatal("the test server is plain HTTP: a Secure cookie would not come back")
	}
	cookie := sessionCookie + "=" + c[0].Value

	// Without the header, another site's form or image could use it.
	if r := e.do("GET", "/api/me", "", nil, "Cookie", cookie); r.status != 403 {
		t.Fatalf("cookie without the header: %d %s", r.status, r.body)
	}
	var me meResult
	e.call("GET", "/api/me", "", nil, 200, &me, "Cookie", cookie, cookieHeader, "1")
	if me.User.Username != "anna" {
		t.Fatalf("me %+v", me)
	}
	// The cookie is a sign-in like any other: listed, and the current one.
	var ts []struct {
		Name    string `json:"name"`
		Current bool   `json:"current"`
	}
	e.call("GET", "/api/me/tokens", "", nil, 200, &ts, "Cookie", cookie, cookieHeader, "1")
	found := false
	for _, x := range ts {
		found = found || (x.Current && x.Name == "Chrome")
	}
	if !found {
		t.Fatalf("tokens %+v", ts)
	}

	// Changing the password keeps the browser signed in, with a new cookie.
	r = e.call("PUT", "/api/me/password", "", map[string]string{"current": pw, "new": "a new long password"}, 200, &res, "Cookie", cookie, cookieHeader, "1")
	if res.Token != "" {
		t.Fatalf("new sign-in shows its token: %s", r.body)
	}
	c = (&http.Response{Header: r.header}).Cookies()
	if len(c) != 1 || c[0].Value == "" || sessionCookie+"="+c[0].Value == cookie {
		t.Fatalf("new cookie %+v", c)
	}
	if r := e.do("GET", "/api/me", "", nil, "Cookie", cookie, cookieHeader, "1"); r.status != 401 {
		t.Fatalf("old cookie after the password change: %d", r.status)
	}
	cookie = sessionCookie + "=" + c[0].Value

	// Signing out ends the sign-in and clears the cookie.
	r = e.call("POST", "/api/auth/signout", "", nil, 204, nil, "Cookie", cookie, cookieHeader, "1")
	c = (&http.Response{Header: r.header}).Cookies()
	if len(c) != 1 || c[0].Name != sessionCookie || c[0].MaxAge >= 0 {
		t.Fatalf("signing out leaves the cookie: %+v", c)
	}
	if r := e.do("GET", "/api/me", "", nil, "Cookie", cookie, cookieHeader, "1"); r.status != 401 {
		t.Fatalf("cookie after signing out: %d", r.status)
	}

	// Other apps keep using tokens; a header wins over a stale cookie.
	var tok signinResult
	e.call("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: "a new long password"}, 200, &tok)
	if tok.Token == "" {
		t.Fatal("a token sign-in has no token")
	}
	e.call("GET", "/api/me", tok.Token, nil, 200, nil, "Cookie", cookie)
}

// Behind a proxy that ends HTTPS the cookie is Secure.
func TestCookieSecureBehindProxy(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.TrustProxy = true })
	e.user("anna", true)
	r := e.call("POST", "/api/auth/password", "", passwordSignin{Username: "anna", Password: pw}, 200, nil, askCookie, "cookie", "X-Forwarded-Proto", "https")
	c := (&http.Response{Header: r.header}).Cookies()
	if len(c) != 1 || !c[0].Secure {
		t.Fatalf("cookie %+v", c)
	}
}

// The API's description is served, and describes itself.
func TestOpenAPIServed(t *testing.T) {
	e := newEnv(t)
	var doc struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	e.call("GET", "/api/openapi.json", "", nil, 200, &doc)
	if doc.OpenAPI != "3.1.0" || doc.Paths["/api/openapi.json"]["get"] == nil || doc.Paths["/api/projects/{id}/segments"]["put"] == nil {
		t.Fatalf("openapi.json: %s, %d paths", doc.OpenAPI, len(doc.Paths))
	}
}
