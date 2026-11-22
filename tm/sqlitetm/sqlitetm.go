// Package sqlitetm stores translation memories in a SQLite database file.
//
// One database holds any number of memories. Exact matches use an index on
// the normalized source. Fuzzy candidates come from a term index (units
// sharing the most words) and are scored with the fuzzy package, so results
// agree with the in-memory backend. Concordance search uses an FTS5 trigram
// index, which finds any substring, case-insensitively. The driver is pure Go (SQLite compiled to WebAssembly),
// so no C compiler is needed on any platform.
package sqlitetm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "github.com/ncruces/go-sqlite3/driver" // registers the "sqlite3" driver
	_ "github.com/ncruces/go-sqlite3/embed"  // embeds the SQLite build (with FTS5)

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/fuzzy"
	"github.com/4nonX/interlinea/tm"
)

// Errors returned by this package.
var (
	ErrNotFound = errors.New("sqlitetm: memory not found")
	ErrExists   = errors.New("sqlitetm: a memory with this name already exists")
)

const (
	schemaVersion  = 1
	candidateLimit = 200
	timeLayout     = time.RFC3339Nano
)

// DB is an open database file. It is safe for concurrent use; writes are
// serialized inside the process, and SQLite's busy timeout handles other
// processes.
type DB struct {
	sql *sql.DB
	wmu sync.Mutex
	now func() time.Time
}

// Open opens or creates the database at path and brings its schema up to date.
// Parent directories are created as needed.
func Open(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlitetm: empty database path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	dsn := fileDSN(filepath.ToSlash(abs))
	sdb, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlitetm: open %s: %w", path, err)
	}
	d := &DB{sql: sdb, now: func() time.Time { return time.Now().UTC() }}
	if err := d.migrate(context.Background()); err != nil {
		sdb.Close()
		return nil, err
	}
	return d, nil
}

// fileDSN builds the SQLite URI for an absolute path with forward slashes
// ("/home/dan/x.db" or "C:/Users/Dan/x.db") and the connection settings.
//
// The path is written without an authority ("file:/home/..." and
// "file:C:/Users/..."). The SQLite inside the driver decodes URIs the Unix
// way, so "file:///C:/..." would become "/C:/...", which Windows rejects.
func fileDSN(slashPath string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(wal)")
	q.Add("_pragma", "synchronous(normal)")
	q.Add("_pragma", "busy_timeout(10000)")
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
}

func (d *DB) migrate(ctx context.Context) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	var current int
	if err := d.sql.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("sqlitetm: read schema version: %w", err)
	}
	if current > schemaVersion {
		return fmt.Errorf("sqlitetm: database schema version %d is newer than this program supports (%d)", current, schemaVersion)
	}
	for v := current; v < schemaVersion; v++ {
		tx, err := d.sql.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlitetm: migrate to version %d: %w", v+1, err)
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

// Summary describes a stored memory.
type Summary struct {
	tm.Info
	Units     int
	CreatedAt time.Time
}

// Create adds a new, empty memory.
func (d *DB) Create(ctx context.Context, info tm.Info) (*Memory, error) {
	info, err := tm.ValidateInfo(info)
	if err != nil {
		return nil, err
	}
	d.wmu.Lock()
	defer d.wmu.Unlock()
	res, err := d.sql.ExecContext(ctx,
		`INSERT INTO memories(name, source_lang, target_lang, created_at) VALUES (?, ?, ?, ?)`,
		info.Name, info.SourceLang, info.TargetLang, d.now().Format(timeLayout))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrExists
		}
		return nil, fmt.Errorf("sqlitetm: create memory: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Memory{db: d, id: id, info: info}, nil
}

// Memory returns the memory with the given name (case-insensitive).
func (d *DB) Memory(ctx context.Context, name string) (*Memory, error) {
	m := &Memory{db: d}
	err := d.sql.QueryRowContext(ctx,
		`SELECT id, name, source_lang, target_lang FROM memories WHERE name = ?`, strings.TrimSpace(name)).
		Scan(&m.id, &m.info.Name, &m.info.SourceLang, &m.info.TargetLang)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlitetm: load memory: %w", err)
	}
	return m, nil
}

// List returns all memories ordered by name.
func (d *DB) List(ctx context.Context) ([]Summary, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT m.name, m.source_lang, m.target_lang, m.created_at,
		       (SELECT count(*) FROM units u WHERE u.memory_id = m.id)
		FROM memories m ORDER BY m.name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("sqlitetm: list: %w", err)
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var s Summary
		var created string
		if err := rows.Scan(&s.Name, &s.SourceLang, &s.TargetLang, &created, &s.Units); err != nil {
			return nil, err
		}
		s.CreatedAt = parseTime(created)
		out = append(out, s)
	}
	return out, rows.Err()
}

