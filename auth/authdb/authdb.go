// Package authdb keeps the people who work on a server and what they may
// do: accounts with Argon2id password hashes, identities from OpenID
// Connect providers linked to them, sign-in tokens for the desktop and
// web apps, and grants of a role on a project, memory or termbase.
//
// Roles are ordered: read < suggest < edit < approve < manage. A grant on
// one resource ("project" 12) is joined by grants on every resource of the
// kind ("project" "*") and on everything ("all" "*"); the highest role
// counts. Administrators may do everything.
//
// It runs on a SQLite file or PostgreSQL, like the other stores, and logs
// every change to accounts and grants with the person who made it.
package authdb

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"

	"github.com/4nonX/interlinea/internal/sqlitedb"
	"github.com/4nonX/interlinea/internal/sqlstore"
)

// Errors returned by this package.
var (
	ErrNotFound = errors.New("authdb: not found")
	ErrExists   = errors.New("authdb: the username is taken")
	ErrConflict = errors.New("authdb: changed by someone else")
	// ErrCredentials means the username or password is wrong; which of
	// the two is not said.
	ErrCredentials = errors.New("authdb: wrong username or password")
	ErrDisabled    = errors.New("authdb: the account is disabled")
	// ErrToken means the token is unknown, revoked or expired.
	ErrToken = errors.New("authdb: the sign-in is no longer valid")
	// ErrLastAdmin protects the last administrator from being removed,
	// disabled or demoted.
	ErrLastAdmin = errors.New("authdb: the server needs at least one active administrator")
)

// InvalidError is input the store refuses, with what is wrong.
type InvalidError struct{ Field, Message string }

func (e *InvalidError) Error() string { return "authdb: " + e.Field + ": " + e.Message }

const (
	// timeLayout reads stored times; they are written by stamp.
	timeLayout  = time.RFC3339Nano
	writeLock   = 7341
	migrateLock = 7342

	// MinPassword and MaxPassword bound passwords, in characters.
	MinPassword = 12
	MaxPassword = 1024

	// DefaultTokenTTL is how long a sign-in lasts unless said otherwise.
	DefaultTokenTTL = 30 * 24 * time.Hour
	tokenPrefix     = "il_"
	// touchEvery limits writes of a token's last use.
	touchEvery = time.Minute
)

// Role is what someone may do with a resource.
type Role string

const (
	RoleRead    Role = "read"    // see it
	RoleSuggest Role = "suggest" // propose changes for review
	RoleEdit    Role = "edit"    // change it
	RoleApprove Role = "approve" // review and approve changes
	RoleManage  Role = "manage"  // change its settings and who may use it
)

var roleRank = map[Role]int{RoleRead: 1, RoleSuggest: 2, RoleEdit: 3, RoleApprove: 4, RoleManage: 5}

// Includes reports whether r allows what need allows: all of need's
// permissions are among r's.
func (r Role) Includes(need Role) bool {
	want := need.permissions()
	if len(want) == 0 {
		return false
	}
	have := map[Role]bool{}
	for _, p := range r.permissions() {
		have[p] = true
	}
	// A role that comes with the server is asked for by its own
	// permission: "edit" asks for editing, not for suggesting as well.
	if roleRank[need] > 0 {
		return have[need]
	}
	for _, p := range want {
		if !have[p] {
			return false
		}
	}
	return true
}

// Valid reports whether r is a role that comes with the server or a set
// of permissions.
func (r Role) Valid() bool { return r.permissions() != nil }

// Kind is the kind of resource a grant is on.
type Kind string

const (
	KindProject  Kind = "project"
	KindMemory   Kind = "memory"
	KindTermbase Kind = "termbase"
	// KindAll grants on everything; its resource is "*".
	KindAll Kind = "all"
)

// AnyResource in a grant means every resource of its kind.
const AnyResource = "*"

func (k Kind) valid() bool {
	switch k {
	case KindProject, KindMemory, KindTermbase, KindAll:
		return true
	}
	return false
}

// User is an account.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Admin    bool   `json:"admin"`
	Disabled bool   `json:"disabled"`
	// HasPassword is false for accounts that sign in only through an
	// identity provider.
	HasPassword bool      `json:"hasPassword"`
	CreatedAt   time.Time `json:"createdAt"`
	Version     int       `json:"version"`
}

// Grant gives a user a role on a resource.
type Grant struct {
	UserID   int64  `json:"userId"`
	Kind     Kind   `json:"kind"`
	Resource string `json:"resource"`
	Role     Role   `json:"role"`
}

