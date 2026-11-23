// Package projectdb stores translation projects in a SQLite file (the
// desktop apps' own) or a PostgreSQL database (the server many
// translators work against at once): each project's settings, its files
// with their original bytes, the segments of every translatable file in
// every target language, and the templates new projects start from.
//
// Projects, segments and templates carry versions, and a change made on a
// stale copy is refused (ErrConflict), so two translators working on the
// same project never silently overwrite each other. Every change is
// recorded in a change log in the same transaction, with a sequence
// number clients sync from (Changes).
package projectdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/internal/sqlitedb"
	"github.com/4nonX/interlinea/internal/sqlstore"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/project"
)

// Errors returned by this package.
var (
	ErrNotFound = errors.New("projectdb: not found")
	ErrExists   = errors.New("projectdb: a project with this name already exists")
	// ErrConflict means the project, segment or template changed since
	// the version the caller edited; load it again and reapply the change.
	ErrConflict = errors.New("projectdb: changed by someone else")
	// ErrLocked means the segment is locked in its file.
	ErrLocked = errors.New("projectdb: the segment is locked")
)

// InvalidError lists what is wrong with a project's settings.
type InvalidError struct{ Problems []project.Problem }

func (e *InvalidError) Error() string {
	var b strings.Builder
	b.WriteString("projectdb: invalid project")
	for i, p := range e.Problems {
		if i == 0 {
			b.WriteString(": ")
		} else {
			b.WriteString("; ")
		}
		b.WriteString(p.Field + ": " + p.Message)
	}
	return b.String()
}

const (
	timeLayout  = time.RFC3339Nano
	writeLock   = 7331
	migrateLock = 7332
)

// Kinds of change in the change log.
const (
	ChangeProjectCreated  = "project-created"
	ChangeSettings        = "settings-changed"
	ChangeProjectDeleted  = "project-deleted"
	ChangeFileAdded       = "file-added"
	ChangeFileRemoved     = "file-removed"
	ChangeSegment         = "segment-changed"
	ChangeTemplateSaved   = "template-saved"
	ChangeTemplateDeleted = "template-deleted"
)

// DB is an open database. It is safe for concurrent use.
type DB struct {
	sql *sqlstore.DB
	d   sqlstore.Dialect
	wmu sync.Mutex
	now func() time.Time
}

// Open opens or creates the SQLite database at path and brings its schema
// up to date.
func Open(path string) (*DB, error) {
	sdb, err := sqlitedb.Open(path, "projectdb", migrations)
	if err != nil {
		return nil, err
	}
	return newDB(sdb, sqlstore.SQLite), nil
}

// OpenPostgres uses a PostgreSQL database for projects and brings its
// schema up to date.
func OpenPostgres(ctx context.Context, sdb *sql.DB) (*DB, error) {
	if err := sqlstore.MigratePostgres(ctx, sdb, "projectdb_schema", migrateLock, pgMigrations); err != nil {
		return nil, err
	}
	return newDB(sdb, sqlstore.Postgres), nil
}

