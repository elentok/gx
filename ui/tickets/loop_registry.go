package tickets

import (
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

// epicRun is one epic's in-flight ralph-loop state.
type epicRun struct {
	eventDrainDone chan struct{}
	done, total    int
	sink           *ralphloop.ChannelEventSink
	gate           *ralphloop.Gate
	// scope is written once, by RunOptions.OnScopeResolved shortly after the
	// run starts (see cmdStartImplement) — until then it's the zero RunScope,
	// on which Add is a documented no-op.
	scope ralphloop.RunScope
	// agent is written once, by setAgent right after tryStart succeeds in
	// cmdStartImplement — it lets the drain-then-replace combo
	// (handleDrainReplaceKey in drain_replace.go) capture which agent is
	// driving a live run and reuse it for the replacement run, since agentFor
	// stops returning ok=true the moment finish() deletes the epic from
	// r.runs.
	agent      ralphloop.AgentKind
	state      RunState
	finalError string
	// failureNotifier is set once, by cmdStartImplement right after
	// tryStart, and used only by finish, after the drain below completes:
	// it must stay live across the sink close/drain since the run has
	// already returned by the time finish runs, so sink itself is nil by
	// then (see ralphloop.EpicFailureReporter / EventSink.EpicFailed).
	failureNotifier ralphloop.EpicFailureNotifier
	tickets         map[string]RunTicketSnapshot
	startedAt       time.Time
	// pendingNotifyCloses queues reattach-scan notification ids to close,
	// populated by reduceLiveEvent (running in the event-drain goroutine,
	// which has no way to send a tea.Cmd itself) and drained into an actual
	// notify.Close cmd by the next syncRunSnapshot poll on the active tab.
	pendingNotifyCloses []string
	// pendingToasts queues in-app toasts (epic-complete, needs-repair
	// pause) the same way pendingNotifyCloses queues closes: reduceLiveEvent
	// runs in the event-drain goroutine, which has no way to dispatch a
	// tea.Cmd itself, so toasts wait here for the next syncRunSnapshot poll
	// on the active tab to drain and dispatch them.
	pendingToasts []notify.NotifyMsg
	// pendingReload marks that a ticket in this epic just parked and the
	// queue needs to re-read it from disk before rendering, since the parked
	// row's subtext comes from the ticket file (see drainPendingReload) —
	// not from the event payload, so the row is identical before and after a
	// gx restart.
	pendingReload bool
	// holdsAttach marks a run as one of the ones counted toward
	// loopRegistry.attachCount, so finish only releases the attach lock once
	// the run that actually incremented it ends.
	holdsAttach bool
	// parkedStalled is the human-clearable ticket list from the most recent
	// LiveEventEpicParked, valid only while state == RunStateParked.
	parkedStalled []ralphloop.StalledTicket
	// permitReserved marks the concurrency slot tryStart took on this run's
	// behalf as still unclaimed: the run's first Acquire consumes it instead of
	// competing for a fresh slot, and finish gives it back if the run ended
	// without ever acquiring.
	permitReserved bool
	// scratchDir is this run's repo `.scratch` dir, set unconditionally in
	// tryStart (even "" when the caller omitted it) — the live cost
	// aggregator's tick reads it to load the epic's on-disk state without
	// reaching back into tryStart's local variables.
	scratchDir string
}

type RunState string

const (
	RunStateRunning   RunState = "running"
	RunStateParked    RunState = "parked"
	RunStateCompleted RunState = "completed"
	RunStateFailed    RunState = "failed"
)

type RunTicketSnapshot struct {
	Identifier    string
	Label         string
	Running       bool
	Paused        bool
	Completed     bool
	PauseKind     ralphloop.PauseKind
	PauseReason   string
	ContextTokens int
	StartedAt     time.Time
	// SessionID/Cwd/Agent/PaneID/TabID identify the currently-running
	// iteration's live agent — populated from LiveEventIterationStarted and
	// cleared once LiveEventIterationFinished lands, so other tickets (live
	// cost aggregation, the hard-limit stop/repair seam) can locate the
	// iteration without reaching into ralphloop internals themselves.
	SessionID string
	Cwd       string
	Agent     ralphloop.AgentKind
	PaneID    string
	TabID     string
}

type RunSnapshot struct {
	EpicName      string
	State         RunState
	Done          int
	Total         int
	ContextTokens int
	Paused        bool
	FinalError    string
	Tickets       map[string]RunTicketSnapshot
	StartedAt     time.Time
	// StalledTickets mirrors epicRun.parkedStalled, meaningful only while
	// State == RunStateParked — which is itself what the Queue tab reads to
	// render a parked epic distinctly from running/queued (see
	// loopRegistry.parkedEpics).
	StalledTickets []ralphloop.StalledTicket
}

// loopRegistry enforces "an epic may not have two ralph-loops running at
// once, and at most maxConcurrent epics may run at once" per gx process
// (ticket 01's original single-epic version of this, capacity-lifted by
// ticket 03), which has to hold across every tickets.Model instance in the
// process — not just the one that launched a given run, since a
// worktree-context switch rebuilds the Model but the background loop keeps
// running. A package-level singleton, guarded by a mutex since the launching
// goroutine and the finishing goroutine both touch it, is the simplest thing
// that's still correct if this process ever hosts more than one
// tickets.Model (e.g. --all mode).
type loopRegistry struct {
	mu            sync.Mutex
	maxConcurrent int
	runs          map[string]*epicRun
	snapshots     map[string]*epicRun
	paused        bool
	// Errors survive run removal so every observer sees the failure until an
	// explicit acknowledgement clears it.
	lastErr map[string]error
	// attachCount/attachScratchDir reference-count this process's hold on the
	// per-repo attach lock (ticket 05): the lock is acquired once, on the
	// first tryStart to pass a scratchDir while attachCount is 0, and
	// released once attachCount drops back to 0 in finish.
	attachCount      int
	attachScratchDir string
	// attachErr carries the reason the most recent tryStart was rejected by
	// the attach lock specifically (as opposed to same-process double-start
	// or capacity), for the caller to surface instead of the generic
	// "already running" message. Cleared at the start of every tryStart.
	attachErr error
	// activeCount/permitCond back Acquire/Release (ralphloop.Permit) and count
	// slots tryStart has reserved but whose run has not acquired yet, so a
	// parked run can hold a runs[] entry forever (ticket 08) without counting
	// toward maxConcurrent while a just-started one still does.
	activeCount int
	permitCond  *sync.Cond
	// permitErr records the most recent mismatched Release (see release), which
	// would otherwise be invisible: the release is refused rather than allowed
	// to drive activeCount negative and inflate the cap for the rest of the
	// process's life.
	permitErr error
}

func newLoopRegistry(maxConcurrent int) *loopRegistry {
	r := &loopRegistry{
		maxConcurrent: maxConcurrent,
		runs:          map[string]*epicRun{},
		snapshots:     map[string]*epicRun{},
		lastErr:       map[string]error{},
	}
	r.permitCond = sync.NewCond(&r.mu)
	return r
}

// Acquire blocks until fewer than maxConcurrent epics currently hold a
// permit, then takes one. Implements ralphloop.Permit for RunOptions.Permit.
func (r *loopRegistry) Acquire() {
	r.acquireFor("")
}

// acquireFor takes epicName's slot: the one tryStart already reserved for it
// if that reservation is still unclaimed (so a started run never queues behind
// the very cap it was admitted under), otherwise a fresh slot, blocking until
// one frees. An epicName with no reservation — including "" — always takes the
// blocking path.
func (r *loopRegistry) acquireFor(epicName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.runs[epicName]; run != nil && run.permitReserved {
		run.permitReserved = false
		return
	}
	for r.activeCount >= r.maxConcurrent {
		r.permitCond.Wait()
	}
	r.activeCount++
}

// Release frees one permit this process previously acquired via Acquire,
// waking any run currently blocked waiting for a slot.
func (r *loopRegistry) Release() {
	if err := r.release(); err != nil {
		r.mu.Lock()
		r.permitErr = err
		r.mu.Unlock()
	}
}

// release is Release's reportable form: it refuses a release that has no
// matching acquire instead of decrementing past zero.
func (r *loopRegistry) release() error {
	r.mu.Lock()
	if r.activeCount <= 0 {
		r.mu.Unlock()
		return fmt.Errorf("release of a concurrency permit that was never acquired")
	}
	r.activeCount--
	r.mu.Unlock()
	r.permitCond.Broadcast()
	return nil
}

// permitError reports the most recent mismatched Release, or nil if every
// release so far had a matching acquire.
func (r *loopRegistry) permitError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.permitErr
}

