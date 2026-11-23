package authdb

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

// Cheap hashes keep the tests fast; TestHashParameters checks the real ones.
var testParams = hashParams{time: 1, memory: 64, threads: 1}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func openTemp(t testing.TB) (*DB, string, *clock) {
	t.Helper()
	var db *DB
	var key string
	if pgDSN() != "" {
		db, key = openPG(t)
	} else {
		key = filepath.Join(t.TempDir(), "auth.db")
		var err error
		if db, err = Open(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
	}
	c := &clock{t: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)}
	db.now = c.now
	db.params = testParams
	return db, key, c
}

// reopen opens the store again, as another server process would, on the
// same clock.
func reopen(t testing.TB, key string, c *clock) *DB {
	t.Helper()
	var db *DB
	if isPG(key) {
		db = reopenPG(t, key)
	} else {
		var err error
		if db, err = Open(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
	}
	db.params = testParams
	db.now = c.now
	return db
}

func mustUser(t *testing.T, db *DB, n NewUser) User {
	t.Helper()
	u, err := db.CreateUser(ctx, n, "setup")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func invalidField(err error) string {
	var inv *InvalidError
	if errors.As(err, &inv) {
		return inv.Field
	}
	return ""
}

func TestAccounts(t *testing.T) {
	db, key, clk := openTemp(t)
	if n, _ := db.CountUsers(ctx); n != 0 {
		t.Fatalf("new store has %d users", n)
	}
	admin := mustUser(t, db, NewUser{Username: " Dan ", Name: "Dan", Email: "dan@example.org", Password: "correct horse battery", Admin: true})
	if admin.Username != "dan" || !admin.Admin || !admin.HasPassword || admin.Version != 1 {
		t.Fatalf("admin %+v", admin)
	}
	sso := mustUser(t, db, NewUser{Username: "eva.brandt@agentur.example"})
	if sso.HasPassword || sso.Name != "eva.brandt@agentur.example" {
		t.Fatalf("sso user %+v", sso)
	}
	if _, err := db.CreateUser(ctx, NewUser{Username: "DAN", Password: "another long one"}, "x"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	for _, c := range []struct {
		n     NewUser
		field string
	}{
		{NewUser{Username: ""}, "username"},
		{NewUser{Username: "a b"}, "username"},
		{NewUser{Username: strings.Repeat("a", 65)}, "username"},
		{NewUser{Username: "ok", Password: "short"}, "password"},
		{NewUser{Username: "ok", Name: "a\tb"}, "name"},
		{NewUser{Username: "ok", Email: "nope"}, "email"},
	} {
		if _, err := db.CreateUser(ctx, c.n, "x"); invalidField(err) != c.field {
			t.Errorf("%+v: %v", c.n, err)
		}
	}
	if u, err := reopen(t, key, clk).UserByName(ctx, "DAN"); err != nil || u.ID != admin.ID || u.Email != "dan@example.org" {
		t.Fatalf("reopened %+v %v", u, err)
	}
	all, _ := db.Users(ctx)
	if len(all) != 2 || all[0].Username != "dan" {
		t.Fatalf("users %+v", all)
	}

	// Sign-in by password.
	if u, err := db.CheckPassword(ctx, "Dan", "correct horse battery"); err != nil || u.ID != admin.ID {
		t.Fatalf("sign in: %+v %v", u, err)
	}
	for _, c := range [][2]string{{"dan", "wrong horse battery"}, {"nobody", "correct horse battery"}, {sso.Username, "anything at all"}} {
		if _, err := db.CheckPassword(ctx, c[0], c[1]); !errors.Is(err, ErrCredentials) {
			t.Errorf("%v: %v", c, err)
		}
	}

	// Changes as edited, with conflicts and the last admin kept.
	name := "Dan D."
	u, err := db.UpdateUser(ctx, admin.ID, UserChange{Name: &name}, 1, "dan")
	if err != nil || u.Name != "Dan D." || u.Version != 2 {
		t.Fatalf("update %+v %v", u, err)
	}
	if _, err := db.UpdateUser(ctx, admin.ID, UserChange{Name: &name}, 1, "dan"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	no := false
	if _, err := db.UpdateUser(ctx, admin.ID, UserChange{Admin: &no}, 2, "dan"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the last admin: %v", err)
	}
	yes := true
	if _, err := db.UpdateUser(ctx, admin.ID, UserChange{Disabled: &yes}, 2, "dan"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disabling the last admin: %v", err)
	}
	if err := db.DeleteUser(ctx, admin.ID, "dan"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deleting the last admin: %v", err)
	}
	if _, err := db.UpdateUser(ctx, sso.ID, UserChange{Admin: &yes}, 1, "dan"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateUser(ctx, admin.ID, UserChange{Admin: &no}, 2, "dan"); err != nil {
		t.Fatalf("demoting with another admin: %v", err)
	}
	bad := "a\nb"
	if _, err := db.UpdateUser(ctx, sso.ID, UserChange{Name: &bad}, 2, "dan"); invalidField(err) != "name" {
		t.Fatalf("bad name: %v", err)
	}

	// Disabled accounts can't sign in.
	if _, err := db.UpdateUser(ctx, admin.ID, UserChange{Disabled: &yes}, 3, "eva"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CheckPassword(ctx, "dan", "correct horse battery"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled sign-in: %v", err)
	}
	if _, _, err := db.IssueToken(ctx, admin.ID, "laptop", 0); !errors.Is(err, ErrDisabled) {
		t.Fatalf("token for a disabled account: %v", err)
	}
	if err := db.DeleteUser(ctx, admin.ID, "eva"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.User(ctx, admin.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}

func TestPasswords(t *testing.T) {
	db, _, _ := openTemp(t)
	u := mustUser(t, db, NewUser{Username: "anna", Password: "first password here", Admin: true})
	secret, _, err := db.IssueToken(ctx, u.ID, "laptop", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, u.ID, "short", "anna"); invalidField(err) != "password" {
		t.Fatalf("weak password: %v", err)
	}
	if err := db.SetPassword(ctx, u.ID, "second password here", "anna"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Authenticate(ctx, secret); !errors.Is(err, ErrToken) {
		t.Fatalf("a new password signs out everywhere: %v", err)
	}
	if _, err := db.CheckPassword(ctx, "anna", "first password here"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("old password: %v", err)
	}
	if _, err := db.CheckPassword(ctx, "anna", "second password here"); err != nil {
		t.Fatal(err)
	}
	// Removing the password needs an identity provider to sign in with.
	if err := db.SetPassword(ctx, u.ID, "", "anna"); invalidField(err) != "password" {
		t.Fatalf("removing the only way in: %v", err)
	}
	if err := db.LinkIdentity(ctx, u.ID, "https://login.example", "sub-1", "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, u.ID, "", "anna"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.User(ctx, u.ID); got.HasPassword {
		t.Fatal("password still there")
	}
	if _, err := db.CheckPassword(ctx, "anna", "second password here"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("password sign-in after removal: %v", err)
	}
}

func TestRehash(t *testing.T) {
	db, _, _ := openTemp(t)
	mustUser(t, db, NewUser{Username: "anna", Password: "a password to keep", Admin: true})
	stored := func() string {
		var h string
		if err := db.sql.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE username = 'anna'`).Scan(&h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	before := stored()
	stronger := hashParams{time: 2, memory: 128, threads: 1}
	db.params = stronger
	if _, err := db.CheckPassword(ctx, "anna", "a password to keep"); err != nil {
		t.Fatal(err)
	}
	after := stored()
	if after == before || !strings.Contains(after, "m=128,t=2,p=1") {
		t.Fatalf("not rehashed: %s", after)
	}
	if ok, current := stronger.verify(after, "a password to keep"); !ok || !current {
		t.Fatal("new hash does not verify")
	}
	if _, err := db.CheckPassword(ctx, "anna", "a password to keep"); err != nil {
		t.Fatal(err)
	}
}

func TestHashParameters(t *testing.T) {
	h, err := defaultParams.hash("a real password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("hash %s", h)
	}
	if ok, current := defaultParams.verify(h, "a real password"); !ok || !current {
		t.Fatal("does not verify")
	}
	if ok, _ := defaultParams.verify(h, "a real passwore"); ok {
		t.Fatal("wrong password verifies")
	}
	for _, bad := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=18$m=1,t=1,p=1$AA$AA", "$argon2id$v=19$m=1,t=0,p=1$AA$AA", "$argon2id$v=19$m=1,t=1,p=1$!!$AA"} {
		if ok, _ := defaultParams.verify(bad, "x"); ok {
			t.Errorf("%q verifies", bad)
		}
	}
	// Every hash has its own salt.
	a, _ := testParams.hash("same")
	b, _ := testParams.hash("same")
	if a == b {
		t.Fatal("hashes share a salt")
	}
}

func TestTokens(t *testing.T) {
	db, key, clk := openTemp(t)
	u := mustUser(t, db, NewUser{Username: "anna", Password: "a long password", Admin: true})
	secret, tok, err := db.IssueToken(ctx, u.ID, "Laptop (Windows)", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "il_") || len(secret) < 40 || tok.Name != "Laptop (Windows)" {
		t.Fatalf("token %q %+v", secret, tok)
	}
	var stored string
	if err := db.sql.QueryRowContext(ctx, `SELECT hash FROM tokens WHERE id = ?`, tok.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, secret[3:]) {
		t.Fatal("the secret is stored")
	}
	got, _, err := reopen(t, key, clk).Authenticate(ctx, secret)
	if err != nil || got.ID != u.ID {
		t.Fatalf("authenticate %+v %v", got, err)
	}
	for _, s := range []string{"", "il_nope", secret + "x", "Bearer " + secret} {
		if _, _, err := db.Authenticate(ctx, s); !errors.Is(err, ErrToken) {
			t.Errorf("%q: %v", s, err)
		}
	}
	// Use is recorded at most once a minute.
	clk.add(30 * time.Second)
	if _, tk, _ := db.Authenticate(ctx, secret); !tk.LastUsed.Equal(tok.LastUsed) {
		t.Fatal("last use written within a minute")
	}
	clk.add(time.Minute)
	if _, tk, _ := db.Authenticate(ctx, secret); !tk.LastUsed.After(tok.LastUsed) {
		t.Fatal("last use not written after a minute")
	}
	second, tok2, _ := db.IssueToken(ctx, u.ID, "Phone", 0)
	list, _ := db.Tokens(ctx, u.ID)
	if len(list) != 2 || list[0].ID != tok2.ID {
		t.Fatalf("tokens %+v", list)
	}
	if err := db.RevokeToken(ctx, u.ID, tok2.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeToken(ctx, u.ID, tok2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
	if _, _, err := db.Authenticate(ctx, second); !errors.Is(err, ErrToken) {
		t.Fatalf("revoked: %v", err)
	}
	// Expiry.
	clk.add(time.Hour)
	if _, _, err := db.Authenticate(ctx, secret); !errors.Is(err, ErrToken) {
		t.Fatalf("expired: %v", err)
	}
	if list, _ := db.Tokens(ctx, u.ID); len(list) != 0 {
		t.Fatalf("expired token listed: %+v", list)
	}
	if n, err := db.PurgeExpired(ctx); err != nil || n != 1 {
		t.Fatalf("purge %d %v", n, err)
	}
	// Signing out everywhere.
	s3, _, _ := db.IssueToken(ctx, u.ID, "a", 0)
	s4, _, _ := db.IssueToken(ctx, u.ID, "b", 0)
	if err := db.RevokeTokens(ctx, u.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{s3, s4} {
		if _, _, err := db.Authenticate(ctx, s); !errors.Is(err, ErrToken) {
			t.Fatalf("after revoking all: %v", err)
		}
	}
	// Disabling an account ends its sign-ins.
	other := mustUser(t, db, NewUser{Username: "ben", Password: "a long password"})
	s5, _, _ := db.IssueToken(ctx, other.ID, "a", 0)
	yes := true
	if _, err := db.UpdateUser(ctx, other.ID, UserChange{Disabled: &yes}, 1, "anna"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Authenticate(ctx, s5); !errors.Is(err, ErrToken) {
		t.Fatalf("disabled account's token: %v", err)
	}
}

func TestIdentities(t *testing.T) {
	db, _, _ := openTemp(t)
	a := mustUser(t, db, NewUser{Username: "anna", Password: "a long password", Admin: true})
	b := mustUser(t, db, NewUser{Username: "ben"})
	const iss = "https://login.microsoftonline.com/tenant/v2.0"
	if err := db.LinkIdentity(ctx, a.ID, iss, "abc", "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.LinkIdentity(ctx, a.ID, iss, "abc", "anna"); err != nil {
		t.Fatalf("linking again: %v", err)
	}
	if err := db.LinkIdentity(ctx, b.ID, iss, "abc", "anna"); invalidField(err) != "identity" {
		t.Fatalf("someone else's identity: %v", err)
	}
	if err := db.LinkIdentity(ctx, b.ID, "", "x", "anna"); invalidField(err) != "identity" {
		t.Fatalf("no issuer: %v", err)
	}
	if u, err := db.UserByIdentity(ctx, iss, "abc"); err != nil || u.ID != a.ID {
		t.Fatalf("by identity %+v %v", u, err)
	}
	if _, err := db.UserByIdentity(ctx, iss, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown identity: %v", err)
	}
	if err := db.LinkIdentity(ctx, b.ID, "https://accounts.google.com", "g1", "anna"); err != nil {
		t.Fatal(err)
	}
	// Ben has no password: his only identity stays.
	if err := db.UnlinkIdentity(ctx, b.ID, "https://accounts.google.com", "g1", "anna"); invalidField(err) != "identity" {
		t.Fatalf("removing the only way in: %v", err)
	}
	if err := db.UnlinkIdentity(ctx, a.ID, iss, "abc", "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.UnlinkIdentity(ctx, a.ID, iss, "abc", "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second unlink: %v", err)
	}
	ids, _ := db.Identities(ctx, b.ID)
	if len(ids) != 1 || ids[0].Subject != "g1" {
		t.Fatalf("identities %+v", ids)
	}
}

func TestGrants(t *testing.T) {
	db, _, _ := openTemp(t)
	admin := mustUser(t, db, NewUser{Username: "anna", Password: "a long password", Admin: true})
	tr := mustUser(t, db, NewUser{Username: "ben", Password: "a long password"})
	rev := mustUser(t, db, NewUser{Username: "carl", Password: "a long password"})
	for _, g := range []Grant{
		{UserID: tr.ID, Kind: KindProject, Resource: "12", Role: RoleEdit},
		{UserID: tr.ID, Kind: KindMemory, Resource: AnyResource, Role: RoleRead},
		{UserID: rev.ID, Kind: KindAll, Resource: AnyResource, Role: RoleRead},
		{UserID: rev.ID, Kind: KindTermbase, Resource: "tb-weber", Role: RoleApprove},
	} {
		if err := db.SetGrant(ctx, g, "anna"); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		u        User
		kind     Kind
		resource string
		want     Role
	}{
		{admin, KindProject, "99", RoleManage},
		{tr, KindProject, "12", RoleEdit},
		{tr, KindProject, "13", ""},
		{tr, KindMemory, "Weber DE-EN", RoleRead},
		{tr, KindTermbase, "tb-weber", ""},
		{rev, KindProject, "12", RoleRead},
		{rev, KindTermbase, "tb-weber", RoleApprove},
		{rev, KindTermbase, "tb-medtech", RoleRead},
	}
	for _, c := range cases {
		got, err := db.RoleOn(ctx, c.u, c.kind, c.resource)
		if err != nil || got != c.want {
			t.Errorf("%s on %s %s: %q %v, want %q", c.u.Username, c.kind, c.resource, got, err, c.want)
		}
	}
	if ok, _ := db.Can(ctx, tr, KindProject, "12", RoleSuggest); !ok {
		t.Error("edit includes suggest")
	}
	if ok, _ := db.Can(ctx, tr, KindProject, "12", RoleApprove); ok {
		t.Error("edit does not include approve")
	}
	if ok, _ := db.Can(ctx, tr, KindProject, "12", ""); ok {
		t.Error("an empty role allows nothing")
	}
	// A grant on one resource is replaced, not added.
	if err := db.SetGrant(ctx, Grant{UserID: tr.ID, Kind: KindProject, Resource: "12", Role: RoleSuggest}, "anna"); err != nil {
		t.Fatal(err)
	}
	if r, _ := db.RoleOn(ctx, tr, KindProject, "12"); r != RoleSuggest {
		t.Fatalf("replaced grant: %q", r)
	}
	gs, _ := db.Grants(ctx, tr.ID)
	if len(gs) != 2 {
		t.Fatalf("grants %+v", gs)
	}
	on, _ := db.GrantsOn(ctx, KindProject, "12")
	if len(on) != 1 || on[0].UserID != tr.ID {
		t.Fatalf("grants on 12 %+v", on)
	}
	for _, g := range []Grant{
		{UserID: tr.ID, Kind: "folder", Resource: "x", Role: RoleRead},
		{UserID: tr.ID, Kind: KindProject, Resource: "x", Role: "owner"},
		{UserID: tr.ID, Kind: KindProject, Resource: " ", Role: RoleRead},
		{UserID: tr.ID, Kind: KindAll, Resource: "12", Role: RoleRead},
	} {
		if err := db.SetGrant(ctx, g, "anna"); invalidField(err) == "" {
			t.Errorf("%+v accepted: %v", g, err)
		}
	}
	if err := db.SetGrant(ctx, Grant{UserID: 999, Kind: KindProject, Resource: "1", Role: RoleRead}, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grant to nobody: %v", err)
	}
	if err := db.RevokeGrant(ctx, tr.ID, KindProject, "12", "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeGrant(ctx, tr.ID, KindProject, "12", "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
	if r, _ := db.RoleOn(ctx, tr, KindProject, "12"); r != "" {
		t.Fatalf("after revoke: %q", r)
	}
	// Deleting a resource takes its grants with it.
	if err := db.RevokeResource(ctx, KindTermbase, "tb-weber", "anna"); err != nil {
		t.Fatal(err)
	}
	if r, _ := db.RoleOn(ctx, rev, KindTermbase, "tb-weber"); r != RoleRead {
		t.Fatalf("after deleting the termbase: %q", r)
	}
	// Disabled accounts may do nothing, whatever their grants.
	yes := true
	rev, _ = db.UpdateUser(ctx, rev.ID, UserChange{Disabled: &yes}, rev.Version, "anna")
	if r, _ := db.RoleOn(ctx, rev, KindProject, "12"); r != "" {
		t.Fatalf("disabled: %q", r)
	}
	// Deleting an account takes its grants.
	if err := db.DeleteUser(ctx, tr.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM grants WHERE user_id = ?`, tr.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("grants left %d %v", n, err)
	}
}

func TestRoles(t *testing.T) {
	order := []Role{RoleRead, RoleSuggest, RoleEdit, RoleApprove, RoleManage}
	for i, a := range order {
		for j, b := range order {
			if a.Includes(b) != (i >= j) {
				t.Errorf("%s includes %s = %t", a, b, a.Includes(b))
			}
		}
	}
	if Role("owner").Valid() || Role("owner").Includes(RoleRead) || !RoleRead.Valid() {
		t.Fatal("unknown roles")
	}
}

func TestChangeLog(t *testing.T) {
	db, _, _ := openTemp(t)
	a := mustUser(t, db, NewUser{Username: "anna", Password: "a long password", Admin: true})
	b := mustUser(t, db, NewUser{Username: "ben", Password: "a long password"})
	if err := db.SetGrant(ctx, Grant{UserID: b.ID, Kind: KindProject, Resource: "1", Role: RoleEdit}, "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, b.ID, "a newer long password", "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, b.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	cs, more, err := db.Changes(ctx, 0, 0)
	if err != nil || more {
		t.Fatal(err, more)
	}
	var kinds []string
	for _, c := range cs {
		kinds = append(kinds, c.Kind)
		if c.Detail == "a newer long password" || strings.Contains(c.Detail, "long password") {
			t.Fatal("a password in the log")
		}
	}
	want := "user-created user-created grant-set password-set user-deleted"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("log %v", kinds)
	}
	if cs[0].UserID != a.ID || cs[0].By != "setup" || cs[2].Detail != "project 1 edit" || cs[4].Detail != "ben" {
		t.Fatalf("entries %+v", cs)
	}
	if last, err := db.LastChange(ctx); err != nil || last != cs[len(cs)-1].Seq {
		t.Fatalf("last change %d, %v", last, err)
	}
	page, more, _ := db.Changes(ctx, cs[1].Seq, 2)
	if len(page) != 2 || !more || page[0].Kind != "grant-set" {
		t.Fatalf("page %+v %v", page, more)
	}
}

func TestRenameResource(t *testing.T) {
	db, _, _ := openTemp(t)
	ben := mustUser(t, db, NewUser{Username: "ben", Password: "a long password"})
	carl := mustUser(t, db, NewUser{Username: "carl", Password: "a long password"})
	dora := mustUser(t, db, NewUser{Username: "dora", Password: "a long password"})
	for _, g := range []Grant{
		{UserID: ben.ID, Kind: KindTermbase, Resource: "tb-old", Role: RoleEdit},
		{UserID: carl.ID, Kind: KindTermbase, Resource: "tb-old", Role: RoleRead},
		{UserID: carl.ID, Kind: KindTermbase, Resource: "tb-new", Role: RoleApprove},
		{UserID: dora.ID, Kind: KindMemory, Resource: "tb-old", Role: RoleManage},
	} {
		if err := db.SetGrant(ctx, g, "anna"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RenameResource(ctx, KindTermbase, "tb-old", "tb-new", "anna"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		u    User
		kind Kind
		res  string
		want Role
	}{
		{ben, KindTermbase, "tb-new", RoleEdit},
		{ben, KindTermbase, "tb-old", ""},
		{carl, KindTermbase, "tb-new", RoleApprove}, // the higher role stays
		{dora, KindMemory, "tb-old", RoleManage},    // another kind is untouched
	} {
		if r, err := db.RoleOn(ctx, c.u, c.kind, c.res); err != nil || r != c.want {
			t.Errorf("%s on %s %s: %q %v, want %q", c.u.Username, c.kind, c.res, r, err, c.want)
		}
	}
	if on, _ := db.GrantsOn(ctx, KindTermbase, "tb-new"); len(on) != 2 {
		t.Fatalf("grants on the new name %+v", on)
	}
	for _, c := range []struct {
		kind     Kind
		from, to string
	}{
		{KindAll, "*", "x"},
		{KindTermbase, "tb-new", " "},
		{KindTermbase, "tb-new", AnyResource},
	} {
		if err := db.RenameResource(ctx, c.kind, c.from, c.to, "anna"); invalidField(err) == "" {
			t.Errorf("%+v accepted: %v", c, err)
		}
	}
	if err := db.RenameResource(ctx, KindTermbase, "tb-new", "tb-new", "anna"); err != nil {
		t.Fatal(err)
	}
}

func TestSigninFlowsAndCodes(t *testing.T) {
	db, key, clk := openTemp(t)
	u := mustUser(t, db, NewUser{Username: "eva", Password: "a long password"})
	now := clk.now()
	if err := db.SaveFlow(ctx, "st-1", []byte(`{"a":1}`), now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveFlow(ctx, "st-1", []byte(`{}`), now.Add(time.Minute)); !errors.Is(err, ErrExists) {
		t.Fatalf("same state twice: %v", err)
	}
	if err := db.SaveFlow(ctx, "", nil, now); invalidField(err) != "state" {
		t.Fatalf("no state: %v", err)
	}
	// Another instance (the same database, opened again) finishes it, once.
	other := reopen(t, key, clk)
	got, err := other.TakeFlow(ctx, "st-1")
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("taken %q, %v", got, err)
	}
	if _, err := other.TakeFlow(ctx, "st-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("taken twice: %v", err)
	}
	// Expired flows and codes don't count.
	if err := db.SaveFlow(ctx, "st-2", []byte(`{}`), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCode(ctx, "code-1", u.ID, "Laptop", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCode(ctx, "code-2", u.ID, "Phone", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	clk.add(time.Minute)
	if _, err := db.TakeFlow(ctx, "st-2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired flow: %v", err)
	}
	if _, _, err := db.TakeCode(ctx, "code-2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired code: %v", err)
	}
	id, dev, err := other.TakeCode(ctx, "code-1")
	if err != nil || id != u.ID || dev != "Laptop" {
		t.Fatalf("code %d %q %v", id, dev, err)
	}
	if _, _, err := db.TakeCode(ctx, "code-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("code used twice: %v", err)
	}
	// The hash is stored, not the code.
	var stored string
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM signin_codes WHERE hash = 'code-1'`).Scan(&stored); err != nil || stored != "0" {
		t.Fatalf("plain code stored: %s %v", stored, err)
	}
	if _, err := db.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
}
