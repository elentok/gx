package server

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// IterationInfo is one live iteration in the /v1/iterations payload. The
// transcript path is only located here: parsing it belongs to the transcript
// module.
type IterationInfo struct {
	Address    string `json:"address"`
	Agent      string `json:"agent,omitempty"`
	Pane       string `json:"pane"`
	Tab        string `json:"tab,omitempty"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	Base       string `json:"base,omitempty"`
	Session    string `json:"session,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

// QueueEntry is one ticket in the /v1/queue payload with the scheduler's own
// verdict on it.
type QueueEntry struct {
	Address  string `json:"address"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

func (s *Server) iterations(w http.ResponseWriter, _ *http.Request) {
	list := []IterationInfo{}
	for _, t := range s.idx.snapshot().Tickets {
		if t.Status != "claimed" {
			continue
		}
		if it, ok := s.liveIteration(t.Address); ok {
			list = append(list, it)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
	writeJSON(w, list)
}

// liveIteration resolves a claimed ticket's iteration. It is not live when
// herdr knows no agent under the iteration's label.
func (s *Server) liveIteration(address string) (IterationInfo, bool) {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return IterationInfo{}, false
	}
	worktreeDir := s.worktreeDir(a.Project)
	label, branch, worktree := ralphloop.IterationIdentity(a.Epic, a.ID, worktreeDir)
	agent, err := herdr.AgentGet(label)
	if err != nil || agent.PaneID == "" {
		return IterationInfo{}, false
	}
	it := IterationInfo{
		Address: address, Pane: agent.PaneID, Tab: agent.TabID,
		Worktree: worktree, Branch: branch, Session: agent.AgentSession,
	}
	if run, ok := s.registry.runOf(address); ok {
		it.Agent = run.Agent
	}
	// Base and transcript are best effort: a missing git worktree or session
	// leaves them empty rather than hiding a live pane.
	if base, err := git.MergeBase(worktree, branch, a.Epic); err == nil {
		it.Base = base
	}
	if agent.AgentSession != "" {
		if p, err := transcript.Path(worktree, agent.AgentSession); err == nil {
			it.Transcript = p
		}
	}
	return it, true
}

// worktreeDir is where the project's repo keeps its linked worktrees.
func (s *Server) worktreeDir(project string) string {
	pf, err := config.ReadProjectFile(filepath.Join(s.cfg.TicketStore, project))
	if err != nil || pf.Repo == nil {
		return ""
	}
	repo, err := git.FindRepo(*pf.Repo)
	if err != nil {
		return ""
	}
	return repo.LinkedWorktreeDir()
}

func (s *Server) queue(w http.ResponseWriter, _ *http.Request) {
	list := []QueueEntry{}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, err := tickets.Load(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, e := range epics {
			epicName := filepath.Base(e.Path)
			for _, d := range ralphloop.EpicQueue(e) {
				list = append(list, QueueEntry{
					Address:  project + ":" + epicName + "/" + d.Ticket,
					Decision: d.Decision, Reason: d.Reason,
				})
			}
		}
	}
	writeJSON(w, list)
}
