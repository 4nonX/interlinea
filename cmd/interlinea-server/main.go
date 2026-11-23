// Command interlinea-server runs Interlinea's team server: projects,
// translation memories, termbases and accounts for the desktop and web
// apps, in SQLite files or a PostgreSQL database.
//
// Every option can also be given as an environment variable
// (INTERLINEA_ADDR for -addr, and so on); see -h.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/auth/oidc"
	"github.com/4nonX/interlinea/internal/pgdriver"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/server"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm/tmdb"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.Args[2:]))
	}
	if err := run(context.Background(), os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "interlinea-server:", err)
		os.Exit(1)
	}
}

// healthcheck asks a running server whether it answers, for container
// health checks (the image has no curl).
func healthcheck(args []string) int {
	addr := env("INTERLINEA_ADDR", ":8080")
	if len(args) > 0 {
		addr = args[0]
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := http.Client{Timeout: 5 * time.Second}
	res, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, res.Status)
		return 1
	}
	return 0
}

func env(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

// options are the command's settings.
type options struct {
	addr, data, postgres     string
	publicURL, origins       string
	oidcIssuer, oidcClient   string
	oidcSecret, oidcName     string
	oidcScopes               string
	oidcCreate, trustProxy   bool
	web, tlsCert, tlsKey     string
	tokenDays                int
	adminUser, adminPassword string
	logJSON                  bool
}

func parse(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("interlinea-server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	boolEnv := func(name string) bool { b, _ := strconv.ParseBool(env(name, "false")); return b }
	days, err := strconv.Atoi(env("INTERLINEA_TOKEN_DAYS", "30"))
	if err != nil {
		return o, fmt.Errorf("INTERLINEA_TOKEN_DAYS is not a number")
	}
	fs.StringVar(&o.addr, "addr", env("INTERLINEA_ADDR", ":8080"), "address to listen on (INTERLINEA_ADDR)")
	fs.StringVar(&o.data, "data", env("INTERLINEA_DATA", "data"), "folder of the SQLite files, when not on PostgreSQL (INTERLINEA_DATA)")
	fs.StringVar(&o.postgres, "postgres", env("INTERLINEA_POSTGRES", ""), "PostgreSQL connection string; each store gets a schema (INTERLINEA_POSTGRES)")
	fs.StringVar(&o.publicURL, "public-url", env("INTERLINEA_PUBLIC_URL", ""), "address the apps reach the server at, https://... (INTERLINEA_PUBLIC_URL)")
	fs.StringVar(&o.origins, "origins", env("INTERLINEA_ORIGINS", ""), "other web app addresses allowed to call the API, comma-separated (INTERLINEA_ORIGINS)")
	fs.StringVar(&o.oidcIssuer, "oidc-issuer", env("INTERLINEA_OIDC_ISSUER", ""), "OpenID Connect issuer, to sign in with Entra ID, Google, Keycloak... (INTERLINEA_OIDC_ISSUER)")
	fs.StringVar(&o.oidcClient, "oidc-client-id", env("INTERLINEA_OIDC_CLIENT_ID", ""), "client ID registered at the provider (INTERLINEA_OIDC_CLIENT_ID)")
	fs.StringVar(&o.oidcName, "oidc-name", env("INTERLINEA_OIDC_NAME", "your organization"), "provider name on the sign-in button (INTERLINEA_OIDC_NAME)")
	fs.StringVar(&o.oidcScopes, "oidc-scopes", env("INTERLINEA_OIDC_SCOPES", "openid,email,profile"), "scopes asked for, comma-separated (INTERLINEA_OIDC_SCOPES)")
	fs.BoolVar(&o.oidcCreate, "oidc-create-users", boolEnv("INTERLINEA_OIDC_CREATE_USERS"), "make an account (without roles) for people the provider signs in (INTERLINEA_OIDC_CREATE_USERS)")
	fs.BoolVar(&o.trustProxy, "trust-proxy", boolEnv("INTERLINEA_TRUST_PROXY"), "take client addresses from X-Forwarded-For (INTERLINEA_TRUST_PROXY)")
	fs.StringVar(&o.web, "web", env("INTERLINEA_WEB", ""), "folder of the built web app to serve at / (INTERLINEA_WEB)")
	fs.StringVar(&o.tlsCert, "tls-cert", env("INTERLINEA_TLS_CERT", ""), "TLS certificate file, to serve HTTPS itself (INTERLINEA_TLS_CERT)")
	fs.StringVar(&o.tlsKey, "tls-key", env("INTERLINEA_TLS_KEY", ""), "TLS key file (INTERLINEA_TLS_KEY)")
	fs.IntVar(&o.tokenDays, "token-days", days, "days a sign-in lasts (INTERLINEA_TOKEN_DAYS)")
	fs.BoolVar(&o.logJSON, "log-json", boolEnv("INTERLINEA_LOG_JSON"), "log as JSON lines (INTERLINEA_LOG_JSON)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: interlinea-server [options]")
		fmt.Fprintln(stderr, "       interlinea-server healthcheck [addr]")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
		fmt.Fprintln(stderr, `
Only in the environment:
  INTERLINEA_OIDC_CLIENT_SECRET   the provider's client secret
  INTERLINEA_ADMIN_USER           with INTERLINEA_ADMIN_PASSWORD, the first
  INTERLINEA_ADMIN_PASSWORD       administrator, made while there are no accounts

Without them, a server with no accounts logs a setup token; the web app's
setup page makes the first administrator with it.`)
	}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	o.oidcSecret = os.Getenv("INTERLINEA_OIDC_CLIENT_SECRET")
	o.adminUser = os.Getenv("INTERLINEA_ADMIN_USER")
	o.adminPassword = os.Getenv("INTERLINEA_ADMIN_PASSWORD")
	if o.tokenDays < 1 || o.tokenDays > 3650 {
		return o, fmt.Errorf("a sign-in lasts 1 to 3650 days, not %d", o.tokenDays)
	}
	if (o.tlsCert == "") != (o.tlsKey == "") {
		return o, errors.New("TLS needs both the certificate and the key")
	}
	if (o.oidcIssuer == "") != (o.oidcClient == "") {
		return o, errors.New("sign-in through a provider needs both the issuer and the client ID")
	}
	if (o.adminUser == "") != (o.adminPassword == "") {
		return o, errors.New("INTERLINEA_ADMIN_USER and INTERLINEA_ADMIN_PASSWORD go together")
	}
	return o, nil
}

// withSchema points a PostgreSQL connection string at a schema.
func withSchema(dsn, schema string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("the PostgreSQL connection string is not a valid URL")
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	return dsn + " search_path=" + schema, nil
}

// storeSchemas name the PostgreSQL schema of each store.
var storeSchemas = []string{"interlinea_accounts", "interlinea_projects", "interlinea_memories", "interlinea_termbases"}

func openStores(ctx context.Context, o options, log *slog.Logger) (server.Stores, func(), error) {
	var st server.Stores
	var closers []func() error
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	if o.postgres == "" {
		if err := os.MkdirAll(o.data, 0o700); err != nil {
			return st, nil, err
		}
		var err error
		if st.Auth, err = authdb.Open(filepath.Join(o.data, "accounts.db")); err != nil {
			return st, nil, err
		}
		closers = append(closers, st.Auth.Close)
		if st.Projects, err = projectdb.Open(filepath.Join(o.data, "projects.db")); err != nil {
			closeAll()
			return st, nil, err
		}
		closers = append(closers, st.Projects.Close)
		if st.Memories, err = tmdb.Open(filepath.Join(o.data, "memories.db")); err != nil {
			closeAll()
			return st, nil, err
		}
		closers = append(closers, st.Memories.Close)
		if st.Termbases, err = termdb.Open(filepath.Join(o.data, "termbases.db")); err != nil {
			closeAll()
			return st, nil, err
		}
		closers = append(closers, st.Termbases.Close)
		log.Info("stores opened", "folder", o.data)
		return st, closeAll, nil
	}
	admin, err := pgdriver.Open(ctx, o.postgres)
	if err != nil {
		return st, nil, err
	}
	for _, s := range storeSchemas {
		if _, err := admin.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS `+s); err != nil {
			admin.Close()
			return st, nil, fmt.Errorf("make schema %s: %w", s, err)
		}
	}
	admin.Close()
	dbs := make([]*sql.DB, len(storeSchemas))
	for i, s := range storeSchemas {
		dsn, err := withSchema(o.postgres, s)
		if err != nil {
			closeAll()
			return st, nil, err
		}
		if dbs[i], err = pgdriver.Open(ctx, dsn); err != nil {
			closeAll()
			return st, nil, err
		}
		closers = append(closers, dbs[i].Close)
	}
	if st.Auth, err = authdb.OpenPostgres(ctx, dbs[0]); err != nil {
		closeAll()
		return st, nil, err
	}
	if st.Projects, err = projectdb.OpenPostgres(ctx, dbs[1]); err != nil {
		closeAll()
		return st, nil, err
	}
	if st.Memories, err = tmdb.OpenPostgres(ctx, dbs[2]); err != nil {
		closeAll()
		return st, nil, err
	}
	if st.Termbases, err = termdb.OpenPostgres(ctx, dbs[3]); err != nil {
		closeAll()
		return st, nil, err
	}
	log.Info("stores opened on PostgreSQL", "schemas", strings.Join(storeSchemas, ", "))
	return st, closeAll, nil
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	o, err := parse(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	var h slog.Handler = slog.NewTextHandler(stderr, nil)
	if o.logJSON {
		h = slog.NewJSONHandler(stderr, nil)
	}
	log := slog.New(h)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, closeStores, err := openStores(ctx, o, log)
	if err != nil {
		return err
	}
	defer closeStores()

	cfg := server.Config{
		Stores:     st,
		PublicURL:  o.publicURL,
		Origins:    splitList(o.origins),
		TokenTTL:   time.Duration(o.tokenDays) * 24 * time.Hour,
		TrustProxy: o.trustProxy,
		Version:    version,
		Log:        log,
	}
	if o.web != "" {
		if _, err := os.Stat(filepath.Join(o.web, "index.html")); err != nil {
			return fmt.Errorf("the web app folder %s has no index.html", o.web)
		}
		cfg.Web = os.DirFS(o.web)
	}
	if o.oidcIssuer != "" {
		if o.publicURL == "" {
			return errors.New("sign-in through a provider needs -public-url, where the provider sends people back to")
		}
		p, err := oidc.Discover(ctx, oidc.Config{
			Issuer:       o.oidcIssuer,
			ClientID:     o.oidcClient,
			ClientSecret: o.oidcSecret,
			RedirectURL:  strings.TrimRight(o.publicURL, "/") + "/api/auth/oidc/callback",
			Scopes:       splitList(o.oidcScopes),
		})
		if err != nil {
			return fmt.Errorf("the identity provider: %w", err)
		}
		cfg.OIDC = &server.OIDC{Name: o.oidcName, Provider: p, CreateUsers: o.oidcCreate}
		log.Info("sign-in through a provider", "issuer", o.oidcIssuer, "name", o.oidcName)
	}

	n, err := st.Auth.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		if o.adminUser != "" {
			u, err := st.Auth.CreateUser(ctx, authdb.NewUser{Username: o.adminUser, Password: o.adminPassword, Admin: true}, "setup")
			if err != nil {
				return fmt.Errorf("the first administrator: %w", err)
			}
			log.Info("first administrator made", "username", u.Username)
		} else {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			cfg.SetupToken = hex.EncodeToString(b)
			log.Warn("the server has no accounts yet: make the first administrator with this setup token", "setupToken", cfg.SetupToken)
		}
	}

	srv, err := server.New(cfg)
	if err != nil {
		return err
	}
	defer srv.Close()

	hs := &http.Server{
		Addr:              o.addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(h, slog.LevelWarn),
	}
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := st.Auth.PurgeExpired(ctx); err != nil {
					log.Error("removing expired sign-ins failed", "err", err)
				} else if n > 0 {
					log.Info("expired sign-ins removed", "count", n)
				}
			}
		}
	}()
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", o.addr, "version", version, "tls", o.tlsCert != "")
		if o.tlsCert != "" {
			errc <- hs.ListenAndServeTLS(o.tlsCert, o.tlsKey)
		} else {
			errc <- hs.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	srv.Close()
	if err := hs.Shutdown(sctx); err != nil {
		return err
	}
	return nil
}
