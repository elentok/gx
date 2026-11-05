package sched

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketlog"
)

// ReclaimHost is what the reclaim needs from the server.
type ReclaimHost interface {
	// ConfiguredRunner is the name of the runner new launches use.
	ConfiguredRunner() string
	// RunnerFor is the runner that started run, or nil when unavailable.
	RunnerFor(run runs.Run) agentrunner.Runner
	// ProjectDir is the scratch directory of a project.
	ProjectDir(project string) (string, error)
	Pause(dir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string, run runs.Run) error
	// StalledNudgeDelay is how long an idle agent with no report waits for its nudge.
	StalledNudgeDelay() time.Duration
	FinishSink(addr tickets.Address, ticketPath string) ralphloop.EventSink
	IterationModeFor(addr tickets.Address, t tickets.Ticket) merge.IterationMode
	DefaultDeps() ralphloop.Deps
	Publish(event, id string)
	// Continue hands a reclaimed run to the settle machinery: settle now when
	// settleNow, else wait for the agent to finish first.
	Continue(runner agentrunner.Runner, t runs.TrackedRun, root queue.RootRef, mode merge.IterationMode, one ralphloop.OneIteration, wt ralphloop.IterationWorktree, settleNow bool)
	// ReportedFinished is whether the ticket file says its agent finished.
	ReportedFinished(ticketPath string) bool
}

// Reclaimer takes over the runs a previous server left.
type Reclaimer struct {
	log      *slog.Logger
	host     ReclaimHost
	registry *runs.Registry
	saved    []runs.TrackedRun
}

// NewReclaimer returns a reclaimer for the handles the previous server saved.
func NewReclaimer(log *slog.Logger, host ReclaimHost, registry *runs.Registry, saved []runs.TrackedRun) *Reclaimer {
	return &Reclaimer{log: log, host: host, registry: registry, saved: saved}
}

// errHandleMismatch marks a persisted handle whose pane, agent or worktree is gone.
var errHandleMismatch = errors.New("handle mismatch")

// ReclaimRuns takes over every persisted iteration that is still alive: its
// agent is known to its runner and its worktree exists. It asks nothing, so a
// restart is invisible to running work. A dead agent that left nothing behind
// is reopened; any other handle that no longer matches is paused and left out
// of the registry.
func (r *Reclaimer) ReclaimRuns() {
	for _, t := range r.saved {
		err := r.reclaim(t)
		if err == nil {
			continue
		}
		r.log.Info("not reclaiming iteration", "ticket", t.Address, "reason", err)
		r.pauseUnreclaimed(t, err)
	}
	r.saved = nil
}

// LogFlushingRunners logs one line per runner other than the configured one
// that still hosts live runs; those runs flush on it while new launches don't.
func (r *Reclaimer) LogFlushingRunners() {
	counts := map[string]int{}
	for _, t := range r.registry.List() {
		name := t.Runner
		if name == "" {
			name = runs.RunnerHerdr
		}
		if name != r.host.ConfiguredRunner() {
			counts[name]++
		}
	}
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		r.log.Info(fmt.Sprintf("%d runs still on %s", counts[name], name), "runner", name, "runs", counts[name])
	}
}

// pauseUnreclaimed pauses the ticket of a handle that cannot be reclaimed, so a
// person sees it instead of it staying claimed forever.
func (r *Reclaimer) pauseUnreclaimed(t runs.TrackedRun, cause error) {
	kind := events.IterationError
	if errors.Is(cause, errHandleMismatch) {
		kind = events.HandleMismatch
	}
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err == nil {
		var dir string
		if dir, err = r.host.ProjectDir(addr.Project); err == nil {
			err = r.host.Pause(dir, addr, t.TicketPath, kind, "cannot reclaim iteration after restart: "+cause.Error(), t.Run)
		}
	}
	if err != nil {
		r.log.Warn("pause unreclaimed iteration", "ticket", t.Address, "err", err)
	}
}