// Token is a sign-in of a user on a device. Its secret is shown once,
// when it is issued; the store keeps only a hash.
type Token struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Identity links an account to a subject at an OpenID Connect provider.
type Identity struct {
	UserID  int64  `json:"userId"`
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

// DB is an open store. It is safe for concurrent use.
type DB struct {
	sql *sqlstore.DB
	d   sqlstore.Dialect
	wmu sync.Mutex
	now func() time.Time
	// hash parameters, lowered in tests.
	params hashParams
}

// Open opens or creates the SQLite store at path.
func Open(path string) (*DB, error) {
	sdb, err := sqlitedb.Open(path, "authdb", migrations)
	if err != nil {
		return nil, err
	}
	return newDB(sdb, sqlstore.SQLite), nil
}

// OpenPostgres uses a PostgreSQL database and brings its schema up to date.
func OpenPostgres(ctx context.Context, sdb *sql.DB) (*DB, error) {
	if err := sqlstore.MigratePostgres(ctx, sdb, "authdb_schema", migrateLock, pgMigrations); err != nil {
		return nil, err
	}
	return newDB(sdb, sqlstore.Postgres), nil
}

func newDB(sdb *sql.DB, d sqlstore.Dialect) *DB {
	return &DB{
		sql:    &sqlstore.DB{DB: sdb, Dialect: d, LockKey: writeLock},
		d:      d,
		now:    func() time.Time { return time.Now().UTC() },
		params: defaultParams,
	}
}

// Close closes the store.
func (d *DB) Close() error { return d.sql.Close() }

const schema = `
	CREATE TABLE users (
		id            %[1]s,
		username      TEXT NOT NULL,
		name          TEXT NOT NULL,
		email         TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		admin         INTEGER NOT NULL,
		disabled      INTEGER NOT NULL,
		created_at    TEXT NOT NULL,
		version       INTEGER NOT NULL
	);
	CREATE UNIQUE INDEX users_by_name ON users (username);
	CREATE TABLE identities (
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		issuer  TEXT NOT NULL,
		subject TEXT NOT NULL
	);
	CREATE UNIQUE INDEX identities_by_subject ON identities (issuer, subject);
	CREATE INDEX identities_by_user ON identities (user_id);
	CREATE TABLE tokens (
		id         %[1]s,
		user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		hash       TEXT NOT NULL,
		name       TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_used  TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);
	CREATE UNIQUE INDEX tokens_by_hash ON tokens (hash);
	CREATE INDEX tokens_by_user ON tokens (user_id);
	CREATE TABLE grants (
		user_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		kind     TEXT NOT NULL,
		resource TEXT NOT NULL,
		role     TEXT NOT NULL
	);
	CREATE UNIQUE INDEX grants_by_resource ON grants (user_id, kind, resource);
	CREATE INDEX grants_on ON grants (kind, resource);
	CREATE TABLE changes (
		seq     %[1]s,
		kind    TEXT NOT NULL,
		user_id BIGINT NOT NULL,
		detail  TEXT NOT NULL,
		at      TEXT NOT NULL,
		author  TEXT NOT NULL
	);`

// schemaV2 keeps sign-ins through a provider while they are under way,
// and the one-time codes they end with, so that every server instance
// behind a load balancer can finish them.
const schemaV2 = `
	CREATE TABLE signin_flows (
		state   TEXT PRIMARY KEY,
		data    TEXT NOT NULL,
		expires TEXT NOT NULL
	);
	CREATE TABLE signin_codes (
		hash    TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		device  TEXT NOT NULL,
		expires TEXT NOT NULL
	);`

var (
	migrations   = []string{fmt.Sprintf(schema, "INTEGER PRIMARY KEY AUTOINCREMENT"), schemaV2, schemaV3}
	pgMigrations = []string{fmt.Sprintf(schema, "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"), schemaV2, schemaV3}
)

// Kinds of change in the change log.
const (
	ChangeUserCreated     = "user-created"
	ChangeUserChanged     = "user-changed"
	ChangePasswordSet     = "password-set"
	ChangeUserDeleted     = "user-deleted"
	ChangeIdentityLinked  = "identity-linked"
	ChangeIdentityRemoved = "identity-removed"
	ChangeGrantSet        = "grant-set"
	ChangeGrantRevoked    = "grant-revoked"
	ChangeTokensRevoked   = "tokens-revoked"
)

func noNUL(s string) string { return strings.ReplaceAll(s, "\x00", "") }

// stamp writes a time at a fixed width in UTC, so the database compares
// stored times as text in time order (expiry checks do).
func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }

func parseTime(s string) time.Time {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (d *DB) write(ctx context.Context, fn func(tx *sqlstore.Tx, now time.Time) error) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	tx, err := d.sql.Begin(ctx)
	if err != nil {
		return fmt.Errorf("authdb: begin: %w", err)
	}
	defer sqlstore.Rollback(tx)
	if err := fn(tx, d.now()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("authdb: commit: %w", err)
	}
	return nil
}

