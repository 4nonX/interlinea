//go:build postgres

package termdb

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/internal/pgdriver"
	"github.com/4nonX/interlinea/internal/sqlstore"
)

// With the postgres build tag and INTERLINEA_TEST_POSTGRES set to a
// connection URL, every test of this package runs against PostgreSQL,
// each in a schema of its own that is dropped afterwards.

func pgDSN() string { return os.Getenv("INTERLINEA_TEST_POSTGRES") }

func openPG(t *testing.T) (*DB, string) {
	t.Helper()
	admin, err := pgdriver.Open(ctx, pgDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	b := make([]byte, 6)
	rand.Read(b)
	schema := "t_" + hex.EncodeToString(b)
	if _, err := admin.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_trgm SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a, err := pgdriver.Open(ctx, pgDSN())
		if err == nil {
			a.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`)
			a.Close()
		}
	})
	u, err := url.Parse(pgDSN())
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	dsn := u.String()
	db := reopenPG(t, dsn)
	clock := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	db.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	return db, dsn
}

func reopenPG(t *testing.T, dsn string) *DB {
	t.Helper()
	sdb, err := pgdriver.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenPostgres(ctx, sdb)
	if err != nil {
		sdb.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func isPG(key string) bool { return strings.HasPrefix(key, "postgres") }

// TestRunsOnPostgres makes sure the tagged run really uses PostgreSQL,
// so a missing or misspelled variable fails CI instead of quietly
// testing SQLite twice.
func TestRunsOnPostgres(t *testing.T) {
	if pgDSN() == "" {
		t.Fatal("built with the postgres tag but INTERLINEA_TEST_POSTGRES is not set")
	}
	db, _ := open(t)
	if db.d != sqlstore.Postgres {
		t.Fatalf("dialect = %v", db.d)
	}
	var schema string
	if err := db.sql.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema); err != nil || !strings.HasPrefix(schema, "t_") {
		t.Fatalf("schema = %q, %v", schema, err)
	}
}
