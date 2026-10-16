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
	// Watch follows the session whose child is pid, spawned at spawned,
	// until stop closes.
	Watch(pid int, spawned time.Time, stop <-chan struct{}) (Watcher, error)
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

// Watcher reports what a Watch has seen so far.
type Watcher interface {
	Status() Status
	// ConversationID is "" until the harness reports one.
	ConversationID() string
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

// Lookup returns the harness for kind.
func Lookup(kind agentrunner.Kind) (Harness, error) {
	switch kind {
	case Claude:
		return claudeHarness{}, nil
	}
	return nil, fmt.Errorf("unknown harness %q", kind)
}
