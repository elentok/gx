package server

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// adoptablePause reports whether a late finish can adopt a pause with this
// reason: a zero-commit one, or a merge agent's unreported stop. A reason
// implies its class, so the reason alone decides.
func adoptablePause(status schema.Status, reason schema.PauseReason) bool {
	return status == schema.StatusPaused && (reason == schema.PauseZeroCommit || reason == schema.PauseMergeUnreported)
}

// lateFinish is an adoptable pause whose agent reported finished after it.
type lateFinish struct {
	dir        string
	addr       tickets.Address
	path       string
	pause      ralphloop.Event
	finishedAt time.Time
	commitless bool
}

// inFlight is a set of keys with work running, so overlapping rescans start
// each adoption once.
type inFlight struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}

func (f *inFlight) start(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m[key] != nil {
		return false
	}
	if f.m == nil {
		f.m = map[string]chan struct{}{}
	}
	f.m[key] = make(chan struct{})
	return true
}

func (f *inFlight) end(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	close(f.m[key])
	delete(f.m, key)
}

// running is closed when key's work ends; nil when none runs.
func (f *inFlight) running(key string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[key]
}

// adoptFirst is resume's and relaunch's late-finish check: it adopts a match
// itself, or waits out the adoption a rescan already started, and reports
// whether the ticket ended adopted.
func (s *Server) adoptFirst(dir string, addr tickets.Address, path string) bool {
	lf, ok := lateFinishOf(dir, addr, path)
	if !ok {
		return false
	}
	key := addr.String()
	if s.lateFinishes.start(key) {
		defer s.lateFinishes.end(key)
		return s.adoptLateFinish(lf)
	}
	if done := s.lateFinishes.running(key); done != nil {
		<-done
	}
	t, err := schema.ParseTicket(path)
	return err == nil && t.Status == schema.StatusDone
}

// scanLateFinishes adopts every late finish in the store. It is a finish, not
// a recovery remedy (ADR 0038), so no settings gate holds it.
func (s *Server) scanLateFinishes() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		epics, _ := s.idx.epicsOf(dir)
		for _, e := range epics {
			for _, t := range e.Tickets {
				if !adoptablePause(schema.Status(t.Status), t.PauseReason) {
					continue
				}
				addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(e.Path), ID: t.Identifier}
				lf, ok := lateFinishOf(dir, addr, t.Path)
				if !ok || !s.lateFinishes.start(addr.String()) {
					continue
				}
				s.goSettle(func() {
					defer s.lateFinishes.end(addr.String())
					_ = s.adoptLateFinish(lf)
				})
			}
		}
	}
}

// lateFinishOf reports whether the ticket is untouched since its latest pause,
// an adoptable one, and its agent reported finished after that pause. Only
// status and pause kind are compared: a person who resumed, reset or re-paused
// it changed one of them.
func lateFinishOf(dir string, addr tickets.Address, path string) (lateFinish, bool) {
	t, err := schema.ParseTicket(path)
	if err != nil || !adoptablePause(t.Status, t.PauseReason) || t.IterationStatus != schema.IterationStatusFinished {
		return lateFinish{}, false
	}
	pause, ok := ralphloop.LatestPauseEvent(dir, addr.Epic, addr.ID)
	if !ok || pause.Type != string(events.Paused) || pause.Kind != string(t.PauseReason) || !t.IterationFinishedAt.After(pause.Time) {
		return lateFinish{}, false
	}
	return lateFinish{dir: dir, addr: addr, path: path, pause: pause, finishedAt: t.IterationFinishedAt, commitless: t.IsCommitless()}, true
}

