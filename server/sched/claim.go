package sched

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// KeepClaiming launches queued roots. The tick is the guarantee; the kick from a queue write only makes it sooner.
func (s *Scheduler) KeepClaiming(ctx context.Context) {
	// Reclaim sets holds a claim must see, and the index is empty until then.
	select {
	case <-s.Migrator.StartupDone():
	case <-ctx.Done():
		return
	}
	poll := s.PollInterval
	if poll <= 0 {
		poll = index.DefaultPollInterval
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		s.Projects.Check()
		s.Rec.CheckTrunks()
		s.Rec.StampHandMerged()
		s.releaseWaitingSlots(time.Now())
		s.ClaimNext()
		s.Explain.PublishChanges()
		s.Herdr.SyncBanner()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.Guards.Kicked():
		}
	}
}

// ClaimNext backfills free slots in queue order: it claims and launches the
// frontier tickets of each queued root, up to its per-root cap, until the concurrency limit is reached.
// claimRoot skips a ticket the claim hold names: a claim without an agent behind it would only be rolled back.
// Tests call it to force a pass.
func (s *Scheduler) ClaimNext() {
	if s.Migrator.Migrating() || s.Queue.Blocked() || s.Budget.SoftReached(time.Now()) {
		return
	}
	limit := s.ConcurrencyLimit()
	running := s.Registry.Count()
	seen := map[string]bool{}
	for _, item := range s.Queue.List() {
		addr, err := tickets.ParseQueuedAddress(item.Address)
		if err != nil || seen[queue.RootOf(addr).String()] {
			continue
		}
		seen[queue.RootOf(addr).String()] = true
		// One root fills its own slots before the next root in the queue gets a turn.
		for running < limit {
			launched, err := s.claimRootAfterAdoption(item)
			if err != nil {
				s.Log.Warn("claim queued root", "root", item.Address, "err", err)
				break
			}
			if !launched {
				break
			}
			running++
		}
	}
}

// ClaimTicket claims and launches one ticket the way a claim pass would, but
// without asking the frontier, and reports whether an iteration launched. For
// tests.
func (s *Scheduler) ClaimTicket(address, agentKind string) (bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false, err
	}
	ref, ok, err := s.Writer.FindTicket(address)
	if err != nil || !ok {
		return false, errors.Join(err, errors.New("no ticket "+address))
	}
	epics, err := tickets.Load(ref.ProjectDir)
	if err != nil {
		return false, err
	}
	return s.Launcher.ClaimAndLaunch(queue.RootOf(addr), addr, ref.Ticket, ref.Repo, agent.AgentKind(agentKind), epics)
}

// releaseWaitingSlots frees the slots of agents that waited past the grace
// period, up to the release cap. The agents stay alive.
func (s *Scheduler) releaseWaitingSlots(now time.Time) {
	st, def := s.Settings.Load(), config.DefaultExecutionQueueConfig()
	grace := cmp.Or(st.WaitingSlotGrace, def.WaitingSlotGrace)
	if n := s.Registry.ReleaseWaiting(now, grace, st.MaxReleasedAgents); n > 0 {
		s.Log.Info("released waiting slots", "count", n)
	}
}

// StalledNudgeDelay is how long an idle agent with no report waits for its
// nudge. A negative setting, which no config file can produce, turns the nudge
// off for tests whose fake agents never leave idle again.
func (s *Scheduler) StalledNudgeDelay() time.Duration {
	d := cmp.Or(s.Settings.Load().StalledNudgeDelay, config.DefaultExecutionQueueConfig().StalledNudgeDelay)
	return max(d, 0)
}

// claimRootAfterAdoption is claimRoot once a late-finish adoption in flight is
// done, so a root never launches an investigation the adoption is about to cancel.
func (s *Scheduler) claimRootAfterAdoption(item wire.QueueItem) (bool, error) {
	s.Guards.AdoptMu.Lock()
	defer s.Guards.AdoptMu.Unlock()
	return s.claimRoot(item)
}

// PerRootLimit is how many agents one root may run at once. The one-offs root
// uses the one-off cap instead of the per-root one; old single-ticket one-off
// epics are roots of their own and keep the per-root cap.
func (s *Scheduler) PerRootLimit(root queue.RootRef) int {
	if root.Epic == queue.OneOffsEpic {
		if n := s.Settings.Load().MaxOneOffAgents; n > 0 {
			return n
		}
		return config.DefaultExecutionQueueConfig().MaxOneOffAgents
	}
	if n := s.Settings.Load().MaxAgentsPerRoot; n > 0 {
		return n
	}
	return config.DefaultExecutionQueueConfig().MaxConcurrentTicketsPerEpic
}

