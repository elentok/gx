// Package runs is the server's record of launched iterations, keyed by ticket
// address and persisted to a state-dir file so a restart can reclaim them.
package runs

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

const FileName = "runs.json"

// Run is one launched iteration in the server's run registry.
type Run struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
	// Runner names the agentrunner hosting Session, e.g. "herdr".
	Runner  string              `json:"runner"`
	Session agentrunner.Session `json:"session"`
	// Model and Effort are what the agent launched under; a conflict on its
	// merge inherits them. Empty when the launch named none.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// PauseEvent is the run's agent context on its pause event: the pause drops the
// runs.json entry, so late-finish adoption finds the runner from here.
func (r Run) PauseEvent() ralphloop.Event {
	return ralphloop.Event{Agent: ralphloop.AgentKind(r.Agent), Runner: r.Runner, Label: r.Session.Label, AgentSession: r.Session.SessionID, Model: r.Model, Effort: r.Effort}
}

// RunnerHerdr is the Runner of every launch until the server launches
// through agentrunner, and of any run an older server saved.
const RunnerHerdr = "herdr"

// Launching is the State of a run whose agent may not be started or prompted yet.
const Launching = "launching"

// savedRun reads runs.json, including files written before Run held a
// Session: those carried the herdr pane at the top level instead.
type savedRun struct {
	TrackedRun
	Pane string `json:"pane"`
}

// IterationLabel is the agent label of the iteration of the ticket at address.
func IterationLabel(address string) string {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return address
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	return label
}

// TrackedRun is a Run plus what a restarted server needs to finish it.
type TrackedRun struct {
	Run
	Root       string `json:"root"`
	Repo       string `json:"repo"`
	Workspace  string `json:"workspace"`
	Base       string `json:"base"`
	TicketPath string `json:"ticket_path"`
	// Cwd is the directory the agent started in. Empty for a run an older
	// server saved, which worktree removal then treats as inside its epic.
	Cwd string `json:"cwd,omitempty"`
	// StartedAt is when the claim launched; it survives a restart with the
	// registry file. Zero for a run an older server recorded.
	StartedAt time.Time `json:"started_at,omitzero"`
	// WaitingSince is when the agent asked a question and went idle, still
	// alive for the answer. Zero while it works.
	WaitingSince time.Time `json:"waiting_since,omitzero"`
	// SlotReleased marks a waiting run that outlived the grace period: it stays
	// alive but no longer counts toward the slot caps. Cleared by the answer.
	SlotReleased bool `json:"slot_released,omitempty"`
	// State is runLaunching from the claim until the agent has started and
	// been prompted, so a crash in between leaves a handle reclaim can see.
	State string `json:"state,omitempty"`
	// Watched marks a pane a nudge set working again after its run ended
	// (see watchNudgedPane): it is metered and budget-stopped like a run, but
	// never finished, and never saved, so a restart cannot reclaim it.
	Watched bool `json:"-"`
	// Settling is set once settleRun starts merging the run, while the ticket
	// still reads finished. Never saved: a restart reclaims and settles again.
	Settling bool `json:"-"`
	// RateLimited marks a run the wait loop holds for the agent's rate limit;
	// RateLimitReset is when it resets (zero when unknown). Never saved: a
	// restart re-derives the hold from the runner's rate-limit check.
	RateLimited    bool      `json:"-"`
	RateLimitReset time.Time `json:"-"`
}

// Registry is the server's own record of launched iterations, keyed by
// ticket address (a root may hold several). Independent of the TUI's.
// It lives in a state-dir file too, so a restart can reclaim what is running.
type Registry struct {
	mu   sync.Mutex
	path string
	log  *slog.Logger
	runs map[string]TrackedRun
	// released are the runs whose background-task gate recovery forced open.
	// In memory only: a reclaimed run starts with its gate closed again.
	released map[string]gateRelease
	// budgetStops are the runs the budget hard limit is stopping, true once
	// one path has paused it budget-killed. In memory only.
	budgetStops map[string]bool
}

// Open returns the registry and the handles the previous server left behind.
// The registry itself starts empty: only reclaimed handles go back into it.
func Open(stateDir string, log *slog.Logger) (*Registry, []TrackedRun, error) {
	r := &Registry{path: filepath.Join(stateDir, FileName), log: log, runs: map[string]TrackedRun{}}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var saved []savedRun
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, nil, err
	}
	out := make([]TrackedRun, 0, len(saved))
	for _, s := range saved {
		t := s.TrackedRun
		if t.Session.ID == "" && s.Pane != "" {
			t.Runner = RunnerHerdr
			t.Session = agentrunner.Session{Label: IterationLabel(t.Address), ID: s.Pane}
		}
		out = append(out, t)
	}
	return r, out, nil
}