// permitFor is the ralphloop.Permit to hand epicName's run: its first Acquire
// claims the slot tryStart reserved at start time, later ones (after a park
// released it) queue like any other.
func (r *loopRegistry) permitFor(epicName string) ralphloop.Permit {
	return runPermit{registry: r, epicName: epicName}
}

type runPermit struct {
	registry *loopRegistry
	epicName string
}

func (p runPermit) Acquire() { p.registry.acquireFor(p.epicName) }
func (p runPermit) Release() { p.registry.Release() }

var ralphLoopRegistry = newLoopRegistry(ui.Settings{}.MaxConcurrentEpics())

// ConfigureMaxConcurrentEpics updates the process-wide execution queue cap.
// Existing runs are left alone; a lower cap only delays subsequent starts.
func ConfigureMaxConcurrentEpics(maxConcurrent int) {
	ralphLoopRegistry.setMaxConcurrent(maxConcurrent)
}

func (r *loopRegistry) setMaxConcurrent(maxConcurrent int) {
	r.mu.Lock()
	r.maxConcurrent = max(maxConcurrent, 1)
	r.mu.Unlock()
	r.permitCond.Broadcast()
}

// budgetConfig is process-wide and write-once, loaded from config at
// startup (same call site as ConfigureMaxConcurrentEpics) — there is no
// hot-reload, matching every other config section in the codebase.
var budgetConfig config.BudgetConfig