func newDB(sdb *sql.DB, d sqlstore.Dialect) *DB {
	return &DB{sql: &sqlstore.DB{DB: sdb, Dialect: d, LockKey: writeLock}, d: d, now: func() time.Time { return time.Now().UTC() }}
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

const schema = `
	CREATE TABLE projects (
		id            %[1]s,
		name          TEXT NOT NULL,
		settings_json TEXT NOT NULL,
		version       INTEGER NOT NULL,
		created_at    TEXT NOT NULL,
		created_by    TEXT NOT NULL,
		changed_at    TEXT NOT NULL,
		changed_by    TEXT NOT NULL
	);
	CREATE UNIQUE INDEX projects_by_name ON projects (lower(name));
	CREATE TABLE files (
		id          %[1]s,
		project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		position    INTEGER NOT NULL,
		name        TEXT NOT NULL,
		role        TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		format      TEXT NOT NULL,
		data        %[2]s NOT NULL
	);
	CREATE UNIQUE INDEX files_by_name ON files (project_id, lower(name));
	CREATE TABLE segments (
		id          %[1]s,
		file_id     BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		target_lang TEXT NOT NULL,
		position    INTEGER NOT NULL,
		seg_id      TEXT NOT NULL,
		source_json TEXT NOT NULL,
		target_json TEXT NOT NULL,
		status      TEXT NOT NULL,
		locked      INTEGER NOT NULL,
		version     INTEGER NOT NULL,
		changed_at  TEXT NOT NULL,
		changed_by  TEXT NOT NULL
	);
	CREATE UNIQUE INDEX segments_by_id ON segments (file_id, target_lang, seg_id);
	CREATE INDEX segments_in_order ON segments (file_id, target_lang, position);
	CREATE TABLE templates (
		id            %[1]s,
		name          TEXT NOT NULL,
		template_json TEXT NOT NULL,
		version       INTEGER NOT NULL,
		changed_at    TEXT NOT NULL,
		changed_by    TEXT NOT NULL
	);
	CREATE UNIQUE INDEX templates_by_name ON templates (lower(name));
	CREATE TABLE changes (
		seq         %[1]s,
		project_id  BIGINT NOT NULL,
		project     TEXT NOT NULL,
		kind        TEXT NOT NULL,
		file        TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		segment     TEXT NOT NULL,
		version     INTEGER NOT NULL,
		at          TEXT NOT NULL,
		author      TEXT NOT NULL
	);
	CREATE INDEX changes_by_project ON changes (project_id, seq);`

// Version 2: who works on what, and who has which segment open.
const schemaV2 = `
	CREATE TABLE assignments (
		id          %[1]s,
		project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		file        TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		from_seg    TEXT NOT NULL,
		to_seg      TEXT NOT NULL,
		user_id     BIGINT NOT NULL,
		user_name   TEXT NOT NULL,
		task        TEXT NOT NULL,
		due         TEXT NOT NULL,
		done        INTEGER NOT NULL,
		version     INTEGER NOT NULL,
		changed_at  TEXT NOT NULL,
		changed_by  TEXT NOT NULL
	);
	CREATE INDEX assignments_by_project ON assignments (project_id, file, target_lang);
	CREATE INDEX assignments_by_user ON assignments (user_id);
	CREATE TABLE presence (
		project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		file        TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		seg_id      TEXT NOT NULL,
		user_id     BIGINT NOT NULL,
		user_name   TEXT NOT NULL,
		since       TEXT NOT NULL,
		expires     TEXT NOT NULL
	);
	CREATE UNIQUE INDEX presence_by_user ON presence (project_id, user_id);
	CREATE INDEX presence_by_file ON presence (project_id, file, target_lang);`

var (
	migrations = []string{
		fmt.Sprintf(schema, "INTEGER PRIMARY KEY AUTOINCREMENT", "BLOB"),
		fmt.Sprintf(schemaV2, "INTEGER PRIMARY KEY AUTOINCREMENT"),
	}
	pgMigrations = []string{
		fmt.Sprintf(schema, "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY", "BYTEA"),
		fmt.Sprintf(schemaV2, "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"),
	}
)

// noNUL keeps NUL out of text columns, which PostgreSQL refuses.
func noNUL(s string) string { return strings.ReplaceAll(s, "\x00", "") }

func parseTime(s string) time.Time {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

type change struct {
	projectID             int64
	project, kind         string
	file, target, segment string
	version               int
}

func logChange(ctx context.Context, tx *sqlstore.Tx, c change, by string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO changes(project_id, project, kind, file, target_lang, segment, version, at, author)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.projectID, c.project, c.kind, c.file, c.target, c.segment, c.version, now.Format(timeLayout), noNUL(by))
	if err != nil {
		return fmt.Errorf("projectdb: change log: %w", err)
	}
	return nil
}

func (d *DB) write(ctx context.Context, fn func(tx *sqlstore.Tx, now time.Time) error) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	tx, err := d.sql.Begin(ctx)
	if err != nil {
		return fmt.Errorf("projectdb: begin: %w", err)
	}
	defer sqlstore.Rollback(tx)
	if err := fn(tx, d.now()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("projectdb: commit: %w", err)
	}
	return nil
}

// NewFile is a file added to a project.
type NewFile struct {
	project.File
	// Format names the file's format ("docx", "xliff", "sdlxliff",
	// "pdf"): what the apps need to save the translation back.
	Format string
	// Data is the original file.
	Data []byte
	// Segments are a translatable file's segments per target language:
	// for a monolingual file one list per target language of the project
	// (usually the same sources with empty targets), for a bilingual file
	// one list for its target language. A reference file has none.
	Segments map[string][]project.Segment
}

// Project is a stored project.
type Project struct {
	ID       int64            `json:"id"`
	Settings project.Settings `json:"settings"`
	// Version counts the changes to the settings and the files.
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
	ChangedAt time.Time `json:"changedAt"`
	ChangedBy string    `json:"changedBy"`
}

