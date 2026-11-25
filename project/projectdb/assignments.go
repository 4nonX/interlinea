package projectdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/4nonX/interlinea/internal/sqlstore"
	"github.com/4nonX/interlinea/project"
)

// Task is what an assignment asks of someone.
type Task string

const (
	// TaskTranslate is translating (and editing) the segments.
	TaskTranslate Task = "translate"
	// TaskReview is reviewing them: checking, correcting and confirming.
	TaskReview Task = "review"
)

// Kinds of change for assignments in the change log. The change names
// the file and language; clients read the project's assignments again.
const (
	ChangeAssignment        = "assignment-set"
	ChangeAssignmentRemoved = "assignment-removed"
)

// Assignment gives a person part of a project: a target language of a
// file, all of a file's languages, or all files, and within one file a
// range of segments.
type Assignment struct {
	ID        int64 `json:"id"`
	ProjectID int64 `json:"projectId"`
	// File is the file's name; empty for every file to translate.
	File string `json:"file,omitempty"`
	// TargetLang is the language; empty for every target language.
	TargetLang string `json:"targetLang,omitempty"`
	// FromSegment and ToSegment bound a range of segments by their IDs,
	// both included; empty for the whole file. Only with a File.
	FromSegment string `json:"fromSegment,omitempty"`
	ToSegment   string `json:"toSegment,omitempty"`
	// UserID and UserName identify the person (an authdb account).
	UserID   int64  `json:"userId"`
	UserName string `json:"userName"`
	Task     Task   `json:"task"`
	// Due is the deadline, RFC 3339; empty for the project's.
	Due string `json:"due,omitempty"`
	// Done is set when the person reports the work finished.
	Done      bool      `json:"done"`
	Version   int       `json:"version"`
	ChangedAt time.Time `json:"changedAt"`
	ChangedBy string    `json:"changedBy"`
}

func dropWork(ctx context.Context, tx *sqlstore.Tx, projectID int64, where string, arg any) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM assignments WHERE project_id = ? AND `+where, projectID, arg); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM presence WHERE project_id = ? AND `+where, projectID, arg)
	return err
}

// span is an assignment resolved to positions, for overlap checks.
type span struct {
	file, target string
	from, to     int // segment positions, -1 for the whole file
}

func (a span) overlaps(b span) bool {
	if a.file != "" && b.file != "" && !strings.EqualFold(a.file, b.file) {
		return false
	}
	if a.target != "" && b.target != "" && a.target != b.target {
		return false
	}
	if a.from < 0 || b.from < 0 {
		return true
	}
	return a.from <= b.to && b.from <= a.to
}

// position is a segment's position in a file's language, or -1.
func position(ctx context.Context, tx queryRower, fileID int64, target, segID string) (int, error) {
	var pos int
	err := tx.QueryRowContext(ctx, `SELECT position FROM segments WHERE file_id = ? AND target_lang = ? AND seg_id = ?`, fileID, target, segID).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, nil
	}
	return pos, err
}

