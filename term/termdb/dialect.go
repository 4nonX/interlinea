package termdb

import (
	"context"
	"strings"

	"github.com/4nonX/interlinea/internal/sqlstore"
)

// The termbase store runs on SQLite and on PostgreSQL (see sqlstore).
// What differs for termbases: where an entry's search text lives (an
// FTS5 table on SQLite, a trigram-indexed column on PostgreSQL) and the
// column that orders deleted entries.

type (
	wdb = sqlstore.DB
	wtx = sqlstore.Tx
)

const (
	sqliteDialect   = sqlstore.SQLite
	postgresDialect = sqlstore.Postgres

	// writeLock and migrateLock are the PostgreSQL advisory lock keys of
	// the termbase store.
	writeLock   = 7311
	migrateLock = 7312
)

func rollback(tx *wtx) { sqlstore.Rollback(tx) }

// deletedOrder is the column that orders the deleted table by insertion.
func deletedOrder(d sqlstore.Dialect) string {
	if d == sqlstore.Postgres {
		return "id"
	}
	return "rowid"
}

// setSearch stores an entry's text for search.
func setSearch(ctx context.Context, tx *wtx, row int64, body string) error {
	if tx.Dialect == sqlstore.Postgres {
		_, err := tx.ExecContext(ctx, `UPDATE entries SET search_body = ? WHERE id = ?`, body, row)
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries_fts WHERE rowid = ?`, row); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO entries_fts(rowid, body) VALUES (?, ?)`, row, body)
	return err
}

// dropSearch removes the search text of the entries the condition
// selects (on PostgreSQL it goes with the row).
func dropSearch(ctx context.Context, tx *wtx, where string, args ...any) error {
	if tx.Dialect == sqlstore.Postgres {
		return nil
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM entries_fts WHERE rowid IN (SELECT id FROM entries WHERE `+where+`)`, args...)
	return err
}

// searchCondition selects entries whose text contains the given text
// (any substring, case ignored, at least 3 characters).
func searchCondition(d sqlstore.Dialect, text string) (string, any) {
	if d == sqlstore.Postgres {
		return sqlstore.LikeContains("search_body", text)
	}
	return `id IN (SELECT rowid FROM entries_fts WHERE entries_fts MATCH ?)`, `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}