// reclaimHeadless takes over a headless iteration: a live agent keeps running
// on the adopted session, a finished one merges as if it had just finished, and
// one that died with the server down is reopened when it left nothing behind,
// else pauses stuck with its dir kept. A live agent that was waiting for
// an answer stays waiting and unsettled.
func (r *Reclaimer) reclaimHeadless(t runs.TrackedRun, runner agentrunner.Runner) error {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	re, ok := runner.(sessions.Reattacher)
	if !ok {
		return fmt.Errorf("%w: runner %s cannot reattach", errHandleMismatch, t.Runner)
	}
	session, verdict, err := re.Reattach(runs.IterationLabel(t.Address))
	if err != nil {
		return fmt.Errorf("%w: reattach %s: %v", errHandleMismatch, t.Address, err)
	}
	if verdict == nativerunner.VerdictFinished && t.State == runs.Launching {
		// Never settle it: the agent may have finished without ever being prompted.
		return r.stopLaunching(t, runner, session, "finished")
	}
	if verdict == nativerunner.VerdictDied {
		if reopened, err := r.reopenEmptyClaim(t); err != nil || reopened {
			return err
		}
		dir, err := r.host.ProjectDir(addr.Project)
		if err != nil {
			return err
		}
		return r.host.Pause(dir, addr, t.TicketPath, events.IterationError, nativerunner.ReasonAgentDied, t.Run)
	}
	one, wt, mode, err := r.Resume(t)
	if err != nil {
		return err
	}
	root, err := queue.ParseRootRef(t.Root)
	if err != nil {
		return err
	}
	if verdict == nativerunner.VerdictLive {
		t.Session = session
	}
	t.State = ""
	r.registry.Put(t)
	r.host.Publish(wire.EventReclaimed, t.Address)
	if verdict == nativerunner.VerdictFinished || r.host.ReportedFinished(t.TicketPath) {
		r.host.Continue(runner, t, root, mode, one, wt, true)
		return nil
	}
	if !t.WaitingSince.IsZero() {
		// Idle on purpose, waiting for an answer: finishing it now would merge
		// or pause it for going quiet. The budget hard limit still reaches it
		// through the registry.
		return nil
	}
	r.holdIfRateLimited(runner, t)
	r.host.Continue(runner, t, root, mode, one, wt, false)
	return nil
}

// holdIfRateLimited marks a reclaimed run the runner reports as limited,
// before the first claim pass, so a restart never claims a ticket of a limited
// harness. The wait loop clears the mark when the limit lifts. The mark is
// the whole state: nothing is stored.
func (r *Reclaimer) holdIfRateLimited(runner agentrunner.Runner, t runs.TrackedRun) {
	resetAt, limited, err := runner.RateLimit(t.Session)
	if err != nil || !limited {
		return
	}
	r.registry.MarkRateLimit(t.Address, true, resetAt)
}

// reclaim goes through the runner that started t, whatever runner is
// configured now: a herdr run under a headless server is still herdr's. An
// unavailable runner can't say whether the agent is gone, so only an agent
// the runner reports gone is ever reopened.
func (r *Reclaimer) reclaim(t runs.TrackedRun) error {
	name := t.Runner
	if name == "" {
		name = runs.RunnerHerdr
	}
	runner := r.host.RunnerFor(t.Run)
	if runner == nil {
		return fmt.Errorf("%w: started by %s; %s unavailable", errHandleMismatch, name, name)
	}
	if err := agentrunner.Healthy(runner); err != nil {
		return fmt.Errorf("%w: started by %s; %s unavailable: %v", errHandleMismatch, name, name, err)
	}
	if name != runs.RunnerHerdr {
		return r.reclaimHeadless(t, runner)
	}
	label := runs.IterationLabel(t.Address)
	session, found, err := runner.Find(label)
	if err != nil {
		// A lookup that failed says nothing about whether the agent is gone.
		return fmt.Errorf("%w: agent %s: %v", errHandleMismatch, t.Address, err)
	}
	if !found || session.ID == "" {
		if reopened, err := r.reopenEmptyClaim(t); err != nil || reopened {
			return err
		}
		return fmt.Errorf("%w: no pane", errHandleMismatch)
	}
	if t.State == runs.Launching {
		st, err := runner.Status(session)
		if err != nil {
			return fmt.Errorf("%w: agent %s: %v", errHandleMismatch, t.Address, err)
		}
		if st.Waiting() {
			return r.stopLaunching(t, runner, session, string(st.State))
		}
	}
	one, wt, mode, err := r.Resume(t)
	if err != nil {
		return err
	}
	root, err := queue.ParseRootRef(t.Root)
	if err != nil {
		return err
	}
	t.Runner = runs.RunnerHerdr
	t.Session = agentrunner.Session{Label: label, ID: session.ID, SessionID: t.Session.SessionID}
	t.State = ""
	r.registry.Put(t)
	r.host.Publish(wire.EventReclaimed, t.Address)
	if r.host.ReportedFinished(t.TicketPath) {
		// The wait reads the agent's state, which a leftover background task can hold at working.
		r.host.Continue(runner, t, root, mode, one, wt, true)
		return nil
	}
	r.holdIfRateLimited(runner, t)
	r.host.Continue(runner, t, root, mode, one, wt, false)
	return nil
}