// save writes through a rename so a crash never leaves a torn file. Callers hold mu.
func (r *Registry) save() {
	list := make([]TrackedRun, 0, len(r.runs))
	for _, t := range r.runs {
		if !t.Watched {
			list = append(list, t)
		}
	}
	data, err := json.Marshal(list)
	if err == nil {
		tmp := r.path + ".tmp"
		if err = os.WriteFile(tmp, data, 0o600); err == nil {
			err = os.Rename(tmp, r.path)
		}
	}
	if err != nil && r.log != nil {
		r.log.Warn("persist runs", "err", err)
	}
}

func (r *Registry) Has(address string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[address]
	return ok
}

// WaitGone polls has every poll until address leaves the registry. False means
// it was still there at timeout.
func (r *Registry) WaitGone(address string, timeout, poll time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for r.Has(address) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(poll)
	}
	return true
}

// CountWhere counts the runs that hold a slot and match keep.
func (r *Registry) CountWhere(keep func(TrackedRun) bool) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, t := range r.runs {
		if !t.SlotReleased && keep(t) {
			n++
		}
	}
	return n
}

// CountRoot is how many iterations of root's tickets hold a slot.
func (r *Registry) CountRoot(root string) int {
	return r.CountWhere(func(t TrackedRun) bool { return t.Root == root })
}

// CountProject is how many iterations of the project's tickets hold a slot.
func (r *Registry) CountProject(project string) int {
	return r.CountWhere(func(t TrackedRun) bool { return strings.HasPrefix(t.Root, project+":") })
}

// RunOf returns the run whose iteration is of the ticket at address.
func (r *Registry) RunOf(address string) (Run, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.runs {
		if t.Address == address {
			return t.Run, true
		}
	}
	return Run{}, false
}

// StartedAtOf returns when the iteration of the ticket at address was launched.
func (r *Registry) StartedAtOf(address string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.runs {
		if t.Address == address && !t.StartedAt.IsZero() {
			return t.StartedAt, true
		}
	}
	return time.Time{}, false
}

func (r *Registry) List() []TrackedRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]TrackedRun, 0, len(r.runs))
	for _, t := range r.runs {
		out = append(out, t)
	}
	return out
}

// Count is how many runs hold a slot toward the global cap.
func (r *Registry) Count() int {
	return r.CountWhere(func(TrackedRun) bool { return true })
}

// WaitState is where the run at address stands in its wait for an answer.
type WaitState struct {
	Waiting  bool // the run is alive and waiting
	Released bool // it gave its slot up
	// CapHeld: the grace period is over but the release cap kept the slot.
	CapHeld bool
}

// WaitStateOf reports the wait of the run at address, judged at now against
// grace and the release cap max.
func (r *Registry) WaitStateOf(address string, now time.Time, grace time.Duration, max int) WaitState {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.runs[address]
	if !ok || t.WaitingSince.IsZero() {
		return WaitState{}
	}
	if t.SlotReleased {
		return WaitState{Waiting: true, Released: true}
	}
	if now.Sub(t.WaitingSince) < grace {
		return WaitState{Waiting: true}
	}
	released := 0
	for _, o := range r.runs {
		if o.SlotReleased {
			released++
		}
	}
	return WaitState{Waiting: true, CapHeld: released >= max}
}

// ReleaseWaiting releases the slot of each run that has waited at least grace,
// oldest wait first, while fewer than max runs are released. It reports how
// many it released.
func (r *Registry) ReleaseWaiting(now time.Time, grace time.Duration, max int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	released := 0
	var due []TrackedRun
	for _, t := range r.runs {
		switch {
		case t.SlotReleased:
			released++
		case !t.WaitingSince.IsZero() && now.Sub(t.WaitingSince) >= grace:
			due = append(due, t)
		}
	}
	slices.SortFunc(due, func(a, b TrackedRun) int { return a.WaitingSince.Compare(b.WaitingSince) })
	n := 0
	for _, t := range due {
		if released+n >= max {
			break
		}
		t.SlotReleased = true
		r.runs[t.Address] = t
		n++
	}
	if n > 0 {
		r.save()
	}
	return n
}

func (r *Registry) Tracked(address string) (TrackedRun, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.runs[address]
	return t, ok
}

// gateRelease is what recovery left on a run whose gate it forced open.
type gateRelease struct {
	// pause is the reason the run's finish pauses with instead of merging; ""
	// lets the finish run.
	pause string
}

func (r *Registry) ReleaseGate(address, pause string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released == nil {
		r.released = map[string]gateRelease{}
	}
	r.released[address] = gateRelease{pause: pause}
}

// GatePause is the pause reason release-gate left for the run at address.
func (r *Registry) GatePause(address string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.released[address].pause
}

// GateReleased is the Deps.GateReleased hook of the run at address.
func (r *Registry) GateReleased(address string) func() bool {
	return func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		_, ok := r.released[address]
		return ok
	}
}

// MarkBudgetStop records that the budget hard limit is stopping the run at
// address, before its agent is interrupted.
func (r *Registry) MarkBudgetStop(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.budgetStops == nil {
		r.budgetStops = map[string]bool{}
	}
	if _, ok := r.budgetStops[address]; !ok {
		r.budgetStops[address] = false
	}
}

