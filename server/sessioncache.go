package server

import (
	"sync"
	"time"
)

// recordedMissTTL is how long a "no transcript yet" answer is trusted. A hit is
// kept for good: a recorded session never goes away.
const recordedMissTTL = 15 * time.Second

// recordedCache remembers sessionRecorded answers. Every snapshot asks once per
// ticket, and each miss globs every project dir, which is too slow to repeat.
type recordedCache struct {
	mu      sync.Mutex
	entries map[string]recordedEntry
}

type recordedEntry struct {
	found bool
	at    time.Time
}

// lookup returns the cached answer and whether it is still valid.
func (c *recordedCache) lookup(key string, now time.Time) (found, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, hit := c.entries[key]
	if !hit {
		return false, false
	}
	if e.found || now.Sub(e.at) < recordedMissTTL {
		return e.found, true
	}
	return false, false
}

func (c *recordedCache) store(key string, found bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]recordedEntry{}
	}
	c.entries[key] = recordedEntry{found: found, at: now}
}