// stopLaunching closes the agent of a launch the server died in, which is
// waiting rather than working, so it may never have been prompted; then the
// claim reopens if it is empty.
func (r *Reclaimer) stopLaunching(t runs.TrackedRun, runner agentrunner.Runner, session agentrunner.Session, status string) error {
	if err := runner.Stop(session); err != nil {
		return fmt.Errorf("stop launching agent: %w", err)
	}
	if reopened, err := r.reopenEmptyClaim(t); err != nil || reopened {
		return err
	}
	return fmt.Errorf("%w: launching agent %s", errHandleMismatch, status)
}

// Resume rebuilds the iteration a persisted handle stands for, whose worktree
// must still exist.
func (r *Reclaimer) Resume(t runs.TrackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, merge.IterationMode, error) {
	one, wt, mode, err := r.ResolveRun(t)
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
func (r *Reclaimer) reopenEmptyClaim(t runs.TrackedRun) (bool, error) {
	one, wt, mode, err := r.ResolveRun(t)
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
	dir, err := r.host.ProjectDir(addr.Project)
	if err != nil {
		return false, err
	}
	if err := mode.Discard(r.host.DefaultDeps(), one, wt); err != nil {
		return false, err
	}
	const reason = "agent gone, no commits, no edits"
	if err := ticketlog.StartOver(t.TicketPath, time.Now(), "reopened automatically after restart: "+reason, ""); err != nil {
		return false, err
	}
	if err := ralphloop.AppendEvent(dir, addr.Epic, ralphloop.Event{
		Type: string(events.ClaimReopened), Ticket: addr.ID, Address: t.Address, Reason: reason,
	}); err != nil {
		r.log.Warn("log claim-reopened", "ticket", t.Address, "err", err)
	}
	r.log.Info("reopened empty claim", "ticket", t.Address)
	r.host.Publish(wire.EventTicketChanged, t.Address)
	return true, nil
}

// iterationEmpty reports whether an iteration provably holds no work. A
// directory the ticket doesn't own, or a detached worktree's commits, can't be
// measured, so those count as holding work unless the worktree is gone.
func iterationEmpty(mode merge.IterationMode, wt ralphloop.IterationWorktree, repo, base string) (bool, error) {
	if mode.Feature || mode.ScratchDir != "" {
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

// ResolveRun rebuilds the iteration a persisted handle stands for, whether or
// not its worktree is still on disk.
func (r *Reclaimer) ResolveRun(t runs.TrackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, merge.IterationMode, error) {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, merge.IterationMode{}, err
	}
	dir, err := r.host.ProjectDir(addr.Project)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, merge.IterationMode{}, err
	}
	epics, err := tickets.LoadResolved(addr.Project, dir)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, merge.IterationMode{}, err
	}
	one := ralphloop.OneIteration{
		RepoDir: t.Repo, WorkspaceID: t.Workspace, Epic: addr.Epic, ScratchDir: dir, Agent: ralphloop.AgentKind(t.Agent),
		StallNudgeDelay: r.host.StalledNudgeDelay(),
	}
	for _, e := range epics {
		for _, tk := range e.Tickets {
			if tk.Path == t.TicketPath {
				one.Ticket = tk
			}
		}
	}
	if one.Ticket.Path == "" {
		return one, ralphloop.IterationWorktree{}, merge.IterationMode{}, errors.New("ticket gone")
	}
	one.Sink = r.host.FinishSink(addr, one.Ticket.Path)
	mode := r.host.IterationModeFor(addr, one.Ticket)
	wt, err := mode.Resume(r.host.DefaultDeps(), one, t.Base)
	return one, wt, mode, err
}
