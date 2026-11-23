package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm/tmdb"
)

// The live feed (GET /api/events) is a stream of server-sent events: one
// per change in the stores the person may see, so the apps read again
// what changed. Each event's id is a cursor over the four change logs
// ("project.memory.termbase.account" sequence numbers); a client that
// reconnects with it (Last-Event-ID, or ?since=) gets what it missed.
//
// Event types and their data:
//
//	project   a projectdb.Change (settings, files, segments, assignments)
//	memory    a tmdb.Change
//	termbase  a termdb.Change
//	account   an authdb.Change of the person's own account or grants (all
//	          of them for administrators)
//	presence  {"projectId": n}: who has which segment open changed
//	reset     the client was too far behind; read everything again
//
// Comments (": keep-alive") are sent every 25 seconds.

type stream int

const (
	streamProject stream = iota
	streamMemory
	streamTermbase
	streamAccount
	streams
)

var streamNames = [streams]string{"project", "memory", "termbase", "account"}

// cursor is a position in the four logs.
type cursor [streams]int64

func (c cursor) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", c[0], c[1], c[2], c[3])
}

func parseCursor(s string) (cursor, bool) {
	var c cursor
	parts := strings.Split(s, ".")
	if len(parts) != int(streams) {
		return c, false
	}
	for i, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return c, false
		}
		c[i] = n
	}
	return c, true
}

// change is one entry of one of the logs.
type change struct {
	stream stream
	seq    int64
	data   any
	// What it is about, for deciding who sees it.
	projectID int64
	memory    string
	termbase  string
	userID    int64
}

// presenceEvent says a project's presence changed.
type presenceEvent struct {
	ProjectID int64 `json:"projectId"`
}

type subscriber struct {
	ch chan any // change or presenceEvent
	// lost is set when the subscriber fell too far behind.
	lost bool
}

// feed polls the stores' change logs and passes new entries to the
// subscribers.
type feed struct {
	s *Server

	mu      sync.Mutex
	subs    map[*subscriber]bool
	pos     cursor
	running bool
	done    chan struct{}
	wg      sync.WaitGroup
}

func newFeed(s *Server) *feed {
	return &feed{s: s, subs: map[*subscriber]bool{}, done: make(chan struct{})}
}

// subBuffer is how many events a subscriber may be behind before it is
// told to read everything again.
const subBuffer = 1024

// last reads the newest position of every log.
func (f *feed) last(ctx context.Context) (cursor, error) {
	var c cursor
	var err error
	st := f.s.cfg.Stores
	if c[streamProject], err = st.Projects.LastChange(ctx); err != nil {
		return c, err
	}
	if c[streamMemory], err = st.Memories.LastChange(ctx); err != nil {
		return c, err
	}
	if c[streamTermbase], err = st.Termbases.LastChange(ctx); err != nil {
		return c, err
	}
	if c[streamAccount], err = st.Auth.LastChange(ctx); err != nil {
		return c, err
	}
	return c, nil
}

// subscribe adds a subscriber and returns the feed's position: events
// after it come through the channel.
func (f *feed) subscribe(ctx context.Context) (*subscriber, cursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.done:
		return nil, cursor{}, http.ErrServerClosed
	default:
	}
	if !f.running {
		pos, err := f.last(ctx)
		if err != nil {
			return nil, cursor{}, err
		}
		f.pos = pos
		f.running = true
		f.wg.Add(1)
		go f.run()
	}
	sub := &subscriber{ch: make(chan any, subBuffer)}
	f.subs[sub] = true
	return sub, f.pos, nil
}

func (f *feed) unsubscribe(sub *subscriber) {
	f.mu.Lock()
	delete(f.subs, sub)
	f.mu.Unlock()
}

// send passes an event to every subscriber. Callers hold mu.
func (f *feed) send(ev any) {
	for sub := range f.subs {
		if sub.lost {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			sub.lost = true
			close(sub.ch)
		}
	}
}

// presence tells the subscribers that a project's presence changed.
func (f *feed) presence(projectID int64) {
	f.mu.Lock()
	f.send(presenceEvent{ProjectID: projectID})
	f.mu.Unlock()
}

func (f *feed) stop() {
	f.mu.Lock()
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	f.mu.Unlock()
	f.wg.Wait()
}

