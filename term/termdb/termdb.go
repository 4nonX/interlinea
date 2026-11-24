// Package termdb stores termbases in a SQLite database file.
//
// One database holds any number of termbases. Each entry is kept as a
// whole (its concept fields, languages, terms and their fields) together
// with every earlier version, so the history of an entry can be shown and
// an edit made on a stale copy is refused instead of overwriting someone
// else's change. Terms are indexed by language and normalized text for
// lookups; an FTS5 trigram index over terms and field values serves search
// for any substring, case-insensitively.
package termdb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/4nonX/interlinea/internal/sqlitedb"
	"github.com/4nonX/interlinea/internal/sqlstore"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/term"
)

// Errors returned by this package.
var (
	ErrNotFound = errors.New("termdb: not found")
	ErrExists   = errors.New("termdb: a termbase with this name already exists")
	// ErrConflict means the entry changed since the version the caller
	// edited; load it again and reapply the change.
	ErrConflict = errors.New("termdb: the entry was changed by someone else")
	// ErrNoTerms means an entry has no terms left after trimming.
	ErrNoTerms = errors.New("termdb: the entry has no terms")
	// ErrInUse means a language or field is still used by entries; remove
	// it with RemoveLanguage or RemoveField instead.
	ErrInUse = errors.New("termdb: still used by entries")
)

const timeLayout = time.RFC3339Nano

// Kinds of termbase.
const (
	kindMaster  = "master"
	kindProject = "project"
)

// Kinds of change in the change log.
const (
	ChangeCreated         = "created"
	ChangeChanged         = "changed"
	ChangeDeleted         = "deleted"
	ChangeTermbaseCreated = "termbase-created"
	ChangeTermbaseChanged = "termbase-changed"
	ChangeTermbaseDeleted = "termbase-deleted"
)

