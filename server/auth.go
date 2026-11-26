package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/auth/oidc"
)

// session is who a request comes from.
type session struct {
	User  authdb.User
	Token authdb.Token
	// Cookie is true when the request signed in with the web app's
	// cookie rather than an Authorization header.
	Cookie bool
	secret string
}

// The web app, served by the server itself, signs in with a cookie the
// page's scripts cannot read, so a script injected into the page cannot
// take the sign-in away. Apps ask for it with the X-Interlinea-Session:
// cookie header on a sign-in; every request signed in with the cookie
// must carry the X-Interlinea header, which another site's page cannot
// send without the server's permission (and SameSite=Strict keeps the
// browser from sending the cookie from another site at all).
const (
	sessionCookie = "interlinea_session"
	cookieHeader  = "X-Interlinea"
	askCookie     = "X-Interlinea-Session"
)

// wantsCookie reports whether a sign-in asks for the cookie.
func wantsCookie(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get(askCookie), "cookie")
}

// secure reports whether the browser reached the server over HTTPS.
func (s *Server) secure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return s.cfg.TrustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, secret string, expires time.Time) {
	maxAge := int(time.Until(expires).Seconds())
	if secret == "" || maxAge < 1 {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    secret,
		Path:     "/api/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

// credentials is the sign-in a request carries: the Authorization
// header's token, or the cookie.
func credentials(r *http.Request) (secret string, cookie bool) {
	if h := r.Header.Get("Authorization"); h != "" {
		t, _ := strings.CutPrefix(h, "Bearer ")
		return strings.TrimSpace(t), false
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value, true
	}
	return "", false
}

// by is how changes are signed in the stores' change logs.
func (m session) by() string { return m.User.Username }

type handler func(w http.ResponseWriter, r *http.Request, me session)

// anyone is a route open without signing in.
func (s *Server) anyone(h http.HandlerFunc) http.HandlerFunc { return h }

// signedIn is a route that needs a valid token.
func (s *Server) signedIn(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me, ok := s.authenticate(w, r)
		if ok {
			h(w, r, me)
		}
	}
}

// admin is a route for administrators.
func (s *Server) admin(h handler) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request, me session) {
		if !me.User.Admin {
			forbidden(w, "Only an administrator can do this.")
			return
		}
		h(w, r, me)
	})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (session, bool) {
	secret, cookie := credentials(r)
	if secret == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="interlinea"`)
		writeError(w, http.StatusUnauthorized, "signIn", "Sign in first.")
		return session{}, false
	}
	if cookie && r.Header.Get(cookieHeader) == "" {
		forbidden(w, "A request signed in with the web app's cookie must carry the X-Interlinea header.")
		return session{}, false
	}
	u, t, err := s.cfg.Auth.Authenticate(r.Context(), secret)
	if errors.Is(err, authdb.ErrToken) {
		if cookie {
			s.setSessionCookie(w, r, "", time.Time{})
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="interlinea", error="invalid_token"`)
		writeError(w, http.StatusUnauthorized, "signIn", "The sign-in has ended. Sign in again.")
		return session{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return session{}, false
	}
	return session{User: u, Token: t, Cookie: cookie, secret: secret}, true
}

// randomCode is a secret for a sign-in flow or exchange.
func randomCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tooMany(w http.ResponseWriter, wait time.Duration) {
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "tooMany", fmt.Sprintf("Too many sign-in attempts. Try again in %s.", (time.Duration(secs)*time.Second).String()))
}

// deviceName is the name a sign-in is listed under.
func deviceName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" {
		return "Interlinea"
	}
	if r := []rune(s); len(r) > 100 {
		s = string(r[:100])
	}
	return s
}

// signinResult is the answer to a sign-in.
type signinResult struct {
	// Token goes into the Authorization header of every request. It is
	// shown once, and not at all to a sign-in that asked for the cookie.
	Token     string      `json:"token,omitempty"`
	ExpiresAt time.Time   `json:"expiresAt"`
	User      authdb.User `json:"user"`
}

