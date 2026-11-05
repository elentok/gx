package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketgit"
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
		s.explain.PublishChanges()
		s.herdr.SyncBanner()
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
	if _, missing := s.rec.TrunkMissing.Get(addr.Project); missing {
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
		s.scheduler.CheckDeadlock(root, projectDir, e)
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
		if _, held := s.explain.ClaimHold(addr.Project, t, resolved.ResolvedHarness); held {
			return false, nil
		}
		return s.launcher.ClaimAndLaunch(root, addr, t, repo, resolved.Harness, loaded)
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
		if p.Identifier != *t.Parent || !merge.AdoptablePause(schema.Status(p.Status), p.PauseReason) {
			continue
		}
		return merge.HasLateFinish(dir, tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: p.Identifier}, p.Path)
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

// gitIsAncestor is swapped in tests to count git calls.
var gitIsAncestor = git.IsAncestor

// RootRefs is where a root's feature branch starts (base: the claim base and
// the commitless ref) and where it merges (target). Trunk is read on every
// call, so a merging uses the trunk current then; nothing records it at claim.
// An explicit base: moves only the start; a derived stacked root also merges
// on its blocker's branch until that branch reaches trunk.
func (s *Server) RootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error) {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return "", "", err
	}
	trunk, trunkErr := ticketgit.Trunk(dir, repo)
	derived, err := tickets.DeriveRootBase(project, epics, epic, t, s.projects.MergingOnMerged(project))
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

// MergeTarget is the target of the root epic addr belongs to.
func (s *Server) MergeTarget(addr tickets.Address) (string, error) {
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
	target, err := s.MergeTarget(addr)
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
	ref, _, err := s.RootRefs(addr.Project, epics, addr.Epic, t, repo)
	return ref, err
}