// execer runs statements inside a transaction or on the database.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// logChange appends a change to the change log in the same transaction
// as the change itself, so the log never misses or invents one. An empty
// tbName is the termbase's current name.
func logChange(ctx context.Context, tx execer, tbID int64, tbName, entryID string, version int, kind, by, summary string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO changes(termbase_id, termbase, entry_id, version, kind, at, author, summary)
		VALUES (?, COALESCE(NULLIF(?, ''), (SELECT name FROM termbases WHERE id = ?), ''), ?, ?, ?, ?, ?, ?)`,
		tbID, tbName, tbID, entryID, version, kind, now.Format(timeLayout), by, summary)
	if err != nil {
		return fmt.Errorf("termdb: change log: %w", err)
	}
	return nil
}

var migrations = []string{
	// Version 1: termbases, entries with their versions, a term index for
	// lookups and an FTS5 trigram index for search.
	`CREATE TABLE termbases (
		id              INTEGER PRIMARY KEY,
		name            TEXT NOT NULL UNIQUE COLLATE NOCASE,
		definition_json TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		next_entry      INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE entries (
		id          INTEGER PRIMARY KEY,
		termbase_id INTEGER NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		entry_id    TEXT NOT NULL,
		version     INTEGER NOT NULL,
		entry_json  TEXT NOT NULL,
		created_at  TEXT NOT NULL,
		created_by  TEXT NOT NULL DEFAULT '',
		changed_at  TEXT NOT NULL,
		changed_by  TEXT NOT NULL DEFAULT '',
		UNIQUE (termbase_id, entry_id)
	);
	CREATE TABLE terms (
		entry_row    INTEGER NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		termbase_id  INTEGER NOT NULL,
		lang         TEXT NOT NULL,
		primary_lang TEXT NOT NULL,
		key          TEXT NOT NULL,
		text         TEXT NOT NULL,
		status       TEXT NOT NULL
	);
	CREATE INDEX terms_by_key ON terms(termbase_id, primary_lang, key);
	CREATE INDEX terms_by_entry ON terms(entry_row);
	CREATE TABLE versions (
		entry_row  INTEGER NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		version    INTEGER NOT NULL,
		at         TEXT NOT NULL,
		by         TEXT NOT NULL,
		summary    TEXT NOT NULL,
		entry_json TEXT NOT NULL,
		PRIMARY KEY (entry_row, version)
	) WITHOUT ROWID;
	CREATE TABLE deleted (
		termbase_id INTEGER NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		entry_id    TEXT NOT NULL,
		at          TEXT NOT NULL,
		by          TEXT NOT NULL,
		summary     TEXT NOT NULL,
		entry_json  TEXT NOT NULL
	);
	CREATE INDEX deleted_by_termbase ON deleted(termbase_id, at);
	CREATE VIRTUAL TABLE entries_fts USING fts5(body, tokenize='trigram');`,

	// Version 2: the change log every client syncs from, project
	// termbases layered over masters, and merge requests with a review
	// state per entry.
	`ALTER TABLE termbases ADD COLUMN kind TEXT NOT NULL DEFAULT 'master';
	ALTER TABLE termbases ADD COLUMN upstreams_json TEXT NOT NULL DEFAULT '[]';
	ALTER TABLE termbases ADD COLUMN rules_json TEXT NOT NULL DEFAULT '{}';
	ALTER TABLE entries ADD COLUMN base_termbase INTEGER;
	ALTER TABLE entries ADD COLUMN base_entry TEXT;
	ALTER TABLE entries ADD COLUMN base_version INTEGER;
	ALTER TABLE entries ADD COLUMN base_synced INTEGER;
	CREATE TABLE changes (
		seq         INTEGER PRIMARY KEY AUTOINCREMENT,
		termbase_id INTEGER NOT NULL,
		termbase    TEXT NOT NULL,
		entry_id    TEXT NOT NULL DEFAULT '',
		version     INTEGER NOT NULL DEFAULT 0,
		kind        TEXT NOT NULL,
		at          TEXT NOT NULL,
		by          TEXT NOT NULL,
		summary     TEXT NOT NULL
	);
	CREATE INDEX changes_by_termbase ON changes(termbase_id, seq);
	INSERT INTO changes(termbase_id, termbase, entry_id, version, kind, at, by, summary)
		SELECT termbase_id, termbase, entry_id, version, kind, at, by, summary FROM (
			SELECT e.termbase_id, t.name AS termbase, e.entry_id, v.version, CASE WHEN v.version = 1 THEN 'created' ELSE 'changed' END AS kind, v.at, v.by, v.summary
			FROM versions v JOIN entries e ON e.id = v.entry_row JOIN termbases t ON t.id = e.termbase_id
			UNION ALL
			SELECT d.termbase_id, t.name, d.entry_id, 0, 'deleted', d.at, d.by, d.summary
			FROM deleted d JOIN termbases t ON t.id = d.termbase_id
		) ORDER BY at;
	CREATE TABLE project_removals (
		project_id    INTEGER NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		base_termbase INTEGER NOT NULL,
		base_entry    TEXT NOT NULL,
		base_version  INTEGER NOT NULL,
		at            TEXT NOT NULL,
		by            TEXT NOT NULL,
		PRIMARY KEY (project_id, base_termbase, base_entry)
	);
	CREATE TABLE merge_requests (
		id         INTEGER PRIMARY KEY,
		project_id INTEGER NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		target_id  INTEGER NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		title      TEXT NOT NULL,
		created_at TEXT NOT NULL,
		created_by TEXT NOT NULL,
		deadline   TEXT NOT NULL DEFAULT '',
		state      TEXT NOT NULL DEFAULT 'open',
		closed_at  TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE merge_items (
		id              INTEGER PRIMARY KEY,
		request_id      INTEGER NOT NULL REFERENCES merge_requests(id) ON DELETE CASCADE,
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
		request_id  INTEGER NOT NULL REFERENCES merge_requests(id) ON DELETE CASCADE,
		reviewer    TEXT NOT NULL,
		filter_json TEXT NOT NULL,
		PRIMARY KEY (request_id, reviewer)
	);`,

	// Version 3: the same schema as on PostgreSQL. "by" is a reserved
	// word there, so the columns are called author; a project's masters
	// move from a JSON list to a table.
	`ALTER TABLE versions RENAME COLUMN by TO author;
	ALTER TABLE deleted RENAME COLUMN by TO author;
	ALTER TABLE changes RENAME COLUMN by TO author;
	ALTER TABLE project_removals RENAME COLUMN by TO author;
	CREATE TABLE termbase_upstreams (
		project_id INTEGER NOT NULL REFERENCES termbases(id) ON DELETE CASCADE,
		master_id  INTEGER NOT NULL REFERENCES termbases(id),
		position   INTEGER NOT NULL,
		PRIMARY KEY (project_id, master_id)
	);
	INSERT INTO termbase_upstreams(project_id, master_id, position)
		SELECT t.id, u.value, u.key FROM termbases t, json_each(t.upstreams_json) u;
	ALTER TABLE termbases DROP COLUMN upstreams_json;`,
}

// DB is an open database file. It is safe for concurrent use; writes are
// serialized inside the process, and SQLite's busy timeout handles other
// processes.
type DB struct {
	sql *wdb
	d   sqlstore.Dialect
	wmu sync.Mutex
	now func() time.Time
}

// Open opens or creates the database at path and brings its schema up to
// date. Parent directories are created as needed.
func Open(path string) (*DB, error) {
	sdb, err := sqlitedb.Open(path, "termdb", migrations)
	if err != nil {
		return nil, err
	}
	return newDB(sdb, sqliteDialect), nil
}

func newDB(sdb *sql.DB, d sqlstore.Dialect) *DB {
	return &DB{sql: &wdb{DB: sdb, Dialect: d, LockKey: writeLock}, d: d, now: func() time.Time { return time.Now().UTC() }}
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

// Summary describes a stored termbase.
type Summary struct {
	term.Definition
	Entries   int       `json:"entries"`
	Terms     int       `json:"terms"`
	CreatedAt time.Time `json:"createdAt"`
}

// Termbase is a handle on one stored termbase.
type Termbase struct {
	db   *DB
	id   int64
	name string
}

// Stored is an entry with the version it has in the database.
type Stored struct {
	term.Entry
	Version int `json:"version"`
}

// Version is one state of an entry in its history.
type Version struct {
	Version int        `json:"version"`
	At      time.Time  `json:"at"`
	By      string     `json:"by"`
	Summary string     `json:"summary"`
	Entry   term.Entry `json:"entry"`
}

// Deletion records a deleted entry.
type Deletion struct {
	EntryID string     `json:"entryId"`
	At      time.Time  `json:"at"`
	By      string     `json:"by"`
	Summary string     `json:"summary"`
	Entry   term.Entry `json:"entry"`
}

// cleanDefinition normalizes language tags and drops duplicate languages
// and fields (by name, case-insensitively).
func cleanDefinition(def term.Definition) (term.Definition, error) {
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return def, errors.New("termdb: a termbase needs a name")
	}
	var langs []string
	seen := map[string]bool{}
	for _, l := range def.Languages {
		n := term.NormalizeLang(l)
		if n == "" || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		langs = append(langs, n)
	}
	def.Languages = langs
	var fields []term.Field
	names := map[string]bool{}
	for _, f := range def.Fields {
		f.Name = strings.TrimSpace(f.Name)
		k := strings.ToLower(f.Name)
		if f.Name == "" || names[k] {
			continue
		}
		names[k] = true
		switch f.Level {
		case term.LevelEntry, term.LevelLanguage, term.LevelTerm:
		default:
			return def, fmt.Errorf("termdb: field %q has no valid level", f.Name)
		}
		if f.Kind == "" {
			f.Kind = term.KindText
		}
		if f.Kind != term.KindPicklist {
			f.Values = nil
		}
		fields = append(fields, f)
	}
	def.Fields = fields
	return def, nil
}

// Create adds a new, empty master termbase.
func (d *DB) Create(ctx context.Context, def term.Definition) (*Termbase, error) {
	return d.CreateBy(ctx, def, "")
}

// CreateBy is Create, recording who created the termbase.
func (d *DB) CreateBy(ctx context.Context, def term.Definition, by string) (*Termbase, error) {
	def, err := cleanDefinition(def)
	if err != nil {
		return nil, err
	}
	return d.create(ctx, def, kindMaster, nil, by)
}

// create inserts a termbase of the kind, with its upstreams (projects).
func (d *DB) create(ctx context.Context, def term.Definition, kind string, upstreams []int64, by string) (*Termbase, error) {
	js, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	d.wmu.Lock()
	defer d.wmu.Unlock()
	tx, err := d.sql.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	now := d.now()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM termbases WHERE lower(name) = lower(?)`, def.Name).Scan(&exists); err != nil {
		return nil, err
	}
	if exists > 0 {
		return nil, ErrExists
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO termbases(name, definition_json, created_at, kind) VALUES (?, ?, ?, ?) RETURNING id`,
		def.Name, string(js), now.Format(timeLayout), kind).Scan(&id)
	if err != nil {
		if d.d.IsUnique(err) {
			return nil, ErrExists
		}
		return nil, fmt.Errorf("termdb: create termbase: %w", err)
	}
	if err := setUpstreams(ctx, tx, id, upstreams); err != nil {
		return nil, err
	}
	if err := logChange(ctx, tx, id, def.Name, "", 0, ChangeTermbaseCreated, by, "Created the termbase", now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Termbase{db: d, id: id, name: def.Name}, nil
}

// Termbase returns the termbase with the given name (case-insensitive).
func (d *DB) Termbase(ctx context.Context, name string) (*Termbase, error) {
	t := &Termbase{db: d}
	err := d.sql.QueryRowContext(ctx, `SELECT id, name FROM termbases WHERE lower(name) = lower(?)`, strings.TrimSpace(name)).Scan(&t.id, &t.name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// List returns all termbases by name.
func (d *DB) List(ctx context.Context) ([]Summary, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT t.definition_json, t.created_at,
		       (SELECT COUNT(*) FROM entries e WHERE e.termbase_id = t.id),
		       (SELECT COUNT(*) FROM terms x WHERE x.termbase_id = t.id)
		FROM termbases t ORDER BY lower(t.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var js, created string
		var s Summary
		if err := rows.Scan(&js, &created, &s.Entries, &s.Terms); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(js), &s.Definition); err != nil {
			return nil, fmt.Errorf("termdb: read definition: %w", err)
		}
		s.CreatedAt, _ = time.Parse(timeLayout, created)
		out = append(out, s)
	}
	return out, rows.Err()
}

// Delete removes a termbase with all its entries and their history. A
// master that project termbases are layered over cannot be deleted.
func (d *DB) Delete(ctx context.Context, name string) error {
	return d.DeleteBy(ctx, name, "")
}

// DeleteBy is Delete, recording who deleted the termbase.
func (d *DB) DeleteBy(ctx context.Context, name, by string) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	tx, err := d.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM termbases WHERE lower(name) = lower(?)`, strings.TrimSpace(name)).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var dependents int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM termbase_upstreams WHERE master_id = ?`, id).Scan(&dependents); err != nil {
		return err
	}
	if dependents > 0 {
		return fmt.Errorf("%w: %d project termbases are layered over it", ErrInUse, dependents)
	}
	if err := dropSearch(ctx, tx, `termbase_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM termbases WHERE id = ?`, id); err != nil {
		return err
	}
	if err := logChange(ctx, tx, id, strings.TrimSpace(name), "", 0, ChangeTermbaseDeleted, by, "Deleted the termbase", d.now()); err != nil {
		return err
	}
	return tx.Commit()
}

// Name is the termbase's name when the handle was opened.
func (t *Termbase) Name() string { return t.name }

// Definition returns the termbase's name, languages and fields.
func (t *Termbase) Definition(ctx context.Context) (term.Definition, error) {
	return t.definition(ctx, t.db.sql)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (t *Termbase) definition(ctx context.Context, q querier) (term.Definition, error) {
	var js string
	err := q.QueryRowContext(ctx, `SELECT definition_json FROM termbases WHERE id = ?`, t.id).Scan(&js)
	if errors.Is(err, sql.ErrNoRows) {
		return term.Definition{}, ErrNotFound
	}
	if err != nil {
		return term.Definition{}, err
	}
	var def term.Definition
	if err := json.Unmarshal([]byte(js), &def); err != nil {
		return def, fmt.Errorf("termdb: read definition: %w", err)
	}
	return def, nil
}

func writeDefinition(ctx context.Context, tx *wtx, id int64, def term.Definition) error {
	js, err := json.Marshal(def)
	if err != nil {
		return err
	}
	var clash int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM termbases WHERE lower(name) = lower(?) AND id <> ?`, def.Name, id).Scan(&clash); err != nil {
		return err
	}
	if clash > 0 {
		return ErrExists
	}
	_, err = tx.ExecContext(ctx, `UPDATE termbases SET definition_json = ?, name = ? WHERE id = ?`, string(js), def.Name, id)
	if tx.Dialect.IsUnique(err) {
		return ErrExists
	}
	return err
}

