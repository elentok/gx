package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

const (
	// EventTicketClaimed streams once the ticket file says claimed.
	EventTicketClaimed = "ticket-claimed"
	// EventIterationStarted streams once the agent has taken its prompt.
	EventIterationStarted = "iteration-started"
	// EventIterationLaunchFailed streams when the launch failed and the claim
	// was rolled back.
	EventIterationLaunchFailed = "iteration-launch-failed"

	implementSkill = "gx-implement"
)

// Run is one launched iteration in the server's run registry.
type Run struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
	Pane    string `json:"pane"`
	Tab     string `json:"tab"`
}

// runRegistry is the server's own record of launched iterations, keyed by
// project:epic (one running iteration per root). Independent of the TUI's.
type runRegistry struct {
	mu   sync.Mutex
	runs map[string]Run
}

func (r *runRegistry) has(root string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[root]
	return ok
}

func (r *runRegistry) put(root string, run Run) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs == nil {
		r.runs = map[string]Run{}
	}
	r.runs[root] = run
}

// Runs lists the registry, for tests and later reads.
func (s *Server) Runs() []Run {
	s.registry.mu.Lock()
	defer s.registry.mu.Unlock()
	out := make([]Run, 0, len(s.registry.runs))
	for _, r := range s.registry.runs {
		out = append(out, r)
	}
	return out
}

// kickRunner asks the runner to look at the queue now instead of at its next tick.
func (s *Server) kickRunner() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// keepClaiming launches queued roots while the switch says "server". The tick
// is the guarantee; the kick from a queue write only makes it sooner.
func (s *Server) keepClaiming(ctx context.Context) {
	if s.cfg.Orchestrator != config.OrchestratorServer {
		return
	}
	poll := s.cfg.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		s.claimNext()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.kick:
		}
	}
}

// claimNext claims and launches the frontier ticket of the first queued root
// that has one and no iteration running. It does nothing while herdr is down:
// a claim without an agent behind it would only be rolled back.
func (s *Server) claimNext() {
	if s.herdr.isUnavailable() {
		return
	}
	for _, item := range s.queued.list() {
		launched, err := s.claimRoot(item)
		if err != nil {
			s.log.Warn("claim queued root", "root", item.Address, "err", err)
			continue
		}
		if launched {
			return
		}
	}
}

// claimRoot reports whether it launched an iteration for item's root.
func (s *Server) claimRoot(item QueueItem) (bool, error) {
	addr, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
	if err != nil {
		return false, err
	}
	root := addr.Project + ":" + addr.Epic
	if s.registry.has(root) {
		return false, nil
	}
	projectDir, repo, err := s.projectOf(addr.Project)
	if err != nil {
		return false, err
	}
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return false, err
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		frontier := ralphloop.Frontier(e)
		if len(frontier) == 0 {
			return false, nil
		}
		return true, s.claimAndLaunch(root, addr, frontier[0], repo, ralphloop.AgentKind(item.Agent))
	}
	return false, nil
}

// projectOf finds the project's directory in the store and the repo its
// agents run in.
func (s *Server) projectOf(project string) (dir, repo string, err error) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return "", "", err
	}
	for _, d := range dirs {
		if tickets.ProjectName(d) != project {
			continue
		}
		pf, err := config.ReadProjectFile(d)
		if err != nil {
			return "", "", err
		}
		if pf.Repo == nil {
			return "", "", fmt.Errorf("project %s has no repo", project)
		}
		return d, *pf.Repo, nil
	}
	return "", "", fmt.Errorf("no project %s in the ticket store", project)
}

// claimAndLaunch writes the claim to the ticket file first, so markdown stays
// the truth and nothing streams before it is on disk. A failed launch gives
// the ticket back rather than leaving a claim with no agent behind it.
func (s *Server) claimAndLaunch(root string, addr tickets.Address, t tickets.Ticket, repo string, agent ralphloop.AgentKind) error {
	ticketAddr := tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}.String()
	if err := ralphloop.Claim(t.Path); err != nil {
		return fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	s.events.publish(EventTicketClaimed, ticketAddr)

	run, err := s.launch(addr, ticketAddr, repo, agent)
	if err != nil {
		if rerr := ralphloop.SetStatus(t.Path, "open"); rerr != nil {
			err = errors.Join(err, fmt.Errorf("release claim: %w", rerr))
		}
		s.events.publish(EventIterationLaunchFailed, ticketAddr)
		return fmt.Errorf("launch %s: %w", ticketAddr, err)
	}
	s.registry.put(root, run)
	s.events.publish(EventIterationStarted, ticketAddr)
	return nil
}

func (s *Server) launch(addr tickets.Address, ticketAddr, repo string, agent ralphloop.AgentKind) (Run, error) {
	ws, err := herdr.EnsureWorkspace(addr.Epic, repo)
	if err != nil {
		return Run{}, err
	}
	tab, err := herdr.TabCreate(herdr.TabCreateOptions{WorkspaceID: ws, Cwd: repo, Label: ticketAddr})
	if err != nil {
		return Run{}, err
	}
	if _, err := herdr.AgentStart(herdr.AgentStartOptions{
		Name:      ticketAddr,
		Kind:      string(agent),
		Pane:      tab.RootPaneID,
		AgentArgs: ralphloop.AgentArgs(agent, s.cfg.TicketStore, addr.Epic, "", ""),
	}); err != nil {
		return Run{}, err
	}
	if _, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: tab.RootPaneID, Until: []string{"idle"}}); err != nil {
		return Run{}, err
	}
	if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{
		Target: tab.RootPaneID,
		Text:   ralphloop.SkillPrompt(agent, implementSkill, ticketAddr),
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return Run{}, err
	}
	return Run{Address: ticketAddr, Agent: string(agent), Pane: tab.RootPaneID, Tab: tab.TabID}, nil
}
