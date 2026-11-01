package commandlist

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func countingProbe(calls *atomic.Int32, err error) Probe {
	return func(ctx context.Context, dir string) ([]Entry, error) {
		calls.Add(1)
		if err != nil {
			return nil, err
		}
		return []Entry{{Name: "probed-" + dir, Source: SourceSkill}}, nil
	}
}

func hasName(entries []Entry, name string) bool {
	for _, e := range entries {
		if e.Name == name {
			return true
		}
	}
	return false
}

func TestCache_ServesFreshThenRefreshesAfterMaxAge(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	c := NewCache(0, clock.Now)
	key := Key{Harness: "claude", Dir: "/a"}
	var calls atomic.Int32
	probe := countingProbe(&calls, nil)

	r := c.Get(context.Background(), key, probe)
	if r.Degraded || !hasName(r.Entries, "probed-/a") {
		t.Fatalf("first result = %+v", r)
	}
	clock.Advance(59 * time.Second)
	c.Get(context.Background(), key, probe)
	if got := calls.Load(); got != 1 {
		t.Fatalf("probe calls inside max age = %d, want 1", got)
	}
	clock.Advance(2 * time.Second)
	c.Get(context.Background(), key, probe)
	if got := calls.Load(); got != 2 {
		t.Fatalf("probe calls after max age = %d, want 2", got)
	}
}

func TestCache_CachedReturnsStaleValueWithAge(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	c := NewCache(10*time.Second, clock.Now)
	key := Key{Harness: "claude", Dir: "/a"}
	if _, _, ok := c.Cached(key); ok {
		t.Fatal("Cached ok on empty cache")
	}
	var calls atomic.Int32
	c.Get(context.Background(), key, countingProbe(&calls, nil))
	clock.Advance(30 * time.Second)
	entries, age, ok := c.Cached(key)
	if !ok || age != 30*time.Second || !hasName(entries, "probed-/a") {
		t.Fatalf("Cached = %v, %v, %v", entries, age, ok)
	}
}

func TestCache_SimultaneousGetsShareOneProbe(t *testing.T) {
	c := NewCache(0, nil)
	key := Key{Harness: "claude", Dir: "/a"}
	var calls atomic.Int32
	release := make(chan struct{})
	probe := func(ctx context.Context, dir string) ([]Entry, error) {
		calls.Add(1)
		<-release
		return []Entry{{Name: "x"}}, nil
	}
	const n = 5
	var started, done sync.WaitGroup
	started.Add(n)
	done.Add(n)
	for range n {
		go func() {
			defer done.Done()
			started.Done()
			c.Get(context.Background(), key, probe)
		}()
	}
	started.Wait()
	// A late goroutine would hit the fresh cache, so the count holds either way.
	for calls.Load() == 0 {
		runtime.Gosched()
	}
	close(release)
	done.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("probe calls = %d, want 1", got)
	}
}

func TestCache_FailureReturnsBuiltinsDegradedAndIsNotCached(t *testing.T) {
	c := NewCache(0, nil)
	key := Key{Harness: "claude", Dir: "/a"}
	var calls atomic.Int32
	r := c.Get(context.Background(), key, countingProbe(&calls, errors.New("boom")))
	if !r.Degraded || len(r.Entries) != len(BuiltinsFor("claude", "linux").Entries) {
		t.Fatalf("failed result = degraded %v, %d entries", r.Degraded, len(r.Entries))
	}
	if _, _, ok := c.Cached(key); ok {
		t.Fatal("failure was cached")
	}
	r = c.Get(context.Background(), key, countingProbe(&calls, nil))
	if r.Degraded || calls.Load() != 2 {
		t.Fatalf("after failure: degraded %v, calls %d", r.Degraded, calls.Load())
	}
}

// Codex's "$" menu is skills only; its built-ins belong to "/".
func TestCache_DollarListHasNoBuiltins(t *testing.T) {
	c := NewCache(0, time.Now)
	key := Key{Harness: "codex", Dir: "/a", Kind: "$"}
	var calls atomic.Int32

	ok := c.Get(context.Background(), key, countingProbe(&calls, nil))
	if len(ok.Entries) != 1 || ok.Entries[0].Name != "probed-/a" {
		t.Fatalf("ok $ entries = %+v, want only the probed skill", ok.Entries)
	}
	failed := c.Get(context.Background(), Key{Harness: "codex", Dir: "/b", Kind: "$"}, countingProbe(&calls, errors.New("boom")))
	if !failed.Degraded || len(failed.Entries) != 0 {
		t.Fatalf("failed $ = %+v, want degraded with no entries", failed)
	}
}

func TestCache_KeysDoNotShare(t *testing.T) {
	c := NewCache(0, nil)
	var calls atomic.Int32
	probe := countingProbe(&calls, nil)
	keys := []Key{
		{Harness: "claude", Dir: "/a"},
		{Harness: "claude", Dir: "/b"},
		{Harness: "codex", Dir: "/a", Kind: "$"},
		{Harness: "codex", Dir: "/a", Kind: "/"},
	}
	for _, k := range keys {
		c.Get(context.Background(), k, probe)
	}
	if got := calls.Load(); int(got) != len(keys) {
		t.Fatalf("probe calls = %d, want %d", got, len(keys))
	}
}
