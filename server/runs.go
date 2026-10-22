package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/elentok/gx/events"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// EventReclaimed streams when a restarted server takes over an iteration that
// was already running.
const EventReclaimed = "reclaimed"

const runsFileName = "runs.json"

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

// pauseEvent is the run's agent context on its pause event: the pause drops the
// runs.json entry, so late-finish adoption finds the runner from here.
func (r Run) pauseEvent() ralphloop.Event {
	return ralphloop.Event{Agent: ralphloop.AgentKind(r.Agent), Runner: r.Runner, Label: r.Session.Label, AgentSession: r.Session.SessionID, Model: r.Model, Effort: r.Effort}
}

// runnerHerdr is the Runner of every launch until the server launches
// through agentrunner, and of any run an older server saved.
const runnerHerdr = "herdr"

// runLaunching is the State of a run whose agent may not be started or prompted yet.
const runLaunching = "launching"

// savedRun reads runs.json, including files written before Run held a
// Session: those carried the herdr pane at the top level instead.
type savedRun struct {
	trackedRun
	Pane string `json:"pane"`
}

// iterationLabel is the agent label of the iteration of the ticket at address.
func iterationLabel(address string) string {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return address
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	return label
}

// trackedRun is a Run plus what a restarted server needs to finish it.
type trackedRun struct {
	Run
	Root       string `json:"root"`
	Repo       string `json:"repo"`
	Workspace  string `json:"workspace"`
	Base       string `json:"base"`
	TicketPath string `json:"ticket_path"`
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
}

// runRegistry is the server's own record of launched iterations, keyed by
// ticket address (a root may hold several). Independent of the TUI's.
// It lives in a state-dir file too, so a restart can reclaim what is running.
type runRegistry struct {
	mu   sync.Mutex
	path string
	log  *slog.Logger
	runs map[string]trackedRun
	// released are the runs whose background-task gate recovery forced open.
	// In memory only: a reclaimed run starts with its gate closed again.
	released map[string]gateRelease
	// budgetStops are the runs the budget hard limit is stopping, true once
	// one path has paused it budget-killed. In memory only.
	budgetStops map[string]bool
}

// openRuns returns the registry and the handles the previous server left behind.
// The registry itself starts empty: only reclaimed handles go back into it.
func openRuns(stateDir string, log *slog.Logger) (*runRegistry, []trackedRun, error) {
	r := &runRegistry{path: filepath.Join(stateDir, runsFileName), log: log, runs: map[string]trackedRun{}}
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
	out := make([]trackedRun, 0, len(saved))
	for _, s := range saved {
		t := s.trackedRun
		if t.Session.ID == "" && s.Pane != "" {
			t.Runner = runnerHerdr
			t.Session = agentrunner.Session{Label: iterationLabel(t.Address), ID: s.Pane}
		}
		out = append(out, t)
	}
	return r, out, nil
}

// save writes through a rename so a crash never leaves a torn file. Callers hold mu.
func (r *runRegistry) save() {
	list := make([]trackedRun, 0, len(r.runs))
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

func (r *runRegistry) has(address string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[address]
	return ok
}

// countWhere counts the runs that hold a slot and match keep.
func (r *runRegistry) countWhere(keep func(trackedRun) bool) int {
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

// countRoot is how many iterations of root's tickets hold a slot.
func (r *runRegistry) countRoot(root string) int {
	return r.countWhere(func(t trackedRun) bool { return t.Root == root })
}

// countProject is how many iterations of the project's tickets hold a slot.
func (r *runRegistry) countProject(project string) int {
	return r.countWhere(func(t trackedRun) bool { return strings.HasPrefix(t.Root, project+":") })
}

// runOf returns the run whose iteration is of the ticket at address.
func (r *runRegistry) runOf(address string) (Run, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.runs {
		if t.Address == address {
			return t.Run, true
		}
	}
	return Run{}, false
}

// startedAtOf returns when the iteration of the ticket at address was launched.
func (r *runRegistry) startedAtOf(address string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.runs {
		if t.Address == address && !t.StartedAt.IsZero() {
			return t.StartedAt, true
		}
	}
	return time.Time{}, false
}

func (r *runRegistry) list() []trackedRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]trackedRun, 0, len(r.runs))
	for _, t := range r.runs {
		out = append(out, t)
	}
	return out
}

