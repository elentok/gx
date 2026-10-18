package server

import (
	"cmp"
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

	implementSkill   = "gx-implement"
	codeReviewSkill  = "gx-code-review"
	oneOffSkill      = "gx-one-off"
	evolveSkill      = "gx-evolve"
	investigateSkill = "gx-investigate"
	// mergeSkill carries its flag: the prompt appends the ticket address to it.
	mergeSkill = "gx-merge --ticket"
)

// launchSkill is the skill an agent starts under: code-review tickets get their
// own, since gx-implement would ask them for commits, prompt tickets (the
// one-offs) get one that keeps the TDD rules out of the way, and investigate
// and diagnose tickets run gx-investigate's unattended mode.
func launchSkill(t tickets.Ticket) string {
	switch {
	case t.IsCodeReview():
		return codeReviewSkill
	case t.Type == string(schema.TypePrompt):
		return oneOffSkill
	case t.Type == string(schema.TypeEvolve):
		return evolveSkill
	case t.Type == string(schema.TypeInvestigate), t.Type == string(schema.TypeDiagnose):
		return investigateSkill
	case t.Type == string(schema.TypeMerge):
		return mergeSkill
	}
	return implementSkill
}

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
	ref := retargetIfLanded(derived, trunk, func(a, d string) (bool, error) { return git.IsAncestor(repo, a, d) })
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

// completeRootIfDone lands the root's feature branch once every ticket in its
// epic is done, then dequeues it. A branch that needs a rebase gets a merge
// ticket instead, or parks the root at the cap. With the project's auto-ff-merge off,
// and no manual merge ticket in the epic, the
// branch is left alone: the root is dequeued as complete and a person runs gx-merge.
// completed_at marks the epic's final outcome: with auto-ff-merge on that is
// the land or the park, not all-done. An epic with nothing to land is merged
// as it stands, whatever auto-ff-merge says.
func (s *Server) completeRootIfDone(root rootRef, one ralphloop.OneIteration) {
	project := root.Project
	dir, repo, err := s.projectOf(project)
	if err != nil {
		s.log.Warn("complete root", "root", root, "err", err)
		return
	}
	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		s.log.Warn("complete root", "root", root, "err", err)
		return
	}
	for _, e := range epics {
		if filepath.Base(e.Path) == one.Epic && ralphloop.AllDone(e) {
			s.completeEpic(root, one, dir, repo, epics, e)
		}
	}
}

// completeEpic is completeRootIfDone for the all-done epic e. The land decides
// under its trunk's lock, so a second land into that trunk sees the first one's
// result; notices and the retro go after unlock, so claiming elsewhere never
// waits on them.
//
// Without refs there is no target, so the land locks the epic itself: landEpic
// parks it the way landRoot does, and two finishes still can't both stamp.
func (s *Server) completeEpic(root rootRef, one ralphloop.OneIteration, dir, repo string, epics []tickets.Epic, e tickets.Epic) {
	_, _, target, err := s.epicRefs(root.Project, epics, one.Epic, repo)
	lockKey := target
	if err != nil {
		lockKey = "epic:" + root.String()
	}
	unlock := s.trunkLocks.lock(repo, lockKey)
	d := s.landEpic(root, one, repo, epics, e, target, false)
	unlock()
	s.actOnLand(root, one, dir, e, d)
}

// landOutcome is how a land ended; the zero value is a refusal with nothing
// to act on.
type landOutcome int

const (
	outcomeNone landOutcome = iota
	// outcomeMerged: the epic landed (or had nothing to land) and was dequeued.
	outcomeMerged
	// outcomeCompleted: all-done with auto-ff-merge off; the branch is left alone.
	outcomeCompleted
	outcomeParked
	// outcomeMergeTicket: a non-ff forked a merge ticket; the root stays queued.
	outcomeMergeTicket
	// outcomeAlreadyMerged: merged elsewhere and already dequeued.
	outcomeAlreadyMerged
)

// landDecision is what landEpic decided under the lock, for its caller to act
// on after unlock. first is set only for the caller that made the
// completed_at or merged_at stamp transition: that keeps the retro and the
// epic notice once-only without holding the lock around them.
type landDecision struct {
	result  QueueResult
	outcome landOutcome
	first   bool
}

// completed reports whether the root left the queue as a final outcome.
func (d landDecision) completed() bool {
	return d.outcome == outcomeMerged || d.outcome == outcomeCompleted
}