func normLang(l string) string {
	n, err := langtag.Normalize(l)
	if err != nil {
		return l
	}
	return n
}

// normalize puts the settings' language tags in canonical form, so the
// keys of segments and memories are stable.
func normalize(s project.Settings) project.Settings {
	s.Name = strings.TrimSpace(s.Name)
	s.SourceLang = normLang(s.SourceLang)
	ts := make([]string, len(s.TargetLangs))
	for i, t := range s.TargetLangs {
		ts[i] = normLang(t)
	}
	s.TargetLangs = ts
	mem := make(map[string][]project.MemorySetting, len(s.Memories))
	for l, ms := range s.Memories {
		mem[normLang(l)] = ms
	}
	s.Memories = mem
	files := make([]project.File, len(s.Files))
	for i, f := range s.Files {
		f.Name = strings.TrimSpace(f.Name)
		if f.TargetLang != "" {
			f.TargetLang = normLang(f.TargetLang)
		}
		files[i] = f
	}
	s.Files = files
	return s
}

// targetsOf are the target languages a file is translated into.
func targetsOf(f project.File, s project.Settings) []string {
	if f.Role != project.Translatable {
		return nil
	}
	if f.TargetLang != "" {
		return []string{f.TargetLang}
	}
	return s.TargetLangs
}

// checkSegments makes sure a file brings exactly the segment lists its
// role and languages need, with unique, non-empty IDs.
func checkSegments(nf NewFile, s project.Settings) error {
	want := map[string]bool{}
	for _, t := range targetsOf(nf.File, s) {
		want[t] = true
	}
	got := map[string]bool{}
	for l, segs := range nf.Segments {
		n := normLang(l)
		if !want[n] {
			if nf.Role == project.Reference {
				return fmt.Errorf("projectdb: %s is a reference file and has no segments", nf.Name)
			}
			return fmt.Errorf("projectdb: %s has segments for %s, which it is not translated into", nf.Name, l)
		}
		got[n] = true
		ids := map[string]bool{}
		for _, sg := range segs {
			if strings.TrimSpace(sg.ID) == "" || strings.ContainsRune(sg.ID, 0) {
				return fmt.Errorf("projectdb: %s has a segment without a usable ID", nf.Name)
			}
			if ids[sg.ID] {
				return fmt.Errorf("projectdb: %s has segment %s twice for %s", nf.Name, sg.ID, n)
			}
			ids[sg.ID] = true
			switch sg.Status {
			case project.StatusNew, project.StatusDraft, project.StatusConfirmed:
			default:
				return fmt.Errorf("projectdb: segment %s of %s has the unknown status %q", sg.ID, nf.Name, sg.Status)
			}
		}
	}
	for t := range want {
		if !got[t] {
			return fmt.Errorf("projectdb: %s has no segments for %s", nf.Name, t)
		}
	}
	return nil
}

func validate(s project.Settings, cat project.Catalog) error {
	if ps := project.Validate(s, cat); len(ps) > 0 {
		return &InvalidError{Problems: ps}
	}
	return nil
}

