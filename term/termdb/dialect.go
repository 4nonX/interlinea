package termdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// The termbase store runs on SQLite (the desktop apps' own files) and on
// PostgreSQL (the shared server many translators work against at once).
// The queries are written once, with "?" placeholders; the few places
// where the databases differ (full-text search, the order of deleted
// entries, how a unique violation is reported, the lock that keeps the
// change log in commit order) are methods of dialect.

type dialect int

const (
	sqliteDialect dialect = iota
	postgresDialect
)

// rebind turns "?" placeholders into PostgreSQL's "$1", "$2", ...,
// leaving question marks inside quoted strings alone.
func (d dialect) rebind(q string) string {
	if d != postgresDialect || !strings.Contains(q, "?") {
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

func (d dialect) isUnique(err error) bool {
	if err == nil {
		return false
	}
	if d == postgresDialect {
		return strings.Contains(err.Error(), "SQLSTATE 23505")
	}
	return strings.Contains(err.Error(), "UNIQUE")
}

// deletedOrder is the column that orders the deleted table by insertion.
func (d dialect) deletedOrder() string {
	if d == postgresDialect {
		return "id"
	}
	return "rowid"
}

// lockWrites makes concurrent writers on PostgreSQL commit one after
// another, so change-log sequence numbers are in commit order and a
// client reading "after N" never misses a change committed later with a
// smaller number. SQLite allows one writer at a time anyway.
func (d dialect) lockWrites(ctx context.Context, tx *wtx) error {
	if d != postgresDialect {
		return nil
	}
	_, err := tx.Tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7311)`)
	return err
}

// setSearch stores an entry's text for search.
func (d dialect) setSearch(ctx context.Context, tx *wtx, row int64, body string) error {
	if d == postgresDialect {
		_, err := tx.ExecContext(ctx, `UPDATE entries SET search_body = ? WHERE id = ?`, body, row)
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries_fts WHERE rowid = ?`, row); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO entries_fts(rowid, body) VALUES (?, ?)`, row, body)
	return err
}

// dropSearch removes the search text of entries (SQLite keeps it in a
// separate table; on PostgreSQL it goes with the row).
func (d dialect) dropSearch(ctx context.Context, tx *wtx, where string, args ...any) error {
	if d == postgresDialect {
		return nil
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM entries_fts WHERE rowid IN (SELECT id FROM entries WHERE `+where+`)`, args...)
	return err
}

// searchCondition selects entries whose text contains the given text
// (any substring, case ignored, at least 3 characters).
func (d dialect) searchCondition(text string) (string, any) {
	if d == postgresDialect {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
		return `search_body ILIKE ? ESCAPE '\'`, "%" + esc + "%"
	}
	return `id IN (SELECT rowid FROM entries_fts WHERE entries_fts MATCH ?)`, `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}

// wdb and wtx wrap the database and its transactions so every query is
// rebound for the dialect.
type wdb struct {
	*sql.DB
	d dialect
}

type wtx struct {
	*sql.Tx
	d dialect
}

func (w *wdb) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return w.DB.ExecContext(ctx, w.d.rebind(q), args...)
}

func (w *wdb) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return w.DB.QueryContext(ctx, w.d.rebind(q), args...)
}

func (w *wdb) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return w.DB.QueryRowContext(ctx, w.d.rebind(q), args...)
}

// begin starts a write transaction, holding the write lock on
// PostgreSQL.
func (w *wdb) begin(ctx context.Context) (*wtx, error) {
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	t := &wtx{Tx: tx, d: w.d}
	if err := w.d.lockWrites(ctx, t); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return t, nil
}

// beginRead starts a read-only transaction (a consistent snapshot).
func (w *wdb) beginRead(ctx context.Context) (*wtx, error) {
	tx, err := w.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return &wtx{Tx: tx, d: w.d}, nil
}

func (t *wtx) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, t.d.rebind(q), args...)
}

func (t *wtx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, t.d.rebind(q), args...)
}

func (t *wtx) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, t.d.rebind(q), args...)
}

// rollback rolls back unless the transaction was committed; for defer.
func rollback(tx *wtx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		_ = err // the transaction is gone either way
	}
}