func logChange(ctx context.Context, tx *sqlstore.Tx, kind string, userID int64, detail, by string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO changes(kind, user_id, detail, at, author) VALUES (?, ?, ?, ?, ?)`,
		kind, userID, noNUL(detail), stamp(now), noNUL(by))
	if err != nil {
		return fmt.Errorf("authdb: change log: %w", err)
	}
	return nil
}

// Passwords are hashed with Argon2id and stored in the PHC string format,
// so parameters can be raised later; a hash with older parameters is
// replaced at the next sign-in.
type hashParams struct {
	time, memory uint32
	threads      uint8
}

// defaultParams follow RFC 9106's second recommended option (64 MiB).
var defaultParams = hashParams{time: 3, memory: 64 * 1024, threads: 4}

const keyLen = 32

func (p hashParams) hash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// verify checks a password against a stored hash; current reports whether
// the hash uses p.
func (p hashParams) verify(stored, password string) (ok, current bool) {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, false
	}
	var m, t uint32
	var th uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &th); err != nil || t == 0 || th == 0 {
		return false, false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, false
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, th, uint32(len(want)))
	ok = subtle.ConstantTimeCompare(got, want) == 1
	return ok, m == p.memory && t == p.time && th == p.threads
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// spend hashes a password for nothing, so a wrong username takes as long
// as a wrong password.
func (d *DB) spend(password string) {
	dummyOnce.Do(func() { dummyHash, _ = d.params.hash("not a password of anyone") })
	d.params.verify(dummyHash, password)
}

// CheckPasswordPolicy reports what is wrong with a new password.
func CheckPasswordPolicy(password string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case !utf8.ValidString(password):
		return &InvalidError{"password", "The password is not valid text."}
	case n < MinPassword:
		return &InvalidError{"password", fmt.Sprintf("Use at least %d characters.", MinPassword)}
	case n > MaxPassword:
		return &InvalidError{"password", fmt.Sprintf("Use at most %d characters.", MaxPassword)}
	case strings.ContainsRune(password, 0):
		return &InvalidError{"password", "The password can't contain a NUL character."}
	}
	return nil
}

// NormalizeUsername returns the username as stored (trimmed, lower case)
// or what is wrong with it. Usernames are 1 to 64 letters, digits and
// . _ - @ characters.
func NormalizeUsername(s string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(s))
	if u == "" {
		return "", &InvalidError{"username", "Choose a username."}
	}
	if utf8.RuneCountInString(u) > 64 {
		return "", &InvalidError{"username", "Use at most 64 characters."}
	}
	for _, r := range u {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._-@", r) {
			return "", &InvalidError{"username", "Use letters, digits and . _ - @ only."}
		}
	}
	return u, nil
}

func cleanText(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", &InvalidError{field, "Line breaks and tabs are not allowed."}
	}
	if utf8.RuneCountInString(s) > max {
		return "", &InvalidError{field, fmt.Sprintf("Use at most %d characters.", max)}
	}
	return s, nil
}

// NewUser is an account to create. Password may be empty for an account
// that signs in only through an identity provider.
type NewUser struct {
	Username string
	Name     string
	Email    string
	Password string
	Admin    bool
}

const userColumns = `id, username, name, email, password_hash, admin, disabled, created_at, version`

func scanUser(row interface{ Scan(...any) error }) (User, string, error) {
	var u User
	var hash, created string
	var admin, disabled int
	if err := row.Scan(&u.ID, &u.Username, &u.Name, &u.Email, &hash, &admin, &disabled, &created, &u.Version); err != nil {
		return User{}, "", err
	}
	u.Admin, u.Disabled, u.HasPassword, u.CreatedAt = admin != 0, disabled != 0, hash != "", parseTime(created)
	return u, hash, nil
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func loadUser(ctx context.Context, q rowQuerier, where string, arg any) (User, string, error) {
	u, hash, err := scanUser(q.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return u, hash, err
}

// CreateUser adds an account.
func (d *DB) CreateUser(ctx context.Context, n NewUser, by string) (User, error) {
	username, err := NormalizeUsername(n.Username)
	if err != nil {
		return User{}, err
	}
	name, err := cleanText("name", n.Name, 200)
	if err != nil {
		return User{}, err
	}
	if name == "" {
		name = username
	}
	email, err := cleanText("email", n.Email, 320)
	if err != nil {
		return User{}, err
	}
	if email != "" && !strings.Contains(email, "@") {
		return User{}, &InvalidError{"email", "This is not an email address."}
	}
	hash := ""
	if n.Password != "" {
		if err := CheckPasswordPolicy(n.Password); err != nil {
			return User{}, err
		}
		if hash, err = d.params.hash(n.Password); err != nil {
			return User{}, err
		}
	}
	var u User
	err = d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		var c int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE username = ?`, username).Scan(&c); err != nil {
			return err
		}
		if c > 0 {
			return ErrExists
		}
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO users(username, name, email, password_hash, admin, disabled, created_at, version)
			VALUES (?, ?, ?, ?, ?, 0, ?, 1) RETURNING id`, username, name, email, hash, boolInt(n.Admin), stamp(now)).Scan(&id)
		if err != nil {
			if d.d.IsUnique(err) {
				return ErrExists
			}
			return fmt.Errorf("authdb: create user: %w", err)
		}
		u = User{ID: id, Username: username, Name: name, Email: email, Admin: n.Admin, HasPassword: hash != "", CreatedAt: now, Version: 1}
		return logChange(ctx, tx, ChangeUserCreated, id, username, by, now)
	})
	return u, err
}

// User returns an account by ID.
func (d *DB) User(ctx context.Context, id int64) (User, error) {
	u, _, err := loadUser(ctx, d.sql, "id = ?", id)
	return u, err
}

// UserByName returns an account by username (case-insensitive).
func (d *DB) UserByName(ctx context.Context, username string) (User, error) {
	u, _, err := loadUser(ctx, d.sql, "username = ?", strings.ToLower(strings.TrimSpace(username)))
	return u, err
}

// Users returns all accounts by username.
func (d *DB) Users(ctx context.Context) ([]User, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, _, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers is the number of accounts; a new server has none.
func (d *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// UserChange changes an account; nil fields stay as they are.
type UserChange struct {
	Name     *string
	Email    *string
	Admin    *bool
	Disabled *bool
}

func activeAdmins(ctx context.Context, tx *sqlstore.Tx, except int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE admin = 1 AND disabled = 0 AND id <> ?`, except).Scan(&n)
	return n, err
}

