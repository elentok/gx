package ralphloop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
)

func TestPauseGate_NotPausedInitially(t *testing.T) {
	g := newPauseGate()
	if g.isPaused() {
		t.Error("isPaused() = true for a fresh gate, want false")
	}
}

func TestPauseGate_PauseMarksPausedAndRecordsReason(t *testing.T) {
	g := newPauseGate()
	g.pause("iter-01", "context occupancy breach")

	if !g.isPaused() {
		t.Error("isPaused() = false after pause(), want true")
	}
	if got := g.snapshot()["iter-01"]; got != "context occupancy breach" {
		t.Errorf(`snapshot()["iter-01"] = %q, want "context occupancy breach"`, got)
	}
}

func TestPauseGate_MultiplePausedIterations_AllStayPausedUntilOneResumeSignal(t *testing.T) {
	g := newPauseGate()
	g.pause("iter-01", "breach one")
	g.pause("iter-02", "breach two")

	if got := g.snapshot(); len(got) != 2 {
		t.Fatalf("snapshot() = %v, want both iterations recorded as paused", got)
	}

	d := Deps{
		ResumeSignaled: func(path string) (bool, error) { return true, nil },
		Sleep:          func(time.Duration) {},
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			g.waitForResume(d, "unused")
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned for both waiters")
	}

	if g.isPaused() {
		t.Error("isPaused() = true after resume, want false")
	}
}

func TestPauseGate_WaitForResume_BlocksUntilSignaled(t *testing.T) {
	g := newPauseGate()
	g.pause("iter-01", "breach")

	var mu sync.Mutex
	signaled := false
	d := Deps{
		ResumeSignaled: func(path string) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return signaled, nil
		},
		Sleep: func(time.Duration) {},
	}

	returned := make(chan struct{})
	go func() {
		g.waitForResume(d, "unused")
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("waitForResume() returned before the signal was set")
	case <-time.After(50 * time.Millisecond):
	}

	mu.Lock()
	signaled = true
	mu.Unlock()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned after the signal was set")
	}
}

func TestResume_WritesSignalFile_ConsumedOnceByDefaultResumeSignaled(t *testing.T) {
	scratchDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(scratchDir, "my-epic"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := Resume(scratchDir, "my-epic"); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	path := resumeSignalPath(scratchDir, "my-epic")
	signaled, err := defaultResumeSignaled(path)
	if err != nil {
		t.Fatalf("defaultResumeSignaled() error = %v", err)
	}
	if !signaled {
		t.Fatal("defaultResumeSignaled() = false right after Resume(), want true")
	}

	signaled, err = defaultResumeSignaled(path)
	if err != nil {
		t.Fatalf("defaultResumeSignaled() second call error = %v", err)
	}
	if signaled {
		t.Error("defaultResumeSignaled() = true on second call, want false: the signal should be consumed, not re-fired")
	}
}

func TestResume_CreatesEpicDirIfMissing(t *testing.T) {
	scratchDir := t.TempDir()

	if err := Resume(scratchDir, "brand-new-epic"); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	if _, err := os.Stat(resumeSignalPath(scratchDir, "brand-new-epic")); err != nil {
		t.Errorf("resume signal file not created: %v", err)
	}
}

// TestRun_SmartZoneBreach_PausesResumesAndKeepsSchedulingCorrect drives a
// full Run() through: iter-01 breaching the smart zone (Ctrl-C sent, ticket
// stays claimed, no new ticket scheduled while paused, other running
// iterations finish normally), a `gx ralph-loop resume` signal waking it,
// and the loop then correctly backfilling and completing the epic.
func TestRun_SmartZoneBreach_PausesResumesAndKeepsSchedulingCorrect(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** open\n",
		"03-c.md": "# C\n\n**Status:** open\n",
	})
	d, _, removed := fakeDeps()

	var mu sync.Mutex
	var createdBranches []string
	origAddWorktree := d.AddWorktree
	d.AddWorktree = func(repoDir, path, branch, base string) error {
		mu.Lock()
		createdBranches = append(createdBranches, branch)
		mu.Unlock()
		return origAddWorktree(repoDir, path, branch, base)
	}

	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	wait, started, release := gatedAgentWait(d.AgentWait)
	var breachOnce sync.Once
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if slices.Contains(opts.Until, "done") && strings.Contains(opts.Target, "iter-01") {
			breached := false
			breachOnce.Do(func() { breached = true })
			if breached {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
		}
		return wait(opts)
	}

	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if strings.Contains(cwd, "iter-01") {
			return 999999, true, nil
		}
		return 0, false, nil
	}

	sendKeysCh := make(chan []string, 1)
	d.AgentSendKeys = func(target string, keys ...string) error {
		sendKeysCh <- keys
		return nil
	}

	var resumeMu sync.Mutex
	resumeAllowed := false
	d.ResumeSignaled = func(path string) (bool, error) {
		resumeMu.Lock()
		defer resumeMu.Unlock()
		if resumeAllowed {
			resumeAllowed = false
			return true, nil
		}
		return false, nil
	}
	d.Sleep = func(time.Duration) {}

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, &out)
	}()

	// iter-01 and iter-02 both claimed and started (2 slots).
	pane2 := <-started

	keys := <-sendKeysCh
	if len(keys) == 0 || keys[0] != "ctrl-c" {
		t.Fatalf("AgentSendKeys keys = %v, want [ctrl-c]", keys)
	}

	raw01, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw01), "Status:** claimed") {
		t.Errorf("ticket 01 status = %s, want claimed (paused, not reverted or done) while paused", raw01)
	}

	// iter-02 finishes normally while iter-01 stays paused.
	release(pane2)

	// Give the scheduler a moment to (wrongly, if buggy) backfill a third
	// ticket while still paused, then confirm it didn't.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	createdSoFar := slices.Clone(createdBranches)
	mu.Unlock()
	if len(createdSoFar) != 3 { // feature worktree + iter-01 + iter-02
		t.Fatalf("worktrees created while paused = %v, want exactly [epic, iter-01, iter-02] (no backfill until resumed)", createdSoFar)
	}

	// The run-log is readable and shows the pause while the run is still
	// blocked mid-pause, not only after Run() eventually returns.
	midPauseEvents, midPauseOK, midPauseErr := readEvents(scratchDir, "epic")
	if midPauseErr != nil || !midPauseOK {
		t.Fatalf("readEvents mid-pause: ok=%v err=%v", midPauseOK, midPauseErr)
	}
	sawPause := false
	for _, ev := range midPauseEvents {
		if ev.Type == eventPausedSmartZone && ev.Ticket == 1 {
			sawPause = true
		}
	}
	if !sawPause {
		t.Errorf("mid-pause events = %+v, want a paused-smart-zone event for ticket 1", midPauseEvents)
	}

	resumeMu.Lock()
	resumeAllowed = true
	resumeMu.Unlock()

	// iter-01 re-enters its wait step post-resume and finishes normally.
	pane1 := <-started
	release(pane1)

	// Ticket 03 is now backfilled since the pause cleared.
	pane3 := <-started
	release(pane3)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 3 {
		t.Errorf("removed worktree branches = %v, want all 3 iterations removed", *removed)
	}

	if !strings.Contains(out.String(), "paused iter-01") {
		t.Errorf("output = %q, want a paused report mentioning iter-01", out.String())
	}
	if !strings.Contains(out.String(), "resumed iter-01") {
		t.Errorf("output = %q, want a resumed report mentioning iter-01", out.String())
	}
}

