package projectdb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/internal/sqlstore"
	"github.com/4nonX/interlinea/project"
)

// How a file's segments are cut differently.
const (
	// ResegmentSplit cuts a segment in two.
	ResegmentSplit = "split"
	// ResegmentJoin makes one segment of a segment and the one after it.
	ResegmentJoin = "join"
	// ResegmentJoinInParagraph joins only segments of one paragraph, for
	// files whose paragraphs the program they go back to decides (XLIFF).
	ResegmentJoinInParagraph = "join-in-paragraph"
)

// Resegment splits a segment where before, the start of its source, ends,
// or joins it with the segment after it, in one target language of a
// file and as seen at version. Other segments of the same stretch of the
// file may get another name with it. What the
// segments do not allow comes back as a *project.ErrResegment. It
// returns the file's segments as they are afterwards. Assignments that
// began or ended at a segment that is gone begin or end at what took its
// place.
func (d *DB) Resegment(ctx context.Context, projectID int64, file, target, segID, how string, before content.Content, version int, by string) ([]StoredSegment, error) {
	target = normLang(target)
	var out []StoredSegment
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		cur, err := loadProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		fid, err := fileID(ctx, tx, projectID, file)
		if err != nil {
			return err
		}
		stored, err := querySegments(ctx, tx, fid, target)
		if err != nil {
			return err
		}
		at := -1
		segs := make([]project.Segment, len(stored))
		for i, s := range stored {
			segs[i] = s.Segment
			if s.ID == segID {
				at = i
			}
		}
		if at < 0 {
			return ErrNotFound
		}
		if stored[at].Version != version {
			return ErrConflict
		}
		var changed []project.Segment
		switch how {
		case ResegmentSplit:
			changed, err = project.Split(segs, at, before)
		case ResegmentJoin:
			changed, err = project.Join(segs, at)
		case ResegmentJoinInParagraph:
			changed, err = project.JoinInParagraph(segs, at)
		default:
			return fmt.Errorf("projectdb: unknown way to resegment %q", how)
		}
		if err != nil {
			return err
		}
		// What is different: segs[lo:hiOld] became changed[lo:hiNew].
		same := func(a, b project.Segment) bool {
			x, _ := json.Marshal(a)
			y, _ := json.Marshal(b)
			return string(x) == string(y)
		}
		lo := 0
		for lo < len(segs) && lo < len(changed) && same(segs[lo], changed[lo]) {
			lo++
		}
		hiOld, hiNew := len(segs), len(changed)
		for hiOld > lo && hiNew > lo && same(segs[hiOld-1], changed[hiNew-1]) {
			hiOld--
			hiNew--
		}
		if lo == hiOld && lo == hiNew {
			out = stored
			return nil
		}
		gone, come := stored[lo:hiOld], changed[lo:hiNew]
		next := version
		for _, s := range gone {
			if s.Version > next {
				next = s.Version
			}
		}
		next++
		pos := 0
		if lo < len(stored) {
			if pos, err = position(ctx, tx, fid, target, stored[lo].ID); err != nil {
				return err
			}
		}
		for _, s := range gone {
			if _, err := tx.ExecContext(ctx, `DELETE FROM segments WHERE file_id = ? AND target_lang = ? AND seg_id = ?`, fid, target, s.ID); err != nil {
				return err
			}
		}
		if len(come) != len(gone) {
			if _, err := tx.ExecContext(ctx, `UPDATE segments SET position = position + ? WHERE file_id = ? AND target_lang = ? AND position > ?`, len(come)-len(gone), fid, target, pos); err != nil {
				return err
			}
		}
		st, err := tx.PrepareContext(ctx, `INSERT INTO segments(file_id, target_lang, position, seg_id, source_json, target_json, status, locked, version, changed_at, changed_by, extra_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for k, sg := range come {
			src, err := json.Marshal(sg.Source)
			if err != nil {
				return err
			}
			tgt, err := json.Marshal(sg.Target)
			if err != nil {
				return err
			}
			extra, err := extraJSON(sg)
			if err != nil {
				return err
			}
			if _, err := st.ExecContext(ctx, fid, target, pos+k, sg.ID, noNUL(string(src)), noNUL(string(tgt)), string(sg.Status), boolInt(sg.Locked), next, now.Format(timeLayout), noNUL(by), extra); err != nil {
				return fmt.Errorf("projectdb: segment %s: %w", sg.ID, err)
			}
		}
		// Work given out from or up to a segment that is another one now
		// is given out from the segment that has its first words, or up
		// to the one that has its last. A
		// range given out for every language at once is given out for
		// each language first, to the same person, as its ends are no
		// longer the same in all.
		letters := func(c content.Content) int {
			n := 0
			for _, r := range c.Plain() {
				if !unicode.IsSpace(r) {
					n++
				}
			}
			return n
		}
		ends := make([]int, len(come))
		total := 0
		for k, sg := range come {
			total += letters(sg.Source)
			ends[k] = total
		}
		holder := func(at int) string {
			for k := range come {
				if at < ends[k] {
					return come[k].ID
				}
			}
			return come[len(come)-1].ID
		}
		off := 0
		for _, s := range gone {
			start := off
			off += letters(s.Source)
			if len(come) == 0 {
				continue
			}
			first, last := holder(start), holder(off-1)
			if first == s.ID && last == s.ID {
				continue
			}
			if err := perLanguage(ctx, tx, cur, projectID, file, s.ID, by, now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE assignments SET from_seg = ? WHERE project_id = ? AND lower(file) = lower(?) AND target_lang = ? AND from_seg = ?`, first, projectID, file, target, s.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE assignments SET to_seg = ? WHERE project_id = ? AND lower(file) = lower(?) AND target_lang = ? AND to_seg = ?`, last, projectID, file, target, s.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM presence WHERE project_id = ? AND lower(file) = lower(?) AND target_lang = ? AND seg_id NOT IN (SELECT seg_id FROM segments WHERE file_id = ? AND target_lang = ?)`, projectID, file, target, fid, target); err != nil {
			return err
		}
		if err := logChange(ctx, tx, change{projectID: projectID, project: cur.Settings.Name, kind: ChangeResegmented, file: file, target: target, segment: come[0].ID, version: next}, by, now); err != nil {
			return err
		}
		out, err = querySegments(ctx, tx, fid, target)
		return err
	})
	return out, err
}

