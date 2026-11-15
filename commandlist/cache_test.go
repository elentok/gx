package commandlist

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/modelrules"
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

type fakeTicker struct {
	c       chan time.Time
	stopped atomic.Bool
	period  atomic.Int64
}

func newFakeTicker() *fakeTicker { return &fakeTicker{c: make(chan time.Time)} }

func (f *fakeTicker) ticker(d time.Duration) (<-chan time.Time, func()) {
	f.period.Store(int64(d))
	return f.c, func() { f.stopped.Store(true) }
}

// modelProbe is a ModelProbe whose calls are counted and whose reply the test
// sets. When gate is non-nil each call waits for it to close.
type modelProbe struct {
	calls atomic.Int32
	mu    sync.Mutex
	rows  []Model
	err   error
	gate  chan struct{}
	dir   string
	env   []string
}

func (p *modelProbe) set(rows []Model, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rows, p.err = rows, err
}

func (p *modelProbe) probe(ctx context.Context, dir string, env []string) ([]Model, error) {
	p.calls.Add(1)
	if p.gate != nil {
		<-p.gate
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dir, p.env = dir, env
	return p.rows, p.err
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		runtime.Gosched()
	}
}

func hasModels(c *ModelCache, harness string) bool {
	_, ok := c.Models(harness)
	return ok
}

// runModelCache starts c.Run and returns a stop that cancels it and waits.
func runModelCache(t *testing.T, c *ModelCache) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	stop = func() { cancel(); <-done }
	t.Cleanup(stop)
	return stop
}

func TestModelCache_NoListUntilARefreshSucceeds(t *testing.T) {
	var claude, codex modelProbe
	claude.set([]Model{{Value: "opus", ID: "claude-opus-4-1"}}, nil)
	codex.set(nil, errors.New("boom"))
	c := NewModelCache(ModelCacheDeps{Claude: claude.probe, Codex: codex.probe, NewTicker: newFakeTicker().ticker})
	if hasModels(c, "claude") || hasModels(c, "unknown") {
		t.Fatal("list before any refresh")
	}
	runModelCache(t, c)
	waitFor(t, "claude list", func() bool { return hasModels(c, "claude") })
	waitFor(t, "codex probe", func() bool { return codex.calls.Load() == 1 })
	if hasModels(c, "codex") {
		t.Fatal("codex has a list after a failed read")
	}
}

func TestModelCache_ReadsRunInDirWithEnv(t *testing.T) {
	var claude, codex modelProbe
	claude.set([]Model{}, nil)
	codex.set([]Model{}, nil)
	env := []string{"A=1"}
	c := NewModelCache(ModelCacheDeps{Dir: "/work", Env: env, Claude: claude.probe, Codex: codex.probe, NewTicker: newFakeTicker().ticker})
	runModelCache(t, c)
	waitFor(t, "both lists", func() bool { return hasModels(c, "claude") && hasModels(c, "codex") })
	for name, p := range map[string]*modelProbe{"claude": &claude, "codex": &codex} {
		p.mu.Lock()
		if p.dir != "/work" || len(p.env) != 1 || p.env[0] != "A=1" {
			t.Errorf("%s probe ran in %q with %v", name, p.dir, p.env)
		}
		p.mu.Unlock()
	}
}

func TestModelCache_OverlappingTriggersRunOneProbe(t *testing.T) {
	claude := modelProbe{gate: make(chan struct{})}
	claude.set([]Model{{Value: "opus", ID: "claude-opus-4-1"}}, nil)
	var codex modelProbe
	codex.set([]Model{}, nil)
	c := NewModelCache(ModelCacheDeps{Claude: claude.probe, Codex: codex.probe, NewTicker: newFakeTicker().ticker})
	runModelCache(t, c)
	waitFor(t, "start read", func() bool { return claude.calls.Load() == 1 })
	c.Refresh("claude") // the picker opens
	c.Refresh("claude")
	close(claude.gate)
	waitFor(t, "claude list", func() bool { return hasModels(c, "claude") })
	if got := claude.calls.Load(); got != 1 {
		t.Fatalf("claude probes = %d, want 1", got)
	}
	c.Refresh("claude")
	waitFor(t, "second read", func() bool { return claude.calls.Load() == 2 })
}

