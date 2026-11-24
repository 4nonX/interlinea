// Package tmdb stores translation memories in a SQLite file (the desktop
// apps' own) or a PostgreSQL database (the server many translators use
// at once).
//
// One database holds any number of memories. Exact matches use an index
// on the normalized source. Fuzzy candidates come from a term index
// (units sharing the most words) and are scored with the fuzzy package,
// so results agree with the in-memory backend. Concordance search finds
// any substring, case-insensitively: with an FTS5 trigram index on
// SQLite, a pg_trgm index on PostgreSQL.
//
// Every unit has a version, and an edit made on a stale copy is refused
// (ErrConflict). Every created, changed and deleted unit is recorded in a
// change log in the same transaction, with a sequence number clients sync
// from (Changes).
package tmdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/4nonX/interlinea/internal/textnorm"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/fuzzy"
	"github.com/4nonX/interlinea/internal/sqlitedb"
	"github.com/4nonX/interlinea/internal/sqlstore"
	"github.com/4nonX/interlinea/tm"
)

// Errors returned by this package.
var (
	ErrNotFound = errors.New("tmdb: not found")
	ErrExists   = errors.New("tmdb: a memory with this name already exists")
	// ErrConflict means the unit changed since the version the caller
	// edited; load it again and reapply the change.
	ErrConflict = errors.New("tmdb: the unit was changed by someone else")
)

const (
	candidateLimit = 200
	timeLayout     = time.RFC3339Nano

	writeLock   = 7321
	migrateLock = 7322
)

// Kinds of change in the change log.
const (
	ChangeCreated       = "created"
	ChangeChanged       = "changed"
	ChangeDeleted       = "deleted"
	ChangeMemoryCreated = "memory-created"
	ChangeMemoryDeleted = "memory-deleted"
)

// DB is an open database. It is safe for concurrent use; writes are
// serialized inside the process, by SQLite's locking between processes
// and by a transaction lock on PostgreSQL.
type DB struct {
	sql *sqlstore.DB
	d   sqlstore.Dialect
	wmu sync.Mutex
	now func() time.Time
}

// Open opens or creates the SQLite database at path and brings its schema
// up to date. Parent directories are created as needed.
func Open(path string) (*DB, error) {
	sdb, err := sqlitedb.Open(path, "tmdb", migrations)
	if err != nil {
		return nil, err
	}
	return newDB(sdb, sqlstore.SQLite), nil
}

// OpenPostgres uses a PostgreSQL database (opened with any database/sql
// driver for it) for memories and brings its schema up to date. The
// database user needs the right to create the pg_trgm extension once.
func OpenPostgres(ctx context.Context, sdb *sql.DB) (*DB, error) {
	if err := sqlstore.MigratePostgres(ctx, sdb, "tmdb_schema", migrateLock, pgMigrations); err != nil {
		return nil, err
	}
	return newDB(sdb, sqlstore.Postgres), nil
}