// sendNotice: a merge always speaks, as an epic merged elsewhere notifies with
// its dequeue. A park with a kind waits on a person's fix, so its chat notice
// goes every time; a needs-rebase park and a completion are the epic's outcome
// and speak only when their stamp is new.
func (d landDecision) sendNotice() bool {
	switch d.outcome {
	case outcomeMerged:
		return true
	case outcomeCompleted:
		return d.first
	case outcomeParked:
		return d.result.ParkKind != "" || d.first
	}
	return false
}

func (d landDecision) submitRetro() bool {
	switch d.outcome {
	case outcomeMerged, outcomeCompleted, outcomeParked, outcomeAlreadyMerged:
		return d.first
	}
	return false
}

func parkDecision(first bool, reason string, kind events.Kind) landDecision {
	return landDecision{
		result:  QueueResult{Parked: true, ParkKind: string(kind), Message: reason},
		outcome: outcomeParked,
		first:   first,
	}
}

// landEpic is completeEpic's work under target's trunk lock: stamps, git and
// the dequeue. force lands with auto-ff-merge off: the manual trigger's direct
// land.
func (s *Server) landEpic(root rootRef, one ralphloop.OneIteration, repo string, epics []tickets.Epic, e tickets.Epic, target string, force bool) landDecision {
	project := root.Project
	fresh := tickets.LoadEpicTicketMD(e.Path)
	already := !fresh.MergedAt.IsZero()
	first := fresh.CompletedAt.IsZero()
	merged := true
	switch {
	case already:
		if first {
			s.stampEpic(root, e.Path, "completed_at")
		}
	case s.nothingToLand(project, epics, one, repo):
		s.stampEpic(root, e.Path, "merged_at", "completed_at")
		first = true
	case force || s.autoFFMerge(project) || hasManualMergeTicket(e):
		reason, kind := s.landRoot(project, epics, one, repo)
		// A non-ff below the cap is not an outcome: the merge ticket makes the
		// epic not all-done, so the root stays queued and its finish lands again.
		// The ticket is never manual, even under force: it counts against the cap.
		if reason != "" && kind == "" {
			if id := s.forkMergeTicket(root, e, target, false); id != "" {
				return landDecision{result: QueueResult{MergeTicket: tickets.Address{Project: project, Epic: root.Epic, ID: id}.String()}, outcome: outcomeMergeTicket}
			}
		}
		if reason != "" {
			s.stampEpic(root, e.Path, "completed_at")
			return parkDecision(first, reason, kind)
		}
		s.stampEpic(root, e.Path, "merged_at", "completed_at")
		first = true
	default:
		s.stampEpic(root, e.Path, "completed_at")
		merged = false
	}
	d := landDecision{result: QueueResult{Merged: merged}, outcome: outcomeCompleted, first: first}
	if merged {
		d.outcome = outcomeMerged
	}
	removed, err := s.queued.removeRoot(root)
	if err != nil {
		s.log.Warn("dequeue completed root", "root", root, "err", err)
	} else if removed {
		s.events.publish(EventQueueChanged, root.String())
	}
	// An epic merged elsewhere notifies with the dequeue, so a double land sends one.
	if already && !removed {
		d.outcome = outcomeAlreadyMerged
	}
	return d
}

// actOnLand sends d's notices and retro. It runs after the trunk lock is
// released, and returns d's result.
func (s *Server) actOnLand(root rootRef, one ralphloop.OneIteration, dir string, e tickets.Epic, d landDecision) QueueResult {
	if d.result.Parked {
		s.parkRoot(root, one, dir, e, d)
	} else if d.sendNotice() {
		s.notifyEpicOutcome(root.Project, dir, one, e, d.result.Merged)
	}
	if d.submitRetro() {
		s.submitRetro(root, dir, e)
	}
	// Last, so a watcher of root-completed sees the retro already filed.
	if d.completed() {
		s.events.publish(EventRootCompleted, root.String())
	}
	return d.result
}

// parkRoot reports the park of e's all-done root; an empty kind is a
// needs-rebase park.
func (s *Server) parkRoot(root rootRef, one ralphloop.OneIteration, dir string, e tickets.Epic, d landDecision) {
	project := root.Project
	reason, kind := d.result.Message, d.result.ParkKind
	s.events.publishEvent(Event{Type: EventRootParked, Address: root.String(), Kind: kind, Level: EventLevelWarning, Message: reason})
	s.log.Warn("root parked", "root", root, "kind", kind, "reason", reason)
	if !d.sendNotice() {
		return
	}
	if s.chat.Enabled() {
		s.chat.Park(project, s.chatOverride(project), one.Epic, one.Ticket.Path, one.Ticket.Identifier, string(schema.StatusNeedsAnswer), reason, ralphloop.CountsOf(e))
	}
	// Only a needs-rebase park, the one without a kind, is the
	// epic's outcome; any other park waits on a person's fix and then lands.
	if kind == "" {
		s.notifyEpicOutcome(project, dir, one, e, false)
	}
}

