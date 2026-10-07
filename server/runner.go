package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	// EventTicketClaimed streams once the ticket file says claimed.
	EventTicketClaimed = "ticket-claimed"
	// EventIterationStarted streams once the agent has taken its prompt.
	EventIterationStarted = "iteration-started"
	// EventIterationLaunchFailed streams when the launch failed and the claim
	// was rolled back.
	EventIterationLaunchFailed = "iteration-launch-failed"
	// EventTicketDone streams once the ticket's commits are landed and the file says done.
	EventTicketDone = "ticket-done"
	// EventIterationParked streams when a finished iteration ended without landing
	// (needs-answer or zero commits).
	EventIterationParked = "iteration-parked"
	// EventTicketParked streams once a person's park is written to the ticket file.
	EventTicketParked = "ticket-parked"
	// EventTicketCancelled streams once per ticket a cancel wrote.
	EventTicketCancelled = "ticket-cancelled"
	// EventIterationFailed streams when finishing or landing errored; the ticket stays claimed.
	EventIterationFailed = "iteration-failed"
	// EventRootCompleted streams once every ticket of a queued root is done.
	EventRootCompleted = "root-completed"

	implementSkill = "gx-implement"
)

// VerdictClaimRereadMismatch is the explain verdict for a ticket whose file
// changed since the index saw it, so the last claim pass skipped it.
const VerdictClaimRereadMismatch = "claim re-read mismatch"

// refusals holds the tickets the last claim pass refused for a changed file,
// until a claim or a later pass that finds the file unchanged clears them.
type refusals struct {
	mu   sync.Mutex
	addr map[string]bool
}

func (r *refusals) set(addr string, refused bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.addr == nil {
		r.addr = map[string]bool{}
	}
	if refused {
		r.addr[addr] = true
	} else {
		delete(r.addr, addr)
	}
}

func (r *refusals) has(addr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.addr[addr]
}

// landGuard lets a stop wait for lands in flight and keeps new ones from
// starting: a land cut off halfway leaves a ticket the store cannot explain.
type landGuard struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// begin reports false once the server is stopping; the caller must then not land.
func (g *landGuard) begin() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}

func (g *landGuard) end() { g.wg.Done() }

// closeAndWait refuses new lands, then reports whether the running ones
// finished within timeout.
func (g *landGuard) closeAndWait(timeout time.Duration) bool {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	done := make(chan struct{})
	go func() { g.wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
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
		s.publishVerdictChanges()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.kick:
		}
	}
}

// claimNext backfills free slots in queue order: it claims and launches the
// frontier ticket of each queued root that has one and no iteration running,
// until the concurrency limit is reached. It does nothing while herdr is down:
// a claim without an agent behind it would only be rolled back.
func (s *Server) claimNext() {
	if s.herdr.isUnavailable() || s.pause.blocked() || s.budgetSoftReached(time.Now()) {
		return
	}
	limit := s.concurrencyLimit()
	running := s.registry.count()
	for _, item := range s.queued.list() {
		if running >= limit {
			return
		}
		launched, err := s.claimRoot(item)
		if err != nil {
			s.log.Warn("claim queued root", "root", item.Address, "err", err)
			continue
		}
		if launched {
			running++
		}
	}
}

