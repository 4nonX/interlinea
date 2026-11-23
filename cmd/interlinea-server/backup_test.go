package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmdb"
)

// A backup taken while the stores are open has what was written to them,
// and opens as a data folder.
func TestBackup(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	auth, err := authdb.Open(filepath.Join(data, "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	if _, err := auth.CreateUser(ctx, authdb.NewUser{Username: "anna", Password: "a long enough password", Admin: true}, "test"); err != nil {
		t.Fatal(err)
	}
	mems, err := tmdb.Open(filepath.Join(data, "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mems.Close()
	m, err := mems.Create(ctx, tm.Info{Name: "Weber DE-EN", SourceLang: "de-DE", TargetLang: "en-GB"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text("Der Mieter zahlt die Miete."), Target: content.Text("The tenant pays the rent."), CreatedBy: "anna"}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	projects, err := projectdb.Open(filepath.Join(data, "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer projects.Close()
	terms, err := termdb.Open(filepath.Join(data, "termbases.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer terms.Close()

	to := filepath.Join(t.TempDir(), "backup")
	var out, errs bytes.Buffer
	if err := backup([]string{"-data", data, to}, &out, &errs); err != nil {
		t.Fatalf("%v\n%s", err, errs.String())
	}
	if !strings.Contains(out.String(), "backup written to") {
		t.Fatalf("output %q", out.String())
	}
	for _, name := range storeFiles {
		st, err := os.Stat(filepath.Join(to, name))
		if err != nil || st.Size() == 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// The copies open, with the data.
	a2, err := authdb.Open(filepath.Join(to, "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Close()
	if u, err := a2.UserByName(ctx, "anna"); err != nil || !u.Admin {
		t.Fatalf("anna in the backup: %+v %v", u, err)
	}
	m2, err := tmdb.Open(filepath.Join(to, "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	mem, err := m2.Memory(ctx, "Weber DE-EN")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := mem.Count(ctx); err != nil || n != 1 {
		t.Fatalf("units in the backup: %d %v", n, err)
	}

	// Not over an earlier backup, and not from a folder that isn't one.
	if err := backup([]string{"-data", data, to}, &out, &errs); err == nil || !strings.Contains(err.Error(), "exists already") {
		t.Fatalf("backup over a backup: %v", err)
	}
	if err := backup([]string{"-data", t.TempDir(), filepath.Join(t.TempDir(), "b")}, &out, &errs); err == nil || !strings.Contains(err.Error(), "not a server's data folder") {
		t.Fatalf("backup of an empty folder: %v", err)
	}
	t.Setenv("INTERLINEA_POSTGRES", "postgres://x")
	if err := backup([]string{"-data", data, filepath.Join(t.TempDir(), "c")}, &out, &errs); err == nil || !strings.Contains(err.Error(), "pg_dump") {
		t.Fatalf("backup on PostgreSQL: %v", err)
	}
}
