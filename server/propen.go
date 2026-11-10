package server

import (
	"path/filepath"
	"slices"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// OpenPR runs one attempt of the PR flow's open step for root.
func (p PRFlow) OpenPR(root queue.RootRef) (prflow.Outcome, error) {
	return prflow.Open(prOpenHost{p.s}, root)
}

// CreatePR is the Create PR route's handler.
func (p PRFlow) CreatePR(req wire.QueueRequest) (wire.QueueResult, error) {
	return prflow.Create(prOpenHost{p.s}, req)
}

// TryAgain is the Try again route's handler.
func (p PRFlow) TryAgain(req wire.QueueRequest) (wire.QueueResult, error) {
	return prflow.TryAgain(prOpenHost{p.s}, req)
}

// PROpenBlock is why root's last open attempt could not go on, "" when it is not blocked.
func (p PRFlow) PROpenBlock(root queue.RootRef) string { return p.s.prReasons.Reason(root) }

// SetPRMergedHook sets what runs when an open attempt finds the epic branch's
// PR already merged.
func (p PRFlow) SetPRMergedHook(f func(queue.RootRef, pr.HeadPR)) { p.s.prMergedHook = f }

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

func (h prOpenHost) PRTextSkill(project string) string { return h.s.projects.PRTextSkill(project) }

func (h prOpenHost) Reasons() *prflow.Reasons { return &h.s.prReasons }

func (h prOpenHost) Epic(project, epic string) (tickets.Epic, bool) {
	dir, err := h.s.projects.Dir(project)
	if err != nil {
		return tickets.Epic{}, false
	}
	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		return tickets.Epic{}, false
	}
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return filepath.Base(e.Path) == epic })
	if i < 0 {
		return tickets.Epic{}, false
	}
	return epics[i], true
}

func (h prOpenHost) EpicState(root queue.RootRef, e tickets.Epic) wire.EpicState {
	return h.s.EpicState(root, e)
}

func (h prOpenHost) Kick() { h.s.prWatch.Kick() }

func (h prOpenHost) PRMerged(root queue.RootRef, p pr.HeadPR) {
	if h.s.prMergedHook != nil {
		h.s.prMergedHook(root, p)
		return
	}
	// Give the epic the PR's identity first: the Trunk update retries from the
	// watcher's list, which holds epics that have one.
	dir := h.EpicDir(root.Project, root.Epic)
	if err := tickets.StampEpicFields(dir, map[string]any{"pr_number": p.Number, "pr_url": p.URL}); err != nil {
		h.s.log.Warn("stamp adopted merged PR", "root", root, "err", err)
		return
	}
	if err := h.s.prWatch.Merged(prflow.Target{Project: root.Project, Epic: root.Epic, Dir: dir, Number: p.Number, URL: p.URL}); err != nil {
		h.s.log.Warn("adopted merged PR", "root", root, "err", err)
	}
}
