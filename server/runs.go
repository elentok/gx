package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/herdr"
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
	Pane    string `json:"pane"`
	Tab     string `json:"tab"`
}

// trackedRun is a Run plus what a restarted server needs to finish it.
type trackedRun struct {
	Run
	Root       string `json:"root"`
	Repo       string `json:"repo"`
	Workspace  string `json:"workspace"`
	Base       string `json:"base"`
	TicketPath string `json:"ticket_path"`
}

// runRegistry is the server's own record of launched iterations, keyed by
// project:epic (one running iteration per root). Independent of the TUI's.
// It lives in a state-dir file too, so a restart can reclaim what is running.
type runRegistry struct {
	mu   sync.Mutex
	path string
	log  *slog.Logger
	runs map[string]trackedRun
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
	var saved []trackedRun
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, nil, err
	}
	return r, saved, nil
}

// save writes through a rename so a crash never leaves a torn file. Callers hold mu.
func (r *runRegistry) save() {
	list := make([]trackedRun, 0, len(r.runs))
	for _, t := range r.runs {
		list = append(list, t)
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

func (r *runRegistry) has(root string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[root]
	return ok
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

func (r *runRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

func (r *runRegistry) delete(root string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runs, root)
	r.save()
}

func (r *runRegistry) put(t trackedRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[t.Root] = t
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
// agent is known to herdr and its worktree exists. It asks nothing, so a
// restart is invisible to running work. Handles that no longer match are left
// out of the registry.
func (s *Server) reclaimRuns() {
	for _, t := range s.savedRuns {
		if err := s.reclaim(t); err != nil {
			s.log.Info("not reclaiming iteration", "ticket", t.Address, "reason", err)
		}
	}
	s.savedRuns = nil
}

func (s *Server) reclaim(t trackedRun) error {
	agent, err := herdr.AgentGet(t.Address)
	if err != nil {
		return err
	}
	if agent.PaneID == "" {
		return errors.New("no pane")
	}
	one, wt, err := s.resume(t)
	if err != nil {
		return err
	}
	t.Pane, t.Tab = agent.PaneID, agent.TabID
	s.registry.put(t)
	s.events.publish(EventReclaimed, t.Address)
	go s.finishRun(ralphloop.DefaultDeps(), t.Root, one, wt, t.Run, t.Address)
	return nil
}

// resume rebuilds the iteration a persisted handle stands for.
func (s *Server) resume(t trackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, error) {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, err
	}
	dir, _, err := s.projectOf(addr.Project)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, err
	}
	epics, err := tickets.Load(dir)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, err
	}
	one := ralphloop.OneIteration{
		RepoDir: t.Repo, WorkspaceID: t.Workspace, Epic: addr.Epic, ScratchDir: s.cfg.TicketStore, Agent: ralphloop.AgentKind(t.Agent),
	}
	for _, e := range epics {
		for _, tk := range e.Tickets {
			if tk.Path == t.TicketPath {
				one.Ticket = tk
			}
		}
	}
	if one.Ticket.Path == "" {
		return one, ralphloop.IterationWorktree{}, errors.New("ticket gone")
	}
	wt, err := ralphloop.ResumeIteration(ralphloop.DefaultDeps(), one, t.Base)
	if err != nil {
		return one, wt, err
	}
	if _, err := os.Stat(wt.Path); err != nil {
		return one, wt, err
	}
	return one, wt, nil
}
