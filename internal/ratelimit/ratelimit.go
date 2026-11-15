package ratelimit

import (
	"sync"
	"time"
)

// Each key held costs memory, and an attacker chooses how many keys there are.
const maxKeys = 10_000

type window struct {
	started time.Time
	count   int
}

// Limiter allows each key a number of attempts in a window of time. The windows starts
// at the key's first attempt, and the count starts again when it ends.
type Limiter struct {
	limit   int
	length  time.Duration
	now     func() time.Time
	mu      sync.Mutex
	windows map[string]window
}

func New(limit int, per time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		length:  per,
		now:     time.Now,
		windows: make(map[string]window),
	}
}

// Allow counts one attempt for the key. It reports whether the attempt is within the limit
// and, when it is not, how long the key has to wait
func (l *Limiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	current, held := l.windows[key]
	if held && now.Sub(current.started) >= l.length {
		delete(l.windows, key)
		held = false
	}
	if !held {
		if len(l.windows) >= maxKeys {
			l.forgetFinished(now)
		}
		// Still fulls; every key held is inside its window. Refusing new keys keeps
		// the memory bounded without giving anyone a fresh count.
		if len(l.windows) >= maxKeys {
			return false, l.length
		}
		current = window{started: now}
	}

	current.count++
	l.windows[key] = current
	if current.count > l.limit {
		return false, current.started.Add(l.length).Sub(now)
	}
	return true, 0
}

func (l *Limiter) forgetFinished(now time.Time) {
	for key, held := range l.windows {
		if now.Sub(held.started) >= l.length {
			delete(l.windows, key)
		}
	}
}