// SetBudgetConfig stores cfg for the registry layer and the Queue tab's
// model to read via Model.settings.Budget.
func SetBudgetConfig(cfg config.BudgetConfig) {
	budgetConfig = cfg
}

var runRalphLoop = ralphloop.Run

// tryStart claims an epic slot and starts the stream drain before returning
// the producer sink. Snapshots, rather than the channel, are shared with views.
//
// scratchDir is variadic-as-optional so the many pre-existing callers
// (mostly tests exercising slot/pause/concurrency logic, not the attach
// lock) don't need updating: passed, it's the repo's `.scratch` dir and
// tryStart acquires ticket 05's per-repo attach lock the first time
// attachCount is 0; omitted, the run skips attach-lock participation
// entirely.
func (r *loopRegistry) tryStart(epicName string, done, total int, scratchDir ...string) (*ralphloop.ChannelEventSink, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attachErr = nil
	if r.paused {
		return nil, false
	}
	if _, exists := r.runs[epicName]; exists {
		return nil, false
	}
	// Reserve the slot here, under the same lock that records the run: the run
	// goroutine only reaches its own Acquire seconds later, after worktree and
	// reconcile work, and until then a poll tick that saw free capacity would
	// keep draining the pending list into runs that just block.
	if r.activeCount >= r.maxConcurrent {
		return nil, false
	}

	dir := ""
	if len(scratchDir) > 0 {
		dir = scratchDir[0]
	}
	holdsAttach := false
	if dir != "" {
		if r.attachCount == 0 {
			foreignPID, ok, err := acquireAttachLock(dir)
			if err != nil {
				r.attachErr = err
				return nil, false
			}
			if !ok {
				r.attachErr = fmt.Errorf("a ralph-loop is already running (attached by process %d)", foreignPID)
				return nil, false
			}
			r.attachScratchDir = dir
			startCostAggregator()
		}
		r.attachCount++
		holdsAttach = true
	}

	sink := ralphloop.NewChannelEventSink()
	run := &epicRun{
		eventDrainDone: make(chan struct{}),
		done:           done, total: total, sink: sink, gate: ralphloop.NewGate(),
		state: RunStateRunning, tickets: map[string]RunTicketSnapshot{},
		startedAt: time.Now(), holdsAttach: holdsAttach, permitReserved: true,
		scratchDir: dir,
	}
	r.activeCount++
	r.runs[epicName] = run
	r.snapshots[epicName] = run
	go func() {
		defer close(run.eventDrainDone)
		for event := range sink.Events() {
			r.reduceLiveEvent(epicName, event)
		}
	}()
	return sink, true
}