// UpdateUser changes an account as edited from version. Disabling an
// account signs it out everywhere.
func (d *DB) UpdateUser(ctx context.Context, id int64, ch UserChange, version int, by string) (User, error) {
	var out User
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		u, _, err := loadUser(ctx, tx, "id = ?", id)
		if err != nil {
			return err
		}
		if u.Version != version {
			return ErrConflict
		}
		var changed []string
		if ch.Name != nil {
			n, err := cleanText("name", *ch.Name, 200)
			if err != nil {
				return err
			}
			if n == "" {
				n = u.Username
			}
			u.Name = n
			changed = append(changed, "name")
		}
		if ch.Email != nil {
			e, err := cleanText("email", *ch.Email, 320)
			if err != nil {
				return err
			}
			if e != "" && !strings.Contains(e, "@") {
				return &InvalidError{"email", "This is not an email address."}
			}
			u.Email = e
			changed = append(changed, "email")
		}
		wasAdmin := u.Admin && !u.Disabled
		if ch.Admin != nil {
			u.Admin = *ch.Admin
			changed = append(changed, fmt.Sprintf("admin=%t", u.Admin))
		}
		if ch.Disabled != nil {
			u.Disabled = *ch.Disabled
			changed = append(changed, fmt.Sprintf("disabled=%t", u.Disabled))
		}
		if wasAdmin && (!u.Admin || u.Disabled) {
			n, err := activeAdmins(ctx, tx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrLastAdmin
			}
		}
		res, err := tx.ExecContext(ctx, `UPDATE users SET name = ?, email = ?, admin = ?, disabled = ?, version = version + 1 WHERE id = ? AND version = ?`,
			u.Name, u.Email, boolInt(u.Admin), boolInt(u.Disabled), id, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrConflict
		}
		if u.Disabled {
			if _, err := tx.ExecContext(ctx, `DELETE FROM tokens WHERE user_id = ?`, id); err != nil {
				return err
			}
		}
		u.Version++
		out = u
		return logChange(ctx, tx, ChangeUserChanged, id, strings.Join(changed, ", "), by, now)
	})
	return out, err
}

// SetPassword gives an account a new password and signs it out
// everywhere. An empty password removes it: the account then signs in
// only through an identity provider, so it must have one linked.
func (d *DB) SetPassword(ctx context.Context, id int64, password, by string) error {
	hash := ""
	if password != "" {
		if err := CheckPasswordPolicy(password); err != nil {
			return err
		}
		var err error
		if hash, err = d.params.hash(password); err != nil {
			return err
		}
	}
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		if _, _, err := loadUser(ctx, tx, "id = ?", id); err != nil {
			return err
		}
		if hash == "" {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM identities WHERE user_id = ?`, id).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return &InvalidError{"password", "The account has no identity provider to sign in with, so it needs a password."}
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, version = version + 1 WHERE id = ?`, hash, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tokens WHERE user_id = ?`, id); err != nil {
			return err
		}
		detail := "set"
		if hash == "" {
			detail = "removed"
		}
		return logChange(ctx, tx, ChangePasswordSet, id, detail, by, now)
	})
}

// DeleteUser removes an account with its identities, tokens and grants.
func (d *DB) DeleteUser(ctx context.Context, id int64, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		u, _, err := loadUser(ctx, tx, "id = ?", id)
		if err != nil {
			return err
		}
		if u.Admin && !u.Disabled {
			n, err := activeAdmins(ctx, tx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrLastAdmin
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id); err != nil {
			return err
		}
		return logChange(ctx, tx, ChangeUserDeleted, id, u.Username, by, now)
	})
}

// CheckPassword signs in with a username and password. A wrong username
// and a wrong password give the same error after the same work. A hash
// made with older parameters is replaced.
func (d *DB) CheckPassword(ctx context.Context, username, password string) (User, error) {
	u, hash, err := loadUser(ctx, d.sql, "username = ?", strings.ToLower(strings.TrimSpace(username)))
	if errors.Is(err, ErrNotFound) || (err == nil && hash == "") {
		d.spend(password)
		return User{}, ErrCredentials
	}
	if err != nil {
		return User{}, err
	}
	ok, current := d.params.verify(hash, password)
	if !ok {
		return User{}, ErrCredentials
	}
	if u.Disabled {
		return User{}, ErrDisabled
	}
	if !current {
		if fresh, err := d.params.hash(password); err == nil {
			// Best effort: a failed rehash leaves the old, valid hash.
			_ = d.write(ctx, func(tx *sqlstore.Tx, _ time.Time) error {
				_, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ? AND password_hash = ?`, fresh, u.ID, hash)
				return err
			})
		}
	}
	return u, nil
}

