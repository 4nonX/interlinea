package termdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/4nonX/interlinea/term"
)

// Project termbases are layered over master termbases, as a branch is
// over the branch it started from: translators work in the project, which
// shows the masters' entries with the project's own entries over them;
// what the project adds or changes goes back to a master through a merge
// request (see review.go). In git terms a master is a protected main
// branch, a project a branch, a change a commit and a merge request a
// pull request.

// ErrNotProject is returned for project operations on a master termbase.
var ErrNotProject = errors.New("termdb: not a project termbase")

// Info is what a termbase is and what it is layered over.
type Info struct {
	Name string `json:"name"`
	// Kind is "master" or "project".
	Kind string `json:"kind"`
	// Upstreams are the masters a project is layered over, highest
	// priority first.
	Upstreams []string `json:"upstreams,omitempty"`
}

// BaseRef is the master entry and version a project entry started from.
type BaseRef struct {
	Termbase string `json:"termbase"`
	EntryID  string `json:"entryId"`
	Version  int    `json:"version"`
}

// CreateProject creates a project termbase layered over the masters
// (highest priority first). Without languages of its own it takes the
// masters' languages, and their fields.
func (d *DB) CreateProject(ctx context.Context, def term.Definition, upstreams []string, by string) (*Termbase, error) {
	if len(upstreams) == 0 {
		return nil, errors.New("termdb: a project termbase needs at least one master")
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, n := range upstreams {
		m, err := d.Termbase(ctx, n)
		if err != nil {
			return nil, fmt.Errorf("termdb: master %q: %w", n, err)
		}
		info, err := m.Info(ctx)
		if err != nil {
			return nil, err
		}
		if info.Kind != kindMaster {
			return nil, fmt.Errorf("termdb: %q is a project termbase; projects are layered over masters", n)
		}
		if seen[m.id] {
			continue
		}
		seen[m.id] = true
		ids = append(ids, m.id)
		mdef, err := m.Definition(ctx)
		if err != nil {
			return nil, err
		}
		if len(def.Languages) == 0 {
			for _, l := range mdef.Languages {
				if !containsFold(def.Languages, l) {
					def.Languages = append(def.Languages, l)
				}
			}
		}
		for _, f := range mdef.Fields {
			if !hasField(def.Fields, f.Name) {
				def.Fields = append(def.Fields, f)
			}
		}
	}
	def, err := cleanDefinition(def)
	if err != nil {
		return nil, err
	}
	return d.create(ctx, def, kindProject, ids, by)
}

// Info returns the termbase's kind and upstreams.
func (t *Termbase) Info(ctx context.Context) (Info, error) {
	return t.info(ctx, t.db.sql)
}

func (t *Termbase) info(ctx context.Context, q querier) (Info, error) {
	var info Info
	err := q.QueryRowContext(ctx, `SELECT name, kind FROM termbases WHERE id = ?`, t.id).Scan(&info.Name, &info.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return info, ErrNotFound
	}
	if err != nil {
		return info, err
	}
	ups, err := upstreamsOf(ctx, q, t.db, t.id)
	if err != nil {
		return info, err
	}
	for _, u := range ups {
		info.Upstreams = append(info.Upstreams, u.name)
	}
	return info, nil
}

// upstreamsOf lists a project's masters in priority order.
func upstreamsOf(ctx context.Context, q querier, db *DB, projectID int64) ([]*Termbase, error) {
	rows, err := q.QueryContext(ctx, `SELECT m.id, m.name FROM termbase_upstreams u JOIN termbases m ON m.id = u.master_id WHERE u.project_id = ? ORDER BY u.position`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Termbase
	for rows.Next() {
		u := &Termbase{db: db}
		if err := rows.Scan(&u.id, &u.name); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (t *Termbase) upstreams(ctx context.Context, q querier) ([]*Termbase, error) {
	var kind string
	if err := q.QueryRowContext(ctx, `SELECT kind FROM termbases WHERE id = ?`, t.id).Scan(&kind); err != nil {
		return nil, err
	}
	if kind != kindProject {
		return nil, ErrNotProject
	}
	return upstreamsOf(ctx, q, t.db, t.id)
}

// setUpstreams replaces a project's masters.
func setUpstreams(ctx context.Context, tx *wtx, projectID int64, masters []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM termbase_upstreams WHERE project_id = ?`, projectID); err != nil {
		return err
	}
	for i, m := range masters {
		if _, err := tx.ExecContext(ctx, `INSERT INTO termbase_upstreams(project_id, master_id, position) VALUES (?, ?, ?)`, projectID, m, i); err != nil {
			return err
		}
	}
	return nil
}

// SetUpstreams changes the masters a project is layered over.
func (t *Termbase) SetUpstreams(ctx context.Context, names []string, by string) error {
	if len(names) == 0 {
		return errors.New("termdb: a project termbase needs at least one master")
	}
	t.db.wmu.Lock()
	defer t.db.wmu.Unlock()
	tx, err := t.db.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err := t.upstreams(ctx, tx); err != nil {
		return err
	}
	var ids []int64
	for _, n := range names {
		var id int64
		var kind string
		err := tx.QueryRowContext(ctx, `SELECT id, kind FROM termbases WHERE lower(name) = lower(?)`, strings.TrimSpace(n)).Scan(&id, &kind)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("termdb: master %q: %w", n, ErrNotFound)
		}
		if err != nil {
			return err
		}
		if kind != kindMaster {
			return fmt.Errorf("termdb: %q is a project termbase; projects are layered over masters", n)
		}
		ids = append(ids, id)
	}
	if err := setUpstreams(ctx, tx, t.id, ids); err != nil {
		return err
	}
	if err := logChange(ctx, tx, t.id, "", "", 0, ChangeTermbaseChanged, by, "Layered over "+strings.Join(names, ", "), t.db.now()); err != nil {
		return err
	}
	return tx.Commit()
}

// Base returns the master entry a project entry started from; ok is
// false for an entry the project added.
func (t *Termbase) Base(ctx context.Context, entryID string) (ref BaseRef, ok bool, err error) {
	return t.base(ctx, t.db.sql, entryID)
}

func (t *Termbase) base(ctx context.Context, q querier, entryID string) (BaseRef, bool, error) {
	var tb sql.NullInt64
	var be sql.NullString
	var bv sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT base_termbase, base_entry, base_version FROM entries WHERE termbase_id = ? AND entry_id = ?`, t.id, strings.TrimSpace(entryID)).Scan(&tb, &be, &bv)
	if errors.Is(err, sql.ErrNoRows) {
		return BaseRef{}, false, ErrNotFound
	}
	if err != nil {
		return BaseRef{}, false, err
	}
	if !tb.Valid {
		return BaseRef{}, false, nil
	}
	ref := BaseRef{EntryID: be.String, Version: int(bv.Int64)}
	if err := q.QueryRowContext(ctx, `SELECT name FROM termbases WHERE id = ?`, tb.Int64).Scan(&ref.Termbase); err != nil {
		return BaseRef{}, false, fmt.Errorf("termdb: base termbase of %s: %w", entryID, err)
	}
	return ref, true, nil
}

// setBase records that a project entry, at projectVersion, is in step
// with a master entry at version: changes after projectVersion are what
// the next merge request carries.
func setBase(ctx context.Context, tx execer, projectID int64, entryID string, projectVersion int, masterID int64, masterEntry string, version int) error {
	_, err := tx.ExecContext(ctx, `UPDATE entries SET base_termbase = ?, base_entry = ?, base_version = ?, base_synced = ? WHERE termbase_id = ? AND entry_id = ?`,
		masterID, masterEntry, version, projectVersion, projectID, entryID)
	return err
}

// Override copies a master entry into the project so it can be changed
// there; the copy remembers the master version it started from, so the
// change can be merged back three-way. If the project already has its
// own copy, that copy is returned.
func (t *Termbase) Override(ctx context.Context, master, entryID, by string) (Stored, error) {
	t.db.wmu.Lock()
	defer t.db.wmu.Unlock()
	tx, err := t.db.sql.Begin(ctx)
	if err != nil {
		return Stored{}, err
	}
	defer rollback(tx)
	ups, err := t.upstreams(ctx, tx)
	if err != nil {
		return Stored{}, err
	}
	var m *Termbase
	for _, u := range ups {
		if strings.EqualFold(u.name, strings.TrimSpace(master)) {
			m = u
		}
	}
	if m == nil {
		return Stored{}, fmt.Errorf("termdb: %s is not layered over %q", t.name, master)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT entry_id FROM entries WHERE termbase_id = ? AND base_termbase = ? AND base_entry = ?`, t.id, m.id, strings.TrimSpace(entryID)).Scan(&existing)
	if err == nil {
		s, _, err := t.get(ctx, tx, existing)
		return s, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Stored{}, err
	}
	src, _, err := m.get(ctx, tx, entryID)
	if err != nil {
		return Stored{}, err
	}
	def, err := t.definition(ctx, tx)
	if err != nil {
		return Stored{}, err
	}
	e := src.Entry
	e.ID = ""
	e.CreatedAt, e.CreatedBy = time.Time{}, ""
	if addDefinitionFor(&def, e) {
		if err := writeDefinition(ctx, tx, t.id, def); err != nil {
			return Stored{}, err
		}
	}
	out, err := t.insert(ctx, tx, e, by, fmt.Sprintf("Copied from %s (%s, version %d)", m.name, src.ID, src.Version), t.db.now())
	if err != nil {
		return Stored{}, err
	}
	if err := setBase(ctx, tx, t.id, out.ID, out.Version, m.id, src.ID, src.Version); err != nil {
		return Stored{}, err
	}
	return out, tx.Commit()
}

// RemoveFromProject hides a master entry in the project and proposes its
// deletion in the next merge request. The project's own copy, if any, is
// deleted.
func (t *Termbase) RemoveFromProject(ctx context.Context, master, entryID, by string) error {
	t.db.wmu.Lock()
	defer t.db.wmu.Unlock()
	tx, err := t.db.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	ups, err := t.upstreams(ctx, tx)
	if err != nil {
		return err
	}
	var m *Termbase
	for _, u := range ups {
		if strings.EqualFold(u.name, strings.TrimSpace(master)) {
			m = u
		}
	}
	if m == nil {
		return fmt.Errorf("termdb: %s is not layered over %q", t.name, master)
	}
	src, _, err := m.get(ctx, tx, entryID)
	if err != nil {
		return err
	}
	now := t.db.now()
	var copyID string
	err = tx.QueryRowContext(ctx, `SELECT entry_id FROM entries WHERE termbase_id = ? AND base_termbase = ? AND base_entry = ?`, t.id, m.id, src.ID).Scan(&copyID)
	if err == nil {
		s, _, err := t.get(ctx, tx, copyID)
		if err != nil {
			return err
		}
		if err := t.deleteEntry(ctx, tx, s, by, "Removed from the project; deletion proposed for "+m.name, now); err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_removals(project_id, base_termbase, base_entry, base_version, at, author) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (project_id, base_termbase, base_entry) DO UPDATE SET base_version = excluded.base_version, at = excluded.at, author = excluded.author`,
		t.id, m.id, src.ID, src.Version, now.Format(timeLayout), by); err != nil {
		return err
	}
	if err := logChange(ctx, tx, t.id, "", src.ID, src.Version, ChangeDeleted, by, "Removed "+m.name+" entry "+src.ID+" from the project", now); err != nil {
		return err
	}
	return tx.Commit()
}

// Removal is a master entry a project has removed.
type Removal struct {
	BaseRef
	At time.Time `json:"at"`
	By string    `json:"by"`
}

// Removals lists the master entries the project has removed.
func (t *Termbase) Removals(ctx context.Context) ([]Removal, error) {
	rows, err := t.db.sql.QueryContext(ctx, `SELECT tb.name, r.base_entry, r.base_version, r.at, r.author FROM project_removals r JOIN termbases tb ON tb.id = r.base_termbase WHERE r.project_id = ? ORDER BY r.at`, t.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Removal
	for rows.Next() {
		var r Removal
		var at string
		if err := rows.Scan(&r.Termbase, &r.EntryID, &r.Version, &at, &r.By); err != nil {
			return nil, err
		}
		r.At, _ = time.Parse(timeLayout, at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Layered is an entry as a project shows it.
type Layered struct {
	Stored
	// Termbase is where the entry lives: the project or a master.
	Termbase string `json:"termbase"`
	// Base is set for a project entry changed from a master entry.
	Base *BaseRef `json:"base,omitempty"`
}

// Layered returns the project's view: its own entries first, then each
// master's in priority order, without the master entries the project has
// its own copy of or has removed. On a master it returns its entries.
func (t *Termbase) Layered(ctx context.Context) ([]Layered, error) {
	own, err := t.Entries(ctx)
	if err != nil {
		return nil, err
	}
	ups, err := t.upstreams(ctx, t.db.sql)
	if errors.Is(err, ErrNotProject) {
		out := make([]Layered, len(own))
		for i, s := range own {
			out[i] = Layered{Stored: s, Termbase: t.name}
		}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	hidden := map[string]bool{} // "termbaseID/entryID"
	var out []Layered
	for _, s := range own {
		l := Layered{Stored: s, Termbase: t.name}
		ref, ok, err := t.Base(ctx, s.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			l.Base = &ref
		}
		out = append(out, l)
	}
	rows, err := t.db.sql.QueryContext(ctx, `SELECT base_termbase, base_entry FROM entries WHERE termbase_id = ? AND base_termbase IS NOT NULL
		UNION SELECT base_termbase, base_entry FROM project_removals WHERE project_id = ?`, t.id, t.id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var tb int64
		var e string
		if err := rows.Scan(&tb, &e); err != nil {
			rows.Close()
			return nil, err
		}
		hidden[fmt.Sprintf("%d/%s", tb, e)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, u := range ups {
		es, err := u.Entries(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range es {
			if hidden[fmt.Sprintf("%d/%s", u.id, s.ID)] {
				continue
			}
			out = append(out, Layered{Stored: s, Termbase: u.name})
		}
	}
	return out, nil
}

// AddLayeredTo adds the project's view to a term index, project entries
// first so they win over the masters', each under the name of the
// termbase it lives in.
func (t *Termbase) AddLayeredTo(ctx context.Context, x *term.Index) error {
	ls, err := t.Layered(ctx)
	if err != nil {
		return err
	}
	var group []term.Entry
	source := ""
	flush := func() {
		if len(group) > 0 {
			x.Add(source, group)
			group = nil
		}
	}
	for _, l := range ls {
		if l.Termbase != source {
			flush()
			source = l.Termbase
		}
		group = append(group, l.Entry)
	}
	flush()
	return nil
}
