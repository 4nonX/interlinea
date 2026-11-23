// Package sqlitedb opens the SQLite database files of Interlinea's stores
// (memories, termbases) with the same settings and migrates their schema.
//
// The driver is pure Go (SQLite compiled to WebAssembly, with FTS5), so no C
// compiler is needed on any platform.
package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver" // registers the "sqlite3" driver
	_ "github.com/ncruces/go-sqlite3/embed"  // embeds the SQLite build (with FTS5)
)

// Open opens or creates the database at path in WAL mode and applies the
// migrations it has not seen yet. migrations[i] brings the schema from
// version i to i+1; the version is kept in PRAGMA user_version. what names
// the store in error messages ("tmdb", "termdb").
func Open(path, what string, migrations []string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%s: empty database path", what)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	sdb, err := sql.Open("sqlite3", FileDSN(filepath.ToSlash(abs)))
	if err != nil {
		return nil, fmt.Errorf("%s: open %s: %w", what, path, err)
	}
	var mode string
	if err := sdb.QueryRow(`PRAGMA journal_mode = wal`).Scan(&mode); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("%s: enable WAL: %w", what, err)
	}
	if !strings.EqualFold(mode, "wal") {
		sdb.Close()
		return nil, fmt.Errorf("%s: database does not support WAL mode (got %q)", what, mode)
	}
	if err := migrate(context.Background(), sdb, what, migrations); err != nil {
		sdb.Close()
		return nil, err
	}
	return sdb, nil
}

func migrate(ctx context.Context, sdb *sql.DB, what string, migrations []string) error {
	var current int
	if err := sdb.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("%s: read schema version: %w", what, err)
	}
	if current > len(migrations) {
		return fmt.Errorf("%s: database schema version %d is newer than this program supports (%d)", what, current, len(migrations))
	}
	for v := current; v < len(migrations); v++ {
		tx, err := sdb.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: migrate to version %d: %w", what, v+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// IsUnique reports whether err is a UNIQUE constraint violation.
func IsUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE")
}

// Rollback rolls back tx unless it was committed; for use with defer.
func Rollback(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		_ = err // the transaction is gone either way
	}
}

// FileDSN builds the SQLite URI for an absolute path with forward slashes
// ("/home/dan/x.db" or "C:/Users/Dan/x.db") and the connection settings.
//
// The path is written without an authority ("file:/home/..." and
// "file:C:/Users/..."). The SQLite inside the driver decodes URIs the Unix
// way, so "file:///C:/..." would become "/C:/...", which Windows rejects.
func FileDSN(slashPath string) string {
	q := url.Values{}
	// busy_timeout comes first: the driver applies these pragmas to every new
	// connection, and the ones after it may need a lock that another
	// connection holds at that moment. WAL mode is stored in the database
	// file and is set once in Open.
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(normal)")
	// Write transactions take the write lock at BEGIN, where the busy timeout
	// applies; a deferred transaction that reads first could fail with
	// SQLITE_BUSY when it upgrades to a write.
	q.Set("_txlock", "immediate")
	return "file:" + escapeURIPath(slashPath) + "?" + q.Encode()
}

// escapeURIPath percent-encodes every byte of p except unreserved characters
// and the separators "/" and ":", so "?", "#", "%", spaces and non-ASCII
// characters survive the URI unchanged.
func escapeURIPath(p string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9',
			c == '-', c == '.', c == '_', c == '~', c == '/', c == ':':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
