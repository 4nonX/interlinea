package server

import (
	"sync"
	"time"
)

// limiter allows a number of attempts per key in a sliding window, for
// sign-ins: per address, and per username so one account can't be
// guessed at from many addresses.
type limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	seen  map[string][]time.Time
	sweep time.Time
}

func newLimiter(max int, window time.Duration, now func() time.Time) *limiter {
	return &limiter{max: max, window: window, now: now, seen: map[string][]time.Time{}}
}

// recent drops a key's attempts older than the window. Callers hold mu.
func (l *limiter) recent(key string, now time.Time) []time.Time {
	ts := l.seen[key]
	i := 0
	for i < len(ts) && now.Sub(ts[i]) >= l.window {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.seen, key)
		return nil
	}
	l.seen[key] = ts
	return ts
}

// clean drops keys with no recent attempts, at most once a window.
// Callers hold mu.
func (l *limiter) clean(now time.Time) {
	if now.Sub(l.sweep) < l.window {
		return
	}
	l.sweep = now
	for k := range l.seen {
		l.recent(k, now)
	}
}

// wait reports how long a key must wait before its next attempt; zero if
// it may try now.
func (l *limiter) wait(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.clean(now)
	ts := l.recent(key, now)
	if len(ts) < l.max {
		return 0
	}
	return ts[len(ts)-l.max].Add(l.window).Sub(now)
}

// add counts an attempt.
func (l *limiter) add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.seen[key] = append(l.recent(key, now), now)
}

// reset forgets a key's attempts (a successful sign-in).
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.seen, key)
}
