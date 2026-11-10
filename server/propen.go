package server

import (
	"path/filepath"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets/ticketgit"
)

// OpenPR runs one attempt of the PR flow's open step for root.
func (s *Server) OpenPR(root queue.RootRef) (prflow.Outcome, error) {
	return prflow.Open(prOpenHost{s}, root)
}

// PROpenBlock is why root's last open attempt could not go on, "" when it is not blocked.
func (s *Server) PROpenBlock(root queue.RootRef) string { return s.prReasons.Reason(root) }

// SetPRMergedHook sets what runs when an open attempt finds the epic branch's
// PR already merged.
func (s *Server) SetPRMergedHook(f func(queue.RootRef, pr.HeadPR)) { s.prMergedHook = f }

// prOpenHost is the root as the open step sees it.
type prOpenHost struct{ s *Server }

func (h prOpenHost) Env() []string { return h.s.cfg.Env }

func (h prOpenHost) RepoDir(project string) (string, bool) {
	return prWatchHost(h).RepoDir(project)
}

func (h prOpenHost) Trunk(project, repo string) (string, error) {
	return ticketgit.Trunk(filepath.Join(h.s.cfg.TicketStore, project), repo)
}

func (h prOpenHost) EpicDir(project, epic string) string {
	return filepath.Join(h.s.cfg.TicketStore, project, epic)
}

func (h prOpenHost) Worktree(project, epic string) string {
	return filepath.Join(h.s.projects.WorktreeDir(project), epic)
}

func (h prOpenHost) Reasons() *prflow.Reasons { return &h.s.prReasons }

func (h prOpenHost) Kick() { h.s.prWatch.Kick() }

func (h prOpenHost) PRMerged(root queue.RootRef, p pr.HeadPR) {
	if h.s.prMergedHook != nil {
		h.s.prMergedHook(root, p)
	}
}