func newDB(sdb *sql.DB, d sqlstore.Dialect) *DB {
	return &DB{sql: &sqlstore.DB{DB: sdb, Dialect: d, LockKey: writeLock}, d: d, now: func() time.Time { return time.Now().UTC() }}
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

var migrations = []string{
	// Version 1: memories, units, exact-match index, term index for fuzzy
	// candidates, FTS5 trigram index for concordance.
	`CREATE TABLE memories (
		id          INTEGER PRIMARY KEY,
		name        TEXT NOT NULL UNIQUE COLLATE NOCASE,
		source_lang TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		created_at  TEXT NOT NULL
	);
	CREATE TABLE units (
		id              INTEGER PRIMARY KEY,
		memory_id       INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
		source_key      TEXT NOT NULL,
		source_plain    TEXT NOT NULL,
		source_json     TEXT NOT NULL,
		target_plain    TEXT NOT NULL,
		target_json     TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		created_by      TEXT NOT NULL DEFAULT '',
		changed_at      TEXT NOT NULL,
		changed_by      TEXT NOT NULL DEFAULT '',
		usage_count     INTEGER NOT NULL DEFAULT 0,
		attributes_json TEXT NOT NULL DEFAULT '[]',
		notes_json      TEXT NOT NULL DEFAULT '[]'
	);
	CREATE INDEX units_by_key ON units(memory_id, source_key);
	CREATE INDEX units_by_memory ON units(memory_id, id);
	CREATE TABLE unit_terms (
		memory_id INTEGER NOT NULL,
		term      TEXT NOT NULL,
		unit_id   INTEGER NOT NULL REFERENCES units(id) ON DELETE CASCADE,
		PRIMARY KEY (memory_id, term, unit_id)
	) WITHOUT ROWID;
	CREATE INDEX unit_terms_by_unit ON unit_terms(unit_id);
	CREATE VIRTUAL TABLE units_fts USING fts5(
		source_plain, target_plain,
		content='units', content_rowid='id', tokenize='trigram'
	);
	CREATE TRIGGER units_ai AFTER INSERT ON units BEGIN
		INSERT INTO units_fts(rowid, source_plain, target_plain)
		VALUES (new.id, new.source_plain, new.target_plain);
	END;
	CREATE TRIGGER units_ad AFTER DELETE ON units BEGIN
		INSERT INTO units_fts(units_fts, rowid, source_plain, target_plain)
		VALUES ('delete', old.id, old.source_plain, old.target_plain);
	END;
	CREATE TRIGGER units_au AFTER UPDATE OF source_plain, target_plain ON units BEGIN
		INSERT INTO units_fts(units_fts, rowid, source_plain, target_plain)
		VALUES ('delete', old.id, old.source_plain, old.target_plain);
		INSERT INTO units_fts(rowid, source_plain, target_plain)
		VALUES (new.id, new.source_plain, new.target_plain);
	END;`,

	// Version 2: unit versions and the change log, filled in from the
	// units there are.
	`ALTER TABLE units ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
	CREATE TABLE changes (
		seq       INTEGER PRIMARY KEY AUTOINCREMENT,
		memory_id INTEGER NOT NULL,
		memory    TEXT NOT NULL,
		unit_id   INTEGER NOT NULL DEFAULT 0,
		version   INTEGER NOT NULL DEFAULT 0,
		kind      TEXT NOT NULL,
		at        TEXT NOT NULL,
		author    TEXT NOT NULL
	);
	CREATE INDEX changes_by_memory ON changes(memory_id, seq);
	INSERT INTO changes(memory_id, memory, unit_id, version, kind, at, author)
		SELECT m.id, m.name, 0, 0, 'memory-created', m.created_at, '' FROM memories m ORDER BY m.id;
	INSERT INTO changes(memory_id, memory, unit_id, version, kind, at, author)
		SELECT u.memory_id, m.name, u.id, 1, 'created', u.created_at, u.created_by
		FROM units u JOIN memories m ON m.id = u.memory_id ORDER BY u.id;`,

	// Version 3: who a memory is kept for.
	`ALTER TABLE memories ADD COLUMN client TEXT NOT NULL DEFAULT '';`,
}

var pgMigrations = []string{
	`CREATE EXTENSION IF NOT EXISTS pg_trgm;
	CREATE TABLE memories (
		id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		name        TEXT NOT NULL,
		source_lang TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		created_at  TEXT NOT NULL
	);
	CREATE UNIQUE INDEX memories_by_name ON memories (lower(name));
	CREATE TABLE units (
		id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		memory_id       BIGINT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
		source_key      BYTEA NOT NULL,
		source_plain    TEXT NOT NULL,
		source_json     TEXT NOT NULL,
		target_plain    TEXT NOT NULL,
		target_json     TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		created_by      TEXT NOT NULL DEFAULT '',
		changed_at      TEXT NOT NULL,
		changed_by      TEXT NOT NULL DEFAULT '',
		usage_count     INTEGER NOT NULL DEFAULT 0,
		attributes_json TEXT NOT NULL DEFAULT '[]',
		notes_json      TEXT NOT NULL DEFAULT '[]',
		version         INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX units_by_key ON units(memory_id, source_key);
	CREATE INDEX units_by_memory ON units(memory_id, id);
	CREATE INDEX units_source_search ON units USING gin (source_plain gin_trgm_ops);
	CREATE INDEX units_target_search ON units USING gin (target_plain gin_trgm_ops);
	CREATE TABLE unit_terms (
		memory_id BIGINT NOT NULL,
		term      TEXT NOT NULL,
		unit_id   BIGINT NOT NULL REFERENCES units(id) ON DELETE CASCADE,
		PRIMARY KEY (memory_id, term, unit_id)
	);
	CREATE INDEX unit_terms_by_unit ON unit_terms(unit_id);
	CREATE TABLE changes (
		seq       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		memory_id BIGINT NOT NULL,
		memory    TEXT NOT NULL,
		unit_id   BIGINT NOT NULL DEFAULT 0,
		version   INTEGER NOT NULL DEFAULT 0,
		kind      TEXT NOT NULL,
		at        TEXT NOT NULL,
		author    TEXT NOT NULL
	);
	CREATE INDEX changes_by_memory ON changes(memory_id, seq);`,

	`ALTER TABLE memories ADD COLUMN client TEXT NOT NULL DEFAULT '';`,
}

func logChange(ctx context.Context, tx *sqlstore.Tx, memoryID int64, memory string, unitID int64, version int, kind, by string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO changes(memory_id, memory, unit_id, version, kind, at, author)
		VALUES (?, COALESCE(NULLIF(?, ''), (SELECT name FROM memories WHERE id = ?), ''), ?, ?, ?, ?, ?)`,
		memoryID, memory, memoryID, unitID, version, kind, now.Format(timeLayout), noNUL(by))
	if err != nil {
		return fmt.Errorf("tmdb: change log: %w", err)
	}
	return nil
}

// Summary describes a stored memory.
type Summary struct {
	tm.Info
	Units     int
	CreatedAt time.Time
}

// Create adds a new, empty memory.
func (d *DB) Create(ctx context.Context, info tm.Info) (*Memory, error) {
	return d.CreateBy(ctx, info, "")
}

// CreateBy is Create, recording who created the memory.
func (d *DB) CreateBy(ctx context.Context, info tm.Info, by string) (*Memory, error) {
	info, err := tm.ValidateInfo(info)
	if err != nil {
		return nil, err
	}
	d.wmu.Lock()
	defer d.wmu.Unlock()
	tx, err := d.sql.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer sqlstore.Rollback(tx)
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories WHERE lower(name) = lower(?)`, info.Name).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrExists
	}
	now := d.now()
	var id int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO memories(name, source_lang, target_lang, client, created_at) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		info.Name, info.SourceLang, info.TargetLang, noNUL(info.Client), now.Format(timeLayout)).Scan(&id)
	if err != nil {
		if d.d.IsUnique(err) {
			return nil, ErrExists
		}
		return nil, fmt.Errorf("tmdb: create memory: %w", err)
	}
	if err := logChange(ctx, tx, id, info.Name, 0, 0, ChangeMemoryCreated, by, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Memory{db: d, id: id, info: info}, nil
}

