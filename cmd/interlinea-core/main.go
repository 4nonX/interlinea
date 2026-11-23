// Command interlinea-core is the desktop app's core: the team server's
// code, run for one person on their own computer. The app starts it, and
// it keeps projects, memories, termbases and dictionaries in SQLite files
// in the app's data folder.
//
// It listens on the loopback interface only, on a free port, and answers
// only requests with that address as their Host. It makes one account for
// the person (an administrator of their own data) and, at every start, a
// new sign-in for the app, which it prints as one JSON line on standard
// output:
//
//	{"url":"http://127.0.0.1:52817","token":"il_...","version":"..."}
//
// It stops when its standard input closes, so it ends with the app even if
// the app is killed.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/server"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm/tmdb"
)

var version = "dev"

// localUser is the account of the person at the computer.
const localUser = "me"

// handshake is what the app reads from standard output.
type handshake struct {
	URL     string `json:"url"`
	Token   string `json:"token"`
	Version string `json:"version"`
	// User is the person's account.
	User authdb.User `json:"user"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "interlinea-core:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("interlinea-core", flag.ContinueOnError)
	fs.SetOutput(stderr)
	data := fs.String("data", "", "folder of the person's data (the app's data folder)")
	name := fs.String("name", "", "the person's name, for their account")
	port := fs.Int("port", 0, "port to listen on (0 for a free one)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *data == "" {
		return errors.New("say where the data is kept (-data)")
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The app holds standard input open while it runs.
	go func() {
		io.Copy(io.Discard, bufio.NewReader(stdin))
		stop()
	}()

	st, closeStores, err := openStores(*data)
	if err != nil {
		return err
	}
	defer closeStores()

	me, err := localAccount(ctx, st.Auth, *name)
	if err != nil {
		return err
	}
	// Each start is one sign-in, for the app that started it.
	if err := st.Auth.RevokeTokens(ctx, me.ID, "interlinea-core"); err != nil {
		return err
	}
	token, _, err := st.Auth.IssueToken(ctx, me.ID, "Interlinea on this computer", 3650*24*time.Hour)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	addr := ln.Addr().String()
	srv, err := server.New(server.Config{
		Stores:  st,
		Version: version,
		Log:     log,
		Hosts:   []string{addr, "localhost:" + portOf(addr)},
	})
	if err != nil {
		ln.Close()
		return err
	}
	defer srv.Close()
	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute, ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn)}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()

	if err := json.NewEncoder(stdout).Encode(handshake{URL: "http://" + addr, Token: token, Version: version, User: me}); err != nil {
		return err
	}
	log.Info("listening", "addr", addr, "data", *data, "version", version)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Close()
	return hs.Shutdown(sctx)
}

func portOf(addr string) string {
	_, p, _ := net.SplitHostPort(addr)
	return p
}

// localAccount is the person's account, made at the first start.
func localAccount(ctx context.Context, auth *authdb.DB, name string) (authdb.User, error) {
	u, err := auth.UserByName(ctx, localUser)
	if errors.Is(err, authdb.ErrNotFound) {
		return auth.CreateUser(ctx, authdb.NewUser{Username: localUser, Name: name, Admin: true}, "interlinea-core")
	}
	if err != nil {
		return u, err
	}
	if name != "" && name != u.Name {
		n := name
		return auth.UpdateUser(ctx, u.ID, authdb.UserChange{Name: &n}, u.Version, "interlinea-core")
	}
	return u, nil
}

func openStores(dir string) (server.Stores, func(), error) {
	var st server.Stores
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return st, nil, err
	}
	var closers []func() error
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	var err error
	if st.Auth, err = authdb.Open(filepath.Join(dir, "accounts.db")); err != nil {
		return st, nil, err
	}
	closers = append(closers, st.Auth.Close)
	if st.Projects, err = projectdb.Open(filepath.Join(dir, "projects.db")); err != nil {
		closeAll()
		return st, nil, err
	}
	closers = append(closers, st.Projects.Close)
	if st.Memories, err = tmdb.Open(filepath.Join(dir, "memories.db")); err != nil {
		closeAll()
		return st, nil, err
	}
	closers = append(closers, st.Memories.Close)
	if st.Termbases, err = termdb.Open(filepath.Join(dir, "termbases.db")); err != nil {
		closeAll()
		return st, nil, err
	}
	closers = append(closers, st.Termbases.Close)
	return st, closeAll, nil
}