// resolve checks an assignment against its project and finds its span.
func resolve(ctx context.Context, tx queryRower, p Project, a Assignment) (Assignment, span, error) {
	bad := func(field, msg string) error {
		return &InvalidError{Problems: []project.Problem{{Field: field, Message: msg}}}
	}
	a.File = strings.TrimSpace(a.File)
	a.UserName = noNUL(strings.TrimSpace(a.UserName))
	if a.TargetLang != "" {
		a.TargetLang = normLang(a.TargetLang)
	}
	switch {
	case a.UserID <= 0:
		return a, span{}, bad("user", "Choose who the work is for.")
	case a.Task != TaskTranslate && a.Task != TaskReview:
		return a, span{}, bad("task", "The task is translating or reviewing.")
	}
	if a.Due != "" {
		if _, err := time.Parse(time.RFC3339, a.Due); err != nil {
			return a, span{}, bad("due", "The due date is not a valid date and time.")
		}
	}
	if a.TargetLang != "" && !contains(p.Settings.TargetLangs, a.TargetLang) {
		return a, span{}, bad("targetLang", fmt.Sprintf("%s is not a target language of the project.", a.TargetLang))
	}
	sp := span{file: a.File, target: a.TargetLang, from: -1, to: -1}
	if a.File == "" {
		if a.FromSegment != "" || a.ToSegment != "" {
			return a, span{}, bad("fromSegment", "A range of segments is within one file.")
		}
		return a, sp, nil
	}
	var f *project.File
	for i := range p.Settings.Files {
		if strings.EqualFold(p.Settings.Files[i].Name, a.File) {
			f = &p.Settings.Files[i]
		}
	}
	switch {
	case f == nil:
		return a, span{}, bad("file", fmt.Sprintf("The project has no file %s.", a.File))
	case f.Role != project.Translatable:
		return a, span{}, bad("file", fmt.Sprintf("%s is a reference file; there is nothing to translate in it.", f.Name))
	case a.TargetLang != "" && !contains(targetsOf(*f, p.Settings), a.TargetLang):
		return a, span{}, bad("targetLang", fmt.Sprintf("%s is not translated into %s.", f.Name, a.TargetLang))
	}
	a.File = f.Name
	sp.file = f.Name
	if (a.FromSegment == "") != (a.ToSegment == "") {
		return a, span{}, bad("toSegment", "A range has a first and a last segment.")
	}
	if a.FromSegment == "" {
		return a, sp, nil
	}
	fid, err := fileID(ctx, tx, p.ID, f.Name)
	if err != nil {
		return a, span{}, err
	}
	// Positions are the same in every language of a file; the first
	// language the file is translated into is measured.
	lang := a.TargetLang
	if lang == "" {
		lang = targetsOf(*f, p.Settings)[0]
	}
	from, err := position(ctx, tx, fid, lang, a.FromSegment)
	if err != nil {
		return a, span{}, err
	}
	to, err := position(ctx, tx, fid, lang, a.ToSegment)
	if err != nil {
		return a, span{}, err
	}
	switch {
	case from < 0:
		return a, span{}, bad("fromSegment", fmt.Sprintf("%s has no segment %s.", f.Name, a.FromSegment))
	case to < 0:
		return a, span{}, bad("toSegment", fmt.Sprintf("%s has no segment %s.", f.Name, a.ToSegment))
	case to < from:
		return a, span{}, bad("toSegment", "The last segment comes before the first.")
	}
	sp.from, sp.to = from, to
	return a, sp, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

const assignmentColumns = `id, project_id, file, target_lang, from_seg, to_seg, user_id, user_name, task, due, done, version, changed_at, changed_by`

func scanAssignments(rows *sql.Rows) ([]Assignment, error) {
	defer rows.Close()
	var out []Assignment
	for rows.Next() {
		var a Assignment
		var task, changed string
		var done int
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.File, &a.TargetLang, &a.FromSegment, &a.ToSegment, &a.UserID, &a.UserName,
			&task, &a.Due, &done, &a.Version, &changed, &a.ChangedBy); err != nil {
			return nil, err
		}
		a.Task, a.Done, a.ChangedAt = Task(task), done != 0, parseTime(changed)
		out = append(out, a)
	}
	return out, rows.Err()
}

