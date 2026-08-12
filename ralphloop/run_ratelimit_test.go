package ralphloop

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
)

// TestRun_RateLimitDetected_AutoPausesAndResumesWithReprompt drives a full
// Run() through: epic-iter-01's pane showing a Claude rate-limit message (no
// Ctrl-C — unlike a smart-zone breach, the agent isn't interrupted, it's
// already sitting blocked), the loop auto-pausing (no scheduling, other
// iterations keep running) without any `gx ralph-loop resume` call, then
// auto-detecting the reset, re-prompting epic-iter-01's agent to continue, and
// resuming normal scheduling.
func TestRun_RateLimitDetected_AutoPausesAndResumesWithReprompt(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
		"03-c.md": "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# C\n",
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

	// A Claude rate-limit hit has no status of its own (see waitForFinish):
	// the pane just goes idle, same as an ordinary finish, with the
	// rate-limit message still sitting in its recent output. So the gated
	// wait here resolves epic-iter-01's "done" wait to idle exactly like any
	// other iteration's — release(pane) is what simulates that idle
	// transition, whether it's a real finish or, as here, a rate limit.
	wait, started, release := gatedAgentWait(d.AgentWait)
	d.AgentWait = wait

	var rlMu sync.Mutex
	rateLimitCleared := false
	d.ReadPaneRecent = func(pane string) (string, error) {
		if !strings.Contains(pane, "epic-iter-01") {
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

	// No parseable reset time in "Claude usage limit reached", so the wait
	// falls back to re-checking ReadPaneRecent every rateLimitPollInterval.
	// Advance a fake clock on every Sleep so that cadence is actually
	// reached without a real 5-minute wait.
	var clockMu sync.Mutex
	fakeNow := time.Now()
	d.Sleep = func(time.Duration) {
		clockMu.Lock()
		fakeNow = fakeNow.Add(rateLimitPollInterval)
		clockMu.Unlock()
	}
	d.Now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return fakeNow
	}

	sink := newRecordingEventSink()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, sink)
	}()

	// epic-iter-01 and epic-iter-02 both claimed and started (2 slots); either order.
	var pane1, pane2 string
	for range 2 {
		p := <-started
		if strings.Contains(p, "epic-iter-01") {
			pane1 = p
		} else {
			pane2 = p
		}
	}

	// epic-iter-01's pane goes idle showing the rate-limit message.
	release(pane1)

	select {
	case keys := <-sendKeysCh:
		t.Fatalf("AgentSendKeys called with %v, want no interrupt on a rate-limit pause (unlike smart-zone)", keys)
	case <-time.After(50 * time.Millisecond):
	}

	raw01, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw01), "status: claimed") {
		t.Errorf("ticket 01 status = %s, want claimed (paused, not reverted or done) while paused", raw01)
	}

	// epic-iter-02 finishes normally while epic-iter-01 stays paused.
	release(pane2)

	// Give the scheduler a moment to (wrongly, if buggy) backfill a third
	// ticket while still paused, then confirm it didn't.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	createdSoFar := slices.Clone(createdBranches)
	mu.Unlock()
	if len(createdSoFar) != 3 { // feature worktree + epic-iter-01 + epic-iter-02
		t.Fatalf("worktrees created while paused = %v, want exactly [epic, epic-iter-01, epic-iter-02] (no backfill until resumed)", createdSoFar)
	}

	rlMu.Lock()
	rateLimitCleared = true
	rlMu.Unlock()

	// Ticket 03 is backfilled once epic-iter-01's pause clears (its own gate was
	// already closed by the earlier release(pane1), so it re-observes idle
	// and finishes without another release call).
	pane3 := <-started
	release(pane3)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 3 {
		t.Errorf("removed worktree branches = %v, want all 3 iterations removed", *removed)
	}

	if !slices.Contains(*prompts, "continue") {
		t.Errorf("prompts = %v, want a \"continue\" re-prompt sent to epic-iter-01 after the rate-limit reset", *prompts)
	}

	if !hasEvent(sink, LiveEventIterationPaused, func(ev LiveEvent) bool { return ev.Label == "epic-iter-01" }) {
		t.Errorf("events = %+v, want a paused event for epic-iter-01", sink.Events())
	}
	if !hasEvent(sink, LiveEventIterationResumed, func(ev LiveEvent) bool { return ev.Label == "epic-iter-01" }) {
		t.Errorf("events = %+v, want a resumed event for epic-iter-01", sink.Events())
	}
}
