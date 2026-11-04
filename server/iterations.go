package server

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func (s *Server) iterations(w http.ResponseWriter, _ *http.Request) {
	list := []wire.IterationInfo{}
	for _, t := range s.idx.Snapshot().Tickets {
		if t.Status != "claimed" {
			continue
		}
		if it, ok := s.liveIteration(t.Address); ok {
			list = append(list, it.info)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
	writeJSON(w, list)
}

// liveIter is a live iteration with the runner and session hosting it.
type liveIter struct {
	info    wire.IterationInfo
	runner  agentrunner.Runner
	session agentrunner.Session
}

// liveIteration resolves a claimed ticket's iteration through the run's
// runner. It is live when that runner finds the label and the session has not
// exited; blocked is live.
func (s *Server) liveIteration(address string) (liveIter, bool) {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return liveIter{}, false
	}
	runner, sess, ok, err := s.liveSession(a)
	if err != nil || !ok || sess.ID == "" {
		return liveIter{}, false
	}
	st, err := runner.Status(sess)
	// herdr's done is a finished turn resting in its pane, not an exit.
	if err != nil || (st.State == agentrunner.StateDone && runner.Name() != runs.RunnerHerdr) {
		return liveIter{}, false
	}
	if sess.SessionID == "" {
		sess.SessionID = st.SessionID
	}
	label, branch, worktree := ralphloop.IterationIdentity(a.Epic, a.ID, s.projects.WorktreeDir(a.Project))
	it := wire.IterationInfo{Address: address, Worktree: worktree, Branch: branch, Session: sess.SessionID, Runner: runner.Name()}
	if runner.Name() == queue.RunnerPTY {
		it.QueueAgent = queue.AgentIDPrefix + runs.IterationLabel(address)
	}
	if runner.Name() == runs.RunnerHerdr {
		it.Pane = sess.ID
		cli := herdr.CLI{Env: s.cfg.Env}
		if agent, err := cli.AgentGet(label); err == nil {
			it.Tab = agent.TabID
		}
	}
	if run, ok := s.registry.RunOf(address); ok {
		it.Agent = run.Agent
	}
	// Base and transcript are best effort: a missing git worktree or session
	// leaves them empty rather than hiding a live iteration.
	if base, err := git.MergeBase(worktree, branch, a.Epic); err == nil {
		it.Base = base
	}
	if sess.SessionID != "" {
		if p, err := s.projects.TranscriptPath(worktree, sess.SessionID); err == nil {
			it.Transcript = p
		}
	}
	return liveIter{info: it, runner: runner, session: sess}, true
}

func (s *Server) queueView(w http.ResponseWriter, _ *http.Request) {
	list := []wire.QueueEntry{}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, err := tickets.LoadResolved(project, dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, e := range epics {
			epicName := filepath.Base(e.Path)
			for _, d := range ralphloop.EpicQueue(e) {
				list = append(list, wire.QueueEntry{
					Address:  project + ":" + epicName + "/" + d.Ticket,
					Decision: string(d.Decision), Reason: d.Reason,
				})
			}
		}
	}
	writeJSON(w, list)
}