// issue signs u in: with a token in the answer, or with the cookie when
// cookie is true.
func (s *Server) issue(w http.ResponseWriter, r *http.Request, u authdb.User, device string, cookie bool) {
	secret, t, err := s.cfg.Auth.IssueToken(r.Context(), u.ID, deviceName(device), s.cfg.TokenTTL)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if cookie {
		s.setSessionCookie(w, r, secret, t.ExpiresAt)
		secret = ""
	}
	writeJSON(w, http.StatusOK, signinResult{Token: secret, ExpiresAt: t.ExpiresAt, User: u})
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Provider names the identity provider people can sign in with.
	Provider string `json:"provider,omitempty"`
	// Setup is true while the server has no accounts and the first
	// administrator can be made with the setup token.
	Setup bool `json:"setup"`
}

// serverInfo says what the server is: its version, the identity provider
// people can sign in with, and whether it is waiting for its first
// administrator.
//
// Access: anyone.
func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) {
	info := serverInfo{Name: "Interlinea", Version: s.cfg.Version}
	if s.cfg.OIDC != nil {
		info.Provider = s.cfg.OIDC.Name
	}
	if s.cfg.SetupToken != "" {
		n, err := s.cfg.Auth.CountUsers(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		info.Setup = n == 0
	}
	writeJSON(w, http.StatusOK, info)
}

type setupRequest struct {
	SetupToken string `json:"setupToken"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Device     string `json:"device"`
}

// setup makes the first administrator, with the setup token the server logged
// at its first start.
// It works only while the server has no accounts. Like the other sign-ins it
// answers with a token, or sets the web app's cookie when asked to
// (X-Interlinea-Session: cookie).
//
// Access: anyone with the setup token; attempts are limited per address.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if wait := s.signinIP.wait(ip); wait > 0 {
		tooMany(w, wait)
		return
	}
	s.signinIP.add(ip)
	var req setupRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if s.cfg.SetupToken == "" || subtle.ConstantTimeCompare([]byte(req.SetupToken), []byte(s.cfg.SetupToken)) != 1 {
		forbidden(w, "The setup token is wrong. It is in the server's log.")
		return
	}
	if req.Password == "" {
		invalid(w, "password", "The first administrator needs a password.")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.cfg.Auth.CountUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "exists", "The server is set up already. Sign in.")
		return
	}
	u, err := s.cfg.Auth.CreateUser(r.Context(), authdb.NewUser{Username: req.Username, Name: req.Name, Email: req.Email, Password: req.Password, Admin: true}, "setup")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.cfg.Log.Info("first administrator made", "username", u.Username, "from", ip)
	s.issue(w, r, u, req.Device, wantsCookie(r))
}

type passwordSignin struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Device   string `json:"device"`
}

// signinPassword signs in with a username and password.
// The answer has a token for the Authorization header, or, when the request
// has X-Interlinea-Session: cookie, sets the web app's HttpOnly cookie instead
// and leaves the token out. Attempts are limited per address and per account.
//
// Access: anyone.
func (s *Server) signinPassword(w http.ResponseWriter, r *http.Request) {
	var req passwordSignin
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	ip := s.clientIP(r)
	userKey := strings.ToLower(strings.TrimSpace(req.Username))
	for _, c := range []struct {
		l   *limiter
		key string
	}{{s.signinIP, ip}, {s.signinUser, userKey}} {
		if wait := c.l.wait(c.key); wait > 0 {
			s.cfg.Log.Warn("sign-in throttled", "username", userKey, "from", ip)
			tooMany(w, wait)
			return
		}
	}
	s.signinIP.add(ip)
	u, err := s.cfg.Auth.CheckPassword(r.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, authdb.ErrCredentials):
		s.signinUser.add(userKey)
		s.cfg.Log.Warn("sign-in refused", "username", userKey, "from", ip)
		writeError(w, http.StatusUnauthorized, "credentials", "The username or password is wrong.")
		return
	case errors.Is(err, authdb.ErrDisabled):
		s.signinUser.add(userKey)
		forbidden(w, "This account is disabled. Ask an administrator.")
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	s.signinUser.reset(userKey)
	s.issue(w, r, u, req.Device, wantsCookie(r))
}

// signout ends the sign-in the request was made with, and clears the web app's
// cookie if it signed in with it.
//
// Access: signed in.
func (s *Server) signout(w http.ResponseWriter, r *http.Request, me session) {
	if err := s.cfg.Auth.RevokeToken(r.Context(), me.User.ID, me.Token.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if me.Cookie {
		s.setSessionCookie(w, r, "", time.Time{})
	}
	w.WriteHeader(http.StatusNoContent)
}

// Sign-in through a provider: the app asks for the provider's address
// (POST /api/auth/oidc/start), opens it in a browser, and the provider
// sends the browser back to the callback. The callback sends it on to
// the app's return address with a one-time code in the fragment
// (#interlinea-code=...), which the app exchanges for a token
// (POST /api/auth/exchange). Tokens never appear in an address.

const (
	flowTTL     = 10 * time.Minute
	exchangeTTL = 2 * time.Minute
)

// pendingFlow is a sign-in through the provider under way, kept in the
// accounts store so any server instance can finish it.
type pendingFlow struct {
	Flow oidc.Flow `json:"flow"`
	// Back is where the browser goes afterwards.
	Back   string `json:"back"`
	Device string `json:"device"`
	// LinkTo is the account to link the identity to; 0 to sign in.
	LinkTo   int64  `json:"linkTo,omitempty"`
	LinkedBy string `json:"linkedBy,omitempty"`
}

// returnURL checks where a provider sign-in may send the browser back
// to: the server itself, an allowed web app, or an app on this computer
// listening on a loopback address.
func (s *Server) returnURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, false
	}
	switch {
	case s.origins[u.Scheme+"://"+u.Host]:
		return u, true
	case u.Scheme == "http":
		host := u.Hostname()
		if host == "localhost" {
			return u, true
		}
		ip := net.ParseIP(host)
		return u, ip != nil && ip.IsLoopback()
	}
	return nil, false
}

func (s *Server) startFlow(w http.ResponseWriter, r *http.Request, back, device string, linkTo int64, linkedBy string) {
	if s.cfg.OIDC == nil {
		writeError(w, http.StatusNotFound, "notFound", "This server has no identity provider to sign in with.")
		return
	}
	u, ok := s.returnURL(back)
	if !ok {
		invalid(w, "return", "The return address must be this server, an allowed web app, or an app on this computer (http://127.0.0.1:port).")
		return
	}
	f, err := s.cfg.OIDC.Provider.Start()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data, err := json.Marshal(pendingFlow{Flow: f, Back: u.String(), Device: device, LinkTo: linkTo, LinkedBy: linkedBy})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	switch err := s.cfg.Auth.SaveFlow(r.Context(), f.State, data, s.cfg.Now().Add(flowTTL)); {
	case errors.Is(err, authdb.ErrTooMany):
		tooMany(w, time.Minute)
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": f.URL})
}

type oidcStartRequest struct {
	// Return is where the browser goes afterwards.
	Return string `json:"return"`
	Device string `json:"device"`
}

// oidcStart begins a sign-in through the identity provider and answers with
// the provider's address to open in a browser.
// The provider sends the browser to the callback, which sends it on to the
// return address with a one-time code in the fragment (#interlinea-code=...);
// the app trades the code for a sign-in at /api/auth/exchange.
//
// Access: anyone, when the server has an identity provider.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if wait := s.signinIP.wait(ip); wait > 0 {
		tooMany(w, wait)
		return
	}
	s.signinIP.add(ip)
	var req oidcStartRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	s.startFlow(w, r, req.Return, req.Device, 0, "")
}

// back sends the browser to the app with a result in the fragment.
func back(w http.ResponseWriter, r *http.Request, to *url.URL, key, value string) {
	u := *to
	u.Fragment = ""
	u.RawFragment = ""
	http.Redirect(w, r, u.String()+"#"+key+"="+url.QueryEscape(value), http.StatusSeeOther)
}

// dead end page, when there is no app to send the browser back to.
func deadEnd(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>Interlinea sign-in</title><p>%s</p><p>Close this window and sign in again from Interlinea.</p>\n", html.EscapeString(msg))
}

// oidcCallback takes the browser back from the identity provider and sends it
// on to the app's return address, with a one-time code to sign in
// (#interlinea-code=) or to confirm a link (#interlinea-link=), or an error (#interlinea-error=) in
// the fragment. A sign-in that expired or was used already gets a page that
// says so.
//
// Access: anyone; the state ties it to a sign-in begun at oidc/start.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	if s.cfg.OIDC == nil || state == "" {
		deadEnd(w, "This sign-in has expired or was already used.")
		return
	}
	data, err := s.cfg.Auth.TakeFlow(r.Context(), state)
	if errors.Is(err, authdb.ErrNotFound) {
		deadEnd(w, "This sign-in has expired or was already used.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var f pendingFlow
	if err := json.Unmarshal(data, &f); err != nil {
		s.fail(w, r, err)
		return
	}
	backTo, err := url.Parse(f.Back)
	if err != nil {
		deadEnd(w, "This sign-in has expired or was already used.")
		return
	}
	if e := q.Get("error"); e != "" {
		msg := q.Get("error_description")
		if msg == "" {
			msg = e
		}
		back(w, r, backTo, "interlinea-error", "The identity provider refused the sign-in: "+msg)
		return
	}
	claims, err := s.cfg.OIDC.Provider.Finish(r.Context(), f.Flow, state, q.Get("code"))
	if err != nil {
		s.cfg.Log.Warn("provider sign-in failed", "err", err)
		back(w, r, backTo, "interlinea-error", "The sign-in with the identity provider could not be checked.")
		return
	}
	ctx := r.Context()
	if f.LinkTo != 0 {
		// The identity is not linked yet: whoever signed in at the
		// provider may not be the one who began the linking (they may
		// have been sent its address). The browser gets a one-time code,
		// and the link is made when the account that began it confirms
		// the code (POST /api/me/identities/confirm).
		code, err := randomCode()
		if err == nil {
			var data []byte
			data, err = json.Marshal(pendingLink{UserID: f.LinkTo, Issuer: claims.Issuer, Subject: claims.Subject, LinkedBy: f.LinkedBy})
			if err == nil {
				err = s.cfg.Auth.SaveFlow(ctx, linkKey(code), data, s.cfg.Now().Add(exchangeTTL))
			}
		}
		if err != nil {
			s.cfg.Log.Error("linking an identity failed", "err", err)
			back(w, r, backTo, "interlinea-error", "The identity could not be linked.")
			return
		}
		back(w, r, backTo, "interlinea-link", code)
		return
	}
	u, err := s.cfg.Auth.UserByIdentity(ctx, claims.Issuer, claims.Subject)
	if errors.Is(err, authdb.ErrNotFound) && s.cfg.OIDC.CreateUsers {
		u, err = s.createFromClaims(ctx, claims)
	}
	switch {
	case errors.Is(err, authdb.ErrNotFound):
		back(w, r, backTo, "interlinea-error", "No account on this server is linked to this sign-in. Ask an administrator to link it, or sign in with your password and link it yourself.")
		return
	case err != nil:
		s.cfg.Log.Error("provider sign-in failed", "err", err)
		back(w, r, backTo, "interlinea-error", "The server could not sign you in. The error is in its log.")
		return
	case u.Disabled:
		back(w, r, backTo, "interlinea-error", "This account is disabled. Ask an administrator.")
		return
	}
	code, err := randomCode()
	if err == nil {
		err = s.cfg.Auth.SaveCode(ctx, code, u.ID, f.Device, s.cfg.Now().Add(exchangeTTL))
	}
	if err != nil {
		s.cfg.Log.Error("provider sign-in failed", "err", err)
		back(w, r, backTo, "interlinea-error", "The server could not sign you in.")
		return
	}
	back(w, r, backTo, "interlinea-code", code)
}

// createFromClaims makes an account for a person the provider signed in.
func (s *Server) createFromClaims(ctx context.Context, c oidc.Claims) (authdb.User, error) {
	base := c.PreferredUsername
	if base == "" && c.EmailVerified {
		base = c.Email
	}
	base = strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-@", r) {
			return r
		}
		return -1
	}, base)
	if base == "" {
		base = "user"
	}
	if r := []rune(base); len(r) > 56 {
		base = string(r[:56])
	}
	email := ""
	if c.EmailVerified {
		email = c.Email
	}
	for i := 1; i <= 100; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s%d", base, i)
		}
		u, err := s.cfg.Auth.CreateUser(ctx, authdb.NewUser{Username: name, Name: c.Name, Email: email}, s.cfg.OIDC.Name)
		if errors.Is(err, authdb.ErrExists) {
			continue
		}
		var inv *authdb.InvalidError
		if errors.As(err, &inv) && inv.Field == "email" {
			u, err = s.cfg.Auth.CreateUser(ctx, authdb.NewUser{Username: name, Name: c.Name}, s.cfg.OIDC.Name)
		}
		if err != nil {
			return authdb.User{}, err
		}
		if err := s.cfg.Auth.LinkIdentity(ctx, u.ID, c.Issuer, c.Subject, s.cfg.OIDC.Name); err != nil {
			s.cfg.Auth.DeleteUser(ctx, u.ID, s.cfg.OIDC.Name)
			return authdb.User{}, err
		}
		s.cfg.Log.Info("account made for a provider sign-in", "username", u.Username)
		return u, nil
	}
	return authdb.User{}, fmt.Errorf("server: no free username for %q", base)
}

// exchange trades the one-time code from a provider sign-in for a sign-in: a
// token, or the web app's cookie when asked (X-Interlinea-Session: cookie).
// A code works once and for a few minutes.
//
// Access: anyone with a code.
func (s *Server) exchange(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if wait := s.signinIP.wait(ip); wait > 0 {
		tooMany(w, wait)
		return
	}
	s.signinIP.add(ip)
	var req struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if req.Code == "" {
		writeError(w, http.StatusUnauthorized, "signIn", "The sign-in has expired or was already used. Sign in again.")
		return
	}
	userID, device, err := s.cfg.Auth.TakeCode(r.Context(), req.Code)
	if errors.Is(err, authdb.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "signIn", "The sign-in has expired or was already used. Sign in again.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	u, err := s.cfg.Auth.User(r.Context(), userID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if u.Disabled {
		forbidden(w, "This account is disabled. Ask an administrator.")
		return
	}
	s.issue(w, r, u, device, wantsCookie(r))
}

type meResult struct {
	User       authdb.User       `json:"user"`
	Grants     []authdb.Grant    `json:"grants"`
	Identities []authdb.Identity `json:"identities"`
}

// me returns the signed-in person's account, roles and linked provider
// sign-ins.
//
// Access: signed in.
func (s *Server) me(w http.ResponseWriter, r *http.Request, me session) {
	gs, err := s.cfg.Auth.Grants(r.Context(), me.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids, err := s.cfg.Auth.Identities(r.Context(), me.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, meResult{User: me.User, Grants: nonNil(gs), Identities: nonNil(ids)})
}

// nonNil makes empty lists [] in JSON, not null.
func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

// changeOwnPassword sets the signed-in person's password, after checking the
// current one if they have one.
// Every other sign-in of theirs ends; this one continues with a new token (or
// cookie), which the answer carries.
//
// Access: signed in.
func (s *Server) changeOwnPassword(w http.ResponseWriter, r *http.Request, me session) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if req.New == "" {
		invalid(w, "new", "Choose the new password.")
		return
	}
	if me.User.HasPassword {
		key := me.User.Username
		if wait := s.signinUser.wait(key); wait > 0 {
			tooMany(w, wait)
			return
		}
		if _, err := s.cfg.Auth.CheckPassword(r.Context(), me.User.Username, req.Current); err != nil {
			if errors.Is(err, authdb.ErrCredentials) {
				s.signinUser.add(key)
				invalid(w, "current", "The current password is wrong.")
				return
			}
			s.fail(w, r, err)
			return
		}
	}
	if err := s.cfg.Auth.SetPassword(r.Context(), me.User.ID, req.New, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	// Setting a password signs out every device; this one stays signed in
	// with a new token.
	u, err := s.cfg.Auth.User(r.Context(), me.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.issue(w, r, u, me.Token.Name, me.Cookie || wantsCookie(r))
}

// ownTokens lists the signed-in person's sign-ins (devices), marking the one
// the request was made with.
//
// Access: signed in.
func (s *Server) ownTokens(w http.ResponseWriter, r *http.Request, me session) {
	ts, err := s.cfg.Auth.Tokens(r.Context(), me.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type listed struct {
		authdb.Token
		Current bool `json:"current"`
	}
	out := make([]listed, len(ts))
	for i, t := range ts {
		out[i] = listed{Token: t, Current: t.ID == me.Token.ID}
	}
	writeJSON(w, http.StatusOK, out)
}

// revokeOwnToken ends one of the signed-in person's sign-ins.
//
// Access: signed in; only their own sign-ins.
func (s *Server) revokeOwnToken(w http.ResponseWriter, r *http.Request, me session) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.cfg.Auth.RevokeToken(r.Context(), me.User.ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// linkOwnIdentity begins linking a provider sign-in to the signed-in person's
// account, and answers with the provider's address to open.
// The callback sends the browser back with a one-time code in the fragment
// (#interlinea-link=), which the app confirms at /api/me/identities/confirm.
//
// Access: signed in, when the server has an identity provider.
func (s *Server) linkOwnIdentity(w http.ResponseWriter, r *http.Request, me session) {
	var req oidcStartRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	s.startFlow(w, r, req.Return, "", me.User.ID, me.by())
}

// pendingLink is a provider sign-in waiting to be linked to an account.
type pendingLink struct {
	UserID   int64  `json:"userId"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
	LinkedBy string `json:"linkedBy"`
}

// linkKey is where a pending link is kept: by a hash of its code, so the
// store holds nothing that confirms it.
func linkKey(code string) string {
	h := sha256.Sum256([]byte(code))
	return "link:" + hex.EncodeToString(h[:])
}

// confirmOwnIdentity links the provider sign-in that the one-time code
// from the callback (#interlinea-link=) stands for to the signed-in
// person's account, if they began the linking. A code works once and for
// a few minutes.
//
// Access: signed in; only the account that began the linking.
func (s *Server) confirmOwnIdentity(w http.ResponseWriter, r *http.Request, me session) {
	var req struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	gone := func() {
		writeError(w, http.StatusNotFound, "notFound", "The linking has expired or was already used. Link the sign-in again.")
	}
	if req.Code == "" {
		gone()
		return
	}
	data, err := s.cfg.Auth.TakeFlow(r.Context(), linkKey(req.Code))
	if errors.Is(err, authdb.ErrNotFound) {
		gone()
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var l pendingLink
	if err := json.Unmarshal(data, &l); err != nil {
		s.fail(w, r, err)
		return
	}
	if l.UserID != me.User.ID {
		s.cfg.Log.Warn("an identity link confirmed by another account", "began", l.UserID, "confirmed", me.User.ID)
		forbidden(w, "Another account began this linking. Link the sign-in from your own account.")
		return
	}
	if err := s.cfg.Auth.LinkIdentity(r.Context(), me.User.ID, l.Issuer, l.Subject, l.LinkedBy); err != nil {
		s.fail(w, r, err)
		return
	}
	name := ""
	if s.cfg.OIDC != nil {
		name = s.cfg.OIDC.Name
	}
	writeJSON(w, http.StatusOK, map[string]string{"linked": name})
}

// unlinkOwnIdentity removes a provider sign-in (?issuer=&subject=) from the
// signed-in person's account.
// An account without a password keeps its last one.
//
// Access: signed in.
func (s *Server) unlinkOwnIdentity(w http.ResponseWriter, r *http.Request, me session) {
	q := r.URL.Query()
	if err := s.cfg.Auth.UnlinkIdentity(r.Context(), me.User.ID, q.Get("issuer"), q.Get("subject"), me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// person is what everyone signed in may see of an account, to choose whom
// work goes to.
type person struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// people lists the accounts that are not disabled, by name, to give work to
// and to name in grants.
//
// Access: signed in.
func (s *Server) people(w http.ResponseWriter, r *http.Request, me session) {
	us, err := s.cfg.Auth.Users(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []person{}
	for _, u := range us {
		if !u.Disabled {
			out = append(out, person{ID: u.ID, Username: u.Username, Name: u.Name})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
