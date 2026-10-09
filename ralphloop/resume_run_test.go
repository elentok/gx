package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/herdr"
)

// TestRun_SmartZoneBreach_AutoRecoversWithoutBlockingScheduler drives a full
// Run() through: iter-01 breaching the smart zone (Ctrl-C sent, then a
// `/compact` prompt, then a finish-up prompt mentioning the effective
// --smart-zone value), while the scheduler never blocks on it — other
// iterations keep running and backfilling — and the loop then correctly
// completes the epic once iter-01 re-enters its wait step and finishes.
func TestRun_SmartZoneBreach_AutoRecoversWithoutBlockingScheduler(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: implement\n---\n# C\n",
	})
	d, _, removed := fakeDeps()

	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	promptCh := make(chan string, 8)
	onRunnerPrompt(d, func(_ agentrunner.Session, text string) error {
		promptCh <- text
		return nil
	})

	g := newGatedRunner(d.Runner)
	g.finishPollsOnly = true
	var breachOnce sync.Once
	g.timeOut = func(label string) bool {
		breached := false
		if strings.Contains(label, "iter-01") {
			breachOnce.Do(func() { breached = true })
		}
		return breached
	}
	interruptCh := make(chan string, 1)
	g.onInterrupt = func(s agentrunner.Session) { interruptCh <- s.Label }
	d.Runner = g

	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if strings.Contains(cwd, "epic-item-01") {
			return 999999, true, nil
		}
		return 0, false, nil
	}

	d.Sleep = func(time.Duration) {}

	sink := newRecordingEventSink()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, sink)
	}()

	if label := <-interruptCh; !strings.Contains(label, "iter-01") {
		t.Fatalf("interrupted %s, want iter-01", label)
	}

	// Drain the two iterations' initial launch prompts (order not
	// guaranteed) before the breach recovery's own /compact prompt shows up.
	for {
		if got := <-promptCh; got == "/compact" {
			break
		}
	}
	finishPrompt := <-promptCh
	if !strings.Contains(finishPrompt, "130000") {
		t.Errorf("finish-up prompt = %q, want it to mention the effective --smart-zone value 130000", finishPrompt)
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
	var iter1, iter2 string
	for range 2 {
		l := <-g.started
		if strings.Contains(l, "iter-01") {
			iter1 = l
		} else {
			iter2 = l
		}
	}

	// iter-02 finishes and ticket 03 backfills immediately, even though
	// iter-01 is still mid-recovery — proving the scheduler was never
	// blocked by the smart-zone breach (no Gate.pause on this path).
	g.release(iter2)
	iter3 := <-g.started
	g.release(iter1)
	g.release(iter3)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 3 {
		t.Errorf("removed worktree branches = %v, want all 3 iterations removed", *removed)
	}

	if !hasEvent(sink, LiveEventSmartZoneCompactStarted, func(ev LiveEvent) bool { return ev.Identifier == "01" }) {
		t.Errorf("events = %+v, want a compact-started event for ticket 01", sink.Events())
	}
	if !hasEvent(sink, LiveEventSmartZoneFinishingUp, func(ev LiveEvent) bool { return ev.Identifier == "01" }) {
		t.Errorf("events = %+v, want a finishing-up event for ticket 01", sink.Events())
	}
}

// TestRun_SmartZoneBreach_RepeatsWithNoRetryCap drives a full Run() through
// iter-01 breaching the smart zone twice in a row before finally settling:
// each breach fires its own Ctrl-C -> /compact -> finish-up cycle, and the
// scheduler's Gate is never paused by either one, matching the "no retry
// cap" and "Gate.isPaused() stays false throughout" requirements that
// distinguish this recovery path from rate-limit/needs-repair pauses.
func TestRun_SmartZoneBreach_RepeatsWithNoRetryCap(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, removed := fakeDeps()

	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	promptCh := make(chan string, 8)
	onRunnerPrompt(d, func(_ agentrunner.Session, text string) error {
		promptCh <- text
		return nil
	})

	g := newGatedRunner(d.Runner)
	g.finishPollsOnly = true
	interruptCh := make(chan string, 8)
	g.onInterrupt = func(s agentrunner.Session) { interruptCh <- s.Label }
	var breaches int
	var breachMu sync.Mutex
	g.timeOut = func(label string) bool {
		if !strings.Contains(label, "iter-01") {
			return false
		}
		breachMu.Lock()
		defer breachMu.Unlock()
		fire := breaches < 2
		if fire {
			breaches++
		}
		return fire
	}
	d.Runner = g

	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 999999, true, nil
	}

	gate := NewGate()
	d.Sleep = func(time.Duration) {}

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 1, Gate: gate,
		}, d, noopEventSink{})
	}()

	for i := range 2 {
		if label := <-interruptCh; !strings.Contains(label, "iter-01") {
			t.Fatalf("breach %d: interrupted %s, want iter-01", i, label)
		}
		// Drain the iteration's own initial launch prompt (only present
		// ahead of the very first breach) before the recovery's /compact.
		for {
			if got := <-promptCh; got == "/compact" {
				break
			}
		}
		if got := <-promptCh; !strings.Contains(got, "130000") {
			t.Fatalf("breach %d: finish-up prompt = %q, want it to mention 130000", i, got)
		}
		if gate.isPaused() {
			t.Fatalf("breach %d: gate.isPaused() = true, want the scheduler never blocked by smart-zone recovery", i)
		}
	}

	g.release(<-g.started)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the single iteration removed", *removed)
	}
}
