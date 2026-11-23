// Package oidc signs people in through an OpenID Connect provider (Entra
// ID, Google, Keycloak, Authentik, ...) with the authorization code flow
// and PKCE: the server sends the browser to the provider with a state, a
// nonce and a code challenge, takes back the code, exchanges it for an ID
// token, and verifies the token against the provider's published keys.
//
// Only the standard library is used. ID tokens signed with RS256, PS256
// or ES256 are accepted; "none" and shared-secret algorithms never are.
package oidc

import (
	"context"
	"crypto"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Errors returned by this package.
var (
	// ErrInvalidToken means the ID token is not one this client may trust.
	ErrInvalidToken = errors.New("oidc: invalid ID token")
	// ErrProvider means the provider answered with something unusable.
	ErrProvider = errors.New("oidc: provider error")
)

// Config describes a provider registration.
type Config struct {
	// Issuer is the provider's issuer URL, exactly as in its tokens.
	Issuer string
	// ClientID and ClientSecret are the client's registration; the
	// secret is empty for a public client.
	ClientID     string
	ClientSecret string
	// RedirectURL is where the provider sends the browser back.
	RedirectURL string
	// Scopes are requested besides "openid" (default "email profile").
	Scopes []string
	// HTTP is the client for the provider (default: 10 s timeout).
	HTTP *http.Client
}

// Metadata is the part of the provider's discovery document used here.
type Metadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
}

// Provider is a discovered provider. It is safe for concurrent use.
type Provider struct {
	cfg  Config
	meta Metadata
	now  func() time.Time

	mu       sync.Mutex
	keys     map[string]crypto.PublicKey
	keyless  []crypto.PublicKey // keys without a kid
	fetched  time.Time
	fetchErr error
}

const (
	maxBody = 1 << 20
	// clockSkew is allowed between this server's and the provider's clocks.
	clockSkew = time.Minute
	// refetchAfter limits key refetches for unknown key IDs.
	refetchAfter = 5 * time.Minute
)

func (c Config) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func getJSON(ctx context.Context, hc *http.Client, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProvider, err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s answered %d", ErrProvider, u, res.StatusCode)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrProvider, u, err)
	}
	return nil
}

// Discover reads the provider's discovery document and checks it is the
// issuer configured.
func Discover(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, errors.New("oidc: the issuer, client ID and redirect URL are needed")
	}
	issuer := strings.TrimSuffix(cfg.Issuer, "/")
	var m Metadata
	if err := getJSON(ctx, cfg.client(), issuer+"/.well-known/openid-configuration", &m); err != nil {
		return nil, err
	}
	if strings.TrimSuffix(m.Issuer, "/") != issuer {
		return nil, fmt.Errorf("%w: the discovery document is for issuer %q, not %q", ErrProvider, m.Issuer, cfg.Issuer)
	}
	if m.AuthorizationEndpoint == "" || m.TokenEndpoint == "" || m.JWKSURI == "" {
		return nil, fmt.Errorf("%w: the discovery document lacks an endpoint", ErrProvider)
	}
	if len(m.CodeChallengeMethods) > 0 && !contains(m.CodeChallengeMethods, "S256") {
		return nil, fmt.Errorf("%w: the provider does not support PKCE with S256", ErrProvider)
	}
	cfg.Issuer = m.Issuer
	return &Provider{cfg: cfg, meta: m, now: time.Now}, nil
}

// Metadata returns what discovery found.
func (p *Provider) Metadata() Metadata { return p.meta }

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func randomString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Flow is one sign-in in progress: what goes to the provider and what
// must be kept (server-side, never in the browser) until it comes back.
type Flow struct {
	State    string
	Nonce    string
	Verifier string
	// URL sends the browser to the provider.
	URL string
}

