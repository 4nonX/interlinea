//go:build postgres

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/internal/pgdriver"
)

// TestPostgresSchemas opens the four stores on PostgreSQL, each in its
// own schema (their tables share names), twice, as a restarted server
// does.
func TestPostgresSchemas(t *testing.T) {
	dsn := os.Getenv("INTERLINEA_TEST_POSTGRES")
	if dsn == "" {
		t.Fatal("built with the postgres tag but INTERLINEA_TEST_POSTGRES is not set")
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	o := options{postgres: dsn}
	for round := 0; round < 2; round++ {
		st, closeAll, err := openStores(ctx, o, log)
		if err != nil {
			t.Fatal(err)
		}
		if round == 0 {
			if _, err := st.Auth.CreateUser(ctx, authdb.NewUser{Username: "anna", Password: "a long enough password", Admin: true}, "test"); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := st.Auth.CountUsers(ctx); err != nil || n != 1 {
			t.Fatalf("round %d: %d users, %v", round, n, err)
		}
		if _, err := st.Projects.List(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Memories.List(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Termbases.List(ctx); err != nil {
			t.Fatal(err)
		}
		closeAll()
	}
	db, err := pgdriver.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range storeSchemas {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'changes'`, s).Scan(&n); err != nil || n != 1 {
			t.Fatalf("schema %s: %d change tables, %v", s, n, err)
		}
		if _, err := db.ExecContext(ctx, `DROP SCHEMA `+s+` CASCADE`); err != nil {
			t.Fatal(err)
		}
	}
}