// ProjectAtCap reports whether the project's own max-agents cap is reached. The
// cap is read from project.json on every call, so an edit applies at the next
// decision; an absent, unreadable or non-positive cap means no project cap.
func (s *Scheduler) ProjectAtCap(project string) (running, limit int, atCap bool) {
	dir, err := s.Projects.Dir(project)
	if err != nil {
		return 0, 0, false
	}
	pf, err := config.ReadProjectFile(dir)
	if err != nil || pf.MaxAgents == nil || *pf.MaxAgents <= 0 {
		return 0, 0, false
	}
	running, limit = s.Registry.CountProject(project), *pf.MaxAgents
	return running, limit, running >= limit
}

// ConcurrencyLimit is how many agents may run at once across all projects.
func (s *Scheduler) ConcurrencyLimit() int {
	if n := s.Settings.Load().MaxAgents; n > 0 {
		return n
	}
	return config.DefaultExecutionQueueConfig().MaxAgents
}

// claimRoot reports whether it launched an iteration for item's root. A claim
// that paused instead (ambiguous base) launched nothing, so it used no slot.
func (s *Scheduler) claimRoot(item wire.QueueItem) (bool, error) {
	addr, err := tickets.ParseQueuedAddress(item.Address)
	if err != nil {
		return false, err
	}
	root := queue.RootOf(addr)
	if s.Registry.CountRoot(root.String()) >= s.PerRootLimit(root) {
		return false, nil
	}
	if _, _, atCap := s.ProjectAtCap(addr.Project); atCap {
		return false, nil
	}
	if _, missing := s.Projects.UnavailablePath(addr.Project); missing {
		return false, nil
	}
	if _, held := s.Disks.FreshHeldDisk(addr.Project); held {
		return false, nil
	}
	if _, missing := s.Rec.TrunkMissing.Get(addr.Project); missing {
		return false, nil
	}
	if _, blocked := s.Rec.MigrationBlock(addr.Project); blocked {
		return false, nil
	}
	projectDir, repo, err := s.Projects.Of(addr.Project)
	if err != nil {
		return false, err
	}
	// The epics of the last scan, not a fresh load: this runs every poll tick for
	// every queued root. The sum check below catches a file the scan has not seen.
	loaded, err := s.Projects.Epics(addr.Project)
	if err != nil {
		return false, err
	}
	for _, e := range loaded {
		if filepath.Base(e.Path) != addr.Epic {
			continue
		}
		s.CheckDeadlock(root, projectDir, e)
		t, ok := s.NextUntracked(addr.Project, e)
		if !ok {
			s.Merger.RetryHeldMerge(root, e, loaded, repo)
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
		if seen, _ := s.Index.SumOf(ticketAddr); seen != onDisk {
			s.Guards.Refused.Set(ticketAddr, true)
			return false, nil
		}
		s.Guards.Refused.Set(ticketAddr, false)
		resolved := s.Harness.ResolveTicket(tickets.Address{Project: addr.Project, Epic: addr.Epic, ID: t.Identifier}, t)
		if _, held := s.Explain.ClaimHold(addr.Project, t, resolved.ResolvedHarness); held {
			return false, nil
		}
		return s.Launcher.ClaimAndLaunch(root, addr, t, repo, resolved.Harness, loaded)
	}
	return false, nil
}

// parentAwaitsAdoption reports whether t's parent reported finished after its
// pause and is not adopted yet. The report alone makes t claimable, so without
// this a claim landing between the report and the adoption's lock (adoptMu)
// launches an investigation the adoption is about to cancel.
func (s *Scheduler) parentAwaitsAdoption(dir string, addr tickets.Address, e tickets.Epic, t tickets.Ticket) bool {
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

// NextUntracked is the first frontier ticket of e with no registry entry: the
// ticket a claim of e's root starts next, and the one its pending row names.
// The last scan can still list a ticket whose launch already began.
func (s *Scheduler) NextUntracked(project string, e tickets.Epic) (tickets.Ticket, bool) {
	for _, t := range ralphloop.Frontier(e) {
		addr := tickets.Address{Project: project, Epic: filepath.Base(e.Path), ID: t.Identifier}
		if !s.Registry.Has(addr.String()) {
			return t, true
		}
	}
	return tickets.Ticket{}, false
}
