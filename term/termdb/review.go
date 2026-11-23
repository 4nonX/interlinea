package termdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/4nonX/interlinea/term"
)

// A merge request carries what a project added, changed and removed back
// to one of its masters. Every entry in it is reviewed on its own: it can
// be approved (and so merged into the master at once), approved with
// edits, sent back with a request for changes, rejected or postponed,
// one by one or for a filtered selection at a time, so a request with
// thousands of new terms is worked through in parts. A request can be
// split among reviewers by scope. Rules set on the master (ReviewRules)
// decide entries that need no review, and record which rule did.

// Item kinds.
const (
	ItemNew     = "new"
	ItemChanged = "changed"
	ItemDeleted = "deleted"
)

// Item states.
const (
	StateWaiting   = "waiting"
	StateApproved  = "approved"
	StateEdited    = "approved-with-edits"
	StateChanges   = "changes-requested"
	StateRejected  = "rejected"
	StatePostponed = "postponed"
	StateReverted  = "reverted"
)

// Errors of the review.
var (
	// ErrNeedsResolution means the master changed the same places as the
	// project since the project's copy was made; approve with an edited
	// entry (the reviewer's resolution) instead.
	ErrNeedsResolution = errors.New("termdb: the master changed the same places; resolve the conflicts first")
	ErrClosed          = errors.New("termdb: the merge request is closed")
	ErrDecided         = errors.New("termdb: the entry has already been decided")
)

