package authdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/4nonX/interlinea/internal/sqlstore"
)

// A role is a set of permissions. There are five permissions, and five
// roles that come with the server, each a permission and the ones before
// it: read, suggest, edit, approve, manage. Administrators define more
// roles as other sets, such as a reviewer who reads, suggests and approves
// but does not edit.
//
// A Role value is the name of a built-in role, or a set written as its
// permissions joined by "+", in their order ("read+suggest+approve"). A
// grant is stored with the name of its role; what someone may do is always
// worked out as a set.

// Permissions lists the permissions in their order.
var Permissions = []Role{RoleRead, RoleSuggest, RoleEdit, RoleApprove, RoleManage}

// ChangeRoleSet and ChangeRoleDeleted are kinds of change in the change log.
const (
	ChangeRoleSet     = "role-set"
	ChangeRoleDeleted = "role-deleted"
)

// ErrRoleInUse means a role still has grants.
var ErrRoleInUse = errors.New("authdb: the role is in use")

// schemaV3 keeps the roles administrators define.
const schemaV3 = `
	CREATE TABLE roles (
		name        TEXT PRIMARY KEY,
		permissions TEXT NOT NULL
	);`

// permissions is the set a Role value stands for; nil if it is none.
func (r Role) permissions() []Role {
	if n := roleRank[r]; n > 0 {
		return append([]Role(nil), Permissions[:n]...)
	}
	if !strings.Contains(string(r), "+") {
		return nil
	}
	var out []Role
	for _, p := range strings.Split(string(r), "+") {
		if roleRank[Role(p)] == 0 {
			return nil
		}
		out = append(out, Role(p))
	}
	return out
}

// Permissions lists what the role allows, in the permissions' order.
func (r Role) Permissions() []Role { return roleOf(r.permissions()).permissions() }

// roleOf writes a set of permissions as a Role: the built-in role that is
// exactly this set if there is one, else the permissions joined by "+".
func roleOf(perms []Role) Role {
	has := map[Role]bool{}
	for _, p := range perms {
		if roleRank[p] > 0 {
			has[p] = true
		}
	}
	// Whatever is allowed, seeing it is.
	if len(has) > 0 {
		has[RoleRead] = true
	}
	var in []string
	ladder := true
	for i, p := range Permissions {
		if has[p] {
			in = append(in, string(p))
			if len(in) != i+1 {
				ladder = false
			}
		}
	}
	switch {
	case len(in) == 0:
		return ""
	case ladder:
		return Role(in[len(in)-1])
	}
	return Role(strings.Join(in, "+"))
}

// Union is the role that allows what either allows.
func Union(a, b Role) Role {
	return roleOf(append(a.permissions(), b.permissions()...))
}

// RoleDef is a role as it is defined: its name and what it allows.
type RoleDef struct {
	Name        string `json:"name"`
	Permissions []Role `json:"permissions"`
	// BuiltIn roles come with the server and cannot be changed.
	BuiltIn bool `json:"builtIn"`
	// Grants counts the grants that use the role.
	Grants int `json:"grants"`
}

func checkRoleName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", &InvalidError{"name", "Give the role a name."}
	case len([]rune(name)) > 60:
		return "", &InvalidError{"name", "The name is at most 60 characters long."}
	case strings.ContainsAny(name, "+\x00\n\r\t/"):
		return "", &InvalidError{"name", "The name cannot contain +, / or line breaks."}
	case roleRank[Role(strings.ToLower(name))] > 0:
		return "", &InvalidError{"name", "Read, suggest, edit, approve and manage are the roles that come with the server; choose another name."}
	}
	return name, nil
}

func checkPermissions(perms []Role) ([]Role, error) {
	for _, p := range perms {
		if roleRank[p] == 0 {
			return nil, &InvalidError{"permissions", fmt.Sprintf("%q is not a permission: they are read, suggest, edit, approve and manage.", p)}
		}
	}
	has := map[Role]bool{}
	for _, p := range perms {
		has[p] = true
	}
	if !has[RoleRead] {
		return nil, &InvalidError{"permissions", "A role includes read: nothing can be done with what cannot be seen."}
	}
	var set []Role
	for _, p := range Permissions {
		if has[p] {
			set = append(set, p)
		}
	}
	return set, nil
}

type roleQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// defined loads the roles administrators made, by lower-case name.
func defined(ctx context.Context, q roleQuerier) (map[string]RoleDef, error) {
	rows, err := q.QueryContext(ctx, `SELECT name, permissions FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]RoleDef{}
	for rows.Next() {
		var name, perms string
		if err := rows.Scan(&name, &perms); err != nil {
			return nil, err
		}
		out[strings.ToLower(name)] = RoleDef{Name: name, Permissions: Role(perms + "+").split()}
	}
	return out, rows.Err()
}

// split reads a stored list of permissions ("read+approve+").
func (r Role) split() []Role {
	var out []Role
	for _, p := range strings.Split(string(r), "+") {
		if roleRank[Role(p)] > 0 {
			out = append(out, Role(p))
		}
	}
	return out
}

// resolve is the set a stored role name stands for: a built-in role, or
// one of those defined. A name that is neither allows nothing.
func resolve(name string, defs map[string]RoleDef) []Role {
	if ps := Role(name).permissions(); ps != nil {
		return ps
	}
	return defs[strings.ToLower(name)].Permissions
}

// Roles lists the roles: the five that come with the server, then those
// defined, by name, each with how many grants use it.
func (d *DB) Roles(ctx context.Context) ([]RoleDef, error) {
	defs, err := defined(ctx, d.sql)
	if err != nil {
		return nil, err
	}
	used := map[string]int{}
	rows, err := d.sql.QueryContext(ctx, `SELECT role, COUNT(*) FROM grants GROUP BY role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var n int
		if err := rows.Scan(&role, &n); err != nil {
			return nil, err
		}
		used[strings.ToLower(role)] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []RoleDef
	for _, r := range Permissions {
		out = append(out, RoleDef{Name: string(r), Permissions: r.permissions(), BuiltIn: true, Grants: used[string(r)]})
	}
	var own []RoleDef
	for key, def := range defs {
		def.Grants = used[key]
		own = append(own, def)
	}
	sort.Slice(own, func(i, j int) bool { return strings.ToLower(own[i].Name) < strings.ToLower(own[j].Name) })
	return append(out, own...), nil
}

// SetRole defines a role or changes what it allows. With was, the role of
// that name is renamed to name as well, and the grants that use it follow.
// A change takes effect for everyone who has the role at once.
func (d *DB) SetRole(ctx context.Context, was, name string, perms []Role, by string) (RoleDef, error) {
	name, err := checkRoleName(name)
	if err != nil {
		return RoleDef{}, err
	}
	set, err := checkPermissions(perms)
	if err != nil {
		return RoleDef{}, err
	}
	was = strings.TrimSpace(was)
	var stored []string
	for _, p := range set {
		stored = append(stored, string(p))
	}
	err = d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		defs, err := defined(ctx, tx)
		if err != nil {
			return err
		}
		old, renaming := defs[strings.ToLower(was)]
		if was != "" && !renaming {
			return ErrNotFound
		}
		if existing, ok := defs[strings.ToLower(name)]; ok && (!renaming || !strings.EqualFold(existing.Name, old.Name)) {
			if renaming {
				return &InvalidError{"name", fmt.Sprintf("There is a role %q already.", existing.Name)}
			}
			// The same name again changes that role.
			old, renaming = existing, true
		}
		if renaming {
			if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE name = ?`, old.Name); err != nil {
				return err
			}
			if old.Name != name {
				if _, err := tx.ExecContext(ctx, `UPDATE grants SET role = ? WHERE role = ?`, name, old.Name); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO roles(name, permissions) VALUES (?, ?)`, name, strings.Join(stored, "+")); err != nil {
			return err
		}
		return logChange(ctx, tx, ChangeRoleSet, 0, fmt.Sprintf("%s: %s", name, strings.Join(stored, ", ")), by, now)
	})
	if err != nil {
		return RoleDef{}, err
	}
	return RoleDef{Name: name, Permissions: set}, nil
}

// DeleteRole removes a role nobody has. One with grants is not removed:
// ErrRoleInUse.
func (d *DB) DeleteRole(ctx context.Context, name, by string) error {
	return d.write(ctx, func(tx *sqlstore.Tx, now time.Time) error {
		defs, err := defined(ctx, tx)
		if err != nil {
			return err
		}
		def, ok := defs[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return ErrNotFound
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM grants WHERE role = ?`, def.Name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrRoleInUse
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE name = ?`, def.Name); err != nil {
			return err
		}
		return logChange(ctx, tx, ChangeRoleDeleted, 0, def.Name, by, now)
	})
}