// notifyEpicOutcome sends the epic-level notice. A one-off skips it.
func (s *Server) notifyEpicOutcome(project, dir string, one ralphloop.OneIteration, e tickets.Epic, merged bool) {
	// The epic, not one: a one-off's merge ticket is not its one-off file.
	if !s.chat.Enabled() || IsOneOffEpic(e) {
		return
	}
	elapsed, cost := ralphloop.EpicTotals(e)
	counts := ralphloop.CountsOf(e)
	counts.Recovered = recoveredCount(dir, one.Epic)
	s.chat.EpicComplete(project, s.chatOverride(project), one.Epic, counts, elapsed, cost, merged)
}

// submitRetro files the evolve one-off for a completed loop epic. It runs at
// the epic's final outcome, where completed_at is stamped: the land, a root
// park, or all-done with auto-ff-merge off. A refused submit is logged, never an error:
// the dedupe key refuses a second live retro of the same epic.
func (s *Server) submitRetro(root rootRef, projectDir string, e tickets.Epic) {
	if e.IsMap || IsOneOffEpic(e) || !e.EvolvedAt.IsZero() {
		return
	}
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil {
		s.log.Warn("retro hook: read project file", "root", root, "err", err)
		return
	}
	mode := config.Config{Evolve: s.settings.Load().Evolve}.EvolveMode(pf)
	if mode == config.EvolveOff || mode == "" {
		return
	}
	if err := EnsureOutboxProject(s.cfg.TicketStore, mode); err != nil {
		s.log.Warn("retro hook: create outbox project", "root", root, "err", err)
		return
	}
	// A maintainer's retro writes to the gx project; without one it has nowhere to go.
	if mode == config.EvolveMaintainer {
		if _, err := s.projectDir("gx"); err != nil {
			s.log.Warn("retro hook: maintainer mode needs a gx project, not submitting", "root", root, "err", err)
			return
		}
	}
	res, err := s.oneOff(RetroSubmit(root.Project, filepath.Base(e.Path), time.Now()))
	switch {
	case err != nil:
		s.log.Warn("retro hook: submit", "root", root, "err", err)
	case res.Refused:
		s.log.Info("retro hook: submit refused", "root", root, "reason", res.Reason, "message", res.Message)
	}
}

// stampEpic stamps each of keys with now on the epic. A failed stamp is
// logged, never fatal: the land it records has already happened.
func (s *Server) stampEpic(root rootRef, epicPath string, keys ...string) {
	now := time.Now().UTC()
	fields := make(map[string]any, len(keys))
	for _, k := range keys {
		fields[k] = now
	}
	if err := tickets.StampEpicFields(epicPath, fields); err != nil {
		s.log.Warn("stamp epic", "root", root, "keys", keys, "err", err)
	}
}

// nothingToLand reports whether the root's feature branch is missing or has no
// commits ahead of its target. An error reading either means there may be
// something to land: never call an epic merged on a guess.
func (s *Server) nothingToLand(project string, epics []tickets.Epic, one ralphloop.OneIteration, repo string) bool {
	branch := "refs/heads/" + one.Epic
	if _, err := git.RevParse(repo, branch); err != nil {
		return errors.Is(err, git.ErrRefNotFound)
	}
	_, _, target, err := s.epicRefs(project, epics, one.Epic, repo)
	if err != nil {
		return false
	}
	landed, err := git.IsAncestor(repo, branch, target)
	return err == nil && landed
}

