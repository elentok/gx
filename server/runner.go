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
	// EventRootParked streams when a finished root cannot fast-forward onto its target.
	EventRootParked = "root-parked"

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
		s.checkProjects()
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
// frontier tickets of each queued root, up to its per-root cap, until the concurrency limit is reached. It does nothing while herdr is down:
// a claim without an agent behind it would only be rolled back.
func (s *Server) claimNext() {
	if s.herdr.isUnavailable() || s.pause.blocked() || s.budgetSoftReached(time.Now()) {
		return
	}
	limit := s.concurrencyLimit()
	running := s.registry.count()
	seen := map[string]bool{}
	for _, item := range s.queued.list() {
		addr, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
		if err != nil || seen[rootOf(addr).String()] {
			continue
		}
		seen[rootOf(addr).String()] = true
		// One root fills its own slots before the next root in the queue gets a turn.
		for running < limit {
			launched, err := s.claimRoot(item)
			if err != nil {
				s.log.Warn("claim queued root", "root", item.Address, "err", err)
				break
			}
			if !launched {
				break
			}
			running++
		}
	}
}

// perRootLimit is how many agents one root may run at once.
func (s *Server) perRootLimit() int {
	if s.cfg.MaxAgentsPerRoot > 0 {
		return s.cfg.MaxAgentsPerRoot
	}
	return config.DefaultExecutionQueueConfig().MaxConcurrentTicketsPerEpic
}

func (s *Server) concurrencyLimit() int {
	if s.cfg.MaxAgents > 0 {
		return s.cfg.MaxAgents
	}
	return config.DefaultExecutionQueueConfig().MaxAgents
}

// claimRoot reports whether it launched an iteration for item's root. A claim
// that parked instead (ambiguous base) launched nothing, so it used no slot.
func (s *Server) claimRoot(item QueueItem) (bool, error) {
	addr, err := tickets.ParseAddress(item.Address, tickets.AddressContext{})
	if err != nil {
		return false, err
	}
	root := rootOf(addr)
	if s.registry.countRoot(root.String()) >= s.perRootLimit() {
		return false, nil
	}
	if _, missing := s.unavailablePath(addr.Project); missing {
		return false, nil
	}
	_, repo, err := s.projectOf(addr.Project)
	if err != nil {
		return false, err
	}
	// The epics of the last scan, not a fresh load: this runs every poll tick for
	// every queued root. The sum check below catches a file the scan has not seen.
	loaded, err := s.projectEpics(addr.Project)
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
		return s.claimAndLaunch(root, addr, t, repo, ralphloop.AgentKind(item.Agent), loaded)
	}
	return false, nil
}

