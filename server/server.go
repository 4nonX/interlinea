// Package server is Interlinea's team server: an HTTP JSON API over the
// stores of projects, translation memories, termbases and accounts, for
// the desktop and web apps.
//
// Every request but the sign-in ones carries a token in an
// "Authorization: Bearer il_..." header; tokens come from signing in with
// a password or through an OpenID Connect provider. What a person may do
// follows their grants (authdb roles) and, in projects, their
// assignments. Writes name the version they were made from and are
// refused with 409 when someone else saved in between. A live feed of
// changes (/api/events) tells the apps what to read again.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/auth/oidc"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm/tmdb"
)

// Stores are the databases the server works on.
type Stores struct {
	Auth      *authdb.DB
	Projects  *projectdb.DB
	Memories  *tmdb.DB
	Termbases *termdb.DB
}

// OIDC is an identity provider people can sign in with.
type OIDC struct {
	// Name is shown on the sign-in button ("Microsoft", "Keycloak").
	Name     string
	Provider *oidc.Provider
	// CreateUsers makes an account, without grants, for someone the
	// provider signs in who has none yet. Otherwise an administrator (or
	// the person, signed in with a password) links the identity first.
	CreateUsers bool
}

// Config sets up a server.
type Config struct {
	Stores
	// PublicURL is the server's address as the apps reach it
	// ("https://interlinea.example.com"); sign-in through a provider
	// returns there.
	PublicURL string
	// Origins are web apps on other addresses allowed to call the API
	// ("http://localhost:5173" while developing the web app).
	Origins []string
	// OIDC is the identity provider, if any.
	OIDC *OIDC
	// SetupToken lets the first administrator be made through the API
	// while there are no accounts. Empty turns that off.
	SetupToken string
	// TokenTTL is how long a sign-in lasts (default 30 days).
	TokenTTL time.Duration
	// TrustProxy takes the client's address from X-Forwarded-For, for a
	// server behind a reverse proxy.
	TrustProxy bool
	// Web, if set, is the web app, served at /.
	Web fs.FS
	// Version is reported by /api/server.
	Version string
	Log     *slog.Logger
	// Now is the clock (tests).
	Now func() time.Time
	// EventPoll is how often the live feed looks for changes (default 1s).
	EventPoll time.Duration
	// Hosts, if set, are the only Host headers answered ("127.0.0.1:4711"):
	// the desktop app's core listens on the loopback interface, and a web
	// page that points a name of its own at that address (DNS rebinding)
	// must not reach it.
	Hosts []string
}

// Server handles the API.
type Server struct {
	cfg     Config
	mux     *http.ServeMux
	origins map[string]bool
	public  *url.URL

	signinIP   *limiter
	signinUser *limiter

	// mu makes setting up the first administrator one at a time.
	mu sync.Mutex

	feed *feed

	dicts dictionaries
	terms termIndexes
}

// New makes a server.
func New(cfg Config) (*Server, error) {
	if cfg.Auth == nil || cfg.Projects == nil || cfg.Memories == nil || cfg.Termbases == nil {
		return nil, errors.New("server: all four stores are needed")
	}
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = authdb.DefaultTokenTTL
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.EventPoll <= 0 {
		cfg.EventPoll = time.Second
	}
	for i, h := range cfg.Hosts {
		cfg.Hosts[i] = strings.ToLower(h)
	}
	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		origins:    map[string]bool{},
		signinIP:   newLimiter(20, time.Minute, cfg.Now),
		signinUser: newLimiter(10, 15*time.Minute, cfg.Now),
	}
	if cfg.PublicURL != "" {
		u, err := url.Parse(strings.TrimRight(cfg.PublicURL, "/"))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, fmt.Errorf("server: the public address %q is not an http(s) URL", cfg.PublicURL)
		}
		s.public = u
		s.origins[u.Scheme+"://"+u.Host] = true
	}
	if cfg.OIDC != nil && s.public == nil {
		return nil, errors.New("server: sign-in through a provider needs the public address")
	}
	for _, o := range cfg.Origins {
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || (u.Path != "" && u.Path != "/") {
			return nil, fmt.Errorf("server: %q is not an origin (scheme://host[:port])", o)
		}
		s.origins[u.Scheme+"://"+u.Host] = true
	}
	s.feed = newFeed(s)
	s.routes()
	return s, nil
}

// Close stops the live feed.
func (s *Server) Close() { s.feed.stop() }

