package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
)

type sse struct {
	typ, id, data string
}

// feedReader reads a person's live feed.
type feedReader struct {
	t      *testing.T
	events chan sse
	cancel context.CancelFunc
}

func (e *env) listen(token, lastID string) *feedReader {
	e.t.Helper()
	cctx, cancel := context.WithCancel(ctx)
	req, _ := http.NewRequestWithContext(cctx, "GET", e.hs.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		e.t.Fatal(err)
	}
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		cancel()
		e.t.Fatalf("events: %d %v", res.StatusCode, res.Header)
	}
	s := &feedReader{t: e.t, events: make(chan sse, 100), cancel: cancel}
	go func() {
		defer res.Body.Close()
		defer close(s.events)
		sc := bufio.NewScanner(res.Body)
		var cur sse
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if cur.typ != "" {
					s.events <- cur
				}
				cur = sse{}
			case strings.HasPrefix(line, "event: "):
				cur.typ = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				cur.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				cur.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	e.t.Cleanup(cancel)
	if h := s.next(); h.typ != "hello" {
		e.t.Fatalf("first event %+v", h)
	}
	return s
}

func (s *feedReader) next() sse {
	s.t.Helper()
	select {
	case ev, ok := <-s.events:
		if !ok {
			return sse{typ: "closed"}
		}
		return ev
	case <-time.After(5 * time.Second):
		s.t.Fatal("no event")
		return sse{}
	}
}

// until skips events up to one of a type.
func (s *feedReader) until(typ string) sse {
	s.t.Helper()
	for {
		ev := s.next()
		if ev.typ == typ || ev.typ == "closed" {
			return ev
		}
	}
}

func (s *feedReader) quiet(d time.Duration) {
	s.t.Helper()
	select {
	case ev, ok := <-s.events:
		if ok {
			s.t.Fatalf("unexpected event %+v", ev)
		}
	case <-time.After(d):
	}
}