// count is how many runs hold a slot toward the global cap.
func (r *runRegistry) count() int {
	return r.countWhere(func(trackedRun) bool { return true })
}

// waitState is where the run at address stands in its wait for an answer.
type waitState struct {
	Waiting  bool // the run is alive and waiting
	Released bool // it gave its slot up
	// CapHeld: the grace period is over but the release cap kept the slot.
	CapHeld bool
}

// waitStateOf reports the wait of the run at address, judged at now against
// grace and the release cap max.
func (r *runRegistry) waitStateOf(address string, now time.Time, grace time.Duration, max int) waitState {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.runs[address]
	if !ok || t.WaitingSince.IsZero() {
		return waitState{}
	}
	if t.SlotReleased {
		return waitState{Waiting: true, Released: true}
	}
	if now.Sub(t.WaitingSince) < grace {
		return waitState{Waiting: true}
	}
	released := 0
	for _, o := range r.runs {
		if o.SlotReleased {
			released++
		}
	}
	return waitState{Waiting: true, CapHeld: released >= max}
}

// releaseWaiting releases the slot of each run that has waited at least grace,
// oldest wait first, while fewer than max runs are released. It reports how
// many it released.
func (r *runRegistry) releaseWaiting(now time.Time, grace time.Duration, max int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	released := 0
	var due []trackedRun
	for _, t := range r.runs {
		switch {
		case t.SlotReleased:
			released++
		case !t.WaitingSince.IsZero() && now.Sub(t.WaitingSince) >= grace:
			due = append(due, t)
		}
	}
	slices.SortFunc(due, func(a, b trackedRun) int { return a.WaitingSince.Compare(b.WaitingSince) })
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

func (r *runRegistry) tracked(address string) (trackedRun, bool) {
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

func (r *runRegistry) releaseGate(address, pause string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released == nil {
		r.released = map[string]gateRelease{}
	}
	r.released[address] = gateRelease{pause: pause}
}

// gatePause is the pause reason release-gate left for the run at address.
func (r *runRegistry) gatePause(address string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.released[address].pause
}

// gateReleased is the Deps.GateReleased hook of the run at address.
func (r *runRegistry) gateReleased(address string) func() bool {
	return func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		_, ok := r.released[address]
		return ok
	}
}

// markBudgetStop records that the budget hard limit is stopping the run at
// address, before its agent is interrupted.
func (r *runRegistry) markBudgetStop(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.budgetStops == nil {
		r.budgetStops = map[string]bool{}
	}
	if _, ok := r.budgetStops[address]; !ok {
		r.budgetStops[address] = false
	}
}

// claimBudgetStop reports whether the run at address is being budget-stopped,
// and whether this caller is the first to claim it, so the run is paused
// budget-killed exactly once: by the kill or by the run's own settle.
func (r *runRegistry) claimBudgetStop(address string) (stopping, first bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paused, ok := r.budgetStops[address]
	if !ok {
		return false, false
	}
	r.budgetStops[address] = true
	return true, !paused
}

// deleteWatched drops the watched pane t, unless a run has since replaced it
// (e.g. a relaunch claimed the ticket again).
func (r *runRegistry) deleteWatched(t trackedRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.runs[t.Address]; ok && cur.Watched && cur.Session.ID == t.Session.ID {
		r.deleteLocked(t.Address)
	}
}

func (r *runRegistry) delete(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteLocked(address)
}

func (r *runRegistry) deleteLocked(address string) {
	delete(r.runs, address)
	delete(r.released, address)
	delete(r.budgetStops, address)
	r.save()
}

// markWaiting flags the run at address as waiting for an answer since at.
func (r *runRegistry) markWaiting(address string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.runs[address]; ok {
		t.WaitingSince = at
		r.runs[address] = t
		r.save()
	}
}

// clearWaiting ends the run's wait for an answer. The run takes its slot back
// at once, even over the limit.
func (r *runRegistry) clearWaiting(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.runs[address]; ok {
		t.WaitingSince = time.Time{}
		t.SlotReleased = false
		r.runs[address] = t
		r.save()
	}
}

func (r *runRegistry) put(t trackedRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[t.Address] = t
	r.save()
}

// Runs lists the registry, for tests and later reads.
func (s *Server) Runs() []Run {
	s.registry.mu.Lock()
	defer s.registry.mu.Unlock()
	out := make([]Run, 0, len(s.registry.runs))
	for _, t := range s.registry.runs {
		out = append(out, t.Run)
	}
	return out
}

// reclaimRuns takes over every persisted iteration that is still alive: its
// agent is known to its runner and its worktree exists. It asks nothing, so a
// restart is invisible to running work. A dead agent that left nothing behind
// is reopened; any other handle that no longer matches is paused and left out
// of the registry.
func (s *Server) reclaimRuns() {
	for _, t := range s.savedRuns {
		err := s.reclaim(t)
		if err == nil {
			continue
		}
		s.log.Info("not reclaiming iteration", "ticket", t.Address, "reason", err)
		s.pauseUnreclaimed(t, err)
	}
	s.savedRuns = nil
}

// logFlushingRunners logs one line per runner other than the configured one
// that still hosts live runs; those runs flush on it while new launches don't.
func (s *Server) logFlushingRunners() {
	counts := map[string]int{}
	for _, t := range s.registry.list() {
		name := t.Runner
		if name == "" {
			name = runnerHerdr
		}
		if name != s.cfg.Runner.Name() {
			counts[name]++
		}
	}
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		s.log.Info(fmt.Sprintf("%d runs still on %s", counts[name], name), "runner", name, "runs", counts[name])
	}
}