// LinkIdentity links an account to a subject of an identity provider. A
// subject belongs to one account.
func (d *DB) LinkIdentity(ctx context.Context, userID int64, issuer, subject, by string) error {
	issuer, subject = strings.TrimSpace(issuer), strings.TrimSpace(subject)
	if issuer == "" || subject == "" || strings.ContainsRune(issuer+subject, 0) {
		return &InvalidError{"identity", "An identity needs an issuer and a subject."}
	}
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		if _, _, err := loadUser(ctx, tx, "id = ?", userID); err != nil {
			return err
		}
		var owner int64
		err := tx.QueryRowContext(ctx, `SELECT user_id FROM identities WHERE issuer = ? AND subject = ?`, issuer, subject).Scan(&owner)
		switch {
		case err == nil && owner == userID:
			return nil
		case err == nil:
			return &InvalidError{"identity", "This identity belongs to another account."}
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO identities(user_id, issuer, subject) VALUES (?, ?, ?)`, userID, issuer, subject); err != nil {
			return err
		}
		return logChange(ctx, tx, ChangeIdentityLinked, userID, issuer+" "+subject, by, now)
	})
}

// UnlinkIdentity removes a link. An account without a password keeps at
// least one identity.
func (d *DB) UnlinkIdentity(ctx context.Context, userID int64, issuer, subject, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		_, hash, err := loadUser(ctx, tx, "id = ?", userID)
		if err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM identities WHERE user_id = ?`, userID).Scan(&n); err != nil {
			return err
		}
		if hash == "" && n <= 1 {
			return &InvalidError{"identity", "The account has no password, so it keeps its last identity provider."}
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM identities WHERE user_id = ? AND issuer = ? AND subject = ?`, userID, issuer, subject)
		if err != nil {
			return err
		}
		if k, err := res.RowsAffected(); err != nil {
			return err
		} else if k == 0 {
			return ErrNotFound
		}
		return logChange(ctx, tx, ChangeIdentityRemoved, userID, issuer+" "+subject, by, now)
	})
}

// UserByIdentity returns the account linked to a provider's subject.
func (d *DB) UserByIdentity(ctx context.Context, issuer, subject string) (User, error) {
	var id int64
	err := d.sql.QueryRowContext(ctx, `SELECT user_id FROM identities WHERE issuer = ? AND subject = ?`, issuer, subject).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return d.User(ctx, id)
}

// Identities returns an account's linked identities.
func (d *DB) Identities(ctx context.Context, userID int64) ([]Identity, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT user_id, issuer, subject FROM identities WHERE user_id = ? ORDER BY issuer, subject`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var i Identity
		if err := rows.Scan(&i.UserID, &i.Issuer, &i.Subject); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func tokenHash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// IssueToken signs a user in on a device and returns the token's secret,
// which is shown only now. ttl 0 means DefaultTokenTTL.
func (d *DB) IssueToken(ctx context.Context, userID int64, name string, ttl time.Duration) (string, Token, error) {
	if ttl <= 0 {
		ttl = DefaultTokenTTL
	}
	name, err := cleanText("name", name, 200)
	if err != nil {
		return "", Token{}, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Token{}, err
	}
	secret := tokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	var t Token
	err = d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		u, _, err := loadUser(ctx, tx, "id = ?", userID)
		if err != nil {
			return err
		}
		if u.Disabled {
			return ErrDisabled
		}
		at, exp := stamp(now), stamp(now.Add(ttl))
		var id int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO tokens(user_id, hash, name, created_at, last_used, expires_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
			userID, tokenHash(secret), name, at, at, exp).Scan(&id); err != nil {
			return err
		}
		t = Token{ID: id, UserID: userID, Name: name, CreatedAt: now, LastUsed: now, ExpiresAt: now.Add(ttl)}
		return nil
	})
	if err != nil {
		return "", Token{}, err
	}
	return secret, t, nil
}