func projectChange(t *testing.T, ev sse) projectdb.Change {
	t.Helper()
	if ev.typ != "project" {
		t.Fatalf("event %+v", ev)
	}
	var c projectdb.Change
	if err := json.Unmarshal([]byte(ev.data), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEvents(t *testing.T) {
	te := newTeam(t)
	pmFeed := te.listen(te.pm, "")
	benFeed := te.listen(te.ben, "")

	p := te.createLease()
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	if c := projectChange(t, pmFeed.until("project")); c.Kind != projectdb.ChangeProjectCreated || c.ProjectID != p.ID {
		t.Fatalf("project event %+v", c)
	}
	// The memories made with it come through as memory events.
	if ev := pmFeed.until("memory"); !strings.Contains(ev.data, "Mietvertrag Hofmann") {
		t.Fatalf("memory event %+v", ev)
	}
	benFeed.quiet(200 * time.Millisecond)

	// Work for Ben opens the project to him: he gets the assignment.
	te.call("POST", base+"/assignments", te.pm, projectdb.Assignment{TargetLang: "en-GB", UserID: te.benID, Task: projectdb.TaskTranslate}, 201, nil)
	c := projectChange(t, benFeed.until("project"))
	if c.Kind != projectdb.ChangeAssignment {
		t.Fatalf("Ben's event %+v", c)
	}
	last := ""
	te.call("PUT", segPath(p.ID, "Mietvertrag.docx", "en-GB", "s1", 1), te.ben, segmentSave{Target: content.Text("Lease."), Status: project.StatusDraft}, 200, nil)
	ev := benFeed.until("project")
	if projectChange(t, ev).Kind != projectdb.ChangeSegment {
		t.Fatalf("segment event %+v", ev)
	}
	last = ev.id
	if _, ok := parseCursor(last); !ok {
		t.Fatalf("event id %q", last)
	}
	// Presence comes through at once.
	te.call("PUT", base+"/presence", te.pm, map[string]string{"file": "Mietvertrag.docx", "lang": "en-GB", "segment": "s2"}, 200, nil)
	if ev := benFeed.until("presence"); !strings.Contains(ev.data, fmt.Sprint(p.ID)) {
		t.Fatalf("presence %+v", ev)
	}

	// Ben goes offline; what he missed comes when he reconnects.
	benFeed.cancel()
	te.call("PUT", segPath(p.ID, "Mietvertrag.docx", "en-GB", "s2", 1), te.pm, segmentSave{Target: content.Text("Rent."), Status: project.StatusDraft}, 200, nil)
	te.call("PUT", segPath(p.ID, "Mietvertrag.docx", "fr-FR", "s2", 1), te.pm, segmentSave{Target: content.Text("Loyer."), Status: project.StatusDraft}, 200, nil)
	time.Sleep(100 * time.Millisecond)
	back := te.listen(te.ben, last)
	got := projectChange(t, back.until("project"))
	if got.Segment != "s2" || got.TargetLang != "en-GB" {
		t.Fatalf("caught up %+v", got)
	}
	got = projectChange(t, back.until("project"))
	if got.TargetLang != "fr-FR" {
		t.Fatalf("second missed change %+v", got)
	}
	// A new change after the catch-up comes once.
	te.call("PUT", segPath(p.ID, "Mietvertrag.docx", "en-GB", "s3", 1), te.pm, segmentSave{Target: content.Text("Deposit."), Status: project.StatusDraft}, 200, nil)
	if got := projectChange(t, back.until("project")); got.Segment != "s3" {
		t.Fatalf("live after catch-up %+v", got)
	}
	back.quiet(200 * time.Millisecond)

	// Disabling Ben ends his stream.
	yes := true
	u := mustUser(t, te.env, "ben")
	te.call("PATCH", fmt.Sprintf("/api/users/%d?version=%d", u.ID, u.Version), te.admin, userPatch{Disabled: &yes}, 200, nil)
	if ev := back.until("signedOut"); ev.typ != "signedOut" {
		t.Fatalf("after disabling %+v", ev)
	}
	if ev := back.next(); ev.typ != "closed" {
		t.Fatalf("stream still open: %+v", ev)
	}
}

func TestEventsAccountAndReset(t *testing.T) {
	e := newEnv(t)
	_, admin := e.user("anna", true)
	ben, benTok := e.user("ben", false)
	carl, _ := e.user("carl", false)
	feed := e.listen(benTok, "")
	adminFeed := e.listen(admin, "")
	// Ben sees changes to his own grants, not Carl's; administrators see
	// both.
	if _, err := e.st.Memories.Create(ctx, tmInfo("M")); err != nil {
		t.Fatal(err)
	}
	e.call("PUT", "/api/grants", admin, authdb.Grant{UserID: carl.ID, Kind: authdb.KindMemory, Resource: "M", Role: authdb.RoleRead}, 204, nil)
	e.call("PUT", "/api/grants", admin, authdb.Grant{UserID: ben.ID, Kind: authdb.KindMemory, Resource: "M", Role: authdb.RoleRead}, 204, nil)
	ev := feed.until("account")
	var c authdb.Change
	json.Unmarshal([]byte(ev.data), &c)
	if c.UserID != ben.ID {
		t.Fatalf("Ben's account event %+v", c)
	}
	n := 0
	for n < 2 {
		ev := adminFeed.until("account")
		json.Unmarshal([]byte(ev.data), &c)
		if c.Kind == authdb.ChangeGrantSet {
			n++
		}
	}
	// The memory is his now: its changes reach him.
	m, _ := e.st.Memories.Memory(ctx, "M")
	if _, err := m.Add(ctx, tmUnit("a", "b"), 0); err != nil {
		t.Fatal(err)
	}
	if ev := feed.until("memory"); !strings.Contains(ev.data, `"memory":"M"`) {
		t.Fatalf("memory event %+v", ev)
	}
	// A client far behind is told to read everything again.
	if r := e.do("GET", "/api/events?since=nonsense", benTok, nil); r.status != 422 {
		t.Fatalf("bad cursor: %d", r.status)
	}
	for i := 0; i < maxCatchUp+5; i++ {
		if _, err := m.Add(ctx, tmUnit(fmt.Sprintf("s%d", i), "t"), 0); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	late := e.listen(benTok, "0.0.0.0")
	if ev := late.next(); ev.typ != "reset" {
		t.Fatalf("far behind: %+v", ev)
	}
}

func TestCursor(t *testing.T) {
	c := cursor{1, 22, 333, 0}
	got, ok := parseCursor(c.String())
	if !ok || got != c {
		t.Fatalf("%v %v", got, ok)
	}
	for _, bad := range []string{"", "1.2.3", "1.2.3.x", "1.2.3.-4", "1.2.3.4.5"} {
		if _, ok := parseCursor(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}
