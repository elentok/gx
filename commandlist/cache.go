package commandlist

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// DefaultMaxAge is how long a successful probe is served from the cache.
const DefaultMaxAge = 60 * time.Second

// Probe asks a harness for its commands in dir.
type Probe func(ctx context.Context, dir string) ([]Entry, error)

// Key identifies one cached list. Kind separates Codex's "$" and "/" lists
// and is empty for harnesses with one list.
type Key struct {
	Harness string
	Dir     string
	Kind    string
}

// Result is a command list. Degraded means the probe failed and Entries are
// only the built-ins.
type Result struct {
	Entries  []Entry
	Degraded bool
}

type cacheEntry struct {
	entries []Entry
	at      time.Time
}

type flight struct {
	done chan struct{}
	res  Result
}

// Cache holds probe results per Key. Simultaneous Gets for one key share a
// single probe run.
type Cache struct {
	maxAge time.Duration
	now    func() time.Time

	mu      sync.Mutex
	entries map[Key]cacheEntry
	flights map[Key]*flight
}

// NewCache returns a Cache. A zero maxAge means DefaultMaxAge; a nil now means
// time.Now.
func NewCache(maxAge time.Duration, now func() time.Time) *Cache {
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	if now == nil {
		now = time.Now
	}
	return &Cache{
		maxAge:  maxAge,
		now:     now,
		entries: map[Key]cacheEntry{},
		flights: map[Key]*flight{},
	}
}

// Cached returns the last successful list for key and its age, even when it is
// older than the max age. ok is false when there is none.
func (c *Cache) Cached(key Key) (entries []Entry, age time.Duration, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, 0, false
	}
	return e.entries, c.now().Sub(e.at), true
}

// Get returns the list for key: from the cache while it is fresh, otherwise
// from probe. A failed probe returns the built-ins, Degraded, and is not
// cached.
func (c *Cache) Get(ctx context.Context, key Key, probe Probe) Result {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.now().Sub(e.at) < c.maxAge {
		c.mu.Unlock()
		return Result{Entries: e.entries}
	}
	if f, ok := c.flights[key]; ok {
		c.mu.Unlock()
		<-f.done
		return f.res
	}
	f := &flight{done: make(chan struct{})}
	c.flights[key] = f
	c.mu.Unlock()

	reply, err := probe(ctx, key.Dir)

	c.mu.Lock()
	if err != nil {
		f.res = Result{Entries: BuiltinsFor(key.Harness, runtime.GOOS).Entries, Degraded: true}
	} else {
		f.res = Result{Entries: Merge(reply, BuiltinsFor(key.Harness, runtime.GOOS).Entries)}
		c.entries[key] = cacheEntry{entries: f.res.Entries, at: c.now()}
	}
	delete(c.flights, key)
	c.mu.Unlock()
	close(f.done)
	return f.res
}