// TestRun_RateLimitDetected_AutoPausesAndResumesWithReprompt drives a full
// Run() through: iter-01's pane showing a Claude rate-limit message (no
// Ctrl-C — unlike a smart-zone breach, the agent isn't interrupted, it's
// already sitting blocked), the loop auto-pausing (no scheduling, other
// iterations keep running) without any `gx ralph-loop resume` call, then
// auto-detecting the reset, re-prompting iter-01's agent to continue, and
// resuming normal scheduling.
func TestRun_RateLimitDetected_AutoPausesAndResumesWithReprompt(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** open\n",
		"03-c.md": "# C\n\n**Status:** open\n",
	})
	d, prompts, removed := fakeDeps()

	var mu sync.Mutex
	var createdBranches []string
	origAddWorktree := d.AddWorktree
	d.AddWorktree = func(repoDir, path, branch, base string) error {
		mu.Lock()
		createdBranches = append(createdBranches, branch)
		mu.Unlock()
		return origAddWorktree(repoDir, path, branch, base)
	}

	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	wait, started, release := gatedAgentWait(d.AgentWait)
	var breachOnce sync.Once
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if slices.Contains(opts.Until, "done") && strings.Contains(opts.Target, "iter-01") {
			breached := false
			breachOnce.Do(func() { breached = true })
			if breached {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
		}
		return wait(opts)
	}

	var rlMu sync.Mutex
	rateLimitCleared := false
	d.ReadPaneRecent = func(pane string) (string, error) {
		if !strings.Contains(pane, "iter-01") {
			return "working on it", nil
		}
		rlMu.Lock()
		defer rlMu.Unlock()
		if rateLimitCleared {
			return "back to normal", nil
		}
		return "Claude usage limit reached", nil
	}

	sendKeysCh := make(chan []string, 1)
	d.AgentSendKeys = func(target string, keys ...string) error {
		sendKeysCh <- keys
		return nil
	}
	d.Sleep = func(time.Duration) {}

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, &out)
	}()

	// iter-01 and iter-02 both claimed and started (2 slots).
	pane2 := <-started

	select {
	case keys := <-sendKeysCh:
		t.Fatalf("AgentSendKeys called with %v, want no interrupt on a rate-limit pause (unlike smart-zone)", keys)
	case <-time.After(50 * time.Millisecond):
	}

	raw01, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw01), "Status:** claimed") {
		t.Errorf("ticket 01 status = %s, want claimed (paused, not reverted or done) while paused", raw01)
	}

	// iter-02 finishes normally while iter-01 stays paused.
	release(pane2)

	// Give the scheduler a moment to (wrongly, if buggy) backfill a third
	// ticket while still paused, then confirm it didn't.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	createdSoFar := slices.Clone(createdBranches)
	mu.Unlock()
	if len(createdSoFar) != 3 { // feature worktree + iter-01 + iter-02
		t.Fatalf("worktrees created while paused = %v, want exactly [epic, iter-01, iter-02] (no backfill until resumed)", createdSoFar)
	}

	rlMu.Lock()
	rateLimitCleared = true
	rlMu.Unlock()

	// iter-01 re-enters its wait step post-resume and finishes normally.
	pane1 := <-started
	release(pane1)

	// Ticket 03 is now backfilled since the pause cleared.
	pane3 := <-started
	release(pane3)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 3 {
		t.Errorf("removed worktree branches = %v, want all 3 iterations removed", *removed)
	}

	if !slices.Contains(*prompts, "continue") {
		t.Errorf("prompts = %v, want a \"continue\" re-prompt sent to iter-01 after the rate-limit reset", *prompts)
	}

	if !strings.Contains(out.String(), "paused iter-01") {
		t.Errorf("output = %q, want a paused report mentioning iter-01", out.String())
	}
	if !strings.Contains(out.String(), "resumed iter-01") {
		t.Errorf("output = %q, want a resumed report mentioning iter-01", out.String())
	}
}
