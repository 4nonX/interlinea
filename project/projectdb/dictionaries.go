package projectdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/4nonX/interlinea/internal/sqlstore"
	"github.com/4nonX/interlinea/project"
)

// Dictionary is a spelling dictionary (Hunspell .aff and .dic files) the
// projects' quality checks spell with.
type Dictionary struct {
	// Name identifies it ("de_DE_frami").
	Name string `json:"name"`
	// Lang is the language it spells (a BCP 47 tag).
	Lang string `json:"lang"`
	// Locales are the languages its archive declared for it.
	Locales []string `json:"locales"`
	// Words counts its words.
	Words     int       `json:"words"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
}

// Dictionaries lists the installed dictionaries by name.
func (d *DB) Dictionaries(ctx context.Context) ([]Dictionary, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT name, lang, locales, words, created_at, created_by FROM dictionaries ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dictionary
	for rows.Next() {
		var x Dictionary
		var locales, created string
		if err := rows.Scan(&x.Name, &x.Lang, &locales, &x.Words, &created, &x.CreatedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(locales), &x.Locales); err != nil {
			return nil, fmt.Errorf("projectdb: dictionary %s: %w", x.Name, err)
		}
		x.CreatedAt = parseTime(created)
		out = append(out, x)
	}
	return out, rows.Err()
}

// DictionaryFiles returns a dictionary's .aff and .dic files.
func (d *DB) DictionaryFiles(ctx context.Context, name string) (Dictionary, []byte, []byte, error) {
	var x Dictionary
	var locales, created string
	var aff, dic []byte
	err := d.sql.QueryRowContext(ctx, `SELECT name, lang, locales, words, created_at, created_by, aff, dic FROM dictionaries WHERE lower(name) = lower(?)`,
		strings.TrimSpace(name)).Scan(&x.Name, &x.Lang, &locales, &x.Words, &created, &x.CreatedBy, &aff, &dic)
	if errors.Is(err, sql.ErrNoRows) {
		return x, nil, nil, ErrNotFound
	}
	if err != nil {
		return x, nil, nil, err
	}
	if err := json.Unmarshal([]byte(locales), &x.Locales); err != nil {
		return x, nil, nil, fmt.Errorf("projectdb: dictionary %s: %w", x.Name, err)
	}
	x.CreatedAt = parseTime(created)
	return x, aff, dic, nil
}

// SaveDictionary installs a dictionary, replacing one of the same name.
func (d *DB) SaveDictionary(ctx context.Context, x Dictionary, aff, dic []byte, by string) (Dictionary, error) {
	x.Name = strings.TrimSpace(x.Name)
	if x.Name == "" || strings.ContainsFunc(x.Name, unicode.IsControl) {
		return x, &InvalidError{Problems: []project.Problem{{Field: "name", Message: "A dictionary needs a name without control characters."}}}
	}
	if x.Locales == nil {
		x.Locales = []string{}
	}
	locales, err := json.Marshal(x.Locales)
	if err != nil {
		return x, err
	}
	err = d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM dictionaries WHERE lower(name) = lower(?)`, x.Name); err != nil {
			return err
		}
		x.CreatedAt, x.CreatedBy = now, by
		_, err := tx.ExecContext(ctx, `INSERT INTO dictionaries(name, lang, locales, words, aff, dic, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			noNUL(x.Name), noNUL(x.Lang), string(locales), x.Words, aff, dic, now.Format(timeLayout), noNUL(by))
		return err
	})
	return x, err
}

// DeleteDictionary removes a dictionary.
func (d *DB) DeleteDictionary(ctx context.Context, name string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM dictionaries WHERE lower(name) = lower(?)`, strings.TrimSpace(name))
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}