// Create stores a new project with its files. The settings' file list is
// taken from files, in their order. cat is passed to project.Validate.
func (d *DB) Create(ctx context.Context, s project.Settings, files []NewFile, cat project.Catalog, by string) (Project, error) {
	s.Files = make([]project.File, len(files))
	for i, f := range files {
		s.Files[i] = f.File
	}
	s = normalize(s)
	for i := range files {
		files[i].File = s.Files[i]
	}
	if err := validate(s, cat); err != nil {
		return Project{}, err
	}
	for _, f := range files {
		if err := checkSegments(f, s); err != nil {
			return Project{}, err
		}
	}
	var p Project
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE lower(name) = lower(?)`, s.Name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrExists
		}
		js, err := json.Marshal(s)
		if err != nil {
			return err
		}
		at := now.Format(timeLayout)
		var id int64
		err = tx.QueryRowContext(ctx, `INSERT INTO projects(name, settings_json, version, created_at, created_by, changed_at, changed_by)
			VALUES (?, ?, 1, ?, ?, ?, ?) RETURNING id`, s.Name, string(js), at, noNUL(by), at, noNUL(by)).Scan(&id)
		if err != nil {
			if d.d.IsUnique(err) {
				return ErrExists
			}
			return fmt.Errorf("projectdb: create project: %w", err)
		}
		if err := logChange(ctx, tx, change{projectID: id, project: s.Name, kind: ChangeProjectCreated, version: 1}, by, now); err != nil {
			return err
		}
		for i, f := range files {
			if err := insertFile(ctx, tx, id, i, f, by, now); err != nil {
				return err
			}
		}
		p = Project{ID: id, Settings: s, Version: 1, CreatedAt: now, CreatedBy: by, ChangedAt: now, ChangedBy: by}
		return nil
	})
	return p, err
}

func insertFile(ctx context.Context, tx *sqlstore.Tx, projectID int64, position int, f NewFile, by string, now time.Time) error {
	data := f.Data
	if data == nil {
		data = []byte{}
	}
	var fileID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO files(project_id, position, name, role, target_lang, format, data)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		projectID, position, f.Name, string(f.Role), f.TargetLang, noNUL(f.Format), data).Scan(&fileID)
	if err != nil {
		return fmt.Errorf("projectdb: add %s: %w", f.Name, err)
	}
	langs := make([]string, 0, len(f.Segments))
	for l := range f.Segments {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	for _, l := range langs {
		if err := insertSegments(ctx, tx, fileID, normLang(l), f.Segments[l], by, now); err != nil {
			return err
		}
	}
	return nil
}

func insertSegments(ctx context.Context, tx *sqlstore.Tx, fileID int64, target string, segs []project.Segment, by string, now time.Time) error {
	st, err := tx.PrepareContext(ctx, `INSERT INTO segments(file_id, target_lang, position, seg_id, source_json, target_json, status, locked, version, changed_at, changed_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	at := now.Format(timeLayout)
	for i, sg := range segs {
		src, err := json.Marshal(sg.Source)
		if err != nil {
			return err
		}
		tgt, err := json.Marshal(sg.Target)
		if err != nil {
			return err
		}
		if _, err := st.ExecContext(ctx, fileID, target, i, sg.ID, string(src), string(tgt), string(sg.Status), boolInt(sg.Locked), at, noNUL(by)); err != nil {
			return fmt.Errorf("projectdb: segment %s: %w", sg.ID, err)
		}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

const projectColumns = `id, settings_json, version, created_at, created_by, changed_at, changed_by`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var js, created, changed string
	if err := row.Scan(&p.ID, &js, &p.Version, &created, &p.CreatedBy, &changed, &p.ChangedBy); err != nil {
		return Project{}, err
	}
	if err := json.Unmarshal([]byte(js), &p.Settings); err != nil {
		return Project{}, fmt.Errorf("projectdb: project %d settings: %w", p.ID, err)
	}
	p.CreatedAt, p.ChangedAt = parseTime(created), parseTime(changed)
	return p, nil
}

type queryRower interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func loadProject(ctx context.Context, q queryRower, id int64) (Project, error) {
	p, err := scanProject(q.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// Project returns a project by ID.
func (d *DB) Project(ctx context.Context, id int64) (Project, error) {
	return loadProject(ctx, d.sql, id)
}

// ProjectByName returns a project by name (case-insensitive).
func (d *DB) ProjectByName(ctx context.Context, name string) (Project, error) {
	p, err := scanProject(d.sql.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE lower(name) = lower(?)`, strings.TrimSpace(name)))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// Summary is a project in the project list.
type Summary struct {
	Project
	// Segments and Confirmed count the segments to translate (not
	// locked) over all files and target languages.
	Segments  int `json:"segments"`
	Confirmed int `json:"confirmed"`
}

// List returns all projects, the ones due first first, then by name;
// projects without a due date come last.
func (d *DB) List(ctx context.Context) ([]Summary, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT p.id, p.settings_json, p.version, p.created_at, p.created_by, p.changed_at, p.changed_by,
			(SELECT count(*) FROM segments s JOIN files f ON f.id = s.file_id WHERE f.project_id = p.id AND s.locked = 0),
			(SELECT count(*) FROM segments s JOIN files f ON f.id = s.file_id WHERE f.project_id = p.id AND s.locked = 0 AND s.status = 'confirmed')
		FROM projects p`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var sm Summary
		var js, created, changed string
		if err := rows.Scan(&sm.ID, &js, &sm.Version, &created, &sm.CreatedBy, &changed, &sm.ChangedBy, &sm.Segments, &sm.Confirmed); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(js), &sm.Settings); err != nil {
			return nil, fmt.Errorf("projectdb: project %d settings: %w", sm.ID, err)
		}
		sm.CreatedAt, sm.ChangedAt = parseTime(created), parseTime(changed)
		out = append(out, sm)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Settings, out[j].Settings
		if (a.Due == "") != (b.Due == "") {
			return a.Due != ""
		}
		if a.Due != b.Due {
			ta, _ := time.Parse(time.RFC3339, a.Due)
			tb, _ := time.Parse(time.RFC3339, b.Due)
			return ta.Before(tb)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out, nil
}

// bumpProject writes new settings if the project is still at version.
func bumpProject(ctx context.Context, tx *sqlstore.Tx, id int64, s project.Settings, version int, by string, now time.Time) error {
	js, err := json.Marshal(s)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE projects SET name = ?, settings_json = ?, version = version + 1, changed_at = ?, changed_by = ?
		WHERE id = ? AND version = ?`, s.Name, string(js), now.Format(timeLayout), noNUL(by), id, version)
	if err != nil {
		return fmt.Errorf("projectdb: update project: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrConflict
	}
	return nil
}

func sameFiles(a, b []project.File) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func currentProject(ctx context.Context, tx *sqlstore.Tx, id int64, version int) (Project, error) {
	cur, err := loadProject(ctx, tx, id)
	if err != nil {
		return Project{}, err
	}
	if cur.Version != version {
		return Project{}, ErrConflict
	}
	return cur, nil
}

// UpdateSettings replaces a project's settings, as edited from version.
// Files are added and removed with AddFile and RemoveFile, so s.Files
// must list the project's files as they are. A target language added to
// the project gets segments for every monolingual file, copied from the
// file's segments in the first target language with empty translations;
// a target language removed takes its segments with it.
func (d *DB) UpdateSettings(ctx context.Context, id int64, s project.Settings, version int, cat project.Catalog, by string) (Project, error) {
	s = normalize(s)
	if err := validate(s, cat); err != nil {
		return Project{}, err
	}
	var out Project
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := currentProject(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if !sameFiles(cur.Settings.Files, s.Files) {
			return fmt.Errorf("projectdb: the settings list other files than the project has; add and remove files with AddFile and RemoveFile")
		}
		if !strings.EqualFold(cur.Settings.Name, s.Name) {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE lower(name) = lower(?) AND id <> ?`, s.Name, id).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return ErrExists
			}
		}
		if cur.Settings.SourceLang != s.SourceLang {
			return fmt.Errorf("projectdb: the source language of a project can't change")
		}
		old := map[string]bool{}
		for _, t := range cur.Settings.TargetLangs {
			old[t] = true
		}
		now2 := map[string]bool{}
		for _, t := range s.TargetLangs {
			now2[t] = true
		}
		// New languages copy the sources of the first target language.
		first := cur.Settings.TargetLangs[0]
		for _, t := range s.TargetLangs {
			if old[t] {
				continue
			}
			for _, f := range cur.Settings.Files {
				if f.Role != project.Translatable || f.TargetLang != "" {
					continue
				}
				fileID, err := fileID(ctx, tx, id, f.Name)
				if err != nil {
					return err
				}
				segs, err := querySegments(ctx, tx, fileID, first)
				if err != nil {
					return err
				}
				fresh := make([]project.Segment, len(segs))
				for i, sg := range segs {
					fresh[i] = project.Segment{ID: sg.ID, Source: sg.Source, Status: project.StatusNew, Locked: sg.Locked}
				}
				if err := insertSegments(ctx, tx, fileID, t, fresh, by, now); err != nil {
					return err
				}
			}
		}
		// Removed languages go after the new ones are copied, which may
		// copy from a language being removed.
		for _, t := range cur.Settings.TargetLangs {
			if now2[t] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM segments WHERE target_lang = ? AND file_id IN (SELECT id FROM files WHERE project_id = ?)`, t, id); err != nil {
				return err
			}
			if err := dropWork(ctx, tx, id, "target_lang = ?", t); err != nil {
				return err
			}
		}
		if err := bumpProject(ctx, tx, id, s, version, by, now); err != nil {
			return err
		}
		if err := logChange(ctx, tx, change{projectID: id, project: s.Name, kind: ChangeSettings, version: version + 1}, by, now); err != nil {
			return err
		}
		out = cur
		out.Settings, out.Version, out.ChangedAt, out.ChangedBy = s, version+1, now, by
		return nil
	})
	return out, err
}

func fileID(ctx context.Context, tx queryRower, projectID int64, name string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM files WHERE project_id = ? AND lower(name) = lower(?)`, projectID, strings.TrimSpace(name)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// AddFile adds a file to a project edited from version.
func (d *DB) AddFile(ctx context.Context, id int64, f NewFile, version int, cat project.Catalog, by string) (Project, error) {
	var out Project
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := currentProject(ctx, tx, id, version)
		if err != nil {
			return err
		}
		s := cur.Settings
		s.Files = append(append([]project.File(nil), s.Files...), f.File)
		s = normalize(s)
		f.File = s.Files[len(s.Files)-1]
		if err := validate(s, cat); err != nil {
			return err
		}
		if err := checkSegments(f, s); err != nil {
			return err
		}
		if err := insertFile(ctx, tx, id, len(s.Files)-1, f, by, now); err != nil {
			if d.d.IsUnique(err) {
				return &InvalidError{Problems: []project.Problem{{Field: "files", Message: f.Name + " is in the project already."}}}
			}
			return err
		}
		if err := bumpProject(ctx, tx, id, s, version, by, now); err != nil {
			return err
		}
		if err := logChange(ctx, tx, change{projectID: id, project: s.Name, kind: ChangeFileAdded, file: f.Name, version: version + 1}, by, now); err != nil {
			return err
		}
		out = cur
		out.Settings, out.Version, out.ChangedAt, out.ChangedBy = s, version+1, now, by
		return nil
	})
	return out, err
}

// RemoveFile removes a file and its segments from a project edited from
// version. The last file to translate can't be removed.
func (d *DB) RemoveFile(ctx context.Context, id int64, name string, version int, cat project.Catalog, by string) (Project, error) {
	var out Project
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := currentProject(ctx, tx, id, version)
		if err != nil {
			return err
		}
		s := cur.Settings
		var files []project.File
		var removed string
		for _, f := range s.Files {
			if strings.EqualFold(f.Name, strings.TrimSpace(name)) {
				removed = f.Name
				continue
			}
			files = append(files, f)
		}
		if removed == "" {
			return ErrNotFound
		}
		s.Files = files
		if err := validate(s, cat); err != nil {
			return err
		}
		fid, err := fileID(ctx, tx, id, removed)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, fid); err != nil {
			return err
		}
		if err := dropWork(ctx, tx, id, "lower(file) = lower(?)", removed); err != nil {
			return err
		}
		for i, f := range s.Files {
			if _, err := tx.ExecContext(ctx, `UPDATE files SET position = ? WHERE project_id = ? AND lower(name) = lower(?)`, i, id, f.Name); err != nil {
				return err
			}
		}
		if err := bumpProject(ctx, tx, id, s, version, by, now); err != nil {
			return err
		}
		if err := logChange(ctx, tx, change{projectID: id, project: s.Name, kind: ChangeFileRemoved, file: removed, version: version + 1}, by, now); err != nil {
			return err
		}
		out = cur
		out.Settings, out.Version, out.ChangedAt, out.ChangedBy = s, version+1, now, by
		return nil
	})
	return out, err
}

