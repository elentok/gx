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

// TestRun_SmartZoneBreach_AutoRecoversWithoutBlockingScheduler drives a full
// Run() through: iter-01 breaching the smart zone (Ctrl-C sent, then a
// `/compact` prompt, then a finish-up prompt mentioning the effective
// --smart-zone value), while the scheduler never blocks on it — other
// iterations keep running and backfilling — and the loop then correctly
// completes the epic once iter-01 re-enters its wait step and finishes.
func TestRun_SmartZoneBreach_AutoRecoversWithoutBlockingScheduler(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# C\n",
	})
	d, _, removed := fakeDeps()

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
		if strings.Contains(cwd, "epic-item-01") {
			return 999999, true, nil
		}
		return 0, false, nil
	}

	sendKeysCh := make(chan []string, 1)
	d.AgentSendKeys = func(target string, keys ...string) error {
		sendKeysCh <- keys
		return nil
	}

	promptCh := make(chan string, 8)
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCh <- opts.Text
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
	}

	d.ResumeSignaled = func(path string) (bool, error) { return false, nil }
	d.Sleep = func(time.Duration) {}

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, NewTextEventSink(&out))
	}()

	keys := <-sendKeysCh
	if len(keys) == 0 || keys[0] != "ctrl+c" {
		t.Fatalf("AgentSendKeys keys = %v, want [ctrl+c]", keys)
	}

	// Drain the two iterations' initial launch prompts (order not
	// guaranteed) before the breach recovery's own /compact prompt shows up.
	for {
		if got := <-promptCh; got == "/compact" {
			break
		}
	}
	finishPrompt := <-promptCh
	if !strings.Contains(finishPrompt, "110000") {
		t.Errorf("finish-up prompt = %q, want it to mention the effective --smart-zone value 110000", finishPrompt)
	}
	if !strings.Contains(finishPrompt, "implement") {
		t.Errorf("finish-up prompt = %q, want it to reference the implement skill", finishPrompt)
	}

	raw01, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw01), "status: claimed") {
		t.Errorf("ticket 01 status = %s, want claimed while recovering", raw01)
	}

	// iter-01 and iter-02 both reach their gated wait (2 slots) — order
	// between iter-02's ordinary registration and iter-01's post-recovery
	// re-registration isn't guaranteed, since nothing blocks iter-01
	// between the breach and re-entering the poll loop anymore.
	var pane1, pane2 string
	for range 2 {
		p := <-started
		if strings.Contains(p, "iter-01") {
			pane1 = p
		} else {
			pane2 = p
		}
	}

	// iter-02 finishes and ticket 03 backfills immediately, even though
	// iter-01 is still mid-recovery — proving the scheduler was never
	// blocked by the smart-zone breach (no Gate.pause on this path).
	release(pane2)
	pane3 := <-started
	release(pane1)
	release(pane3)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 3 {
		t.Errorf("removed worktree branches = %v, want all 3 iterations removed", *removed)
	}

	if !strings.Contains(out.String(), "ticket 01: context budget exceeded; compacting...") {
		t.Errorf("output = %q, want a compacting report for ticket 01", out.String())
	}
	if !strings.Contains(out.String(), "ticket 01: compacted; telling the agent to finish up...") {
		t.Errorf("output = %q, want a finish-up report for ticket 01", out.String())
	}
}

// TestRun_SmartZoneBreach_RepeatsWithNoRetryCap drives a full Run() through
// iter-01 breaching the smart zone twice in a row before finally settling:
// each breach fires its own Ctrl-C -> /compact -> finish-up cycle, and the
// scheduler's Gate is never paused by either one, matching the "no retry
// cap" and "Gate.isPaused() stays false throughout" requirements that
// distinguish this recovery path from rate-limit/needs-attention pauses.
func TestRun_SmartZoneBreach_RepeatsWithNoRetryCap(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()

	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	wait, started, release := gatedAgentWait(d.AgentWait)
	var breaches int
	var breachMu sync.Mutex
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if slices.Contains(opts.Until, "done") && strings.Contains(opts.Target, "iter-01") {
			breachMu.Lock()
			fire := breaches < 2
			if fire {
				breaches++
			}
			breachMu.Unlock()
			if fire {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
		}
		return wait(opts)
	}

	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 999999, true, nil
	}

	sendKeysCh := make(chan []string, 8)
	d.AgentSendKeys = func(target string, keys ...string) error {
		sendKeysCh <- keys
		return nil
	}

	promptCh := make(chan string, 8)
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCh <- opts.Text
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
	}

	gate := NewGate()
	d.ResumeSignaled = func(path string) (bool, error) { return false, nil }
	d.Sleep = func(time.Duration) {}

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 1, Gate: gate,
		}, d, NewTextEventSink(&out))
	}()

	for i := range 2 {
		keys := <-sendKeysCh
		if len(keys) == 0 || keys[0] != "ctrl+c" {
			t.Fatalf("breach %d: AgentSendKeys keys = %v, want [ctrl+c]", i, keys)
		}
		// Drain the iteration's own initial launch prompt (only present
		// ahead of the very first breach) before the recovery's /compact.
		for {
			if got := <-promptCh; got == "/compact" {
				break
			}
		}
		if got := <-promptCh; !strings.Contains(got, "110000") {
			t.Fatalf("breach %d: finish-up prompt = %q, want it to mention 110000", i, got)
		}
		if gate.isPaused() {
			t.Fatalf("breach %d: gate.isPaused() = true, want the scheduler never blocked by smart-zone recovery", i)
		}
	}

	pane1 := <-started
	release(pane1)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the single iteration removed", *removed)
	}
}
