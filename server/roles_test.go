package server

import (
	"fmt"
	"testing"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/term"
)

// TestCustomRoles defines a role of its own and checks what its holder may
// do: a reviewer who reads, suggests and approves, but does not edit.
func TestCustomRoles(t *testing.T) {
	e := newEnv(t)
	_, admin := e.user("anna", true)
	rita, ritaTok := e.user("rita", false)

	var roles []authdb.RoleDef
	e.call("GET", "/api/roles", ritaTok, nil, 200, &roles)
	if len(roles) != 5 || roles[2].Name != "edit" || len(roles[2].Permissions) != 3 || !roles[2].BuiltIn {
		t.Fatalf("the roles that come with the server: %+v", roles)
	}
	// Only administrators define roles.
	body := map[string]any{"permissions": []string{"read", "suggest", "approve"}}
	if r := e.do("PUT", "/api/roles/Reviewer", ritaTok, body); r.status != 403 {
		t.Fatalf("someone defines a role: %d", r.status)
	}
	var def authdb.RoleDef
	e.call("PUT", "/api/roles/Reviewer", admin, body, 200, &def)
	if def.Name != "Reviewer" || len(def.Permissions) != 3 {
		t.Fatalf("defined: %+v", def)
	}
	for name, perms := range map[string][]string{"edit": {"read"}, "Blind": {"approve"}, "Odd": {"read", "fly"}} {
		if r := e.do("PUT", "/api/roles/"+name, admin, map[string]any{"permissions": perms}); r.status != 422 {
			t.Fatalf("role %s with %v: %d", name, perms, r.status)
		}
	}

	e.call("POST", "/api/termbases", admin, newTermbase{Definition: term.Definition{Name: "Recht", Languages: []string{"de-DE", "en-GB"}}}, 201, nil)
	entry := map[string]any{"entry": term.Entry{Languages: []term.Language{{Lang: "de-DE", Terms: []term.Term{{Text: "Kaution"}}}}}}
	e.call("POST", "/api/termbases/Recht/entries", admin, entry, 201, nil)
	e.call("POST", "/api/termbases", admin, newTermbase{Definition: term.Definition{Name: "Fall"}, Upstreams: []string{"Recht"}}, 201, nil)
	e.call("POST", "/api/termbases/Fall/entries", admin, map[string]any{"entry": term.Entry{Languages: []term.Language{{Lang: "de-DE", Terms: []term.Term{{Text: "Staffelmiete"}}}}}}, 201, nil)

	// Rita has no role yet: the termbase isn't there for her.
	if r := e.do("GET", "/api/termbases/Recht", ritaTok, nil); r.status != 404 {
		t.Fatalf("without a role: %d", r.status)
	}
	if r := e.do("PUT", "/api/grants", admin, authdb.Grant{UserID: rita.ID, Kind: authdb.KindTermbase, Resource: "Recht", Role: "Nobody"}); r.status != 422 {
		t.Fatalf("a grant of a role that is not there: %d", r.status)
	}
	e.call("PUT", "/api/grants", admin, authdb.Grant{UserID: rita.ID, Kind: authdb.KindTermbase, Resource: "Recht", Role: "Reviewer"}, 204, nil)
	e.call("PUT", "/api/grants", admin, authdb.Grant{UserID: rita.ID, Kind: authdb.KindTermbase, Resource: "Fall", Role: "read"}, 204, nil)
	var tb termbaseDetail
	e.call("GET", "/api/termbases/Recht", ritaTok, nil, 200, &tb)
	if tb.Role != "read+suggest+approve" {
		t.Fatalf("her role on the termbase: %q", tb.Role)
	}
	// She reads but does not edit or manage.
	if r := e.do("POST", "/api/termbases/Recht/entries", ritaTok, entry); r.status != 403 {
		t.Fatalf("a reviewer adds an entry: %d", r.status)
	}
	if r := e.do("PUT", "/api/termbases/Recht/name", ritaTok, map[string]string{"name": "Mein Recht"}); r.status != 403 {
		t.Fatalf("a reviewer renames the termbase: %d", r.status)
	}
	// She decides a merge request into it.
	var mr struct{ ID int64 }
	e.call("POST", "/api/termbases/Fall/merge-requests", admin, map[string]string{"master": "Recht", "title": "Neu"}, 201, &mr)
	var res struct{ Decided int }
	e.call("POST", fmt.Sprintf("/api/merge-requests/%d/decide", mr.ID), ritaTok, map[string]any{"filter": map[string]any{}, "decision": map[string]string{"state": "approved"}}, 200, &res)
	if res.Decided != 1 {
		t.Fatalf("decided: %+v", res)
	}

	// The role changes: its holders lose approving at once.
	e.call("GET", "/api/roles", admin, nil, 200, &roles)
	if len(roles) != 6 || roles[5].Name != "Reviewer" || roles[5].Grants != 1 {
		t.Fatalf("roles: %+v", roles)
	}
	e.call("PUT", "/api/roles/Reviewer", admin, map[string]any{"permissions": []string{"read", "suggest"}}, 200, nil)
	e.call("GET", "/api/termbases/Recht", ritaTok, nil, 200, &tb)
	if tb.Role != "suggest" {
		t.Fatalf("after the change: %q", tb.Role)
	}
	// Renamed, the grant follows.
	e.call("PUT", "/api/roles/Reviewer", admin, map[string]any{"name": "Vorschlagende", "permissions": []string{"read", "suggest"}}, 200, &def)
	var grants []authdb.Grant
	e.call("GET", fmt.Sprintf("/api/users/%d/grants", rita.ID), admin, nil, 200, &grants)
	found := false
	for _, g := range grants {
		found = found || (g.Resource == "Recht" && g.Role == "Vorschlagende")
	}
	if !found {
		t.Fatalf("grants after renaming: %+v", grants)
	}
	// In use it stays; unused it goes.
	if r := e.do("DELETE", "/api/roles/Vorschlagende", admin, nil); r.status != 409 {
		t.Fatalf("deleting a role people have: %d", r.status)
	}
	e.call("DELETE", fmt.Sprintf("/api/grants?userId=%d&kind=termbase&resource=Recht", rita.ID), admin, nil, 204, nil)
	e.call("DELETE", "/api/roles/Vorschlagende", admin, nil, 204, nil)
	if r := e.do("DELETE", "/api/roles/manage", admin, nil); r.status != 404 {
		t.Fatalf("deleting a role that comes with the server: %d", r.status)
	}
}