// Rename gives the termbase a new name.
func (t *Termbase) Rename(ctx context.Context, name string) error {
	return t.RenameBy(ctx, name, "")
}

// RenameBy is Rename, recording who renamed the termbase.
func (t *Termbase) RenameBy(ctx context.Context, name, by string) error {
	return t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		def.Name = strings.TrimSpace(name)
		if def.Name == "" {
			return errors.New("termdb: a termbase needs a name")
		}
		t.name = def.Name
		return nil
	})
}

// SetDefinition replaces the termbase's languages and fields (its name is
// changed with Rename). A language or field that entries still use cannot
// be dropped this way: that is what RemoveLanguage and RemoveField do, as
// they change the entries too.
func (t *Termbase) SetDefinition(ctx context.Context, def term.Definition) error {
	return t.SetDefinitionBy(ctx, def, "")
}

// SetDefinitionBy is SetDefinition, recording who changed it.
func (t *Termbase) SetDefinitionBy(ctx context.Context, def term.Definition, by string) error {
	return t.write(ctx, by, func(tx *wtx, cur *term.Definition) error {
		def.Name = cur.Name
		clean, err := cleanDefinition(def)
		if err != nil {
			return err
		}
		used, err := t.usage(ctx, tx)
		if err != nil {
			return err
		}
		for l := range used.langs {
			if !containsFold(clean.Languages, l) {
				return fmt.Errorf("%w: language %s", ErrInUse, l)
			}
		}
		for f := range used.fields {
			if !hasField(clean.Fields, f) {
				return fmt.Errorf("%w: field %q", ErrInUse, f)
			}
		}
		*cur = clean
		return nil
	})
}