func queryAssignments(ctx context.Context, q interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}, where string, args ...any) ([]Assignment, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+assignmentColumns+` FROM assignments WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	return scanAssignments(rows)
}

// checkOverlap refuses a second person for the same task on the same
// segments: each segment has one translator and one reviewer.
func checkOverlap(ctx context.Context, tx *sqlstore.Tx, p Project, a Assignment, sp span) error {
	others, err := queryAssignments(ctx, tx, `project_id = ? AND task = ? AND id <> ?`, p.ID, string(a.Task), a.ID)
	if err != nil {
		return err
	}
	for _, o := range others {
		_, osp, err := resolve(ctx, tx, p, o)
		if err != nil {
			// An assignment whose segments no longer exist overlaps nothing.
			continue
		}
		if sp.overlaps(osp) {
			what := "translating"
			if a.Task == TaskReview {
				what = "reviewing"
			}
			return &InvalidError{Problems: []project.Problem{{Field: "fromSegment",
				Message: fmt.Sprintf("%s is %s some of these segments already.", o.UserName, what)}}}
		}
	}
	return nil
}

// Assign gives a person part of a project. Each segment has at most one
// translator and one reviewer.
func (d *DB) Assign(ctx context.Context, a Assignment, by string) (Assignment, error) {
	var out Assignment
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		p, err := loadProject(ctx, tx, a.ProjectID)
		if err != nil {
			return err
		}
		a.ID = 0
		a, sp, err := resolve(ctx, tx, p, a)
		if err != nil {
			return err
		}
		if err := checkOverlap(ctx, tx, p, a, sp); err != nil {
			return err
		}
		at := now.Format(timeLayout)
		if err := tx.QueryRowContext(ctx, `INSERT INTO assignments(project_id, file, target_lang, from_seg, to_seg, user_id, user_name, task, due, done, version, changed_at, changed_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 1, ?, ?) RETURNING id`,
			p.ID, a.File, a.TargetLang, a.FromSegment, a.ToSegment, a.UserID, a.UserName, string(a.Task), a.Due, at, noNUL(by)).Scan(&a.ID); err != nil {
			return err
		}
		a.Done, a.Version, a.ChangedAt, a.ChangedBy = false, 1, now, by
		out = a
		return logChange(ctx, tx, change{projectID: p.ID, project: p.Settings.Name, kind: ChangeAssignment, file: a.File, target: a.TargetLang}, by, now)
	})
	return out, err
}

// UpdateAssignment changes an assignment as edited from version: who,
// what, due date, or done.
func (d *DB) UpdateAssignment(ctx context.Context, a Assignment, version int, by string) (Assignment, error) {
	var out Assignment
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := queryAssignments(ctx, tx, `id = ?`, a.ID)
		if err != nil {
			return err
		}
		if len(cur) == 0 {
			return ErrNotFound
		}
		if cur[0].Version != version {
			return ErrConflict
		}
		p, err := loadProject(ctx, tx, cur[0].ProjectID)
		if err != nil {
			return err
		}
		a.ProjectID = p.ID
		a, sp, err := resolve(ctx, tx, p, a)
		if err != nil {
			return err
		}
		if err := checkOverlap(ctx, tx, p, a, sp); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE assignments SET file = ?, target_lang = ?, from_seg = ?, to_seg = ?, user_id = ?, user_name = ?, task = ?, due = ?, done = ?,
			version = version + 1, changed_at = ?, changed_by = ? WHERE id = ? AND version = ?`,
			a.File, a.TargetLang, a.FromSegment, a.ToSegment, a.UserID, a.UserName, string(a.Task), a.Due, boolInt(a.Done),
			now.Format(timeLayout), noNUL(by), a.ID, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrConflict
		}
		a.Version, a.ChangedAt, a.ChangedBy = version+1, now, by
		out = a
		return logChange(ctx, tx, change{projectID: p.ID, project: p.Settings.Name, kind: ChangeAssignment, file: a.File, target: a.TargetLang}, by, now)
	})
	return out, err
}