// Start begins a sign-in.
func (p *Provider) Start() (Flow, error) {
	var f Flow
	var err error
	if f.State, err = randomString(); err != nil {
		return f, err
	}
	if f.Nonce, err = randomString(); err != nil {
		return f, err
	}
	if f.Verifier, err = randomString(); err != nil {
		return f, err
	}
	sum := sha256.Sum256([]byte(f.Verifier))
	scopes := p.cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"email", "profile"}
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.cfg.ClientID},
		"redirect_uri":          {p.cfg.RedirectURL},
		"scope":                 {"openid " + strings.Join(scopes, " ")},
		"state":                 {f.State},
		"nonce":                 {f.Nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	sep := "?"
	if strings.Contains(p.meta.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	f.URL = p.meta.AuthorizationEndpoint + sep + q.Encode()
	return f, nil
}

// Claims are what an ID token says about the person.
type Claims struct {
	Issuer            string `json:"iss"`
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

// Finish completes a sign-in: it checks the state the provider returned
// against the flow's, exchanges the code and verifies the ID token.
func (p *Provider) Finish(ctx context.Context, f Flow, state, code string) (Claims, error) {
	if f.State == "" || subtle.ConstantTimeCompare([]byte(state), []byte(f.State)) != 1 {
		return Claims{}, errors.New("oidc: the sign-in came back with another state")
	}
	if code == "" {
		return Claims{}, errors.New("oidc: the provider sent no code")
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.cfg.RedirectURL},
		"code_verifier": {f.Verifier},
	}
	if p.cfg.ClientSecret == "" {
		form.Set("client_id", p.cfg.ClientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Claims{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if p.cfg.ClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(p.cfg.ClientID), url.QueryEscape(p.cfg.ClientSecret))
	}
	res, err := p.cfg.client().Do(req)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	var tok struct {
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return Claims{}, fmt.Errorf("%w: token answer: %v", ErrProvider, err)
	}
	if res.StatusCode != http.StatusOK || tok.Error != "" {
		return Claims{}, fmt.Errorf("%w: the code was refused (%s: %s)", ErrProvider, tok.Error, tok.Description)
	}
	if tok.IDToken == "" {
		return Claims{}, fmt.Errorf("%w: no ID token in the answer", ErrProvider)
	}
	return p.Verify(ctx, tok.IDToken, f.Nonce)
}

type header struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type payload struct {
	Claims
	Audience  audience `json:"aud"`
	AZP       string   `json:"azp"`
	Expires   *float64 `json:"exp"`
	IssuedAt  *float64 `json:"iat"`
	NotBefore *float64 `json:"nbf"`
	Nonce     string   `json:"nonce"`
}

// audience is a string or a list of strings.
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidToken, fmt.Sprintf(format, args...))
}

// Verify checks an ID token: its signature against the provider's keys,
// its issuer, audience, times and nonce.
func (p *Provider) Verify(ctx context.Context, idToken, nonce string) (Claims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return Claims{}, invalid("not a signed JWT")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, invalid("header: %v", err)
	}
	var h header
	if err := json.Unmarshal(hb, &h); err != nil {
		return Claims{}, invalid("header: %v", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, invalid("signature: %v", err)
	}
	signed := []byte(parts[0] + "." + parts[1])
	if err := p.checkSignature(ctx, h, signed, sig); err != nil {
		return Claims{}, err
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, invalid("payload: %v", err)
	}
	var c payload
	if err := json.Unmarshal(pb, &c); err != nil {
		return Claims{}, invalid("payload: %v", err)
	}
	now := p.now()
	switch {
	case c.Issuer != p.cfg.Issuer:
		return Claims{}, invalid("issued by %q, not %q", c.Issuer, p.cfg.Issuer)
	case c.Subject == "":
		return Claims{}, invalid("no subject")
	case !contains(c.Audience, p.cfg.ClientID):
		return Claims{}, invalid("not for this client")
	case len(c.Audience) > 1 && c.AZP != p.cfg.ClientID:
		return Claims{}, invalid("several audiences and another authorized party")
	case c.Expires == nil:
		return Claims{}, invalid("no expiry")
	case !now.Before(unix(*c.Expires).Add(clockSkew)):
		return Claims{}, invalid("expired")
	case c.IssuedAt != nil && unix(*c.IssuedAt).After(now.Add(clockSkew)):
		return Claims{}, invalid("issued in the future")
	case c.NotBefore != nil && unix(*c.NotBefore).After(now.Add(clockSkew)):
		return Claims{}, invalid("not valid yet")
	case nonce == "" || subtle.ConstantTimeCompare([]byte(c.Nonce), []byte(nonce)) != 1:
		return Claims{}, invalid("another nonce")
	}
	return c.Claims, nil
}

func unix(f float64) time.Time {
	sec := int64(f)
	return time.Unix(sec, int64((f-float64(sec))*1e9))
}

