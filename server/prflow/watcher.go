package prflow

import (
	"context"
	"errors"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

// Poll intervals and the backoff cap (spec "Watching the PR").
const (
	PendingInterval = 45 * time.Second
	IdleInterval    = 3 * time.Minute
	MaxBackoff      = 5 * time.Minute
)

// Target is one epic whose PR the watcher polls.
type Target struct {
	Project string
	Epic    string
	// Dir is the epic's directory, where its activity log lives.
	Dir    string
	Number int
	URL    string
}

// Host is what the watcher needs from the server root.
type Host interface {
	// Watched is every epic with a PR number and URL and no merged_at. It
	// reads the index, not the disk.
	Watched() []Target
	// Env is the environment gh runs under.
	Env() []string
	// RepoDir is the project's repo checkout.
	RepoDir(project string) (string, bool)
	// NoChecksGrace is how long a PR with no checks waits before it counts as green.
	NoChecksGrace() time.Duration
	// AutoPR is every epic (Project, Epic and Dir set) in the Awaiting PR state
	// of a `pr` project with auto-pr on, read live from project.json.
	AutoPR() []Target
	// OpenPR runs one Open attempt for root.
	OpenPR(root queue.RootRef) (Outcome, error)
	// CancelStalePRText cancels the open or running pr-text ticket of every epic
	// without a PR in a project whose delivery is no longer `pr`, read live.
	CancelStalePRText()
	// PauseCIFailing stores "CI failing" with reason on the epic and sends the
	// one notice for it.
	PauseCIFailing(t Target, reason string) error
	// PauseForeignConflict stores "pull request conflicts with others' commits"
	// with reason on the epic and sends the one notice for it.
	PauseForeignConflict(t Target, reason string) error
	// ForkPRMerge forks a PR-mode Merge ticket for the epic's conflicting PR, or
	// stores "needs rebase" at the cap; it forks none while one is pending.
	ForkPRMerge(root queue.RootRef) error
	// CIFixSkills is the skills a ci-fix ticket of project runs.
	CIFixSkills(project string) []string
	// CIFixCaps is the ci-fix attempts allowed per check and per PR.
	CIFixCaps() (perCheck, perPR int)
	// FileFlakyFollowUp files f in the project's own follow-ups epic, or notes a
	// repeat on the open draft with the same key.
	FileFlakyFollowUp(project string, from tickets.Address, f ticketwrite.FollowUp) error
	// CancelFlowTickets cancels, with their agents, every unfinished ci-fix and
	// Merge ticket of the epic.
	CancelFlowTickets(t Target)
	// SyncTrunkAfterMerge fetches origin and fast-forwards local Trunk, then
	// stamps merged_at and releases the root. False means the update failed and
	// a block is stored; the watcher retries it every tick.
	SyncTrunkAfterMerge(t Target) (synced bool)
	// QueueRoot queues the epic's ticket so it is claimed.
	QueueRoot(root queue.RootRef, ticketID string) error
}

// View is the live state of one epic's PR. It exists only in memory.
type View struct {
	Number  int
	URL     string
	State   string // OPEN, MERGED or CLOSED
	Draft   bool
	HeadSHA string
	// Rollup is GitHub's cheap rollup state for the head.
	Rollup string
	Checks pr.CheckCounts
	CI     pr.ChecksResult
	// FailedChecks is the failed checks of the head with their run and job ids.
	FailedChecks []pr.Check
	// HeadChecks is the newest check per key on the head, whatever its state.
	HeadChecks []pr.Check
	Mergeable  string
	// LastError is the newest poll failure, empty after a good poll.
	LastError   string
	Unreachable bool
}

// Watcher polls every watched PR and keeps a View per epic.
type Watcher struct {
	host Host
	kick chan struct{}

	tickMu sync.Mutex
	mu     sync.Mutex
	views  map[epicKey]View
	// repos caches each project's owner/name; down is the projects in an outage.
	repos map[string]string
	down  map[string]bool
	// fails counts consecutive failing ticks; retryAfter is the longest
	// retry-after of the last one.
	fails      int
	retryAfter time.Duration
	// seen and readied are per epic: the head last seen, and the head already
	// marked ready.
	seen    map[epicKey]seenHead
	readied map[epicKey]string
	clock   func() time.Time
}

// SetClock replaces the watcher's clock, for tests.
func (w *Watcher) SetClock(now func() time.Time) {
	w.mu.Lock()
	w.clock = now
	w.mu.Unlock()
}

func (w *Watcher) now() time.Time {
	w.mu.Lock()
	c := w.clock
	w.mu.Unlock()
	if c == nil {
		return time.Now()
	}
	return c()
}

type epicKey struct{ project, epic string }

// NewWatcher makes a watcher over host.
func NewWatcher(host Host) *Watcher {
	return &Watcher{
		host:  host,
		kick:  make(chan struct{}, 1),
		views: map[epicKey]View{},
		repos: map[string]string{},
		down:  map[string]bool{},

		seen:    map[epicKey]seenHead{},
		readied: map[epicKey]string{},
	}
}

// Kick asks for an early poll.
func (w *Watcher) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// View is the live view of an epic's PR, false when it is not watched.
func (w *Watcher) View(project, epic string) (View, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	v, ok := w.views[epicKey{project, epic}]
	return v, ok
}

// Run polls until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	for {
		w.Tick(ctx)
		t := time.NewTimer(w.Delay())
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-w.kick:
			t.Stop()
		case <-t.C:
		}
	}
}

