package ralphloop

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

func TestPauseGate_NotPausedInitially(t *testing.T) {
	g := NewGate()
	if g.isPaused() {
		t.Error("isPaused() = true for a fresh gate, want false")
	}
}

func TestPauseGate_PauseMarksPausedAndRecordsReason(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "context occupancy breach")

	if !g.isPaused() {
		t.Error("isPaused() = false after pause(), want true")
	}
	if got := g.snapshot()["iter-01"]; got != "context occupancy breach" {
		t.Errorf(`snapshot()["iter-01"] = %q, want "context occupancy breach"`, got)
	}
}

func TestPauseGate_MultiplePausedIterations_AllStayPausedUntilOneResumeSignal(t *testing.T) {
	g := NewGate()
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
	g := NewGate()
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

// TestPauseGate_ForceResume_WakesWaiterWithoutWaitingForSleep is the
// in-process control path's whole point: unlike a `gx ralph-loop resume`
// file signal, which the leader only notices on its next resumePollInterval
// tick, ForceResume must release a waiter immediately — even mid-tick, with
// Sleep still blocked and ResumeSignaled still reporting false.
func TestPauseGate_ForceResume_WakesWaiterWithoutWaitingForSleep(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "breach")

	sleepStarted := make(chan struct{})
	sleepBlock := make(chan struct{})
	d := Deps{
		ResumeSignaled: func(path string) (bool, error) { return false, nil },
		Sleep: func(time.Duration) {
			close(sleepStarted)
			<-sleepBlock // never closed here: a real poll tick would still be blocked
		},
	}

	returned := make(chan struct{})
	go func() {
		g.waitForResume(d, "unused")
		close(returned)
	}()

	select {
	case <-sleepStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never reached its poll-interval sleep")
	}

	if wasPaused := g.ForceResume("iter-01"); !wasPaused {
		t.Fatal("ForceResume(\"iter-01\") = false, want true: iter-01 was paused")
	}

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() did not return after ForceResume, even with Sleep still blocked")
	}

	if g.isPaused() {
		t.Error("isPaused() = true after ForceResume, want false")
	}
}

func TestPauseGate_ForceResume_UnknownLabelReturnsFalse(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "breach")

	if g.ForceResume("iter-02") {
		t.Error("ForceResume(unpaused label) = true, want false")
	}
	if !g.isPaused() {
		t.Error("isPaused() = false after ForceResume of an unrelated label, want true: iter-01 is still paused")
	}
}

// TestPauseGate_ForceResumeBeforePause_WaitForResumeReturnsImmediately covers
// the race where ForceResume lands in the gap between a caller's pause() and
// its own subsequent waitForResume() call (e.g. a TUI operator resuming the
// instant it observes the pause). Without the len(reasons)==0 short-circuit
// in waitForResume, that late arrival would make it the new leader for a
// pause that no longer exists, polling forever for a resume signal that
// already happened.
func TestPauseGate_ForceResumeBeforePause_WaitForResumeReturnsImmediately(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "breach")

	if !g.ForceResume("iter-01") {
		t.Fatal("ForceResume(iter-01) = false, want true")
	}

	d := Deps{
		ResumeSignaled: func(path string) (bool, error) { return false, nil },
		Sleep:          func(time.Duration) {},
	}

	returned := make(chan struct{})
	go func() {
		g.waitForResume(d, "unused")
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned even though the label was never actually paused")
	}
}

func TestWaitForFinish_CodexContextBreachPausesAndResumes(t *testing.T) {
	gate := NewGate()
	var waits int
	var interrupted bool
	var observedCwd, observedSession string
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 1 {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(target string, keys ...string) error {
			interrupted = slices.Equal(keys, []string{"ctrl+c"})
			return nil
		},
		ReadCodexContext: func(cwd, sessionID string) (int, bool, error) {
			observedCwd, observedSession = cwd, sessionID
			return 150001, true, nil
		},
		ResumeSignaled: func(path string) (bool, error) { return true, nil },
		Sleep:          func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label:            "iter-01",
		Agent:            AgentCodex,
		Pane:             "pane-1",
		SessionCwd:       "/repo/iter-01",
		SmartZone:        150000,
		Gate:             gate,
		ResumeSignalPath: "unused",
	}, "codex-session-1")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if !interrupted {
		t.Error("AgentSendKeys did not interrupt the Codex pane with ctrl-c")
	}
	if observedCwd != "/repo/iter-01" || observedSession != "codex-session-1" {
		t.Errorf("ReadCodexContext(%q, %q), want (/repo/iter-01, codex-session-1)", observedCwd, observedSession)
	}
	if gate.isPaused() {
		t.Error("gate remains paused after the resume signal")
	}
}

