package holder

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
)

// A test binary doubles as the holder and as a launcher that starts the
// holder detached and exits at once, like the server going away. Its
// TestMain calls ServeTestBinary first. The tests of every package that
// starts holders share this, so it lives here and not in a _test file.
const (
	HolderEnv       = "GX_TEST_HOLDER"
	HolderLaunchEnv = "GX_TEST_HOLDER_LAUNCH"
	// HolderNoOpsEnv makes the holder an older one, serving only core ops.
	HolderNoOpsEnv = "GX_TEST_HOLDER_NO_OPS"
	// HolderDenyTimeoutEnv overrides the holder's deny-reason timeout.
	HolderDenyTimeoutEnv = "GX_TEST_HOLDER_DENY_TIMEOUT"
	// HolderOwnerEnv is the pid of the test binary that started the holder.
	// The holder is detached on purpose, so a test run killed before its
	// cleanup would leave it, and its claude, running for good. The holder
	// watches this pid and exits with it.
	HolderOwnerEnv = "GX_TEST_HOLDER_OWNER"
)

// ServeTestBinary runs the holder or the launcher role the environment asks
// for and reports whether it did, so TestMain can return.
func ServeTestBinary() bool {
	switch {
	case os.Getenv(HolderLaunchEnv) == "1":
		launchTestHolder()
		return true
	case os.Getenv(HolderEnv) == "1":
		serveTestHolder()
		return true
	}
	return false
}

// exitWithOwner kills the holder and its child once the owner pid is gone.
func exitWithOwner(owner int) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for syscall.Kill(owner, 0) == nil {
		<-tick.C
	}
	_ = exec.Command("pkill", "-KILL", "-P", strconv.Itoa(os.Getpid())).Run()
	os.Exit(1)
}

// serveTestHolder takes the gx agent-holder flags: --session-id <id> --dir <dir> -- <cmd...>.
func serveTestHolder() {
	args := os.Args[1:]
	dir := args[slices.Index(args, "--dir")+1]
	// The child may be this binary too, as the TUI fake.
	_ = os.Unsetenv(HolderEnv)
	if owner, err := strconv.Atoi(os.Getenv(HolderOwnerEnv)); err == nil && owner > 0 {
		go exitWithOwner(owner)
	}
	if os.Getenv(HolderNoOpsEnv) == "1" {
		holderOptionalOps = nil
	}
	if d, err := time.ParseDuration(os.Getenv(HolderDenyTimeoutEnv)); err == nil {
		holderDenyTimeout = d
	}
	kind := harness.Claude
	if i := slices.Index(args, "--harness"); i >= 0 {
		kind = agentrunner.Kind(args[i+1])
	}
	if err := RunHarnessHolder(dir, kind, HolderOptions{}, args[slices.Index(args, "--")+1:]); err != nil {
		// Like the real command, whose error cobra prints.
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func launchTestHolder() {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), HolderEnv+"=1", HolderLaunchEnv+"=")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString(strconv.Itoa(cmd.Process.Pid))
}
