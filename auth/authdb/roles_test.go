package authdb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestRoleSets(t *testing.T) {
	// The roles that come with the server are a permission and the ones before it.
	if !RoleManage.Includes(RoleEdit) || !RoleEdit.Includes(RoleSuggest) || RoleEdit.Includes(RoleApprove) || RoleRead.Includes(RoleSuggest) {
		t.Fatal("the built-in ladder")
	}
	reviewer := roleOf([]Role{RoleApprove, RoleRead, RoleSuggest})
	if reviewer != "read+suggest+approve" || !reviewer.Valid() {
		t.Fatalf("a set of its own: %q", reviewer)
	}
	if !reviewer.Includes(RoleApprove) || !reviewer.Includes(RoleRead) || reviewer.Includes(RoleEdit) || reviewer.Includes(RoleManage) {
		t.Fatal("a reviewer approves but does not edit")
	}
	if roleOf([]Role{RoleRead, RoleSuggest, RoleEdit}) != RoleEdit || roleOf(nil) != "" {
		t.Fatal("a set that is a built-in role is named as it")
	}
	if Union(reviewer, RoleEdit) != RoleApprove || Union("", RoleRead) != RoleRead || Union(RoleRead, "") != RoleRead {
		t.Fatalf("union: %q", Union(reviewer, RoleEdit))
	}
	for _, bad := range []Role{"", "admin", "read+fly", "Reviewer"} {
		if bad.Valid() || bad.Includes(RoleRead) || RoleManage.Includes(bad) {
			t.Errorf("%q is no set of permissions", bad)
		}
	}
	if got := reviewer.Permissions(); len(got) != 3 || got[2] != RoleApprove {
		t.Fatalf("permissions %v", got)
	}
}

func TestDefinedRoles(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.params = hashParams{time: 1, memory: 8, threads: 1}
	ben, err := d.CreateUser(ctx, NewUser{Username: "ben", Password: "a long enough password"}, "test")
	if err != nil {
		t.Fatal(err)
	}

	roles, _ := d.Roles(ctx)
	if len(roles) != 5 || !roles[0].BuiltIn || roles[4].Name != "manage" || len(roles[4].Permissions) != 5 {
		t.Fatalf("built-in roles: %+v", roles)
	}
	var inv *InvalidError
	for name, perms := range map[string][]Role{
		"":        {RoleRead},
		"Edit":    {RoleRead},
		"a+b":     {RoleRead},
		"Blind":   {RoleApprove},
		"Unknown": {RoleRead, "fly"},
		"Empty":   {},
	} {
		if _, err := d.SetRole(ctx, "", name, perms, "admin"); !errors.As(err, &inv) {
			t.Errorf("role %q with %v: %v", name, perms, err)
		}
	}
	def, err := d.SetRole(ctx, "", " Reviewer ", []Role{RoleApprove, RoleSuggest, RoleRead}, "admin")
	if err != nil || def.Name != "Reviewer" || len(def.Permissions) != 3 {
		t.Fatalf("define: %+v %v", def, err)
	}
	// A grant names the role, however it is spelled; one that is not there is refused.
	if err := d.SetGrant(ctx, Grant{UserID: ben.ID, Kind: KindTermbase, Resource: "Recht", Role: "reviewer"}, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetGrant(ctx, Grant{UserID: ben.ID, Kind: KindTermbase, Resource: "Recht", Role: "Nobody"}, "admin"); !errors.As(err, &inv) {
		t.Fatalf("a grant of a role that is not there: %v", err)
	}
	gs, _ := d.Grants(ctx, ben.ID)
	if len(gs) != 1 || gs[0].Role != "Reviewer" {
		t.Fatalf("stored grant: %+v", gs)
	}
	can := func(need Role) bool {
		ok, err := d.Can(ctx, ben, KindTermbase, "Recht", need)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !can(RoleRead) || !can(RoleSuggest) || !can(RoleApprove) || can(RoleEdit) || can(RoleManage) {
		t.Fatal("what a reviewer may do")
	}
	if r, _ := d.RoleOn(ctx, ben, KindTermbase, "Recht"); r != "read+suggest+approve" {
		t.Fatalf("role on the termbase: %q", r)
	}
	// Grants add up: edit on all termbases and the reviewer's role make approve.
	if err := d.SetGrant(ctx, Grant{UserID: ben.ID, Kind: KindTermbase, Resource: "*", Role: RoleEdit}, "admin"); err != nil {
		t.Fatal(err)
	}
	if r, _ := d.RoleOn(ctx, ben, KindTermbase, "Recht"); r != RoleApprove {
		t.Fatalf("both grants together: %q", r)
	}
	if r, _ := d.RoleOn(ctx, ben, KindTermbase, "Other"); r != RoleEdit {
		t.Fatalf("on another termbase: %q", r)
	}
	d.RevokeGrant(ctx, ben.ID, KindTermbase, "*", "admin")

	// Changing the role changes what its holders may do, at once.
	if _, err := d.SetRole(ctx, "", "Reviewer", []Role{RoleRead, RoleEdit}, "admin"); err != nil {
		t.Fatal(err)
	}
	if can(RoleApprove) || !can(RoleEdit) {
		t.Fatal("the changed role")
	}
	// Renaming takes the grants along.
	if _, err := d.SetRole(ctx, "Reviewer", "Lektorat", []Role{RoleRead, RoleSuggest, RoleApprove}, "admin"); err != nil {
		t.Fatal(err)
	}
	gs, _ = d.Grants(ctx, ben.ID)
	if gs[0].Role != "Lektorat" || !can(RoleApprove) {
		t.Fatalf("after renaming: %+v", gs)
	}
	if _, err := d.SetRole(ctx, "Reviewer", "Other", []Role{RoleRead}, "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("renaming a role that is gone: %v", err)
	}
	d.SetRole(ctx, "", "Leser", []Role{RoleRead}, "admin")
	if _, err := d.SetRole(ctx, "Leser", "lektorat", []Role{RoleRead}, "admin"); !errors.As(err, &inv) {
		t.Fatalf("renaming onto another role: %v", err)
	}
	roles, _ = d.Roles(ctx)
	if len(roles) != 7 || roles[5].Name != "Lektorat" || roles[5].Grants != 1 || roles[5].BuiltIn || roles[6].Name != "Leser" {
		t.Fatalf("roles: %+v", roles)
	}
	// A role people have stays; one nobody has goes.
	if err := d.DeleteRole(ctx, "lektorat", "admin"); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("deleting a role in use: %v", err)
	}
	if err := d.DeleteRole(ctx, "Leser", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteRole(ctx, "read", "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting a built-in role: %v", err)
	}
	// A termbase renamed onto one where the person has another role keeps
	// the role that allows more.
	d.SetGrant(ctx, Grant{UserID: ben.ID, Kind: KindTermbase, Resource: "Neu", Role: RoleRead}, "admin")
	if err := d.RenameResource(ctx, KindTermbase, "Recht", "Neu", "admin"); err != nil {
		t.Fatal(err)
	}
	gs, _ = d.Grants(ctx, ben.ID)
	if len(gs) != 1 || gs[0].Resource != "Neu" || gs[0].Role != "Lektorat" {
		t.Fatalf("after renaming the termbase: %+v", gs)
	}
	cs, _, _ := d.Changes(ctx, 0, 100)
	seen := map[string]bool{}
	for _, c := range cs {
		seen[c.Kind] = true
	}
	if !seen[ChangeRoleSet] || !seen[ChangeRoleDeleted] {
		t.Fatalf("the change log: %+v", cs)
	}
}
