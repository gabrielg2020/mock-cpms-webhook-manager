package ratelimit

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// newTestLimiter returns a limiter and a function that moves its clock forward.
func newTestLimiter(limit int, per time.Duration) (*Limiter, func(time.Duration)) {
	now := time.Date(2026, time.November, 15, 12, 0, 0, 0, time.UTC)
	limiter := New(limit, per)
	limiter.now = func() time.Time { return now }
	return limiter, func(by time.Duration) { now = now.Add(by) }
}

func TestAllowStopsAtTheLimit(t *testing.T) {
	limiter, advance := newTestLimiter(3, time.Minute)

	for attempt := 1; attempt <= 3; attempt++ {
		if allowed, _ := limiter.Allow("a"); !allowed {
			t.Fatalf("attempt %d was refused, want the first 3 allowed", attempt)
		}
	}

	advance(20 * time.Second)
	allowed, retryAfter := limiter.Allow("a")
	if allowed {
		t.Error("attempt 4 was allowed")
	}
	if retryAfter != 40*time.Second {
		t.Errorf("wait = %v, want the 40s left in the window", retryAfter)
	}

	if allowed, _ := limiter.Allow("b"); !allowed {
		t.Error("another key was refused because of the first key's attempts")
	}
}

func TestAllowStartsAgainAfterTheWindow(t *testing.T) {
	limiter, advance := newTestLimiter(2, time.Minute)
	limiter.Allow("a")
	limiter.Allow("a")
	if allowed, _ := limiter.Allow("a"); allowed {
		t.Fatal("attempt 3 was allowed")
	}

	advance(59 * time.Second)
	if allowed, _ := limiter.Allow("a"); allowed {
		t.Error("allowed one second before the window ended")
	}

	advance(time.Second)
	if allowed, _ := limiter.Allow("a"); !allowed {
		t.Error("refused after the window ended")
	}
	if allowed, _ := limiter.Allow("a"); !allowed {
		t.Error("the new window did not start with a full count")
	}
}

func TestAllowKeepsItsMemoryBounded(t *testing.T) {
	limiter, advance := newTestLimiter(5, time.Minute)
	for key := range maxKeys {
		limiter.Allow(strconv.Itoa(key))
	}

	if allowed, _ := limiter.Allow("one more"); allowed {
		t.Error("a new key was allowed when the limiter was full, so its memory is not bounded")
	}
	if got := len(limiter.windows); got != maxKeys {
		t.Errorf("keys held = %d, want %d", got, maxKeys)
	}
	if allowed, _ := limiter.Allow("0"); !allowed {
		t.Error("a key already held was refused because the limiter was full")
	}

	advance(time.Minute)
	if allowed, _ := limiter.Allow("one more"); !allowed {
		t.Error("a new key was refused after every window had ended")
	}
	if got := len(limiter.windows); got != 1 {
		t.Errorf("keys held after the windows ended = %d, want 1", got)
	}
}

func TestAllowIsSafeToCallFromManyRequestsAtOnce(t *testing.T) {
	limiter := New(100, time.Minute)

	var wait sync.WaitGroup
	allowed := make(chan bool, 400)
	for range 400 {
		wait.Go(func() {
			ok, _ := limiter.Allow("a")
			allowed <- ok
		})
	}
	wait.Wait()
	close(allowed)

	count := 0
	for ok := range allowed {
		if ok {
			count++
		}
	}
	if count != 100 {
		t.Errorf("attempts allowed = %d, want exactly 100", count)
	}
}