// Memory returns the memory with the given name (case-insensitive).
func (d *DB) Memory(ctx context.Context, name string) (*Memory, error) {
	m := &Memory{db: d}
	err := d.sql.QueryRowContext(ctx,
		`SELECT id, name, source_lang, target_lang, client FROM memories WHERE lower(name) = lower(?)`, strings.TrimSpace(name)).
		Scan(&m.id, &m.info.Name, &m.info.SourceLang, &m.info.TargetLang, &m.info.Client)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tmdb: load memory: %w", err)
	}
	return m, nil
}

// List returns all memories ordered by name.
func (d *DB) List(ctx context.Context) ([]Summary, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT m.name, m.source_lang, m.target_lang, m.client, m.created_at,
		       (SELECT count(*) FROM units u WHERE u.memory_id = m.id)
		FROM memories m ORDER BY lower(m.name)`)
	if err != nil {
		return nil, fmt.Errorf("tmdb: list: %w", err)
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var s Summary
		var created string
		if err := rows.Scan(&s.Name, &s.SourceLang, &s.TargetLang, &s.Client, &created, &s.Units); err != nil {
			return nil, err
		}
		s.CreatedAt = parseTime(created)
		out = append(out, s)
	}
	return out, rows.Err()
}

// Delete removes a memory and all its units.
func (d *DB) Delete(ctx context.Context, name string) error {
	return d.DeleteBy(ctx, name, "")
}

// DeleteBy is Delete, recording who deleted the memory.
func (d *DB) DeleteBy(ctx context.Context, name, by string) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	tx, err := d.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer sqlstore.Rollback(tx)
	var id int64
	var real string
	err = tx.QueryRowContext(ctx, `SELECT id, name FROM memories WHERE lower(name) = lower(?)`, strings.TrimSpace(name)).Scan(&id, &real)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id); err != nil {
		return fmt.Errorf("tmdb: delete: %w", err)
	}
	if err := logChange(ctx, tx, id, real, 0, 0, ChangeMemoryDeleted, by, d.now()); err != nil {
		return err
	}
	return tx.Commit()
}

// Change is one entry of the change log. Seq increases with every change
// across the database; a client that has seen everything up to Seq asks
// for what came after it.
type Change struct {
	Seq     int64     `json:"seq"`
	Memory  string    `json:"memory"`
	UnitID  int64     `json:"unitId,omitempty"`
	Version int       `json:"version,omitempty"`
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
	By      string    `json:"by"`
}

// ChangeQuery selects changes.
type ChangeQuery struct {
	// Since returns changes after this sequence number.
	Since int64
	// Memories limits the log to these memories (by current or former
	// name); empty for all.
	Memories []string
	// Limit is the most changes returned (default 1000, at most 10000).
	Limit int
}

// Changes returns changes in order; more reports whether there are more.
func (d *DB) Changes(ctx context.Context, q ChangeQuery) (changes []Change, more bool, err error) {
	if q.Limit <= 0 {
		q.Limit = 1000
	}
	if q.Limit > 10000 {
		q.Limit = 10000
	}
	query := `SELECT seq, memory, unit_id, version, kind, at, author FROM changes WHERE seq > ?`
	args := []any{q.Since}
	if len(q.Memories) > 0 {
		var ph []string
		var names []any
		for _, n := range q.Memories {
			ph = append(ph, "lower(?)")
			names = append(names, strings.TrimSpace(n))
		}
		list := strings.Join(ph, ",")
		query += ` AND (memory_id IN (SELECT id FROM memories WHERE lower(name) IN (` + list + `)) OR lower(memory) IN (` + list + `))`
		args = append(args, names...)
		args = append(args, names...)
	}
	query += ` ORDER BY seq LIMIT ?`
	args = append(args, q.Limit+1)
	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Change
		var at string
		if err := rows.Scan(&c.Seq, &c.Memory, &c.UnitID, &c.Version, &c.Kind, &at, &c.By); err != nil {
			return nil, false, err
		}
		c.At = parseTime(at)
		changes = append(changes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(changes) > q.Limit {
		return changes[:q.Limit], true, nil
	}
	return changes, false, nil
}

// LastChange is the sequence number of the latest change (0 if none).
func (d *DB) LastChange(ctx context.Context) (int64, error) {
	var seq int64
	err := d.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM changes`).Scan(&seq)
	return seq, err
}

