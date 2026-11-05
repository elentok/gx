package ralphloop

import (
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

// idleRunner hosts one idle session under label, with ID "pane-1" and
// SessionID "sess-1".
func idleRunner(label string) *runnerfake.Runner {
	return paneRunner(label, "pane-1", "sess-1")
}

// ticketAddr is the address iteration prompts name for ticket id in the
// fixture epic under scratchDir.
func ticketAddr(scratchDir, epicName, id string) string {
	return tickets.ProjectName(scratchDir) + ":" + epicName + "/" + id
}

// testDeps is DefaultDeps for a test: real behavior everywhere except that a
// pause polls at full speed rather than on the wall clock, so a test that pauses
// unexpectedly surfaces as its own failure instead of a multi-minute wait.
func testDeps() deps.Deps {
	return testDepsWithOverrides(deps.DepsOverrides{})
}

// testStateDir is the state dir merge locks live under for this package's
// tests; TestMain sets it.
var testStateDir string

// mergeLockDir is where an epic's merge lock lives under testStateDir.
func mergeLockDir(scratchDir, epic string) (string, error) {
	return mergeLockDirFor(testDeps(), scratchDir, epic)
}

func mustMergeLockDir(t *testing.T, scratchDir, epic string) string {
	t.Helper()
	dir, err := mergeLockDir(scratchDir, epic)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// testDepsWithOverrides is testDeps with DepsOverrides applied, for tests
// that need to isolate HOME/CODEX_HOME/PATH without mutating process env via
// t.Setenv.
func testDepsWithOverrides(overrides deps.DepsOverrides) deps.Deps {
	if overrides.StateDir == "" {
		overrides.StateDir = testStateDir
	}
	if overrides.Home == "" {
		// Set by fixture.SetHomeEnv for the tests that point the process HOME at a fixture.
		overrides.Home = fixture.HomeDir()
	}
	d := deps.DefaultDepsWithOverrides(overrides)
	d.PauseTimer = readyTimer
	return d
}

// withProcessEnv is the process environment with a fake herdr's entries
// layered over it, for DepsOverrides.Env.
func withProcessEnv(fakeEnv []string) []string {
	return append(os.Environ(), fakeEnv...)
}

// setProcessEnv points the real process env var key at value for the rest of
// the test, restored on cleanup via a manual os.Setenv/os.Unsetenv pair
// rather than t.Setenv (which panics once a test has called t.Parallel()).
// Only for tests driving herdrfake helpers (RegisterCodexRollout,
// NewClaudeCompact) that read real process env directly and have no
// DepsOverrides-style seam of their own — everything else should use
// testDepsWithOverrides instead. A test using this must stay non-parallel
// with any other test that also touches the same env var.
func setProcessEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("Setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// fakeDeps returns a Deps wired to in-memory fakes plus a record of prompt
// texts sent (in call order) and worktree branches removed, for assertions.
// All fake operations are safe to call concurrently, since iterations may run
// in parallel.
func fakeDeps() (d deps.Deps, prompts *[]string, removedBranches *[]string) {
	var mu sync.Mutex
	promptsSlice := []string{}
	removedSlice := []string{}
	branchByPath := map[string]string{}

	record := func(text string) {
		mu.Lock()
		promptsSlice = append(promptsSlice, text)
		mu.Unlock()
	}
	runner := runnerfake.NewRunner()
	runner.IDs = func(label string) (string, string) { return "pane-" + label, "sess-pane-" + label }
	runner.Adopt = true
	// Agents finish their turn right away.
	runner.PromptState = agentrunner.StateDone
	d = deps.Deps{
		MergeLockStateDir: testStateDir,
		Runner:            recordingRunner{Runner: runner, record: record, hooks: &runnerHooks{}},
		VerifyCodexSession: func(cwd, sessionID string) (bool, error) {
			return true, nil
		},
		WorktreeDir: func(repoDir string) (string, error) {
			return "/fake/worktrees", nil
		},
		AddWorktree: func(repoDir, path, branch, base string) error {
			mu.Lock()
			branchByPath[path] = branch
			mu.Unlock()
			return nil
		},
		RemoveWorktree: func(repoDir, path string, force bool) error {
			mu.Lock()
			removedSlice = append(removedSlice, branchByPath[path])
			mu.Unlock()
			return nil
		},
		DeleteBranch: func(repoDir, branch string) error {
			return nil
		},
		RevParse: func(dir, ref string) (string, error) {
			return "deadbeef", nil
		},
		MergeBase: func(dir, refA, refB string) (string, error) {
			return "deadbeef", nil
		},
		CommitsAhead: func(dir, fromExclusive, toRef string) (int, error) {
			return 1, nil
		},
		CommitSubjects: func(dir, fromExclusive, toRef string) ([]string, error) {
			return nil, nil
		},
		CherryPickRange: func(dir, fromExclusive, toInclusive string) error {
			return nil
		},
		CherryPickInProgress: func(dir string) (bool, error) {
			return false, nil
		},
		AbortCherryPick: func(dir string) error {
			return nil
		},
		IsAncestor: func(dir, ancestor, descendant string) (bool, error) {
			return true, nil
		},
		PatchesApplied: func(dir, upstream, base, branch string) (bool, error) {
			return false, nil
		},
		AppendTrailers: func(dir string, trailers ...history.Trailer) error {
			return nil
		},
		WorktreeExists: func(path string) (bool, error) {
			return true, nil
		},
		InstallDeps: func(path, logDir string) ([]string, error) {
			return nil, nil
		},
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 0, false, nil
		},
		ReadUnexecutedToolCall: func(cwd, sessionID string) (bool, error) {
			return false, nil
		},
		Sleep: func(time.Duration) {},
		Now:   time.Now,
		// A pause polls at full speed; a test about the pause itself replaces
		// this with a timer that scripts the human clearing the block.
		PauseTimer: readyTimer,
	}
	return d, &promptsSlice, &removedSlice
}

// recordingRunner adds Runner prompts to fakeDeps' prompts slice, so tests
// asserting the prompt sequence see initial prompts and herdr-sent ones alike.
type recordingRunner struct {
	*runnerfake.Runner
	record func(text string)
	hooks  *runnerHooks
}

type runnerHooks struct {
	mu      sync.Mutex
	starts  []func(opts agentrunner.StartOptions) error
	prompts []func(s agentrunner.Session, text string) error
	waits   []func(s agentrunner.Session)
	stops   []func(s agentrunner.Session)
}

// onRunnerStop runs hook whenever a session is stopped.
func onRunnerStop(d deps.Deps, hook func(s agentrunner.Session)) {
	h := d.Runner.(recordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stops = append(h.stops, hook)
}

func (r recordingRunner) Stop(s agentrunner.Session) error {
	r.hooks.mu.Lock()
	fns := slices.Clone(r.hooks.stops)
	r.hooks.mu.Unlock()
	for _, fn := range fns {
		fn(s)
	}
	return r.Runner.Stop(s)
}

// onRunnerWait runs hook before every Runner wait of fakeDeps' d, e.g. to
// hold an agent back from finishing.
func onRunnerWait(d deps.Deps, hook func(s agentrunner.Session)) {
	h := d.Runner.(recordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.waits = append(h.waits, hook)
}

func (r recordingRunner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	r.hooks.mu.Lock()
	fns := slices.Clone(r.hooks.waits)
	r.hooks.mu.Unlock()
	for _, fn := range fns {
		fn(s)
	}
	return r.Runner.Wait(s, states, timeout)
}

// onRunnerStart runs hook before every Runner start of fakeDeps' d, after
// any hooks added earlier; a hook's error fails the start.
func onRunnerStart(d deps.Deps, hook func(opts agentrunner.StartOptions) error) {
	h := d.Runner.(recordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.starts = append(h.starts, hook)
}

// onRunnerPrompt is onRunnerStart for prompts.
func onRunnerPrompt(d deps.Deps, hook func(s agentrunner.Session, text string) error) {
	h := d.Runner.(recordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prompts = append(h.prompts, hook)
}

func (r recordingRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	r.hooks.mu.Lock()
	fns := slices.Clone(r.hooks.starts)
	r.hooks.mu.Unlock()
	for _, fn := range fns {
		if err := fn(opts); err != nil {
			return agentrunner.Session{}, err
		}
	}
	return r.Runner.Start(opts)
}

func (r recordingRunner) Prompt(s agentrunner.Session, text string) error {
	r.hooks.mu.Lock()
	fns := slices.Clone(r.hooks.prompts)
	r.hooks.mu.Unlock()
	for _, fn := range fns {
		if err := fn(s, text); err != nil {
			return err
		}
	}
	if err := r.Runner.Prompt(s, text); err != nil {
		return err
	}
	r.record(text)
	return nil
}

// fakeRunner is the runnerfake behind fakeDeps' Runner.
func fakeRunner(d deps.Deps) *runnerfake.Runner {
	return d.Runner.(recordingRunner).Runner
}

// hostLiveAgent hosts label's agent in fakeDeps' Runner, idle, as a restart
// finds it still running from before the crash.
func hostLiveAgent(t *testing.T, d deps.Deps, label string) {
	t.Helper()
	if _, err := fakeRunner(d).Start(agentrunner.StartOptions{Label: label, Epic: "epic"}); err != nil {
		t.Fatalf("Start(%s): %v", label, err)
	}
}

// hostLiveAgentWithSession is hostLiveAgent for an agent whose native session
// id is sessionID ("" for one that has not reported any yet).
func hostLiveAgentWithSession(t *testing.T, d deps.Deps, label, sessionID string) {
	t.Helper()
	r := fakeRunner(d)
	ids := r.IDs
	r.IDs = func(l string) (string, string) {
		id, _ := ids(l)
		return id, sessionID
	}
	defer func() { r.IDs = ids }()
	hostLiveAgent(t, d, label)
}
