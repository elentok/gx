package server

import (
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sched"
	"github.com/elentok/gx/server/ticketops"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// newTicketWriter needs s.log, idx, events, projects, chat, registry, guards,
// harness and budget set. Limits and disks come later in New, so the host
// reads them per call.
func newTicketWriter(s *Server) *ticketwrite.Writer {
	return ticketwrite.New(ticketwrite.Deps{
		Log: s.log, Index: s.idx, Events: s.events, Projects: s.projects, Chat: s.chat,
		Registry: s.registry, Guards: s.guards, Harness: s.harness, Budget: s.budget,
		Host: ticketWriteHost{s}, BlockWatchPoll: sched.BlockWatchPoll,
	})
}

// ticketWriteHost is ticketwrite.Host.
type ticketWriteHost struct{ s *Server }

func (h ticketWriteHost) HoldPauseForRecovery(addr tickets.Address, ticketPath string, kind events.Kind, reason string) bool {
	return h.s.recovery.HoldPauseForRecovery(addr, ticketPath, kind, reason)
}

func (h ticketWriteHost) WriteFollowUp(from tickets.Address, f ticketwrite.FollowUp) (string, error) {
	return h.s.recovery.WriteFollowUp(from, f)
}

func (h ticketWriteHost) HerdrUnavailable() bool { return h.s.herdr.IsUnavailable() }

func (h ticketWriteHost) StopRun(run runs.Run) error {
	r, err := h.s.harness.RunnerOf(run)
	if err != nil {
		return err
	}
	return r.Stop(run.Session)
}

func (h ticketWriteHost) TakeTierOverride(ticketPath string) agent.Tier {
	return ticketops.TakeTierOverride(ticketPath)
}

func (h ticketWriteHost) HerdrHolds(r agentrunner.Runner) (string, bool) { return h.s.herdr.Holds(r) }

func (h ticketWriteHost) UsageGate(agent agent.AgentKind, now time.Time) (string, bool) {
	return h.s.limits.UsageGate(agent, now)
}

func (h ticketWriteHost) FreshHeldDisk(project string) (wire.DiskStatus, bool) {
	return h.s.disks.FreshHeldDisk(project)
}

func (h ticketWriteHost) BudgetRefusal() (wire.QueueResult, bool) { return budgetRefusal(h.s) }

func (h ticketWriteHost) SettleLive(addrs []tickets.Address, stop bool, reason string, blocks func(agentrunner.Runner, agentrunner.Session) bool) (wire.QueueResult, bool, error) {
	return h.s.ops.SettleLive(addrs, stop, reason, blocks)
}

func (h ticketWriteHost) ClaimAndLaunch(root queue.RootRef, queued tickets.Address, t tickets.Ticket, repo string, agent agent.AgentKind, epics []tickets.Epic) (bool, error) {
	return h.s.launcher.ClaimAndLaunch(root, queued, t, repo, agent, epics)
}

func (h ticketWriteHost) CancelInvestigateChild(ref queue.TicketRef, source string) error {
	return h.s.ops.CancelInvestigateChild(ref, source)
}

func (h ticketWriteHost) AdoptFirst(dir string, addr tickets.Address, path string) bool {
	return h.s.rec.AdoptFirst(dir, addr, path)
}

func (h ticketWriteHost) InvestigateRetryFailure(qc queue.Ctx, ref queue.TicketRef) (recovery.Failure, wire.QueueResult, bool) {
	return h.s.recovery.InvestigateRetryFailure(qc, ref)
}

func (h ticketWriteHost) AppendRecoveryApplied(ref queue.TicketRef, f recovery.Failure, investigator string) {
	h.s.recovery.AppendRecoveryApplied(ref, f, investigator)
}

func (h ticketWriteHost) LiveIteration(address string) (ticketwrite.LiveIter, bool) {
	return h.s.scheduler.LiveIteration(address)
}

func (h ticketWriteHost) ResettleWaiting(t runs.TrackedRun) error { return resettleWaiting(h.s, t) }