// Memory is one stored translation memory. It implements tm.Memory and tm.BatchAdder.
type Memory struct {
	db   *DB
	id   int64
	info tm.Info
}

var (
	_ tm.Memory     = (*Memory)(nil)
	_ tm.BatchAdder = (*Memory)(nil)
)

// Info implements tm.Memory.
func (m *Memory) Info() tm.Info { return m.info }

// Add implements tm.Memory.
func (m *Memory) Add(ctx context.Context, u tm.Unit, policy tm.AddPolicy) (tm.Unit, error) {
	var stored tm.Unit
	err := m.write(ctx, func(w *writeStmts) error {
		var err error
		stored, err = m.addTx(ctx, w, u, policy)
		return err
	})
	return stored, err
}

// AddBatch implements tm.BatchAdder: all units are stored in one transaction.
func (m *Memory) AddBatch(ctx context.Context, units []tm.Unit, policy tm.AddPolicy) error {
	return m.write(ctx, func(w *writeStmts) error {
		for _, u := range units {
			if _, err := m.addTx(ctx, w, u, policy); err != nil {
				return err
			}
		}
		return nil
	})
}

// writeStmts are the statements used by addTx, prepared once per transaction.
type writeStmts struct {
	tx         *sqlstore.Tx
	byKey      *sql.Stmt
	insertUnit *sql.Stmt
	updateUnit *sql.Stmt
	insertTerm *sql.Stmt
}

func (w *writeStmts) close() {
	for _, st := range []*sql.Stmt{w.byKey, w.insertUnit, w.updateUnit, w.insertTerm} {
		if st != nil {
			st.Close()
		}
	}
}