func (f *feed) run() {
	defer f.wg.Done()
	t := time.NewTicker(f.s.cfg.EventPoll)
	defer t.Stop()
	for {
		select {
		case <-f.done:
			return
		case <-t.C:
		}
		f.mu.Lock()
		from := f.pos
		f.mu.Unlock()
		cs, to, err := f.s.changesSince(context.Background(), from, 1000)
		if err != nil {
			f.s.cfg.Log.Error("reading the change logs failed", "err", err)
			continue
		}
		f.mu.Lock()
		for _, c := range cs {
			f.send(c)
		}
		f.pos = to
		f.mu.Unlock()
	}
}

// changesSince reads up to limit changes of each log after from, and the
// position after them.
func (s *Server) changesSince(ctx context.Context, from cursor, limit int) ([]change, cursor, error) {
	to := from
	var out []change
	st := s.cfg.Stores
	pcs, _, err := st.Projects.Changes(ctx, projectdb.ChangeQuery{Since: from[streamProject], Limit: limit})
	if err != nil {
		return nil, from, err
	}
	for _, c := range pcs {
		out = append(out, change{stream: streamProject, seq: c.Seq, data: c, projectID: c.ProjectID})
		to[streamProject] = c.Seq
	}
	mcs, _, err := st.Memories.Changes(ctx, tmdb.ChangeQuery{Since: from[streamMemory], Limit: limit})
	if err != nil {
		return nil, from, err
	}
	for _, c := range mcs {
		out = append(out, change{stream: streamMemory, seq: c.Seq, data: c, memory: c.Memory})
		to[streamMemory] = c.Seq
	}
	tcs, _, err := st.Termbases.Changes(ctx, termdb.ChangeQuery{Since: from[streamTermbase], Limit: limit})
	if err != nil {
		return nil, from, err
	}
	for _, c := range tcs {
		out = append(out, change{stream: streamTermbase, seq: c.Seq, data: c, termbase: c.Termbase})
		to[streamTermbase] = c.Seq
	}
	acs, _, err := st.Auth.Changes(ctx, from[streamAccount], limit)
	if err != nil {
		return nil, from, err
	}
	for _, c := range acs {
		out = append(out, change{stream: streamAccount, seq: c.Seq, data: c, userID: c.UserID})
		to[streamAccount] = c.Seq
	}
	return out, to, nil
}

// visibility decides which events a subscriber sees, remembering the
// answers for a few seconds.
type visibility struct {
	s  *Server
	me session

	projects  map[int64]cached
	memories  map[string]cached
	termbases map[string]cached
}

type cached struct {
	ok bool
	at time.Time
}

const visibilityTTL = 5 * time.Second

func (v *visibility) forget() {
	v.projects, v.memories, v.termbases = map[int64]cached{}, map[string]cached{}, map[string]cached{}
}

func (v *visibility) project(ctx context.Context, id int64) (bool, error) {
	now := v.s.cfg.Now()
	if c, ok := v.projects[id]; ok && now.Sub(c.at) < visibilityTTL {
		return c.ok, nil
	}
	a, err := v.s.accessTo(ctx, v.me, id)
	if err != nil {
		return false, err
	}
	v.projects[id] = cached{a.read(), now}
	return a.read(), nil
}

func (v *visibility) memory(ctx context.Context, name string) (bool, error) {
	now := v.s.cfg.Now()
	key := strings.ToLower(name)
	if c, ok := v.memories[key]; ok && now.Sub(c.at) < visibilityTTL {
		return c.ok, nil
	}
	r, err := v.s.cfg.Auth.RoleOn(ctx, v.me.User, authdb.KindMemory, name)
	if err != nil {
		return false, err
	}
	ok := r.Includes(authdb.RoleRead)
	if !ok {
		// Memories a project uses are read through the project.
		ok, err = v.viaProject(ctx, func(p projectdb.Summary) bool {
			for _, ms := range p.Settings.Memories {
				for _, m := range ms {
					if strings.EqualFold(m.Memory, name) {
						return true
					}
				}
			}
			return false
		})
		if err != nil {
			return false, err
		}
	}
	v.memories[key] = cached{ok, now}
	return ok, nil
}

func (v *visibility) termbase(ctx context.Context, name string) (bool, error) {
	now := v.s.cfg.Now()
	key := strings.ToLower(name)
	if c, ok := v.termbases[key]; ok && now.Sub(c.at) < visibilityTTL {
		return c.ok, nil
	}
	r, err := v.s.termbaseRole(ctx, v.me, name)
	if err != nil {
		return false, err
	}
	ok := r.Includes(authdb.RoleRead)
	if !ok {
		ok, err = v.viaProject(ctx, func(p projectdb.Summary) bool { return usesTermbase(name)(p.Settings) })
		if err != nil {
			return false, err
		}
	}
	v.termbases[key] = cached{ok, now}
	return ok, nil
}

