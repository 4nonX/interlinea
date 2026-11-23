package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type started struct {
	hs   handshake
	in   *io.PipeWriter
	done chan error
}

func start(t *testing.T, dir, name string) started {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), []string{"-data", dir, "-name", name}, inR, outW, io.Discard)
		outW.Close()
	}()
	line, err := bufio.NewReader(outR).ReadString('\n')
	if err != nil {
		t.Fatalf("no handshake: %v (%v)", err, <-done)
	}
	var hs handshake
	if err := json.Unmarshal([]byte(line), &hs); err != nil {
		t.Fatalf("handshake %q: %v", line, err)
	}
	go io.Copy(io.Discard, outR)
	return started{hs: hs, in: inW, done: done}
}

func (s started) stop(t *testing.T) {
	t.Helper()
	s.in.Close()
	select {
	case err := <-s.done:
		if err != nil {
			t.Fatalf("stopped with %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the core did not stop when its input closed")
	}
}

func get(t *testing.T, url, token, host string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if host != "" {
		req.Host = host
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestCore(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir, "Dan Dreßen")
	if !strings.HasPrefix(s.hs.URL, "http://127.0.0.1:") || !strings.HasPrefix(s.hs.Token, "il_") || s.hs.User.Username != localUser || !s.hs.User.Admin || s.hs.User.Name != "Dan Dreßen" {
		t.Fatalf("handshake %+v", s.hs)
	}
	res := get(t, s.hs.URL+"/api/me", s.hs.Token, "")
	var me struct {
		User struct{ Username string }
	}
	json.NewDecoder(res.Body).Decode(&me)
	res.Body.Close()
	if res.StatusCode != 200 || me.User.Username != localUser {
		t.Fatalf("me: %d %+v", res.StatusCode, me)
	}
	// A web page that names the address otherwise (DNS rebinding) is
	// refused, and so is a request without the token.
	if res := get(t, s.hs.URL+"/api/me", s.hs.Token, "evil.example:80"); res.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("other host: %d", res.StatusCode)
	}
	if res := get(t, s.hs.URL+"/api/projects", "", ""); res.StatusCode != 401 {
		t.Fatalf("no token: %d", res.StatusCode)
	}
	old := s.hs.Token
	s.stop(t)

	// The next start keeps the account, renames it, and the old sign-in ends.
	s = start(t, dir, "Dan")
	defer s.stop(t)
	if s.hs.User.Name != "Dan" || s.hs.User.ID == 0 {
		t.Fatalf("second start %+v", s.hs.User)
	}
	if res := get(t, s.hs.URL+"/api/me", old, ""); res.StatusCode != 401 {
		t.Fatalf("the old sign-in still works: %d", res.StatusCode)
	}
	if res := get(t, s.hs.URL+"/api/me", s.hs.Token, ""); res.StatusCode != 200 {
		t.Fatalf("the new sign-in: %d", res.StatusCode)
	}
}

func TestCoreNeedsData(t *testing.T) {
	if err := run(context.Background(), nil, strings.NewReader(""), io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "-data") {
		t.Fatalf("no data folder: %v", err)
	}
}
