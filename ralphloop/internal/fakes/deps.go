// Package fakes holds ralphloop's test doubles: an in-memory Deps, a Runner
// that records prompts and runs hooks, and recording EventSinks. Only tests
// import it.
package fakes

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/testutil/runnerfake"
)

// StateDir is the state dir merge locks live under for the importing package's
// tests; its TestMain sets it.
var StateDir string

// ReadyTimer is a Deps.PauseTimer that has already fired: a pause test polls at
// full speed instead of waiting out a real interval.
func ReadyTimer(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

// PaneRunner hosts one idle session under label, with the given pane and
// session IDs.
func PaneRunner(label, pane, sessionID string) *runnerfake.Runner {
	r := runnerfake.NewRunner()
	r.IDs = func(string) (string, string) { return pane, sessionID }
	if _, err := r.Start(agentrunner.StartOptions{Label: label}); err != nil {
		panic(err)
	}
	return r
}

// IdleRunner hosts one idle session under label, with ID "pane-1" and
// SessionID "sess-1".
func IdleRunner(label string) *runnerfake.Runner {
	return PaneRunner(label, "pane-1", "sess-1")
}

// TestDeps is DefaultDeps for a test: real behavior everywhere except that a
// pause polls at full speed rather than on the wall clock, so a test that pauses
// unexpectedly surfaces as its own failure instead of a multi-minute wait.
func TestDeps() deps.Deps {
	return TestDepsWithOverrides(deps.DepsOverrides{})
}

// TestDepsWithOverrides is TestDeps with DepsOverrides applied, for tests
// that need to isolate HOME/CODEX_HOME/PATH without mutating process env via
// t.Setenv.
func TestDepsWithOverrides(overrides deps.DepsOverrides) deps.Deps {
	if overrides.StateDir == "" {
		overrides.StateDir = StateDir
	}
	if overrides.Home == "" {
		// Set by fixture.SetHomeEnv for the tests that point the process HOME at a fixture.
		overrides.Home = fixture.HomeDir()
	}
	d := deps.DefaultDepsWithOverrides(overrides)
	d.PauseTimer = ReadyTimer
	return d
}

// FakeDeps returns a Deps wired to in-memory fakes plus a record of prompt
// texts sent (in call order) and worktree branches removed, for assertions.
// All fake operations are safe to call concurrently, since iterations may run
// in parallel.
func FakeDeps() (d deps.Deps, prompts *[]string, removedBranches *[]string) {
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
		MergeLockStateDir: StateDir,
		Runner:            RecordingRunner{Runner: runner, record: record, hooks: &runnerHooks{}},
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
		PauseTimer: ReadyTimer,
	}
	return d, &promptsSlice, &removedSlice
}

// RecordingRunner adds Runner prompts to FakeDeps' prompts slice, so tests
// asserting the prompt sequence see initial prompts and herdr-sent ones alike.
type RecordingRunner struct {
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

// OnRunnerStop runs hook whenever a session is stopped.
func OnRunnerStop(d deps.Deps, hook func(s agentrunner.Session)) {
	h := d.Runner.(RecordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stops = append(h.stops, hook)
}

func (r RecordingRunner) Stop(s agentrunner.Session) error {
	r.hooks.mu.Lock()
	fns := slices.Clone(r.hooks.stops)
	r.hooks.mu.Unlock()
	for _, fn := range fns {
		fn(s)
	}
	return r.Runner.Stop(s)
}

// OnRunnerWait runs hook before every Runner wait of FakeDeps' d, e.g. to
// hold an agent back from finishing.
func OnRunnerWait(d deps.Deps, hook func(s agentrunner.Session)) {
	h := d.Runner.(RecordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.waits = append(h.waits, hook)
}

func (r RecordingRunner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	r.hooks.mu.Lock()
	fns := slices.Clone(r.hooks.waits)
	r.hooks.mu.Unlock()
	for _, fn := range fns {
		fn(s)
	}
	return r.Runner.Wait(s, states, timeout)
}

// OnRunnerStart runs hook before every Runner start of FakeDeps' d, after
// any hooks added earlier; a hook's error fails the start.
func OnRunnerStart(d deps.Deps, hook func(opts agentrunner.StartOptions) error) {
	h := d.Runner.(RecordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.starts = append(h.starts, hook)
}

// OnRunnerPrompt is OnRunnerStart for prompts.
func OnRunnerPrompt(d deps.Deps, hook func(s agentrunner.Session, text string) error) {
	h := d.Runner.(RecordingRunner).hooks
	h.mu.Lock()
	defer h.mu.Unlock()
	h.prompts = append(h.prompts, hook)
}

func (r RecordingRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
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

func (r RecordingRunner) Prompt(s agentrunner.Session, text string) error {
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

// FakeRunner is the runnerfake behind FakeDeps' Runner.
func FakeRunner(d deps.Deps) *runnerfake.Runner {
	return d.Runner.(RecordingRunner).Runner
}

// HostLiveAgent hosts label's agent in FakeDeps' Runner, idle, as a restart
// finds it still running from before the crash.
func HostLiveAgent(t *testing.T, d deps.Deps, label string) {
	t.Helper()
	if _, err := FakeRunner(d).Start(agentrunner.StartOptions{Label: label, Epic: "epic"}); err != nil {
		t.Fatalf("Start(%s): %v", label, err)
	}
}

// HostLiveAgentWithSession is HostLiveAgent for an agent whose native session
// id is sessionID ("" for one that has not reported any yet).
func HostLiveAgentWithSession(t *testing.T, d deps.Deps, label, sessionID string) {
	t.Helper()
	r := FakeRunner(d)
	ids := r.IDs
	r.IDs = func(l string) (string, string) {
		id, _ := ids(l)
		return id, sessionID
	}
	defer func() { r.IDs = ids }()
	HostLiveAgent(t, d, label)
}
