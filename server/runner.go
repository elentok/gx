package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/budget"
	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/runs"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// WaitSettled waits for every settle goroutine; see runs.Guards.WaitSettled.
func (s *Server) WaitSettled(timeout time.Duration) bool {
	return s.guards.WaitSettled(timeout)
}

// keepClaiming launches queued roots. The tick is the guarantee; the kick from a queue write only makes it sooner.
func (s *Server) keepClaiming(ctx context.Context) {
	// Reclaim sets holds a claim must see, and the index is empty until then.
	select {
	case <-s.migrator.StartupDone():
	case <-ctx.Done():
		return
	}
	poll := s.cfg.PollInterval
	if poll <= 0 {
		poll = index.DefaultPollInterval
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		s.projects.Check()
		s.rec.CheckTrunks()
		s.rec.StampHandMerged()
		s.releaseWaitingSlots(time.Now())
		s.claimNext()
		s.publishVerdictChanges()
		s.syncHerdrBanner()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.guards.Kicked():
		}
	}
}

// claimNext backfills free slots in queue order: it claims and launches the
// frontier tickets of each queued root, up to its per-root cap, until the concurrency limit is reached.
// claimRoot skips a ticket the claim hold names: a claim without an agent behind it would only be rolled back.
func (s *Server) claimNext() {
	if s.migrator.Migrating() || s.queue.Blocked() || s.budget.SoftReached(time.Now()) {
		return
	}
	limit := s.concurrencyLimit()
	running := s.registry.Count()
	seen := map[string]bool{}
	for _, item := range s.queue.List() {
		addr, err := tickets.ParseQueuedAddress(item.Address)
		if err != nil || seen[queue.RootOf(addr).String()] {
			continue
		}
		seen[queue.RootOf(addr).String()] = true
		// One root fills its own slots before the next root in the queue gets a turn.
		for running < limit {
			launched, err := s.claimRootAfterAdoption(item)
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

// releaseWaitingSlots frees the slots of agents that waited past the grace
// period, up to the release cap. The agents stay alive.
func (s *Server) releaseWaitingSlots(now time.Time) {
	st, def := s.settings.Load(), config.DefaultExecutionQueueConfig()
	grace := cmp.Or(st.WaitingSlotGrace, def.WaitingSlotGrace)
	if n := s.registry.ReleaseWaiting(now, grace, st.MaxReleasedAgents); n > 0 {
		s.log.Info("released waiting slots", "count", n)
	}
}

// stalledNudgeDelay is how long an idle agent with no report waits for its
// nudge. A negative setting, which no config file can produce, turns the nudge
// off for tests whose fake agents never leave idle again.
func (s *Server) stalledNudgeDelay() time.Duration {
	d := cmp.Or(s.settings.Load().StalledNudgeDelay, config.DefaultExecutionQueueConfig().StalledNudgeDelay)
	return max(d, 0)
}

// claimRootAfterAdoption is claimRoot once a late-finish adoption in flight is
// done, so a root never launches an investigation the adoption is about to cancel.
func (s *Server) claimRootAfterAdoption(item wire.QueueItem) (bool, error) {
	s.guards.AdoptMu.Lock()
	defer s.guards.AdoptMu.Unlock()
	return s.claimRoot(item)
}

// perRootLimit is how many agents one root may run at once. The one-offs root
// uses the one-off cap instead of the per-root one; old single-ticket one-off
// epics are roots of their own and keep the per-root cap.
func (s *Server) perRootLimit(root queue.RootRef) int {
	if root.Epic == queue.OneOffsEpic {
		if n := s.settings.Load().MaxOneOffAgents; n > 0 {
			return n
		}
		return config.DefaultExecutionQueueConfig().MaxOneOffAgents
	}
	if n := s.settings.Load().MaxAgentsPerRoot; n > 0 {
		return n
	}
	return config.DefaultExecutionQueueConfig().MaxConcurrentTicketsPerEpic
}

// projectAtCap reports whether the project's own max-agents cap is reached. The
// cap is read from project.json on every call, so an edit applies at the next
// decision; an absent, unreadable or non-positive cap means no project cap.
func (s *Server) projectAtCap(project string) (running, limit int, atCap bool) {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return 0, 0, false
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.MaxAgents == nil || *pf.MaxAgents <= 0 {
		return 0, 0, false
	}
	running, limit = s.registry.CountProject(project), *pf.MaxAgents
	return running, limit, running >= limit
}

func (s *Server) concurrencyLimit() int {
	if n := s.settings.Load().MaxAgents; n > 0 {
		return n
	}
	return config.DefaultExecutionQueueConfig().MaxAgents
}

// claimRoot reports whether it launched an iteration for item's root. A claim
// that paused instead (ambiguous base) launched nothing, so it used no slot.
func (s *Server) claimRoot(item wire.QueueItem) (bool, error) {
	addr, err := tickets.ParseQueuedAddress(item.Address)
	if err != nil {
		return false, err
	}
	root := queue.RootOf(addr)
	if s.registry.CountRoot(root.String()) >= s.perRootLimit(root) {
		return false, nil
	}
	if _, _, atCap := s.projectAtCap(addr.Project); atCap {
		return false, nil
	}
	if _, missing := s.projects.UnavailablePath(addr.Project); missing {
		return false, nil
	}
	if _, held := s.disks.FreshHeldDisk(addr.Project); held {
		return false, nil
	}
	if _, missing := s.rec.trunkMissing.Get(addr.Project); missing {
		return false, nil
	}
	if _, blocked := s.rec.MigrationBlock(addr.Project); blocked {
		return false, nil
	}
	projectDir, repo, err := s.projects.Of(addr.Project)
	if err != nil {
		return false, err
	}
	// The epics of the last scan, not a fresh load: this runs every poll tick for
	// every queued root. The sum check below catches a file the scan has not seen.
	loaded, err := s.projects.Epics(addr.Project)
	if err != nil {
		return false, err
	}
	for _, e := range loaded {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		s.checkDeadlock(root, projectDir, e)
		t, ok := s.nextUntracked(addr.Project, e)
		if !ok {
			s.merger.RetryHeldMerge(root, e, loaded, repo)
			return false, nil
		}
		if s.parentAwaitsAdoption(projectDir, addr, e, t) {
			return false, nil
		}
		ticketAddr := tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}.String()
		// A missed watch event can leave the index behind the file; claiming
		// on that view could schedule the wrong thing, so wait for the index.
		onDisk, err := index.FileSum(t.Path)
		if err != nil {
			return false, err
		}
		if seen, _ := s.idx.SumOf(ticketAddr); seen != onDisk {
			s.guards.Refused.Set(ticketAddr, true)
			return false, nil
		}
		s.guards.Refused.Set(ticketAddr, false)
		resolved := s.harness.ResolveTicket(tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}, t)
		if _, held := s.claimHold(addr.Project, t, resolved.ResolvedHarness); held {
			return false, nil
		}
		return s.claimAndLaunch(root, addr, t, repo, resolved.Harness, loaded)
	}
	return false, nil
}

// parentAwaitsAdoption reports whether t's parent reported finished after its
// pause and is not adopted yet. The report alone makes t claimable, so without
// this a claim landing between the report and the adoption's lock (adoptMu)
// launches an investigation the adoption is about to cancel.
func (s *Server) parentAwaitsAdoption(dir string, addr tickets.Address, e tickets.Epic, t tickets.Ticket) bool {
	if t.Parent == nil {
		return false
	}
	for _, p := range e.Tickets {
		if p.Identifier != *t.Parent || !adoptablePause(schema.Status(p.Status), p.PauseReason) {
			continue
		}
		_, ok := lateFinishOf(dir, tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: p.Identifier}, p.Path)
		return ok
	}
	return false
}