// Unassign removes an assignment.
func (d *DB) Unassign(ctx context.Context, id int64, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := queryAssignments(ctx, tx, `id = ?`, id)
		if err != nil {
			return err
		}
		if len(cur) == 0 {
			return ErrNotFound
		}
		p, err := loadProject(ctx, tx, cur[0].ProjectID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM assignments WHERE id = ?`, id); err != nil {
			return err
		}
		return logChange(ctx, tx, change{projectID: p.ID, project: p.Settings.Name, kind: ChangeAssignmentRemoved, file: cur[0].File, target: cur[0].TargetLang}, by, now)
	})
}

// Assignments returns a project's assignments.
func (d *DB) Assignments(ctx context.Context, projectID int64) ([]Assignment, error) {
	return queryAssignments(ctx, d.sql, `project_id = ?`, projectID)
}

// AssignmentsOf returns a person's assignments in every project.
func (d *DB) AssignmentsOf(ctx context.Context, userID int64) ([]Assignment, error) {
	return queryAssignments(ctx, d.sql, `user_id = ?`, userID)
}

// AssignedTo reports whether a person is assigned task (or, for
// TaskTranslate, also TaskReview, since reviewers correct) on a segment.
func (d *DB) AssignedTo(ctx context.Context, projectID, userID int64, file, target, segID string, task Task) (bool, error) {
	target = normLang(target)
	p, err := loadProject(ctx, d.sql, projectID)
	if err != nil {
		return false, err
	}
	mine, err := queryAssignments(ctx, d.sql, `project_id = ? AND user_id = ?`, projectID, userID)
	if err != nil {
		return false, err
	}
	fid, err := fileID(ctx, d.sql, projectID, file)
	if err != nil {
		return false, err
	}
	pos, err := position(ctx, d.sql, fid, target, segID)
	if err != nil || pos < 0 {
		return false, err
	}
	seg := span{file: file, target: target, from: pos, to: pos}
	for _, a := range mine {
		if a.Task != task && !(task == TaskTranslate && a.Task == TaskReview) {
			continue
		}
		if a.TargetLang == "" && a.FromSegment != "" {
			// A range for every language is measured in the language asked
			// about, whose segments may be split or joined differently.
			a.TargetLang = target
		}
		_, sp, err := resolve(ctx, d.sql, p, a)
		if err != nil {
			continue
		}
		if sp.overlaps(seg) {
			return true, nil
		}
	}
	return false, nil
}

// Presence marks who has a segment open.
type Presence struct {
	File       string    `json:"file"`
	TargetLang string    `json:"targetLang"`
	Segment    string    `json:"segment"`
	UserID     int64     `json:"userId"`
	UserName   string    `json:"userName"`
	Since      time.Time `json:"since"`
	Expires    time.Time `json:"expires"`
}

// stamp formats presence times at a fixed width in UTC, so the database
// compares them as text in time order.
func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }

// PresenceTTL is how long a mark lasts without being renewed.
const PresenceTTL = 2 * time.Minute

// Touch marks that a person has a segment open, replacing the segment they
// had open in the project before. Editors renew it within PresenceTTL.
func (d *DB) Touch(ctx context.Context, projectID int64, file, target, segID string, userID int64, userName string) (Presence, error) {
	target = normLang(target)
	var out Presence
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		fid, err := fileID(ctx, tx, projectID, file)
		if err != nil {
			return err
		}
		pos, err := position(ctx, tx, fid, target, segID)
		if err != nil {
			return err
		}
		if pos < 0 {
			return ErrNotFound
		}
		var since string
		err = tx.QueryRowContext(ctx, `SELECT since FROM presence WHERE project_id = ? AND user_id = ? AND lower(file) = lower(?) AND target_lang = ? AND seg_id = ?`,
			projectID, userID, file, target, segID).Scan(&since)
		if errors.Is(err, sql.ErrNoRows) {
			since = stamp(now)
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM presence WHERE project_id = ? AND (user_id = ? OR expires <= ?)`, projectID, userID, stamp(now)); err != nil {
			return err
		}
		exp := now.Add(PresenceTTL)
		var real string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM files WHERE id = ?`, fid).Scan(&real); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO presence(project_id, file, target_lang, seg_id, user_id, user_name, since, expires) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			projectID, real, target, segID, userID, noNUL(userName), since, stamp(exp)); err != nil {
			return err
		}
		out = Presence{File: real, TargetLang: target, Segment: segID, UserID: userID, UserName: noNUL(userName), Since: parseTime(since), Expires: exp}
		return nil
	})
	return out, err
}

// Leave removes a person's mark in a project (they closed the editor).
func (d *DB) Leave(ctx context.Context, projectID, userID int64) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM presence WHERE project_id = ? AND user_id = ?`, projectID, userID)
		return err
	})
}

// Present returns the marks in a project that have not expired, for a
// file and language, or for the whole project when file is empty.
func (d *DB) Present(ctx context.Context, projectID int64, file, target string) ([]Presence, error) {
	q := `SELECT file, target_lang, seg_id, user_id, user_name, since, expires FROM presence WHERE project_id = ? AND expires > ?`
	args := []any{projectID, stamp(d.now())}
	if file != "" {
		q += ` AND lower(file) = lower(?) AND target_lang = ?`
		args = append(args, file, normLang(target))
	}
	rows, err := d.sql.QueryContext(ctx, q+` ORDER BY user_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Presence
	for rows.Next() {
		var p Presence
		var since, exp string
		if err := rows.Scan(&p.File, &p.TargetLang, &p.Segment, &p.UserID, &p.UserName, &since, &exp); err != nil {
			return nil, err
		}
		p.Since, p.Expires = parseTime(since), parseTime(exp)
		out = append(out, p)
	}
	return out, rows.Err()
}