func containsFold(xs []string, x string) bool {
	for _, y := range xs {
		if strings.EqualFold(x, y) {
			return true
		}
	}
	return false
}

func hasField(fs []term.Field, name string) bool {
	for _, f := range fs {
		if strings.EqualFold(f.Name, name) {
			return true
		}
	}
	return false
}

type usage struct {
	langs  map[string]bool
	fields map[string]bool
}

func (t *Termbase) usage(ctx context.Context, q querier) (usage, error) {
	u := usage{langs: map[string]bool{}, fields: map[string]bool{}}
	entries, err := t.entries(ctx, q, "")
	if err != nil {
		return u, err
	}
	for _, e := range entries {
		for _, v := range e.Fields {
			u.fields[v.Field] = true
		}
		for _, l := range e.Languages {
			u.langs[l.Lang] = true
			for _, v := range l.Fields {
				u.fields[v.Field] = true
			}
			for _, tm := range l.Terms {
				for _, v := range tm.Fields {
					u.fields[v.Field] = true
				}
			}
		}
	}
	return u, nil
}

// write runs fn on the definition inside a write transaction and stores
// the result; by is who made a change to the definition.
func (t *Termbase) write(ctx context.Context, by string, fn func(tx *wtx, def *term.Definition) error) error {
	t.db.wmu.Lock()
	defer t.db.wmu.Unlock()
	tx, err := t.db.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	def, err := t.definition(ctx, tx)
	if err != nil {
		return err
	}
	before, _ := json.Marshal(def)
	if err := fn(tx, &def); err != nil {
		return err
	}
	if after, _ := json.Marshal(def); !bytes.Equal(before, after) {
		if err := writeDefinition(ctx, tx, t.id, def); err != nil {
			return err
		}
		if err := logChange(ctx, tx, t.id, def.Name, "", 0, ChangeTermbaseChanged, by, "Changed the termbase's languages, fields or name", t.db.now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveLanguage removes a language from the termbase and from every entry,
// recording the change in each entry's history. Entries left without terms
// are deleted. It returns how many entries changed.
func (t *Termbase) RemoveLanguage(ctx context.Context, lang, by string) (int, error) {
	lang = term.NormalizeLang(lang)
	changed := 0
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		var keep []string
		for _, l := range def.Languages {
			if !strings.EqualFold(l, lang) {
				keep = append(keep, l)
			}
		}
		def.Languages = keep
		entries, err := t.entries(ctx, tx, "")
		if err != nil {
			return err
		}
		now := t.db.now()
		for _, s := range entries {
			e := s.Entry
			var langs []term.Language
			for _, l := range e.Languages {
				if !strings.EqualFold(l.Lang, lang) {
					langs = append(langs, l)
				}
			}
			if len(langs) == len(e.Languages) {
				continue
			}
			changed++
			e.Languages = langs
			summary := "Removed the language " + lang
			if e.TermCount() == 0 {
				if err := t.deleteEntry(ctx, tx, s, by, summary+"; no terms were left", now); err != nil {
					return err
				}
				continue
			}
			if _, err := t.update(ctx, tx, e, s.Version, by, summary, now); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

// RemoveField removes a field from the termbase and its values from every
// entry, recording the change in each entry's history. It returns how many
// entries changed.
func (t *Termbase) RemoveField(ctx context.Context, name, by string) (int, error) {
	changed := 0
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		var keep []term.Field
		for _, f := range def.Fields {
			if !strings.EqualFold(f.Name, name) {
				keep = append(keep, f)
			}
		}
		def.Fields = keep
		entries, err := t.entries(ctx, tx, "")
		if err != nil {
			return err
		}
		now := t.db.now()
		drop := func(vs []term.Value) ([]term.Value, bool) {
			var out []term.Value
			for _, v := range vs {
				if !strings.EqualFold(v.Field, name) {
					out = append(out, v)
				}
			}
			return out, len(out) != len(vs)
		}
		for _, s := range entries {
			e := s.Entry
			var hit, c bool
			e.Fields, c = drop(e.Fields)
			hit = hit || c
			for i := range e.Languages {
				e.Languages[i].Fields, c = drop(e.Languages[i].Fields)
				hit = hit || c
				for j := range e.Languages[i].Terms {
					e.Languages[i].Terms[j].Fields, c = drop(e.Languages[i].Terms[j].Fields)
					hit = hit || c
				}
			}
			if !hit {
				continue
			}
			changed++
			if _, err := t.update(ctx, tx, e, s.Version, by, fmt.Sprintf("Removed the field %q", name), now); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

// RenameField renames a field in the termbase and in every entry that has
// a value for it, recording the change in each entry's history. It returns
// how many entries changed.
func (t *Termbase) RenameField(ctx context.Context, name, to, by string) (int, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return 0, errors.New("termdb: the field needs a name")
	}
	changed := 0
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		at := -1
		for i, f := range def.Fields {
			if strings.EqualFold(f.Name, name) {
				at = i
			} else if strings.EqualFold(f.Name, to) {
				return fmt.Errorf("termdb: the termbase already has a field %q", f.Name)
			}
		}
		if at < 0 {
			return fmt.Errorf("termdb: the termbase has no field %q", name)
		}
		if def.Fields[at].Name == to {
			return nil
		}
		def.Fields[at].Name = to
		entries, err := t.entries(ctx, tx, "")
		if err != nil {
			return err
		}
		now := t.db.now()
		rename := func(vs []term.Value) bool {
			hit := false
			for i := range vs {
				if strings.EqualFold(vs[i].Field, name) {
					vs[i].Field = to
					hit = true
				}
			}
			return hit
		}
		for _, s := range entries {
			e := s.Entry
			hit := rename(e.Fields)
			for i := range e.Languages {
				hit = rename(e.Languages[i].Fields) || hit
				for j := range e.Languages[i].Terms {
					hit = rename(e.Languages[i].Terms[j].Fields) || hit
				}
			}
			if !hit {
				continue
			}
			changed++
			if _, err := t.update(ctx, tx, e, s.Version, by, fmt.Sprintf("Renamed the field %q to %q", name, to), now); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

// Count returns the number of entries and terms.
func (t *Termbase) Count(ctx context.Context) (entries, terms int, err error) {
	err = t.db.sql.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM entries WHERE termbase_id = ?), (SELECT COUNT(*) FROM terms WHERE termbase_id = ?)`, t.id, t.id).Scan(&entries, &terms)
	return entries, terms, err
}

// prepare cleans an entry and resolves its languages to the termbase's
// (an entry in "de" goes to the termbase's "de-DE").
func prepare(e term.Entry, def term.Definition) (term.Entry, error) {
	e = cloneEntry(e)
	for i := range e.Languages {
		e.Languages[i].Lang = term.ResolveLang(e.Languages[i].Lang, def.Languages)
	}
	if !e.Clean() {
		return e, ErrNoTerms
	}
	e.Languages = mergeSameLang(e.Languages)
	return e, nil
}

// mergeSameLang joins sections that resolved to the same language.
func mergeSameLang(ls []term.Language) []term.Language {
	var out []term.Language
	at := map[string]int{}
	for _, l := range ls {
		k := strings.ToLower(l.Lang)
		if i, ok := at[k]; ok {
			out[i].Fields = append(out[i].Fields, l.Fields...)
			seen := map[string]bool{}
			for _, t := range out[i].Terms {
				seen[term.Key(t.Text)] = true
			}
			for _, t := range l.Terms {
				if !seen[term.Key(t.Text)] {
					out[i].Terms = append(out[i].Terms, t)
				}
			}
			continue
		}
		at[k] = len(out)
		out = append(out, l)
	}
	return out
}

func cloneEntry(e term.Entry) term.Entry {
	b, _ := json.Marshal(e)
	var c term.Entry
	_ = json.Unmarshal(b, &c)
	return c
}

// addDefinitionFor adds the entry's languages and fields that the
// definition lacks. It reports whether the definition changed.
func addDefinitionFor(def *term.Definition, e term.Entry) bool {
	changed := false
	addField := func(name string, level term.Level) {
		if !hasField(def.Fields, name) {
			def.Fields = append(def.Fields, term.Field{Name: name, Level: level, Kind: term.KindText})
			changed = true
		}
	}
	for _, v := range e.Fields {
		addField(v.Field, term.LevelEntry)
	}
	for _, l := range e.Languages {
		if !containsFold(def.Languages, l.Lang) {
			def.Languages = append(def.Languages, l.Lang)
			changed = true
		}
		for _, v := range l.Fields {
			addField(v.Field, term.LevelLanguage)
		}
		for _, tm := range l.Terms {
			for _, v := range tm.Fields {
				addField(v.Field, term.LevelTerm)
			}
		}
	}
	return changed
}

// Add stores a new entry. An empty or already used ID is replaced by the
// next free one (c1, c2, ...). Languages and fields the termbase lacks are added to
// its definition.
func (t *Termbase) Add(ctx context.Context, e term.Entry, by, summary string) (Stored, error) {
	var out Stored
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		p, err := prepare(e, *def)
		if err != nil {
			return err
		}
		addDefinitionFor(def, p)
		out, err = t.insert(ctx, tx, p, by, summary, t.db.now())
		return err
	})
	return out, err
}

func (t *Termbase) freeID(ctx context.Context, tx *wtx, want string) (string, error) {
	want = strings.TrimSpace(want)
	if want != "" {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE termbase_id = ? AND entry_id = ?`, t.id, want).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return want, nil
		}
	}
	var next int64
	if err := tx.QueryRowContext(ctx, `SELECT next_entry FROM termbases WHERE id = ?`, t.id).Scan(&next); err != nil {
		return "", err
	}
	for ; ; next++ {
		// "c1", "c2": valid XML IDs, so they survive a TBX export.
		id := "c" + strconv.FormatInt(next, 10)
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE termbase_id = ? AND entry_id = ?`, t.id, id).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE termbases SET next_entry = ? WHERE id = ?`, next+1, t.id); err != nil {
				return "", err
			}
			return id, nil
		}
	}
}

func (t *Termbase) insert(ctx context.Context, tx *wtx, e term.Entry, by, summary string, now time.Time) (Stored, error) {
	id, err := t.freeID(ctx, tx, e.ID)
	if err != nil {
		return Stored{}, err
	}
	e.ID = id
	if e.CreatedAt.IsZero() {
		e.CreatedAt, e.CreatedBy = now, by
	}
	e.ChangedAt, e.ChangedBy = now, by
	js, err := json.Marshal(e)
	if err != nil {
		return Stored{}, err
	}
	var row int64
	err = tx.QueryRowContext(ctx, `INSERT INTO entries(termbase_id, entry_id, version, entry_json, created_at, created_by, changed_at, changed_by) VALUES (?, ?, 1, ?, ?, ?, ?, ?) RETURNING id`,
		t.id, e.ID, string(js), e.CreatedAt.Format(timeLayout), e.CreatedBy, now.Format(timeLayout), by).Scan(&row)
	if err != nil {
		return Stored{}, fmt.Errorf("termdb: add entry: %w", err)
	}
	if err := t.index(ctx, tx, row, e); err != nil {
		return Stored{}, err
	}
	if summary == "" {
		summary = "Created"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO versions(entry_row, version, at, author, summary, entry_json) VALUES (?, 1, ?, ?, ?, ?)`, row, now.Format(timeLayout), by, summary, string(js)); err != nil {
		return Stored{}, err
	}
	if err := logChange(ctx, tx, t.id, "", e.ID, 1, ChangeCreated, by, summary, now); err != nil {
		return Stored{}, err
	}
	return Stored{Entry: e, Version: 1}, nil
}

// body is the text the search index holds for an entry.
func body(e term.Entry) string {
	var b strings.Builder
	add := func(s string) {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	for _, v := range e.Fields {
		add(v.Value)
	}
	for _, l := range e.Languages {
		for _, v := range l.Fields {
			add(v.Value)
		}
		for _, tm := range l.Terms {
			add(tm.Text)
			for _, v := range tm.Fields {
				add(v.Value)
			}
		}
	}
	return b.String()
}

func (t *Termbase) index(ctx context.Context, tx *wtx, row int64, e term.Entry) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM terms WHERE entry_row = ?`, row); err != nil {
		return err
	}
	for _, l := range e.Languages {
		for _, tm := range l.Terms {
			if _, err := tx.ExecContext(ctx, `INSERT INTO terms(entry_row, termbase_id, lang, primary_lang, key, text, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				row, t.id, l.Lang, langtag.Primary(l.Lang), term.Key(tm.Text), tm.Text, tm.Status); err != nil {
				return err
			}
		}
	}
	return setSearch(ctx, tx, row, body(e))
}

// Get returns an entry by its ID.
func (t *Termbase) Get(ctx context.Context, id string) (Stored, error) {
	s, _, err := t.get(ctx, t.db.sql, id)
	return s, err
}

func (t *Termbase) get(ctx context.Context, q querier, id string) (Stored, int64, error) {
	var js string
	var s Stored
	var row int64
	err := q.QueryRowContext(ctx, `SELECT id, version, entry_json FROM entries WHERE termbase_id = ? AND entry_id = ?`, t.id, strings.TrimSpace(id)).Scan(&row, &s.Version, &js)
	if errors.Is(err, sql.ErrNoRows) {
		return s, 0, ErrNotFound
	}
	if err != nil {
		return s, 0, err
	}
	if err := json.Unmarshal([]byte(js), &s.Entry); err != nil {
		return s, 0, fmt.Errorf("termdb: read entry %s: %w", id, err)
	}
	return s, row, nil
}

// Update replaces an entry. version is the version the caller edited; if
// the entry changed since, ErrConflict is returned and nothing is written.
func (t *Termbase) Update(ctx context.Context, e term.Entry, version int, by, summary string) (Stored, error) {
	var out Stored
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		p, err := prepare(e, *def)
		if err != nil {
			return err
		}
		addDefinitionFor(def, p)
		out, err = t.update(ctx, tx, p, version, by, summary, t.db.now())
		return err
	})
	return out, err
}

func (t *Termbase) update(ctx context.Context, tx *wtx, e term.Entry, version int, by, summary string, now time.Time) (Stored, error) {
	cur, row, err := t.get(ctx, tx, e.ID)
	if err != nil {
		return Stored{}, err
	}
	if cur.Version != version {
		return Stored{}, ErrConflict
	}
	e.CreatedAt, e.CreatedBy = cur.CreatedAt, cur.CreatedBy
	e.ChangedAt, e.ChangedBy = now, by
	js, err := json.Marshal(e)
	if err != nil {
		return Stored{}, err
	}
	next := version + 1
	// The version check is part of the update itself, so two writers
	// that both read version n cannot both write n+1.
	res, err := tx.ExecContext(ctx, `UPDATE entries SET version = ?, entry_json = ?, changed_at = ?, changed_by = ? WHERE id = ? AND version = ?`, next, string(js), now.Format(timeLayout), by, row, version)
	if err != nil {
		return Stored{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return Stored{}, err
	} else if n != 1 {
		return Stored{}, ErrConflict
	}
	if err := t.index(ctx, tx, row, e); err != nil {
		return Stored{}, err
	}
	if summary == "" {
		summary = "Changed"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO versions(entry_row, version, at, author, summary, entry_json) VALUES (?, ?, ?, ?, ?, ?)`, row, next, now.Format(timeLayout), by, summary, string(js)); err != nil {
		return Stored{}, err
	}
	if err := logChange(ctx, tx, t.id, "", e.ID, next, ChangeChanged, by, summary, now); err != nil {
		return Stored{}, err
	}
	return Stored{Entry: e, Version: next}, nil
}

// DeleteEntry deletes an entry. The deletion and the entry's last state
// are kept (see Deleted).
func (t *Termbase) DeleteEntry(ctx context.Context, id, by, summary string) error {
	return t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		s, _, err := t.get(ctx, tx, id)
		if err != nil {
			return err
		}
		return t.deleteEntry(ctx, tx, s, by, summary, t.db.now())
	})
}

func (t *Termbase) deleteEntry(ctx context.Context, tx *wtx, s Stored, by, summary string, now time.Time) error {
	js, err := json.Marshal(s.Entry)
	if err != nil {
		return err
	}
	if summary == "" {
		summary = "Deleted"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO deleted(termbase_id, entry_id, at, author, summary, entry_json) VALUES (?, ?, ?, ?, ?, ?)`, t.id, s.ID, now.Format(timeLayout), by, summary, string(js)); err != nil {
		return err
	}
	if err := dropSearch(ctx, tx, `termbase_id = ? AND entry_id = ?`, t.id, s.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE termbase_id = ? AND entry_id = ?`, t.id, s.ID); err != nil {
		return err
	}
	return logChange(ctx, tx, t.id, "", s.ID, s.Version, ChangeDeleted, by, summary, now)
}

// Deleted lists deleted entries, most recent first.
func (t *Termbase) Deleted(ctx context.Context) ([]Deletion, error) {
	rows, err := t.db.sql.QueryContext(ctx, `SELECT entry_id, at, author, summary, entry_json FROM deleted WHERE termbase_id = ? ORDER BY at DESC, `+deletedOrder(t.db.d)+` DESC`, t.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deletion
	for rows.Next() {
		var d Deletion
		var at, js string
		if err := rows.Scan(&d.EntryID, &at, &d.By, &d.Summary, &js); err != nil {
			return nil, err
		}
		d.At, _ = time.Parse(timeLayout, at)
		if err := json.Unmarshal([]byte(js), &d.Entry); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Restore brings a deleted entry back as it was when it was deleted, under
// its old ID if that is still free, and takes it off the list of deleted
// entries. Its versions went with the deletion; the entry starts a new
// history that says where it came from.
func (t *Termbase) Restore(ctx context.Context, entryID, by string) (Stored, error) {
	var out Stored
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		var js string
		err := tx.QueryRowContext(ctx, `SELECT entry_json FROM deleted WHERE termbase_id = ? AND entry_id = ? ORDER BY at DESC, `+deletedOrder(t.db.d)+` DESC LIMIT 1`, t.id, entryID).Scan(&js)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var e term.Entry
		if err := json.Unmarshal([]byte(js), &e); err != nil {
			return err
		}
		e.ID = entryID
		p, err := prepare(e, *def)
		if err != nil {
			return err
		}
		addDefinitionFor(def, p)
		if out, err = t.insert(ctx, tx, p, by, "Restored after it was deleted", t.db.now()); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM deleted WHERE termbase_id = ? AND entry_id = ?`, t.id, entryID)
		return err
	})
	return out, err
}

// History returns every version of an entry, oldest first.
func (t *Termbase) History(ctx context.Context, id string) ([]Version, error) {
	_, row, err := t.get(ctx, t.db.sql, id)
	if err != nil {
		return nil, err
	}
	rows, err := t.db.sql.QueryContext(ctx, `SELECT version, at, author, summary, entry_json FROM versions WHERE entry_row = ? ORDER BY version`, row)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		var at, js string
		if err := rows.Scan(&v.Version, &at, &v.By, &v.Summary, &js); err != nil {
			return nil, err
		}
		v.At, _ = time.Parse(timeLayout, at)
		if err := json.Unmarshal([]byte(js), &v.Entry); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Entries returns all entries in the order they were added.
func (t *Termbase) Entries(ctx context.Context) ([]Stored, error) {
	return t.entries(ctx, t.db.sql, "")
}

func (t *Termbase) entries(ctx context.Context, q querier, where string, args ...any) ([]Stored, error) {
	query := `SELECT version, entry_json FROM entries WHERE termbase_id = ?`
	if where != "" {
		query += " AND " + where
	}
	query += " ORDER BY id"
	rows, err := q.QueryContext(ctx, query, append([]any{t.id}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Stored
	for rows.Next() {
		var s Stored
		var js string
		if err := rows.Scan(&s.Version, &js); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(js), &s.Entry); err != nil {
			return nil, fmt.Errorf("termdb: read entry: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Lookup returns the entries that have the term (case and spacing aside)
// in the language, or in any variant of it (de-AT for de-DE). An empty
// language looks in all languages.
func (t *Termbase) Lookup(ctx context.Context, text, lang string) ([]Stored, error) {
	key := term.Key(text)
	if lang == "" {
		return t.entries(ctx, t.db.sql, `id IN (SELECT entry_row FROM terms WHERE termbase_id = ? AND key = ?)`, t.id, key)
	}
	return t.entries(ctx, t.db.sql, `id IN (SELECT entry_row FROM terms WHERE termbase_id = ? AND primary_lang = ? AND key = ?)`, t.id, langtag.Primary(lang), key)
}

// Query selects entries for Search.
type Query struct {
	// Text is searched in terms and field values (any substring, case
	// ignored); with Lang, only in that language's terms.
	Text string `json:"text,omitempty"`
	// Lang limits the search to entries with terms in this language (any
	// variant).
	Lang string `json:"lang,omitempty"`
	// Status keeps entries with a term of this status (in Lang, if given).
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// Search returns the entries matching q, in the order they were added,
// and how many match in total.
func (t *Termbase) Search(ctx context.Context, q Query) ([]Stored, int, error) {
	text := strings.TrimSpace(q.Text)
	var candidates []Stored
	var err error
	if len([]rune(text)) >= 3 {
		cond, arg := searchCondition(t.db.d, text)
		candidates, err = t.entries(ctx, t.db.sql, cond, arg)
	} else {
		candidates, err = t.entries(ctx, t.db.sql, "")
	}
	if err != nil {
		return nil, 0, err
	}
	needle := strings.ToLower(text)
	status := term.NormalizeStatus(q.Status)
	var matched []Stored
	for _, s := range candidates {
		if keep(s.Entry, needle, q.Lang, status) {
			matched = append(matched, s)
		}
	}
	total := len(matched)
	if q.Offset > 0 {
		if q.Offset >= len(matched) {
			matched = nil
		} else {
			matched = matched[q.Offset:]
		}
	}
	if q.Limit > 0 && len(matched) > q.Limit {
		matched = matched[:q.Limit]
	}
	return matched, total, nil
}

func keep(e term.Entry, needle, lang, status string) bool {
	inLang := func(l term.Language) bool {
		return lang == "" || langtag.Primary(l.Lang) == langtag.Primary(lang)
	}
	textOK := needle == ""
	if !textOK && lang == "" {
		textOK = strings.Contains(strings.ToLower(body(e)), needle)
	}
	statusOK := status == ""
	langOK := lang == ""
	for _, l := range e.Languages {
		if !inLang(l) {
			continue
		}
		langOK = langOK || len(l.Terms) > 0
		for _, tm := range l.Terms {
			if !textOK && strings.Contains(strings.ToLower(tm.Text), needle) {
				textOK = true
			}
			if !statusOK && tm.Status == status {
				statusOK = true
			}
		}
	}
	return textOK && statusOK && langOK
}

// Export returns the termbase with all its entries, for term.Write.
func (t *Termbase) Export(ctx context.Context) (*term.Termbase, error) {
	def, err := t.Definition(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := t.Entries(ctx)
	if err != nil {
		return nil, err
	}
	tb := &term.Termbase{Definition: def}
	for _, s := range stored {
		tb.Entries = append(tb.Entries, s.Entry)
	}
	return tb, nil
}

// ImportReport tells what an import did.
type ImportReport struct {
	term.PlanReport
	// Invalid counts incoming entries without terms.
	Invalid int `json:"invalid"`
	// Languages and Fields list what was added to the definition.
	Languages []string `json:"languages,omitempty"`
	Fields    []string `json:"fields,omitempty"`
}

// Plan works out what Import would do, without writing anything.
func (t *Termbase) Plan(ctx context.Context, tb *term.Termbase, p term.Policy) ([]term.Action, ImportReport, error) {
	var actions []term.Action
	var rep ImportReport
	err := func() error {
		tx, err := t.db.sql.BeginRead(ctx)
		if err != nil {
			return err
		}
		defer rollback(tx)
		def, err := t.definition(ctx, tx)
		if err != nil {
			return err
		}
		actions, rep, _, err = t.plan(ctx, tx, &def, tb, p)
		return err
	}()
	return actions, rep, err
}

func (t *Termbase) plan(ctx context.Context, q querier, def *term.Definition, tb *term.Termbase, p term.Policy) ([]term.Action, ImportReport, []term.Entry, error) {
	var rep ImportReport
	before := *def
	before.Languages = append([]string(nil), def.Languages...)
	before.Fields = append([]term.Field(nil), def.Fields...)
	// Fields the file declares keep their level, kind and values.
	for _, f := range tb.Fields {
		if !hasField(def.Fields, f.Name) && strings.TrimSpace(f.Name) != "" {
			if f.Kind == "" {
				f.Kind = term.KindText
			}
			def.Fields = append(def.Fields, f)
		}
	}
	for _, l := range tb.Languages {
		r := term.ResolveLang(l, def.Languages)
		if r != "" && !containsFold(def.Languages, r) {
			def.Languages = append(def.Languages, r)
		}
	}
	var incoming []term.Entry
	for _, e := range tb.Entries {
		pe, err := prepare(e, *def)
		if err != nil {
			rep.Invalid++
			continue
		}
		addDefinitionFor(def, pe)
		incoming = append(incoming, pe)
	}
	if p.Lang != "" {
		p.Lang = term.ResolveLang(p.Lang, def.Languages)
	}
	stored, err := t.entries(ctx, q, "")
	if err != nil {
		return nil, rep, nil, err
	}
	existing := make([]term.Entry, len(stored))
	for i, s := range stored {
		existing[i] = s.Entry
	}
	actions, pr := term.Plan(existing, incoming, p)
	rep.PlanReport = pr
	for _, l := range def.Languages {
		if !containsFold(before.Languages, l) {
			rep.Languages = append(rep.Languages, l)
		}
	}
	for _, f := range def.Fields {
		if !hasField(before.Fields, f.Name) {
			rep.Fields = append(rep.Fields, f.Name)
		}
	}
	return actions, rep, incoming, nil
}

// Import adds the entries of tb as p says, in one transaction, and adds
// the languages and fields the termbase lacks. source names the file in the
// entries' history.
func (t *Termbase) Import(ctx context.Context, tb *term.Termbase, p term.Policy, by, source string) (ImportReport, error) {
	var rep ImportReport
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		actions, r, _, err := t.plan(ctx, tx, def, tb, p)
		if err != nil {
			return err
		}
		rep = r
		now := t.db.now()
		from := "Imported"
		if source != "" {
			from += " from " + source
		}
		// Several entries of the file can change the same existing entry;
		// the plan's last action for it holds the combined result.
		last := map[string]int{}
		for i, a := range actions {
			if a.Kind == "merge" || a.Kind == "overwrite" {
				last[a.Existing] = i
			}
		}
		for i, a := range actions {
			switch a.Kind {
			case "add":
				if _, err := t.insert(ctx, tx, *a.Entry, by, from, now); err != nil {
					return err
				}
			case "merge", "overwrite":
				if last[a.Existing] != i {
					continue
				}
				cur, _, err := t.get(ctx, tx, a.Existing)
				if err != nil {
					return err
				}
				e := *a.Entry
				e.ID = cur.ID
				summary := "Merged with an entry " + strings.ToLower(from[:1]) + from[1:]
				if a.Kind == "overwrite" {
					summary = "Replaced by an entry " + strings.ToLower(from[:1]) + from[1:]
				}
				if _, err := t.update(ctx, tx, e, cur.Version, by, summary, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return rep, err
}

// Merge combines entries into the one with keepID (as when duplicates are
// merged): terms and field values of the others are added to it and the
// others are deleted. The history of the kept entry records the merge.
func (t *Termbase) Merge(ctx context.Context, keepID string, others []string, by string) (Stored, error) {
	var out Stored
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		keep, _, err := t.get(ctx, tx, keepID)
		if err != nil {
			return err
		}
		merged := keep.Entry
		var gone []Stored
		for _, id := range others {
			if strings.TrimSpace(id) == strings.TrimSpace(keepID) {
				continue
			}
			o, _, err := t.get(ctx, tx, id)
			if err != nil {
				return fmt.Errorf("entry %s: %w", id, err)
			}
			merged, _ = term.MergeEntries(merged, o.Entry)
			gone = append(gone, o)
		}
		if len(gone) == 0 {
			out = keep
			return nil
		}
		now := t.db.now()
		ids := make([]string, len(gone))
		for i, o := range gone {
			ids[i] = o.ID
			if err := t.deleteEntry(ctx, tx, o, by, "Merged into entry "+keep.ID, now); err != nil {
				return err
			}
		}
		merged.ID = keep.ID
		out, err = t.update(ctx, tx, merged, keep.Version, by, "Merged with entries "+strings.Join(ids, ", "), now)
		return err
	})
	return out, err
}

// Duplicates returns groups of entry IDs that share a term (in lang, or in
// any language when lang is empty).
func (t *Termbase) Duplicates(ctx context.Context, lang string) ([][]string, error) {
	stored, err := t.Entries(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]term.Entry, len(stored))
	for i, s := range stored {
		entries[i] = s.Entry
	}
	if lang != "" {
		def, err := t.Definition(ctx)
		if err != nil {
			return nil, err
		}
		lang = term.ResolveLang(lang, def.Languages)
	}
	var out [][]string
	for _, g := range term.Duplicates(entries, lang) {
		ids := make([]string, len(g))
		for i, n := range g {
			ids[i] = entries[n].ID
		}
		out = append(out, ids)
	}
	return out, nil
}

// AddTo adds the termbase's entries to a recognition index under its name.
func (t *Termbase) AddTo(ctx context.Context, x *term.Index) error {
	stored, err := t.Entries(ctx)
	if err != nil {
		return err
	}
	entries := make([]term.Entry, len(stored))
	for i, s := range stored {
		entries[i] = s.Entry
	}
	x.Add(t.name, entries)
	return nil
}