func TestModelCache_FailedRefreshKeepsLastGoodList(t *testing.T) {
	var claude, codex modelProbe
	claude.set([]Model{{Value: "opus", ID: "claude-opus-4-1"}}, nil)
	codex.set([]Model{}, nil)
	c := NewModelCache(ModelCacheDeps{Claude: claude.probe, Codex: codex.probe, NewTicker: newFakeTicker().ticker})
	runModelCache(t, c)
	waitFor(t, "claude list", func() bool { return hasModels(c, "claude") })

	claude.set(nil, errors.New("boom"))
	// Refresh again until the failed read has run: a refresh only starts once
	// the slot is free.
	waitFor(t, "failed read", func() bool {
		c.Refresh("claude")
		return claude.calls.Load() >= 2
	})
	got, ok := c.Models("claude")
	if !ok || len(got) != 1 || got[0].ID != "claude-opus-4-1" {
		t.Fatalf("Models = %v, %v; want the last good list", got, ok)
	}
}

func TestModelCache_TimerRefreshesEverySixHours(t *testing.T) {
	var claude, codex modelProbe
	claude.set([]Model{}, nil)
	codex.set([]Model{}, nil)
	tk := newFakeTicker()
	c := NewModelCache(ModelCacheDeps{Claude: claude.probe, Codex: codex.probe, NewTicker: tk.ticker})
	stop := runModelCache(t, c)
	waitFor(t, "lists", func() bool { return hasModels(c, "claude") && hasModels(c, "codex") })
	if got := time.Duration(tk.period.Load()); got != 6*time.Hour {
		t.Fatalf("timer period = %v, want 6h", got)
	}
	tk.c <- time.Time{}
	waitFor(t, "timer reads", func() bool { return claude.calls.Load() == 2 && codex.calls.Load() == 2 })
	stop()
	if !tk.stopped.Load() {
		t.Fatal("timer still running after Run returned")
	}
}

func TestModelCache_StopCancelsReadsAndNoRefreshAfter(t *testing.T) {
	var started atomic.Bool
	probe := func(ctx context.Context, _ string, _ []string) ([]Model, error) {
		started.Store(true)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	var codex modelProbe
	codex.set([]Model{}, nil)
	c := NewModelCache(ModelCacheDeps{Claude: probe, Codex: codex.probe, NewTicker: newFakeTicker().ticker})
	stop := runModelCache(t, c)
	waitFor(t, "read started", started.Load)
	stop() // returns only after the blocked read ends
	before := codex.calls.Load()
	c.Refresh("codex")
	if codex.calls.Load() != before {
		t.Fatal("refresh ran after Run returned")
	}
}

func TestModelCache_EntriesHaveTheRulesShape(t *testing.T) {
	var claude, codex modelProbe
	claude.set([]Model{}, nil)
	codex.set([]Model{{Value: "gpt-5", ID: "gpt-5-codex", Hidden: true, Upgrade: "gpt-6"}}, nil)
	c := NewModelCache(ModelCacheDeps{Claude: claude.probe, Codex: codex.probe, NewTicker: newFakeTicker().ticker})
	if _, ok := c.Entries("codex"); ok {
		t.Fatal("entries before a refresh")
	}
	runModelCache(t, c)
	waitFor(t, "codex list", func() bool { return hasModels(c, "codex") })
	got, _ := c.Entries("codex")
	want := modelrules.Entry{Value: "gpt-5", ID: "gpt-5-codex", Hidden: true, Successor: "gpt-6"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Entries = %v, want [%v]", got, want)
	}
}
