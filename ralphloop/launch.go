package ralphloop

import (
	"fmt"
	"slices"
	"sync"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// iterationParams are the per-ticket inputs to runIteration.
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
	Agent           AgentKind
	Skill           string
	Ticket          tickets.Ticket
	// ScratchDir locates this run's run-log.jsonl (see eventlog.go);
	// logEvent no-ops if it's empty. The epic name half of that path is
	// FeatureBranch, not a separate field — Run names the feature branch
	// after the epic, so the two are always the same value.
	ScratchDir string
	// FeatureLock serializes the only step that mutates the shared feature
	// worktree's working directory (cherry-picking a finished iteration's
	// commits onto it), so concurrently-running iterations never do so at
	// the same time.
	FeatureLock *sync.Mutex
	// WorktreeLock serializes every `git worktree add`/`git worktree remove`
	// against RepoDir: git's own worktree administrative files (.git/
	// worktrees/<name>/) aren't safe under concurrent add/remove calls on the
	// same repo, so without this two iterations creating/removing their
	// worktrees at once can corrupt each other's metadata (seen in CI as
	// "failed to read .git/worktrees/<name>/commondir: Success").
	WorktreeLock *sync.Mutex

	// SmartZone is the context-token ceiling before an iteration (or its
	// conflict-resolution agent) gets paused.
	SmartZone int
	// Gate is the pause/resume coordinator shared by every iteration in this
	// Run call.
	Gate *Gate
	// ResumeSignalPath is where a paused iteration polls for `gx ralph-loop
	// resume`.
	ResumeSignalPath string
	// Sink receives this Run call's lifecycle events, safe to call
	// concurrently from any iteration's goroutine.
	Sink EventSink
	// Report writes a line to the loop's output, safe to call concurrently.
	// Nil (a no-op) when driven through Run, which has no legacy text sink to
	// source one from — see launchAndPromptParams.Report for why this
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
func (p iterationParams) launchAndPromptParams(label, pane, tab, prompt, sessionCwd, startEvent, finishEvent string) launchAndPromptParams {
	return launchAndPromptParams{
		Label:            label,
		Agent:            p.Agent,
		Pane:             pane,
		Tab:              tab,
		Prompt:           prompt,
		SessionCwd:       sessionCwd,
		SmartZone:        p.SmartZone,
		Gate:             p.Gate,
		ResumeSignalPath: p.ResumeSignalPath,
		Sink:             p.Sink,
		Ticket:           p.Ticket.Identifier,
		TicketPath:       p.Ticket.Path,
		ScratchDir:       p.ScratchDir,
		EpicName:         p.FeatureBranch,
		StartEvent:       startEvent,
		FinishEvent:      finishEvent,
	}
}

// logTicketEvent appends eventType to p's run-log with p's ticket number
// plus the given pane/tab/session/cwd — the shared shape behind every event
// this package logs outside launchAndPrompt's own generic start/finish pair
// (needs-info, cherry-picked, conflict-hit, conflict-resolved,
// deps-installed).
func (p iterationParams) logTicketEvent(eventType, pane, tab, agentSession, cwd string) {
	p.logTicketEventReason(eventType, pane, tab, agentSession, cwd, "")
}

// logTicketEventReason is logTicketEvent plus a Reason, for events that
// carry one (currently only deps-installed, whose Reason is the install
// command run).
func (p iterationParams) logTicketEventReason(eventType, pane, tab, agentSession, cwd, reason string) {
	p.logTicketEventSHA(eventType, pane, tab, agentSession, cwd, reason, "")
}

// logTicketEventSHA is logTicketEventReason plus a SHA, for the
// cherry-picked event — the feature branch's landed tip, recorded so startup
// reconciliation can later confirm it's still reachable (see Event.SHA).
func (p iterationParams) logTicketEventSHA(eventType, pane, tab, agentSession, cwd, reason, sha string) {
	_ = logEvent(p.ScratchDir, p.FeatureBranch, Event{
		Type:         eventType,
		Ticket:       p.Ticket.Identifier,
		Agent:        p.Agent,
		Pane:         pane,
		Tab:          tab,
		AgentSession: agentSession,
		SHA:          sha,
		Cwd:          cwd,
		Reason:       reason,
	})
}

// launchAndPromptParams are the per-call inputs to launchAndPrompt.
type launchAndPromptParams struct {
	Label  string // agent name/tab label, used in error messages
	Agent  AgentKind
	Pane   string // pane id to launch the agent in and send the prompt to
	Tab    string // tab id owning Pane, recorded on logged events
	Prompt string // initial skill prompt text

	// FinishTimeoutMs bounds the final "wait for the agent to finish" step, so
	// a stuck agent surfaces as a distinct error instead of blocking forever.
	// Zero means wait indefinitely.
	FinishTimeoutMs int

	// SessionCwd is the cwd Pane's agent was launched in.
	SessionCwd string
	// SmartZone is the context-token ceiling before this agent gets paused.
	SmartZone int
	// Gate is the pause/resume coordinator shared across the whole Run call.
	Gate *Gate
	// ResumeSignalPath is where a paused agent polls for `gx ralph-loop
	// resume`.
	ResumeSignalPath string
	// Sink receives this Run call's lifecycle events, safe to call
	// concurrently.
	Sink EventSink
	// Report writes a line to the loop's output, safe to call concurrently.
	// Only recoverCodexRateLimit's Codex quota-exhaustion path still reads
	// this (waitForAttentionRecovery moved to Sink in ticket 04a, closing the
	// gap that left operator-intervention pauses with no live event) — the
	// rest of the package reports through Sink instead. Run itself has no
	// legacy text sink to source one from, so it leaves this nil (report()
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
	// StartEvent/FinishEvent are the run-log event types logged when the
	// agent starts/finishes; "" skips logging that transition.
	StartEvent  string
	FinishEvent string
}

// logLifecycleEvent appends eventType to p's run-log with p's Ticket/Pane/
// Tab, plus agentSession if known. A no-op if eventType is "".
func (p launchAndPromptParams) logLifecycleEvent(eventType, agentSession string) {
	if eventType == "" {
		return
	}
	p.logAgentEvent(eventType, agentSession, "")
}

func (p launchAndPromptParams) logAgentEvent(eventType, agentSession, reason string) {
	_ = logEvent(p.ScratchDir, p.EpicName, Event{
		Type:         eventType,
		Ticket:       p.Ticket,
		Agent:        p.Agent,
		Pane:         p.Pane,
		Tab:          p.Tab,
		AgentSession: agentSession,
		Cwd:          p.SessionCwd,
		Reason:       reason,
	})
}

func (p launchAndPromptParams) report(format string, args ...any) {
	if p.Report == nil {
		return
	}
	p.Report(format, args...)
}

// sink returns p.Sink, or a no-op EventSink when unset — tests that build a
// launchAndPromptParams directly to exercise pause/resume plumbing in
// isolation don't always wire one up.
func (p launchAndPromptParams) sink() EventSink {
	if p.Sink == nil {
		return noopEventSink{}
	}
	return p.Sink
}

// launchAndPrompt runs the shared agent lifecycle protocol: launch the agent in
// Pane, wait for it to reach idle, send Prompt and wait for it to start
// working, then wait for it to finish (idle or done) — pausing the whole
// loop via Gate if this agent's context occupancy breaches SmartZone before
// it finishes.
func launchAndPrompt(d Deps, p launchAndPromptParams) (string, error) {
	startedAgent, err := d.AgentStart(herdr.AgentStartOptions{
		Name:      p.Label,
		Kind:      string(p.Agent),
		Pane:      p.Pane,
		AgentArgs: agentArgs(p.Agent, p.ScratchDir, p.EpicName),
	})
	if err != nil {
		return "", fmt.Errorf("launching %s: %w", p.Agent, err)
	}

	if _, err := d.AgentWait(herdr.AgentWaitOptions{
		Target: p.Pane,
		Until:  []string{"idle"},
	}); err != nil {
		return "", fmt.Errorf("waiting for %s to reach idle after launch: %w", p.Agent, err)
	}

	promptedAgent, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target: p.Pane,
		Text:   p.Prompt,
		Wait:   true,
		Until:  []string{"working"},
	})
	if err != nil {
		return "", fmt.Errorf("sending initial prompt: %w", err)
	}

	// Claude normally exposes its native session ID at process startup, while
	// Codex creates/discovers its rollout only after the first prompt begins.
	// Prefer the post-prompt value when present, retaining the startup value as
	// a fallback for agents whose prompt response omits agent_session.
	sessionID := startedAgent.AgentSession
	if promptedAgent.AgentSession != "" {
		sessionID = promptedAgent.AgentSession
	}
	p.logLifecycleEvent(p.StartEvent, sessionID)
	if p.StartEvent != "" {
		p.sink().IterationStarted(p.Ticket, p.Label, p.SessionCwd, sessionID)
		emitContextOccupancy(d, p.sink(), p.Agent, p.Ticket, p.SessionCwd, sessionID)
	}

	if err := waitForFinish(d, p, sessionID); err != nil {
		return "", err
	}
	return sessionID, nil
}

// plainFinishStates are the herdr agent_status values that mean "the agent's
// turn is over" for every agent kind. waitForFinish appends "blocked" to
// these for Codex, which needs its own rate-limit/attention-recovery handling
// rather than being treated as finished.
var plainFinishStates = []string{"idle", "done"}

// alreadyFinished reports whether status (a herdr tab's current agent_status,
// e.g. from TabList) already matches one of waitForFinish's plain-completion
// target states. "blocked" is deliberately excluded even though waitForFinish
// treats it as a Codex finish state too — a pane already sitting blocked at
// reattach still needs waitForFinish's rate-limit/attention-recovery
// handling, not a bare skip.
func alreadyFinished(status string) bool {
	return slices.Contains(plainFinishStates, status)
}