// Routes is the pattern table, also used by the tests to check that every
// route needs the right sign-in.
func (s *Server) routes() {
	// Signing in.
	s.handle("GET /api/server", s.anyone(s.serverInfo))
	s.handle("GET /api/openapi.json", s.anyone(s.openAPI))
	s.handle("POST /api/setup", s.anyone(s.setup))
	s.handle("POST /api/auth/password", s.anyone(s.signinPassword))
	s.handle("POST /api/auth/oidc/start", s.anyone(s.oidcStart))
	s.handle("GET /api/auth/oidc/callback", s.anyone(s.oidcCallback))
	s.handle("POST /api/auth/exchange", s.anyone(s.exchange))
	s.handle("POST /api/auth/signout", s.signedIn(s.signout))

	// The signed-in person.
	s.handle("GET /api/me", s.signedIn(s.me))
	s.handle("PUT /api/me/password", s.signedIn(s.changeOwnPassword))
	s.handle("GET /api/me/tokens", s.signedIn(s.ownTokens))
	s.handle("DELETE /api/me/tokens/{id}", s.signedIn(s.revokeOwnToken))
	s.handle("POST /api/me/identities", s.signedIn(s.linkOwnIdentity))
	s.handle("DELETE /api/me/identities", s.signedIn(s.unlinkOwnIdentity))
	s.handle("GET /api/me/assignments", s.signedIn(s.ownAssignments))
	s.handle("GET /api/people", s.signedIn(s.people))

	// Accounts (administrators).
	s.handle("GET /api/users", s.admin(s.listUsers))
	s.handle("POST /api/users", s.admin(s.createUser))
	s.handle("GET /api/users/{id}", s.admin(s.getUser))
	s.handle("PATCH /api/users/{id}", s.admin(s.updateUser))
	s.handle("DELETE /api/users/{id}", s.admin(s.deleteUser))
	s.handle("PUT /api/users/{id}/password", s.admin(s.setUserPassword))
	s.handle("GET /api/users/{id}/tokens", s.admin(s.userTokens))
	s.handle("DELETE /api/users/{id}/tokens", s.admin(s.revokeUserTokens))
	s.handle("GET /api/users/{id}/identities", s.admin(s.userIdentities))
	s.handle("POST /api/users/{id}/identities", s.admin(s.linkUserIdentity))
	s.handle("DELETE /api/users/{id}/identities", s.admin(s.unlinkUserIdentity))
	s.handle("GET /api/users/{id}/grants", s.admin(s.userGrants))

	// Grants: administrators, and managers of the resource.
	s.handle("GET /api/grants", s.signedIn(s.grantsOn))
	s.handle("PUT /api/grants", s.signedIn(s.setGrant))
	s.handle("GET /api/roles", s.signedIn(s.listRoles))
	s.handle("PUT /api/roles/{name}", s.admin(s.setRole))
	s.handle("DELETE /api/roles/{name}", s.admin(s.deleteRole))
	s.handle("DELETE /api/grants", s.signedIn(s.revokeGrant))

	s.projectRoutes()
	s.memoryRoutes()
	s.alignRoutes()
	s.mcpRoutes()
	s.qaRoutes()
	s.termbaseRoutes()
	s.handle("GET /api/events", s.signedIn(s.events))

	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "ok\n")
	})
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "notFound", "There is no such address in the API.")
	})
	if s.cfg.Web != nil {
		s.mux.Handle("/", webApp(s.cfg.Web))
	}
}

func (s *Server) handle(pattern string, h http.HandlerFunc) { s.mux.HandleFunc(pattern, h) }

// ServeHTTP answers a request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	if len(s.cfg.Hosts) > 0 && !slices.Contains(s.cfg.Hosts, strings.ToLower(r.Host)) {
		writeError(w, http.StatusMisdirectedRequest, "forbidden", "This address is not served here.")
		return
	}
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		h.Set("Cache-Control", "no-store")
		if origin := r.Header.Get("Origin"); origin != "" && s.origins[origin] {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID, If-Match, X-Interlinea, X-Interlinea-Session")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}
	s.mux.ServeHTTP(w, r)
}

// webApp serves the web app's files, and its page for any other path so
// the app's own addresses work on reload.
func webApp(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if st, err := fs.Stat(files, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// clientIP is the address a request came from.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Request bodies are limited; files come as their bytes in base64, so the
// limit for those routes is larger.
const (
	maxBody     = 4 << 20
	maxFileBody = 256 << 20
)

func readJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "badRequest", "Send JSON (Content-Type: application/json).")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "tooLarge", fmt.Sprintf("The request is larger than %d MB.", limit>>20))
			return false
		}
		writeError(w, http.StatusBadRequest, "badRequest", "The request is not valid JSON for this address: "+err.Error())
		return false
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, "badRequest", "The request has more than one JSON value.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

