package servertest

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

// holderArg is the first argument that runs the test binary as a holder. An
// argument, not an env var: the holder's child inherits its env.
const holderArg = "servertest-agent-holder"

// TestPidEnv is the pid of the test process a test holder exits with, so a
// test killed before its cleanup leaves no holder running. TestMain sets it.
const TestPidEnv = "GX_SERVERTEST_PID"

// WithTestHolder hosts interactive sessions in the test binary itself, whose
// TestMain must call RunSessionProcesses. Pair it with the fake harness
// (agentfake.HarnessKind) so no session runs claude.
func WithTestHolder() func(*server.Config) {
	return func(c *server.Config) { c.Holder = []string{os.Args[0], holderArg} }
}

// WithChatHarness makes the session chat read transcripts of kind's sessions
// instead of claude's.
func WithChatHarness(kind string) func(*server.Config) {
	return func(c *server.Config) { c.ChatHarness = kind }
}

// WithChatPollInterval shortens how often a working session's transcript size
// is checked.
func WithChatPollInterval(d time.Duration) func(*server.Config) {
	return func(c *server.Config) { c.ChatPollInterval = d }
}

// RunSessionProcesses registers the fake harness and, when the test binary
// was started as a session's holder or as the fake harness's program, runs
// that and exits. Call it first in TestMain.
func RunSessionProcesses() {
	agentfake.RegisterHarness()
	if len(os.Args) > 1 && os.Args[1] == holderArg {
		if pid, err := strconv.Atoi(os.Getenv(TestPidEnv)); err == nil {
			go exitWithTest(pid)
		}
		if err := runTestHolder(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	// After the holder check: the holder's env carries the program's marker.
	if os.Getenv(agentfake.ProgramEnv) == "1" {
		agentfake.Program()
	}
	// The holders this test process starts inherit it, unless a test set one.
	if os.Getenv(TestPidEnv) == "" {
		_ = os.Setenv(TestPidEnv, strconv.Itoa(os.Getpid()))
	}
}

// exitWithTest ends the holder once the test process pid is gone. The
// holder's child goes with it, its pty hung up.
func exitWithTest(pid int) {
	for range time.Tick(200 * time.Millisecond) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			os.Exit(1)
		}
	}
}

// runTestHolder parses `gx agent-holder`'s flags.
func runTestHolder(args []string) error {
	fs := flag.NewFlagSet(holderArg, flag.ContinueOnError)
	var dir, kind string
	var opts nativerunner.HolderOptions
	fs.String("session-id", "", "")
	fs.StringVar(&kind, "harness", "", "")
	fs.StringVar(&opts.GXSession, "gx-session", "", "")
	fs.IntVar(&opts.Cols, "cols", 0, "")
	fs.IntVar(&opts.Rows, "rows", 0, "")
	fs.StringVar(&dir, "dir", "", "")
	fs.BoolVar(&opts.Trust, "trust", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return nativerunner.RunHarnessHolder(dir, agentrunner.Kind(kind), opts, fs.Args())
}

// stopSessionHolders stops every live holder under dir.
func stopSessionHolders(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		c, err := nativerunner.DialHolder(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		_ = c.Stop()
	}
}
