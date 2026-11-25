package projectdb

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/4nonX/interlinea/internal/sqlstore"
	"github.com/4nonX/interlinea/project"
)

// Provider is a translation service the editor asks for a suggestion: a
// machine translation engine, a language model, or anything else that
// answers the contract (see docs/integration.md).
type Provider struct {
	Name string `json:"name"`
	// Kind is what the service is for: ProviderTranslate, the default,
	// or ProviderCheck for one that finds grammar mistakes.
	Kind string `json:"kind"`
	// URL is where a request is sent.
	URL string `json:"url"`
	// HeaderName and HeaderValue are a header sent with each request,
	// which signs it at the service (Authorization: Bearer …). The value
	// is a secret: it is kept and sent, and never given out.
	HeaderName  string    `json:"headerName,omitempty"`
	HeaderValue string    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
	CreatedBy   string    `json:"createdBy"`
}

// What a service is for.
const (
	ProviderTranslate = "translate"
	ProviderCheck     = "check"
)

// Providers lists the translation services, by name.
func (d *DB) Providers(ctx context.Context) ([]Provider, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT name, kind, url, header_name, header_value, created_at, created_by FROM providers ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		var x Provider
		var created string
		if err := rows.Scan(&x.Name, &x.Kind, &x.URL, &x.HeaderName, &x.HeaderValue, &created, &x.CreatedBy); err != nil {
			return nil, err
		}
		x.CreatedAt = parseTime(created)
		out = append(out, x)
	}
	return out, rows.Err()
}

// Provider returns the translation service of a name.
func (d *DB) Provider(ctx context.Context, name string) (Provider, error) {
	var x Provider
	var created string
	err := d.sql.QueryRowContext(ctx, `SELECT name, kind, url, header_name, header_value, created_at, created_by FROM providers WHERE lower(name) = lower(?)`, strings.TrimSpace(name)).
		Scan(&x.Name, &x.Kind, &x.URL, &x.HeaderName, &x.HeaderValue, &created, &x.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	x.CreatedAt = parseTime(created)
	return x, err
}

// SaveProvider adds a translation service, or replaces the one of its
// name. With keepSecret, the one there keeps its header's value.
func (d *DB) SaveProvider(ctx context.Context, x Provider, keepSecret bool, by string) (Provider, error) {
	x.Name, x.URL, x.HeaderName = strings.TrimSpace(x.Name), strings.TrimSpace(x.URL), strings.TrimSpace(x.HeaderName)
	var problems []project.Problem
	if x.Kind == "" {
		x.Kind = ProviderTranslate
	}
	if x.Kind != ProviderTranslate && x.Kind != ProviderCheck {
		problems = append(problems, project.Problem{Field: "kind", Message: "A service is of the kind translate or check."})
	}
	if x.Name == "" || len(x.Name) > 100 || strings.ContainsFunc(x.Name, unicode.IsControl) || strings.ContainsAny(x.Name, "/?#") {
		problems = append(problems, project.Problem{Field: "name", Message: "A service needs a name of at most 100 characters, without / ? or #."})
	}
	if u, err := url.Parse(x.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		problems = append(problems, project.Problem{Field: "url", Message: "The address starts with https:// or http:// and names where requests are sent."})
	}
	if strings.ContainsFunc(x.HeaderName, func(r rune) bool { return r <= ' ' || r >= 0x7F || r == ':' }) {
		problems = append(problems, project.Problem{Field: "headerName", Message: "The header's name has no blanks and no colon."})
	}
	if strings.ContainsAny(x.HeaderValue, "\r\n") {
		problems = append(problems, project.Problem{Field: "headerValue", Message: "The header's value is one line."})
	}
	if x.HeaderName == "" && x.HeaderValue != "" {
		problems = append(problems, project.Problem{Field: "headerName", Message: "Name the header its value is sent in."})
	}
	if len(problems) > 0 {
		return x, &InvalidError{Problems: problems}
	}
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		if keepSecret {
			var was string
			err := tx.QueryRowContext(ctx, `SELECT header_value FROM providers WHERE lower(name) = lower(?)`, x.Name).Scan(&was)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			x.HeaderValue = was
		}
		if x.HeaderName == "" {
			x.HeaderValue = ""
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM providers WHERE lower(name) = lower(?)`, x.Name); err != nil {
			return err
		}
		x.CreatedAt, x.CreatedBy = now, by
		_, err := tx.ExecContext(ctx, `INSERT INTO providers(name, kind, url, header_name, header_value, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			noNUL(x.Name), x.Kind, noNUL(x.URL), noNUL(x.HeaderName), noNUL(x.HeaderValue), now.Format(timeLayout), noNUL(by))
		return err
	})
	return x, err
}

// DeleteProvider removes a translation service.
func (d *DB) DeleteProvider(ctx context.Context, name string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM providers WHERE lower(name) = lower(?)`, strings.TrimSpace(name))
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
