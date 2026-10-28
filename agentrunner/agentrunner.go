// Package agentrunner defines the one interface ralph-loop and the server
// drive to host a coding agent, so herdr and a native runner are two
// implementations of it. Hosting details (workspaces, tabs, nudges, dialog
// rules, pane scraping) stay inside each adapter.
package agentrunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Kind is the agent executable, e.g. "claude" or "codex".
type Kind string

// State is a session's coarse status. Idle and done are treated the same by
// callers; adapters report whichever their host distinguishes.
type State string

const (
	StateWorking State = "working"
	StateIdle    State = "idle"
	StateDone    State = "done"
	// StateBlocked means the agent waits on a prompt gx did not send. Adapters
	// answer the dialogs they know themselves; callers only see blocked plus
	// Status.BlockedReason and pause.
	StateBlocked State = "blocked"
)

// Session is an opaque handle to a hosted agent. Callers own a session when
// its label and cwd match theirs; ID and SessionID mean whatever the adapter
// needs them to mean.
type Session struct {
	Label string `json:"label"`
	// ID is the adapter's handle (herdr pane, native process key).
	ID string `json:"id"`
	// SessionID is the agent's own conversation id, when known.
	SessionID string `json:"session_id,omitempty"`
}

type StartOptions struct {
	Label string
	// Epic groups sessions for List. Labels can't be parsed back to an epic
	// (long epic names are truncated and hashed), so it's passed explicitly.
	Epic string
	Cwd  string
	Kind Kind
	Args []string
	Env  []string
	// NoAdopt makes a live session with the label ErrLabelTaken whatever its
	// cwd, so the launch always gets a fresh agent and prompt.
	NoAdopt bool
	// ResumeSessionID names a previous Claude session to continue instead of
	// starting a new one; the session keeps that id. Only the pty runner
	// resumes: headless, herdr and Codex ignore it and cold-start.
	ResumeSessionID string
}

type Status struct {
	State State
	// Turn never decreases and advances every time the agent starts a new
	// turn, so a caller can tell a prompt was picked up even if the turn
	// already finished. It may advance on other changes too: only equality
	// means anything ("no turn started since"), and it holds across a runner
	// restart, so a caller can compare it with a value it logged earlier.
	Turn          int
	SessionID     string
	BlockedReason string
	// Pending is the open tool-permission request, for runners that can name
	// it (headless). Nil when none is open.
	Pending *Pending
}

// Pending is a tool-permission request waiting for an allow or deny.
type Pending struct {
	// ID is the request id; Answer.RequestID must match it.
	ID          string
	Tool        string
	Input       json.RawMessage
	Description string
	// NotApprovable marks a tool that must never be approved by tool name:
	// plan approval and AskUserQuestion need a real answer, not an allow.
	NotApprovable bool
}

// NotApprovableTools are the tools a permission request cannot approve by name.
var NotApprovableTools = []string{"ExitPlanMode", "AskUserQuestion"}

// LoopDisallowedTools are the interactive tools no loop (ticket) agent may
// use. Agents ask through the ticket's Question section instead.
const LoopDisallowedTools = "AskUserQuestion,EnterPlanMode,ExitPlanMode"

// WithLoopDisallowedTools adds LoopDisallowedTools to the claude ban in args.
func WithLoopDisallowedTools(args []string) []string {
	return withDisallowedTools(args, LoopDisallowedTools)
}

// DesignDisallowedTools are the tools no design agent may use. Design agents
// ask in gx-grilling rounds, which the web page renders.
const DesignDisallowedTools = "AskUserQuestion"

// WithDesignDisallowedTools adds DesignDisallowedTools to the claude ban in
// args.
func WithDesignDisallowedTools(args []string) []string {
	return withDisallowedTools(args, DesignDisallowedTools)
}

// withDisallowedTools merges every claude ban in args (--disallowedTools or
// --disallowed-tools, its tools comma or space separated, or after =) into
// one flag that starts with tools, at the first ban's place or at the end.
// A ban the caller passes adds to tools; it never lifts one.
func withDisallowedTools(args []string, tools string) []string {
	merged := strings.Split(tools, ",")
	add := func(list string) {
		for _, tool := range strings.Split(list, ",") {
			if tool = strings.TrimSpace(tool); tool != "" && !slices.Contains(merged, tool) {
				merged = append(merged, tool)
			}
		}
	}
	out := make([]string, 0, len(args)+2)
	at := -1
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		if name != "--disallowedTools" && name != "--disallowed-tools" {
			out = append(out, args[i])
			continue
		}
		if at < 0 {
			at = len(out)
		}
		if hasValue {
			add(value)
			continue
		}
		for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			add(args[i])
		}
	}
	flag := []string{"--disallowedTools", strings.Join(merged, ",")}
	if at < 0 {
		return append(out, flag...)
	}
	return slices.Insert(out, at, flag...)
}

// Waiting is an agent that is not working: idle, or done.
func (s Status) Waiting() bool {
	return s.State == StateIdle || s.State == StateDone
}

// LabelTakenError is ErrLabelTaken with the holder's cwd, when the host says
// it. Err is the host's own failure.
type LabelTakenError struct {
	HolderCwd string
	Err       error
}

func (e *LabelTakenError) Error() string { return ErrLabelTaken.Error() + ": " + e.Err.Error() }
func (e *LabelTakenError) Unwrap() []error {
	return []error{ErrLabelTaken, e.Err}
}

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

// Answer replies to a blocked prompt. Text is free-form input for prompts
// that are not a plain allow/deny.
type Answer struct {
	Decision Decision
	Text     string
	// RequestID (wire request_id) names the request being answered. Headless
	// refuses an id that is not the open one; empty answers whatever is open.
	RequestID string `json:"request_id,omitempty"`
}

