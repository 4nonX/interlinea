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
	"github.com/4nonX/interlinea/mt"
	"github.com/4nonX/interlinea/project"
)

// Provider is a translation service the editor asks for a suggestion: one
// of the services built in (DeepL, Google, Microsoft, a language model,
// package mt) with the person's key, or a service of one's own that
// answers the contract (see docs/integration.md).
type Provider struct {
	Name string `json:"name"`
	// Kind is what the service is for: ProviderTranslate, the default,
	// or ProviderCheck for one that finds grammar mistakes.
	Kind string `json:"kind"`
	// Service is which service it is (mt.Own, the default, mt.DeepL and
	// so on). A grammar service is always one's own.
	Service mt.Service `json:"service"`
	// URL is where a request is sent: the address of a service of one's
	// own, or of one that speaks the OpenAI API; empty for the others.
	URL string `json:"url"`
	// HeaderName is the header a service of one's own takes its key in
	// (Authorization).
	HeaderName string `json:"headerName,omitempty"`
	// HeaderValue is the key: the header's value for a service of one's
	// own, the service's key for the others. It is a secret: it is kept
	// and sent, and never given out.
	HeaderValue string `json:"-"`
	// Model is the model a language model is asked with; Region the
	// region of a Microsoft Translator key.
	Model     string    `json:"model,omitempty"`
	Region    string    `json:"region,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
}

// Settings are what the service is asked with.
func (x Provider) Settings() mt.Settings {
	return mt.Settings{Service: x.Service, URL: x.URL, Key: x.HeaderValue, HeaderName: x.HeaderName, Model: x.Model, Region: x.Region}
}

// What a service is for.
const (
	ProviderTranslate = "translate"
	ProviderCheck     = "check"
)

// Providers lists the translation services, by name.
func (d *DB) Providers(ctx context.Context) ([]Provider, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT name, kind, service, url, header_name, header_value, model, region, created_at, created_by FROM providers ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		var x Provider
		var created string
		if err := rows.Scan(&x.Name, &x.Kind, &x.Service, &x.URL, &x.HeaderName, &x.HeaderValue, &x.Model, &x.Region, &created, &x.CreatedBy); err != nil {
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
	err := d.sql.QueryRowContext(ctx, `SELECT name, kind, service, url, header_name, header_value, model, region, created_at, created_by FROM providers WHERE lower(name) = lower(?)`, strings.TrimSpace(name)).
		Scan(&x.Name, &x.Kind, &x.Service, &x.URL, &x.HeaderName, &x.HeaderValue, &x.Model, &x.Region, &created, &x.CreatedBy)
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
	x.Model, x.Region = strings.TrimSpace(x.Model), strings.TrimSpace(x.Region)
	var problems []project.Problem
	if x.Kind == "" {
		x.Kind = ProviderTranslate
	}
	if x.Service == "" {
		x.Service = mt.Own
	}
	if x.Kind != ProviderTranslate && x.Kind != ProviderCheck {
		problems = append(problems, project.Problem{Field: "kind", Message: "A service is of the kind translate or check."})
	}
	if x.Kind == ProviderCheck && x.Service != mt.Own {
		problems = append(problems, project.Problem{Field: "service", Message: "A grammar service is one of your own, at its address."})
	}
	if x.Name == "" || len(x.Name) > 100 || strings.ContainsFunc(x.Name, unicode.IsControl) || strings.ContainsAny(x.Name, "/?#") {
		problems = append(problems, project.Problem{Field: "name", Message: "A service needs a name of at most 100 characters, without / ? or #."})
	}
	if x.Service == mt.Own {
		if u, err := url.Parse(x.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			problems = append(problems, project.Problem{Field: "url", Message: "The address starts with https:// or http:// and names where requests are sent."})
		}
		if strings.ContainsFunc(x.HeaderName, func(r rune) bool { return r <= ' ' || r >= 0x7F || r == ':' }) {
			problems = append(problems, project.Problem{Field: "headerName", Message: "The header's name has no blanks and no colon."})
		}
		if x.HeaderName == "" && x.HeaderValue != "" {
			problems = append(problems, project.Problem{Field: "headerName", Message: "Name the header its value is sent in."})
		}
	} else if !keepSecret || x.HeaderValue != "" {
		// A built-in service: what it needs, as package mt says. Its key
		// may be the one kept.
		for _, p := range x.Settings().Check() {
			field := p.Field
			if field == "key" {
				field = "headerValue"
			}
			problems = append(problems, project.Problem{Field: field, Message: p.Message})
		}
	} else {
		probe := x.Settings()
		probe.Key = "kept"
		for _, p := range probe.Check() {
			problems = append(problems, project.Problem{Field: p.Field, Message: p.Message})
		}
	}
	if strings.ContainsAny(x.HeaderValue, "\r\n") {
		problems = append(problems, project.Problem{Field: "headerValue", Message: "The header's value is one line."})
	}
	if len(problems) > 0 {
		return x, &InvalidError{Problems: problems}
	}
	if x.Service != mt.Own {
		x.HeaderName = ""
		if x.Service != mt.OpenAI {
			x.URL = ""
		}
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
		if x.Service != mt.Own && x.HeaderValue == "" {
			return &InvalidError{Problems: []project.Problem{{Field: "headerValue", Message: "Paste the key of your account at the service."}}}
		}
		if x.Service == mt.Own && x.HeaderName == "" {
			x.HeaderValue = ""
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM providers WHERE lower(name) = lower(?)`, x.Name); err != nil {
			return err
		}
		x.CreatedAt, x.CreatedBy = now, by
		_, err := tx.ExecContext(ctx, `INSERT INTO providers(name, kind, service, url, header_name, header_value, model, region, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			noNUL(x.Name), x.Kind, string(x.Service), noNUL(x.URL), noNUL(x.HeaderName), noNUL(x.HeaderValue), noNUL(x.Model), noNUL(x.Region), now.Format(timeLayout), noNUL(by))
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