// projectOf finds the project's directory in the store and the repo its
// agents run in.
func (s *Server) projectOf(project string) (dir, repo string, err error) {
	d, err := s.projectDir(project)
	if err != nil {
		return "", "", err
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

// chatOverride is the project's own notification block from project.json, nil
// when it has none (or is unreadable) so its notifications use the global one.
func (s *Server) chatOverride(project string) *ralphloop.ServerChatConfig {
	dir, err := s.projectDir(project)
	if err != nil {
		return nil
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.Notifications == nil {
		return nil
	}
	n := pf.Notifications
	return &ralphloop.ServerChatConfig{
		TelegramBotToken: n.Telegram.BotToken,
		TelegramChatID:   n.Telegram.ChatID,
		SlackWebhookURL:  n.Slack.WebhookURL,
	}
}

// projectDir finds the project's directory in the store.
func (s *Server) projectDir(project string) (string, error) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return "", err
	}
	for _, d := range dirs {
		if tickets.ProjectName(d) == project {
			return d, nil
		}
	}
	return "", fmt.Errorf("no project %s in the ticket store", project)
}

// claimAndLaunch writes the claim to the ticket file first, so markdown stays
// the truth and nothing streams before it is on disk. A failed launch parks the
// ticket needs-repair rather than leaving a claim with no agent behind it.
// launched is false when the ticket parked on an ambiguous base: no agent runs.
// epics is the project's loaded epics, for the base derivation.
func (s *Server) claimAndLaunch(root rootRef, queued tickets.Address, t tickets.Ticket, repo string, agent ralphloop.AgentKind, epics []tickets.Epic) (launched bool, err error) {
	ticket := tickets.Address{Project: queued.Project, Epic: queued.Epic, ID: t.Identifier}
	ticketAddr := ticket.String()
	rootBase, resolvedBase, leafBase, err := s.resolveBase(queued, t, repo, epics)
	var ambiguous *tickets.AmbiguousBaseError
	if errors.As(err, &ambiguous) {
		reason := "Ambiguous base: choose which of " + strings.Join(ambiguous.Blockers, ", ") + " this ticket starts from, by setting base:."
		if perr := s.parkTicket(s.cfg.TicketStore, ticket, t.Path, events.AmbiguousBase, reason); perr != nil {
			return false, fmt.Errorf("park %s: %w", ticketAddr, perr)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("base for %s: %w", ticketAddr, err)
	}
	if err := ralphloop.ClaimWithBase(t.Path, resolvedBase); err != nil {
		return false, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	s.events.publish(EventTicketClaimed, ticketAddr)

	one := ralphloop.OneIteration{
		RepoDir: repo, Epic: queued.Epic, ScratchDir: s.cfg.TicketStore, Agent: agent, Ticket: t, RootBase: rootBase, LeafBase: leafBase,
	}
	deps := ralphloop.DefaultDeps()
	run, wt, err := s.prepareAndLaunch(deps, &one, ticket)
	if err != nil {
		// Handing the claim back would make the next tick retry the same failure
		// forever, so a person is told instead.
		if perr := s.parkTicket(s.cfg.TicketStore, ticket, t.Path, events.IterationError, "launch failed: "+err.Error()); perr != nil {
			err = errors.Join(err, fmt.Errorf("park: %w", perr))
		}
		s.events.publish(EventIterationLaunchFailed, ticketAddr)
		return false, fmt.Errorf("launch %s: %w", ticketAddr, err)
	}
	s.registry.put(trackedRun{
		Run: run, Root: root.String(), Repo: repo, Workspace: one.WorkspaceID, Base: wt.Base(), TicketPath: t.Path, StartedAt: time.Now(),
	})
	s.events.publish(EventIterationStarted, ticketAddr)
	go s.finishRun(deps, root, one, wt, run, ticketAddr)
	return true, nil
}

// resolveBase derives where t starts from. ref is the branch the root epic's
// feature branch is created from, with its "ref@sha" claim stamp; leaf is the
// unlanded sibling whose branch t starts from instead of the feature tip. A
// commitless ticket reads at the feature tip, so everything comes back empty.
func (s *Server) resolveBase(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (ref, stamp, leaf string, err error) {
	if t.Commitless {
		return "", "", "", nil
	}
	if leaf, err = tickets.DeriveLeafBase(addr.Project, epics, addr.Epic, t); err != nil {
		return "", "", "", err
	}
	ref, err = rootBaseRef(addr.Project, epics, addr.Epic, t, repo)
	if err != nil {
		return "", "", "", err
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

// rootBaseRef is the branch a root's feature branch starts from and lands back onto.
func rootBaseRef(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (string, error) {
	ref, err := tickets.DeriveRootBase(project, epics, epic, t)
	if err != nil {
		return "", err
	}
	if ref == "" {
		ref = git.RemoteDefaultBranch(repo)
	}
	return ref, nil
}

// prepareAndLaunch gives the ticket its own worktree, then launches the agent
// in it. A launch that fails takes the worktree back out so a retry starts clean.
func (s *Server) prepareAndLaunch(deps ralphloop.Deps, one *ralphloop.OneIteration, ticket tickets.Address) (Run, ralphloop.IterationWorktree, error) {
	ws, err := herdr.EnsureWorkspace(ticket.Epic, one.RepoDir)
	if err != nil {
		return Run{}, ralphloop.IterationWorktree{}, err
	}
	one.WorkspaceID = ws
	wt, err := ralphloop.PrepareIteration(deps, *one)
	if err != nil {
		return Run{}, ralphloop.IterationWorktree{}, err
	}
	run, err := s.launch(ticket, ws, wt.Path, one.Agent)
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
func (s *Server) finishRun(deps ralphloop.Deps, root rootRef, one ralphloop.OneIteration, wt ralphloop.IterationWorktree, run Run, ticketAddr string) {
	defer s.kickRunner()
	defer s.registry.delete(ticketAddr)
	addr, _ := tickets.ParseAddress(ticketAddr, tickets.AddressContext{}) // built by claimAndLaunch, always parses
	err := ralphloop.WaitIterationFinished(deps, one, wt, run.Pane)
	var out ralphloop.FinishOutcome
	if err == nil {
		// A stop that began while the agent settled leaves it for the next server.
		if !s.lands.begin() {
			return
		}
		defer s.lands.end()
		out, err = ralphloop.FinishIteration(deps, one, wt, run.Pane, run.Tab)
	}
	if err != nil {
		s.log.Warn("finish iteration", "ticket", ticketAddr, "err", err)
		// Park it: a claimed ticket nobody is working on is stuck, not failed.
		if perr := s.parkTicket(s.cfg.TicketStore, addr, one.Ticket.Path, events.IterationError, err.Error()); perr != nil {
			s.log.Warn("park failed finish", "ticket", ticketAddr, "err", perr)
		}
		s.events.publish(EventIterationFailed, ticketAddr)
		return
	}
	if !out.Landed {
		s.events.publish(EventIterationParked, ticketAddr)
		s.notifyPark(addr, one.Ticket.Path, out.Kind, "iteration ended without landing the ticket")
		return
	}
	s.events.publish(EventTicketDone, ticketAddr)
	s.completeRootIfDone(root, one)
}

// completeRootIfDone lands the root's feature branch once every ticket in its
// epic is done, then dequeues it. A branch that needs a rebase parks the root
// instead: a person runs gx-merge.
func (s *Server) completeRootIfDone(root rootRef, one ralphloop.OneIteration) {
	project := root.Project
	dir, repo, err := s.projectOf(project)
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
		if reason, err := s.landRoot(project, epics, one, repo); err != nil {
			s.log.Warn("land root", "root", root, "err", err)
			s.events.publish(EventIterationFailed, root.String())
			return
		} else if reason != "" {
			s.events.publish(EventRootParked, root.String())
			s.log.Warn("root parked", "root", root, "reason", reason)
			if s.chat != nil {
				s.chat.Park(project, s.chatOverride(project), one.Epic, one.Ticket.Path, one.Ticket.Identifier, string(schema.StatusNeedsAnswer), reason)
			}
			return
		}
		if removed, err := s.queued.removeRoot(root); err != nil {
			s.log.Warn("dequeue completed root", "root", root, "err", err)
		} else if removed {
			s.events.publish(EventQueueChanged, root.String())
		}
		s.events.publish(EventRootCompleted, root.String())
	}
}

// landRoot fast-forwards the root's target to its feature branch through the
// merge core. A non-empty reason means the root must park, not land.
func (s *Server) landRoot(project string, epics []tickets.Epic, one ralphloop.OneIteration, repoDir string) (reason string, err error) {
	target, err := rootBaseRef(project, epics, one.Epic, one.Ticket, repoDir)
	if err != nil {
		return "", err
	}
	info, err := git.IdentifyDir(repoDir)
	if err != nil {
		return "", err
	}
	worktrees, err := git.ListWorktrees(info.Repo)
	if err != nil {
		return "", err
	}
	out, err := git.MergeBranchInto(info.Repo, worktrees, one.Epic, target)
	if err != nil {
		return "", err
	}
	if out.Merged {
		return "", nil
	}
	return fmt.Sprintf("needs rebase: %s onto %s; run gx-merge", out.Branch, out.Target), nil
}

func (s *Server) launch(ticket tickets.Address, ws, cwd string, agent ralphloop.AgentKind) (Run, error) {
	tab, err := herdr.TabCreate(herdr.TabCreateOptions{WorkspaceID: ws, Cwd: cwd, Label: ticket.String(), Env: s.cfg.TabEnv})
	if err != nil {
		return Run{}, err
	}
	// herdr rejects a ticket address as an agent name; every lookup uses the iteration label.
	label, _, _ := ralphloop.IterationIdentity(ticket.Epic, ticket.ID, "")
	if _, err := herdr.AgentStart(herdr.AgentStartOptions{
		Name:      label,
		Kind:      string(agent),
		Pane:      tab.RootPaneID,
		AgentArgs: ralphloop.AgentArgs(agent, s.cfg.TicketStore, ticket.Epic, "", ""),
	}); err != nil {
		return Run{}, err
	}
	if _, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: tab.RootPaneID, Until: []string{"idle"}}); err != nil {
		return Run{}, err
	}
	if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{
		Target: tab.RootPaneID,
		Text:   ralphloop.SkillPrompt(agent, implementSkill, ticket.String()),
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return Run{}, err
	}
	return Run{Address: ticket.String(), Agent: string(agent), Pane: tab.RootPaneID, Tab: tab.TabID}, nil
}