// viaProject reports whether the person sees a project that uses
// something.
func (v *visibility) viaProject(ctx context.Context, uses func(projectdb.Summary) bool) (bool, error) {
	ps, err := v.s.cfg.Projects.List(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		if !uses(p) {
			continue
		}
		ok, err := v.project(ctx, p.ID)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func (v *visibility) sees(ctx context.Context, ev any) (bool, error) {
	switch e := ev.(type) {
	case presenceEvent:
		return v.project(ctx, e.ProjectID)
	case change:
		switch e.stream {
		case streamProject:
			if e.projectID == 0 {
				return true, nil // templates
			}
			return v.project(ctx, e.projectID)
		case streamMemory:
			return v.memory(ctx, e.memory)
		case streamTermbase:
			return v.termbase(ctx, e.termbase)
		case streamAccount:
			return v.me.User.Admin || e.userID == v.me.User.ID, nil
		}
	}
	return false, nil
}

// maxCatchUp is how many missed changes per log a reconnecting client
// gets; further behind, it is told to read everything again.
const maxCatchUp = 5000

func (s *Server) events(w http.ResponseWriter, r *http.Request, me session) {
	rc := http.NewResponseController(w)
	ctx := r.Context()
	var from cursor
	resume := false
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		from, resume = parseCursor(id)
	} else if q := r.URL.Query().Get("since"); q != "" {
		from, resume = parseCursor(q)
		if !resume {
			invalid(w, "since", "since is the id of the last event seen.")
			return
		}
	}
	sub, pos, err := s.feed.subscribe(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer s.feed.unsubscribe(sub)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// Streams last; the server's write timeout doesn't apply.
	rc.SetWriteDeadline(time.Time{})

	vis := &visibility{s: s, me: me}
	vis.forget()
	sent := pos
	write := func(typ, id string, data any) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if id != "" {
			if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, b); err != nil {
			return err
		}
		return rc.Flush()
	}
	// deliver writes an event the person may see; it reports false when
	// the stream should end (an error, or the sign-in ended).
	deliver := func(ev any) bool {
		if c, ok := ev.(change); ok {
			if c.seq <= sent[c.stream] {
				return true // seen in the catch-up
			}
			sent[c.stream] = c.seq
			if c.stream == streamAccount {
				// Grants may have changed: decide again. A revoked
				// sign-in or disabled account ends the stream.
				vis.forget()
				u, t, err := s.cfg.Auth.Authenticate(ctx, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
				if err != nil || t.ID != me.Token.ID {
					write("signedOut", "", map[string]string{"message": "The sign-in has ended."})
					return false
				}
				vis.me.User = u
			}
		}
		ok, err := vis.sees(ctx, ev)
		if err != nil {
			s.cfg.Log.Error("deciding who sees a change failed", "err", err)
			return false
		}
		if !ok {
			return true
		}
		switch e := ev.(type) {
		case presenceEvent:
			return write("presence", "", e) == nil
		case change:
			if c, ok := e.data.(projectdb.Change); ok && strings.HasPrefix(c.Kind, "assignment") {
				vis.forget() // new work may open projects
			}
			return write(streamNames[e.stream], sent.String(), e.data) == nil
		}
		return true
	}

	if err := write("hello", pos.String(), map[string]string{"cursor": pos.String()}); err != nil {
		return
	}
	if resume {
		// What happened between the client's cursor and now.
		sent = from
		for {
			cs, to, err := s.changesSince(ctx, from, 1000)
			if err != nil {
				s.cfg.Log.Error("catching up a client failed", "err", err)
				return
			}
			behind := false
			for i := range to {
				if to[i] < pos[i] && pos[i]-from[i] > maxCatchUp {
					behind = true
				}
			}
			if behind {
				write("reset", pos.String(), map[string]string{"message": "Too much changed. Read everything again."})
				sent = pos
				break
			}
			for _, c := range cs {
				if c.seq > pos[c.stream] {
					continue // comes through the feed
				}
				if !deliver(c) {
					return
				}
			}
			done := true
			for i := range to {
				if to[i] < pos[i] {
					done = false
				}
			}
			if done || to == from {
				break
			}
			from = to
		}
		for i := range sent {
			if sent[i] < pos[i] {
				sent[i] = pos[i]
			}
		}
	}

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.feed.done:
			return
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case ev, ok := <-sub.ch:
			if !ok {
				write("reset", "", map[string]string{"message": "Too much changed. Read everything again."})
				return
			}
			if !deliver(ev) {
				return
			}
		}
	}
}