// MergeRequest is a request to merge a project's changes into a master.
type MergeRequest struct {
	ID        int64     `json:"id"`
	Project   string    `json:"project"`
	Target    string    `json:"target"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
	Deadline  time.Time `json:"deadline,omitzero"`
	State     string    `json:"state"` // open or closed
	ClosedAt  time.Time `json:"closedAt,omitzero"`

	db                  *DB
	projectID, targetID int64
}

// Item is one entry of a merge request with what the reviewer needs to
// decide it.
type Item struct {
	ID             int64     `json:"id"`
	Kind           string    `json:"kind"`
	EntryID        string    `json:"entryId"`
	ProjectVersion int       `json:"projectVersion"`
	Base           *BaseRef  `json:"base,omitempty"`
	Author         string    `json:"author"`
	ChangedAt      time.Time `json:"changedAt"`
	State          string    `json:"state"`
	Reviewer       string    `json:"reviewer,omitempty"`
	DecidedAt      time.Time `json:"decidedAt,omitzero"`
	Comment        string    `json:"comment,omitempty"`
	// Rule names the review rule that decided the entry, if one did.
	Rule string `json:"rule,omitempty"`
	// Result is the master entry the approval wrote.
	Result *BaseRef `json:"result,omitempty"`

	// Entry is the project's version (for a deletion, the master entry
	// the project removed); Master the master's current version, if any.
	Entry  term.Entry `json:"entry"`
	Master *Stored    `json:"master,omitempty"`
	// Changes are what the project changed since its copy (all of the
	// entry for a new one); Conflicts where the master changed the same
	// places since then; Merged the three-way merge, the starting point
	// of a resolution.
	Changes   []term.Change   `json:"changes,omitempty"`
	Conflicts []term.Conflict `json:"conflicts,omitempty"`
	Merged    *term.Entry     `json:"merged,omitempty"`
	// Forbidden is set if the entry has a forbidden term; Duplicates lists
	// master entries that share a term with it (new entries).
	Forbidden  bool     `json:"forbidden,omitempty"`
	Duplicates []string `json:"duplicates,omitempty"`
	// Frequency is how often the entry's terms occur in the project's
	// documents, as the caller counted (ItemQuery.Frequency).
	Frequency int `json:"frequency,omitempty"`
	// Reviewers are those whose assigned scope includes the entry.
	Reviewers []string `json:"reviewers,omitempty"`
}

// Filter selects entries of a merge request. Empty fields do not
// restrict; a filter with several fields selects what matches all of
// them.
type Filter struct {
	// Langs: the entry has terms in one of these languages (or their
	// variants).
	Langs []string `json:"langs,omitempty"`
	// Fields: the entry has these field values (at any level; values
	// compared without case).
	Fields []term.Value `json:"fields,omitempty"`
	// Statuses: the entry has a term with one of these statuses.
	Statuses []string `json:"statuses,omitempty"`
	Authors  []string `json:"authors,omitempty"`
	Kinds    []string `json:"kinds,omitempty"`
	States   []string `json:"states,omitempty"`
	// Conflicts: only entries with (true) or without (false) conflicts.
	Conflicts *bool `json:"conflicts,omitempty"`
	// Since and Until bound the time of the project's change.
	Since time.Time `json:"since,omitzero"`
	Until time.Time `json:"until,omitzero"`
	// Text: a term contains this text (case aside).
	Text string `json:"text,omitempty"`
	// IDs selects items by ID.
	IDs []int64 `json:"ids,omitempty"`
}

func (f Filter) empty() bool {
	return len(f.Langs) == 0 && len(f.Fields) == 0 && len(f.Statuses) == 0 && len(f.Authors) == 0 && len(f.Kinds) == 0 &&
		len(f.States) == 0 && f.Conflicts == nil && f.Since.IsZero() && f.Until.IsZero() && f.Text == "" && len(f.IDs) == 0
}

func (f Filter) match(it *Item) bool {
	if len(f.IDs) > 0 && !containsID(f.IDs, it.ID) {
		return false
	}
	if len(f.Kinds) > 0 && !containsFold(f.Kinds, it.Kind) {
		return false
	}
	if len(f.States) > 0 && !containsFold(f.States, it.State) {
		return false
	}
	if len(f.Authors) > 0 && !containsFold(f.Authors, it.Author) {
		return false
	}
	if f.Conflicts != nil && (len(it.Conflicts) > 0) != *f.Conflicts {
		return false
	}
	if !f.Since.IsZero() && it.ChangedAt.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && it.ChangedAt.After(f.Until) {
		return false
	}
	e := it.Entry
	if len(f.Langs) > 0 {
		ok := false
		for _, l := range e.Languages {
			for _, want := range f.Langs {
				if len(l.Terms) > 0 && (strings.EqualFold(l.Lang, want) || strings.EqualFold(langPrimary(l.Lang), langPrimary(want))) {
					ok = true
				}
			}
		}
		if !ok {
			return false
		}
	}
	for _, fv := range f.Fields {
		if !hasValue(e, fv) {
			return false
		}
	}
	if len(f.Statuses) > 0 {
		ok := false
		for _, l := range e.Languages {
			for _, t := range l.Terms {
				if containsFold(f.Statuses, t.Status) {
					ok = true
				}
			}
		}
		if !ok {
			return false
		}
	}
	if f.Text != "" {
		needle := strings.ToLower(f.Text)
		ok := false
		for _, l := range e.Languages {
			for _, t := range l.Terms {
				if strings.Contains(strings.ToLower(t.Text), needle) {
					ok = true
				}
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func langPrimary(l string) string { return strings.SplitN(term.NormalizeLang(l), "-", 2)[0] }

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func hasValue(e term.Entry, want term.Value) bool {
	match := func(vs []term.Value) bool {
		for _, v := range vs {
			if strings.EqualFold(v.Field, want.Field) && (want.Value == "" || strings.EqualFold(v.Value, want.Value)) {
				return true
			}
		}
		return false
	}
	if match(e.Fields) {
		return true
	}
	for _, l := range e.Languages {
		if match(l.Fields) {
			return true
		}
		for _, t := range l.Terms {
			if match(t.Fields) {
				return true
			}
		}
	}
	return false
}

// ReviewRules are a master's rules for entries that need no review. The
// zero value merges entries identical to the master's and nothing else.
type ReviewRules struct {
	// KeepIdentical turns off merging, without review, of entries that
	// are already in the master as they are.
	KeepIdentical bool `json:"keepIdentical,omitempty"`
	// MinorFields are fields whose changes need no approval (notes,
	// sources): a change to these fields only is merged at once.
	MinorFields []string `json:"minorFields,omitempty"`
	// Trusted approves the entries of these authors in their scope
	// without review.
	Trusted []TrustedRule `json:"trusted,omitempty"`
}

// TrustedRule approves an author's entries within a scope.
type TrustedRule struct {
	Author string `json:"author"`
	Scope  Filter `json:"scope"`
}

// ReviewRules returns the master's review rules.
func (t *Termbase) ReviewRules(ctx context.Context) (ReviewRules, error) {
	return t.reviewRules(ctx, t.db.sql)
}

func (t *Termbase) reviewRules(ctx context.Context, q querier) (ReviewRules, error) {
	var js string
	if err := q.QueryRowContext(ctx, `SELECT rules_json FROM termbases WHERE id = ?`, t.id).Scan(&js); err != nil {
		return ReviewRules{}, err
	}
	var r ReviewRules
	if err := json.Unmarshal([]byte(js), &r); err != nil {
		return r, fmt.Errorf("termdb: read review rules: %w", err)
	}
	return r, nil
}

// SetReviewRules replaces the master's review rules.
func (t *Termbase) SetReviewRules(ctx context.Context, r ReviewRules, by string) error {
	info, err := t.Info(ctx)
	if err != nil {
		return err
	}
	if info.Kind != kindMaster {
		return errors.New("termdb: review rules belong to master termbases")
	}
	for _, tr := range r.Trusted {
		if strings.TrimSpace(tr.Author) == "" {
			return errors.New("termdb: a trusted rule needs an author")
		}
	}
	js, err := json.Marshal(r)
	if err != nil {
		return err
	}
	t.db.wmu.Lock()
	defer t.db.wmu.Unlock()
	tx, err := t.db.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err := tx.ExecContext(ctx, `UPDATE termbases SET rules_json = ? WHERE id = ?`, string(js), t.id); err != nil {
		return err
	}
	if err := logChange(ctx, tx, t.id, "", "", 0, ChangeTermbaseChanged, by, "Changed the review rules", t.db.now()); err != nil {
		return err
	}
	return tx.Commit()
}

// versionAt returns an entry as it was at a version.
func (t *Termbase) versionAt(ctx context.Context, q querier, entryID string, version int) (term.Entry, error) {
	var js string
	err := q.QueryRowContext(ctx, `SELECT v.entry_json FROM versions v JOIN entries e ON e.id = v.entry_row WHERE e.termbase_id = ? AND e.entry_id = ? AND v.version = ?`,
		t.id, entryID, version).Scan(&js)
	if errors.Is(err, sql.ErrNoRows) {
		// A deleted entry keeps its last state in deleted.
		err = q.QueryRowContext(ctx, `SELECT entry_json FROM deleted WHERE termbase_id = ? AND entry_id = ? ORDER BY `+deletedOrder(t.db.d)+` DESC LIMIT 1`, t.id, entryID).Scan(&js)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return term.Entry{}, ErrNotFound
	}
	if err != nil {
		return term.Entry{}, err
	}
	var e term.Entry
	if err := json.Unmarshal([]byte(js), &e); err != nil {
		return e, err
	}
	return e, nil
}

// OpenMergeRequest collects what the project added, changed and removed
// for the master since the last merges, and opens a request for it.
// Entries already in another open request of the project are left out.
// The master's review rules decide what needs no review, at once.
func (t *Termbase) OpenMergeRequest(ctx context.Context, master, title, by string, deadline time.Time) (*MergeRequest, error) {
	t.db.wmu.Lock()
	defer t.db.wmu.Unlock()
	tx, err := t.db.sql.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	ups, err := t.upstreams(ctx, tx)
	if err != nil {
		return nil, err
	}
	var m *Termbase
	for _, u := range ups {
		if strings.EqualFold(u.name, strings.TrimSpace(master)) {
			m = u
		}
	}
	if m == nil {
		return nil, fmt.Errorf("termdb: %s is not layered over %q", t.name, master)
	}
	if strings.TrimSpace(title) == "" {
		title = "Changes from " + t.name
	}
	now := t.db.now()
	dl := ""
	if !deadline.IsZero() {
		dl = deadline.UTC().Format(timeLayout)
	}
	var reqID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO merge_requests(project_id, target_id, title, created_at, created_by, deadline) VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		t.id, m.id, strings.TrimSpace(title), now.Format(timeLayout), by, dl).Scan(&reqID); err != nil {
		return nil, err
	}
	// Entries in an open request already, and the last decision per
	// entry, so rejected or sent-back entries return only once changed.
	busy := map[string]bool{}
	lastDecided := map[string]int{}
	rows, err := tx.QueryContext(ctx, `SELECT i.entry_id, i.kind, i.project_version, i.state, r.state FROM merge_items i JOIN merge_requests r ON r.id = i.request_id
		WHERE r.project_id = ? AND r.target_id = ? AND r.id <> ?`, t.id, m.id, reqID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, kind, state, rstate string
		var v int
		if err := rows.Scan(&id, &kind, &v, &state, &rstate); err != nil {
			rows.Close()
			return nil, err
		}
		key := kind + "/" + id
		if rstate == "open" && (state == StateWaiting || state == StatePostponed) {
			busy[key] = true
		}
		if state == StateRejected || state == StateChanges {
			if v > lastDecided[key] {
				lastDecided[key] = v
			}
		}
	}
	rows.Close()
	type cand struct {
		kind, entryID, author string
		version, baseVersion  int
		baseEntry             string
		changedAt             string
	}
	var cands []cand
	rows, err = tx.QueryContext(ctx, `SELECT entry_id, version, changed_by, changed_at, base_termbase, base_entry, base_version, base_synced FROM entries WHERE termbase_id = ? ORDER BY id`, t.id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c cand
		var btb, bv, bs sql.NullInt64
		var be sql.NullString
		if err := rows.Scan(&c.entryID, &c.version, &c.author, &c.changedAt, &btb, &be, &bv, &bs); err != nil {
			rows.Close()
			return nil, err
		}
		switch {
		case !btb.Valid:
			c.kind = ItemNew
		case btb.Int64 == m.id && int(bs.Int64) < c.version:
			c.kind, c.baseEntry, c.baseVersion = ItemChanged, be.String, int(bv.Int64)
		default:
			continue // in step with its master, or another master's entry
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT base_entry, base_version, author, at FROM project_removals WHERE project_id = ? AND base_termbase = ? ORDER BY at`, t.id, m.id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		c := cand{kind: ItemDeleted}
		if err := rows.Scan(&c.entryID, &c.baseVersion, &c.author, &c.changedAt); err != nil {
			rows.Close()
			return nil, err
		}
		c.baseEntry, c.version = c.entryID, c.baseVersion
		cands = append(cands, c)
	}
	rows.Close()
	for _, c := range cands {
		key := c.kind + "/" + c.entryID
		if busy[key] || (lastDecided[key] > 0 && c.version <= lastDecided[key]) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO merge_items(request_id, entry_id, project_version, kind, base_entry, base_version, author, changed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			reqID, c.entryID, c.version, c.kind, c.baseEntry, c.baseVersion, c.author, c.changedAt); err != nil {
			return nil, err
		}
	}
	mr := &MergeRequest{ID: reqID, Project: t.name, Target: m.name, Title: strings.TrimSpace(title), CreatedAt: now, CreatedBy: by, Deadline: deadline, State: "open", db: t.db, projectID: t.id, targetID: m.id}
	if err := mr.applyRules(ctx, tx, now); err != nil {
		return nil, err
	}
	if err := logChange(ctx, tx, t.id, "", "", 0, ChangeTermbaseChanged, by, fmt.Sprintf("Opened merge request %d into %s", reqID, m.name), now); err != nil {
		return nil, err
	}
	return mr, tx.Commit()
}

