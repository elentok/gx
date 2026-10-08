package tickets

import (
	"maps"
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// epicLandedCostsFn/sessionCostFn are swapped in tests so a stubbed
// aggregation tick never touches real epic/transcript files.
var (
	epicLandedCostsFn = ralphloop.EpicLandedCosts
	sessionCostFn     = ralphloop.SessionCost
)

// budgetConfig is process-wide and write-once, loaded from config at
// startup — there is no hot-reload, matching every other config section in
// the codebase.
var budgetConfig config.BudgetConfig

func SetBudgetConfig(cfg config.BudgetConfig) {
	budgetConfig = cfg
}

// notificationsConfig is process-wide and write-once like budgetConfig —
// budget notifications sum across every running epic, so they use this
// shared config rather than any one epic's own chat wiring.
var notificationsConfig config.NotificationsConfig

func SetNotificationsConfig(cfg config.NotificationsConfig) {
	notificationsConfig = cfg
}

// epicCostSnapshot is one running epic's state as the cost aggregator needs
// it: enough to load the epic's on-disk landed costs and locate each running
// ticket's live session.
type epicCostSnapshot struct {
	EpicName   string
	ScratchDir string
	Tickets    map[string]costTicketSnapshot
}

type costTicketSnapshot struct {
	Running   bool
	Agent     ralphloop.AgentKind
	Cwd       string
	SessionID string
	PaneID    string
	TabID     string
}

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
// every epic running under the current Attach session. All figures it
// produces are estimated, not literal billed dollars.
type costAggregator struct {
	mu sync.Mutex

	total    float64
	perEpic  map[string]float64
	unpriced int
	// consecutiveMisses is an internal reliability signal (never surfaced in
	// any UI, a deliberate scope boundary): incremented once per tick that
	// had any failed epic-load or transcript read, reset on a clean tick.
	consecutiveMisses int

	// baselines lives until the next reset, not per run, so an epic that
	// finishes and relaunches within the same Attach session keeps its
	// original baseline.
	baselines       map[string]float64
	transcriptCache map[transcriptCacheKey]transcriptCacheEntry

	// budgetHighWaterMark is the highest configured threshold already
	// notified for, reset alongside baselines/total at every attach
	// zero-to-one/one-to-zero transition so reattach re-arms it (see ticket
	// 05's reattach-reset requirement).
	budgetHighWaterMark float64

	// softLimitLatch is latched: tripped from the moment the soft limit is
	// crossed until an accepted override (or reattach) clears it — never
	// self-clears on a poll tick alone (see checkBudgetSoftLimit).
	softLimitLatch budgetLimitLatch
	// hardLimitLatch mirrors softLimitLatch for the hard-limit kill (ticket
	// 08) — a separate latch so an override of one limit never affects the
	// other.
	hardLimitLatch budgetLimitLatch
}

var costAgg = &costAggregator{}

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

// reset clears every per-session figure, baseline, cache and latch, so a
// later Attach session in the same process starts clean.
func (a *costAggregator) reset() {
	a.mu.Lock()
	a.total = 0
	a.perEpic = map[string]float64{}
	a.unpriced = 0
	a.consecutiveMisses = 0
	a.baselines = map[string]float64{}
	a.transcriptCache = map[transcriptCacheKey]transcriptCacheEntry{}
	a.budgetHighWaterMark = 0
	a.softLimitLatch.reset()
	a.hardLimitLatch.reset()
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
// epics into the cached total/perEpic/unpriced getters.
func (a *costAggregator) tick(snapshot []epicCostSnapshot) {
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

	a.checkBudgetThresholds(sum)
	a.checkBudgetSoftLimit(sum)
	a.checkBudgetHardLimit(sum, snapshot)
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