func (r *loopRegistry) reduceLiveEvent(epicName string, event ralphloop.LiveEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil {
		return
	}
	switch event.Kind {
	case ralphloop.LiveEventIterationStarted, ralphloop.LiveEventTicketReattached:
		if run.state == RunStateParked {
			run.state = RunStateRunning
			run.parkedStalled = nil
		}
		run.tickets[event.Identifier] = RunTicketSnapshot{
			Identifier: event.Identifier,
			Label:      event.Label,
			Running:    true,
			StartedAt:  time.Now(),
			SessionID:  event.SessionID,
			Cwd:        event.Cwd,
			Agent:      event.AgentKind,
			PaneID:     event.PaneID,
			TabID:      event.TabID,
		}
		if event.Kind == ralphloop.LiveEventTicketReattached {
			run.pendingNotifyCloses = append(run.pendingNotifyCloses, reattachNotifyID(epicName, event.Identifier))
		}
	case ralphloop.LiveEventContextOccupancy:
		ticket, ok := run.tickets[event.Identifier]
		if ok {
			ticket.ContextTokens = event.Tokens
			run.tickets[event.Identifier] = ticket
		}
	case ralphloop.LiveEventIterationPaused:
		for identifier, ticket := range run.tickets {
			if ticket.Label != event.Label {
				continue
			}
			ticket.Running = false
			ticket.Paused = true
			ticket.PauseKind = event.PauseKind
			ticket.PauseReason = event.Reason
			run.tickets[identifier] = ticket
			break
		}
		if event.PauseKind == ralphloop.PauseNeedsRepair {
			run.pendingToasts = append(run.pendingToasts, notify.NotifyMsg{
				Kind:    notify.KindWarning,
				Message: fmt.Sprintf("\U0001f6d1 %s paused: %s", event.Label, event.Reason),
			})
		}
	case ralphloop.LiveEventTicketNeedsHuman:
		// Preserves the toast/row-paused behavior the old
		// LiveEventIterationPaused/PauseNeedsRepair branch gave fault parks.
		// Both statuses set pendingReload so the queue re-reads the ticket
		// from disk and picks up its park-reason subtext (see
		// drainPendingReload).
		if event.Status == string(schema.StatusNeedsRepair) {
			ticket, ok := run.tickets[event.Identifier]
			if ok {
				ticket.Running = false
				ticket.Paused = true
				ticket.PauseKind = ralphloop.PauseNeedsRepair
				ticket.PauseReason = event.Reason
				run.tickets[event.Identifier] = ticket
			}
			run.pendingToasts = append(run.pendingToasts, notify.NotifyMsg{
				Kind:    notify.KindWarning,
				Message: fmt.Sprintf("\U0001f6d1 %s paused: %s", event.Identifier, event.Reason),
			})
		}
		if event.Status == string(schema.StatusNeedsAnswer) || event.Status == string(schema.StatusNeedsRepair) {
			run.pendingReload = true
		}
	case ralphloop.LiveEventIterationResumed:
		for identifier, ticket := range run.tickets {
			if ticket.Label != event.Label {
				continue
			}
			ticket.Running = true
			ticket.Paused = false
			ticket.PauseKind = ""
			ticket.PauseReason = ""
			run.tickets[identifier] = ticket
			run.pendingNotifyCloses = append(run.pendingNotifyCloses, reattachNotifyID(epicName, identifier))
			break
		}
	case ralphloop.LiveEventIterationFinished:
		ticket, ok := run.tickets[event.Identifier]
		if ok {
			ticket.Running = false
			ticket.Paused = false
			ticket.Completed = true
			ticket.PauseKind = ""
			ticket.PauseReason = ""
			ticket.ContextTokens = 0
			ticket.SessionID = ""
			ticket.Cwd = ""
			ticket.Agent = ""
			ticket.PaneID = ""
			ticket.TabID = ""
			run.tickets[event.Identifier] = ticket
			// Stats.Completed/Total already describe the whole epic, recomputed
			// from disk (see IterationStats) — read them rather than counting
			// this run's own landed tickets, so a resumed run's Done/Total match
			// what a fresh run over the same epic would report.
			run.done = event.Stats.Completed
			run.total = event.Stats.Total
		}
	case ralphloop.LiveEventEpicParked:
		run.state = RunStateParked
		run.parkedStalled = event.Stalled
	case ralphloop.LiveEventNotificationFailed:
		run.pendingToasts = append(run.pendingToasts, notify.NotifyMsg{
			Kind:    notify.KindWarning,
			Message: fmt.Sprintf("%s notification failed: %s", event.Channel, event.Reason),
		})
	case ralphloop.LiveEventEpicComplete:
		// Done/Total were already synced to the epic's true state by the last
		// LiveEventIterationFinished (or seeded correctly at tryStart if the
		// epic was already complete when this run started) — event.Completed
		// is this run's own landed-ticket count, a different number (see
		// EventSink.EpicComplete), not the epic-wide done count.
		run.state = RunStateCompleted
		run.pendingToasts = append(run.pendingToasts, notify.NotifyMsg{
			Kind:    notify.KindSuccess,
			Message: fmt.Sprintf("\U0001f389 epic %q complete (%s)", epicName, formatElapsed(event.ElapsedSeconds)),
		})
	case ralphloop.LiveEventDrainComplete:
		// Done/Total were already synced to the epic's true state by the last
		// LiveEventIterationFinished — event.Completed is this run's own
		// landed-ticket count, a different number (see EventSink.EpicComplete),
		// not the epic-wide done count.
		//
		// run.state = RunStateCompleted duplicates the LiveEventEpicComplete
		// case above rather than falling through to it, because a drained run
		// never gets a LiveEventEpicComplete at all — Run's drain-exit path
		// skips that call outright (see EventSink.EpicComplete's
		// notification-only doc comment). If EpicComplete's reducer case above
		// ever grows behavior beyond the state set + toast, this case needs
		// the same addition.
		run.state = RunStateCompleted
		run.pendingToasts = append(run.pendingToasts, notify.NotifyMsg{
			Kind:    notify.KindWarning,
			Message: fmt.Sprintf("\U0001f6d1 epic %q drained: %d ticket(s) landed (%s)", epicName, event.Completed, formatElapsed(event.ElapsedSeconds)),
		})
	}
}

