package tickets

import (
	"maps"
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// epicLandedCostsFn/sessionCostFn are swapped in tests so a stubbed
// aggregation tick never touches real epic/transcript files, mirroring
// runRalphLoop's own seam-swap pattern (see loop_registry.go).
var (
	epicLandedCostsFn = ralphloop.EpicLandedCosts
	sessionCostFn     = ralphloop.SessionCost
)

const defaultTickInterval = 30 * time.Second

// transcriptCacheKey identifies one Claude session's transcript. Codex never
// reaches this cache — sessionCostFn is only called for Claude tickets, see
// costAggregator.tick.
type transcriptCacheKey struct {
	cwd, sessionID string
}

type transcriptCacheEntry struct {
	mtime time.Time
	cost  float64
}

// costAggregator computes the estimated API-equivalent dollar spend across
// every epic running under the current Attach session, polled on a fixed
// interval for the poller's lifetime (see ticket 04's "What to build"). All
// figures it produces are estimated, not literal billed dollars.
type costAggregator struct {
	mu      sync.Mutex
	stopCh  chan struct{}
	doneCh  chan struct{}
	running bool

	total    float64
	perEpic  map[string]float64
	unpriced int
	// consecutiveMisses is an internal reliability signal (never surfaced in
	// any UI, a deliberate scope boundary): incremented once per tick that
	// had any failed epic-load or transcript read, reset on a clean tick.
	consecutiveMisses int

	// baselines lives for the whole poller lifetime, not per epicRun, so an
	// epic that finishes and relaunches within the same Attach session keeps
	// its original baseline (epicRun itself is deleted from loopRegistry.runs
	// on finish, then recreated fresh on relaunch).
	baselines       map[string]float64
	transcriptCache map[transcriptCacheKey]transcriptCacheEntry

	// tickInterval is a field, not a hardcoded const in the ticker call, so
	// tests can inject a short interval instead of waiting out a real 30s.
	tickInterval time.Duration
}

var costAgg = &costAggregator{tickInterval: defaultTickInterval}

// startCostAggregator starts costAgg's poller goroutine if it isn't already
// running, called from tryStart at the attach zero-to-one transition.
func startCostAggregator() {
	costAgg.start()
}

// stopCostAggregator stops costAgg's poller goroutine and blocks until it has
// fully exited, called from finish at the attach one-to-zero transition so a
// caller like CanQuit/tests can rely on it being gone once finish returns.
func stopCostAggregator() {
	costAgg.stop()
}

// LiveSpend returns the current Attach session's estimated API-equivalent
// cost, summed across every running epic's (landed-since-baseline +
// in-flight) contribution as of the last poll tick.
func LiveSpend() float64 {
	return costAgg.liveSpend()
}

// LiveSpendByEpic returns a copy of the current per-epic breakdown behind
// LiveSpend, keyed by epic name.
func LiveSpendByEpic() map[string]float64 {
	return costAgg.liveSpendByEpic()
}

// UnpricedRunningCount returns how many currently-running iterations have no
// cost source (Codex, see ticket 04's "Codex exclusion") and so are excluded
// from LiveSpend entirely rather than priced as $0.
func UnpricedRunningCount() int {
	return costAgg.unpricedRunningCount()
}

func (a *costAggregator) start() {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.total = 0
	a.perEpic = map[string]float64{}
	a.unpriced = 0
	a.consecutiveMisses = 0
	a.baselines = map[string]float64{}
	a.transcriptCache = map[transcriptCacheKey]transcriptCacheEntry{}
	a.stopCh = make(chan struct{})
	a.doneCh = make(chan struct{})
	interval := a.tickInterval
	if interval <= 0 {
		interval = defaultTickInterval
	}
	stopCh := a.stopCh
	doneCh := a.doneCh
	a.mu.Unlock()

	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				a.tick()
			}
		}
	}()
}