// MergeRequests lists a project's requests, or a master's incoming ones,
// newest first.
func (t *Termbase) MergeRequests(ctx context.Context) ([]*MergeRequest, error) {
	rows, err := t.db.sql.QueryContext(ctx, `SELECT id FROM merge_requests WHERE project_id = ? OR target_id = ? ORDER BY id DESC`, t.id, t.id)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []*MergeRequest
	for _, id := range ids {
		mr, err := t.db.MergeRequest(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, mr)
	}
	return out, nil
}

// MergeRequest returns a request by ID.
func (d *DB) MergeRequest(ctx context.Context, id int64) (*MergeRequest, error) {
	return d.mergeRequest(ctx, d.sql, id)
}

func (d *DB) mergeRequest(ctx context.Context, q querier, id int64) (*MergeRequest, error) {
	mr := &MergeRequest{db: d, ID: id}
	var created, deadline, closed string
	err := q.QueryRowContext(ctx, `SELECT r.project_id, r.target_id, p.name, m.name, r.title, r.created_at, r.created_by, r.deadline, r.state, r.closed_at
		FROM merge_requests r JOIN termbases p ON p.id = r.project_id JOIN termbases m ON m.id = r.target_id WHERE r.id = ?`, id).
		Scan(&mr.projectID, &mr.targetID, &mr.Project, &mr.Target, &mr.Title, &created, &mr.CreatedBy, &deadline, &mr.State, &closed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	mr.CreatedAt, _ = time.Parse(timeLayout, created)
	if deadline != "" {
		mr.Deadline, _ = time.Parse(timeLayout, deadline)
	}
	if closed != "" {
		mr.ClosedAt, _ = time.Parse(timeLayout, closed)
	}
	return mr, nil
}

func (mr *MergeRequest) project() *Termbase {
	return &Termbase{db: mr.db, id: mr.projectID, name: mr.Project}
}
func (mr *MergeRequest) target() *Termbase {
	return &Termbase{db: mr.db, id: mr.targetID, name: mr.Target}
}

// loadItems reads the request's items with everything a reviewer needs.
func (mr *MergeRequest) loadItems(ctx context.Context, q querier) ([]*Item, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, kind, entry_id, project_version, base_entry, base_version, author, changed_at, state, reviewer, decided_at, comment, rule, result_entry, result_version
		FROM merge_items WHERE request_id = ? ORDER BY id`, mr.ID)
	if err != nil {
		return nil, err
	}
	var items []*Item
	for rows.Next() {
		it := &Item{}
		var be, changed, decided, re string
		var bv, rv int
		if err := rows.Scan(&it.ID, &it.Kind, &it.EntryID, &it.ProjectVersion, &be, &bv, &it.Author, &changed, &it.State, &it.Reviewer, &decided, &it.Comment, &it.Rule, &re, &rv); err != nil {
			rows.Close()
			return nil, err
		}
		if be != "" {
			it.Base = &BaseRef{Termbase: mr.Target, EntryID: be, Version: bv}
		}
		if re != "" {
			it.Result = &BaseRef{Termbase: mr.Target, EntryID: re, Version: rv}
		}
		it.ChangedAt, _ = time.Parse(timeLayout, changed)
		if decided != "" {
			it.DecidedAt, _ = time.Parse(timeLayout, decided)
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	p, m := mr.project(), mr.target()
	// Master terms, for duplicate detection of new entries.
	masterTerms := map[string][]string{}
	mrows, err := q.QueryContext(ctx, `SELECT x.primary_lang, x.key, e.entry_id FROM terms x JOIN entries e ON e.id = x.entry_row WHERE x.termbase_id = ?`, mr.targetID)
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var lang, key, id string
		if err := mrows.Scan(&lang, &key, &id); err != nil {
			mrows.Close()
			return nil, err
		}
		k := lang + "\x00" + key
		masterTerms[k] = append(masterTerms[k], id)
	}
	mrows.Close()
	for _, it := range items {
		switch it.Kind {
		case ItemDeleted:
			e, err := m.versionAt(ctx, q, it.EntryID, it.Base.Version)
			if err != nil {
				return nil, err
			}
			it.Entry = e
		default:
			e, err := p.versionAt(ctx, q, it.EntryID, it.ProjectVersion)
			if err != nil {
				return nil, err
			}
			it.Entry = e
		}
		if it.Base != nil {
			if cur, _, err := m.get(ctx, q, it.Base.EntryID); err == nil {
				c := cur
				it.Master = &c
			} else if !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		}
		switch it.Kind {
		case ItemNew:
			it.Changes = term.Diff(term.Entry{}, it.Entry)
			seen := map[string]bool{}
			for _, l := range it.Entry.Languages {
				for _, tm := range l.Terms {
					for _, id := range masterTerms[langPrimary(l.Lang)+"\x00"+term.Key(tm.Text)] {
						if !seen[id] {
							seen[id] = true
							it.Duplicates = append(it.Duplicates, id)
						}
					}
				}
			}
		case ItemChanged:
			base, err := m.versionAt(ctx, q, it.Base.EntryID, it.Base.Version)
			if err != nil {
				return nil, err
			}
			it.Changes = term.Diff(base, it.Entry)
			if it.Master == nil {
				it.Conflicts = []term.Conflict{{Path: "entry", Base: "present", Ours: "changed", Theirs: "deleted"}}
			} else if it.Master.Version != it.Base.Version {
				merged, cs := term.Merge3(base, it.Entry, it.Master.Entry)
				it.Conflicts = cs
				it.Merged = &merged
			}
		case ItemDeleted:
			if it.Master == nil {
				it.Changes = nil // already deleted in the master
			} else {
				it.Changes = term.Diff(it.Entry, term.Entry{})
				if it.Master.Version != it.Base.Version {
					it.Conflicts = []term.Conflict{{Path: "entry", Base: "present", Ours: "deleted", Theirs: "changed"}}
				}
			}
		}
		for _, l := range it.Entry.Languages {
			for _, tm := range l.Terms {
				if tm.Status == term.StatusForbidden {
					it.Forbidden = true
				}
			}
		}
	}
	// Assigned reviewers.
	assign, err := mr.assignments(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		for _, a := range assign {
			if a.Scope.match(it) {
				it.Reviewers = append(it.Reviewers, a.Reviewer)
			}
		}
	}
	return items, nil
}

// Assignment is a reviewer's part of a request.
type Assignment struct {
	Reviewer string `json:"reviewer"`
	Scope    Filter `json:"scope"`
}

func (mr *MergeRequest) assignments(ctx context.Context, q querier) ([]Assignment, error) {
	rows, err := q.QueryContext(ctx, `SELECT reviewer, filter_json FROM merge_assignments WHERE request_id = ? ORDER BY reviewer`, mr.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Assignment
	for rows.Next() {
		var a Assignment
		var js string
		if err := rows.Scan(&a.Reviewer, &js); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(js), &a.Scope); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Assignments lists who reviews which part of the request.
func (mr *MergeRequest) Assignments(ctx context.Context) ([]Assignment, error) {
	return mr.assignments(ctx, mr.db.sql)
}

// Assign gives a reviewer a part of the request (an empty scope is all
// of it); assigning again replaces the reviewer's scope.
func (mr *MergeRequest) Assign(ctx context.Context, reviewer string, scope Filter) error {
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return errors.New("termdb: a reviewer needs a name")
	}
	js, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	mr.db.wmu.Lock()
	defer mr.db.wmu.Unlock()
	_, err = mr.db.sql.ExecContext(ctx, `INSERT INTO merge_assignments(request_id, reviewer, filter_json) VALUES (?, ?, ?)
		ON CONFLICT (request_id, reviewer) DO UPDATE SET filter_json = excluded.filter_json`, mr.ID, reviewer, string(js))
	return err
}

// Unassign removes a reviewer's part.
func (mr *MergeRequest) Unassign(ctx context.Context, reviewer string) error {
	mr.db.wmu.Lock()
	defer mr.db.wmu.Unlock()
	_, err := mr.db.sql.ExecContext(ctx, `DELETE FROM merge_assignments WHERE request_id = ? AND reviewer = ?`, mr.ID, strings.TrimSpace(reviewer))
	return err
}

// ItemQuery selects and orders items.
type ItemQuery struct {
	Filter Filter
	// Reviewer limits the items to the reviewer's assigned scope.
	Reviewer string
	// Sort is "priority" (default: conflicts, forbidden terms and
	// duplicates first, then the most frequent, then oldest), "date"
	// (oldest change first) or "author".
	Sort string
	// Frequency gives how often each project entry's terms occur in the
	// project's documents (entry ID to count), for the priority order.
	Frequency map[string]int
	Limit     int
	Offset    int
}

// Items returns the items that match, in order, and how many match in
// total.
func (mr *MergeRequest) Items(ctx context.Context, q ItemQuery) ([]Item, int, error) {
	items, err := mr.loadItems(ctx, mr.db.sql)
	if err != nil {
		return nil, 0, err
	}
	sel, err := mr.selectItems(ctx, items, q)
	if err != nil {
		return nil, 0, err
	}
	total := len(sel)
	if q.Offset > 0 {
		if q.Offset >= len(sel) {
			sel = nil
		} else {
			sel = sel[q.Offset:]
		}
	}
	if q.Limit > 0 && len(sel) > q.Limit {
		sel = sel[:q.Limit]
	}
	out := make([]Item, len(sel))
	for i, it := range sel {
		out[i] = *it
	}
	return out, total, nil
}

func (mr *MergeRequest) selectItems(ctx context.Context, items []*Item, q ItemQuery) ([]*Item, error) {
	var sel []*Item
	for _, it := range items {
		it.Frequency = q.Frequency[it.EntryID]
		if !q.Filter.match(it) {
			continue
		}
		if q.Reviewer != "" && !containsFold(it.Reviewers, q.Reviewer) {
			continue
		}
		sel = append(sel, it)
	}
	switch q.Sort {
	case "", "priority":
		rank := func(it *Item) int {
			switch {
			case len(it.Conflicts) > 0:
				return 0
			case it.Forbidden:
				return 1
			case len(it.Duplicates) > 0:
				return 2
			}
			return 3
		}
		sort.SliceStable(sel, func(i, j int) bool {
			a, b := sel[i], sel[j]
			if rank(a) != rank(b) {
				return rank(a) < rank(b)
			}
			if a.Frequency != b.Frequency {
				return a.Frequency > b.Frequency
			}
			return a.ChangedAt.Before(b.ChangedAt)
		})
	case "date":
		sort.SliceStable(sel, func(i, j int) bool { return sel[i].ChangedAt.Before(sel[j].ChangedAt) })
	case "author":
		sort.SliceStable(sel, func(i, j int) bool { return strings.ToLower(sel[i].Author) < strings.ToLower(sel[j].Author) })
	default:
		return nil, fmt.Errorf("termdb: unknown sort %q", q.Sort)
	}
	return sel, nil
}

// Sample returns up to n waiting items picked at random from those the
// filter selects, for a spot check of a large selection. The same seed
// gives the same sample.
func (mr *MergeRequest) Sample(ctx context.Context, f Filter, n int, seed int64) ([]Item, error) {
	f.States = []string{StateWaiting}
	items, _, err := mr.Items(ctx, ItemQuery{Filter: f, Sort: "date"})
	if err != nil {
		return nil, err
	}
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	if n < len(items) {
		items = items[:n]
	}
	return items, nil
}

// Progress counts the request's items per state, in total and per
// assigned reviewer.
type Progress struct {
	Total      int                       `json:"total"`
	States     map[string]int            `json:"states"`
	Conflicts  int                       `json:"conflicts"`
	ByReviewer map[string]map[string]int `json:"byReviewer,omitempty"`
}

// Progress reports how far the review is.
func (mr *MergeRequest) Progress(ctx context.Context) (Progress, error) {
	items, err := mr.loadItems(ctx, mr.db.sql)
	if err != nil {
		return Progress{}, err
	}
	p := Progress{States: map[string]int{}, ByReviewer: map[string]map[string]int{}}
	for _, it := range items {
		p.Total++
		p.States[it.State]++
		if len(it.Conflicts) > 0 && it.State == StateWaiting {
			p.Conflicts++
		}
		for _, r := range it.Reviewers {
			if p.ByReviewer[r] == nil {
				p.ByReviewer[r] = map[string]int{}
			}
			p.ByReviewer[r][it.State]++
		}
	}
	return p, nil
}

// Decision is what a reviewer decides.
type Decision struct {
	// State is StateApproved, StateEdited, StateChanges, StateRejected or
	// StatePostponed.
	State   string `json:"state"`
	Comment string `json:"comment,omitempty"`
	// Edited is the entry to merge instead of the project's (with
	// StateEdited), for example the resolution of a conflict. Only for a
	// single item.
	Edited *term.Entry `json:"edited,omitempty"`
}

// DecideResult says what a decision did.
type DecideResult struct {
	Decided int `json:"decided"`
	// NeedResolution are the items left waiting because the master
	// changed the same places; decide them one by one with an edited
	// entry.
	NeedResolution []int64 `json:"needResolution,omitempty"`
}

// Decide applies a decision to the items the filter selects (waiting and
// postponed ones; use Filter.IDs for single items). Approvals merge into
// the master in the same transaction; items whose merge has conflicts
// are left waiting and listed. Nothing is written if the request is
// closed.
func (mr *MergeRequest) Decide(ctx context.Context, f Filter, d Decision, by string) (DecideResult, error) {
	switch d.State {
	case StateApproved, StateEdited, StateChanges, StateRejected, StatePostponed:
	default:
		return DecideResult{}, fmt.Errorf("termdb: %q is not a decision", d.State)
	}
	if d.State == StateEdited && d.Edited == nil {
		return DecideResult{}, errors.New("termdb: an approval with edits needs the edited entry")
	}
	if d.State == StateChanges && strings.TrimSpace(d.Comment) == "" {
		return DecideResult{}, errors.New("termdb: a request for changes needs a comment")
	}
	mr.db.wmu.Lock()
	defer mr.db.wmu.Unlock()
	tx, err := mr.db.sql.Begin(ctx)
	if err != nil {
		return DecideResult{}, err
	}
	defer rollback(tx)
	cur, err := mr.db.mergeRequest(ctx, tx, mr.ID)
	if err != nil {
		return DecideResult{}, err
	}
	if cur.State != "open" {
		return DecideResult{}, ErrClosed
	}
	items, err := mr.loadItems(ctx, tx)
	if err != nil {
		return DecideResult{}, err
	}
	if len(f.States) == 0 && len(f.IDs) == 0 {
		f.States = []string{StateWaiting, StatePostponed}
	}
	sel, err := mr.selectItems(ctx, items, ItemQuery{Filter: f, Sort: "date"})
	if err != nil {
		return DecideResult{}, err
	}
	if d.Edited != nil && len(sel) != 1 {
		return DecideResult{}, errors.New("termdb: an edited entry decides one item; select it by ID")
	}
	var res DecideResult
	now := mr.db.now()
	for _, it := range sel {
		if it.State != StateWaiting && it.State != StatePostponed {
			if len(f.IDs) > 0 {
				return DecideResult{}, fmt.Errorf("%w: item %d is %s", ErrDecided, it.ID, it.State)
			}
			continue
		}
		state := d.State
		if state == StateApproved || state == StateEdited {
			err := mr.apply(ctx, tx, it, d.Edited, by, now)
			if errors.Is(err, ErrNeedsResolution) {
				res.NeedResolution = append(res.NeedResolution, it.ID)
				continue
			}
			if err != nil {
				return DecideResult{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE merge_items SET state = ?, reviewer = ?, decided_at = ?, comment = ? WHERE id = ?`,
			state, by, now.Format(timeLayout), strings.TrimSpace(d.Comment), it.ID); err != nil {
			return DecideResult{}, err
		}
		res.Decided++
	}
	if len(f.IDs) == 1 && len(res.NeedResolution) == 1 {
		return res, ErrNeedsResolution
	}
	return res, tx.Commit()
}