// StoredFile is a file's stored description and bytes.
type StoredFile struct {
	project.File
	Format string
	Data   []byte
}

// File returns a file of a project with its original bytes.
func (d *DB) File(ctx context.Context, projectID int64, name string) (StoredFile, error) {
	var f StoredFile
	var role string
	err := d.sql.QueryRowContext(ctx, `SELECT name, role, target_lang, format, data FROM files WHERE project_id = ? AND lower(name) = lower(?)`,
		projectID, strings.TrimSpace(name)).Scan(&f.Name, &role, &f.TargetLang, &f.Format, &f.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredFile{}, ErrNotFound
	}
	if err != nil {
		return StoredFile{}, err
	}
	f.Role = project.Role(role)
	return f, nil
}

// Delete removes a project with its files and segments.
func (d *DB) Delete(ctx context.Context, id int64, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := loadProject(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
			return err
		}
		return logChange(ctx, tx, change{projectID: id, project: cur.Settings.Name, kind: ChangeProjectDeleted, version: cur.Version}, by, now)
	})
}

// StoredSegment is a segment with its version.
type StoredSegment struct {
	project.Segment
	Version   int       `json:"version"`
	ChangedAt time.Time `json:"changedAt"`
	ChangedBy string    `json:"changedBy"`
}