// nextUntracked is the first frontier ticket of e with no registry entry: the
// ticket a claim of e's root starts next, and the one its pending row names.
// The last scan can still list a ticket whose launch already began.
func (s *Server) nextUntracked(project string, e tickets.Epic) (tickets.Ticket, bool) {
	for _, t := range ralphloop.Frontier(e) {
		addr := tickets.Address{Project: project, Epic: filepath.Base(e.Path), ID: t.Identifier}
		if !s.registry.Has(addr.String()) {
			return t, true
		}
	}
	return tickets.Ticket{}, false
}

// claimAndLaunch writes the claim to the ticket file first, so markdown stays
// the truth and nothing streams before it is on disk. A failed launch pauses the
// ticket stuck rather than leaving a claim with no agent behind it.
// launched is false when the ticket paused on an ambiguous base: no agent runs.
// epics is the project's loaded epics, for the base derivation.
func (s *Server) claimAndLaunch(root queue.RootRef, queued tickets.Address, t tickets.Ticket, repo string, agent ralphloop.AgentKind, epics []tickets.Epic) (launched bool, err error) {
	ticket := tickets.Address{Project: queued.Project, Epic: queued.Epic, ID: t.Identifier}
	ticketAddr := ticket.String()
	// The project dir, not the store root: the run log, merge lock and
	// conflict-resolution tickets all live under <project>/<epic>.
	dir, err := s.projects.Dir(queued.Project)
	if err != nil {
		return false, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	rootBase, resolvedBase, leafBase, err := s.resolveBase(queued, t, repo, epics)
	var ambiguous *tickets.AmbiguousBaseError
	if errors.As(err, &ambiguous) {
		reason := "Ambiguous base: choose which of " + strings.Join(ambiguous.Blockers, ", ") + " this ticket starts from, by setting base:."
		if perr := s.writer.Pause(dir, ticket, t.Path, events.AmbiguousBase, reason, runs.Run{}); perr != nil {
			return false, fmt.Errorf("pause %s: %w", ticketAddr, perr)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("base for %s: %w", ticketAddr, err)
	}
	answered := answeredSession(t)
	if err := ralphloop.ClaimWithBase(t.Path, resolvedBase); err != nil {
		return false, fmt.Errorf("claim %s: %w", ticketAddr, err)
	}
	s.idx.Rescan() // the claim must show now, not when the backstop poll comes
	s.events.Publish(wire.EventTicketClaimed, ticketAddr)

	if err := budget.ClearLongTicketMark(t.Path); err != nil {
		s.log.Warn("clear long-ticket mark", "ticket", ticketAddr, "err", err)
	}
	model, effort, ok := s.writer.LaunchModel(dir, t, ticket, agent)
	if !ok {
		return false, nil
	}
	one := ralphloop.OneIteration{
		RepoDir: repo, Epic: queued.Epic, ScratchDir: dir, Agent: agent, Ticket: t, RootBase: rootBase, LeafBase: leafBase,
		Model: model, Effort: effort, Sink: s.writer.FinishSink(ticket, t.Path), StallNudgeDelay: s.stalledNudgeDelay(),
	}
	deps := s.projects.DepsWith(s.harness.LaunchRunner(ticket.Project, agent))
	deps.GateReleased = s.registry.GateReleased(ticketAddr)
	// Only Claude and Codex on the pty runner resume; every other claim starts
	// cold, and so does one under another harness than the answered session's:
	// the Handoff carries it over. launch drops a Codex id whose rollout is gone.
	earlier := s.harness.EarlierRunHarness(ticket)
	switchedHarness := earlier != "" && earlier != string(agent)
	if (agent == ralphloop.AgentClaude || agent == ralphloop.AgentCodex) && deps.Runner.Name() == string(agentrunner.ChoicePTY) && !switchedHarness {
		one.ResumeSessionID = answered
	}
	mode := s.landing.IterationModeFor(ticket, t)
	if mode.commitless && mode.scratchDir == "" {
		if mode.ref, err = s.commitlessRef(ticket, t, repo, epics); err != nil {
			return false, fmt.Errorf("base for %s: %w", ticketAddr, err)
		}
	}
	if mode.Feature {
		if one.TargetTip, err = s.mergeTargetTip(ticket, repo); err != nil {
			return false, fmt.Errorf("merge target for %s: %w", ticketAddr, err)
		}
	}
	entry := runs.TrackedRun{
		Run:  runs.Run{Address: ticketAddr, Agent: string(agent), Runner: deps.Runner.Name(), Model: one.Model, Effort: one.Effort},
		Root: root.String(), Repo: repo, TicketPath: t.Path, StartedAt: time.Now(), State: runs.Launching,
	}
	if !s.registry.PutIfAbsent(entry) {
		return false, nil // a stale scan offered a ticket that is already running
	}
	run, wt, err := s.prepareAndLaunch(deps, &one, ticket, mode, func(wt ralphloop.IterationWorktree) {
		entry.Workspace, entry.Base, entry.Cwd = one.WorkspaceID, wt.Base(), wt.Path
		s.registry.Put(entry)
	})
	if err != nil {
		s.registry.DeleteStarted(ticketAddr, entry.StartedAt)
		// Handing the claim back would make the next tick retry the same failure
		// forever, so a person is told instead.
		kind, reason := events.IterationError, "launch failed: "+err.Error()
		if taken := (*nameTakenError)(nil); errors.As(err, &taken) {
			kind, reason = events.AgentNameTaken, "agent name taken: "+taken.reason
		} else if k, ok := ralphloop.LaunchFailureKind(err); ok {
			kind = k
		}
		if perr := s.writer.Pause(dir, ticket, t.Path, kind, reason, entry.Run); perr != nil {
			err = errors.Join(err, fmt.Errorf("pause: %w", perr))
		}
		s.events.Publish(wire.EventIterationLaunchFailed, ticketAddr)
		return false, fmt.Errorf("launch %s: %w", ticketAddr, err)
	}
	s.registry.Put(runs.TrackedRun{
		Run: run, Root: root.String(), Repo: repo, Workspace: one.WorkspaceID, Base: wt.Base(), Cwd: wt.Path, TicketPath: t.Path, StartedAt: entry.StartedAt,
	})
	s.events.Publish(wire.EventIterationStarted, ticketAddr)
	s.guards.GoSettle(func() { s.finishRun(runContext{deps, root, mode, one, wt, run, ticketAddr}) })
	return true, nil
}

// resolveBase derives where t starts from. ref is the branch the root epic's
// feature branch is created from (the epic's refs, not t's), with its
// "ref@sha" claim stamp; leaf is the
// unmerged sibling whose branch t starts from instead of the feature tip. A
// commitless ticket reads at the feature tip, so everything comes back empty.
func (s *Server) resolveBase(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (ref, stamp, leaf string, err error) {
	if t.Commitless {
		return "", "", "", nil
	}
	if leaf, err = tickets.DeriveLeafBase(addr.Project, epics, addr.Epic, t); err != nil {
		return "", "", "", err
	}
	_, ref, _, err = s.merger.EpicRefs(addr.Project, epics, addr.Epic, repo)
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

// mergingOnMerged reports whether the project uses the on-merged merging policy.
// An unreadable project file falls back to the default policy.
func (s *Server) mergingOnMerged(project string) bool {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return true
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.LandingPolicy() == config.LandingOnLanded
}

// rootRefs is where a root's feature branch starts (base: the claim base and
// the commitless ref) and where it merges (target). Trunk is read on every
// call, so a merging uses the trunk current then; nothing records it at claim.
// An explicit base: moves only the start; a derived stacked root also merges
// on its blocker's branch until that branch reaches trunk.
func (s *Server) rootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error) {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return "", "", err
	}
	trunk, trunkErr := tickets.Trunk(dir, repo)
	derived, err := tickets.DeriveRootBase(project, epics, epic, t, s.mergingOnMerged(project))
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
		// merged on a trunk that isn't there.
		return derived, derived, nil
	}
	ref := retargetIfMerged(derived, trunk, func(a, d string) (bool, error) { return gitIsAncestor(repo, a, d) })
	return ref, ref, nil
}

// mergeTarget is the target of the root epic addr belongs to.
func (s *Server) mergeTarget(addr tickets.Address) (string, error) {
	dir, repo, err := s.projects.Of(addr.Project)
	if err != nil {
		return "", err
	}
	epics, err := tickets.LoadResolved(addr.Project, dir)
	if err != nil {
		return "", err
	}
	_, _, target, err := s.merger.EpicRefs(addr.Project, epics, addr.Epic, repo)
	return target, err
}

// mergeTargetTip is the SHA addr's merge target points at now.
func (s *Server) mergeTargetTip(addr tickets.Address, repo string) (string, error) {
	target, err := s.mergeTarget(addr)
	if err != nil {
		return "", err
	}
	return git.RevParse(repo, target)
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

// retargetIfMerged is derived, never stored: once the blocker's branch is
// reachable from trunk the dependent targets trunk again.
// An unresolvable branch counts as not merged; never retarget on a guess.
func retargetIfMerged(ref, trunk string, isAncestor func(ancestor, descendant string) (bool, error)) string {
	if merged, err := isAncestor(ref, trunk); err == nil && merged {
		return trunk
	}
	return ref
}

// commitlessRef is the ref a commitless ticket's detached worktree starts at:
// where its blockers resolve to.
func (s *Server) commitlessRef(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (string, error) {
	if ref, ok := srvrecovery.InvestigateRef(repo, addr, t, epics); ok && t.Type == string(schema.TypeInvestigate) {
		return ref, nil
	}
	ref, _, err := s.rootRefs(addr.Project, epics, addr.Epic, t, repo)
	return ref, err
}

// prepareAndLaunch gives the ticket somewhere to run, then launches the agent
// in it. A launch that fails takes that back out so a retry starts clean.
// prepared sees the worktree before the agent starts.
func (s *Server) prepareAndLaunch(
	deps ralphloop.Deps, one *ralphloop.OneIteration, ticket tickets.Address, mode IterationMode,
	prepared func(ralphloop.IterationWorktree),
) (runs.Run, ralphloop.IterationWorktree, error) {
	wt, err := mode.prepare(deps, *one)
	if err != nil {
		return runs.Run{}, ralphloop.IterationWorktree{}, err
	}
	prepared(wt)
	run, err := s.launch(deps, ticket, *one, wt.Path)
	if err != nil {
		if derr := mode.discard(deps, *one, wt); derr != nil {
			err = errors.Join(err, fmt.Errorf("discard worktree: %w", derr))
		}
		return runs.Run{}, ralphloop.IterationWorktree{}, err
	}
	return run, wt, nil
}

// finishRun waits for the agent to settle, then merges (or pauses) the ticket and
// moves the root on: the next frontier ticket, or the root's completion.
func (s *Server) finishRun(rc runContext) {
	rc.one.Runner = rc.run.Runner
	rc.one.OnRateLimit = func(limited bool, resetAt time.Time) {
		s.registry.MarkRateLimit(rc.ticketAddr, limited, resetAt)
	}
	addr, _ := tickets.ParseAddress(rc.ticketAddr, tickets.AddressContext{}) // built by claimAndLaunch, always parses
	s.budget.GuardCallbacks(addr, &rc.one)
	stop := make(chan struct{})
	if !rc.one.PausesAtOnce() {
		go s.watchBlocked(rc.deps.Runner, rc.run.Session, rc.ticketAddr, stop)
	}
	err := ralphloop.WaitIterationFinished(rc.deps, rc.one, rc.wt, rc.run.Session)
	close(stop)
	s.settleRun(rc, err)
}

// blockWatchPoll bounds each Wait of watchBlocked. A herdr Wait is one herdr
// process, so a short poll would cost an idle server a process per tick.
const blockWatchPoll = 30 * time.Second

// watchBlocked publishes a ticket change each time the agent enters or leaves
// a blocked state, until stop closes or the session ends. The settle wait
// never returns on a block (a blocked iteration stays running), and the
// permission card is read only when a page fetches, so without this an open
// page never shows the prompt.
func (s *Server) watchBlocked(r agentrunner.Runner, sess agentrunner.Session, addr string, stop <-chan struct{}) {
	blocked := false
	for {
		until := []agentrunner.State{agentrunner.StateBlocked}
		if blocked {
			until = []agentrunner.State{agentrunner.StateWorking, agentrunner.StateIdle, agentrunner.StateDone}
		}
		_, err := r.Wait(sess, until, blockWatchPoll)
		select {
		case <-stop:
			return
		default:
		}
		if errors.Is(err, agentrunner.ErrTimeout) {
			continue
		}
		if err != nil {
			return
		}
		blocked = !blocked
		s.events.Publish(wire.EventTicketChanged, addr)
	}
}

// runContext is everything settling one claimed iteration needs.
type runContext struct {
	deps       ralphloop.Deps
	root       queue.RootRef
	mode       IterationMode
	one        ralphloop.OneIteration
	wt         ralphloop.IterationWorktree
	run        runs.Run
	ticketAddr string
}

// settleRun merges (or pauses) an iteration whose wait ended with waitErr. A run
// found already finished at reattach skips the wait and comes straight here.
func (s *Server) settleRun(rc runContext, err error) {
	deps, root, mode, one, wt, run, ticketAddr := rc.deps, rc.root, rc.mode, rc.one, rc.wt, rc.run, rc.ticketAddr
	defer s.guards.Kick()
	s.registry.MarkSettling(ticketAddr)
	keepRun := false
	defer func() {
		if !keepRun {
			s.registry.Delete(ticketAddr)
			// The sweep skips a tracked run, so this is the earliest it can archive it.
			if a, err := tickets.ParseAddress(ticketAddr, tickets.AddressContext{}); err == nil && a.Epic == queue.OneOffsEpic {
				s.idx.Rescan()
			}
		}
	}()
	// A run the budget hard limit is stopping never finishes, merges or pauses
	// for its own reason: the first of it and the kill pauses it budget-killed.
	if stopping, first := s.registry.ClaimBudgetStop(ticketAddr); stopping {
		if t, ok := s.registry.Tracked(ticketAddr); ok && first {
			if perr := s.budget.PauseKilled(t); perr != nil {
				s.log.Error("budget kill", "ticket", ticketAddr, "err", perr)
			}
		}
		return
	}
	addr, _ := tickets.ParseAddress(ticketAddr, tickets.AddressContext{}) // built by claimAndLaunch, always parses
	if s.recovery.PauseReleasedGate(one.ScratchDir, addr, one.Ticket.Path, run) {
		return
	}
	var blocked *ralphloop.BlockedPausedError
	if errors.As(err, &blocked) {
		// The wait already wrote the pause. The agent is alive and waits for a
		// person: hand the run to the waiting-run machinery as a question does.
		keepRun = true
		s.registry.MarkWaiting(ticketAddr, time.Now())
		s.events.Publish(wire.EventIterationPaused, ticketAddr)
		s.writer.AnnouncePause(addr, one.Ticket.Path, events.BlockedPane, blocked.Reason)
		go s.writer.WatchUnblock(deps.Runner, run.Session, ticketAddr)
		return
	}
	var mismatch *ralphloop.ModelMismatchError
	if errors.As(err, &mismatch) {
		s.writer.PauseModelMismatch(rc.one.ScratchDir, rc.one.Ticket.Path, addr, rc.run, mismatch)
		return
	}
	var out ralphloop.FinishOutcome
	if err == nil {
		// A stop that began while the agent settled leaves it for the next
		// server, which reclaims it from the saved registry: keep it there.
		if !s.guards.Merges.Begin() {
			keepRun = true
			return
		}
		defer s.guards.Merges.End()
		one.Model, one.Effort = s.projects.RunAgentConfig(addr.Project, one.Agent, run)
		one.KeepAskedAgent = !s.cfg.DropAskedAgent && keepsAskedAgent(run)
		out, err = mode.finish(deps, one, wt, run.Session.ID)
		// The finish path wrote the merge or the pause; the store watch is only a hint.
		s.idx.Rescan()
	}
	if err != nil {
		s.log.Warn("finish iteration", "ticket", ticketAddr, "err", err)
		// Pause it: a claimed ticket nobody is working on is stuck, not failed.
		if perr := s.writer.Pause(one.ScratchDir, addr, one.Ticket.Path, events.IterationError, err.Error(), run); perr != nil {
			s.log.Warn("pause failed finish", "ticket", ticketAddr, "err", perr)
		}
		s.events.Publish(wire.EventIterationFailed, ticketAddr)
		return
	}
	if !out.Merged {
		// The finish path already wrote the pause; tell a person its own reason.
		if one.KeepAskedAgent && out.Kind == events.SelfReported {
			// The agent waits idle for the answer: its run, worktree and session stay.
			keepRun = true
			s.registry.MarkWaiting(ticketAddr, time.Now())
		}
		s.events.Publish(wire.EventIterationPaused, ticketAddr)
		s.writer.AnnouncePause(addr, one.Ticket.Path, out.Kind, out.Reason)
		return
	}
	// Stop already cleans an adopted session; a run found finished at reattach
	// was never adopted, so its I/O files are removed here.
	if cerr := agentrunner.Cleanup(deps.Runner, run.Session.Label); cerr != nil {
		s.log.Warn("clean up agent", "ticket", ticketAddr, "err", cerr)
	}
	s.events.Publish(wire.EventTicketDone, ticketAddr)
	s.writer.StampFinished(addr, one.Ticket.Path, false)
	s.oneOffs.NotifyResult(addr, one.Ticket.Path)
	s.notifyDone(addr, one.Ticket.Path)
	s.recovery.FileFollowUp(addr, one.Ticket.Path)
	if mode.afterDone != nil {
		mode.afterDone()
	}
	s.merger.CompleteRootIfDone(root, one)
}

// keepsAskedAgent reports whether run's agent stays alive after it asks: a
// Claude agent on a native runner, or a Codex agent on the pty runner.
// Herdr-hosted runs clean up as before.
func keepsAskedAgent(run runs.Run) bool {
	switch run.Agent {
	case string(ralphloop.AgentClaude):
		return run.Runner != "" && run.Runner != runs.RunnerHerdr
	case string(ralphloop.AgentCodex):
		return run.Runner == string(agentrunner.ChoicePTY)
	}
	return false
}

// notifyDone tells chat a ticket merged. A one-off stays silent: it speaks
// only through notifyResult, when its submit asked for it.
func (s *Server) notifyDone(addr tickets.Address, ticketPath string) {
	if !s.chat.Enabled() || queue.IsOneOffTicket(ticketPath) {
		return
	}
	if e, t, ok := s.FreshTicket(addr); ok && !queue.IsOneOffEpic(e) {
		s.chat.TicketDone(addr.Project, s.projects.ChatOverride(addr.Project), addr.Epic, t, ralphloop.CountsOf(e))
	}
}
