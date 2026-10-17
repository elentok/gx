// Package harness is what runs inside an interactive agent session: how to
// launch and resume it, how to read its status, its startup dialogs and its
// display name. A runner decides how a session is hosted; a harness decides
// what runs in it.
package harness

import (
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// Harness is one agent executable's side of an interactive session.
type Harness interface {
	// Launch is the command for a new session.
	Launch(opts LaunchOptions) Command
	// Resume is the command for a saved conversation; ok is false when the
	// harness can't resume.
	Resume(conversationID string, opts LaunchOptions) (cmd Command, ok bool)
	// Watch follows the session whose child is pid, spawned at spawned and
	// shown on scr, until stop closes.
	Watch(pid int, spawned time.Time, scr Screen, stop <-chan struct{}) (Watcher, error)
	// StartupScreen classifies a pre-session screen: the scrolled-off tail
	// and the visible rows.
	StartupScreen(scrolled, rows []string) Startup
	// DisplayName reads the session's name from its raw terminal output.
	DisplayName(raw []byte) (string, bool)
}

// LaunchOptions are the caller's parts of a launch.
type LaunchOptions struct {
	// SessionID is the conversation id a new session starts with.
	SessionID string
	// Args follow the harness's own args.
	Args []string
	// Env is added to the cleaned-up environment.
	Env []string
}

// Command is what to run for a session.
type Command struct {
	Name string
	Args []string
	Env  []string
}

// Screen is the session's terminal as its host renders it.
type Screen interface {
	Rows() []string
	ScrollbackTail(n int) []string
}

// Watcher reports what a Watch has seen so far.
type Watcher interface {
	Status() Status
	// ConversationID is "" until the harness reports one.
	ConversationID() string
	// Program takes each OSC 7501 report the child writes, in order.
	Program(r ProgramReport)
	// OnChange sets f, called after each change to Status or ConversationID
	// the Watcher sees apart from the child's output, such as a file it
	// follows. A change the output drives may skip it.
	OnChange(f func())
}

// PidStatusWatcher is a Watcher that follows a claude-style pid status file
// and can freeze it. A host checks Watch's result for it: only such a watcher
// backs the frozen status and deny-reason ops.
type PidStatusWatcher interface {
	Watcher
	// PidState returns the last status read, nil before the file first
	// appears, and the turn count.
	PidState() (*PidStatus, int)
	// Hold freezes what PidState and Status report until Release.
	Hold()
	Release()
	// WaitLive waits up to timeout for the live status to meet cond and
	// returns the live turn it met it at.
	WaitLive(timeout time.Duration, cond func(s PidStatus, turn int) bool) (int, bool)
}

// State is a Session status, less Exited: the server derives that from
// process death.
type State int

const (
	Starting State = iota
	Working
	Idle
	Blocked
	RateLimited
)

// BlockedKind is what a Blocked session waits on.
type BlockedKind string

const (
	BlockedPermission BlockedKind = "permission"
	BlockedQuestion   BlockedKind = "question"
	// BlockedDialog is a pre-session dialog.
	BlockedDialog BlockedKind = "dialog"
)

// Status is a Session status as a harness reports it.
type Status struct {
	State State
	// Background is set while Working with a background task running.
	Background bool
	// Cancelled is set while Idle after an Esc-cancel rather than a finished
	// turn.
	Cancelled bool
	// Blocked and Message are set while Blocked.
	Blocked BlockedKind
	Message string
}

// Dialog is a pre-session dialog a harness can answer by key.
type Dialog int

const (
	NoDialog Dialog = iota
	TrustDialog
)

// Startup is a classified pre-session screen; Key answers Dialog.
type Startup struct {
	Dialog Dialog
	Key    string
}

// Claude is the claude harness's name.
const Claude agentrunner.Kind = "claude"

// registered holds harnesses added at run time. Only test binaries add any,
// so a fake never ships in the production registry.
var registered = map[agentrunner.Kind]Harness{}

// Register adds h as kind. It is not safe to call while sessions run: call
// it from TestMain, before any Lookup.
func Register(kind agentrunner.Kind, h Harness) {
	registered[kind] = h
}

// Lookup returns the harness for kind.
func Lookup(kind agentrunner.Kind) (Harness, error) {
	switch kind {
	case Claude:
		return claudeHarness{}, nil
	}
	if h, ok := registered[kind]; ok {
		return h, nil
	}
	return nil, fmt.Errorf("unknown harness %q", kind)
}
