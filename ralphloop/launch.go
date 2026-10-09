package ralphloop

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
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
	// Model and Effort are the resolved per-agent config the launched agent
	// process starts under (see RunOptions.Agents); empty omits that one
	// flag from the launch argv.
	Model  string
	Effort string
	Skill  string
	Ticket tickets.Ticket
	// ScratchDir locates this run's run-log.jsonl (see eventlog.go);
	// logEvent no-ops if it's empty. The epic name half of that path is
	// FeatureBranch, not a separate field — Run names the feature branch
	// after the epic, so the two are always the same value.
	ScratchDir string
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
		Label:       label,
		Agent:       p.Agent,
		Model:       p.Model,
		Effort:      p.Effort,
		Pane:        pane,
		Tab:         tab,
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

// logTicketEvent appends eventType to p's run-log with p's ticket number
// plus the given pane/tab/session/cwd — the shared shape behind every event
// this package logs outside launchAndPrompt's own generic start/finish pair
// (needs-answer, cherry-picked, conflict-hit, conflict-resolved,
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

// session is the Runner session hosting the agent. Herdr-only paths
// (reattach, attach to a live agent) leave Session zero; herdrrunner's
// Session.ID is the pane id, so those still address the right agent.
func (p launchAndPromptParams) session() agentrunner.Session {
	if p.Session != (agentrunner.Session{}) {
		return p.Session
	}
	return agentrunner.Session{Label: p.Label, ID: p.Pane}
}

// launchAndPromptParams are the per-call inputs to launchAndPrompt.
type launchAndPromptParams struct {
	Label string // agent name/tab label, used in error messages
	Agent AgentKind
	// Model and Effort are the resolved per-agent config the launched agent
	// process starts under; empty omits that one flag from the launch argv.
	Model  string
	Effort string
	// Session is the Runner session hosting the agent, for a fresh launch
	// started through Deps.Runner; zero on the herdr-only paths.
	Session agentrunner.Session
	Pane    string // pane id to launch the agent in and send the prompt to
	Tab     string // tab id owning Pane, recorded on logged events
	Prompt  string // initial skill prompt text

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
	// TicketData is the full ticket IterationStarted reports, so a consumer
	// rendering a chat message has the ticket's own fields (title, etc.)
	// without re-reading TicketPath itself.
	TicketData tickets.Ticket
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
	_ = logEvent(p.ScratchDir, p.EpicName, p.agentEvent(eventType, agentSession, reason))
}

func (p launchAndPromptParams) agentEvent(eventType, agentSession, reason string) Event {
	return Event{
		Type:         eventType,
		Ticket:       p.Ticket,
		Agent:        p.Agent,
		Pane:         p.Pane,
		Tab:          p.Tab,
		AgentSession: agentSession,
		Cwd:          p.SessionCwd,
		Reason:       reason,
	}
}

// logGateEvent is logAgentEvent for a background-task gate event on taskID.
func (p launchAndPromptParams) logGateEvent(eventType events.Type, agentSession, taskID, reason string) {
	ev := p.agentEvent(string(eventType), agentSession, reason)
	ev.TaskID = taskID
	_ = logEvent(p.ScratchDir, p.EpicName, ev)
}