var (
	// ErrLabelTaken: Start found a live session already using the label.
	ErrLabelTaken = errors.New("agentrunner: label taken")
	// ErrNotDelivered: Prompt could not get the agent to start a new turn.
	// Callers start a fresh session rather than retrying.
	ErrNotDelivered = errors.New("agentrunner: prompt not delivered")
	// ErrNotReady: the session can't accept input (e.g. blocked on a dialog).
	ErrNotReady = errors.New("agentrunner: session not ready")
	// ErrTimeout: Wait gave up before the session reached a wanted state.
	ErrTimeout = errors.New("agentrunner: wait timed out")
	// ErrNotFound: the session is unknown or already stopped.
	ErrNotFound = errors.New("agentrunner: session not found")
	// ErrContextExhausted: RateLimit found the agent out of context window
	// (Codex only). It's an error, not a Status field, because detecting it
	// costs the same pane read RateLimit already makes, which Status, polled
	// far more often, should not. The wrapping error's message carries the
	// evidence the adapter found, for a pause reason.
	ErrContextExhausted = errors.New("agentrunner: context window exhausted")
	// ErrMissingCapability: the agent lacks a protocol capability the runner
	// relies on. Retrying cannot help; the agent needs upgrading.
	ErrMissingCapability = errors.New("agentrunner: agent missing capability")
	// ErrSessionDied: Status or Wait found the session gone without a Stop
	// (the agent exited, or its host crashed). Build it with SessionDied so
	// the message carries why; callers pause the ticket for repair with that message.
	ErrSessionDied = errors.New("agentrunner: session died")
	// ErrInvalidDecision: Answer got a decision the blocked prompt can't take,
	// e.g. allow on a dialog that is not a permission prompt.
	ErrInvalidDecision = errors.New("agentrunner: invalid decision")
	// ErrRequestMismatch: Answer named a RequestID that is not the open
	// permission request (already answered, cancelled or never opened).
	ErrRequestMismatch = errors.New("agentrunner: request id is not the open request")
)

// SessionDied wraps ErrSessionDied with reason, e.g. "claude exited (code 1)".
func SessionDied(reason string) error {
	return fmt.Errorf("%w: %s", ErrSessionDied, reason)
}

type Runner interface {
	// Name identifies the runner kind; it is recorded on every run it hosts.
	Name() string
	// Start launches the agent and returns once it is idle and ready for a
	// prompt. An adapter whose sessions outlive gx may instead adopt a live
	// session with the same label and cwd, returning it in whatever state it
	// is in; a live session on another cwd is ErrLabelTaken.
	Start(opts StartOptions) (Session, error)
	// Prompt returns once a new turn started, or ErrNotDelivered. For
	// "/compact" that includes any confirmation the agent asks for itself.
	Prompt(s Session, text string) error
	Status(s Session) (Status, error)
	// Wait blocks until the session is in one of states, or returns
	// ErrTimeout.
	Wait(s Session, states []State, timeout time.Duration) (Status, error)
	// Interrupt stops the current turn and keeps the session alive.
	Interrupt(s Session) error
	// Stop ends the session. Stopping an already stopped session is not an
	// error.
	Stop(s Session) error
	Find(label string) (Session, bool, error)
	List(epic string) ([]Session, error)
	// RateLimit reports when the agent's rate limit resets, if it is limited.
	// A zero resetAt means limited with an unknown reset. It returns
	// ErrContextExhausted when the agent ran out of context instead.
	RateLimit(s Session) (resetAt time.Time, limited bool, err error)
	Answer(s Session, a Answer) error
}

// HealthChecker is implemented by runners whose host can go away (herdr).
type HealthChecker interface {
	Healthy() error
}

// Cleaner is implemented by runners that keep per-agent files on disk
// (nativerunner.Headless), dropped once the iteration landed.
type Cleaner interface {
	Cleanup(label string) error
}

// Nudger is implemented by runners whose Prompt nudges a prompt the agent
// didn't pick up (herdr, pty). onNudge hears each decision
// (claudeui.NudgeDecision).
type Nudger interface {
	PromptNudged(s Session, text string, onNudge func(decision string)) error
}

// PromptNudged is r.Prompt reporting nudge decisions to onNudge; a runner
// that never nudges just prompts.
func PromptNudged(r Runner, s Session, text string, onNudge func(decision string)) error {
	if n, ok := r.(Nudger); ok {
		return n.PromptNudged(s, text, onNudge)
	}
	return r.Prompt(s, text)
}

// Cleanup drops label's leftovers when r keeps any; otherwise it is a no-op.
func Cleanup(r Runner, label string) error {
	if c, ok := r.(Cleaner); ok {
		return c.Cleanup(label)
	}
	return nil
}

// OwnerPID is a process gx knows belongs to an agent session. Start is its
// start time in Unix seconds; a pid is never reported without one, so the
// owner walk can't match a recycled pid.
type OwnerPID struct {
	PID   int
	Start int64
}

// ProcessOwner is implemented by runners that know which pids run an agent
// session. A session whose agent already ended returns no pids and no error.
type ProcessOwner interface {
	OwnerPIDs(s Session) ([]OwnerPID, error)
}

// OwnerPIDs returns the pids r knows for s; runners without ProcessOwner
// know none.
func OwnerPIDs(r Runner, s Session) ([]OwnerPID, error) {
	if o, ok := r.(ProcessOwner); ok {
		return o.OwnerPIDs(s)
	}
	return nil, nil
}

// Healthy reports r's host health; runners without a separate host are
// always healthy.
func Healthy(r Runner) error {
	if h, ok := r.(HealthChecker); ok {
		return h.Healthy()
	}
	return nil
}