func (r *loopRegistry) runSnapshot(epicName string) (RunSnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil {
		return RunSnapshot{}, false
	}
	return copyRunSnapshot(epicName, run, r.paused), true
}

// drainPendingNotifyCloses hands back and clears epicName's queued
// reattach-scan notification ids (see epicRun.pendingNotifyCloses) so the
// caller can turn them into notify.Close cmds exactly once.
func (r *loopRegistry) drainPendingNotifyCloses(epicName string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil || len(run.pendingNotifyCloses) == 0 {
		return nil
	}
	ids := run.pendingNotifyCloses
	run.pendingNotifyCloses = nil
	return ids
}

// drainPendingToasts hands back and clears epicName's queued toasts (see
// epicRun.pendingToasts) so the caller can turn them into notify cmds
// exactly once.
func (r *loopRegistry) drainPendingToasts(epicName string) []notify.NotifyMsg {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil || len(run.pendingToasts) == 0 {
		return nil
	}
	toasts := run.pendingToasts
	run.pendingToasts = nil
	return toasts
}

// drainPendingReload hands back and clears epicName's queued reload flag
// (see epicRun.pendingReload) so the caller re-reads the epic's tickets from
// disk exactly once.
func (r *loopRegistry) drainPendingReload(epicName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil || !run.pendingReload {
		return false
	}
	run.pendingReload = false
	return true
}