func (m *Memory) write(ctx context.Context, fn func(*writeStmts) error) error {
	m.db.wmu.Lock()
	defer m.db.wmu.Unlock()
	tx, err := m.db.sql.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tmdb: begin: %w", err)
	}
	defer sqlstore.Rollback(tx)
	w := &writeStmts{tx: tx}
	defer w.close()
	prep := func(dst **sql.Stmt, query string) {
		if err == nil {
			*dst, err = tx.PrepareContext(ctx, query)
		}
	}
	prep(&w.byKey, `SELECT `+unitColumns+` FROM units WHERE memory_id = ? AND source_key = ? ORDER BY id`)
	prep(&w.insertUnit, `INSERT INTO units(
			memory_id, source_key, source_plain, source_json, target_plain, target_json,
			created_at, created_by, changed_at, changed_by, usage_count, attributes_json, notes_json, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1) RETURNING id`)
	prep(&w.updateUnit, `UPDATE units SET
			source_key = ?, source_plain = ?, source_json = ?, target_plain = ?, target_json = ?,
			changed_at = ?, changed_by = ?, usage_count = ?, attributes_json = ?, notes_json = ?, version = version + 1
		WHERE id = ? AND version = ?`)
	prep(&w.insertTerm, `INSERT INTO unit_terms(memory_id, term, unit_id) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`)
	if err != nil {
		return fmt.Errorf("tmdb: prepare: %w", err)
	}
	if err := fn(w); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tmdb: commit: %w", err)
	}
	return nil
}

func (m *Memory) addTx(ctx context.Context, w *writeStmts, u tm.Unit, policy tm.AddPolicy) (tm.Unit, error) {
	now := m.db.now()
	prepared, err := tm.Prepare(u, now)
	if err != nil {
		return tm.Unit{}, err
	}
	key := tm.SourceKey(prepared.Source)
	rows, err := w.byKey.QueryContext(ctx, m.id, m.db.keyArg(key))
	if err != nil {
		return tm.Unit{}, fmt.Errorf("tmdb: query: %w", err)
	}
	existing, err := scanUnits(rows)
	if err != nil {
		return tm.Unit{}, err
	}
	var target *tm.Unit
	switch policy {
	case tm.Overwrite:
		if n := len(existing); n > 0 {
			target = &existing[n-1]
		}
	default:
		for i := range existing {
			if existing[i].Target.Equal(prepared.Target) {
				target = &existing[i]
				break
			}
		}
	}
	if target != nil {
		upd := *target
		upd.Source = prepared.Source
		upd.Target = prepared.Target
		upd.ChangedAt = u.ChangedAt
		if upd.ChangedAt.IsZero() {
			upd.ChangedAt = now
		}
		upd.ChangedAt = upd.ChangedAt.UTC()
		if u.ChangedBy != "" {
			upd.ChangedBy = u.ChangedBy
		}
		if len(u.Attributes) > 0 {
			upd.Attributes = prepared.Attributes
		}
		if len(u.Notes) > 0 {
			upd.Notes = prepared.Notes
		}
		if u.UsageCount > upd.UsageCount {
			upd.UsageCount = u.UsageCount
		}
		if err := m.updateTx(ctx, w, upd, target.Version, now); err != nil {
			return tm.Unit{}, err
		}
		upd.Version = target.Version + 1
		return upd, nil
	}
	row, err := encode(prepared)
	if err != nil {
		return tm.Unit{}, err
	}
	err = w.insertUnit.QueryRowContext(ctx,
		m.id, m.db.keyArg(key), row.sourcePlain, row.sourceJSON, row.targetPlain, row.targetJSON,
		row.createdAt, noNUL(prepared.CreatedBy), row.changedAt, noNUL(prepared.ChangedBy),
		prepared.UsageCount, row.attrs, row.notes).Scan(&prepared.ID)
	if err != nil {
		return tm.Unit{}, fmt.Errorf("tmdb: insert unit: %w", err)
	}
	prepared.Version = 1
	if err := m.indexTerms(ctx, w, prepared); err != nil {
		return tm.Unit{}, err
	}
	if err := logChange(ctx, w.tx, m.id, "", prepared.ID, 1, ChangeCreated, prepared.ChangedBy, now); err != nil {
		return tm.Unit{}, err
	}
	return prepared, nil
}

func (m *Memory) indexTerms(ctx context.Context, w *writeStmts, u tm.Unit) error {
	for _, term := range tm.SearchTerms(u.Source) {
		if _, err := w.insertTerm.ExecContext(ctx, m.id, term, u.ID); err != nil {
			return fmt.Errorf("tmdb: index unit: %w", err)
		}
	}
	return nil
}