// Delay is the wait before the next poll: the pending interval while any CI is
// pending, else the idle one; after failures, doubling from that up to the cap
// (or the retry-after, if longer).
func (w *Watcher) Delay() time.Duration {
	grace := w.host.NoChecksGrace()
	now := w.now()
	w.mu.Lock()
	defer w.mu.Unlock()
	base := IdleInterval
	for k, v := range w.views {
		// A draft with no checks is about to be marked ready when its grace
		// window ends, so it polls as fast as pending CI.
		inGrace := v.CI == pr.ChecksNone && v.Draft && now.Sub(w.seen[k].at) < grace
		if v.CI == pr.ChecksPending || inGrace {
			base = PendingInterval
			break
		}
	}
	if w.fails == 0 {
		return base
	}
	d := base
	for i := 1; i < w.fails && d < MaxBackoff; i++ {
		d *= 2
	}
	return max(min(d, MaxBackoff), w.retryAfter)
}

// Tick runs one poll of every watched PR.
func (w *Watcher) Tick(ctx context.Context) {
	// Run's pass and a test's forced pass must not interleave, or one outage logs twice.
	w.tickMu.Lock()
	defer w.tickMu.Unlock()
	targets := w.liveTargets()
	byProject := map[string][]Target{}
	for _, t := range targets {
		byProject[t.Project] = append(byProject[t.Project], t)
	}
	w.mu.Lock()
	for k := range w.views {
		if !slices.ContainsFunc(targets, func(t Target) bool { return t.Project == k.project && t.Epic == k.epic }) {
			delete(w.views, k)
		}
	}
	w.mu.Unlock()

	failed := false
	var retryAfter time.Duration
	for project, ts := range byProject {
		if ctx.Err() != nil {
			return
		}
		if err := w.pollProject(project, ts); err != nil {
			failed = true
			var le *pr.PRListError
			if errors.As(err, &le) {
				retryAfter = max(retryAfter, le.RetryAfter)
			}
			w.outage(project, ts, err)
		} else {
			w.recovered(project, ts)
		}
	}
	w.retryMerged()
	w.host.CancelStalePRText()
	w.autoOpen(ctx)
	w.mu.Lock()
	defer w.mu.Unlock()
	if failed {
		w.fails++
		w.retryAfter = retryAfter
	} else {
		w.fails, w.retryAfter = 0, 0
	}
}

// autoOpen opens a PR for each Auto-PR epic, except one whose newest "PR closed"
// entry is newer than its newest "PR opened": a closed PR is never reopened
// automatically. A failed attempt keeps its stored reason and retries next tick.
func (w *Watcher) autoOpen(ctx context.Context) {
	for _, t := range w.host.AutoPR() {
		if ctx.Err() != nil {
			return
		}
		closed, _ := LastClosed(t.Dir)
		opened, _ := LastOpened(t.Dir)
		if closed.After(opened) {
			continue
		}
		_, _ = w.host.OpenPR(queue.RootRef{Project: t.Project, Epic: t.Epic})
	}
}