// errHandleMismatch marks a persisted handle whose pane, agent or worktree is gone.
var errHandleMismatch = errors.New("handle mismatch")

// pauseUnreclaimed pauses the ticket of a handle that cannot be reclaimed, so a
// person sees it instead of it staying claimed forever.
func (s *Server) pauseUnreclaimed(t trackedRun, cause error) {
	kind := events.IterationError
	if errors.Is(cause, errHandleMismatch) {
		kind = events.HandleMismatch
	}
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err == nil {
		var dir string
		if dir, _, err = s.projectOf(addr.Project); err == nil {
			err = s.pauseTicket(dir, addr, t.TicketPath, kind, "cannot reclaim iteration after restart: "+cause.Error(), t.Run)
		}
	}
	if err != nil {
		s.log.Warn("pause unreclaimed iteration", "ticket", t.Address, "err", err)
	}
}

// reclaimHeadless takes over a headless iteration: a live agent keeps running
// on the adopted session, a finished one merges as if it had just finished, and
// one that died with the server down is reopened when it left nothing behind,
// else pauses stuck with its dir kept. A live agent that was waiting for
// an answer stays waiting and unsettled.
func (s *Server) reclaimHeadless(t trackedRun, runner agentrunner.Runner) error {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	re, ok := runner.(reattacher)
	if !ok {
		return fmt.Errorf("%w: runner %s cannot reattach", errHandleMismatch, t.Runner)
	}
	session, verdict, err := re.Reattach(iterationLabel(t.Address))
	if err != nil {
		return fmt.Errorf("%w: reattach %s: %v", errHandleMismatch, t.Address, err)
	}
	if verdict == nativerunner.VerdictFinished && t.State == runLaunching {
		// Never settle it: the agent may have finished without ever being prompted.
		return s.stopLaunching(t, runner, session, "finished")
	}
	if verdict == nativerunner.VerdictDied {
		if reopened, err := s.reopenEmptyClaim(t); err != nil || reopened {
			return err
		}
		dir, _, err := s.projectOf(addr.Project)
		if err != nil {
			return err
		}
		return s.pauseTicket(dir, addr, t.TicketPath, events.IterationError, nativerunner.ReasonAgentDied, t.Run)
	}
	one, wt, mode, err := s.resume(t)
	if err != nil {
		return err
	}
	root, err := parseRootRef(t.Root)
	if err != nil {
		return err
	}
	if verdict == nativerunner.VerdictLive {
		t.Session = session
	}
	t.State = ""
	s.registry.put(t)
	s.events.publish(EventReclaimed, t.Address)
	deps := s.depsWith(runner)
	deps.GateReleased = s.registry.gateReleased(t.Address)
	if verdict == nativerunner.VerdictFinished {
		s.goSettle(func() { s.settleRun(runContext{deps, root, mode, one, wt, t.Run, t.Address}, nil) })
		return nil
	}
	if !t.WaitingSince.IsZero() {
		// Idle on purpose, waiting for an answer: finishing it now would merge
		// or pause it for going quiet. The budget hard limit still reaches it
		// through the registry.
		return nil
	}
	s.goSettle(func() { s.finishRun(runContext{deps, root, mode, one, wt, t.Run, t.Address}) })
	return nil
}

