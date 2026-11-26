package server

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Someone signed in with no grants and no work calls every route of the
// server on the things others made: a project, its memories and
// termbases, a merge request, another account. Only the routes that are
// for everyone signed in answer with success; every other one refuses, or
// says the thing doesn't exist. A new route that forgets its check fails
// here.
func TestEveryRouteRefusesSomeoneWithoutGrants(t *testing.T) {
	te := newTeam(t)
	p := te.createLease()
	var mr struct {
		ID int64 `json:"id"`
	}
	te.call("POST", "/api/termbases/Mietvertrag%20Hofmann/merge-requests", te.pm, map[string]any{"master": "tb-weber", "title": "Neue Begriffe"}, 201, &mr)
	values := map[string]string{
		"/api/projects/{id}":       fmt.Sprint(p.ID),
		"/api/merge-requests/{id}": fmt.Sprint(mr.ID),
		"/api/users/{id}":          fmt.Sprint(mustUser(t, te.env, "paula").ID),
		"/api/me/tokens/{id}":      "1",
		"/api/memories/{name}":     url.PathEscape("Weber DE-EN"),
		"/api/termbases/{name}":    "tb-weber",
	}
	// For everyone signed in: their own account, the people and roles to
	// choose from, lists that show each person only what they may see,
	// and the tools that work on what the request itself brings.
	open := map[string]bool{
		"GET /api/me": true, "GET /api/me/tokens": true, "GET /api/me/assignments": true,
		"GET /api/people": true, "GET /api/roles": true, "GET /api/catalog": true,
		"GET /api/memories": true, "GET /api/termbases": true, "GET /api/projects": true,
		"GET /api/templates": true, "GET /api/providers": true, "GET /api/qa/kinds": true,
		"GET /api/dictionaries": true, "GET /api/projects/defaults": true, "GET /api/export-presets": true,
		"GET /api/providers/services": true,
		"POST /mcp":                   true, "POST /api/align": true, "POST /api/files/inspect": true,
		"POST /api/memoryfiles/inspect": true, "POST /api/termfiles/inspect": true,
	}
	skip := map[string]bool{
		"GET /api/events":             true, // a stream; its events are checked in events_test.go
		"POST /api/auth/signout":      true, // would end the sign-in the sweep uses
		"GET /api/openapi.json":       true,
		"GET /api/server":             true,
		"GET /mcp":                    true,
		"GET /api/auth/oidc/callback": true,
	}
	param := regexp.MustCompile(`\{[^}]+\}`)
	for _, route := range te.srv.patterns {
		if skip[route] || !strings.HasPrefix(strings.SplitN(route, " ", 2)[1], "/api/") && !strings.HasSuffix(route, "/mcp") {
			continue
		}
		method, pattern, _ := strings.Cut(route, " ")
		path := pattern
		for prefix, v := range values {
			if strings.HasPrefix(path, prefix) {
				path = strings.Replace(path, prefix, strings.Replace(prefix, prefix[strings.LastIndex(prefix, "{"):], v, 1), 1)
			}
		}
		path = param.ReplaceAllString(path, "1")
		var body any
		if method != "GET" && method != "DELETE" {
			body = map[string]any{}
		}
		r := te.do(method, path+"?version=1&file=Mietvertrag.txt&lang=en-GB&segment=s1&kind=memory&resource=Weber%20DE-EN", te.eve, body)
		ok := r.status >= 200 && r.status < 300
		if testing.Verbose() && !open[route] {
			t.Logf("%d %s", r.status, route)
		}
		if ok && !open[route] {
			t.Errorf("%s (%s) answered someone without grants: %d %.200s", route, path, r.status, r.body)
		}
	}
	// Routes that refuse an empty request before they look at who asks,
	// again with a request that is right.
	eve := mustUser(t, te.env, "eve")
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"PUT", "/api/grants", map[string]any{"userId": eve.ID, "kind": "memory", "resource": "Weber DE-EN", "role": "manage"}},
		{"PUT", "/api/grants", map[string]any{"userId": eve.ID, "kind": "project", "resource": "*", "role": "manage"}},
		{"DELETE", fmt.Sprintf("/api/grants?userId=%d&kind=project&resource=*", mustUser(t, te.env, "paula").ID), nil},
		{"POST", "/api/termbases/tb-weber/export", map[string]any{"options": map[string]any{"format": "tbx"}}},
	} {
		if r := te.do(c.method, c.path, te.eve, c.body); r.status != 403 && r.status != 404 {
			t.Errorf("%s %s answered someone without grants: %d %.200s", c.method, c.path, r.status, r.body)
		}
	}
}