// Authenticate returns the user a token's secret belongs to. Unknown,
// revoked and expired tokens and disabled accounts give ErrToken.
func (d *DB) Authenticate(ctx context.Context, secret string) (User, Token, error) {
	if !strings.HasPrefix(secret, tokenPrefix) {
		return User{}, Token{}, ErrToken
	}
	var t Token
	var created, used, exp string
	err := d.sql.QueryRowContext(ctx, `SELECT id, user_id, name, created_at, last_used, expires_at FROM tokens WHERE hash = ?`, tokenHash(secret)).
		Scan(&t.ID, &t.UserID, &t.Name, &created, &used, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, Token{}, ErrToken
	}
	if err != nil {
		return User{}, Token{}, err
	}
	t.CreatedAt, t.LastUsed, t.ExpiresAt = parseTime(created), parseTime(used), parseTime(exp)
	now := d.now()
	if !now.Before(t.ExpiresAt) {
		return User{}, Token{}, ErrToken
	}
	u, err := d.User(ctx, t.UserID)
	if err != nil || u.Disabled {
		return User{}, Token{}, ErrToken
	}
	if now.Sub(t.LastUsed) >= touchEvery {
		if err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
			_, err := tx.ExecContext(ctx, `UPDATE tokens SET last_used = ? WHERE id = ?`, stamp(now), t.ID)
			return err
		}); err != nil {
			return User{}, Token{}, err
		}
		t.LastUsed = now
	}
	return u, t, nil
}

// Tokens returns a user's current sign-ins, newest first.
func (d *DB) Tokens(ctx context.Context, userID int64) ([]Token, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, user_id, name, created_at, last_used, expires_at FROM tokens WHERE user_id = ? AND expires_at > ? ORDER BY id DESC`,
		userID, stamp(d.now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		var t Token
		var created, used, exp string
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &created, &used, &exp); err != nil {
			return nil, err
		}
		t.CreatedAt, t.LastUsed, t.ExpiresAt = parseTime(created), parseTime(used), parseTime(exp)
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken signs a user out on one device.
func (d *DB) RevokeToken(ctx context.Context, userID, tokenID int64) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM tokens WHERE id = ? AND user_id = ?`, tokenID, userID)
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

// RevokeTokens signs a user out everywhere.
func (d *DB) RevokeTokens(ctx context.Context, userID int64, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM tokens WHERE user_id = ?`, userID); err != nil {
			return err
		}
		return logChange(ctx, tx, ChangeTokensRevoked, userID, "", by, now)
	})
}

// PurgeExpired deletes expired tokens and returns how many.
func (d *DB) PurgeExpired(ctx context.Context) (int, error) {
	var n int64
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM tokens WHERE expires_at <= ?`, stamp(now))
		if err != nil {
			return err
		}
		if n, err = res.RowsAffected(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM signin_flows WHERE expires <= ?`, stamp(now)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM signin_codes WHERE expires <= ?`, stamp(now))
		return err
	})
	return int(n), err
}

// SetClock sets the store's clock (default time.Now): tests, and servers
// that keep their own.
func (d *DB) SetClock(now func() time.Time) { d.now = now }

// ErrTooMany refuses a new sign-in while too many are under way.
var ErrTooMany = errors.New("authdb: too many sign-ins under way")

// MaxFlows bounds the provider sign-ins under way, so starting them can't
// fill the database.
const MaxFlows = 10000

// SaveFlow keeps a provider sign-in under way by its state until expires.
func (d *DB) SaveFlow(ctx context.Context, state string, data []byte, expires time.Time) error {
	if state == "" {
		return &InvalidError{"state", "A sign-in needs its state."}
	}
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM signin_flows WHERE expires <= ?`, stamp(now)); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM signin_flows`).Scan(&n); err != nil {
			return err
		}
		if n >= MaxFlows {
			return ErrTooMany
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO signin_flows(state, data, expires) VALUES (?, ?, ?)`, state, string(data), stamp(expires))
		if d.d.IsUnique(err) {
			return ErrExists
		}
		return err
	})
}

// TakeFlow returns a sign-in under way and forgets it: each is finished
// once. ErrNotFound if there is none, or it expired.
func (d *DB) TakeFlow(ctx context.Context, state string) ([]byte, error) {
	var data string
	err := d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		var exp string
		err := tx.QueryRowContext(ctx, `SELECT data, expires FROM signin_flows WHERE state = ?`, state).Scan(&data, &exp)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM signin_flows WHERE state = ?`, state); err != nil {
			return err
		}
		if !parseTime(exp).After(now) {
			return ErrNotFound
		}
		return nil
	})
	return []byte(data), err
}

