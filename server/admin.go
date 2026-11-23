package server

import (
	"net/http"
	"strconv"

	"github.com/4nonX/interlinea/auth/authdb"
)

// listUsers lists all accounts.
//
// Access: administrators.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, me session) {
	us, err := s.cfg.Auth.Users(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(us))
}

type newUserRequest struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	// Password may be empty for someone who signs in only through the
	// identity provider; link their identity afterwards.
	Password string `json:"password"`
	Admin    bool   `json:"admin"`
}

// createUser makes an account.
//
// Access: administrators.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request, me session) {
	var req newUserRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	u, err := s.cfg.Auth.CreateUser(r.Context(), authdb.NewUser{Username: req.Username, Name: req.Name, Email: req.Email, Password: req.Password, Admin: req.Admin}, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// getUser returns an account.
//
// Access: administrators.
func (s *Server) getUser(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	u, err := s.cfg.Auth.User(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

type userPatch struct {
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Admin    *bool   `json:"admin"`
	Disabled *bool   `json:"disabled"`
}

// updateUser changes an account's name, email, administrator flag or disabled
// state; fields left out stay.
// Disabling an account ends all its sign-ins. Send the version read (If-Match
// or ?version=).
//
// Access: administrators.
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	var req userPatch
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	u, err := s.cfg.Auth.UpdateUser(r.Context(), id, authdb.UserChange{Name: req.Name, Email: req.Email, Admin: req.Admin, Disabled: req.Disabled}, v, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A disabled account is signed out everywhere.
	if u.Disabled {
		if err := s.cfg.Auth.RevokeTokens(r.Context(), u.ID, me.by()); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, u)
}

// deleteUser deletes an account with its sign-ins, grants and linked
// identities.
//
// Access: administrators.
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.cfg.Auth.DeleteUser(r.Context(), id, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setUserPassword sets or removes an account's password and ends its sign-ins.
//
// Access: administrators.
func (s *Server) setUserPassword(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		// Password is the new password; empty removes it (the account
		// then signs in only through the identity provider).
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if err := s.cfg.Auth.SetPassword(r.Context(), id, req.Password, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// userTokens lists an account's sign-ins.
//
// Access: administrators.
func (s *Server) userTokens(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.cfg.Auth.User(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	ts, err := s.cfg.Auth.Tokens(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(ts))
}

// revokeUserTokens ends all of an account's sign-ins.
//
// Access: administrators.
func (s *Server) revokeUserTokens(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.cfg.Auth.RevokeTokens(r.Context(), id, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// userIdentities lists the provider sign-ins linked to an account.
//
// Access: administrators.
func (s *Server) userIdentities(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.cfg.Auth.User(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	ids, err := s.cfg.Auth.Identities(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(ids))
}

// linkUserIdentity links a provider sign-in (issuer and subject; the issuer
// defaults to the server's provider) to an account.
//
// Access: administrators.
func (s *Server) linkUserIdentity(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if req.Issuer == "" && s.cfg.OIDC != nil {
		req.Issuer = s.cfg.OIDC.Provider.Metadata().Issuer
	}
	if err := s.cfg.Auth.LinkIdentity(r.Context(), id, req.Issuer, req.Subject, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// unlinkUserIdentity removes a provider sign-in (?issuer=&subject=) from an
// account.
//
// Access: administrators.
func (s *Server) unlinkUserIdentity(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	if err := s.cfg.Auth.UnlinkIdentity(r.Context(), id, q.Get("issuer"), q.Get("subject"), me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// userGrants lists an account's roles.
//
// Access: administrators.
func (s *Server) userGrants(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.cfg.Auth.User(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	gs, err := s.cfg.Auth.Grants(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(gs))
}

// mayGrant checks that someone may see and change the grants on a
// resource: administrators on everything, managers on what they manage.
// Grants on every resource of a kind, or on everything, are for
// administrators. A resource other than * must exist.
func (s *Server) mayGrant(w http.ResponseWriter, r *http.Request, me session, kind authdb.Kind, resource string) bool {
	ctx := r.Context()
	if resource != authdb.AnyResource && kind != authdb.KindAll {
		var err error
		switch kind {
		case authdb.KindProject:
			var id int64
			id, err = strconv.ParseInt(resource, 10, 64)
			if err == nil {
				_, err = s.cfg.Projects.Project(ctx, id)
			} else {
				invalid(w, "resource", "A project is named by its number.")
				return false
			}
		case authdb.KindMemory:
			_, err = s.cfg.Memories.Memory(ctx, resource)
		case authdb.KindTermbase:
			_, err = s.cfg.Termbases.Termbase(ctx, resource)
		default:
			invalid(w, "kind", "A grant is on a project, memory, termbase or everything.")
			return false
		}
		if err != nil {
			s.fail(w, r, err)
			return false
		}
	}
	if me.User.Admin {
		return true
	}
	if kind == authdb.KindAll || resource == authdb.AnyResource {
		forbidden(w, "Only an administrator gives roles on everything.")
		return false
	}
	ok, err := s.cfg.Auth.Can(ctx, me.User, kind, resource, authdb.RoleManage)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if !ok {
		forbidden(w, "Only its managers and administrators decide who may use it.")
	}
	return ok
}

// grantView is a grant with the person's name, for lists.
type grantView struct {
	authdb.Grant
	Username string `json:"username"`
	Name     string `json:"name"`
}

// grantsOn lists who has which role on a project, memory or termbase
// (?kind=&resource=).
//
// Access: administrators, and those who manage the resource.
func (s *Server) grantsOn(w http.ResponseWriter, r *http.Request, me session) {
	q := r.URL.Query()
	kind, resource := authdb.Kind(q.Get("kind")), q.Get("resource")
	if !s.mayGrant(w, r, me, kind, resource) {
		return
	}
	gs, err := s.cfg.Auth.GrantsOn(r.Context(), kind, resource)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []grantView{}
	for _, g := range gs {
		u, err := s.cfg.Auth.User(r.Context(), g.UserID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out = append(out, grantView{Grant: g, Username: u.Username, Name: u.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// setGrant gives someone a role on a project, memory, termbase or everything
// of a kind (resource *).
//
// Access: administrators, and those who manage the resource (for one resource).
func (s *Server) setGrant(w http.ResponseWriter, r *http.Request, me session) {
	var g authdb.Grant
	if !readJSON(w, r, &g, maxBody) {
		return
	}
	if !s.mayGrant(w, r, me, g.Kind, g.Resource) {
		return
	}
	if err := s.cfg.Auth.SetGrant(r.Context(), g, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// revokeGrant takes a role back (?userId=&kind=&resource=).
//
// Access: administrators, and those who manage the resource.
func (s *Server) revokeGrant(w http.ResponseWriter, r *http.Request, me session) {
	q := r.URL.Query()
	kind, resource := authdb.Kind(q.Get("kind")), q.Get("resource")
	uid, err := strconv.ParseInt(q.Get("userId"), 10, 64)
	if err != nil {
		invalid(w, "userId", "Say whose grant goes.")
		return
	}
	if !s.mayGrant(w, r, me, kind, resource) {
		return
	}
	if err := s.cfg.Auth.RevokeGrant(r.Context(), uid, kind, resource, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
