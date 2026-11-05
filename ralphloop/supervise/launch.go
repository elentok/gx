package supervise

import (
	"slices"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// SessionOrDefault is the Runner session hosting the agent. Herdr-only paths
// (reattach, attach to a live agent) leave Session zero; herdrrunner's
// Session.ID is the pane id, so those still address the right agent.
func (p LaunchParams) SessionOrDefault() agentrunner.Session {
	if p.Session != (agentrunner.Session{}) {
		return p.Session
	}
	return agentrunner.Session{Label: p.Label, ID: p.Pane}
}

// LaunchParams are the per-call inputs to launchAndPrompt.
type LaunchParams struct {
	Label string // agent name/tab label, used in error messages
	Agent agent.AgentKind
	// Model and Effort are the resolved per-agent config the launched agent
	// process starts under; empty omits that one flag from the launch argv.
	Model  string
	Effort string
	// Session is the Runner session hosting the agent, for a fresh launch
	// started through Deps.Runner; zero on the herdr-only paths.
	Session agentrunner.Session
	Pane    string // pane id to launch the agent in and send the prompt to
	Prompt  string // initial skill prompt text

	// FinishTimeoutMs bounds the final "wait for the agent to finish" step, so
	// a stuck agent surfaces as a distinct error instead of blocking forever.
	// Zero means wait indefinitely.
	FinishTimeoutMs int

	// SessionCwd is the cwd Pane's agent was launched in.
	SessionCwd string
	// Input restarts the finish debounce on a person's key; nil sees none.
	Input *PersonInput
	// SmartZone is the context-token ceiling before this agent gets paused.
	SmartZone int
	// LongTicketTurns and OnLongTicket: see OneIteration.
	LongTicketTurns int
	OnLongTicket    func(turns int)
	// Gate is the pause/resume coordinator shared across the whole Run call.
	Gate *Gate
	// Sink receives this Run call's lifecycle events, safe to call
	// concurrently.
	Sink runlog.EventSink
	// Report writes a line to the loop's output, safe to call concurrently.
	// Only the reattach path still reads this — the rest of the package
	// reports through Sink instead. Run itself has no
	// legacy text sink to source one from, so it leaves this nil (Reportf
	// below no-ops on a nil Report); tests exercising that path in isolation
	// set it directly.
	Report func(format string, args ...any)

	// Ticket, TicketPath, ScratchDir, and EpicName locate the run-log.jsonl entries this
	// call logs (see eventlog.go). Ticket is the ticket's Identifier (not
	// Number), so lettered split siblings sharing a Number are distinguishable
	// in the run log.
	Ticket     string
	TicketPath string
	ScratchDir string
	EpicName   string
	// TicketData is the full ticket IterationStarted reports, so a consumer
	// rendering a chat message has the ticket's own fields (title, etc.)
	// without re-reading TicketPath itself.
	TicketData tickets.Ticket
	// VerifyCap opts this pane into the verify token cap when TicketData is a
	// verify ticket, and holds the iteration-wide state every wait of it shares.
	// Only the main iteration launch sets it; nil means no cap.
	VerifyCap *VerifyCapState
	// StartEvent/FinishEvent are the run-log event types logged when the
	// agent starts/finishes; "" skips logging that transition.
	StartEvent  string
	FinishEvent string
}

// LogLifecycleEvent appends eventType to p's run-log with p's Ticket/Pane/
// Tab, plus agentSession if known. A no-op if eventType is "".
func (p LaunchParams) LogLifecycleEvent(eventType, agentSession string) {
	if eventType == "" {
		return
	}
	p.LogAgentEvent(eventType, agentSession, "")
}

func (p LaunchParams) LogAgentEvent(eventType, agentSession, reason string) {
	_ = runlog.AppendEvent(p.ScratchDir, p.EpicName, p.AgentEvent(eventType, agentSession, reason))
}

func (p LaunchParams) AgentEvent(eventType, agentSession, reason string) runlog.Event {
	return runlog.Event{
		Type:         eventType,
		Ticket:       p.Ticket,
		Agent:        p.Agent,
		Pane:         p.Pane,
		AgentSession: agentSession,
		Cwd:          p.SessionCwd,
		Reason:       reason,
	}
}

// LogGateEvent is LogAgentEvent for a background-task gate event on taskID.
func (p LaunchParams) LogGateEvent(eventType events.Type, agentSession, taskID, reason string) {
	ev := p.AgentEvent(string(eventType), agentSession, reason)
	ev.TaskID = taskID
	_ = runlog.AppendEvent(p.ScratchDir, p.EpicName, ev)
}

// LogAgentStartEvent is LogLifecycleEvent for the launch-time start event
// only, additionally recording seq (see Event.StateChangeSeq) so a later
// collided reattach can look up this launch's baseline by AgentSession — see
// noActivitySinceLaunch.
func (p LaunchParams) LogAgentStartEvent(eventType, agentSession string, seq int) {
	if eventType == "" {
		return
	}
	_ = runlog.AppendEvent(p.ScratchDir, p.EpicName, runlog.Event{
		Type:           eventType,
		Ticket:         p.Ticket,
		Agent:          p.Agent,
		Pane:           p.Pane,
		AgentSession:   agentSession,
		Cwd:            p.SessionCwd,
		StateChangeSeq: seq,
	})
}

func (p LaunchParams) Reportf(format string, args ...any) {
	if p.Report == nil {
		return
	}
	p.Report(format, args...)
}

// EventSink returns p.Sink, or a no-op EventSink when unset — tests that build a
// LaunchParams directly to exercise pause/resume plumbing in
// isolation don't always wire one up.
func (p LaunchParams) EventSink() runlog.EventSink {
	if p.Sink == nil {
		return runlog.NoopEventSink{}
	}
	return p.Sink
}

// PlainFinishStates are the agent state values that mean "the agent's turn is
// over" for every agent kind. "blocked" needs its own quota/pause handling
// rather than being treated as finished.
var PlainFinishStates = []string{"idle", "done"}

// RunnerFinishStates is PlainFinishStates for Runner.Wait.
var RunnerFinishStates = []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}

// AlreadyFinished reports whether status (a herdr tab's current agent_status,
// e.g. from TabList) already matches one of WaitForFinish's plain-completion
// target states. "blocked" is deliberately excluded even though WaitForFinish
// treats it as a finish state of its own — a pane already sitting blocked at
// reattach still needs WaitForFinish's dwell-and-pause handling, not a bare
// skip.
func AlreadyFinished(status string) bool {
	return slices.Contains(PlainFinishStates, status)
}