// perLanguage gives out for each language the ranges of a file that are
// given out for every language at once and begin or end at a segment.
func perLanguage(ctx context.Context, tx *sqlstore.Tx, p Project, projectID int64, file, segID, by string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, from_seg, to_seg, user_id, user_name, task, due, done FROM assignments
		WHERE project_id = ? AND lower(file) = lower(?) AND target_lang = '' AND (from_seg = ? OR to_seg = ?)`,
		projectID, file, segID, segID)
	if err != nil {
		return err
	}
	type row struct {
		id, userID           int64
		from, to, user, task string
		due                  string
		done                 int
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.from, &r.to, &r.userID, &r.user, &r.task, &r.due, &r.done); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(all) == 0 {
		return nil
	}
	var langs []string
	for _, f := range p.Settings.Files {
		if strings.EqualFold(f.Name, file) {
			langs = targetsOf(f, p.Settings)
		}
	}
	for _, r := range all {
		if _, err := tx.ExecContext(ctx, `DELETE FROM assignments WHERE id = ?`, r.id); err != nil {
			return err
		}
		for _, l := range langs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO assignments(project_id, file, target_lang, from_seg, to_seg, user_id, user_name, task, due, done, version, changed_at, changed_by)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
				projectID, file, l, r.from, r.to, r.userID, r.user, r.task, r.due, r.done, now.Format(timeLayout), noNUL(by)); err != nil {
				return err
			}
		}
	}
	return logChange(ctx, tx, change{projectID: projectID, project: p.Settings.Name, kind: ChangeAssignment, file: file, version: p.Version}, by, now)
}