func (s *Server) concurrencyLimit() int {
	if s.cfg.MaxConcurrentRoots > 0 {
		return s.cfg.MaxConcurrentRoots
	}
	return config.DefaultExecutionQueueConfig().MaxConcurrentEpics
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
	loaded, err := tickets.Load(projectDir)
	if err != nil {
		return false, err
	}
	for _, e := range tickets.ResolveCrossEpic(addr.Project, loaded) {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		frontier := ralphloop.Frontier(e)
		if len(frontier) == 0 {
			return false, nil
		}
		t := frontier[0]
		ticketAddr := tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}.String()
		// A missed watch event can leave the index behind the file; claiming
		// on that view could schedule the wrong thing, so wait for the index.
		onDisk, err := fileSum(t.Path)
		if err != nil {
			return false, err
		}
		if seen, _ := s.idx.sumOf(ticketAddr); seen != onDisk {
			s.refused.set(ticketAddr, true)
			return false, nil
		}
		s.refused.set(ticketAddr, false)
		return true, s.claimAndLaunch(root, addr, t, repo, ralphloop.AgentKind(item.Agent))
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
	rootBase, resolvedBase, leafBase, err := s.resolveBase(addr, t, repo)
	var ambiguous *tickets.AmbiguousBaseError
	if errors.As(err, &ambiguous) {
		reason := "Ambiguous base: choose which of " + strings.Join(ambiguous.Blockers, ", ") + " this ticket starts from, by setting base:."
		if perr := ralphloop.ParkAmbiguousBase(s.cfg.TicketStore, addr.Epic, t.Identifier, t.Path, reason); perr != nil {
			return fmt.Errorf("park %s: %w", ticketAddr, perr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("base for %s: %w", ticketAddr, err)
	}
	if err := ralphloop.ClaimWithBase(t.Path, resolvedBase); err != nil {
		return fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	s.events.publish(EventTicketClaimed, ticketAddr)

	one := ralphloop.OneIteration{
		RepoDir: repo, Epic: addr.Epic, ScratchDir: s.cfg.TicketStore, Agent: agent, Ticket: t, RootBase: rootBase, LeafBase: leafBase,
	}
	deps := ralphloop.DefaultDeps()
	run, wt, err := s.prepareAndLaunch(deps, &one, addr, ticketAddr)
	if err != nil {
		if rerr := ralphloop.SetStatus(t.Path, "open"); rerr != nil {
			err = errors.Join(err, fmt.Errorf("release claim: %w", rerr))
		}
		s.events.publish(EventIterationLaunchFailed, ticketAddr)
		return fmt.Errorf("launch %s: %w", ticketAddr, err)
	}
	s.registry.put(trackedRun{
		Run: run, Root: root, Repo: repo, Workspace: one.WorkspaceID, Base: wt.Base(), TicketPath: t.Path,
	})
	s.events.publish(EventIterationStarted, ticketAddr)
	go s.finishRun(deps, root, one, wt, run, ticketAddr)
	return nil
}

// resolveBase derives where t starts from. ref is the branch the root epic's
// feature branch is created from, with its "ref@sha" claim stamp; leaf is the
// unlanded sibling whose branch t starts from instead of the feature tip. A
// commitless ticket reads at the feature tip, so everything comes back empty.
func (s *Server) resolveBase(addr tickets.Address, t tickets.Ticket, repo string) (ref, stamp, leaf string, err error) {
	if t.Commitless {
		return "", "", "", nil
	}
	dir, _, err := s.projectOf(addr.Project)
	if err != nil {
		return "", "", "", err
	}
	epics, err := tickets.Load(dir)
	if err != nil {
		return "", "", "", err
	}
	if leaf, err = tickets.DeriveLeafBase(addr.Project, epics, addr.Epic, t); err != nil {
		return "", "", "", err
	}
	ref, err = tickets.DeriveRootBase(addr.Project, epics, addr.Epic, t)
	if err != nil {
		return "", "", "", err
	}
	if ref == "" {
		ref = git.RemoteDefaultBranch(repo)
	}
	sha, err := git.RevParse(repo, ref)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	// A feature branch that already exists is adopted, not recreated; its
	// merge-base stays the branch's true start even after ref moves on.
	switch _, err := git.RevParse(repo, "refs/heads/"+addr.Epic); {
	case err == nil:
		if sha, err = git.MergeBase(repo, "refs/heads/"+addr.Epic, ref); err != nil {
			return "", "", "", fmt.Errorf("merge-base of %s and %s: %w", addr.Epic, ref, err)
		}
	case !errors.Is(err, git.ErrRefNotFound):
		return "", "", "", fmt.Errorf("resolve %s: %w", addr.Epic, err)
	}
	return ref, ref + "@" + sha, leaf, nil
}

// prepareAndLaunch gives the ticket its own worktree, then launches the agent
// in it. A launch that fails takes the worktree back out so a retry starts clean.
func (s *Server) prepareAndLaunch(deps ralphloop.Deps, one *ralphloop.OneIteration, addr tickets.Address, ticketAddr string) (Run, ralphloop.IterationWorktree, error) {
	ws, err := herdr.EnsureWorkspace(addr.Epic, one.RepoDir)
	if err != nil {
		return Run{}, ralphloop.IterationWorktree{}, err
	}
	one.WorkspaceID = ws
	wt, err := ralphloop.PrepareIteration(deps, *one)
	if err != nil {
		return Run{}, ralphloop.IterationWorktree{}, err
	}
	run, err := s.launch(addr, ticketAddr, ws, wt.Path, one.Agent)
	if err != nil {
		if derr := ralphloop.DiscardIteration(deps, *one, wt); derr != nil {
			err = errors.Join(err, fmt.Errorf("discard worktree: %w", derr))
		}
		return Run{}, ralphloop.IterationWorktree{}, err
	}
	return run, wt, nil
}

// finishRun waits for the agent to settle, then lands (or parks) the ticket and
// moves the root on: the next frontier ticket, or the root's completion.
func (s *Server) finishRun(deps ralphloop.Deps, root string, one ralphloop.OneIteration, wt ralphloop.IterationWorktree, run Run, ticketAddr string) {
	defer s.kickRunner()
	defer s.registry.delete(root)
	_, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: run.Pane, Until: []string{"idle", "done"}})
	if err == nil {
		// A stop that began while the agent settled leaves it for the next server.
		if !s.lands.begin() {
			return
		}
		defer s.lands.end()
		err = ralphloop.FinishIteration(deps, one, wt, run.Pane, run.Tab)
	}
	if err != nil {
		s.log.Warn("finish iteration", "ticket", ticketAddr, "err", err)
		s.events.publish(EventIterationFailed, ticketAddr)
		return
	}
	t, err := schema.ParseTicket(one.Ticket.Path)
	if err != nil || t.Status != "done" {
		s.events.publish(EventIterationParked, ticketAddr)
		s.notifyPark(one, ticketAddr, string(t.Status), events.Kind(t.ParkKind))
		return
	}
	s.events.publish(EventTicketDone, ticketAddr)
	s.completeRootIfDone(root, one)
}

// notifyPark sends the one chat message for a park. Anything that is not an
// explicit needs-repair reads as a question for a person.
func (s *Server) notifyPark(one ralphloop.OneIteration, ticketAddr, status string, kind events.Kind) {
	addr, err := tickets.ParseAddress(ticketAddr, tickets.AddressContext{})
	if err != nil {
		return
	}
	if status != string(schema.StatusNeedsRepair) {
		status = string(schema.StatusNeedsAnswer)
	}
	if kind.CauseHerdr() && s.herdr.isUnavailable() {
		s.holdPark(addr.Project, addr.Epic, addr.ID, status)
		return
	}
	s.chat.Park(addr.Project, addr.Epic, one.Ticket.Path, addr.ID, status, "iteration ended without landing the ticket")
}

// completeRootIfDone dequeues the root once every ticket in its epic is done.
func (s *Server) completeRootIfDone(root string, one ralphloop.OneIteration) {
	project, _, _ := strings.Cut(root, ":")
	dir, _, err := s.projectOf(project)
	if err != nil {
		s.log.Warn("complete root", "root", root, "err", err)
		return
	}
	epics, err := tickets.Load(dir)
	if err != nil {
		s.log.Warn("complete root", "root", root, "err", err)
		return
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != one.Epic || !ralphloop.AllDone(e) {
			continue
		}
		for _, item := range s.queued.list() {
			if a, err := tickets.ParseAddress(item.Address, tickets.AddressContext{}); err == nil && a.Project+":"+a.Epic == root {
				_, _ = s.queueRemove(QueueRequest{Address: item.Address})
			}
		}
		s.events.publish(EventRootCompleted, root)
	}
}

func (s *Server) launch(addr tickets.Address, ticketAddr, ws, cwd string, agent ralphloop.AgentKind) (Run, error) {
	tab, err := herdr.TabCreate(herdr.TabCreateOptions{WorkspaceID: ws, Cwd: cwd, Label: ticketAddr})
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