// updateTx writes a changed unit if it is still at version, and logs it.
func (m *Memory) updateTx(ctx context.Context, w *writeStmts, u tm.Unit, version int, now time.Time) error {
	row, err := encode(u)
	if err != nil {
		return err
	}
	res, err := w.updateUnit.ExecContext(ctx,
		m.db.keyArg(tm.SourceKey(u.Source)), row.sourcePlain, row.sourceJSON, row.targetPlain, row.targetJSON,
		row.changedAt, noNUL(u.ChangedBy), u.UsageCount, row.attrs, row.notes, u.ID, version)
	if err != nil {
		return fmt.Errorf("tmdb: update unit: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrConflict
	}
	if _, err := w.tx.ExecContext(ctx, `DELETE FROM unit_terms WHERE unit_id = ?`, u.ID); err != nil {
		return err
	}
	if err := m.indexTerms(ctx, w, u); err != nil {
		return err
	}
	return logChange(ctx, w.tx, m.id, "", u.ID, version+1, ChangeChanged, u.ChangedBy, now)
}

// Unit returns a unit by ID.
func (m *Memory) Unit(ctx context.Context, id int64) (tm.Unit, error) {
	us, err := m.queryUnits(ctx, m.db.sql, `WHERE memory_id = ? AND id = ?`, m.id, id)
	if err != nil {
		return tm.Unit{}, err
	}
	if len(us) == 0 {
		return tm.Unit{}, ErrNotFound
	}
	return us[0], nil
}

// Update replaces a unit's source, target, attributes and notes, as the
// memory editor does. version is the version the caller edited; if the
// unit changed since, ErrConflict is returned and nothing is written.
func (m *Memory) Update(ctx context.Context, u tm.Unit, version int, by string) (tm.Unit, error) {
	var out tm.Unit
	err := m.write(ctx, func(w *writeStmts) error {
		us, err := m.queryUnits(ctx, w.tx, `WHERE memory_id = ? AND id = ?`, m.id, u.ID)
		if err != nil {
			return err
		}
		if len(us) == 0 {
			return ErrNotFound
		}
		cur := us[0]
		if cur.Version != version {
			return ErrConflict
		}
		now := m.db.now()
		p, err := tm.Prepare(u, now)
		if err != nil {
			return err
		}
		next := cur
		next.Source, next.Target = p.Source, p.Target
		next.Attributes, next.Notes = p.Attributes, p.Notes
		next.ChangedAt, next.ChangedBy = now, by
		if err := m.updateTx(ctx, w, next, version, now); err != nil {
			return err
		}
		next.Version = version + 1
		out = next
		return nil
	})
	return out, err
}

// DeleteUnit deletes a unit.
func (m *Memory) DeleteUnit(ctx context.Context, id int64, by string) error {
	return m.write(ctx, func(w *writeStmts) error {
		var version int
		err := w.tx.QueryRowContext(ctx, `SELECT version FROM units WHERE memory_id = ? AND id = ?`, m.id, id).Scan(&version)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := w.tx.ExecContext(ctx, `DELETE FROM units WHERE id = ?`, id); err != nil {
			return err
		}
		return logChange(ctx, w.tx, m.id, "", id, version, ChangeDeleted, by, m.db.now())
	})
}

// Lookup implements tm.Memory.
func (m *Memory) Lookup(ctx context.Context, source content.Content, opts tm.LookupOptions) ([]tm.Match, error) {
	if opts.MinScore <= 0 {
		opts.MinScore = 70
	}
	if opts.Limit <= 0 {
		opts.Limit = 5
	}
	byID := map[int64]tm.Unit{}
	exact, err := m.queryUnits(ctx, m.db.sql,
		`WHERE memory_id = ? AND source_key = ? ORDER BY id DESC LIMIT ?`, m.id, m.db.keyArg(tm.SourceKey(source)), candidateLimit)
	if err != nil {
		return nil, err
	}
	for _, u := range exact {
		byID[u.ID] = u
	}
	cands, err := m.candidates(ctx, source)
	if err != nil {
		return nil, err
	}
	for _, u := range cands {
		byID[u.ID] = u
	}
	var out []tm.Match
	for _, u := range byID {
		if s := fuzzy.Score(source, u.Source); s >= opts.MinScore {
			out = append(out, tm.Match{Unit: u, Score: s})
		}
	}
	tm.SortMatches(out)
	if len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

// candidates returns the units sharing the most words with source. Segments
// without words or numbers (only punctuation or tags) fall back to recent
// units of similar length.
func (m *Memory) candidates(ctx context.Context, source content.Content) ([]tm.Unit, error) {
	terms := tm.SearchTerms(source)
	if len(terms) == 0 {
		n := utf8.RuneCountInString(source.Plain())
		return m.queryUnits(ctx, m.db.sql,
			`WHERE memory_id = ? AND length(source_plain) <= ? ORDER BY id DESC LIMIT ?`,
			m.id, n*2+2, candidateLimit)
	}
	args := make([]any, 0, len(terms)+2)
	args = append(args, m.id)
	for _, t := range terms {
		args = append(args, t)
	}
	args = append(args, candidateLimit)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(terms)), ",")
	out, err := m.queryUnits(ctx, m.db.sql, `
		JOIN (
			SELECT unit_id, count(*) AS shared FROM unit_terms
			WHERE memory_id = ? AND term IN (`+placeholders+`)
			GROUP BY unit_id ORDER BY shared DESC, unit_id DESC LIMIT ?
		) c ON c.unit_id = units.id`, args...)
	if err != nil {
		return nil, err
	}
	// A short segment with a word in another form shares too few words
	// with its match, or none: short units that hold the beginning of one
	// of its words are candidates too, found through the index the
	// concordance search uses.
	keys := tm.FormKeys(source)
	if len(keys) == 0 {
		return out, nil
	}
	have := map[int64]bool{}
	for _, u := range out {
		have[u.ID] = true
	}
	longest := utf8.RuneCountInString(source.Plain())*2 + 2
	for _, k := range keys {
		var more []tm.Unit
		if m.db.d == sqlstore.Postgres {
			cond, arg := sqlstore.LikeContains("source_plain", k)
			more, err = m.queryUnits(ctx, m.db.sql,
				`WHERE memory_id = ? AND `+cond+` AND char_length(source_plain) <= ? ORDER BY id DESC LIMIT ?`,
				m.id, arg, longest, formCandidates)
		} else {
			more, err = m.queryUnits(ctx, m.db.sql, `
			WHERE id IN (
				SELECT f.rowid FROM units_fts f JOIN units x ON x.id = f.rowid
				WHERE units_fts MATCH ? AND x.memory_id = ? AND length(x.source_plain) <= ?)
			ORDER BY id DESC LIMIT ?`,
				"source_plain : "+ftsQuote(k), m.id, longest, formCandidates)
		}
		if err != nil {
			return nil, err
		}
		for _, u := range more {
			if !have[u.ID] {
				have[u.ID] = true
				out = append(out, u)
			}
		}
	}
	return out, nil
}