func (r *loopRegistry) runSnapshots() []RunSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.snapshots))
	for name := range r.snapshots {
		names = append(names, name)
	}
	sort.Strings(names)
	snapshots := make([]RunSnapshot, len(names))
	for i, name := range names {
		snapshots[i] = copyRunSnapshot(name, r.snapshots[name], r.paused)
	}
	return snapshots
}

func copyRunSnapshot(epicName string, run *epicRun, queuePaused bool) RunSnapshot {
	tickets := make(map[string]RunTicketSnapshot, len(run.tickets))
	contextTokens := 0
	paused := queuePaused && run.state == RunStateRunning
	for identifier, ticket := range run.tickets {
		tickets[identifier] = ticket
		contextTokens += ticket.ContextTokens
		paused = paused || ticket.Paused
	}
	return RunSnapshot{
		EpicName:       epicName,
		State:          run.state,
		Done:           run.done,
		Total:          run.total,
		ContextTokens:  contextTokens,
		Paused:         paused,
		FinalError:     run.finalError,
		Tickets:        tickets,
		StartedAt:      run.startedAt,
		StalledTickets: run.parkedStalled,
	}
}

// parkedEpics reports every epic whose run is currently parked, mapped to its
// stalled tickets. The Queue tab derives its parked rendering and its
// execution-complete predicate from this rather than mirroring park/unpark
// events into a map of its own, so the run state stays the single source of
// truth (ticket 13c).
func (r *loopRegistry) parkedEpics() map[string][]ralphloop.StalledTicket {
	r.mu.Lock()
	defer r.mu.Unlock()
	parked := map[string][]ralphloop.StalledTicket{}
	for name, run := range r.snapshots {
		if run.state == RunStateParked {
			parked[name] = run.parkedStalled
		}
	}
	return parked
}

// parkedStalledFor is parkedEpics for a single epic: ok reports whether
// epicName's run is parked at all, separately from whether it has any stalled
// tickets to name (a park event can carry none).
func (r *loopRegistry) parkedStalledFor(epicName string) (stalled []ralphloop.StalledTicket, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil || run.state != RunStateParked {
		return nil, false
	}
	return run.parkedStalled, true
}

// resumeParked cosmetically wakes epicName's parked run (see
// ralphloop.Gate.WakeParked) so its next poll pass rechecks the frontier
// immediately instead of waiting out the current park interval. A no-op if
// epicName isn't currently parked.
func (r *loopRegistry) resumeParked(epicName string) {
	r.mu.Lock()
	run := r.runs[epicName]
	parked := run != nil && run.state == RunStateParked
	r.mu.Unlock()
	if !parked {
		return
	}
	run.gate.WakeParked()
}

func (r *loopRegistry) gateFor(epicName string) *ralphloop.Gate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.runs[epicName]; run != nil {
		return run.gate
	}
	return nil
}

