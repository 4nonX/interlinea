package termdb

import (
	"context"
	"strings"
	"time"
)

// Change is one entry of the change log: every created, changed and
// deleted entry and every change to a termbase itself, in the order they
// happened. Seq increases with every change across the whole database,
// so a client that has seen everything up to Seq asks for what came
// after it and is up to date.
type Change struct {
	Seq      int64     `json:"seq"`
	Termbase string    `json:"termbase"`
	EntryID  string    `json:"entryId,omitempty"`
	Version  int       `json:"version,omitempty"`
	Kind     string    `json:"kind"`
	At       time.Time `json:"at"`
	By       string    `json:"by"`
	Summary  string    `json:"summary"`
}

// ChangeQuery selects changes.
type ChangeQuery struct {
	// Since returns changes after this sequence number (0 for all).
	Since int64
	// Termbases limits the log to these termbases (by name); empty for
	// all.
	Termbases []string
	// Limit is the most changes returned (default 1000, at most 10000).
	Limit int
}

// Changes returns changes in order. More reports whether there are more
// after the last one returned; ask again with Since set to its Seq.
func (d *DB) Changes(ctx context.Context, q ChangeQuery) (changes []Change, more bool, err error) {
	if q.Limit <= 0 {
		q.Limit = 1000
	}
	if q.Limit > 10000 {
		q.Limit = 10000
	}
	query := `SELECT seq, termbase, entry_id, version, kind, at, by, summary FROM changes WHERE seq > ?`
	args := []any{q.Since}
	if len(q.Termbases) > 0 {
		var ph []string
		for _, n := range q.Termbases {
			ph = append(ph, "?")
			args = append(args, strings.TrimSpace(n))
		}
		// By the termbase's current name, or the name it had in the
		// change (renamed or deleted termbases).
		query += ` AND (termbase_id IN (SELECT id FROM termbases WHERE name IN (` + strings.Join(ph, ",") + `)) OR termbase COLLATE NOCASE IN (` + strings.Join(ph, ",") + `))`
		args = append(args, args[1:]...)
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
		if err := rows.Scan(&c.Seq, &c.Termbase, &c.EntryID, &c.Version, &c.Kind, &at, &c.By, &c.Summary); err != nil {
			return nil, false, err
		}
		c.At, _ = time.Parse(timeLayout, at)
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

// LastChange is the sequence number of the latest change (0 if none): a
// new client starts from here after loading everything.
func (d *DB) LastChange(ctx context.Context) (int64, error) {
	var seq int64
	err := d.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM changes`).Scan(&seq)
	return seq, err
}