// SaveCode keeps the one-time code a provider sign-in ends with, which the
// app exchanges for a token. Only its hash is stored.
func (d *DB) SaveCode(ctx context.Context, code string, userID int64, device string, expires time.Time) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO signin_codes(hash, user_id, device, expires) VALUES (?, ?, ?, ?)`, tokenHash(code), userID, noNUL(device), stamp(expires))
		return err
	})
}

// TakeCode returns whom a one-time code signs in and forgets it.
// ErrNotFound if it doesn't exist, was used, or expired.
func (d *DB) TakeCode(ctx context.Context, code string) (userID int64, device string, err error) {
	err = d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		var exp string
		h := tokenHash(code)
		err := tx.QueryRowContext(ctx, `SELECT user_id, device, expires FROM signin_codes WHERE hash = ?`, h).Scan(&userID, &device, &exp)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM signin_codes WHERE hash = ?`, h); err != nil {
			return err
		}
		if !parseTime(exp).After(now) {
			return ErrNotFound
		}
		return nil
	})
	return userID, device, err
}

func checkGrant(g Grant) (Grant, error) {
	g.Resource = strings.TrimSpace(g.Resource)
	switch {
	case !g.Kind.valid():
		return g, &InvalidError{"kind", "A grant is on a project, memory, termbase or everything."}
	case strings.TrimSpace(string(g.Role)) == "":
		return g, &InvalidError{"role", "Say which role."}
	case g.Resource == "" || strings.ContainsRune(g.Resource, 0):
		return g, &InvalidError{"resource", "Name the resource, or * for all of the kind."}
	case g.Kind == KindAll && g.Resource != AnyResource:
		return g, &InvalidError{"resource", "A grant on everything is on *."}
	}
	return g, nil
}

// SetGrant gives a user a role on a resource, replacing the role they had
// on exactly that resource.
func (d *DB) SetGrant(ctx context.Context, g Grant, by string) error {
	g, err := checkGrant(g)
	if err != nil {
		return err
	}
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		if _, _, err := loadUser(ctx, tx, "id = ?", g.UserID); err != nil {
			return err
		}
		// The role is one that comes with the server or one defined; it
		// is stored under its name as written there.
		if roleRank[Role(strings.ToLower(string(g.Role)))] > 0 {
			g.Role = Role(strings.ToLower(string(g.Role)))
		} else {
			defs, err := defined(ctx, tx)
			if err != nil {
				return err
			}
			def, ok := defs[strings.ToLower(strings.TrimSpace(string(g.Role)))]
			if !ok {
				return &InvalidError{"role", fmt.Sprintf("There is no role %q.", g.Role)}
			}
			g.Role = Role(def.Name)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM grants WHERE user_id = ? AND kind = ? AND resource = ?`, g.UserID, string(g.Kind), g.Resource); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO grants(user_id, kind, resource, role) VALUES (?, ?, ?, ?)`, g.UserID, string(g.Kind), g.Resource, string(g.Role)); err != nil {
			return err
		}
		return logChange(ctx, tx, ChangeGrantSet, g.UserID, fmt.Sprintf("%s %s %s", g.Kind, g.Resource, g.Role), by, now)
	})
}

