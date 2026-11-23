package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

var ctx = context.Background()

// provider is an OpenID Connect provider for the tests: discovery, keys
// and a token endpoint that checks the code, the PKCE verifier and the
// client's credentials.
type provider struct {
	t      *testing.T
	srv    *httptest.Server
	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey
	kid    string

	mu         sync.Mutex
	codes      map[string]code
	keyFetches int
	noPKCE     bool
	secret     string
}

type code struct {
	challenge string
	claims    map[string]any
}

func newProvider(t *testing.T) *provider {
	t.Helper()
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ek, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{t: t, rsaKey: rk, ecKey: ek, kid: "rsa-1", codes: map[string]code{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{
			"issuer":                 p.srv.URL,
			"authorization_endpoint": p.srv.URL + "/authorize",
			"token_endpoint":         p.srv.URL + "/token",
			"jwks_uri":               p.srv.URL + "/keys",
		}
		if !p.noPKCE {
			m["code_challenge_methods_supported"] = []string{"S256"}
		} else {
			m["code_challenge_methods_supported"] = []string{"plain"}
		}
		json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.keyFetches++
		kid := p.kid
		p.mu.Unlock()
		b := func(x *big.Int) string { return base64.RawURLEncoding.EncodeToString(x.Bytes()) }
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{
			{"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256", "n": b(p.rsaKey.N), "e": b(big.NewInt(int64(p.rsaKey.E)))},
			{"kty": "EC", "kid": "ec-1", "crv": "P-256", "x": b(p.ecKey.X), "y": b(p.ecKey.Y)},
			{"kty": "oct", "kid": "hmac", "k": "c2VjcmV0"},
			{"kty": "RSA", "kid": "enc", "use": "enc", "n": b(p.rsaKey.N), "e": "AQAB"},
		}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		fail := func(e string) {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": e, "error_description": "test provider"})
		}
		if p.secret != "" {
			// RFC 6749 2.3.1: both are form-encoded before Basic auth.
			rawID, rawSecret, ok := r.BasicAuth()
			id, _ := url.QueryUnescape(rawID)
			secret, _ := url.QueryUnescape(rawSecret)
			if !ok || id != "interlinea" || secret != p.secret {
				fail("invalid_client")
				return
			}
		} else if r.Form.Get("client_id") != "interlinea" {
			fail("invalid_client")
			return
		}
		p.mu.Lock()
		c, ok := p.codes[r.Form.Get("code")]
		delete(p.codes, r.Form.Get("code"))
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("redirect_uri") != "https://il.example/callback" ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge {
			fail("invalid_grant")
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id_token": p.sign("RS256", p.kid, c.claims), "token_type": "Bearer"})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// authorize does what the provider's login page does: it remembers the
// code challenge and hands out a code for the claims.
func (p *provider) authorize(t *testing.T, flowURL string, claims map[string]any) (state, authCode string) {
	t.Helper()
	u, err := url.Parse(flowURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" || !strings.HasPrefix(q.Get("scope"), "openid ") {
		t.Fatalf("authorization request %v", q)
	}
	if n, _ := claims["nonce"].(string); n == "" {
		claims["nonce"] = q.Get("nonce")
	}
	c := "code-" + q.Get("state")[:8]
	p.mu.Lock()
	p.codes[c] = code{challenge: q.Get("code_challenge"), claims: claims}
	p.mu.Unlock()
	return q.Get("state"), c
}

func (p *provider) sign(alg, kid string, claims map[string]any) string {
	h := map[string]string{"alg": alg, "typ": "JWT"}
	if kid != "" {
		h["kid"] = kid
	}
	hb, _ := json.Marshal(h)
	cb, _ := json.Marshal(claims)
	signed := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	sum := sha256.Sum256([]byte(signed))
	var sig []byte
	var err error
	switch alg {
	case "RS256":
		sig, err = rsa.SignPKCS1v15(rand.Reader, p.rsaKey, crypto.SHA256, sum[:])
	case "PS256":
		sig, err = rsa.SignPSS(rand.Reader, p.rsaKey, crypto.SHA256, sum[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	case "ES256":
		var r, s *big.Int
		r, s, err = ecdsa.Sign(rand.Reader, p.ecKey, sum[:])
		if err == nil {
			sig = make([]byte, 64)
			r.FillBytes(sig[:32])
			s.FillBytes(sig[32:])
		}
	default:
		sig = []byte("not a signature")
	}
	if err != nil {
		p.t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (p *provider) claims(nonce string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{
		"iss": p.srv.URL, "sub": "user-42", "aud": "interlinea",
		"exp": now + 300, "iat": now, "nonce": nonce,
		"email": "eva@agentur.example", "email_verified": true, "name": "Eva Brandt", "preferred_username": "eva",
	}
}

func (p *provider) discover(t *testing.T, secret string) *Provider {
	t.Helper()
	pr, err := Discover(ctx, Config{Issuer: p.srv.URL, ClientID: "interlinea", ClientSecret: secret, RedirectURL: "https://il.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	return pr
}

func TestCodeFlow(t *testing.T) {
	for _, secret := range []string{"", "s3cr3t:with/specials"} {
		p := newProvider(t)
		p.secret = secret
		pr := p.discover(t, secret)
		f, err := pr.Start()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(f.URL, p.srv.URL+"/authorize?") || f.State == f.Nonce || f.Verifier == "" {
			t.Fatalf("flow %+v", f)
		}
		state, code := p.authorize(t, f.URL, p.claims(""))
		c, err := pr.Finish(ctx, f, state, code)
		if err != nil {
			t.Fatalf("secret %q: %v", secret, err)
		}
		if c.Subject != "user-42" || c.Email != "eva@agentur.example" || !c.EmailVerified || c.Name != "Eva Brandt" || c.PreferredUsername != "eva" || c.Issuer != p.srv.URL {
			t.Fatalf("claims %+v", c)
		}
		// A code is used once.
		if _, err := pr.Finish(ctx, f, state, code); !errors.Is(err, ErrProvider) {
			t.Fatalf("code reused: %v", err)
		}
	}
}

func TestFinishChecks(t *testing.T) {
	p := newProvider(t)
	pr := p.discover(t, "")
	f, _ := pr.Start()
	state, code := p.authorize(t, f.URL, p.claims(""))
	if _, err := pr.Finish(ctx, f, state+"x", code); err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("other state: %v", err)
	}
	if _, err := pr.Finish(ctx, f, state, ""); err == nil {
		t.Fatal("no code accepted")
	}
	// A verifier other than the one the challenge was made from.
	g := f
	g.Verifier = "another verifier"
	if _, err := pr.Finish(ctx, g, state, code); !errors.Is(err, ErrProvider) {
		t.Fatalf("other verifier: %v", err)
	}
	// The nonce is checked: a token for another sign-in is refused.
	f2, _ := pr.Start()
	claims := p.claims("nonce of another sign-in")
	state2, code2 := p.authorize(t, f2.URL, claims)
	if _, err := pr.Finish(ctx, f2, state2, code2); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("other nonce: %v", err)
	}
}

func TestVerify(t *testing.T) {
	p := newProvider(t)
	pr := p.discover(t, "")
	const nonce = "n-1"
	ok := func(alg, kid string, change func(map[string]any)) error {
		c := p.claims(nonce)
		if change != nil {
			change(c)
		}
		_, err := pr.Verify(ctx, p.sign(alg, kid, c), nonce)
		return err
	}
	for _, alg := range []struct{ alg, kid string }{{"RS256", "rsa-1"}, {"PS256", "rsa-1"}, {"ES256", "ec-1"}, {"ES256", ""}} {
		if err := ok(alg.alg, alg.kid, nil); err != nil {
			t.Errorf("%s/%s: %v", alg.alg, alg.kid, err)
		}
	}
	now := time.Now().Unix()
	bad := map[string]struct {
		alg, kid string
		change   func(map[string]any)
	}{
		"alg none":            {"none", "rsa-1", nil},
		"shared secret":       {"HS256", "hmac", nil},
		"ES256 under RSA kid": {"ES256", "rsa-1", nil},
		"other issuer":        {"RS256", "rsa-1", func(c map[string]any) { c["iss"] = "https://evil.example" }},
		"no subject":          {"RS256", "rsa-1", func(c map[string]any) { delete(c, "sub") }},
		"other audience":      {"RS256", "rsa-1", func(c map[string]any) { c["aud"] = "someone-else" }},
		"several audiences, other azp": {"RS256", "rsa-1", func(c map[string]any) {
			c["aud"] = []string{"interlinea", "other"}
			c["azp"] = "other"
		}},
		"no expiry":        {"RS256", "rsa-1", func(c map[string]any) { delete(c, "exp") }},
		"expired":          {"RS256", "rsa-1", func(c map[string]any) { c["exp"] = now - 120 }},
		"issued in future": {"RS256", "rsa-1", func(c map[string]any) { c["iat"] = now + 600 }},
		"not valid yet":    {"RS256", "rsa-1", func(c map[string]any) { c["nbf"] = now + 600 }},
		"other nonce":      {"RS256", "rsa-1", func(c map[string]any) { c["nonce"] = "n-2" }},
		"no nonce":         {"RS256", "rsa-1", func(c map[string]any) { delete(c, "nonce") }},
	}
	for name, b := range bad {
		if err := ok(b.alg, b.kid, b.change); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Several audiences with this client as authorized party are fine;
	// a minute of clock skew is allowed.
	if err := ok("RS256", "rsa-1", func(c map[string]any) {
		c["aud"] = []string{"interlinea", "other"}
		c["azp"] = "interlinea"
		c["exp"] = now - 30
	}); err != nil {
		t.Fatalf("several audiences: %v", err)
	}
	// A changed payload breaks the signature.
	tok := p.sign("RS256", "rsa-1", p.claims(nonce))
	parts := strings.Split(tok, ".")
	other, _ := json.Marshal(map[string]any{"iss": p.srv.URL, "sub": "admin", "aud": "interlinea", "exp": now + 300, "nonce": nonce})
	forged := parts[0] + "." + base64.RawURLEncoding.EncodeToString(other) + "." + parts[2]
	if _, err := pr.Verify(ctx, forged, nonce); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("forged payload: %v", err)
	}
	for _, s := range []string{"", "a.b", "a.b.c.d", "!!.!!.!!"} {
		if _, err := pr.Verify(ctx, s, nonce); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestKeyRotation(t *testing.T) {
	p := newProvider(t)
	pr := p.discover(t, "")
	clock := time.Now()
	pr.now = func() time.Time { return clock }
	if _, err := pr.Verify(ctx, p.sign("RS256", "rsa-1", p.claims("n")), "n"); err != nil {
		t.Fatal(err)
	}
	// The provider rotates: same key material under a new ID here, which
	// stands for a new key the client has not seen.
	p.mu.Lock()
	p.kid = "rsa-2"
	p.mu.Unlock()
	fetches := func() int { p.mu.Lock(); defer p.mu.Unlock(); return p.keyFetches }
	before := fetches()
	tok := p.sign("RS256", "rsa-2", p.claims("n"))
	// Within the refetch interval the unknown key ID is not fetched again...
	if _, err := pr.Verify(ctx, tok, "n"); !errors.Is(err, ErrInvalidToken) || fetches() != before {
		t.Fatalf("refetched too early (%d fetches): %v", fetches()-before, err)
	}
	// ...after it, the new keys are fetched and the token verifies.
	clock = clock.Add(refetchAfter)
	if _, err := pr.Verify(ctx, tok, "n"); err != nil || fetches() != before+1 {
		t.Fatalf("after rotation (%d fetches): %v", fetches()-before, err)
	}
}

func TestDiscovery(t *testing.T) {
	p := newProvider(t)
	if _, err := Discover(ctx, Config{Issuer: p.srv.URL, ClientID: "interlinea"}); err == nil {
		t.Fatal("no redirect URL accepted")
	}
	if _, err := Discover(ctx, Config{Issuer: p.srv.URL + "/other", ClientID: "interlinea", RedirectURL: "https://il.example/callback"}); !errors.Is(err, ErrProvider) {
		t.Fatalf("missing document: %v", err)
	}
	// An issuer configured with a trailing slash is the same issuer.
	if _, err := Discover(ctx, Config{Issuer: p.srv.URL + "/", ClientID: "interlinea", RedirectURL: "https://il.example/callback"}); err != nil {
		t.Fatalf("trailing slash: %v", err)
	}
	p.noPKCE = true
	if _, err := Discover(ctx, Config{Issuer: p.srv.URL, ClientID: "interlinea", RedirectURL: "https://il.example/callback"}); !errors.Is(err, ErrProvider) {
		t.Fatalf("provider without S256: %v", err)
	}
	// A discovery document naming another issuer is refused.
	mux := http.NewServeMux()
	var other *httptest.Server
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": "https://evil.example", "authorization_endpoint": other.URL + "/a", "token_endpoint": other.URL + "/t", "jwks_uri": other.URL + "/k"})
	})
	other = httptest.NewServer(mux)
	defer other.Close()
	if _, err := Discover(ctx, Config{Issuer: other.URL, ClientID: "x", RedirectURL: "https://il.example/cb"}); !errors.Is(err, ErrProvider) {
		t.Fatalf("other issuer: %v", err)
	}
}

func TestWeakKeysIgnored(t *testing.T) {
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	b := func(x *big.Int) string { return base64.RawURLEncoding.EncodeToString(x.Bytes()) }
	if _, err := (jwk{Kty: "RSA", N: b(small.N), E: "AQAB"}).public(); err == nil {
		t.Fatal("1024-bit RSA key accepted")
	}
	if _, err := (jwk{Kty: "EC", Crv: "P-256", X: b(big.NewInt(1)), Y: b(big.NewInt(2))}).public(); err == nil {
		t.Fatal("point off the curve accepted")
	}
	if _, err := (jwk{Kty: "EC", Crv: "P-384", X: "AA", Y: "AA"}).public(); err == nil {
		t.Fatal("P-384 accepted")
	}
}