func querySegments(ctx context.Context, q interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}, fileID int64, target string) ([]StoredSegment, error) {
	rows, err := q.QueryContext(ctx, `SELECT seg_id, source_json, target_json, status, locked, version, changed_at, changed_by
		FROM segments WHERE file_id = ? AND target_lang = ? ORDER BY position`, fileID, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredSegment
	for rows.Next() {
		var s StoredSegment
		var src, tgt, status, changed string
		var locked int
		if err := rows.Scan(&s.ID, &src, &tgt, &status, &locked, &s.Version, &changed, &s.ChangedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(src), &s.Source); err != nil {
			return nil, fmt.Errorf("projectdb: segment %s source: %w", s.ID, err)
		}
		if err := json.Unmarshal([]byte(tgt), &s.Target); err != nil {
			return nil, fmt.Errorf("projectdb: segment %s target: %w", s.ID, err)
		}
		s.Status, s.Locked, s.ChangedAt = project.Status(status), locked != 0, parseTime(changed)
		out = append(out, s)
	}
	return out, rows.Err()
}

// Segments returns a file's segments in a target language, in order.
func (d *DB) Segments(ctx context.Context, projectID int64, file, target string) ([]StoredSegment, error) {
	fid, err := fileID(ctx, d.sql, projectID, file)
	if err != nil {
		return nil, err
	}
	return querySegments(ctx, d.sql, fid, normLang(target))
}

// SaveSegment stores a segment's translation and status, as edited from
// version.
func (d *DB) SaveSegment(ctx context.Context, projectID int64, file, target, segID string, tgt content.Content, status project.Status, version int, by string) (StoredSegment, error) {
	switch status {
	case project.StatusNew, project.StatusDraft, project.StatusConfirmed:
	default:
		return StoredSegment{}, fmt.Errorf("projectdb: unknown status %q", status)
	}
	target = normLang(target)
	var out StoredSegment
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := loadProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		fid, err := fileID(ctx, tx, projectID, file)
		if err != nil {
			return err
		}
		var locked, ver int
		err = tx.QueryRowContext(ctx, `SELECT locked, version FROM segments WHERE file_id = ? AND target_lang = ? AND seg_id = ?`, fid, target, segID).Scan(&locked, &ver)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if locked != 0 {
			return ErrLocked
		}
		if ver != version {
			return ErrConflict
		}
		js, err := json.Marshal(tgt)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE segments SET target_json = ?, status = ?, version = version + 1, changed_at = ?, changed_by = ?
			WHERE file_id = ? AND target_lang = ? AND seg_id = ? AND version = ?`,
			string(js), string(status), now.Format(timeLayout), noNUL(by), fid, target, segID, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrConflict
		}
		if err := logChange(ctx, tx, change{projectID: projectID, project: cur.Settings.Name, kind: ChangeSegment, file: file, target: target, segment: segID, version: version + 1}, by, now); err != nil {
			return err
		}
		segs, err := querySegments(ctx, tx, fid, target)
		if err != nil {
			return err
		}
		for _, s := range segs {
			if s.ID == segID {
				out = s
			}
		}
		return nil
	})
	return out, err
}

// ApplyTranslations writes pre-translation results into a target
// language's segments in one transaction. A segment that was translated,
// locked or removed in the meantime is left alone; the result counts the
// segments written.
func (d *DB) ApplyTranslations(ctx context.Context, projectID int64, target string, ts []project.Translation, by string) (int, error) {
	target = normLang(target)
	n := 0
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := loadProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		ids := map[string]int64{}
		for _, t := range ts {
			fid, ok := ids[t.File]
			if !ok {
				if fid, err = fileID(ctx, tx, projectID, t.File); errors.Is(err, ErrNotFound) {
					continue
				} else if err != nil {
					return err
				}
				ids[t.File] = fid
			}
			switch t.Status {
			case project.StatusDraft, project.StatusConfirmed:
			default:
				return fmt.Errorf("projectdb: pre-translation of %s %s has the status %q", t.File, t.ID, t.Status)
			}
			js, err := json.Marshal(t.Target)
			if err != nil {
				return err
			}
			res, err := tx.ExecContext(ctx, `UPDATE segments SET target_json = ?, status = ?, version = version + 1, changed_at = ?, changed_by = ?
				WHERE file_id = ? AND target_lang = ? AND seg_id = ? AND status = 'new' AND locked = 0 AND target_json = '[]'`,
				string(js), string(t.Status), now.Format(timeLayout), noNUL(by), fid, target, t.ID)
			if err != nil {
				return err
			}
			if k, err := res.RowsAffected(); err != nil {
				return err
			} else if k == 1 {
				var ver int
				if err := tx.QueryRowContext(ctx, `SELECT version FROM segments WHERE file_id = ? AND target_lang = ? AND seg_id = ?`, fid, target, t.ID).Scan(&ver); err != nil {
					return err
				}
				if err := logChange(ctx, tx, change{projectID: projectID, project: cur.Settings.Name, kind: ChangeSegment, file: t.File, target: target, segment: t.ID, version: ver}, by, now); err != nil {
					return err
				}
				n++
			}
		}
		return nil
	})
	return n, err
}

// StoredTemplate is a template with its version.
type StoredTemplate struct {
	project.Template
	Version   int       `json:"version"`
	ChangedAt time.Time `json:"changedAt"`
	ChangedBy string    `json:"changedBy"`
}

// SaveTemplate stores a template under its name. version is 0 for a new
// template, else the version the caller edited.
func (d *DB) SaveTemplate(ctx context.Context, t project.Template, version int, cat project.Catalog, by string) (StoredTemplate, error) {
	t.Name = strings.TrimSpace(t.Name)
	if ps := project.ValidateTemplate(t, cat); len(ps) > 0 {
		return StoredTemplate{}, &InvalidError{Problems: ps}
	}
	var out StoredTemplate
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		js, err := json.Marshal(t)
		if err != nil {
			return err
		}
		at := now.Format(timeLayout)
		var cur int
		err = tx.QueryRowContext(ctx, `SELECT version FROM templates WHERE lower(name) = lower(?)`, t.Name).Scan(&cur)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if version != 0 {
				return ErrNotFound
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO templates(name, template_json, version, changed_at, changed_by) VALUES (?, ?, 1, ?, ?)`,
				t.Name, string(js), at, noNUL(by)); err != nil {
				return err
			}
		case err != nil:
			return err
		case cur != version:
			return ErrConflict
		default:
			if _, err := tx.ExecContext(ctx, `UPDATE templates SET name = ?, template_json = ?, version = version + 1, changed_at = ?, changed_by = ? WHERE lower(name) = lower(?) AND version = ?`,
				t.Name, string(js), at, noNUL(by), t.Name, version); err != nil {
				return err
			}
		}
		if err := logChange(ctx, tx, change{kind: ChangeTemplateSaved, project: t.Name, version: version + 1}, by, now); err != nil {
			return err
		}
		out = StoredTemplate{Template: t, Version: version + 1, ChangedAt: now, ChangedBy: by}
		return nil
	})
	return out, err
}