// drain stops epicName's live run from admitting new claims (via its Gate's
// Drain, see ralphloop/pause.go) without interrupting any iteration already
// in flight; ok reports whether a run was found to drain.
func (r *loopRegistry) drain(epicName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[epicName]
	if run == nil {
		return false
	}
	run.gate.Drain()
	return true
}

// setAgent records which agent is driving epicName's live run, called back
// right after tryStart succeeds in cmdStartImplement — see epicRun.agent's
// doc comment for why this is captured rather than re-derived later.
func (r *loopRegistry) setAgent(epicName string, agent ralphloop.AgentKind) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.runs[epicName]; run != nil {
		run.agent = agent
	}
}

// agentFor returns the agent driving epicName's live run. ok is false once
// the epic is no longer running (finish deletes it from r.runs) — capture the
// value while the run is still confirmed live rather than re-fetching later.
func (r *loopRegistry) agentFor(epicName string) (ralphloop.AgentKind, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[epicName]
	if run == nil {
		return "", false
	}
	return run.agent, true
}

// setScope records epicName's resolved RunScope, called back from
// RunOptions.OnScopeResolved once Run has loaded the epic and resolved it.
func (r *loopRegistry) setScope(epicName string, scope ralphloop.RunScope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.runs[epicName]; run != nil {
		run.scope = scope
	}
}

// setFailureNotifier records the reporter finish will call, after the
// drain, if the run ends in error. Called once by cmdStartImplement right
// after tryStart, mirroring setScope.
func (r *loopRegistry) setFailureNotifier(epicName string, notifier ralphloop.EpicFailureNotifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.runs[epicName]; run != nil {
		run.failureNotifier = notifier
	}
}

// scopeFor returns epicName's live RunScope, for "add to queue" (ticket 10)
// to widen via RunScope.Add. ok is false once the epic is no longer running.
func (r *loopRegistry) scopeFor(epicName string) (ralphloop.RunScope, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[epicName]
	if run == nil {
		return ralphloop.RunScope{}, false
	}
	return run.scope, true
}

func (r *loopRegistry) pause() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = true
	for _, run := range r.runs {
		run.gate.Pause(ralphloop.QueuePauseLabel, "queue paused")
	}
}

func (r *loopRegistry) resume() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = false
	for _, run := range r.runs {
		run.gate.ForceResume(ralphloop.QueuePauseLabel)
	}
}

func (r *loopRegistry) isPaused() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paused
}

// holdsAttach reports whether this process currently holds the per-repo
// attach lock (ticket 05), for SelfAttached's tab-label signal.
func (r *loopRegistry) holdsAttach() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attachCount > 0
}

// SelfAttached reports whether this process holds the per-repo attach lock —
// the Queue tab label's "(attached)" suffix (ticket 07) is self-only and
// never reflects a foreign process's attachment (surfaced separately via
// ForeignAttachPID and ticket 05's hard-block error).
func SelfAttached() bool {
	return ralphLoopRegistry.holdsAttach()
}

// ForeignAttachPID reports the pid of a live foreign process currently
// holding scratchDir's attach lock, or 0 when unattached, the lock is stale,
// or this process itself holds it. It shells out to `ps` (via
// attachLockIsStale) so callers should poll it on a refresh cadence rather
// than call it on every render.
func ForeignAttachPID(scratchDir string) int {
	if ralphLoopRegistry.holdsAttach() {
		return 0
	}
	info, err := readAttachLock(attachLockPath(scratchDir))
	if err != nil {
		return 0
	}
	if attachLockIsStale(info) {
		return 0
	}
	return info.PID
}

