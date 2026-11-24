// Package sqlstore is what Interlinea's stores (memories, termbases)
// share to run on SQLite (the desktop apps' own files) and on PostgreSQL
// (the server many translators work against at once): queries written
// once with "?" placeholders and rebound for PostgreSQL, transactions
// that serialize writers on PostgreSQL so change-log sequence numbers are
// in commit order, and the few differences between the two.
package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Dialect is the database a store runs on.
type Dialect int

const (
	SQLite Dialect = iota
	Postgres
)

func (d Dialect) String() string {
	if d == Postgres {
		return "postgres"
	}
	return "sqlite"
}

// Rebind turns "?" placeholders into PostgreSQL's "$1", "$2", ...,
// leaving question marks inside quoted strings alone.
func (d Dialect) Rebind(q string) string {
	if d != Postgres || !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 16)
	n := 0
	inString := false
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'':
			inString = !inString
			b.WriteByte(c)
		case c == '?' && !inString:
			n++
			fmt.Fprintf(&b, "$%d", n)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// IsUnique reports whether err is a unique-constraint violation.
func (d Dialect) IsUnique(err error) bool {
	if err == nil {
		return false
	}
	if d == Postgres {
		return strings.Contains(err.Error(), "SQLSTATE 23505")
	}
	return strings.Contains(err.Error(), "UNIQUE")
}

// LikeContains is a condition that column contains text (any substring,
// case ignored) on PostgreSQL, with its argument.
func LikeContains(column, text string) (string, any) {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
	return column + ` ILIKE ? ESCAPE '\'`, "%" + esc + "%"
}

// DB wraps a database so every query is rebound for its dialect.
type DB struct {
	*sql.DB
	Dialect Dialect
	// LockKey identifies the store's write lock on PostgreSQL (each store
	// has its own, so memories and termbases do not wait for each other).
	LockKey int64
}

// Tx is a transaction of a DB.
type Tx struct {
	*sql.Tx
	Dialect Dialect
}

func (w *DB) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return w.DB.ExecContext(ctx, w.Dialect.Rebind(q), args...)
}

func (w *DB) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return w.DB.QueryContext(ctx, w.Dialect.Rebind(q), args...)
}

func (w *DB) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return w.DB.QueryRowContext(ctx, w.Dialect.Rebind(q), args...)
}

// Begin starts a write transaction. On PostgreSQL it takes the store's
// transaction-level lock first, so concurrent writers, in this process
// or in other server processes, commit one after another and the change
// log's sequence numbers are in commit order: a client reading "after N"
// never misses a change committed later with a smaller number. SQLite
// allows one writer at a time anyway.
func (w *DB) Begin(ctx context.Context) (*Tx, error) {
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if w.Dialect == Postgres {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, w.LockKey); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return &Tx{Tx: tx, Dialect: w.Dialect}, nil
}

// BeginRead starts a read-only transaction (a consistent snapshot).
func (w *DB) BeginRead(ctx context.Context) (*Tx, error) {
	tx, err := w.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, Dialect: w.Dialect}, nil
}

func (t *Tx) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, t.Dialect.Rebind(q), args...)
}

func (t *Tx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, t.Dialect.Rebind(q), args...)
}

func (t *Tx) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, t.Dialect.Rebind(q), args...)
}

func (t *Tx) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return t.Tx.PrepareContext(ctx, t.Dialect.Rebind(q))
}

// Rollback rolls back unless the transaction was committed; for defer.
func Rollback(tx *Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		_ = err // the transaction is gone either way
	}
}

// MigratePostgres brings a store's schema on PostgreSQL up to date:
// migrations[i] takes it from version i to i+1, recorded in table. One
// process migrates at a time; the others wait and find it current.
func MigratePostgres(ctx context.Context, sdb *sql.DB, table string, lockKey int64, migrations []string) error {
	tx, err := sdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: migrate: %w", table, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("%s: migrate: %w", table, err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("%s: migrate: %w", table, err)
	}
	var current int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM `+table).Scan(&current); err != nil {
		return fmt.Errorf("%s: read schema version: %w", table, err)
	}
	if current > len(migrations) {
		return fmt.Errorf("%s: database schema version %d is newer than this program supports (%d)", table, current, len(migrations))
	}
	for v := current; v < len(migrations); v++ {
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			return fmt.Errorf("%s: migrate to version %d: %w", table, v+1, err)
		}
	}
	if current < len(migrations) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+table+`(version) VALUES ($1)`, len(migrations)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EnsureTrigram makes the pg_trgm extension in the public schema, where
// every store's schema sees it. CREATE EXTENSION IF NOT EXISTS does not
// keep two sessions from both finding it missing and both creating it, so
// the statement runs under a lock that servers and stores starting at the
// same time share.
func EnsureTrigram(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7421900514)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_trgm SCHEMA public`); err != nil {
		return err
	}
	return tx.Commit()
}