// adoptLateFinish merges a late finish with commits, or closes a commitless one
// with none, through the iteration's own finish path. The pause dropped the
// runs.json entry, so the runner and label come from the pause event. A late
// commit with no commitless report, or none at all, stays paused. A merge
// ticket closes once its finish checks pass, and stays paused until then, so
// a later rescan retries after a person fixes the epic.
func (s *Server) adoptLateFinish(lf lateFinish) bool {
	addr := lf.addr
	label, branch, worktree := ralphloop.IterationIdentity(addr.Epic, addr.ID, s.worktreeDir(addr.Project))
	if lf.pause.Label != "" {
		label = lf.pause.Label
	}
	run := Run{
		Address: addr.String(), Agent: string(lf.pause.Agent), Runner: lf.pause.Runner,
		Session: agentrunner.Session{Label: label, SessionID: lf.pause.AgentSession},
		Model:   lf.pause.Model, Effort: lf.pause.Effort,
	}
	runner := s.runnerForRun(run)
	if runner == nil {
		s.log.Warn("late finish: runner unavailable", "ticket", addr, "runner", lf.pause.Runner)
		return false
	}
	_, repo, err := s.projectOf(addr.Project)
	if err != nil {
		s.log.Warn("late finish", "ticket", addr, "err", err)
		return false
	}
	base, _ := git.MergeBase(worktree, branch, addr.Epic)
	one, wt, mode, err := s.resolveRun(trackedRun{Run: run, Repo: repo, Base: base, TicketPath: lf.path})
	if err != nil {
		s.log.Warn("late finish", "ticket", addr, "err", err)
		return false
	}
	// Only a merge ticket runs in the feature worktree, and only it pauses merge-unreported.
	if mode.feature != (lf.pause.Kind == string(events.MergeUnreported)) {
		return false
	}
	deps := s.depsWith(runner)
	ahead := 0
	if !mode.commitless && !mode.feature {
		if ahead, err = deps.CommitsAhead(wt.Path, wt.Base(), wt.Branch); err != nil || (ahead == 0 && !lf.commitless) {
			return false
		}
	}
	pane := ""
	if sess, ok, err := runner.Find(label); err == nil && ok {
		if err := ralphloop.WaitIterationFinished(deps, one, wt, sess); err != nil {
			s.log.Warn("late finish: wait", "ticket", addr, "err", err)
			return false
		}
		pane = sess.ID
	}
	// A person may have resumed or re-paused it while the agent settled.
	if _, ok := lateFinishOf(lf.dir, addr, lf.path); !ok {
		return false
	}
	// A failing check here would re-pause iteration-error, which is never adopted.
	if mode.feature {
		target, err := mode.target()
		if err != nil {
			s.log.Warn("late finish: merge target", "ticket", addr, "err", err)
			return false
		}
		if _, fault, err := ralphloop.MergeFinishFault(deps, one, wt.Path, target); err != nil || fault != "" {
			return false
		}
	}
	if !s.merges.begin() {
		return false
	}
	defer s.merges.end()
	// The finish makes the ticket's investigations claimable; the claim waits
	// until the unstarted ones are cancelled.
	s.adoptMu.Lock()
	unlocked := false
	unlock := func() {
		if !unlocked {
			unlocked = true
			s.adoptMu.Unlock()
		}
	}
	defer unlock()
	one.Model, one.Effort = s.runAgentConfig(addr.Project, one.Agent, run)
	if err := tickets.EndOpenRound(lf.path, tickets.SourceGx, tickets.HowAuto, "adopted"); err != nil {
		s.log.Warn("late finish: close question", "ticket", addr, "err", err)
		return false
	}
	out, err := mode.finish(deps, one, wt, pane)
	if err != nil {
		if perr := s.pauseTicket(lf.dir, addr, lf.path, events.IterationError, err.Error(), run); perr != nil {
			s.log.Warn("late finish: pause failed finish", "ticket", addr, "err", perr)
		}
		s.events.publish(EventIterationFailed, addr.String())
		return false
	}
	if !out.Merged {
		s.log.Warn("late finish re-paused", "ticket", addr, "kind", out.Kind, "reason", out.Reason)
		s.events.publish(EventIterationPaused, addr.String())
		return false
	}
	sha, outcome := "", "closed commitless"
	if mode.feature {
		outcome = "closed merge"
	} else if ahead > 0 {
		sha = ralphloop.LatestMergedSHA(lf.dir, addr.Epic, addr.ID)
		outcome = "merged " + sha
	}
	s.logEvent(addr, ralphloop.Event{
		Type: string(events.LateFinishAdopted), SHA: sha,
		Reason: fmt.Sprintf("paused %s, finished %s; %s",
			lf.pause.Time.UTC().Format(time.RFC3339), lf.finishedAt.UTC().Format(time.RFC3339), outcome),
	})
	s.events.publish(EventTicketDone, addr.String())
	s.stampRetro(addr, lf.path, false)
	s.cancelUnstartedInvestigations(lf.dir, addr)
	unlock()
	// The adopted ticket may be the epic's last; settleRun would have completed it.
	s.completeRootIfDone(rootOf(addr), one)
	s.kickRunner()
	return true
}

// cancelUnstartedInvestigations cancels the adopted ticket's investigate
// children that have not been claimed yet; a running one finishes its report.
func (s *Server) cancelUnstartedInvestigations(dir string, addr tickets.Address) {
	epics, err := tickets.LoadResolved(addr.Project, dir)
	if err != nil {
		s.log.Warn("late finish: load investigations", "ticket", addr, "err", err)
		return
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		forks := e.ForkParents()
		for _, t := range e.Tickets {
			parent, ok := forks.Of(t)
			if !ok || parent.Identifier != addr.ID || t.Type != string(schema.TypeInvestigate) || t.Status != string(schema.StatusOpen) {
				continue
			}
			if err := ralphloop.CancelWithResolution(t.Path, "late finish adopted"); err != nil {
				s.log.Warn("late finish: cancel investigation", "ticket", t.Identifier, "err", err)
				continue
			}
			child := tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}
			s.stampRetro(child, t.Path, true)
			s.events.publish(EventTicketCancelled, child.String())
		}
	}
}