// apiError is the body of every error answer.
type apiError struct {
	// Code is stable for the apps: badRequest, signIn, credentials,
	// forbidden, notFound, exists, conflict, invalid, badFile, inUse,
	// locked, closed, decided, needsResolution, notProject, notOnServer,
	// lastAdmin, tooMany, tooLarge, server.
	Code string `json:"code"`
	// Message says what went wrong, for people.
	Message string `json:"message"`
	// Problems are the problems of a refused request, by field.
	Problems []project.Problem `json:"problems,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Code: code, Message: msg})
}

// fail answers with the error a store returned.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var pinv *projectdb.InvalidError
	var ainv *authdb.InvalidError
	var fp *fileProblem
	switch {
	case errors.As(err, &fp):
		writeError(w, http.StatusUnprocessableEntity, "badFile", fp.msg)
	case errors.As(err, &pinv):
		msg := "The settings have problems."
		if len(pinv.Problems) > 0 {
			msg = pinv.Problems[0].Message
		}
		writeJSON(w, http.StatusUnprocessableEntity, apiError{Code: "invalid", Message: msg, Problems: pinv.Problems})
	case errors.As(err, &ainv):
		writeJSON(w, http.StatusUnprocessableEntity, apiError{Code: "invalid", Message: ainv.Message,
			Problems: []project.Problem{{Field: ainv.Field, Message: ainv.Message}}})
	case errors.Is(err, authdb.ErrNotFound), errors.Is(err, projectdb.ErrNotFound), errors.Is(err, tmdb.ErrNotFound), errors.Is(err, termdb.ErrNotFound):
		writeError(w, http.StatusNotFound, "notFound", "It doesn't exist, or it was deleted.")
	case errors.Is(err, authdb.ErrExists):
		writeError(w, http.StatusConflict, "exists", "The username is taken.")
	case errors.Is(err, projectdb.ErrExists):
		writeError(w, http.StatusConflict, "exists", "A project with this name exists already.")
	case errors.Is(err, tmdb.ErrExists):
		writeError(w, http.StatusConflict, "exists", "A memory with this name exists already.")
	case errors.Is(err, termdb.ErrExists):
		writeError(w, http.StatusConflict, "exists", "A termbase with this name exists already.")
	case errors.Is(err, authdb.ErrConflict), errors.Is(err, projectdb.ErrConflict), errors.Is(err, tmdb.ErrConflict), errors.Is(err, termdb.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "Someone else saved a change in the meantime. Load it again and redo yours.")
	case errors.Is(err, projectdb.ErrLocked):
		writeError(w, http.StatusConflict, "locked", "The segment is locked.")
	case errors.Is(err, authdb.ErrRoleInUse):
		writeError(w, http.StatusConflict, "inUse", "People have this role. Give them another role first.")
	case errors.Is(err, authdb.ErrLastAdmin):
		writeError(w, http.StatusConflict, "lastAdmin", "The server needs at least one active administrator.")
	case errors.Is(err, termdb.ErrNoTerms):
		writeJSON(w, http.StatusUnprocessableEntity, apiError{Code: "invalid", Message: "An entry needs at least one term.",
			Problems: []project.Problem{{Field: "languages", Message: "An entry needs at least one term."}}})
	case errors.Is(err, termdb.ErrInUse):
		writeError(w, http.StatusConflict, "inUse", "Entries still use it. Remove it from them first.")
	case errors.Is(err, termdb.ErrNotProject):
		writeError(w, http.StatusConflict, "notProject", "That is a master termbase, not a project termbase.")
	case errors.Is(err, termdb.ErrClosed):
		writeError(w, http.StatusConflict, "closed", "The merge request is closed.")
	case errors.Is(err, termdb.ErrDecided):
		writeError(w, http.StatusConflict, "decided", "The entry has been decided already.")
	case errors.Is(err, termdb.ErrNeedsResolution):
		writeError(w, http.StatusConflict, "needsResolution", "The master changed the same places. Resolve the conflicts first.")
	case errors.Is(err, context.Canceled):
		// The client went away; nobody reads the answer.
	default:
		s.cfg.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "server", "The server could not do this. The error is in its log.")
	}
}

func invalid(w http.ResponseWriter, field, msg string) {
	writeJSON(w, http.StatusUnprocessableEntity, apiError{Code: "invalid", Message: msg, Problems: []project.Problem{{Field: field, Message: msg}}})
}

func forbidden(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusForbidden, "forbidden", msg)
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "notFound", "It doesn't exist, or it was deleted.")
		return 0, false
	}
	return id, true
}

// queryInt reads a whole number from the query string, with a default.
func queryInt(w http.ResponseWriter, r *http.Request, name string, def, min, max int) (int, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		invalid(w, name, fmt.Sprintf("%s is a number from %d to %d.", name, min, max))
		return 0, false
	}
	return n, true
}

// version reads the version a write was made from (If-Match or the
// "version" query parameter).
func version(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := strings.Trim(r.Header.Get("If-Match"), `"W/`)
	if v == "" {
		v = r.URL.Query().Get("version")
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		writeError(w, http.StatusPreconditionRequired, "badRequest", "Say which version you changed (If-Match or ?version=).")
		return 0, false
	}
	return n, true
}
