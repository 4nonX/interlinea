package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithSchema(t *testing.T) {
	got, err := withSchema("postgres://u:p@db:5432/interlinea?sslmode=disable", "interlinea_projects")
	if err != nil || !strings.Contains(got, "search_path=interlinea_projects%2Cpublic") || !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("%s %v", got, err)
	}
	got, _ = withSchema("host=db dbname=interlinea", "interlinea_memories")
	if got != "host=db dbname=interlinea search_path=interlinea_memories,public" {
		t.Fatal(got)
	}
	if _, err := withSchema("postgres://%zz", "x"); err == nil {
		t.Fatal("bad URL accepted")
	}
}

func TestParse(t *testing.T) {
	var out bytes.Buffer
	t.Setenv("INTERLINEA_ADDR", ":9000")
	t.Setenv("INTERLINEA_TOKEN_DAYS", "7")
	o, err := parse([]string{"-data", "/srv/il", "-origins", "http://localhost:5173"}, &out)
	if err != nil || o.addr != ":9000" || o.data != "/srv/il" || o.tokenDays != 7 || o.origins != "http://localhost:5173" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, args := range [][]string{
		{"-tls-cert", "c.pem"},
		{"-oidc-issuer", "https://login.example"},
		{"-token-days", "0"},
		{"extra"},
	} {
		if _, err := parse(args, &out); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	t.Setenv("INTERLINEA_ADMIN_USER", "anna")
	if _, err := parse(nil, &out); err == nil {
		t.Error("an administrator without a password accepted")
	}
	if got := splitList(" a, ,b ,"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("split %v", got)
	}
}

// TestRun starts the server on SQLite files, makes the first
// administrator from the environment and checks that it answers.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	t.Setenv("INTERLINEA_ADMIN_USER", "anna")
	t.Setenv("INTERLINEA_ADMIN_PASSWORD", "a long enough password")
	web := filepath.Join(dir, "web")
	os.MkdirAll(web, 0o755)
	os.WriteFile(filepath.Join(web, "index.html"), []byte("<title>Interlinea</title>"), 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	var log bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-addr", addr, "-data", filepath.Join(dir, "data"), "-web", web}, &log)
	}()
	var res *http.Response
	for i := 0; i < 600; i++ { // up to 30 s: hashing is slow under the race detector
		if res, err = http.Post("http://"+addr+"/api/auth/password", "application/json",
			strings.NewReader(`{"username":"anna","password":"a long enough password"}`)); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err, log.String())
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("sign-in: %d\n%s", res.StatusCode, log.String())
	}
	if code := healthcheck([]string{addr}); code != 0 {
		t.Fatalf("healthcheck %d", code)
	}
	for _, f := range []string{"accounts.db", "projects.db", "memories.db", "termbases.db"} {
		if _, err := os.Stat(filepath.Join(dir, "data", f)); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no shutdown")
	}
	if !strings.Contains(log.String(), "first administrator made") {
		t.Fatalf("log:\n%s", log.String())
	}
}