// logAgentStartEvent is logLifecycleEvent for the launch-time start event
// only, additionally recording seq (see Event.StateChangeSeq) so a later
// collided reattach can look up this launch's baseline by AgentSession — see
// noActivitySinceLaunch.
func (p launchAndPromptParams) logAgentStartEvent(eventType, agentSession string, seq int) {
	if eventType == "" {
		return
	}
	_ = logEvent(p.ScratchDir, p.EpicName, Event{
		Type:           eventType,
		Ticket:         p.Ticket,
		Agent:          p.Agent,
		Pane:           p.Pane,
		Tab:            p.Tab,
		AgentSession:   agentSession,
		Cwd:            p.SessionCwd,
		StateChangeSeq: seq,
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

// launchFailure carries the events kind of a failed launch up to the loop's
// catch-all park, so the park event matches the launch-failed events.
type launchFailure struct {
	Kind events.Kind
	Err  error
}

func (e *launchFailure) Error() string { return e.Err.Error() }
func (e *launchFailure) Unwrap() error { return e.Err }

// logLaunchFailed appends one launch-failed event for a failed attempt.
func (p iterationParams) logLaunchFailed(label string, attempt int, kind events.Kind, err error) {
	_ = logEvent(p.ScratchDir, p.FeatureBranch, Event{
		Type:    string(events.LaunchFailed),
		Ticket:  p.Ticket.Identifier,
		Kind:    string(kind),
		Label:   label,
		Attempt: attempt,
		Reason:  err.Error(),
	})
}

// startAndPrompt starts a session and sends its initial prompt. A prompt the
// agent never picks up (ErrNotDelivered) points at a bad session rather than a
// slow agent, so it gets exactly one fresh session before parking as
// agent_prompt_stalled. Any other prompt error (e.g. ErrNotReady on a blocked
// agent) returns with the session still live, for the blocked-pane park.
// onFail is called for every failed attempt, to log launch-failed.
//
// A label already live before Start means Start adopted our own earlier
// session (e.g. a second gx process racing the same ticket): that one is
// returned unprompted with Adopted set, since it may be mid-turn.
func startAndPrompt(r agentrunner.Runner, opts agentrunner.StartOptions, prompt string, onFail func(attempt int, kind events.Kind, err error)) (launched, error) {
	_, adopted, _ := r.Find(opts.Label)
	for attempt := 1; ; attempt++ {
		s, err := r.Start(opts)
		if err != nil {
			kind := events.IterationError
			if errors.Is(err, agentrunner.ErrLabelTaken) {
				kind = events.AgentNameTaken
			}
			err = fmt.Errorf("starting %s: %w", opts.Label, err)
			onFail(attempt, kind, err)
			return launched{}, &launchFailure{Kind: kind, Err: err}
		}
		if adopted {
			return launched{Session: s, Adopted: true}, nil
		}
		status, err := r.Status(s)
		if err != nil {
			log.Printf("reading %s status before initial prompt: %v", opts.Label, err)
		}
		err = r.Prompt(s, prompt)
		if !errors.Is(err, agentrunner.ErrNotDelivered) {
			return launched{Session: s, Baseline: status.Turn}, err
		}
		err = fmt.Errorf("sending initial prompt: %w", err)
		onFail(attempt, events.AgentPromptStalled, err)
		if stopErr := r.Stop(s); stopErr != nil {
			log.Printf("stopping undelivered session %s: %v", s.Label, stopErr)
		}
		if attempt == 2 {
			return launched{}, &launchFailure{Kind: events.AgentPromptStalled, Err: err}
		}
	}
}

// StartAndPrompt is startAndPrompt for callers outside the loop (the server's
// launch): same retry on an undelivered prompt, no launch-failed events.
func StartAndPrompt(r agentrunner.Runner, opts agentrunner.StartOptions, prompt string) (agentrunner.Session, error) {
	l, err := startAndPrompt(r, opts, prompt, func(int, events.Kind, error) {})
	return l.Session, err
}

// launched is a session startAndPrompt started and prompted, or adopted.
type launched struct {
	agentrunner.Session
	Adopted bool
	// Baseline is the session's Turn before its initial prompt: the
	// stalled-since-launch baseline (see noActivitySinceLaunch). Zero when
	// adopted or unknown.
	Baseline int
}

// promptedLaunch is the post-prompt half of a launch for a session started
// and prompted through Deps.Runner, with baseline its pre-prompt Turn.
func promptedLaunch(d Deps, p launchAndPromptParams, baseline int) (string, error) {
	status, err := d.Runner.Status(p.Session)
	if err != nil {
		log.Printf("reading %s status after initial prompt: %v", p.Label, err)
	}
	// Codex only knows its session id once the first prompt begins.
	sessionID := status.SessionID
	if sessionID == "" {
		sessionID = p.Session.SessionID
	}
	return startedLaunch(d, p, sessionID, baseline)
}

// startedLaunch logs and reports a freshly prompted agent's start, with seq as
// its stalled-since-launch baseline, then waits for it to finish.
func startedLaunch(d Deps, p launchAndPromptParams, sessionID string, seq int) (string, error) {
	p.reportStarted(d, sessionID, seq)
	return waitLaunched(d, p, sessionID)
}

func (p launchAndPromptParams) reportStarted(d Deps, sessionID string, seq int) {
	p.logAgentStartEvent(p.StartEvent, sessionID, seq)
	if p.StartEvent != "" {
		p.sink().IterationStarted(p.TicketData, p.Label, p.SessionCwd, sessionID, p.Agent, p.Pane, p.Tab)
		emitContextOccupancy(d, p.sink(), p.Agent, p.Ticket, p.SessionCwd, sessionID)
	}
}

// adoptedLaunch takes over p.Session, a live session startAndPrompt adopted
// unprompted. An idle one whose turn hasn't moved past its logged launch
// baseline (see noActivitySinceLaunch) never got its prompt, so it gets it
// now; an idle one that has moved is already finished; anything else is
// waited out.
func adoptedLaunch(d Deps, p launchAndPromptParams) (string, error) {
	status, err := d.Runner.Status(p.Session)
	if err != nil {
		return "", fmt.Errorf("reading adopted %s status: %w", p.Label, err)
	}
	sessionID := cmp.Or(status.SessionID, p.Session.SessionID)
	// Seq 0 keeps the original launch's baseline the one noActivitySinceLaunch
	// finds.
	p.reportStarted(d, sessionID, 0)
	if alreadyFinished(string(status.State)) {
		if !noActivitySinceLaunch(p.ScratchDir, p.EpicName, sessionID, status.Turn) {
			p.logLifecycleEvent(p.FinishEvent, sessionID)
			return sessionID, nil
		}
		if err := d.Runner.Prompt(p.Session, p.Prompt); err != nil {
			return "", fmt.Errorf("sending initial prompt to stalled adopted session: %w", err)
		}
	}
	return waitLaunched(d, p, sessionID)
}

// waitLaunched is waitForFinish keeping sessionID on a blocked-pane park,
// whose caller still records the live session.
func waitLaunched(d Deps, p launchAndPromptParams, sessionID string) (string, error) {
	if err := waitForFinish(d, p, sessionID); err != nil {
		if errors.Is(err, errBlockedPaneParked) {
			return sessionID, err
		}
		return "", err
	}
	return sessionID, nil
}

// noActivitySinceLaunch reports whether a pane's current state_change_seq
// still matches the baseline stamped on its own iteration-started event (see
// logAgentStartEvent), found in the epic's run log by matching agentSession.
// adoptedLaunch uses it as its "has this session done anything since this
// iteration launched it" check. A missing log, an untracked session, or no
// matching event all fall through to false — the safe default of trusting the
// caller's existing behavior.
func noActivitySinceLaunch(scratchDir, epicName, agentSession string, currentSeq int) bool {
	if agentSession == "" {
		return false
	}
	evs, ok, err := ReadEvents(scratchDir, epicName)
	if !ok || err != nil {
		return false
	}
	for _, ev := range evs {
		if ev.Type == string(events.IterationStarted) && ev.AgentSession == agentSession && ev.StateChangeSeq != 0 {
			return currentSeq == ev.StateChangeSeq
		}
	}
	return false
}

// plainFinishStates are the agent state values that mean "the agent's turn is
// over" for every agent kind. "blocked" needs its own quota/park handling
// rather than being treated as finished.
var plainFinishStates = []string{"idle", "done"}

// runnerFinishStates is plainFinishStates for Runner.Wait.
var runnerFinishStates = []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}

// alreadyFinished reports whether status (a herdr tab's current agent_status,
// e.g. from TabList) already matches one of waitForFinish's plain-completion
// target states. "blocked" is deliberately excluded even though waitForFinish
// treats it as a finish state of its own — a pane already sitting blocked at
// reattach still needs waitForFinish's dwell-and-park handling, not a bare
// skip.
func alreadyFinished(status string) bool {
	return slices.Contains(plainFinishStates, status)
}
