package ralphloop

import (
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/supervise"
	"github.com/elentok/gx/tickets"
)

// iterationParams are the per-ticket inputs to an iteration's launch and finish.
type iterationParams struct {
	WorkspaceID string
	RepoDir     string
	// WorktreeDir is the directory this Run call's iteration worktrees live
	// in (git.Repo.LinkedWorktreeDir for RepoDir's repo), resolved once in
	// Run and threaded through so every iteration's path is
	// filepath.Join(WorktreeDir, iterLabel(ticketNumber)) — deterministic,
	// so a reattached iteration can recompute it without asking herdr.
	WorktreeDir     string
	FeatureWorktree string
	FeatureBranch   string
	Agent           agent.AgentKind
	// Runner and Label name the agentrunner and iteration label hosting the
	// agent, recorded on the server finish path's pause events.
	Runner string
	Label  string
	// Model and Effort are the resolved per-agent config the launched agent
	// process starts under (see agent.ResolveTier); empty omits that one
	// flag from the launch argv.
	Model  string
	Effort string
	Skill  string
	// KeepAskedAgent leaves the worktree, pane and session in place when the
	// agent's paused (answer) report is adopted: the agent waits idle for the answer.
	KeepAskedAgent bool
	Ticket         tickets.Ticket
	// ScratchDir locates this run's run-log.jsonl (see eventlog.go);
	// runlog.AppendEvent no-ops if it's empty. The epic name half of that path is
	// FeatureBranch, not a separate field — Run names the feature branch
	// after the epic, so the two are always the same value.
	ScratchDir string

	// SmartZone is the context-token ceiling before an iteration (or its
	// conflict-resolution agent) gets paused.
	SmartZone int
	// verifyCap is shared by every wait of this iteration; see
	// supervise.LaunchParams.VerifyCap. Always non-nil.
	verifyCap *supervise.VerifyCapState
	// Gate is the pause/resume coordinator shared by every iteration in this
	// Run call.
	Gate *supervise.Gate
	// Sink receives this Run call's lifecycle events, safe to call
	// concurrently from any iteration's goroutine.
	Sink runlog.EventSink
	// Report writes a line to the loop's output, safe to call concurrently.
	// Nil (a no-op) when driven through Run, which has no legacy text sink to
	// source one from — see supervise.LaunchParams.Report for why this
	// package still carries the field forward instead of dropping it.
	Report func(format string, args ...any)
}

// launchAndPromptParams builds the launchAndPrompt call for one pane in this
// iteration (the iteration's own pane, or its conflict-resolution pane),
// carrying over the smart-zone guardrail fields every pane in an iteration
// shares. startEvent/finishEvent name the run-log event types logged when
// the agent starts/finishes (see eventlog.go); pass "" for either to skip
// logging that transition (the conflict-resolution pane logs conflict-hit/
// conflict-resolved itself instead, around the generic start/finish here).
func (p iterationParams) launchAndPromptParams(label, pane, prompt, sessionCwd, startEvent, finishEvent string) supervise.LaunchParams {
	return supervise.LaunchParams{
		Label:       label,
		Agent:       p.Agent,
		Model:       p.Model,
		Effort:      p.Effort,
		Pane:        pane,
		Prompt:      prompt,
		SessionCwd:  sessionCwd,
		SmartZone:   p.SmartZone,
		Gate:        p.Gate,
		Sink:        p.Sink,
		Ticket:      p.Ticket.Identifier,
		TicketPath:  p.Ticket.Path,
		TicketData:  p.Ticket,
		ScratchDir:  p.ScratchDir,
		EpicName:    p.FeatureBranch,
		StartEvent:  startEvent,
		FinishEvent: finishEvent,
	}
}

// agentEvent is the pause event fields naming the iteration's agent.
func (p iterationParams) agentEvent(pane, sessionID, cwd string) runlog.Event {
	return runlog.Event{Agent: p.Agent, Runner: p.Runner, Label: p.Label, Pane: pane, AgentSession: sessionID, Cwd: cwd}
}

// logTicketEvent appends eventType to p's run-log with p's ticket number
// plus the given pane/tab/session/cwd — the shared shape behind every event
// this package logs outside launchAndPrompt's own generic start/finish pair
// (paused (answer), cherry-picked, conflict-hit, conflict-resolved,
// deps-installed).
func (p iterationParams) logTicketEvent(eventType, pane, agentSession, cwd string) {
	p.logTicketEventReason(eventType, pane, agentSession, cwd, "")
}

// logTicketEventReason is logTicketEvent plus a Reason, for events that
// carry one (currently only deps-installed, whose Reason is the install
// command run).
func (p iterationParams) logTicketEventReason(eventType, pane, agentSession, cwd, reason string) {
	p.logTicketEventSHA(eventType, pane, agentSession, cwd, reason, "")
}

// logTicketEventSHA is logTicketEventReason plus a SHA, for the
// cherry-picked event — the feature branch's merged tip, recorded so startup
// reconciliation can later confirm it's still reachable (see Event.SHA).
func (p iterationParams) logTicketEventSHA(eventType, pane, agentSession, cwd, reason, sha string) {
	_ = runlog.AppendEvent(p.ScratchDir, p.FeatureBranch, runlog.Event{
		Type:         eventType,
		Ticket:       p.Ticket.Identifier,
		Agent:        p.Agent,
		Pane:         pane,
		AgentSession: agentSession,
		SHA:          sha,
		Cwd:          cwd,
		Reason:       reason,
	})
}

// logLaunchFailed appends one launch-failed event for a failed attempt.
func (p iterationParams) logLaunchFailed(label string, attempt int, kind events.Kind, err error) {
	_ = runlog.AppendEvent(p.ScratchDir, p.FeatureBranch, runlog.Event{
		Type:    string(events.LaunchFailed),
		Ticket:  p.Ticket.Identifier,
		Kind:    string(kind),
		Label:   label,
		Attempt: attempt,
		Reason:  err.Error(),
	})
}

// logPromptNudged appends one prompt-nudged event for a nudge decision.
func (p iterationParams) logPromptNudged(label string, attempt int, decision string) {
	_ = runlog.AppendEvent(p.ScratchDir, p.FeatureBranch, runlog.Event{
		Type:    string(events.PromptNudged),
		Ticket:  p.Ticket.Identifier,
		Label:   label,
		Attempt: attempt,
		Reason:  decision,
	})
}