// Delete removes a memory and all its units.
func (d *DB) Delete(ctx context.Context, name string) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	res, err := d.sql.ExecContext(ctx, `DELETE FROM memories WHERE name = ?`, strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("sqlitetm: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
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
	tx         *sql.Tx
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
	tx, err := m.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlitetm: begin: %w", err)
	}
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
			created_at, created_by, changed_at, changed_by, usage_count, attributes_json, notes_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	prep(&w.updateUnit, `UPDATE units SET
			source_plain = ?, source_json = ?, target_plain = ?, target_json = ?,
			changed_at = ?, changed_by = ?, usage_count = ?, attributes_json = ?, notes_json = ?
		WHERE id = ?`)
	prep(&w.insertTerm, `INSERT OR IGNORE INTO unit_terms(memory_id, term, unit_id) VALUES (?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("sqlitetm: prepare: %w", err)
	}
	if err := fn(w); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlitetm: commit: %w", err)
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
	rows, err := w.byKey.QueryContext(ctx, m.id, key)
	if err != nil {
		return tm.Unit{}, fmt.Errorf("sqlitetm: query: %w", err)
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
		row, err := encode(upd)
		if err != nil {
			return tm.Unit{}, err
		}
		_, err = w.updateUnit.ExecContext(ctx,
			row.sourcePlain, row.sourceJSON, row.targetPlain, row.targetJSON,
			row.changedAt, upd.ChangedBy, upd.UsageCount, row.attrs, row.notes, upd.ID)
		if err != nil {
			return tm.Unit{}, fmt.Errorf("sqlitetm: update unit: %w", err)
		}
		return upd, nil
	}
	row, err := encode(prepared)
	if err != nil {
		return tm.Unit{}, err
	}
	res, err := w.insertUnit.ExecContext(ctx,
		m.id, key, row.sourcePlain, row.sourceJSON, row.targetPlain, row.targetJSON,
		row.createdAt, prepared.CreatedBy, row.changedAt, prepared.ChangedBy,
		prepared.UsageCount, row.attrs, row.notes)
	if err != nil {
		return tm.Unit{}, fmt.Errorf("sqlitetm: insert unit: %w", err)
	}
	if prepared.ID, err = res.LastInsertId(); err != nil {
		return tm.Unit{}, err
	}
	for _, term := range tm.SearchTerms(prepared.Source) {
		if _, err := w.insertTerm.ExecContext(ctx, m.id, term, prepared.ID); err != nil {
			return tm.Unit{}, fmt.Errorf("sqlitetm: index unit: %w", err)
		}
	}
	return prepared, nil
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
		`WHERE memory_id = ? AND source_key = ? ORDER BY id DESC LIMIT ?`, m.id, tm.SourceKey(source), candidateLimit)
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
	return m.queryUnits(ctx, m.db.sql, `
		JOIN (
			SELECT unit_id, count(*) AS shared FROM unit_terms
			WHERE memory_id = ? AND term IN (`+placeholders+`)
			GROUP BY unit_id ORDER BY shared DESC, unit_id DESC LIMIT ?
		) c ON c.unit_id = units.id`, args...)
}

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
	rows, err := m.queryUnits(ctx, m.db.sql, `
		WHERE id IN (
			SELECT f.rowid FROM units_fts f JOIN units x ON x.id = f.rowid
			WHERE units_fts MATCH ? AND x.memory_id = ?)
		ORDER BY id DESC LIMIT ?`,
		column+" : "+ftsQuote(q), m.id, limit*4+20)
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
	changed_at, changed_by, usage_count, attributes_json, notes_json`

func (m *Memory) queryUnits(ctx context.Context, q querier, where string, args ...any) ([]tm.Unit, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+unitColumns+` FROM units `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlitetm: query: %w", err)
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
			&change, &u.ChangedBy, &u.UsageCount, &attrs, &notes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(src), &u.Source); err != nil {
			return nil, fmt.Errorf("sqlitetm: unit %d source: %w", u.ID, err)
		}
		if err := json.Unmarshal([]byte(tgt), &u.Target); err != nil {
			return nil, fmt.Errorf("sqlitetm: unit %d target: %w", u.ID, err)
		}
		if err := json.Unmarshal([]byte(attrs), &u.Attributes); err != nil {
			return nil, fmt.Errorf("sqlitetm: unit %d attributes: %w", u.ID, err)
		}
		if err := json.Unmarshal([]byte(notes), &u.Notes); err != nil {
			return nil, fmt.Errorf("sqlitetm: unit %d notes: %w", u.ID, err)
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
		sourcePlain: u.Source.Plain(), sourceJSON: string(src),
		targetPlain: u.Target.Plain(), targetJSON: string(tgt),
		createdAt: u.CreatedAt.UTC().Format(timeLayout), changedAt: u.ChangedAt.UTC().Format(timeLayout),
		attrs: string(a), notes: string(n),
	}, nil
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