func (a *costAggregator) stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	stopCh := a.stopCh
	doneCh := a.doneCh
	a.mu.Unlock()

	close(stopCh)
	<-doneCh

	// Reset so a later Attach session in the same process starts clean —
	// baselines/transcriptCache are scoped to one poller lifetime.
	a.mu.Lock()
	a.total = 0
	a.perEpic = map[string]float64{}
	a.unpriced = 0
	a.consecutiveMisses = 0
	a.baselines = map[string]float64{}
	a.transcriptCache = map[transcriptCacheKey]transcriptCacheEntry{}
	a.mu.Unlock()
}

func (a *costAggregator) liveSpend() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.total
}

func (a *costAggregator) liveSpendByEpic() map[string]float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]float64, len(a.perEpic))
	maps.Copy(out, a.perEpic)
	return out
}

func (a *costAggregator) unpricedRunningCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.unpriced
}

// tick runs one aggregation pass: for every running epic, baseline it on
// first observation, then sum (landed-since-baseline + in-flight) across
// epics into the cached total/perEpic/unpriced getters. Reads the registry's
// running-epic state in one locked snapshot up front, then does its own
// (unlocked, potentially slow) disk I/O against that copy — see
// loopRegistry.costSnapshot.
func (a *costAggregator) tick() {
	snapshot := ralphLoopRegistry.costSnapshot()

	a.mu.Lock()
	baselines := a.baselines
	perEpic := make(map[string]float64, len(snapshot))
	unpriced := 0
	missed := false
	a.mu.Unlock()

	for _, epic := range snapshot {
		total, perTicket, err := epicLandedCostsFn(epic.ScratchDir, epic.EpicName)
		if err != nil {
			missed = true
			a.mu.Lock()
			total = a.perEpic[epic.EpicName]
			a.mu.Unlock()
			perTicket = nil
		}

		a.mu.Lock()
		if _, ok := baselines[epic.EpicName]; !ok {
			baselines[epic.EpicName] = total
		}
		baseline := baselines[epic.EpicName]
		a.mu.Unlock()

		inFlight := 0.0
		for identifier, ticket := range epic.Tickets {
			if !ticket.Running {
				continue
			}
			if perTicket[identifier] != 0 {
				// Already landed on disk and folded into total above — don't
				// also count it as in-flight this tick (the double-count
				// guard).
				continue
			}
			if ticket.Agent == ralphloop.AgentCodex {
				unpriced++
				continue
			}
			cost, ok := a.transcriptCost(ticket.Cwd, ticket.SessionID)
			if !ok {
				missed = true
				continue
			}
			inFlight += cost
		}

		perEpic[epic.EpicName] = (total - baseline) + inFlight
	}

	sum := 0.0
	for _, cost := range perEpic {
		sum += cost
	}

	a.mu.Lock()
	a.baselines = baselines
	a.perEpic = perEpic
	a.total = sum
	a.unpriced = unpriced
	if missed {
		a.consecutiveMisses++
	} else {
		a.consecutiveMisses = 0
	}
	a.mu.Unlock()
}

// transcriptCost returns cwd/sessionID's Claude transcript cost, reusing the
// cached value if the transcript's mtime hasn't changed since the last tick
// (the mtime-guard: no re-parse when nothing changed). ok is false on a Stat
// failure or a session-cost miss — a miss for this ticket this tick only,
// with the cache left untouched so the next tick retries.
func (a *costAggregator) transcriptCost(cwd, sessionID string) (cost float64, ok bool) {
	if cwd == "" || sessionID == "" {
		return 0, false
	}
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		return 0, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	key := transcriptCacheKey{cwd: cwd, sessionID: sessionID}

	a.mu.Lock()
	entry, cached := a.transcriptCache[key]
	a.mu.Unlock()
	if cached && !info.ModTime().After(entry.mtime) {
		return entry.cost, true
	}

	sessionCost, sessionOK, err := sessionCostFn(cwd, sessionID)
	if err != nil || !sessionOK {
		return 0, false
	}

	a.mu.Lock()
	a.transcriptCache[key] = transcriptCacheEntry{mtime: info.ModTime(), cost: sessionCost}
	a.mu.Unlock()
	return sessionCost, true
}