// apply merges an approved item into the master and brings the project's
// copy in step with the result.
func (mr *MergeRequest) apply(ctx context.Context, tx *wtx, it *Item, edited *term.Entry, by string, now time.Time) error {
	p, m := mr.project(), mr.target()
	mdef, err := m.definition(ctx, tx)
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("Merged from %s (merge request %d)", mr.Project, mr.ID)
	var result Stored
	before := 0
	switch it.Kind {
	case ItemNew:
		e := it.Entry
		if edited != nil {
			e = *edited
		}
		e.ID = ""
		e.CreatedAt, e.CreatedBy = time.Time{}, ""
		pe, err := prepare(e, mdef)
		if err != nil {
			return err
		}
		if addDefinitionFor(&mdef, pe) {
			if err := writeDefinition(ctx, tx, m.id, mdef); err != nil {
				return err
			}
		}
		if result, err = m.insert(ctx, tx, pe, by, summary, now); err != nil {
			return err
		}
	case ItemChanged:
		if it.Master == nil {
			if edited == nil {
				return ErrNeedsResolution
			}
			// The master deleted the entry; the reviewer brings it back.
			pe, err := prepare(*edited, mdef)
			if err != nil {
				return err
			}
			pe.ID = it.Base.EntryID
			if result, err = m.insert(ctx, tx, pe, by, summary, now); err != nil {
				return err
			}
			break
		}
		before = it.Master.Version
		var e term.Entry
		switch {
		case edited != nil:
			e = *edited
		case it.Master.Version == it.Base.Version:
			e = it.Entry // fast-forward
		case len(it.Conflicts) == 0:
			e = *it.Merged
		default:
			return ErrNeedsResolution
		}
		e.ID = it.Master.ID
		pe, err := prepare(e, mdef)
		if err != nil {
			return err
		}
		if addDefinitionFor(&mdef, pe) {
			if err := writeDefinition(ctx, tx, m.id, mdef); err != nil {
				return err
			}
		}
		if result, err = m.update(ctx, tx, pe, it.Master.Version, by, summary, now); err != nil {
			return err
		}
	case ItemDeleted:
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_removals WHERE project_id = ? AND base_termbase = ? AND base_entry = ?`, p.id, m.id, it.EntryID); err != nil {
			return err
		}
		if it.Master == nil {
			return nil // already gone
		}
		if it.Master.Version != it.Base.Version && edited == nil {
			return ErrNeedsResolution
		}
		before = it.Master.Version
		if err := m.deleteEntry(ctx, tx, *it.Master, by, summary, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE merge_items SET result_entry = ?, result_version = 0, master_before = ? WHERE id = ?`, it.EntryID, before, it.ID)
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE merge_items SET result_entry = ?, result_version = ?, master_before = ? WHERE id = ?`, result.ID, result.Version, before, it.ID); err != nil {
		return err
	}
	// The project's copy now follows the merged master entry. If the
	// project changed it again during the review, those later changes
	// stay pending for the next request.
	pcur, _, err := p.get(ctx, tx, it.EntryID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	synced := pcur.Version
	if pcur.Version == it.ProjectVersion && !sameContent(pcur.Entry, result.Entry) {
		e := result.Entry
		e.ID = pcur.ID
		up, err := p.update(ctx, tx, e, pcur.Version, by, "Brought in step with "+mr.Target+" after the merge", now)
		if err != nil {
			return err
		}
		synced = up.Version
	} else if pcur.Version != it.ProjectVersion {
		synced = it.ProjectVersion
	}
	return setBase(ctx, tx, p.id, pcur.ID, synced, m.id, result.ID, result.Version)
}

func sameContent(a, b term.Entry) bool {
	return len(term.Diff(a, b)) == 0
}

// applyRules decides, when a request opens, the items the master's rules
// say need no review.
func (mr *MergeRequest) applyRules(ctx context.Context, tx *wtx, now time.Time) error {
	rules, err := mr.target().reviewRules(ctx, tx)
	if err != nil {
		return err
	}
	items, err := mr.loadItems(ctx, tx)
	if err != nil {
		return err
	}
	decide := func(it *Item, rule string) error {
		if err := mr.apply(ctx, tx, it, nil, "", now); err != nil {
			if errors.Is(err, ErrNeedsResolution) {
				return nil
			}
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE merge_items SET state = ?, decided_at = ?, rule = ? WHERE id = ?`, StateApproved, now.Format(timeLayout), rule, it.ID)
		return err
	}
	for _, it := range items {
		if len(it.Conflicts) > 0 {
			continue
		}
		switch {
		case !rules.KeepIdentical && it.Kind == ItemChanged && it.Master != nil && sameContent(it.Entry, it.Master.Entry):
			if err := decide(it, "identical"); err != nil {
				return err
			}
		case !rules.KeepIdentical && it.Kind == ItemNew && len(it.Duplicates) == 1:
			// A new entry the master already has as it is: link the
			// project's entry to it instead of adding a copy.
			m := mr.target()
			ms, _, err := m.get(ctx, tx, it.Duplicates[0])
			if err != nil {
				return err
			}
			if !sameContent(it.Entry, ms.Entry) {
				continue
			}
			if err := setBase(ctx, tx, mr.projectID, it.EntryID, it.ProjectVersion, mr.targetID, ms.ID, ms.Version); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE merge_items SET state = ?, decided_at = ?, rule = ?, result_entry = ?, result_version = ? WHERE id = ?`,
				StateApproved, now.Format(timeLayout), "identical", ms.ID, ms.Version, it.ID); err != nil {
				return err
			}
		case len(rules.MinorFields) > 0 && it.Kind == ItemChanged && onlyFields(it.Changes, rules.MinorFields):
			if err := decide(it, "minor-fields"); err != nil {
				return err
			}
		default:
			for _, tr := range rules.Trusted {
				if strings.EqualFold(tr.Author, it.Author) && (tr.Scope.empty() || tr.Scope.match(it)) {
					if err := decide(it, "trusted:"+tr.Author); err != nil {
						return err
					}
					break
				}
			}
		}
	}
	return nil
}

// onlyFields reports whether every change is to one of the fields.
func onlyFields(cs []term.Change, fields []string) bool {
	if len(cs) == 0 {
		return false
	}
	for _, c := range cs {
		ok := false
		for _, f := range fields {
			if strings.HasSuffix(strings.ToLower(c.Path), " field "+strings.ToLower(f)) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Close closes the request; items not decided return in the next one.
func (mr *MergeRequest) Close(ctx context.Context, by string) error {
	mr.db.wmu.Lock()
	defer mr.db.wmu.Unlock()
	tx, err := mr.db.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	now := mr.db.now()
	res, err := tx.ExecContext(ctx, `UPDATE merge_requests SET state = 'closed', closed_at = ? WHERE id = ? AND state = 'open'`, now.Format(timeLayout), mr.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrClosed
	}
	if err := logChange(ctx, tx, mr.projectID, "", "", 0, ChangeTermbaseChanged, by, fmt.Sprintf("Closed merge request %d", mr.ID), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	mr.State, mr.ClosedAt = "closed", now
	return nil
}

// Revert undoes an approved item: the master entry goes back to the
// version before the merge (or is deleted again, or restored for an
// approved deletion), as a new change. If the master entry changed after
// the merge, ErrConflict is returned and nothing is written.
func (mr *MergeRequest) Revert(ctx context.Context, itemID int64, by string) error {
	mr.db.wmu.Lock()
	defer mr.db.wmu.Unlock()
	tx, err := mr.db.sql.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var kind, state, re, rule string
	var rv, before int
	err = tx.QueryRowContext(ctx, `SELECT kind, state, result_entry, result_version, master_before, rule FROM merge_items WHERE id = ? AND request_id = ?`, itemID, mr.ID).
		Scan(&kind, &state, &re, &rv, &before, &rule)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != StateApproved && state != StateEdited {
		return fmt.Errorf("termdb: item %d is %s, not approved", itemID, state)
	}
	if rule == "identical" {
		return fmt.Errorf("termdb: item %d was already in %s as it is; nothing was merged", itemID, mr.Target)
	}
	m := mr.target()
	now := mr.db.now()
	summary := fmt.Sprintf("Reverted the merge of merge request %d", mr.ID)
	switch {
	case kind == ItemDeleted:
		e, err := m.versionAt(ctx, tx, re, before)
		if err != nil {
			return err
		}
		e.ID = re
		if _, err := m.insert(ctx, tx, e, by, summary, now); err != nil {
			return err
		}
	case before == 0:
		cur, _, err := m.get(ctx, tx, re)
		if err != nil {
			return err
		}
		if cur.Version != rv {
			return ErrConflict
		}
		if err := m.deleteEntry(ctx, tx, cur, by, summary, now); err != nil {
			return err
		}
	default:
		cur, _, err := m.get(ctx, tx, re)
		if err != nil {
			return err
		}
		if cur.Version != rv {
			return ErrConflict
		}
		old, err := m.versionAt(ctx, tx, re, before)
		if err != nil {
			return err
		}
		old.ID = re
		if _, err := m.update(ctx, tx, old, cur.Version, by, summary, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE merge_items SET state = ?, reviewer = ?, decided_at = ? WHERE id = ?`, StateReverted, by, now.Format(timeLayout), itemID); err != nil {
		return err
	}
	return tx.Commit()
}

// Revert sets an entry back to an earlier version, as a new version.
func (t *Termbase) Revert(ctx context.Context, entryID string, toVersion int, by string) (Stored, error) {
	var out Stored
	err := t.write(ctx, by, func(tx *wtx, def *term.Definition) error {
		cur, _, err := t.get(ctx, tx, entryID)
		if err != nil {
			return err
		}
		old, err := t.versionAt(ctx, tx, cur.ID, toVersion)
		if err != nil {
			return err
		}
		old.ID = cur.ID
		p, err := prepare(old, *def)
		if err != nil {
			return err
		}
		addDefinitionFor(def, p)
		out, err = t.update(ctx, tx, p, cur.Version, by, fmt.Sprintf("Reverted to version %d", toVersion), t.db.now())
		return err
	})
	return out, err
}