// landRoot fast-forwards the root's target to its feature branch through the
// merge core. A non-empty reason means the root must park, not land; kind
// names the park's cause, empty for a needs-rebase one. Every error parks the
// root: one left all-done in the queue looks stuck.
func (s *Server) landRoot(project string, epics []tickets.Epic, one ralphloop.OneIteration, repoDir string) (reason string, kind events.Kind) {
	_, _, target, err := s.epicRefs(project, epics, one.Epic, repoDir)
	if err != nil {
		return s.refsFailed(project, one.Epic, err)
	}
	info, err := git.IdentifyDir(repoDir)
	if err != nil {
		return landFailed(project, one.Epic, err)
	}
	worktrees, err := git.ListWorktrees(info.Repo)
	if err != nil {
		return landFailed(project, one.Epic, err)
	}
	needsRebase := fmt.Sprintf("needs rebase: %s onto %s; run gx server epics merge %s:%s", one.Epic, target, project, one.Epic)
	out, err := s.mergeInto(info.Repo, worktrees, one.Epic, target)
	if err != nil {
		// Trunk moved outside the server between git's ff check and the ff:
		// that is a non-ff, not a failed land.
		if ancestor, aerr := git.IsAncestor(info.Repo.Root, "refs/heads/"+target, one.Epic); aerr == nil && !ancestor {
			return needsRebase, ""
		}
		// No pre-check: git decides whether local changes block the land, and
		// only a refused land looks at them.
		if dir := git.CheckoutDirForBranch(info.Repo, worktrees, target); dir != "" {
			if changes, cerr := git.UncommittedChanges(dir); cerr == nil && len(changes) > 0 {
				return fmt.Sprintf("trunk dirty: %s checked out at %s has local changes in %s; commit or discard them, then run gx server epics merge %s:%s",
					target, dir, changedFiles(changes), project, one.Epic), events.DirtyTrunk
			}
		}
		return landFailed(project, one.Epic, err)
	}
	if out.Merged {
		return "", ""
	}
	return needsRebase, ""
}

// refsFailed is the park reason and kind for an epic whose refs can't be
// worked out.
func (s *Server) refsFailed(project, epic string, err error) (string, events.Kind) {
	if missing := (*tickets.ErrTrunkMissing)(nil); errors.As(err, &missing) {
		// Nothing lands; the project stops claiming until a person fixes trunk.
		s.enterTrunkMissing(project, err.Error())
		return fmt.Sprintf("%s: %v; fix it and run gx-merge %s", events.TrunkMissing, err, epic), events.TrunkMissing
	}
	return landFailed(project, epic, err)
}

func landFailed(project, epic string, err error) (string, events.Kind) {
	return fmt.Sprintf("land failed: %v; fix it, then run gx server epics merge %s:%s", err, project, epic), events.LandError
}

// maxReasonFiles caps the files a dirty-trunk reason lists.
const maxReasonFiles = 5

// changedFiles lists the changes' paths, the first maxReasonFiles then "+N more".
func changedFiles(changes []git.Change) string {
	paths := make([]string, 0, min(len(changes), maxReasonFiles))
	for _, c := range changes[:min(len(changes), maxReasonFiles)] {
		paths = append(paths, c.Path)
	}
	list := strings.Join(paths, ", ")
	if extra := len(changes) - maxReasonFiles; extra > 0 {
		list += fmt.Sprintf(" +%d more", extra)
	}
	return list
}