// reclaim goes through the runner that started t, whatever runner is
// configured now: a herdr run under a headless server is still herdr's. An
// unavailable runner can't say whether the agent is gone, so only an agent
// the runner reports gone is ever reopened.
func (s *Server) reclaim(t trackedRun) error {
	name := t.Runner
	if name == "" {
		name = runnerHerdr
	}
	runner := s.runnerForRun(t.Run)
	if runner == nil {
		return fmt.Errorf("%w: started by %s; %s unavailable", errHandleMismatch, name, name)
	}
	if err := agentrunner.Healthy(runner); err != nil {
		return fmt.Errorf("%w: started by %s; %s unavailable: %v", errHandleMismatch, name, name, err)
	}
	if name != runnerHerdr {
		return s.reclaimHeadless(t, runner)
	}
	label := iterationLabel(t.Address)
	session, found, err := runner.Find(label)
	if err != nil {
		// A lookup that failed says nothing about whether the agent is gone.
		return fmt.Errorf("%w: agent %s: %v", errHandleMismatch, t.Address, err)
	}
	if !found || session.ID == "" {
		if reopened, err := s.reopenEmptyClaim(t); err != nil || reopened {
			return err
		}
		return fmt.Errorf("%w: no pane", errHandleMismatch)
	}
	if t.State == runLaunching {
		st, err := runner.Status(session)
		if err != nil {
			return fmt.Errorf("%w: agent %s: %v", errHandleMismatch, t.Address, err)
		}
		if st.Waiting() {
			return s.stopLaunching(t, runner, session, string(st.State))
		}
	}
	one, wt, mode, err := s.resume(t)
	if err != nil {
		return err
	}
	root, err := parseRootRef(t.Root)
	if err != nil {
		return err
	}
	t.Runner = runnerHerdr
	t.Session = agentrunner.Session{Label: label, ID: session.ID, SessionID: t.Session.SessionID}
	t.State = ""
	s.registry.put(t)
	s.events.publish(EventReclaimed, t.Address)
	deps := s.depsWith(runner)
	deps.GateReleased = s.registry.gateReleased(t.Address)
	s.goSettle(func() { s.finishRun(runContext{deps, root, mode, one, wt, t.Run, t.Address}) })
	return nil
}

// stopLaunching closes the agent of a launch the server died in, which is
// waiting rather than working, so it may never have been prompted; then the
// claim reopens if it is empty.
func (s *Server) stopLaunching(t trackedRun, runner agentrunner.Runner, session agentrunner.Session, status string) error {
	if err := runner.Stop(session); err != nil {
		return fmt.Errorf("stop launching agent: %w", err)
	}
	if reopened, err := s.reopenEmptyClaim(t); err != nil || reopened {
		return err
	}
	return fmt.Errorf("%w: launching agent %s", errHandleMismatch, status)
}

