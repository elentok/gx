package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/elentok/gx/events"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
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
	// Runner names the agentrunner hosting Session, e.g. "herdr".
	Runner  string              `json:"runner"`
	Session agentrunner.Session `json:"session"`
	// Tab is the herdr tab herdr-only cleanup closes; empty for other runners.
	Tab string `json:"tab,omitempty"`
}

// runnerHerdr is the Runner of every launch until the server launches
// through agentrunner, and of any run an older server saved.
const runnerHerdr = "herdr"

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
	released map[string]bool
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

func (r *runRegistry) has(address string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[address]
	return ok
}

// countRoot is how many iterations of root's tickets are running.
func (r *runRegistry) countRoot(root string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, t := range r.runs {
		if t.Root == root {
			n++
		}
	}
	return n
}

// countProject is how many iterations of the project's tickets are running.
func (r *runRegistry) countProject(project string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, t := range r.runs {
		if strings.HasPrefix(t.Root, project+":") {
			n++
		}
	}
	return n
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

func (r *runRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

func (r *runRegistry) tracked(address string) (trackedRun, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.runs[address]
	return t, ok
}

func (r *runRegistry) releaseGate(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released == nil {
		r.released = map[string]bool{}
	}
	r.released[address] = true
}

// gateReleased is the Deps.GateReleased hook of the run at address.
func (r *runRegistry) gateReleased(address string) func() bool {
	return func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.released[address]
	}
}

func (r *runRegistry) delete(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runs, address)
	delete(r.released, address)
	r.save()
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
// agent is known to herdr and its worktree exists. It asks nothing, so a
// restart is invisible to running work. Handles that no longer match are left
// out of the registry.
func (s *Server) reclaimRuns() {
	for _, t := range s.savedRuns {
		err := s.reclaim(t)
		if err == nil {
			continue
		}
		s.log.Info("not reclaiming iteration", "ticket", t.Address, "reason", err)
		if errors.Is(err, errHandleMismatch) {
			s.parkMismatch(t, err)
		}
	}
	s.savedRuns = nil
}

// errHandleMismatch marks a persisted handle whose pane, agent or worktree is gone.
var errHandleMismatch = errors.New("handle mismatch")

// parkMismatch parks the ticket of a handle that cannot be reclaimed, so a
// person sees it instead of it staying claimed forever.
func (s *Server) parkMismatch(t trackedRun, cause error) {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err == nil {
		var dir string
		if dir, _, err = s.projectOf(addr.Project); err == nil {
			err = s.parkTicket(dir, addr, t.TicketPath, events.HandleMismatch, "cannot reclaim iteration after restart: "+cause.Error())
		}
	}
	if err != nil {
		s.log.Warn("park handle mismatch", "ticket", t.Address, "err", err)
	}
}

func (s *Server) reclaim(t trackedRun) error {
	label := iterationLabel(t.Address)
	agent, err := herdr.AgentGet(label)
	if err != nil {
		return fmt.Errorf("%w: agent %s: %v", errHandleMismatch, t.Address, err)
	}
	if agent.PaneID == "" {
		return fmt.Errorf("%w: no pane", errHandleMismatch)
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
	t.Session = agentrunner.Session{Label: label, ID: agent.PaneID, SessionID: t.Session.SessionID}
	t.Tab = agent.TabID
	s.registry.put(t)
	s.events.publish(EventReclaimed, t.Address)
	deps := ralphloop.DefaultDeps()
	deps.GateReleased = s.registry.gateReleased(t.Address)
	go s.finishRun(deps, root, mode, one, wt, t.Run, t.Address)
	return nil
}

// resume rebuilds the iteration a persisted handle stands for.
func (s *Server) resume(t trackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, iterationMode, error) {
	addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{})
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, iterationMode{}, err
	}
	dir, _, err := s.projectOf(addr.Project)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, iterationMode{}, err
	}
	epics, err := tickets.Load(dir)
	if err != nil {
		return ralphloop.OneIteration{}, ralphloop.IterationWorktree{}, iterationMode{}, err
	}
	one := ralphloop.OneIteration{
		RepoDir: t.Repo, WorkspaceID: t.Workspace, Epic: addr.Epic, ScratchDir: dir, Agent: ralphloop.AgentKind(t.Agent),
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
	mode := s.iterationModeFor(addr, one.Ticket)
	wt, err := mode.resume(ralphloop.DefaultDeps(), one, t.Base)
	if err != nil {
		return one, wt, mode, err
	}
	if _, err := os.Stat(wt.Path); err != nil {
		return one, wt, mode, fmt.Errorf("%w: worktree: %v", errHandleMismatch, err)
	}
	return one, wt, mode, nil
}