func launchPrompt(agent ralphloop.AgentKind, skill, note, address string) string {
	prompt := ralphloop.SkillPrompt(agent, skill, address)
	if note != "" {
		prompt += "\n\n" + note
	}
	return prompt
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

// evolveDirs are the dirs a retro's `gx tickets` verbs write to outside the
// ticket's own epic: the follow-ups epic and the gx project's ticket dir.
// A dir that does not resolve is left out; the verb then fails with its own error.
func (s *Server) evolveDirs(ticket tickets.Address) []string {
	var dirs []string
	if d, err := s.followUpsEpicPath(ticket.Project); err == nil {
		dirs = append(dirs, d)
	}
	if d, err := s.projectDir("gx"); err == nil {
		dirs = append(dirs, d)
	}
	return dirs
}

// agentConfig is agent's model and effort for project: config agents.* with
// the project's overrides on top.
func (s *Server) agentConfig(project string, agent ralphloop.AgentKind) config.AgentConfig {
	set := s.settings.Load()
	cfg := config.Config{Agents: config.AgentsConfig{Claude: set.AgentsClaude, Codex: set.AgentsCodex}}
	if dir, err := s.projectDir(project); err == nil {
		if pf, err := config.ReadProjectFile(dir); err == nil {
			cfg = cfg.WithProject(pf)
		}
	}
	if agent == ralphloop.AgentCodex {
		return cfg.Agents.Codex
	}
	return cfg.Agents.Claude
}

// runAgentConfig is the model and effort run's agent launched under, else
// the project's agents.* config. Every launch, finish and adoption resolves
// them here.
func (s *Server) runAgentConfig(project string, agent ralphloop.AgentKind, run Run) (model, effort string) {
	ac := s.agentConfig(project, agent)
	return cmp.Or(run.Model, ac.Model), cmp.Or(run.Effort, ac.Effort)
}

// launch starts one's agent in cwd. Its Codex sandbox gets one.ScratchDir, the
// project dir where the epic's tickets live.
func (s *Server) launch(deps ralphloop.Deps, ticket tickets.Address, one ralphloop.OneIteration, cwd string) (Run, error) {
	t, agent := one.Ticket, one.Agent
	var extraDirs []string
	if t.Type == string(schema.TypeEvolve) {
		extraDirs = s.evolveDirs(ticket)
	}
	// herdr rejects a ticket address as an agent name; every lookup uses the iteration label.
	label, _, _ := ralphloop.IterationIdentity(ticket.Epic, ticket.ID, "")
	start := func() (agentrunner.Session, error) {
		return ralphloop.StartAndPrompt(deps.Runner, agentrunner.StartOptions{
			Label: label,
			Epic:  ticket.Epic,
			Cwd:   cwd,
			Kind:  agentrunner.Kind(agent),
			Args:  ralphloop.AgentArgs(agent, one.ScratchDir, ticket.Epic, one.Model, one.Effort, extraDirs...),
			Env:   s.settings.Load().TabEnv,
		}, launchPrompt(agent, launchSkill(t), investigatePrompt(t), ticket.String()), func(attempt int, decision string) {
			s.logEvent(ticket, ralphloop.Event{Type: string(events.PromptNudged), Label: label, Attempt: attempt, Reason: decision})
		})
	}
	session, err := start()
	if errors.Is(err, agentrunner.ErrLabelTaken) {
		if h := s.labelHolder(deps.Runner, ticket, label, err); h.stale {
			if serr := deps.Runner.Stop(h.Session); serr != nil {
				s.log.Warn("close stale holder", "ticket", ticket.String(), "pane", h.ID, "err", serr)
			} else {
				s.logEvent(ticket, ralphloop.Event{Type: string(events.StalePaneClosed), Label: label, Pane: h.ID, Cwd: h.cwd, Reason: string(h.State)})
				session, err = start()
			}
		}
	}
	if errors.Is(err, agentrunner.ErrLabelTaken) {
		taken := &nameTakenError{reason: s.labelHolder(deps.Runner, ticket, label, err).describe(), err: err}
		s.logEvent(ticket, ralphloop.Event{Type: string(events.LaunchFailed), Kind: string(events.AgentNameTaken), Label: label, Reason: taken.Error()})
		return Run{}, taken
	}
	if err != nil {
		return Run{}, err
	}
	run := Run{Address: ticket.String(), Agent: string(agent), Runner: deps.Runner.Name(), Session: session, Model: one.Model, Effort: one.Effort}
	return run, nil
}

// logEvent appends ev to ticket's run log, best effort.
func (s *Server) logEvent(ticket tickets.Address, ev ralphloop.Event) {
	dir, err := s.projectDir(ticket.Project)
	if err != nil {
		return
	}
	ev.Ticket, ev.Address = ticket.ID, ticket.String()
	_ = ralphloop.AppendEvent(dir, ticket.Epic, ev)
}

// nameTakenError is a launch whose label a holder gx may not close still
// has; reason names that holder.
type nameTakenError struct {
	reason string
	err    error
}

func (e *nameTakenError) Error() string { return e.err.Error() + ": " + e.reason }
func (e *nameTakenError) Unwrap() error { return e.err }

// labelHolder is the live session holding a launch's label.
type labelHolder struct {
	agentrunner.Session
	agentrunner.Status
	cwd   string
	found bool
	// stale: no other run tracks it and it is not mid-turn, so nothing is
	// lost by closing it.
	stale bool
}

func (h labelHolder) describe() string {
	if !h.found {
		return "holder not found"
	}
	return fmt.Sprintf("held by pane %s (cwd %s, status %s)", h.ID, cmp.Or(h.cwd, "unknown"), h.State)
}

// labelHolder finds who holds label after Start failed with takenErr. Only
// herdr's error says the holder's cwd. The launching ticket's own registry
// entry has no session yet, so it never counts as tracking the holder.
func (s *Server) labelHolder(r agentrunner.Runner, ticket tickets.Address, label string, takenErr error) labelHolder {
	var h labelHolder
	var taken *herdr.AgentNameTakenError
	if errors.As(takenErr, &taken) {
		h.cwd = taken.CandidateCwd
	}
	sess, ok, err := r.Find(label)
	if err != nil || !ok {
		return h
	}
	h.Session, h.found = sess, true
	if h.Status, err = r.Status(sess); err != nil {
		return h
	}
	tracked := slices.ContainsFunc(s.registry.list(), func(t trackedRun) bool {
		return t.Address != ticket.String() && (t.Session.Label == sess.Label || (sess.ID != "" && t.Session.ID == sess.ID))
	})
	h.stale = !tracked && (h.State == agentrunner.StateIdle || h.State == agentrunner.StateDone)
	return h
}
