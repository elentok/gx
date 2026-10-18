package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
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

	implementSkill   = "gx-implement"
	codeReviewSkill  = "gx-code-review"
	oneOffSkill      = "gx-one-off"
	evolveSkill      = "gx-evolve"
	investigateSkill = "gx-investigate"
	// mergeSkill carries its flag: the prompt appends the ticket address to it.
	mergeSkill = "gx-merge --ticket"
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

// trunkLocks serializes root lands per repo and trunk. Unserialized, two
// lands into one trunk race on git's own locks and the second fails instead
// of seeing a clean non-ff.
type trunkLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (l *trunkLocks) lock(repo, trunk string) (unlock func()) {
	m := l.mutex(repo, trunk)
	m.Lock()
	return m.Unlock
}

func (l *trunkLocks) mutex(repo, trunk string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := repo + "\x00" + trunk
	m := l.locks[key]
	if m == nil {
		if l.locks == nil {
			l.locks = map[string]*sync.Mutex{}
		}
		m = &sync.Mutex{}
		l.locks[key] = m
	}
	return m
}

// tryLock is lock without the wait: ok is false when the trunk is held.
func (l *trunkLocks) tryLock(repo, trunk string) (unlock func(), ok bool) {
	m := l.mutex(repo, trunk)
	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}

// goSettle runs f, a finishRun, settleRun or recoverFrom, on its own
// goroutine. Each writes to the ticket store and may outlive Serve's stop.
func (s *Server) goSettle(f func()) {
	s.settling.Add(1)
	go func() {
		defer s.settling.Done()
		f()
	}()
}

