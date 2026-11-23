package termdb

import (
	"context"
	"database/sql"
	"fmt"
)

// pgMigrations are the PostgreSQL schema versions; the same tables as on
// SQLite after its third migration, with search text in a trigram-indexed
// column instead of an FTS5 table.
var pgMigrations = []string{
	`CREATE EXTENSION IF NOT EXISTS pg_trgm;
	CREATE TABLE termbases (
		id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		name            TEXT NOT NULL,
		definition_json TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		next_entry      BIGINT NOT NULL DEFAULT 1,
		kind            TEXT NOT NULL DEFAULT 'master',
		rules_json      TEXT NOT NULL DEFAULT '{}'
	);
	CREATE UNIQUE INDEX termbases_by_name ON termbases (lower(name));
	CREATE TABLE termbase_upstreams (
		project_id BIGINT NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		master_id  BIGINT NOT NULL REFERENCES termbases(id),
		position   INTEGER NOT NULL,
		PRIMARY KEY (project_id, master_id)
	);
	CREATE TABLE entries (
		id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		termbase_id   BIGINT NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		entry_id      TEXT NOT NULL,
		version       INTEGER NOT NULL,
		entry_json    TEXT NOT NULL,
		created_at    TEXT NOT NULL,
		created_by    TEXT NOT NULL DEFAULT '',
		changed_at    TEXT NOT NULL,
		changed_by    TEXT NOT NULL DEFAULT '',
		base_termbase BIGINT,
		base_entry    TEXT,
		base_version  INTEGER,
		base_synced   INTEGER,
		search_body   TEXT NOT NULL DEFAULT '',
		UNIQUE (termbase_id, entry_id)
	);
	CREATE INDEX entries_search ON entries USING gin (search_body gin_trgm_ops);
	CREATE TABLE terms (
		entry_row    BIGINT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		termbase_id  BIGINT NOT NULL,
		lang         TEXT NOT NULL,
		primary_lang TEXT NOT NULL,
		key          TEXT NOT NULL,
		text         TEXT NOT NULL,
		status       TEXT NOT NULL
	);
	CREATE INDEX terms_by_key ON terms(termbase_id, primary_lang, key);
	CREATE INDEX terms_by_entry ON terms(entry_row);
	CREATE TABLE versions (
		entry_row  BIGINT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		version    INTEGER NOT NULL,
		at         TEXT NOT NULL,
		author     TEXT NOT NULL,
		summary    TEXT NOT NULL,
		entry_json TEXT NOT NULL,
		PRIMARY KEY (entry_row, version)
	);
	CREATE TABLE deleted (
		id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		termbase_id BIGINT NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		entry_id    TEXT NOT NULL,
		at          TEXT NOT NULL,
		author      TEXT NOT NULL,
		summary     TEXT NOT NULL,
		entry_json  TEXT NOT NULL
	);
	CREATE INDEX deleted_by_termbase ON deleted(termbase_id, at);
	CREATE TABLE changes (
		seq         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		termbase_id BIGINT NOT NULL,
		termbase    TEXT NOT NULL,
		entry_id    TEXT NOT NULL DEFAULT '',
		version     INTEGER NOT NULL DEFAULT 0,
		kind        TEXT NOT NULL,
		at          TEXT NOT NULL,
		author      TEXT NOT NULL,
		summary     TEXT NOT NULL
	);
	CREATE INDEX changes_by_termbase ON changes(termbase_id, seq);
	CREATE TABLE project_removals (
		project_id    BIGINT NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		base_termbase BIGINT NOT NULL,
		base_entry    TEXT NOT NULL,
		base_version  INTEGER NOT NULL,
		at            TEXT NOT NULL,
		author        TEXT NOT NULL,
		PRIMARY KEY (project_id, base_termbase, base_entry)
	);
	CREATE TABLE merge_requests (
		id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		project_id BIGINT NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		target_id  BIGINT NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		title      TEXT NOT NULL,
		created_at TEXT NOT NULL,
		created_by TEXT NOT NULL,
		deadline   TEXT NOT NULL DEFAULT '',
		state      TEXT NOT NULL DEFAULT 'open',
		closed_at  TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE merge_items (
		id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		request_id      BIGINT NOT NULL REFERENCES merge_requests(id) ON DELETE CASCADE,
		entry_id        TEXT NOT NULL,
		project_version INTEGER NOT NULL,
		kind            TEXT NOT NULL,
		base_entry      TEXT NOT NULL DEFAULT '',
		base_version    INTEGER NOT NULL DEFAULT 0,
		author          TEXT NOT NULL DEFAULT '',
		changed_at      TEXT NOT NULL,
		state           TEXT NOT NULL DEFAULT 'waiting',
		reviewer        TEXT NOT NULL DEFAULT '',
		decided_at      TEXT NOT NULL DEFAULT '',
		comment         TEXT NOT NULL DEFAULT '',
		rule            TEXT NOT NULL DEFAULT '',
		result_entry    TEXT NOT NULL DEFAULT '',
		result_version  INTEGER NOT NULL DEFAULT 0,
		master_before   INTEGER NOT NULL DEFAULT 0,
		UNIQUE (request_id, kind, entry_id)
	);
	CREATE INDEX merge_items_by_request ON merge_items(request_id, state);
	CREATE TABLE merge_assignments (
		request_id  BIGINT NOT NULL REFERENCES merge_requests(id) ON DELETE CASCADE,
		reviewer    TEXT NOT NULL,
		filter_json TEXT NOT NULL,
		PRIMARY KEY (request_id, reviewer)
	);`,
}

// OpenPostgres uses a PostgreSQL database (opened with any database/sql
// driver for it, such as pgx's) for termbases and brings its schema up to
// date. Several server processes may share the database: writes take a
// transaction-level lock so the change log stays in commit order. The
// database user needs the right to create the pg_trgm extension once.
func OpenPostgres(ctx context.Context, sdb *sql.DB) (*DB, error) {
	if err := migratePostgres(ctx, sdb); err != nil {
		return nil, err
	}
	return newDB(sdb, postgresDialect), nil
}

func migratePostgres(ctx context.Context, sdb *sql.DB) error {
	tx, err := sdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("termdb: migrate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// One migrating process at a time; the others wait and then find the
	// schema current.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7312)`); err != nil {
		return fmt.Errorf("termdb: migrate: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS termdb_schema (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("termdb: migrate: %w", err)
	}
	var current int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM termdb_schema`).Scan(&current); err != nil {
		return fmt.Errorf("termdb: read schema version: %w", err)
	}
	if current > len(pgMigrations) {
		return fmt.Errorf("termdb: database schema version %d is newer than this program supports (%d)", current, len(pgMigrations))
	}
	for v := current; v < len(pgMigrations); v++ {
		if _, err := tx.ExecContext(ctx, pgMigrations[v]); err != nil {
			return fmt.Errorf("termdb: migrate to version %d: %w", v+1, err)
		}
	}
	if current < len(pgMigrations) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM termdb_schema`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO termdb_schema(version) VALUES ($1)`, len(pgMigrations)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