func (p *Provider) checkSignature(ctx context.Context, h header, signed, sig []byte) error {
	var verify func(crypto.PublicKey) bool
	switch h.Alg {
	case "RS256", "PS256":
		sum := sha256.Sum256(signed)
		verify = func(k crypto.PublicKey) bool {
			rk, ok := k.(*rsa.PublicKey)
			if !ok {
				return false
			}
			if h.Alg == "PS256" {
				return rsa.VerifyPSS(rk, crypto.SHA256, sum[:], sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) == nil
			}
			return rsa.VerifyPKCS1v15(rk, crypto.SHA256, sum[:], sig) == nil
		}
	case "ES256":
		sum := sha256.Sum256(signed)
		verify = func(k crypto.PublicKey) bool {
			ek, ok := k.(*ecdsa.PublicKey)
			if !ok || ek.Curve != elliptic.P256() || len(sig) != 64 {
				return false
			}
			r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
			return ecdsa.Verify(ek, sum[:], r, s)
		}
	default:
		return invalid("the algorithm %q is not accepted", h.Alg)
	}
	keys, err := p.keysFor(ctx, h.Kid, false)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if verify(k) {
			return nil
		}
	}
	// The provider may have rotated its keys since they were fetched.
	if fresh, err := p.keysFor(ctx, h.Kid, true); err == nil {
		for _, k := range fresh {
			if verify(k) {
				return nil
			}
		}
	}
	return invalid("the signature does not match the provider's keys")
}

// keysFor returns the keys a token with kid may be signed with, fetching
// the provider's keys when none are known yet, or when refresh is set and
// the last fetch is old enough.
func (p *Provider) keysFor(ctx context.Context, kid string, refresh bool) ([]crypto.PublicKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	stale := p.keys == nil || (refresh && p.now().Sub(p.fetched) >= refetchAfter)
	if stale {
		keys, keyless, err := fetchKeys(ctx, p.cfg.client(), p.meta.JWKSURI)
		p.fetched = p.now()
		if err != nil {
			p.fetchErr = err
			if p.keys == nil {
				return nil, err
			}
		} else {
			p.keys, p.keyless, p.fetchErr = keys, keyless, nil
		}
	}
	if kid != "" {
		if k, ok := p.keys[kid]; ok {
			return []crypto.PublicKey{k}, nil
		}
		return p.keyless, nil
	}
	all := append([]crypto.PublicKey(nil), p.keyless...)
	for _, k := range p.keys {
		all = append(all, k)
	}
	return all, nil
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func fetchKeys(ctx context.Context, hc *http.Client, u string) (map[string]crypto.PublicKey, []crypto.PublicKey, error) {
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := getJSON(ctx, hc, u, &set); err != nil {
		return nil, nil, err
	}
	keys := map[string]crypto.PublicKey{}
	var keyless []crypto.PublicKey
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pk, err := k.public()
		if err != nil {
			// A key of a kind this package does not use; others still count.
			continue
		}
		if k.Kid == "" {
			keyless = append(keyless, pk)
		} else {
			keys[k.Kid] = pk
		}
	}
	if len(keys) == 0 && len(keyless) == 0 {
		return nil, nil, fmt.Errorf("%w: the provider publishes no usable signing key", ErrProvider)
	}
	return keys, keyless, nil
}

func b64int(s string) (*big.Int, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("empty")
	}
	return new(big.Int).SetBytes(b), nil
}

func (k jwk) public() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		n, err := b64int(k.N)
		if err != nil {
			return nil, err
		}
		e, err := b64int(k.E)
		if err != nil {
			return nil, err
		}
		if !e.IsInt64() || e.Int64() < 3 || n.BitLen() < 2048 {
			return nil, errors.New("weak RSA key")
		}
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
	case "EC":
		if k.Crv != "P-256" {
			return nil, errors.New("unsupported curve")
		}
		x, err := b64int(k.X)
		if err != nil {
			return nil, err
		}
		y, err := b64int(k.Y)
		if err != nil {
			return nil, err
		}
		if x.BitLen() > 256 || y.BitLen() > 256 {
			return nil, errors.New("coordinate too long")
		}
		// crypto/ecdh refuses points that are not on the curve.
		point := make([]byte, 65)
		point[0] = 4
		x.FillBytes(point[1:33])
		y.FillBytes(point[33:])
		if _, err := ecdh.P256().NewPublicKey(point); err != nil {
			return nil, errors.New("point not on the curve")
		}
		return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
	}
	return nil, errors.New("unsupported key type")
}