func TestWaitForFinish_CodexBlockedMarksNeedsAttentionThenRecovers(t *testing.T) {
	ticketPath := writeTicket(t, "# Ticket\n\n**Status:** claimed\n")
	scratchDir := t.TempDir()
	gate := NewGate()
	var waits int
	var sawNeedsAttention bool
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			switch waits {
			case 1:
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
			case 2:
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			default:
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			}
		},
		ResumeSignaled: func(string) (bool, error) {
			raw, err := os.ReadFile(ticketPath)
			if err == nil {
				sawNeedsAttention = strings.Contains(string(raw), "needs-attention")
			}
			return false, nil
		},
		Sleep: func(time.Duration) {},
	}

	if err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		ScratchDir: scratchDir, EpicName: "epic", Gate: gate,
	}, "codex-session-1"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if !sawNeedsAttention {
		t.Error("ticket was not marked needs-attention while Codex was blocked")
	}
	if gate.isPaused() {
		t.Error("gate remains paused after Codex recovered")
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "claimed") {
		t.Errorf("ticket status = %s, want claimed after recovery", raw)
	}
	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok || len(events) < 2 {
		t.Fatalf("readEvents() = %+v, ok=%v, err=%v", events, ok, err)
	}
	if events[0].Type != eventNeedsAttention || events[0].Pane != "pane-1" || events[0].Reason == "" {
		t.Errorf("attention event = %+v, want pane and reason", events[0])
	}
}

func TestWaitForFinish_CodexQuotaDoesNotBecomeNeedsAttention(t *testing.T) {
	ticketPath := writeTicket(t, "# Ticket\n\n**Status:** claimed\n")
	gate := NewGate()
	var waits, prompts, quotaChecks int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits < 3 {
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts++
			return herdr.Agent{}, nil
		},
		ReadCodexRateLimit: func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
			quotaChecks++
			if quotaChecks > 1 {
				return codexsession.RateLimit{}, false, nil
			}
			return codexsession.RateLimit{Quota: "primary", ResetAt: time.Now().Add(-time.Second)}, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	if err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		Gate: gate,
	}, "codex-session-1"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if prompts != 1 {
		t.Errorf("continue prompts = %d, want 1", prompts)
	}
	if gate.isPaused() {
		t.Error("gate remains paused after the Codex quota reset")
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "needs-attention") {
		t.Errorf("ticket status = %s, quota exhaustion must not become needs-attention", raw)
	}
}

func TestWaitForFinish_CodexIgnoresClaudeTerminalRateLimitText(t *testing.T) {
	var waits, prompts int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 1 {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts++
			return herdr.Agent{}, nil
		},
		ReadPaneRecent: func(string) (string, error) { return "Claude usage limit reached", nil },
		Sleep:          func(time.Duration) {},
	}

	if err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Gate: NewGate(),
	}, "codex-session-1"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if prompts != 0 {
		t.Errorf("continue prompts = %d, want 0 from Claude terminal text", prompts)
	}
}

