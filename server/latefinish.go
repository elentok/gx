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

// lateFinish is a zero-commit park whose agent reported finished after it.
type lateFinish struct {
	dir        string
	addr       tickets.Address
	path       string
	park       ralphloop.Event
	finishedAt time.Time
	commitless bool
}

// inFlight is a set of keys with work running, so overlapping rescans start
// each adoption once.
type inFlight struct {
	mu sync.Mutex
	m  map[string]bool
}

func (f *inFlight) start(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m[key] {
		return false
	}
	if f.m == nil {
		f.m = map[string]bool{}
	}
	f.m[key] = true
	return true
}

func (f *inFlight) end(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, key)
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
				if t.Status != string(schema.StatusNeedsAnswer) || t.ParkKind != schema.ParkKindZeroCommit {
					continue
				}
				addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(e.Path), ID: t.Identifier}
				lf, ok := lateFinishOf(dir, addr, t.Path)
				if !ok || !s.lateFinishes.start(addr.String()) {
					continue
				}
				s.goSettle(func() {
					defer s.lateFinishes.end(addr.String())
					s.adoptLateFinish(lf)
				})
			}
		}
	}
}

// lateFinishOf reports whether the ticket is untouched since its latest park,
// a zero-commit one, and its agent reported finished after that park. Only
// status and park kind are compared: a person who unparked, reset or re-parked
// it changed one of them.
func lateFinishOf(dir string, addr tickets.Address, path string) (lateFinish, bool) {
	t, err := schema.ParseTicket(path)
	if err != nil || t.Status != schema.StatusNeedsAnswer || t.ParkKind != schema.ParkKindZeroCommit ||
		t.IterationStatus != schema.IterationStatusFinished {
		return lateFinish{}, false
	}
	park, ok := ralphloop.LatestParkEvent(dir, addr.Epic, addr.ID)
	if !ok || events.Type(park.Type) != events.NeedsAnswer || events.Kind(park.Kind) != events.ZeroCommit ||
		!t.FinishedAt.After(park.Time) {
		return lateFinish{}, false
	}
	return lateFinish{dir: dir, addr: addr, path: path, park: park, finishedAt: t.FinishedAt, commitless: t.IsCommitless()}, true
}

// adoptLateFinish lands a late finish with commits, or closes a commitless one
// with none, through the iteration's own finish path. The park dropped the
// runs.json entry, so the runner and label come from the park event. A late
// commit with no commitless report, or none at all, stays parked.
func (s *Server) adoptLateFinish(lf lateFinish) {
	addr := lf.addr
	label, branch, worktree := ralphloop.IterationIdentity(addr.Epic, addr.ID, s.worktreeDir(addr.Project))
	if lf.park.Label != "" {
		label = lf.park.Label
	}
	run := Run{
		Address: addr.String(), Agent: string(lf.park.Agent), Runner: lf.park.Runner,
		Session: agentrunner.Session{Label: label, SessionID: lf.park.AgentSession},
	}
	runner := s.runnerForRun(run)
	if runner == nil {
		s.log.Warn("late finish: runner unavailable", "ticket", addr, "runner", lf.park.Runner)
		return
	}
	_, repo, err := s.projectOf(addr.Project)
	if err != nil {
		s.log.Warn("late finish", "ticket", addr, "err", err)
		return
	}
	base, _ := git.MergeBase(worktree, branch, addr.Epic)
	one, wt, mode, err := s.resolveRun(trackedRun{Run: run, Repo: repo, Base: base, TicketPath: lf.path})
	if err != nil {
		s.log.Warn("late finish", "ticket", addr, "err", err)
		return
	}
	// Merge tickets run in the feature worktree and park other kinds.
	if mode.feature {
		return
	}
	deps := s.depsWith(runner)
	ahead := 0
	if !mode.commitless {
		if ahead, err = deps.CommitsAhead(wt.Path, wt.Base(), wt.Branch); err != nil || (ahead == 0 && !lf.commitless) {
			return
		}
	}
	pane := ""
	if sess, ok, err := runner.Find(label); err == nil && ok {
		if err := ralphloop.WaitIterationFinished(deps, one, wt, sess); err != nil {
			s.log.Warn("late finish: wait", "ticket", addr, "err", err)
			return
		}
		pane = sess.ID
	}
	// A person may have unparked or re-parked it while the agent settled.
	if _, ok := lateFinishOf(lf.dir, addr, lf.path); !ok {
		return
	}
	if !s.lands.begin() {
		return
	}
	defer s.lands.end()
	ac := s.agentConfig(addr.Project, one.Agent)
	one.Model, one.Effort = ac.Model, ac.Effort
	out, err := mode.finish(deps, one, wt, pane)
	if err != nil {
		if perr := s.parkTicket(lf.dir, addr, lf.path, events.IterationError, err.Error(), run); perr != nil {
			s.log.Warn("late finish: park failed finish", "ticket", addr, "err", perr)
		}
		s.events.publish(EventIterationFailed, addr.String())
		return
	}
	if !out.Landed {
		s.log.Warn("late finish re-parked", "ticket", addr, "kind", out.Kind, "reason", out.Reason)
		s.events.publish(EventIterationParked, addr.String())
		return
	}
	sha, outcome := "", "closed commitless"
	if ahead > 0 {
		sha = ralphloop.LatestLandedSHA(lf.dir, addr.Epic, addr.ID)
		outcome = "landed " + sha
	}
	s.logEvent(addr, ralphloop.Event{
		Type: string(events.LateFinishAdopted), SHA: sha,
		Reason: fmt.Sprintf("parked %s, finished %s; %s",
			lf.park.Time.UTC().Format(time.RFC3339), lf.finishedAt.UTC().Format(time.RFC3339), outcome),
	})
	s.events.publish(EventTicketDone, addr.String())
	s.stampRetro(addr, lf.path, false)
	s.kickRunner()
}