// ClaimBudgetStop reports whether the run at address is being budget-stopped,
// and whether this caller is the first to claim it, so the run is paused
// budget-killed exactly once: by the kill or by the run's own settle.
func (r *Registry) ClaimBudgetStop(address string) (stopping, first bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paused, ok := r.budgetStops[address]
	if !ok {
		return false, false
	}
	r.budgetStops[address] = true
	return true, !paused
}

// DeleteWatched drops the watched pane t, unless a run has since replaced it
// (e.g. a relaunch claimed the ticket again).
func (r *Registry) DeleteWatched(t TrackedRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.runs[t.Address]; ok && cur.Watched && cur.Session.ID == t.Session.ID {
		r.deleteLocked(t.Address)
	}
}

func (r *Registry) Delete(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteLocked(address)
}

func (r *Registry) deleteLocked(address string) {
	delete(r.runs, address)
	delete(r.released, address)
	delete(r.budgetStops, address)
	r.save()
}

// MarkWaiting flags the run at address as waiting for an answer since at.
func (r *Registry) MarkWaiting(address string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.runs[address]; ok {
		t.WaitingSince = at
		r.runs[address] = t
		r.save()
	}
}

// MarkRateLimit sets or clears the rate-limit hold of the run at address.
func (r *Registry) MarkRateLimit(address string, limited bool, resetAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.runs[address]; ok {
		t.RateLimited, t.RateLimitReset = limited, resetAt
		r.runs[address] = t
	}
}

// RateLimitedRuns are the held runs, ordered by address.
func (r *Registry) RateLimitedRuns() []TrackedRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	var held []TrackedRun
	for _, t := range r.runs {
		if t.RateLimited {
			held = append(held, t)
		}
	}
	slices.SortFunc(held, func(a, b TrackedRun) int { return strings.Compare(a.Address, b.Address) })
	return held
}

// RateLimitHeld reports the held run of agent that resets last (any run when
// none has a known reset), whose limit the whole harness shares.
func (r *Registry) RateLimitHeld(agent string) (TrackedRun, bool) {
	var best TrackedRun
	found := false
	for _, t := range r.RateLimitedRuns() {
		if t.Agent == agent && (!found || t.RateLimitReset.After(best.RateLimitReset)) {
			best, found = t, true
		}
	}
	return best, found
}

// MarkSettling flags the run at address as inside settleRun.
func (r *Registry) MarkSettling(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.runs[address]; ok {
		t.Settling = true
		r.runs[address] = t
	}
}

// ClearWaiting ends the run's wait for an answer. The run takes its slot back
// at once, even over the limit.
func (r *Registry) ClearWaiting(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.runs[address]; ok {
		t.WaitingSince = time.Time{}
		t.SlotReleased = false
		r.runs[address] = t
		r.save()
	}
}

// PutIfAbsent records t unless its ticket already has an entry, so two claims
// of one ticket cannot both launch.
func (r *Registry) PutIfAbsent(t TrackedRun) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[t.Address]; ok {
		return false
	}
	r.runs[t.Address] = t
	r.save()
	return true
}

// DeleteStarted drops the entry at address only if it is the one started at
// startedAt, so a failed launch never removes another run's entry.
func (r *Registry) DeleteStarted(address string, startedAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.runs[address]; ok && cur.StartedAt.Equal(startedAt) {
		r.deleteLocked(address)
	}
}

func (r *Registry) Put(t TrackedRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[t.Address] = t
	r.save()
}

// Runs lists the registered runs.
func (r *Registry) Runs() []Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Run, 0, len(r.runs))
	for _, t := range r.runs {
		out = append(out, t.Run)
	}
	return out
}

// PutRun records a launched iteration without launching one, so tests can hold
// a run live as long as they need.
func (r *Registry) PutRun(root string, run Run) { r.Put(TrackedRun{Run: run, Root: root}) }

// PutRunAt is PutRun with the time the iteration was launched.
func (r *Registry) PutRunAt(root string, run Run, at time.Time) {
	r.Put(TrackedRun{Run: run, Root: root, StartedAt: at})
}

// PutRunFrom is PutRun with the base the iteration branched from.
func (r *Registry) PutRunFrom(root string, run Run, base string) {
	r.Put(TrackedRun{Run: run, Root: root, Base: base})
}

// PutRunIn registers a run of address in root, started in cwd, on a session
// that does not exist (a stopped agent whose entry settleRun has not yet
// deleted). launching marks it as one whose agent may not have started.
func (r *Registry) PutRunIn(address, root, cwd, runner string, launching bool) {
	t := TrackedRun{Run: Run{Address: address, Runner: runner, Session: agentrunner.Session{Label: "gone", ID: "gone"}}, Root: root, Cwd: cwd}
	if launching {
		t.State = Launching
	}
	r.Put(t)
}

// IsGateReleased reports whether the run's background-task gate was released.
func (r *Registry) IsGateReleased(address string) bool { return r.GateReleased(address)() }
