package commandlist

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/elentok/gx/modelrules"
)

// ModelRefreshInterval is how often the model lists refresh on their own.
const ModelRefreshInterval = 6 * time.Hour

// ModelProbe reads one harness's model list in dir with env.
type ModelProbe func(ctx context.Context, dir string, env []string) ([]Model, error)

// Ticker returns a channel that fires every d, and a stop func.
type Ticker func(d time.Duration) (<-chan time.Time, func())

// ModelCacheDeps is what NewModelCache needs. Nil fields take their defaults.
type ModelCacheDeps struct {
	// Dir and Env are where both reads run.
	Dir string
	Env []string
	// Claude and Codex default to ProbeClaudeModels and ProbeCodexModels.
	Claude, Codex ModelProbe
	// Interval defaults to ModelRefreshInterval; NewTicker to a time.Ticker.
	Interval  time.Duration
	NewTicker Ticker
	Log       *slog.Logger
}

type modelSlot struct {
	probe   ModelProbe
	models  []Model
	have    bool
	running bool
}

// ModelCache keeps the last good model list of each harness. Reads never wait
// for a probe and never fail; refreshes run in the background.
type ModelCache struct {
	env       []string
	interval  time.Duration
	newTicker Ticker
	log       *slog.Logger
	workDir   string

	wg sync.WaitGroup

	mu    sync.Mutex
	slots map[string]*modelSlot
	ctx   context.Context // set by Run; refreshes before it are dropped
}

// NewModelCache returns a ModelCache with no lists yet.
func NewModelCache(d ModelCacheDeps) *ModelCache {
	if d.Claude == nil {
		d.Claude = ProbeClaudeModels
	}
	if d.Codex == nil {
		d.Codex = ProbeCodexModels
	}
	if d.Interval <= 0 {
		d.Interval = ModelRefreshInterval
	}
	if d.NewTicker == nil {
		d.NewTicker = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &ModelCache{
		env: d.Env, workDir: d.Dir, interval: d.Interval, newTicker: d.NewTicker, log: d.Log,
		slots: map[string]*modelSlot{
			harnessClaude: {probe: d.Claude},
			harnessCodex:  {probe: d.Codex},
		},
	}
}

// Models returns the last good list of harnessName. ok is false for "no list
// yet" and for an unknown harness.
func (c *ModelCache) Models(harnessName string) (models []Model, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.slots[harnessName]
	if s == nil || !s.have {
		return nil, false
	}
	return append([]Model(nil), s.models...), true
}

// Entries is Models in the shape the modelrules functions take.
func (c *ModelCache) Entries(harnessName string) (entries []modelrules.Entry, ok bool) {
	models, ok := c.Models(harnessName)
	if !ok {
		return nil, false
	}
	entries = make([]modelrules.Entry, len(models))
	for i, m := range models {
		entries[i] = modelrules.Entry{Value: m.Value, ID: m.ID, Hidden: m.Hidden, Successor: m.Upgrade}
	}
	return entries, true
}

// Refreshing reports whether a read of harnessName is running now.
func (c *ModelCache) Refreshing(harnessName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.slots[harnessName]
	return s != nil && s.running
}

// Refresh starts a background read of harnessName and returns at once. It does
// nothing while that harness is already being read, or before Run.
func (c *ModelCache) Refresh(harnessName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.slots[harnessName]
	if s == nil || s.running || c.ctx == nil || c.ctx.Err() != nil {
		return
	}
	s.running = true
	ctx := c.ctx
	c.wg.Go(func() {
		models, err := s.probe(ctx, c.workDir, c.env)
		c.mu.Lock()
		defer c.mu.Unlock()
		s.running = false
		if err != nil {
			if ctx.Err() == nil {
				c.log.Warn("model list refresh failed", "harness", harnessName, "err", err)
			}
			return
		}
		s.models, s.have = models, true
	})
}

func (c *ModelCache) refreshAll() {
	c.Refresh(harnessClaude)
	c.Refresh(harnessCodex)
}

// Run refreshes both lists, then again every interval, until ctx ends. It
// waits for reads in flight before it returns.
func (c *ModelCache) Run(ctx context.Context) {
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
	ticks, stop := c.newTicker(c.interval)
	defer stop()
	c.refreshAll()
	for {
		select {
		case <-ctx.Done():
			c.wg.Wait()
			return
		case <-ticks:
			c.refreshAll()
		}
	}
}
