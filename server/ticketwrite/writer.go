// Package ticketwrite holds the single-ticket writes: pausing a ticket and
// announcing the pause, the recovery hold, the model-mismatch pause and the
// retro stamp.
package ticketwrite

import (
	"log/slog"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/budget"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// FollowUp is what a caller supplies to file a follow-up: the title and the
// text of each section, plus the key that makes two filings the same problem.
type FollowUp struct {
	Slug, Title, Source, Problem, SuggestedFix, Key string
}

// Host is what the writer needs from the root, in three parts so each stays
// small.
type Host interface {
	PauseHost
	LaunchHost
	LiveHost
}

// PauseHost is what pausing needs.
type PauseHost interface {
	// HoldPauseForRecovery starts the pause's recovery and reports whether it
	// kept the pause's chat message back.
	HoldPauseForRecovery(addr tickets.Address, ticketPath string, kind events.Kind, reason string) bool
	// WriteFollowUp files the follow-up, or notes a repeat on the open one.
	WriteFollowUp(from tickets.Address, f FollowUp) (string, error)
	// HerdrUnavailable reports whether herdr is down.
	HerdrUnavailable() bool
	// StopRun stops the run's session.
	StopRun(run runs.Run) error
	// TakeTierOverride returns the ticket's one-time tier and clears it.
	TakeTierOverride(ticketPath string) agent.Tier
}

// LaunchHost is what relaunching a ticket needs.
type LaunchHost interface {
	// HerdrHolds reports why r cannot take work now: herdr is down.
	HerdrHolds(r agentrunner.Runner) (reason string, held bool)
	// UsageGate reports why agent is held by a usage limit now.
	UsageGate(agent agent.AgentKind, now time.Time) (verdict string, held bool)
	// FreshHeldDisk reports the held disk status of project, if any.
	FreshHeldDisk(project string) (wire.DiskStatus, bool)
	// BudgetRefusal refuses new agent work past the daily soft limit.
	BudgetRefusal() (wire.QueueResult, bool)
	// SettleLive refuses (or, with stop, stops) the live agents of addrs that block.
	SettleLive(addrs []tickets.Address, stop bool, reason string, blocks func(agentrunner.Runner, agentrunner.Session) bool) (wire.QueueResult, bool, error)
	// ClaimAndLaunch claims the ticket and starts its iteration.
	ClaimAndLaunch(root queue.RootRef, queued tickets.Address, t tickets.Ticket, repo string, agent agent.AgentKind, epics []tickets.Epic) (bool, error)
	// CancelInvestigateChild withdraws ref's unfinished investigate child.
	CancelInvestigateChild(ref queue.TicketRef, source string) error
	// AdoptFirst adopts a late finish of the ticket and reports whether it ended adopted.
	AdoptFirst(dir string, addr tickets.Address, path string) bool
	// InvestigateRetryFailure checks that an investigator may retry ref.
	InvestigateRetryFailure(qc queue.Ctx, ref queue.TicketRef) (recovery.Failure, wire.QueueResult, bool)
	// AppendRecoveryApplied records the recovery an investigate retry applied.
	AppendRecoveryApplied(ref queue.TicketRef, f recovery.Failure, investigator string)
}

// LiveHost is what the nudge and the blocked pause need of live agents.
type LiveHost interface {
	// LiveIteration resolves a ticket's live iteration.
	LiveIteration(address string) (LiveIter, bool)
	// ResettleWaiting ends a run's wait for an answer and settles it.
	ResettleWaiting(t runs.TrackedRun) error
}

// LiveIter is a live iteration with the runner and session hosting it.
type LiveIter struct {
	Info    wire.IterationInfo
	Runner  agentrunner.Runner
	Session agentrunner.Session
}

// Deps is what New needs.
type Deps struct {
	Log      *slog.Logger
	Index    *index.Index
	Events   *broker.Broker
	Projects *projects.Registry
	Chat     *notify.ServerChat
	Registry *runs.Registry
	Guards   *runs.Guards
	Harness  *harnesschoice.Harness
	Budget   *budget.Budget
	Host     Host
	// BlockWatchPoll bounds each wait of the unblock watch.
	BlockWatchPoll time.Duration
}

// Writer makes the single-ticket writes.
type Writer struct {
	d    Deps
	fold PauseFold
	// nudges rate-limits Nudge.
	nudges NudgeLimiter
	// unblockMu makes leaving a headless block happen once, whoever sees it first.
	unblockMu sync.Mutex
}

// New returns a Writer. The pause fold's cap flushes the digest to chat.
func New(d Deps) *Writer {
	w := &Writer{d: d}
	w.fold.OnCap = w.FlushPauseFold
	return w
}

// FlushPauseFold sends the held herdr-outage pauses as one digest.
func (w *Writer) FlushPauseFold() {
	w.d.Chat.PauseDigest(w.fold.Flush())
}

// holdPause queues the pause line for the digest.
func (w *Writer) holdPause(addr tickets.Address, ticketPath, status string, kind events.Kind, reason string) {
	w.fold.Hold(notify.PauseDigestLine(addr.Project, addr.Epic, addr.ID, status, PauseChatDetail(ticketPath, kind, reason)))
}
