// Package servertest is seam A: an in-process server on a temp state dir,
// temp socket and temp ticket store, driven only through the API client. Start
// changes no process env, so harnesses may run in parallel subtests; the
// package's TestMain must call herdrfake.RunHelperProcess.
//
// # The runner rule
//
// A test that sets no runner runs on the PTY runner, built the production way
// over holder processes and a TUI fake; OnTurn runs a hook for each agent turn
// and Starts records each launch. Pick another runner only for a reason:
//
//   - PTY (the default): the test needs a real agent turn.
//   - runnerfake (WithRunner, WithRunnerFor): the test sets a runner state
//     directly, or checks only what the server passed to Start.
//   - herdr (WithHerdr): herdr is the subject.
//
// The fake herdr executable is first in PATH of every server's Config.Env but
// fails any call unless the test asked for WithHerdr, so a stray herdr call
// fails the test and names the command.
package servertest

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/herdrfake"
)

// Harness is a running test server.
type Harness struct {
	Client *apiclient.Client
	Server *server.Server // for reading the run registry; drive everything else through Client
	Herdr  *herdrfake.State
	// Disks is the free-space source, unless an option set its own FreeSpace.
	Disks *FakeDisks
	// Programs is the PATH lookup, unless an option set its own LookPath.
	Programs *Programs
	// System is the CPU and memory source, unless an option set its own SystemHealth.
	System *FakeSystem
	// Processes is the process table source, unless an option set its own ProcessSource.
	Processes   *FakeProcesses
	StateDir    string
	SessionsDir string // the interactive sessions' state dirs
	// ConversationsDir holds the linked sessions' conversation records.
	ConversationsDir string
	TicketStore      string
	TCPAddr          string // empty unless started with StartWithTCP
	// Banners records the OS banners the server sent, unless an option set
	// its own Notifier. A harness never shows a real banner.
	Banners *Banners
	// Sounds records the session sounds played, unless an option set its own
	// Player.
	Sounds    *Sounds
	herdrDown atomic.Bool

	extraRunners []agentrunner.Runner
	// pty is set when the harness builds the PTY runner itself; turn is the
	// OnTurn hook it runs on a prompt.
	pty  bool
	turn atomic.Pointer[func(*Turn)]
	// tb reports a failure from a goroutine that is not the test's.
	tb testing.TB
	// failStart, stops and stopGate back FailNextStart, Stops and BlockStops.
	failStart atomic.Pointer[error]
	stops     atomic.Int32
	stopGate  atomic.Pointer[chan struct{}]

	startsMu sync.Mutex
	starts   []AgentStart
	// Stop does what SIGTERM does to the real server: shut down, release the
	// lock, and return Serve's result. Safe to call more than once.
	Stop func() error

	cfg server.Config
}

// WithConfigFile points the server's config reload at path.
func WithConfigFile(path string) func(*server.Config) {
	return func(c *server.Config) { c.ConfigPath = path }
}