func (r *loopRegistry) finish(epicName string, err error) {
	r.mu.Lock()
	run := r.runs[epicName]
	r.mu.Unlock()
	if run != nil && run.sink != nil {
		run.sink.Close()
		<-run.eventDrainDone
	}
	if run != nil && err != nil && run.failureNotifier != nil {
		// Fired after the drain completes, from a reporter that was
		// constructed independently of the sink just closed above — the
		// one way to still reach chat once the run has already returned
		// (see ralphloop.EventSink.EpicFailed's doc comment).
		run.failureNotifier.EpicFailed(epicName, err)
	}

	r.mu.Lock()
	stopAggregator := false
	if run != nil {
		run.state = RunStateCompleted
		if err != nil {
			run.state = RunStateFailed
			run.finalError = err.Error()
		}
		run.sink = nil
		if run.permitReserved {
			run.permitReserved = false
			r.activeCount--
			r.permitCond.Broadcast()
		}
		if run.holdsAttach {
			r.attachCount--
			if r.attachCount <= 0 {
				releaseAttachLock(r.attachScratchDir)
				r.attachCount = 0
				r.attachScratchDir = ""
				stopAggregator = true
			}
		}
	}
	delete(r.runs, epicName)
	if err != nil {
		r.lastErr[epicName] = err
	} else {
		delete(r.lastErr, epicName)
	}
	r.mu.Unlock()
	// stopCostAggregator blocks on the poller goroutine exiting, which itself
	// needs r.mu (via runningEpicNames/costSnapshot) to complete an in-flight
	// tick — must run after r.mu is released above, not under defer, or the
	// two deadlock.
	if stopAggregator {
		stopCostAggregator()
	}
}

func (r *loopRegistry) lastError(epicName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr[epicName]
}

// takeAttachError returns and clears the reason the most recent tryStart was
// rejected by the attach lock, or nil if that rejection had another cause.
func (r *loopRegistry) takeAttachError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.attachErr
	r.attachErr = nil
	return err
}

// CanQuit implements the app shell's quit-guard duck type (see
// ui/app/model_quit.go): it blocks quitting gx while this process has any
// epic's ralph-loop in flight, since an interrupted loop can leave the
// worktree mid-cherry-pick. This is warn-then-allow, not a hard block —
// reconcile.go recovers an interrupted loop on the next run, so there's no
// correctness reason to prevent quitting outright.
func (m Model) CanQuit() bool {
	return !ralphLoopRegistry.isRunning()
}

// isRunning reports whether any epic currently has a run in flight.
func (r *loopRegistry) isRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs) > 0
}

func (r *loopRegistry) availableSlots() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paused {
		return 0
	}
	return max(r.maxConcurrent-r.activeCount, 0)
}

// isRunningEpic reports whether epicName has a run in flight, so a poll loop
// knows to keep going without accessing the run's stream directly.
func (r *loopRegistry) isRunningEpic(epicName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[epicName]
	return ok
}

// epicCostSnapshot is one running epic's state as the live cost aggregator
// needs it: enough to load the epic's on-disk landed costs and locate each
// running ticket's live session, without the aggregator holding r.mu across
// its own (slow, disk-bound) work.
type epicCostSnapshot struct {
	EpicName   string
	ScratchDir string
	Tickets    map[string]RunTicketSnapshot
}

// costSnapshot returns a point-in-time copy of every running epic's name,
// scratchDir, and ticket map, gathered in one locked pass.
func (r *loopRegistry) costSnapshot() []epicCostSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.runs))
	for name := range r.runs {
		names = append(names, name)
	}
	sort.Strings(names)
	snapshots := make([]epicCostSnapshot, len(names))
	for i, name := range names {
		run := r.runs[name]
		tickets := make(map[string]RunTicketSnapshot, len(run.tickets))
		maps.Copy(tickets, run.tickets)
		snapshots[i] = epicCostSnapshot{EpicName: name, ScratchDir: run.scratchDir, Tickets: tickets}
	}
	return snapshots
}

// runningEpicNames reports every epic currently mid-run, sorted by name, so a
// reactivated tab can fan out over all of them instead of recovering just one
// (see OnPageActivated).
func (r *loopRegistry) runningEpicNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.runs))
	for name := range r.runs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