// WaitSettled waits for every goSettle goroutine and reports
// whether they all ended within timeout. Serve's stop leaves them running
// (agents outlive the server), so a test must wait before deleting their dirs.
func (s *Server) WaitSettled(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() { s.settling.Wait(); close(done) }()
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

// keepClaiming launches queued roots. The tick is the guarantee; the kick from a queue write only makes it sooner.
func (s *Server) keepClaiming(ctx context.Context) {
	poll := s.cfg.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		s.checkProjects()
		s.checkTrunks()
		s.stampHandLanded()
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
	if n := s.settings.Load().MaxAgentsPerRoot; n > 0 {
		return n
	}
	return config.DefaultExecutionQueueConfig().MaxConcurrentTicketsPerEpic
}

// projectAtCap reports whether the project's own max-agents cap is reached. The
// cap is read from project.json on every call, so an edit applies at the next
// decision; an absent, unreadable or non-positive cap means no project cap.
func (s *Server) projectAtCap(project string) (running, limit int, atCap bool) {
	dir, err := s.projectDir(project)
	if err != nil {
		return 0, 0, false
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.MaxAgents == nil || *pf.MaxAgents <= 0 {
		return 0, 0, false
	}
	running, limit = s.registry.countProject(project), *pf.MaxAgents
	return running, limit, running >= limit
}

func (s *Server) concurrencyLimit() int {
	if n := s.settings.Load().MaxAgents; n > 0 {
		return n
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
	if _, _, atCap := s.projectAtCap(addr.Project); atCap {
		return false, nil
	}
	if _, missing := s.unavailablePath(addr.Project); missing {
		return false, nil
	}
	if _, missing := s.trunkMissing.get(addr.Project); missing {
		return false, nil
	}
	projectDir, repo, err := s.projectOf(addr.Project)
	if err != nil {
		return false, err
	}
	// The epics of the last scan, not a fresh load: this runs every poll tick for
	// every queued root. The sum check below catches a file the scan has not seen.
	loaded, err := s.projectEpics(addr.Project)
	if err != nil {
		return false, err
	}
	for _, e := range loaded {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		s.checkDeadlock(root, projectDir, e)
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
	// The project dir, not the store root: the run log, land lock and
	// conflict-resolution tickets all live under <project>/<epic>.
	dir, err := s.projectDir(queued.Project)
	if err != nil {
		return false, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	rootBase, resolvedBase, leafBase, err := s.resolveBase(queued, t, repo, epics)
	var ambiguous *tickets.AmbiguousBaseError
	if errors.As(err, &ambiguous) {
		reason := "Ambiguous base: choose which of " + strings.Join(ambiguous.Blockers, ", ") + " this ticket starts from, by setting base:."
		if perr := s.parkTicket(dir, ticket, t.Path, events.AmbiguousBase, reason, Run{}); perr != nil {
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

	model, effort := s.runAgentConfig(queued.Project, agent, Run{})
	one := ralphloop.OneIteration{
		RepoDir: repo, Epic: queued.Epic, ScratchDir: dir, Agent: agent, Ticket: t, RootBase: rootBase, LeafBase: leafBase,
		Model: model, Effort: effort, Sink: s.finishSink(ticket, t.Path),
	}
	deps := s.depsFor(ticket.Project)
	deps.GateReleased = s.registry.gateReleased(ticketAddr)
	mode := s.iterationModeFor(ticket, t)
	if mode.commitless && mode.scratchDir == "" {
		if mode.ref, err = s.commitlessRef(ticket, t, repo, epics); err != nil {
			return false, fmt.Errorf("base for %s: %w", ticketAddr, err)
		}
	}
	entry := trackedRun{
		Run:  Run{Address: ticketAddr, Agent: string(agent), Runner: deps.Runner.Name(), Model: one.Model, Effort: one.Effort},
		Root: root.String(), Repo: repo, TicketPath: t.Path, StartedAt: time.Now(), State: runLaunching,
	}
	s.registry.put(entry)
	run, wt, err := s.prepareAndLaunch(deps, &one, ticket, mode, func(wt ralphloop.IterationWorktree) {
		entry.Workspace, entry.Base = one.WorkspaceID, wt.Base()
		s.registry.put(entry)
	})
	if err != nil {
		s.registry.delete(ticketAddr)
		// Handing the claim back would make the next tick retry the same failure
		// forever, so a person is told instead.
		kind, reason := events.IterationError, "launch failed: "+err.Error()
		if taken := (*nameTakenError)(nil); errors.As(err, &taken) {
			kind, reason = events.AgentNameTaken, "agent name taken: "+taken.reason
		}
		if perr := s.parkTicket(dir, ticket, t.Path, kind, reason, entry.Run); perr != nil {
			err = errors.Join(err, fmt.Errorf("park: %w", perr))
		}
		s.events.publish(EventIterationLaunchFailed, ticketAddr)
		return false, fmt.Errorf("launch %s: %w", ticketAddr, err)
	}
	s.registry.put(trackedRun{
		Run: run, Root: root.String(), Repo: repo, Workspace: one.WorkspaceID, Base: wt.Base(), TicketPath: t.Path, StartedAt: entry.StartedAt,
	})
	s.events.publish(EventIterationStarted, ticketAddr)
	s.goSettle(func() { s.finishRun(runContext{deps, root, mode, one, wt, run, ticketAddr}) })
	return true, nil
}

// resolveBase derives where t starts from. ref is the branch the root epic's
// feature branch is created from (the epic's refs, not t's), with its
// "ref@sha" claim stamp; leaf is the
// unlanded sibling whose branch t starts from instead of the feature tip. A
// commitless ticket reads at the feature tip, so everything comes back empty.
func (s *Server) resolveBase(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (ref, stamp, leaf string, err error) {
	if t.Commitless {
		return "", "", "", nil
	}
	if leaf, err = tickets.DeriveLeafBase(addr.Project, epics, addr.Epic, t); err != nil {
		return "", "", "", err
	}
	_, ref, _, err = s.epicRefs(addr.Project, epics, addr.Epic, repo)
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

// landingOnLanded reports whether the project uses the on-landed landing policy.
// An unreadable project file falls back to the default policy.
func (s *Server) landingOnLanded(project string) bool {
	dir, err := s.projectDir(project)
	if err != nil {
		return true
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.LandingPolicy() == config.LandingOnLanded
}

// autoFFMerge reports whether the project opted in to automatic ff-only merges,
// read live. An unreadable project file means off: never merge on a guess.
func (s *Server) autoFFMerge(project string) bool {
	dir, err := s.projectDir(project)
	if err != nil {
		return false
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.AutoFFMergeEnabled()
}

// rootRefs is where a root's feature branch starts (base: the claim base and
// the commitless ref) and where it lands (target). Trunk is read on every
// call, so a landing uses the trunk current then; nothing records it at claim.
// An explicit base: moves only the start; a derived stacked root also lands
// on its blocker's branch until that branch reaches trunk.
func (s *Server) rootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error) {
	dir, err := s.projectDir(project)
	if err != nil {
		return "", "", err
	}
	trunk, trunkErr := tickets.Trunk(dir, repo)
	derived, err := tickets.DeriveRootBase(project, epics, epic, t, s.landingOnLanded(project))
	if err != nil {
		return "", "", err
	}
	targetsTrunk := derived == "" || explicitBase(epics, epic, t)
	switch {
	case targetsTrunk && trunkErr != nil:
		return "", "", trunkErr
	case derived == "":
		return trunk, trunk, nil
	case targetsTrunk:
		return derived, trunk, nil
	case trunkErr != nil:
		// Only a root targeting trunk needs it; a blocker's branch can't have
		// landed on a trunk that isn't there.
		return derived, derived, nil
	}
	ref := retargetIfLanded(derived, trunk, func(a, d string) (bool, error) { return gitIsAncestor(repo, a, d) })
	return ref, ref, nil
}

// epicRefs is rootRefs for epic's Root ticket, the one ticket every caller
// that means "the epic's refs" agrees on, so they all lock the same key. A
// non-root ticket's base: never moves where the epic starts or lands.
func (s *Server) epicRefs(project string, epics []tickets.Epic, epic, repo string) (root tickets.Ticket, base, target string, err error) {
	if i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == epic }); i >= 0 && len(epics[i].Tickets) > 0 {
		root = lowestTicket(epics[i])
	}
	base, target, err = s.rootRefs(project, epics, epic, root, repo)
	return root, base, target, err
}

// landTarget is the target of the root epic addr belongs to.
func (s *Server) landTarget(addr tickets.Address) (string, error) {
	dir, repo, err := s.projectOf(addr.Project)
	if err != nil {
		return "", err
	}
	epics, err := tickets.LoadResolved(addr.Project, dir)
	if err != nil {
		return "", err
	}
	_, _, target, err := s.epicRefs(addr.Project, epics, addr.Epic, repo)
	return target, err
}

// explicitBase reports whether t's base comes from a base: field (the
// ticket's or its epic's) rather than from its blockers.
func explicitBase(epics []tickets.Epic, epic string, t tickets.Ticket) bool {
	if t.Base != "" {
		return true
	}
	for _, e := range epics {
		if e.Name == epic {
			return e.Base != ""
		}
	}
	return false
}

// retargetIfLanded is derived, never stored: once the blocker's branch is
// reachable from trunk the dependent targets trunk again.
// An unresolvable branch counts as not landed; never retarget on a guess.
func retargetIfLanded(ref, trunk string, isAncestor func(ancestor, descendant string) (bool, error)) string {
	if landed, err := isAncestor(ref, trunk); err == nil && landed {
		return trunk
	}
	return ref
}

// commitlessRef is the ref a commitless ticket's detached worktree starts at:
// where its blockers resolve to.
func (s *Server) commitlessRef(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (string, error) {
	if ref, ok := investigateRef(repo, addr, t, epics); ok && t.Type == string(schema.TypeInvestigate) {
		return ref, nil
	}
	ref, _, err := s.rootRefs(addr.Project, epics, addr.Epic, t, repo)
	return ref, err
}

// prepareAndLaunch gives the ticket somewhere to run, then launches the agent
// in it. A launch that fails takes that back out so a retry starts clean.
// prepared sees the worktree before the agent starts.
func (s *Server) prepareAndLaunch(
	deps ralphloop.Deps, one *ralphloop.OneIteration, ticket tickets.Address, mode iterationMode,
	prepared func(ralphloop.IterationWorktree),
) (Run, ralphloop.IterationWorktree, error) {
	wt, err := mode.prepare(deps, *one)
	if err != nil {
		return Run{}, ralphloop.IterationWorktree{}, err
	}
	prepared(wt)
	run, err := s.launch(deps, ticket, *one, wt.Path)
	if err != nil {
		if derr := mode.discard(deps, *one, wt); derr != nil {
			err = errors.Join(err, fmt.Errorf("discard worktree: %w", derr))
		}
		return Run{}, ralphloop.IterationWorktree{}, err
	}
	return run, wt, nil
}

// finishRun waits for the agent to settle, then lands (or parks) the ticket and
// moves the root on: the next frontier ticket, or the root's completion.
func (s *Server) finishRun(rc runContext) {
	s.settleRun(rc, ralphloop.WaitIterationFinished(rc.deps, rc.one, rc.wt, rc.run.Session))
}

// runContext is everything settling one claimed iteration needs.
type runContext struct {
	deps       ralphloop.Deps
	root       rootRef
	mode       iterationMode
	one        ralphloop.OneIteration
	wt         ralphloop.IterationWorktree
	run        Run
	ticketAddr string
}

// settleRun lands (or parks) an iteration whose wait ended with waitErr. A run
// found already finished at reattach skips the wait and comes straight here.
func (s *Server) settleRun(rc runContext, err error) {
	deps, root, mode, one, wt, run, ticketAddr := rc.deps, rc.root, rc.mode, rc.one, rc.wt, rc.run, rc.ticketAddr
	defer s.kickRunner()
	keepRun := false
	defer func() {
		if !keepRun {
			s.registry.delete(ticketAddr)
		}
	}()
	// A run the budget hard limit is stopping never finishes, lands or parks
	// for its own reason: the first of it and the kill parks it budget-killed.
	if stopping, first := s.registry.claimBudgetStop(ticketAddr); stopping {
		if t, ok := s.registry.tracked(ticketAddr); ok && first {
			if perr := s.parkBudgetKilled(t); perr != nil {
				s.log.Error("budget kill", "ticket", ticketAddr, "err", perr)
			}
		}
		return
	}
	addr, _ := tickets.ParseAddress(ticketAddr, tickets.AddressContext{}) // built by claimAndLaunch, always parses
	if s.parkReleasedGate(one.ScratchDir, addr, one.Ticket.Path, run) {
		return
	}
	var out ralphloop.FinishOutcome
	if err == nil {
		// A stop that began while the agent settled leaves it for the next
		// server, which reclaims it from the saved registry: keep it there.
		if !s.lands.begin() {
			keepRun = true
			return
		}
		defer s.lands.end()
		one.Model, one.Effort = s.runAgentConfig(addr.Project, one.Agent, run)
		out, err = mode.finish(deps, one, wt, run.Session.ID)
	}
	if err != nil {
		s.log.Warn("finish iteration", "ticket", ticketAddr, "err", err)
		// Park it: a claimed ticket nobody is working on is stuck, not failed.
		if perr := s.parkTicket(one.ScratchDir, addr, one.Ticket.Path, events.IterationError, err.Error(), run); perr != nil {
			s.log.Warn("park failed finish", "ticket", ticketAddr, "err", perr)
		}
		s.events.publish(EventIterationFailed, ticketAddr)
		return
	}
	if !out.Landed {
		// The finish path already wrote the park; tell a person its own reason.
		s.events.publish(EventIterationParked, ticketAddr)
		if !s.holdParkForRecovery(addr, one.Ticket.Path, out.Kind, out.Reason) {
			s.notifyPark(addr, one.Ticket.Path, out.Kind, out.Reason)
		}
		return
	}
	// Stop already cleans an adopted session; a run found finished at reattach
	// was never adopted, so its I/O files are removed here.
	if cerr := agentrunner.Cleanup(deps.Runner, run.Session.Label); cerr != nil {
		s.log.Warn("clean up agent", "ticket", ticketAddr, "err", cerr)
	}
	s.events.publish(EventTicketDone, ticketAddr)
	s.stampRetro(addr, one.Ticket.Path, false)
	s.notifyResult(addr, one.Ticket.Path)
	s.notifyDone(addr, one.Ticket.Path)
	s.fileFollowUp(addr, one.Ticket.Path)
	s.completeRootIfDone(root, one)
}

// notifyDone tells chat a ticket landed. A one-off stays silent: it speaks
// only through notifyResult, when its submit asked for it.
func (s *Server) notifyDone(addr tickets.Address, ticketPath string) {
	if !s.chat.Enabled() || isOneOffTicket(ticketPath) {
		return
	}
	if e, t, ok := s.freshTicket(addr); ok && !IsOneOffEpic(e) {
		s.chat.TicketDone(addr.Project, s.chatOverride(addr.Project), addr.Epic, t, ralphloop.CountsOf(e))
	}
}

// runnerFor is the configured runner for project: the one new launches use.
func (s *Server) runnerFor(project string) agentrunner.Runner {
	return s.runnerNamed(s.cfg.Runner.Name(), project)
}

// runnerForRun is the runner that started run, whatever the configured one is.
// It is nil when that runner cannot be built.
func (s *Server) runnerForRun(run Run) agentrunner.Runner {
	name := run.Runner
	if name == "" {
		name = runnerHerdr
	}
	project := ""
	if addr, err := tickets.ParseAddress(run.Address, tickets.AddressContext{}); err == nil {
		project = addr.Project
	}
	return s.runnerNamed(name, project)
}

// runnerOf is runnerForRun, failing when the run's runner cannot be built.
func (s *Server) runnerOf(run Run) (agentrunner.Runner, error) {
	if r := s.runnerForRun(run); r != nil {
		return r, nil
	}
	name := run.Runner
	if name == "" {
		name = runnerHerdr
	}
	return nil, fmt.Errorf("started by %s; %s unavailable", name, name)
}

// liveSession is the session of address's live iteration and the runner
// hosting it: a tracked run's own runner, else the configured one (a parked
// ticket's agent can outlive its run).
func (s *Server) liveSession(addr tickets.Address) (agentrunner.Runner, agentrunner.Session, bool, error) {
	runner, err := s.iterationRunner(addr)
	if err != nil {
		return nil, agentrunner.Session{}, false, err
	}
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	sess, ok, err := runner.Find(label)
	return runner, sess, ok, err
}

// iterationRunner is the runner hosting address's iteration: a tracked run's
// own runner, else the configured one.
func (s *Server) iterationRunner(addr tickets.Address) (agentrunner.Runner, error) {
	if run, ok := s.registry.runOf(addr.String()); ok {
		return s.runnerOf(run)
	}
	return s.runnerFor(addr.Project), nil
}

func (s *Server) runnerNamed(name, project string) agentrunner.Runner {
	if s.cfg.RunnerFor != nil {
		if r := s.cfg.RunnerFor(name, project); r != nil {
			return r
		}
	}
	if name == s.cfg.Runner.Name() {
		return s.cfg.Runner
	}
	return nil
}

// depsFor is the default deps with the project's runner, so launch, wait and
// finish all talk to the runner that hosts the agent.
func (s *Server) depsFor(project string) ralphloop.Deps {
	return s.depsWith(s.runnerFor(project))
}

// depsWith is the default deps talking to runner.
func (s *Server) depsWith(runner agentrunner.Runner) ralphloop.Deps {
	deps := ralphloop.DefaultDeps()
	deps.Runner = runner
	if s.cfg.Sleep != nil {
		deps.Sleep = s.cfg.Sleep
	}
	return deps
}