func TestWaitForFinish_ManualAttentionRecheckKeepsBlockedTicketPaused(t *testing.T) {
	ticketPath := writeTicket(t, "# Ticket\n\n**Status:** claimed\n")
	scratchDir := t.TempDir()
	gate := NewGate()
	var waits int
	var reports strings.Builder
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			switch waits {
			case 1, 3:
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
			case 2:
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			default:
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "done"}, nil
			}
		},
		ResumeSignaled: func(string) (bool, error) { return waits == 2, nil },
		Sleep:          func(time.Duration) {},
	}

	if err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		ScratchDir: scratchDir, EpicName: "epic", Gate: gate,
		Report: func(format string, args ...any) { fmt.Fprintf(&reports, format, args...) },
	}, "codex-session-1"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if !strings.Contains(reports.String(), "still needs attention") {
		t.Errorf("reports = %q, want blocked manual-recheck feedback", reports.String())
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
		}, d, NewTextEventSink(&out))
	}()

	// iter-01 and iter-02 both claimed and started (2 slots).
	pane2 := <-started

	keys := <-sendKeysCh
	if len(keys) == 0 || keys[0] != "ctrl+c" {
		t.Fatalf("AgentSendKeys keys = %v, want [ctrl+c]", keys)
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
		if ev.Type == eventPausedSmartZone && ev.Ticket == "01" {
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

// TestRun_SmartZoneBreach_ForceResumeViaGateWakesWithoutFileSignal drives a
// full Run() through a smart-zone pause, exactly like
// TestRun_SmartZoneBreach_PausesResumesAndKeepsSchedulingCorrect, but proves
// the in-process resume path added in ticket 06a actually works end-to-end:
// d.ResumeSignaled is hardcoded to always return false (so the file-signal
// path can never be what unblocks the run) and d.Sleep is a no-op, yet
// calling RunOptions.Gate.ForceResume directly from the test goroutine —
// once it has observed the gate as paused — still wakes iter-01 and lets the
// epic complete.
func TestRun_SmartZoneBreach_ForceResumeViaGateWakesWithoutFileSignal(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** open\n",
		"03-c.md": "# C\n\n**Status:** open\n",
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

	// Never true: only ForceResume, never a `gx ralph-loop resume` file
	// signal, is allowed to unblock this run.
	d.ResumeSignaled = func(path string) (bool, error) { return false, nil }
	d.Sleep = func(time.Duration) {}

	gate := NewGate()
	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2, Gate: gate,
		}, d, NewTextEventSink(&out))
	}()

	// iter-01 and iter-02 both claimed and started (2 slots).
	pane2 := <-started

	keys := <-sendKeysCh
	if len(keys) == 0 || keys[0] != "ctrl+c" {
		t.Fatalf("AgentSendKeys keys = %v, want [ctrl+c]", keys)
	}

	// Wait for the gate to actually record iter-01 as paused before forcing
	// resume, so ForceResume has something to clear rather than racing
	// ahead of pause() (see
	// TestPauseGate_ForceResumeBeforePause_WaitForResumeReturnsImmediately).
	deadline := time.After(2 * time.Second)
	for !gate.isPaused() {
		select {
		case <-deadline:
			t.Fatal("gate never observed as paused")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	// iter-02 finishes normally while iter-01 stays paused.
	release(pane2)

	if !gate.ForceResume("iter-01") {
		t.Fatal("ForceResume(iter-01) = false, want true")
	}

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

	// A Claude rate-limit hit has no status of its own (see waitForFinish):
	// the pane just goes idle, same as an ordinary finish, with the
	// rate-limit message still sitting in its recent output. So the gated
	// wait here resolves iter-01's "done" wait to idle exactly like any
	// other iteration's — release(pane) is what simulates that idle
	// transition, whether it's a real finish or, as here, a rate limit.
	wait, started, release := gatedAgentWait(d.AgentWait)
	d.AgentWait = wait

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

	// No parseable reset time in "Claude usage limit reached", so the wait
	// falls back to re-checking ReadPaneRecent every rateLimitPollInterval.
	// Advance a fake clock on every Sleep so that cadence is actually
	// reached without a real 5-minute wait, and force ResumeSignaled false
	// so the pause only clears via that text recheck, not a spurious signal.
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
	d.ResumeSignaled = func(string) (bool, error) { return false, nil }

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, NewTextEventSink(&out))
	}()

	// iter-01 and iter-02 both claimed and started (2 slots); either order.
	var pane1, pane2 string
	for range 2 {
		p := <-started
		if strings.Contains(p, "iter-01") {
			pane1 = p
		} else {
			pane2 = p
		}
	}

	// iter-01's pane goes idle showing the rate-limit message.
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

	// Ticket 03 is backfilled once iter-01's pause clears (its own gate was
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
		t.Errorf("prompts = %v, want a \"continue\" re-prompt sent to iter-01 after the rate-limit reset", *prompts)
	}

	if !strings.Contains(out.String(), "paused iter-01") {
		t.Errorf("output = %q, want a paused report mentioning iter-01", out.String())
	}
	if !strings.Contains(out.String(), "resumed iter-01") {
		t.Errorf("output = %q, want a resumed report mentioning iter-01", out.String())
	}
}