// RevokeGrant removes a user's grant on exactly one resource.
func (d *DB) RevokeGrant(ctx context.Context, userID int64, kind Kind, resource, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM grants WHERE user_id = ? AND kind = ? AND resource = ?`, userID, string(kind), resource)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrNotFound
		}
		return logChange(ctx, tx, ChangeGrantRevoked, userID, fmt.Sprintf("%s %s", kind, resource), by, now)
	})
}

// RevokeResource removes every grant on a resource, as when it is deleted.
func (d *DB) RevokeResource(ctx context.Context, kind Kind, resource, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		rows, err := tx.QueryContext(ctx, `SELECT user_id FROM grants WHERE kind = ? AND resource = ?`, string(kind), resource)
		if err != nil {
			return err
		}
		var users []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			users = append(users, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM grants WHERE kind = ? AND resource = ?`, string(kind), resource); err != nil {
			return err
		}
		for _, id := range users {
			if err := logChange(ctx, tx, ChangeGrantRevoked, id, fmt.Sprintf("%s %s", kind, resource), by, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// RenameResource moves every grant on a resource to its new name, as
// when a termbase is renamed. A user with grants on both names keeps the
// higher role.
func (d *DB) RenameResource(ctx context.Context, kind Kind, from, to, by string) error {
	to = strings.TrimSpace(to)
	if to == "" || to == AnyResource || strings.ContainsRune(to, 0) {
		return &InvalidError{"resource", "Name the resource."}
	}
	if kind == KindAll || !kind.valid() {
		return &InvalidError{"kind", "Only a project, memory or termbase is renamed."}
	}
	if from == to {
		return nil
	}
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		// The definitions are read first: a transaction reads one result
		// at a time.
		defs, err := defined(ctx, tx)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT user_id, resource, role FROM grants WHERE kind = ? AND (resource = ? OR resource = ?)`, string(kind), from, to)
		if err != nil {
			return err
		}
		// Someone with a role under both names keeps the one that allows
		// more.
		best := map[int64]Role{}
		moved := map[int64]bool{}
		for rows.Next() {
			var id int64
			var res, role string
			if err := rows.Scan(&id, &res, &role); err != nil {
				rows.Close()
				return err
			}
			if len(resolve(role, defs)) > len(resolve(string(best[id]), defs)) {
				best[id] = Role(role)
			}
			if res == from {
				moved[id] = true
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM grants WHERE kind = ? AND (resource = ? OR resource = ?)`, string(kind), from, to); err != nil {
			return err
		}
		ids := make([]int64, 0, len(best))
		for id := range best {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, `INSERT INTO grants(user_id, kind, resource, role) VALUES (?, ?, ?, ?)`, id, string(kind), to, string(best[id])); err != nil {
				return err
			}
			if moved[id] {
				if err := logChange(ctx, tx, ChangeGrantSet, id, fmt.Sprintf("%s %s %s (was %s)", kind, to, best[id], from), by, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func scanGrants(rows *sql.Rows) ([]Grant, error) {
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		var kind, role string
		if err := rows.Scan(&g.UserID, &kind, &g.Resource, &role); err != nil {
			return nil, err
		}
		g.Kind, g.Role = Kind(kind), Role(role)
		out = append(out, g)
	}
	return out, rows.Err()
}

// Grants returns a user's grants.
func (d *DB) Grants(ctx context.Context, userID int64) ([]Grant, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT user_id, kind, resource, role FROM grants WHERE user_id = ? ORDER BY kind, resource`, userID)
	if err != nil {
		return nil, err
	}
	return scanGrants(rows)
}

// GrantsOn returns the grants on a resource itself (not the ones on all
// of its kind).
func (d *DB) GrantsOn(ctx context.Context, kind Kind, resource string) ([]Grant, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT user_id, kind, resource, role FROM grants WHERE kind = ? AND resource = ? ORDER BY user_id`, string(kind), resource)
	if err != nil {
		return nil, err
	}
	return scanGrants(rows)
}

// RoleOn is what a user may do with a resource, as a role: everything
// their grants on it, on all of its kind and on everything allow together;
// RoleManage for administrators; "" for nothing. With roles of one's own
// the result may be a set that is no built-in role ("read+approve").
func (d *DB) RoleOn(ctx context.Context, u User, kind Kind, resource string) (Role, error) {
	if u.Disabled {
		return "", nil
	}
	if u.Admin {
		return RoleManage, nil
	}
	rows, err := d.sql.QueryContext(ctx, `SELECT role FROM grants WHERE user_id = ? AND
		((kind = ? AND (resource = ? OR resource = '*')) OR (kind = 'all' AND resource = '*'))`, u.ID, string(kind), resource)
	if err != nil {
		return "", err
	}
	var names []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			rows.Close()
			return "", err
		}
		names = append(names, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(names) == 0 {
		return "", err
	}
	// Every grant adds what its role allows.
	var defs map[string]RoleDef
	var all []Role
	for _, n := range names {
		if ps := Role(n).permissions(); ps != nil {
			all = append(all, ps...)
			continue
		}
		if defs == nil {
			if defs, err = defined(ctx, d.sql); err != nil {
				return "", err
			}
		}
		all = append(all, resolve(n, defs)...)
	}
	return roleOf(all), nil
}

// Can reports whether a user may do what need allows on a resource.
func (d *DB) Can(ctx context.Context, u User, kind Kind, resource string, need Role) (bool, error) {
	r, err := d.RoleOn(ctx, u, kind, resource)
	if err != nil {
		return false, err
	}
	return r.Includes(need), nil
}

// Change is one entry of the change log of accounts and grants.
type Change struct {
	Seq    int64     `json:"seq"`
	Kind   string    `json:"kind"`
	UserID int64     `json:"userId"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
	By     string    `json:"by"`
}

// LastChange returns the sequence number of the latest change (0 for
// none).
func (d *DB) LastChange(ctx context.Context) (int64, error) {
	var seq int64
	err := d.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM changes`).Scan(&seq)
	return seq, err
}

// Changes returns the log after since, up to limit entries (default
// 1000); more reports whether there are more.
func (d *DB) Changes(ctx context.Context, since int64, limit int) (changes []Change, more bool, err error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := d.sql.QueryContext(ctx, `SELECT seq, kind, user_id, detail, at, author FROM changes WHERE seq > ? ORDER BY seq LIMIT ?`, since, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Change
		var at string
		if err := rows.Scan(&c.Seq, &c.Kind, &c.UserID, &c.Detail, &at, &c.By); err != nil {
			return nil, false, err
		}
		c.At = parseTime(at)
		changes = append(changes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(changes) > limit {
		return changes[:limit], true, nil
	}
	return changes, false, nil
}