// Templates returns all templates by name.
func (d *DB) Templates(ctx context.Context) ([]StoredTemplate, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT template_json, version, changed_at, changed_by FROM templates ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredTemplate
	for rows.Next() {
		var t StoredTemplate
		var js, at string
		if err := rows.Scan(&js, &t.Version, &at, &t.ChangedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(js), &t.Template); err != nil {
			return nil, err
		}
		t.ChangedAt = parseTime(at)
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTemplate removes a template.
func (d *DB) DeleteTemplate(ctx context.Context, name, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		var real string
		var ver int
		err := tx.QueryRowContext(ctx, `SELECT name, version FROM templates WHERE lower(name) = lower(?)`, strings.TrimSpace(name)).Scan(&real, &ver)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM templates WHERE lower(name) = lower(?)`, real); err != nil {
			return err
		}
		return logChange(ctx, tx, change{kind: ChangeTemplateDeleted, project: real, version: ver}, by, now)
	})
}

// Change is one entry of the change log. For template changes Project is
// the template's name and ProjectID 0.
type Change struct {
	Seq        int64     `json:"seq"`
	ProjectID  int64     `json:"projectId,omitempty"`
	Project    string    `json:"project"`
	Kind       string    `json:"kind"`
	File       string    `json:"file,omitempty"`
	TargetLang string    `json:"targetLang,omitempty"`
	Segment    string    `json:"segment,omitempty"`
	Version    int       `json:"version,omitempty"`
	At         time.Time `json:"at"`
	By         string    `json:"by"`
}

// ChangeQuery selects changes.
type ChangeQuery struct {
	// Since returns changes after this sequence number.
	Since int64
	// Project limits the log to one project (0 for all, templates
	// included).
	Project int64
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
	query := `SELECT seq, project_id, project, kind, file, target_lang, segment, version, at, author FROM changes WHERE seq > ?`
	args := []any{q.Since}
	if q.Project != 0 {
		query += ` AND project_id = ?`
		args = append(args, q.Project)
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
		if err := rows.Scan(&c.Seq, &c.ProjectID, &c.Project, &c.Kind, &c.File, &c.TargetLang, &c.Segment, &c.Version, &at, &c.By); err != nil {
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
