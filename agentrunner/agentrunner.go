// Package agentrunner defines the one interface ralph-loop and the server
// drive to host a coding agent, so herdr and a native runner are two
// implementations of it. Hosting details (workspaces, tabs, nudges, dialog
// rules, pane scraping) stay inside each adapter.
package agentrunner

import (
	"errors"
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
	// Status.BlockedReason and park.
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
	// evidence the adapter found, for a park reason.
	ErrContextExhausted = errors.New("agentrunner: context window exhausted")
	// ErrMissingCapability: the agent lacks a protocol capability the runner
	// relies on. Retrying cannot help; the agent needs upgrading.
	ErrMissingCapability = errors.New("agentrunner: agent missing capability")
)

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

// Cleanup drops label's leftovers when r keeps any; otherwise it is a no-op.
func Cleanup(r Runner, label string) error {
	if c, ok := r.(Cleaner); ok {
		return c.Cleanup(label)
	}
	return nil
}

// Healthy reports r's host health; runners without a separate host are
// always healthy.
func Healthy(r Runner) error {
	if h, ok := r.(HealthChecker); ok {
		return h.Healthy()
	}
	return nil
}