// liveTargets are the host's targets minus epics whose log says the PR merged.
func (w *Watcher) liveTargets() []Target {
	var out []Target
	for _, t := range w.host.Watched() {
		if mergeRecorded(t.Dir) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (w *Watcher) pollProject(project string, ts []Target) error {
	dir, ok := w.host.RepoDir(project)
	if !ok {
		return nil
	}
	r := pr.Runner{Env: w.host.Env(), Dir: dir}
	repo, err := w.repoOf(project, r)
	if err != nil {
		return err
	}
	numbers := make([]int, len(ts))
	for i, t := range ts {
		numbers[i] = t.Number
	}
	states, err := r.WatchQuery(repo, numbers)
	if err != nil {
		return err
	}
	for _, t := range ts {
		st, ok := states[t.Number]
		if !ok {
			continue
		}
		v, err := w.nextView(r, repo, t, st)
		if err != nil {
			return err
		}
		w.mu.Lock()
		w.views[epicKey{t.Project, t.Epic}] = v
		w.mu.Unlock()
		// An action error belongs to this epic: log it and go on with the
		// others. It is not a GitHub outage.
		if err := w.act(r, repo, t, v); err != nil {
			log.Printf("pr watcher: %s:%s: %v", t.Project, t.Epic, err)
		}
	}
	return nil
}

// nextView folds a poll into the epic's view. The per-check detail call runs
// only when the head or the rollup moved, or the rollup is failing.
func (w *Watcher) nextView(r pr.Runner, repo string, t Target, st pr.WatchState) (View, error) {
	w.mu.Lock()
	v := w.views[epicKey{t.Project, t.Epic}]
	w.mu.Unlock()
	failing := st.Rollup == "FAILURE" || st.Rollup == "ERROR"
	detail := v.Number == 0 || v.HeadSHA != st.HeadSHA || v.Rollup != st.Rollup || failing
	next := View{Number: t.Number, URL: t.URL, State: st.State, Draft: st.IsDraft, HeadSHA: st.HeadSHA,
		Rollup: st.Rollup, Mergeable: st.Mergeable, Checks: v.Checks, CI: v.CI, FailedChecks: v.FailedChecks,
		HeadChecks: v.HeadChecks}
	if detail {
		head, checks, err := r.PRChecks(repo, t.Number)
		if err != nil {
			return v, err
		}
		s := pr.Summarize(head, checks)
		next.CI, next.Checks, next.FailedChecks, next.HeadChecks = s.Result, s.Counts, s.FailedChecks, s.Head
	}
	return next, nil
}

func (w *Watcher) repoOf(project string, r pr.Runner) (string, error) {
	w.mu.Lock()
	repo, ok := w.repos[project]
	w.mu.Unlock()
	if ok {
		return repo, nil
	}
	repo, err := r.Preflight()
	if err != nil {
		return "", err
	}
	w.mu.Lock()
	w.repos[project] = repo
	w.mu.Unlock()
	return repo, nil
}

// outage marks the project's views unreachable and, on the first failing poll
// of an outage, logs it once per epic. It never pauses an epic or writes a ticket.
func (w *Watcher) outage(project string, ts []Target, err error) {
	w.mu.Lock()
	first := !w.down[project]
	w.down[project] = true
	for _, t := range ts {
		k := epicKey{t.Project, t.Epic}
		if v, ok := w.views[k]; ok {
			v.Unreachable, v.LastError = true, err.Error()
			w.views[k] = v
		}
	}
	w.mu.Unlock()
	if first {
		for _, t := range ts {
			_ = Append(t.Dir, Entry{Kind: KindGitHubDown, Detail: err.Error()})
		}
	}
}

func (w *Watcher) recovered(project string, ts []Target) {
	w.mu.Lock()
	was := w.down[project]
	delete(w.down, project)
	for _, t := range ts {
		k := epicKey{t.Project, t.Epic}
		if v, ok := w.views[k]; ok {
			v.Unreachable, v.LastError = false, ""
			w.views[k] = v
		}
	}
	w.mu.Unlock()
	if was {
		for _, t := range ts {
			_ = Append(t.Dir, Entry{Kind: KindGitHubUp})
		}
	}
}