// formCandidates bounds the short units looked at for each word of a
// short segment.
const formCandidates = 50

// Concordance implements tm.Memory.
func (m *Memory) Concordance(ctx context.Context, query string, opts tm.ConcordanceOptions) ([]tm.Unit, error) {
	q := tm.NormalizeQuery(query)
	if q == "" {
		return nil, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	column := "source_plain"
	if opts.InTarget {
		column = "target_plain"
	}
	matches := func(u tm.Unit) bool {
		if opts.InTarget {
			return tm.ContainsQuery(u.Target, q)
		}
		return tm.ContainsQuery(u.Source, q)
	}
	if utf8.RuneCountInString(q) < 3 {
		// Too short for the trigram index: scan newest first, page by page.
		const page = 1000
		var out []tm.Unit
		before := int64(1<<63 - 1)
		for {
			units, err := m.queryUnits(ctx, m.db.sql,
				`WHERE memory_id = ? AND id < ? ORDER BY id DESC LIMIT ?`, m.id, before, page)
			if err != nil {
				return nil, err
			}
			for _, u := range units {
				if matches(u) {
					out = append(out, u)
					if len(out) == limit {
						return out, nil
					}
				}
				before = u.ID
			}
			if len(units) < page {
				return out, nil
			}
		}
	}
	// The trigram index finds substrings case-insensitively. Candidates are
	// checked again with the same rules as the in-memory backend.
	var rows []tm.Unit
	var err error
	if m.db.d == sqlstore.Postgres {
		cond, arg := sqlstore.LikeContains(column, q)
		rows, err = m.queryUnits(ctx, m.db.sql, `WHERE memory_id = ? AND `+cond+` ORDER BY id DESC LIMIT ?`, m.id, arg, limit*4+20)
	} else {
		rows, err = m.queryUnits(ctx, m.db.sql, `
		WHERE id IN (
			SELECT f.rowid FROM units_fts f JOIN units x ON x.id = f.rowid
			WHERE units_fts MATCH ? AND x.memory_id = ?)
		ORDER BY id DESC LIMIT ?`,
			column+" : "+ftsQuote(q), m.id, limit*4+20)
	}
	if err != nil {
		return nil, err
	}
	var out []tm.Unit
	for _, u := range rows {
		if matches(u) {
			out = append(out, u)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

// Count implements tm.Memory.
func (m *Memory) Count(ctx context.Context) (int, error) {
	var n int
	err := m.db.sql.QueryRowContext(ctx, `SELECT count(*) FROM units WHERE memory_id = ?`, m.id).Scan(&n)
	return n, err
}

// Each implements tm.Memory. Units are read in pages, so fn may write to
// the same database.
func (m *Memory) Each(ctx context.Context, fn func(tm.Unit) error) error {
	const page = 500
	var after int64
	for {
		units, err := m.queryUnits(ctx, m.db.sql,
			`WHERE memory_id = ? AND id > ? ORDER BY id LIMIT ?`, m.id, after, page)
		if err != nil {
			return err
		}
		for _, u := range units {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(u); err != nil {
				return err
			}
			after = u.ID
		}
		if len(units) < page {
			return nil
		}
	}
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

const unitColumns = `id, source_json, target_json, created_at, created_by,
	changed_at, changed_by, usage_count, attributes_json, notes_json, version`

func (m *Memory) queryUnits(ctx context.Context, q querier, where string, args ...any) ([]tm.Unit, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+unitColumns+` FROM units `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("tmdb: query: %w", err)
	}
	return scanUnits(rows)
}

// scanUnits reads rows selected with unitColumns and closes them.
func scanUnits(rows *sql.Rows) ([]tm.Unit, error) {
	defer rows.Close()
	var out []tm.Unit
	for rows.Next() {
		var (
			u                         tm.Unit
			src, tgt, created, change string
			attrs, notes              string
		)
		if err := rows.Scan(&u.ID, &src, &tgt, &created, &u.CreatedBy,
			&change, &u.ChangedBy, &u.UsageCount, &attrs, &notes, &u.Version); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(src), &u.Source); err != nil {
			return nil, fmt.Errorf("tmdb: unit %d source: %w", u.ID, err)
		}
		if err := json.Unmarshal([]byte(tgt), &u.Target); err != nil {
			return nil, fmt.Errorf("tmdb: unit %d target: %w", u.ID, err)
		}
		if err := json.Unmarshal([]byte(attrs), &u.Attributes); err != nil {
			return nil, fmt.Errorf("tmdb: unit %d attributes: %w", u.ID, err)
		}
		if err := json.Unmarshal([]byte(notes), &u.Notes); err != nil {
			return nil, fmt.Errorf("tmdb: unit %d notes: %w", u.ID, err)
		}
		u.CreatedAt = parseTime(created)
		u.ChangedAt = parseTime(change)
		out = append(out, u)
	}
	return out, rows.Err()
}

type encoded struct {
	sourcePlain, sourceJSON string
	targetPlain, targetJSON string
	createdAt, changedAt    string
	attrs, notes            string
}

func encode(u tm.Unit) (encoded, error) {
	var e encoded
	src, err := json.Marshal(u.Source)
	if err != nil {
		return e, err
	}
	tgt, err := json.Marshal(u.Target)
	if err != nil {
		return e, err
	}
	attrs := u.Attributes
	if attrs == nil {
		attrs = []tm.Attribute{}
	}
	a, err := json.Marshal(attrs)
	if err != nil {
		return e, err
	}
	notes := u.Notes
	if notes == nil {
		notes = []string{}
	}
	n, err := json.Marshal(notes)
	if err != nil {
		return e, err
	}
	return encoded{
		// The search columns hold composed text, so a search finds a word
		// however the file it came from wrote its accents.
		sourcePlain: noNUL(textnorm.NFC(u.Source.Plain())), sourceJSON: string(src),
		targetPlain: noNUL(textnorm.NFC(u.Target.Plain())), targetJSON: string(tgt),
		createdAt: u.CreatedAt.UTC().Format(timeLayout), changedAt: u.ChangedAt.UTC().Format(timeLayout),
		attrs: string(a), notes: string(n),
	}, nil
}

// keyArg is an exact-match key as a query argument. Keys separate their
// parts with NUL, which PostgreSQL text cannot hold, so there they are
// bytes.
func (d *DB) keyArg(k string) any {
	if d.d == sqlstore.Postgres {
		return []byte(k)
	}
	return k
}

// noNUL drops NUL characters from text stored in plain columns (the
// search columns and author names), which PostgreSQL cannot hold. The
// content itself is stored as JSON, where NUL is escaped, so nothing is
// lost.
func noNUL(s string) string {
	if strings.IndexByte(s, 0) < 0 {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "")
}

func parseTime(s string) time.Time {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// ftsQuote makes a string an FTS5 string literal, which the trigram
// tokenizer matches as a substring.
func ftsQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