// resume rebuilds the iteration a persisted handle stands for, whose worktree
// must still exist.
func (s *Server) resume(t trackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, iterationMode, error) {
	one, wt, mode, err := s.resolveRun(t)
	if err != nil {
		return one, wt, mode, err
	}
	if _, err := os.Stat(wt.Path); err != nil {
		return one, wt, mode, fmt.Errorf("%w: worktree: %v", errHandleMismatch, err)
	}
	return one, wt, mode, nil
}

// reopenEmptyClaim reopens the ticket of a gone agent that left nothing behind
// (no commits past base, no edits), clearing its worktree and branch. It
// reports false, touching nothing, when the iteration holds or may hold work.
func (s *Server) reopenEmptyClaim(t trackedRun) (bool, error) {
	one, wt, mode, err := s.resolveRun(t)
	if err != nil {
		return false, err
	}
	empty, err := iterationEmpty(mode, wt, t.Repo, t.Base)
	if err != nil || !empty {
		return false, err
	}
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return false, err
	}
	dir, _, err := s.projectOf(addr.Project)
	if err != nil {
		return false, err
	}
	if err := mode.discard(ralphloop.DefaultDeps(), one, wt); err != nil {
		return false, err
	}
	const reason = "agent gone, no commits, no edits"
	if err := tickets.Reset(t.TicketPath, time.Now(), "reopened automatically after restart: "+reason); err != nil {
		return false, err
	}
	if err := ralphloop.AppendEvent(dir, addr.Epic, ralphloop.Event{
		Type: string(events.ClaimReopened), Ticket: addr.ID, Address: t.Address, Reason: reason,
	}); err != nil {
		s.log.Warn("log claim-reopened", "ticket", t.Address, "err", err)
	}
	s.log.Info("reopened empty claim", "ticket", t.Address)
	s.events.publish(EventTicketChanged, t.Address)
	return true, nil
}

// iterationEmpty reports whether an iteration provably holds no work. A
// directory the ticket doesn't own, or a detached worktree's commits, can't be
// measured, so those count as holding work unless the worktree is gone.
func iterationEmpty(mode iterationMode, wt ralphloop.IterationWorktree, repo, base string) (bool, error) {
	if mode.feature || mode.scratchDir != "" {
		return false, nil
	}
	_, statErr := os.Stat(wt.Path)
	if statErr == nil {
		changes, err := git.UncommittedChanges(wt.Path)
		if err != nil || len(changes) > 0 {
			return false, err
		}
	}
	if wt.Branch == "" {
		return statErr != nil, nil
	}
	if _, err := git.RevParse(repo, wt.Branch); errors.Is(err, git.ErrRefNotFound) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	if base == "" {
		return false, nil
	}
	commits, err := git.CommitsBetween(git.Repo{Root: repo}, base, wt.Branch)
	return len(commits) == 0, err
}

// resolveRun rebuilds the iteration a persisted handle stands for, whether or
// not its worktree is still on disk.
func (s *Server) resolveRun(t trackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, iterationMode, error) {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, iterationMode{}, err
	}
	dir, _, err := s.projectOf(addr.Project)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, iterationMode{}, err
	}
	epics, err := tickets.LoadResolved(addr.Project, dir)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, iterationMode{}, err
	}
	one := ralphloop.OneIteration{
		RepoDir: t.Repo, WorkspaceID: t.Workspace, Epic: addr.Epic, ScratchDir: dir, Agent: ralphloop.AgentKind(t.Agent),
		StallNudgeDelay: s.stalledNudgeDelay(),
	}
	for _, e := range epics {
		for _, tk := range e.Tickets {
			if tk.Path == t.TicketPath {
				one.Ticket = tk
			}
		}
	}
	if one.Ticket.Path == "" {
		return one, ralphloop.IterationWorktree{}, iterationMode{}, errors.New("ticket gone")
	}
	one.Sink = s.finishSink(addr, one.Ticket.Path)
	mode := s.iterationModeFor(addr, one.Ticket)
	wt, err := mode.resume(ralphloop.DefaultDeps(), one, t.Base)
	return one, wt, mode, err
}
