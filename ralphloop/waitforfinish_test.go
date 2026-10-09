package ralphloop

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

func TestWaitForFinish_CodexNativeContextFailureRecoversDespiteStaleOccupancy(t *testing.T) {
	t.Parallel()
	const failure = "■ stream disconnected before completion: Your input exceeds the context window of this model. Please adjust your input and try again."
	scratchDir := epicScratchDir(t, "epic")
	r := &blipRunner{Runner: idlePromptRunner("iter-20", "codex-session-20"), timeoutOn: 1, contextExhausted: failure}
	d := Deps{
		Runner: r,
		ReadCodexContext: func(string, string) (int, bool, error) {
			return 1_000, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-20", Agent: AgentCodex, Pane: "pane-1", Ticket: "20",
		SessionCwd: "/repo/iter-20", SmartZone: 150_000, ScratchDir: scratchDir,
		EpicName: "epic", Gate: NewGate(),
	}, "codex-session-20")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if r.interrupts != 1 {
		t.Errorf("pane interruptions = %d, want 1", r.interrupts)
	}
	if prompts := r.Prompts("iter-20"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want compact then finish-up", prompts)
	}
	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok || len(events) == 0 {
		t.Fatalf("ReadEvents() = %+v, ok=%v, err=%v", events, ok, err)
	}
	if events[0].Type != string(eventsc.PausedSmartZone) || !strings.Contains(events[0].Reason, "input exceeds the context window") {
		t.Errorf("recovery event = %+v, want native failure evidence", events[0])
	}
}

func TestWaitForFinish_CodexNativeContextFailureDetectedWhenSettled(t *testing.T) {
	t.Parallel()
	r := &blipRunner{
		Runner:           idlePromptRunner("iter-20", "codex-session-20"),
		contextExhausted: `Error running remote compact task: {"error":{"code":"context_length_exceeded"}}`,
	}
	d := Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-20", Agent: AgentCodex, Pane: "pane-1", Ticket: "20",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-20")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if r.interrupts != 1 {
		t.Errorf("pane interruptions = %d, want 1 before accepting settled state", r.interrupts)
	}
}

func TestWaitForFinish_CodexNativeContextFailureRecoveryFailureIsDurable(t *testing.T) {
	t.Parallel()
	const failure = "■ Codex ran out of room in the model's context window."
	r := &blipRunner{Runner: idlePromptRunner("iter-21", "codex-session-21"), contextExhausted: failure}
	r.FailNextPrompts("iter-21", 1, errors.New("compact never landed"))
	d := Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-21", Agent: AgentCodex, Pane: "pane-1", Ticket: "21",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-21")
	if err == nil {
		t.Fatal("waitForFinish: want durable error after failed recovery, got nil")
	}
	if !strings.Contains(err.Error(), "recovery failed") || !strings.Contains(err.Error(), "context window") {
		t.Errorf("waitForFinish error = %q, want it to name the failed recovery and evidence", err.Error())
	}
	if r.interrupts != 1 {
		t.Errorf("pane interruptions = %d, want 1", r.interrupts)
	}
}

func TestWaitForFinish_CodexNativeContextFailureFailsDurablyWithoutFreshTokenEvent(t *testing.T) {
	t.Parallel()
	// No ReadCodexContext dependency at all: the classification and its
	// recovery-failure path must not depend on a fresh high-token record
	// existing to fire — the native exhaustion text is itself the evidence.
	const failure = `Error running remote compact task: {"error":{"code":"context_length_exceeded"}}`
	r := &blipRunner{Runner: idlePromptRunner("iter-21", "codex-session-21"), contextExhausted: failure}
	r.FailNextPrompts("iter-21", 1, errors.New("compact never landed"))
	d := Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-21", Agent: AgentCodex, Pane: "pane-1", Ticket: "21",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-21")
	if err == nil || !strings.Contains(err.Error(), "recovery failed") {
		t.Fatalf("waitForFinish() = %v, want a durable recovery-failed error without any ReadCodexContext dependency", err)
	}
}

func TestWaitForFinish_CodexContextBreachRecovers(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	gate := NewGate()
	var observedCwd, observedSession string
	r := &blipRunner{Runner: idlePromptRunner("iter-01", "codex-session-1"), timeoutOn: 1}
	d := Deps{
		Runner: r,
		ReadCodexContext: func(cwd, sessionID string) (int, bool, error) {
			observedCwd, observedSession = cwd, sessionID
			return 150001, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label:      "iter-01",
		Agent:      AgentCodex,
		Pane:       "pane-1",
		Ticket:     "01",
		TicketPath: ticketPath,
		SessionCwd: "/repo/iter-01",
		SmartZone:  150000,
		Gate:       gate,
	}, "codex-session-1")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if r.interrupts != 1 {
		t.Errorf("interrupts = %d, want one before /compact", r.interrupts)
	}
	if observedCwd != "/repo/iter-01" || observedSession != "codex-session-1" {
		t.Errorf("ReadCodexContext(%q, %q), want (/repo/iter-01, codex-session-1)", observedCwd, observedSession)
	}
	if prompts := r.Prompts("iter-01"); len(prompts) != 2 || prompts[0] != "/compact" || !strings.Contains(prompts[1], "150000") {
		t.Errorf("prompts = %v, want [/compact, <finish-up prompt mentioning 150000>]", prompts)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "needs-repair") {
		t.Errorf("ticket status = %s, a recovered breach must not become needs-repair", raw)
	}
	if gate.isPaused() {
		t.Error("gate.isPaused() = true, want smart-zone recovery to never pause the Gate")
	}
}

// TestRecoverSmartZoneBreach_TranscriptConfirmsLateCompaction verifies the
// gap from research ticket 05: herdr's pane-status wait for "/compact" can
// keep timing out past smartZoneCompactTimeoutMs even though the compact is
// genuinely still running, not stuck. Once the transcript's compaction-
// boundary count rises above its pre-compact baseline, recovery must treat
// that as success (and finish up) instead of reporting a failure.
func TestRecoverSmartZoneBreach_TranscriptConfirmsLateCompaction(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	var compactionCount int
	r := breachRunner(func(wait int) bool {
		// Ticks past smartZoneCompactTimeoutMs before the transcript records
		// the compaction finishing, so recovery must stop polling on its own
		// rather than needing the pane to ever confirm completion.
		if wait == 12 {
			compactionCount = 1
		}
		return true
	})
	d := Deps{
		Runner: r,
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true once the transcript confirms compaction completed")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 {
		t.Errorf("prompts = %v, want 2 (/compact, then finish-up)", prompts)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var sawFailed, sawExpired, sawResumed bool
	for _, e := range events {
		switch e.Type {
		case string(eventsc.SmartZoneRecoveryFailed):
			sawFailed = true
		case string(eventsc.SmartZoneWaitExpired):
			sawExpired = true
		case string(eventsc.Resumed):
			sawResumed = true
		}
	}
	if sawFailed {
		t.Error("smart-zone-recovery-failed event emitted for a compact that merely ran long, want none")
	}
	if !sawExpired {
		t.Error("missing smart-zone-wait-expired event distinguishing the expired-but-completed case")
	}
	if !sawResumed {
		t.Error("missing resumed event after transcript-confirmed recovery")
	}
}

// TestRecoverSmartZoneBreach_GenuineStuckCompactFailsAfterExtendedWait
// verifies the other half of the same gap: when neither herdr's pane status
// nor the transcript's compaction-boundary count ever show completion, the
// wait must eventually give up (at smartZoneCompactExtendedTimeoutMs) and
// report a genuine failure, not poll forever.
func TestRecoverSmartZoneBreach_GenuineStuckCompactFailsAfterExtendedWait(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	r := breachRunner(alwaysTimeout)
	d := Deps{
		Runner: r,
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return 0, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=true, want false: compaction never completed on either signal")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 {
		t.Errorf("prompts = %v, want /compact only, no finish-up after failure", prompts)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var sawFailed bool
	for _, e := range events {
		if e.Type == string(eventsc.SmartZoneRecoveryFailed) {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Error("missing smart-zone-recovery-failed event for a genuinely stuck compact")
	}
}

// TestRecoverSmartZoneBreach_PrematureIdleFallsThroughToTranscriptCheck
// verifies the fix for issue 03: an immediate (non-timeout) "/compact"
// success is not trusted on its own when a compaction-count baseline is
// available. If the transcript's compaction count hasn't advanced past
// baseline yet, that idle/done report was premature and recovery must fall
// through to waitForCompactionSignal instead of sending the finish-up
// prompt right away.
func TestRecoverSmartZoneBreach_PrematureIdleFallsThroughToTranscriptCheck(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	var compactionCount int
	r := breachRunner(func(wait int) bool {
		if wait == 2 {
			// The real compaction lands between the second and third poll
			// tick, after the first tick's premature idle was refused.
			compactionCount = 1
			return true
		}
		return false
	})
	d := Deps{
		Runner: r,
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true once the transcript confirms compaction completed")
	}
	if r.waits != 3 {
		t.Errorf("Runner.Wait calls = %d, want 3: the premature idle must keep polling until the transcript confirms", r.waits)
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up] with finish-up sent only after the fallthrough poll confirms compaction", prompts)
	}
}

// TestRecoverSmartZoneBreach_ImmediateSuccessTrustedWhenAlreadyAdvanced
// verifies the other half of issue 03's fix: when the compaction count has
// already advanced past baseline by the time the immediate "/compact"
// success comes back, that's a genuine completion and recovery proceeds
// straight to the finish-up prompt with no extra polling.
func TestRecoverSmartZoneBreach_ImmediateSuccessTrustedWhenAlreadyAdvanced(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	var sleeps int
	var readCompactionsCalls int
	r := breachRunner(nil)
	d := Deps{
		Runner: r,
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			readCompactionsCalls++
			if readCompactionsCalls == 1 {
				return 0, true, nil // baseline, taken before "/compact" is sent
			}
			return 1, true, nil // already advanced by the first poll tick
		},
		Sleep: func(time.Duration) { sleeps++ },
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true")
	}
	if r.waits != 1 || sleeps != 0 {
		t.Errorf("Runner.Wait calls = %d, gated sleeps = %d; want 1 and 0: a genuine immediate success must not keep polling", r.waits, sleeps)
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up]", prompts)
	}
}

func TestReadCompactBoundaries_ClassifiesEachState(t *testing.T) {
	t.Parallel()
	reads := func(count int, ok bool, err error) func(string, string) (int, bool, error) {
		return func(string, string) (int, bool, error) { return count, ok, err }
	}
	cases := []struct {
		name      string
		agent     AgentKind
		sessionID string
		read      func(string, string) (int, bool, error)
		want      compactBoundarySnapshot
	}{
		{
			name:  "Codex has no boundary signal",
			agent: AgentCodex, sessionID: "sess-19", read: reads(4, true, nil),
			want: compactBoundarySnapshot{state: compactBoundaryUnsupported},
		},
		{
			name:  "nil read dependency is unsupported",
			agent: AgentClaude, sessionID: "sess-19", read: nil,
			want: compactBoundarySnapshot{state: compactBoundaryUnsupported},
		},
		{
			name:  "empty session id is unavailable, not unsupported",
			agent: AgentClaude, sessionID: "", read: reads(4, true, nil),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "read error is unavailable",
			agent: AgentClaude, sessionID: "sess-19", read: reads(0, false, errors.New("read failed")),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "transcript that does not exist yet is unavailable",
			agent: AgentClaude, sessionID: "sess-19", read: reads(0, false, nil),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "a read count is confirmed",
			agent: AgentClaude, sessionID: "sess-19", read: reads(4, true, nil),
			want: compactBoundarySnapshot{state: compactBoundaryConfirmed, count: 4},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := readCompactBoundaries(Deps{ReadCompactions: tc.read}, tc.agent, "/repo/iter-19", tc.sessionID)
			if got != tc.want {
				t.Errorf("readCompactBoundaries() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// gatedBreachParams is the launchAndPromptParams shape the compaction-gate
// tests below share: a Claude iteration whose events land in scratchDir.
func gatedBreachParams(scratchDir string) launchAndPromptParams {
	return launchAndPromptParams{
		Label:      "iter-19",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}
}

// breachRunner hosts gatedBreachParams' session. Its agent reports idle the
// moment it is prompted, and timeoutIf scripts which compact polls time out.
func breachRunner(timeoutIf func(wait int) bool) *blipRunner {
	return &blipRunner{Runner: idlePromptRunner("iter-19", "sess-19"), timeoutIf: timeoutIf}
}

// TestRecoverSmartZoneBreach_GateHoldsWhileBoundaryStaysAtBaseline verifies
// the live incident from research ticket 15: a pane that reports idle the
// instant "/compact" is typed, over and over, while the transcript's
// compaction-boundary count never moves. The finish-up prompt must never go
// out — sending it there is what cancelled the compaction — and the give-up
// must be paced by real poll intervals rather than spinning through the
// extended bound instantly.
func TestRecoverSmartZoneBreach_GateHoldsWhileBoundaryStaysAtBaseline(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	var sleeps []time.Duration
	r := breachRunner(nil)
	d := Deps{
		Runner: r,
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return 0, true, nil
		},
		Sleep: func(d time.Duration) { sleeps = append(sleeps, d) },
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
	}
	if recovered {
		t.Error("recoverSmartZoneBreach returned recovered=true, want false: the transcript never confirmed the compaction")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only: the finish-up prompt cancels an in-progress compaction", prompts)
	}
	wantSleeps := smartZoneCompactExtendedTimeoutMs / smartZonePollMs
	if len(sleeps) != wantSleeps {
		t.Errorf("gated sleeps = %d, want %d (one poll interval per gated tick)", len(sleeps), wantSleeps)
	}
	for i, s := range sleeps {
		if s != smartZonePollMs*time.Millisecond {
			t.Fatalf("sleep %d = %s, want %s", i, s, smartZonePollMs*time.Millisecond)
		}
	}
}

// TestRecoverSmartZoneBreach_GateReleasesOnceBoundaryAdvances is the other
// half: the same premature-idle pane, but with a compaction that genuinely
// lands part-way through. Recovery must resume the moment the boundary count
// moves and send the finish-up prompt then, not before and not never.
func TestRecoverSmartZoneBreach_GateReleasesOnceBoundaryAdvances(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	var sleeps []time.Duration
	compactionCount := 0
	r := breachRunner(func(wait int) bool {
		if wait == 4 {
			compactionCount = 1
		}
		return false
	})
	d := Deps{
		Runner: r,
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		Sleep: func(d time.Duration) { sleeps = append(sleeps, d) },
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true once the boundary count advances")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up]", prompts)
	}
	if len(sleeps) != 3 {
		t.Errorf("gated sleeps = %d, want 3 (one per tick the transcript held the gate)", len(sleeps))
	}
}

// compactCompletionEvents collects the run-log event types scratchDir's epic
// recorded, so a completion-route test can assert on the event it wants by
// absence as much as by presence — the whole point of keeping the gated and
// the timeout route on separate names.
func compactCompletionEvents(t *testing.T, scratchDir string) map[string]bool {
	t.Helper()
	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Type] = true
	}
	return seen
}

// TestRecoverSmartZoneBreach_GatedCompletionLogsItsOwnEvent covers the middle
// state: the pane claimed completion immediately, the gate held it, and the
// boundary landed a few ticks later. Nothing about that wait expired, so it
// must log the gated event rather than borrowing the timeout one.
func TestRecoverSmartZoneBreach_GatedCompletionLogsItsOwnEvent(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	compactionCount := 0
	d := Deps{
		Runner: breachRunner(func(wait int) bool {
			if wait == 4 {
				compactionCount = 1
			}
			return false
		}),
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	if _, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100); err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}

	seen := compactCompletionEvents(t, scratchDir)
	if !seen[string(eventsc.SmartZoneGateReleased)] {
		t.Errorf("missing %s event for a gated-then-confirmed completion", string(eventsc.SmartZoneGateReleased))
	}
	if seen[string(eventsc.SmartZoneWaitExpired)] {
		t.Errorf("%s logged for a gated completion, want it reserved for the genuine timeout route", string(eventsc.SmartZoneWaitExpired))
	}
}

// TestRecoverSmartZoneBreach_TimeoutCompletionKeepsTheExpiredEvent is the
// other direction: a pane wait that really did run past the compact timeout
// keeps the expired event and must not pick up the gated one.
func TestRecoverSmartZoneBreach_TimeoutCompletionKeepsTheExpiredEvent(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	compactionCount := 0
	d := Deps{
		Runner: breachRunner(func(wait int) bool {
			if wait == 12 {
				compactionCount = 1
			}
			return true
		}),
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	if _, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100); err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}

	seen := compactCompletionEvents(t, scratchDir)
	if !seen[string(eventsc.SmartZoneWaitExpired)] {
		t.Errorf("missing %s event for a timeout-then-confirmed completion", string(eventsc.SmartZoneWaitExpired))
	}
	if seen[string(eventsc.SmartZoneGateReleased)] {
		t.Errorf("%s logged for a timeout completion, want it reserved for a gate that actually held", string(eventsc.SmartZoneGateReleased))
	}
}

// TestRecoverSmartZoneBreach_PaneConfirmedCompletionLogsNeitherEvent covers
// the ordinary state: the pane reported completion and the transcript already
// agreed on the first read. Neither route was taken, so neither event belongs
// in the run log.
func TestRecoverSmartZoneBreach_PaneConfirmedCompletionLogsNeitherEvent(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	var reads int
	d := Deps{
		Runner: breachRunner(nil),
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			reads++
			if reads == 1 {
				return 0, true, nil
			}
			return 1, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true: the pane and the transcript both confirmed")
	}

	seen := compactCompletionEvents(t, scratchDir)
	if seen[string(eventsc.SmartZoneWaitExpired)] || seen[string(eventsc.SmartZoneGateReleased)] {
		t.Errorf("events = %v, want neither completion-route event for a pane-confirmed compaction", seen)
	}
}

// TestRecoverSmartZoneBreach_TimeoutPathIsNotDoublePaced pins the pacing
// asymmetry: a pane wait that times out has already consumed its poll interval
// inside Runner.Wait, so the gate must not sleep for it too. Sleeping on both
// branches would double every tick and stretch the extended bound to twice its
// wall-clock budget.
func TestRecoverSmartZoneBreach_TimeoutPathIsNotDoublePaced(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	var sleeps []time.Duration
	r := breachRunner(alwaysTimeout)
	d := Deps{
		Runner: r,
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return 0, true, nil
		},
		Sleep: func(d time.Duration) { sleeps = append(sleeps, d) },
	}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=true, want false: neither signal ever confirmed completion")
	}
	if len(sleeps) != 0 {
		t.Errorf("sleeps = %v, want none: the pane wait itself consumed each tick", sleeps)
	}
	wantWaits := smartZoneCompactExtendedTimeoutMs / smartZonePollMs
	if r.waits != wantWaits {
		t.Errorf("Runner.Wait calls = %d, want %d (one poll tick per interval up to the extended bound)", r.waits, wantWaits)
	}
}

// TestRecoverSmartZoneBreach_UnsupportedFailsOpenButUnavailableDoesNot covers
// the two states that fail open versus closed on the same underlying
// (0, false, nil) read: a build with no ReadCompactions dependency has no
// boundary signal at all and must behave exactly as it did before the gate,
// while a Claude session whose id isn't known yet is merely unavailable and
// must not be trusted on the pane's word.
func TestRecoverSmartZoneBreach_UnsupportedFailsOpenButUnavailableDoesNot(t *testing.T) {
	t.Parallel()
	newDeps := func(r *blipRunner, readCompactions func(string, string) (int, bool, error)) Deps {
		return Deps{
			Runner:          r,
			ReadCompactions: readCompactions,
			Sleep:           func(time.Duration) {},
		}
	}

	t.Run("no boundary signal at all trusts the idle pane", func(t *testing.T) {
		t.Parallel()
		r := breachRunner(nil)
		recovered, err := recoverSmartZoneBreach(newDeps(r, nil), gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
		if err != nil {
			t.Fatalf("recoverSmartZoneBreach: %v", err)
		}
		if prompts := r.Prompts("iter-19"); !recovered || len(prompts) != 2 {
			t.Errorf("recovered = %v, prompts = %v; want the pre-gate behavior: recovered with a finish-up prompt", recovered, prompts)
		}
	})

	t.Run("unidentified session holds the gate closed", func(t *testing.T) {
		t.Parallel()
		r := breachRunner(nil)
		d := newDeps(r, func(string, string) (int, bool, error) { return 0, true, nil })
		recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "", "smart-zone breach", 100)
		if !errors.Is(err, errCompactNeverConfirmed) {
			t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
		}
		if prompts := r.Prompts("iter-19"); recovered || len(prompts) != 1 {
			t.Errorf("recovered = %v, prompts = %v; want no finish-up prompt: an empty session id is unavailable, not unsupported", recovered, prompts)
		}
	})
}

// TestWaitForFinish_AbsorbsGatedGiveUpAndKeepsPolling verifies the call site:
// a gated give-up is a failed recovery, not a failed iteration, so
// waitForFinish swallows that one error and returns to polling — and takes the
// finish once the slow compaction it gave up on finally writes its boundary.
// Any other error around the breach path still aborts the iteration.
func TestWaitForFinish_AbsorbsGatedGiveUpAndKeepsPolling(t *testing.T) {
	t.Parallel()
	boundaries := 0
	r := boundGiveUpRunner(func(wait int) bool {
		if wait == 1 {
			return true
		}
		// The compaction lands for real once recovery has already given up
		// on it, which is what makes the pane's idle report a genuine finish.
		boundaries = 1
		return false
	})
	d := Deps{
		Runner:          r,
		ReadOccupancy:   func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) { return boundaries, true, nil },
		Sleep:           func(time.Duration) {},
	}

	err := waitForFinish(d, boundGiveUpParams(), "sess-19")
	if err != nil {
		t.Fatalf("waitForFinish: %v, want the gated give-up absorbed", err)
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only: no finish-up prompt may be sent on a gated give-up", prompts)
	}
}

func TestWaitForFinish_PropagatesNonGatedRecoveryErrors(t *testing.T) {
	t.Parallel()
	r := boundGiveUpRunner(func(wait int) bool { return wait == 1 })
	r.interruptErr = errors.New("pane is gone")
	d := Deps{
		Runner:          r,
		ReadOccupancy:   func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) { return 0, true, nil },
		Sleep:           func(time.Duration) {},
	}

	err := waitForFinish(d, boundGiveUpParams(), "sess-19")
	if err == nil || !strings.Contains(err.Error(), "pane is gone") {
		t.Fatalf("waitForFinish error = %v, want the transport failure propagated, not absorbed", err)
	}
}

// boundGiveUpParams is the iteration shape the give-up-bound tests share: a
// Claude iteration whose occupancy stays over the smart zone, so every poll
// tick that times out drives one full recovery attempt.
func boundGiveUpParams() launchAndPromptParams {
	return launchAndPromptParams{
		Label: "iter-19", Agent: AgentClaude, Pane: "pane-1", Ticket: "19",
		SessionCwd: "/repo/iter-19", SmartZone: 100, Gate: NewGate(),
	}
}

// boundGiveUpRunner hosts boundGiveUpParams' session; its finish polls time
// out wherever timeoutIf says. Recovery's own compact-completion polls see the
// agent idle right after "/compact": the premature idle the gate exists to
// distrust.
func boundGiveUpRunner(timeoutIf func(wait int) bool) *blipRunner {
	return &blipRunner{Runner: idlePromptRunner("iter-19", "sess-19"), timeoutIf: timeoutIf, finishPollsOnly: true}
}

func alwaysTimeout(int) bool { return true }

// countPrompts reports how many of prompts were the given text.
func countPrompts(prompts []string, text string) int {
	n := 0
	for _, p := range prompts {
		if p == text {
			n++
		}
	}
	return n
}

// TestWaitForFinish_EscalatesAfterTwoConsecutiveGatedGiveUps covers the cycle a
// bounded absorb exists to break: a compaction that never writes a boundary
// makes recovery give up gated, the poll loop resets its elapsed counter, the
// still-high occupancy breaches again, and nothing ever ends the iteration.
// After the bound the loop must escalate rather than try a third time — and
// must never fall back to the finish-up prompt, which is the compaction
// cancellation the gate exists to prevent.
func TestWaitForFinish_EscalatesAfterTwoConsecutiveGatedGiveUps(t *testing.T) {
	t.Parallel()
	r := boundGiveUpRunner(alwaysTimeout)
	d := Deps{
		Runner:          r,
		ReadOccupancy:   func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) { return 0, true, nil },
		Sleep:           func(time.Duration) {},
	}

	err := waitForFinish(d, boundGiveUpParams(), "sess-19")
	if !errors.Is(err, errCompactRecoveryExhausted) {
		t.Fatalf("waitForFinish error = %v, want one wrapping errCompactRecoveryExhausted", err)
	}
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Errorf("waitForFinish error = %v, want the underlying gated give-up preserved", err)
	}
	prompts := r.Prompts("iter-19")
	if got := countPrompts(prompts, "/compact"); got != maxConsecutiveGatedGiveUps {
		t.Errorf("/compact prompts = %d, want %d: the bound stops the loop instead of breaching again", got, maxConsecutiveGatedGiveUps)
	}
	if len(prompts) != maxConsecutiveGatedGiveUps {
		t.Errorf("prompts = %v, want /compact only: the finish-up prompt is never a post-bound fallback", prompts)
	}
}

// TestWaitForFinish_GatedGiveUpDeniesAPaneIdleToEveryPollKind covers the pane
// shape a gated give-up is actually produced by: one that reports idle to the
// compact-completion poll *and* to the ordinary finish poll while the
// compaction is still running. Trusting the finish poll there closes the ticket
// and abandons the worktree mid-compaction, and it also puts the give-up bound
// out of reach — the loop leaves by the finish path before a second give-up can
// ever be counted.
func TestWaitForFinish_GatedGiveUpDeniesAPaneIdleToEveryPollKind(t *testing.T) {
	t.Parallel()
	// Once compacted, every poll — the compact-completion polls and the
	// ordinary finish poll alike — reports idle, which is the pane shape this
	// test exists to distrust.
	r := boundGiveUpRunner(nil)
	r.timeoutIf = func(int) bool { return len(r.Prompts("iter-19")) == 0 }
	d := Deps{
		Runner:          r,
		ReadOccupancy:   func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) { return 0, true, nil },
		Sleep:           func(time.Duration) {},
	}

	err := waitForFinish(d, boundGiveUpParams(), "sess-19")
	if !errors.Is(err, errCompactRecoveryExhausted) {
		t.Fatalf("waitForFinish error = %v, want one wrapping errCompactRecoveryExhausted, not a successful finish", err)
	}
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Errorf("waitForFinish error = %v, want the underlying gated give-up preserved", err)
	}
	prompts := r.Prompts("iter-19")
	if got := countPrompts(prompts, "/compact"); got != 1 {
		t.Errorf("/compact prompts = %d, want 1: an idle pane never reaches a second breach, so the finish poll itself must carry the count", got)
	}
	if len(prompts) != 1 {
		t.Errorf("prompts = %v, want /compact only: no finish-up prompt may go out while the compaction is unconfirmed", prompts)
	}
}

// TestWaitForFinish_SuccessfulRecoveryResetsTheGiveUpCounter pins the bound to
// *consecutive* give-ups. A lifetime tally would escalate a healthy long
// iteration on two unrelated give-ups it had already recovered from.
func TestWaitForFinish_SuccessfulRecoveryResetsTheGiveUpCounter(t *testing.T) {
	t.Parallel()
	thirdLanded := false
	r := boundGiveUpRunner(nil)
	compacts := func() int { return countPrompts(r.Prompts("iter-19"), "/compact") }
	r.timeoutIf = func(int) bool {
		// The agent wraps up on its own after the third breach, so the run
		// must reach a normal finish rather than escalating.
		if compacts() < 3 {
			return true
		}
		// The third compaction lands only once recovery has given up on it,
		// so the finish the pane then reports is corroborated.
		thirdLanded = true
		return false
	}
	d := Deps{
		Runner:        r,
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		// Only the second compaction lands while its own recovery is watching.
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			switch {
			case thirdLanded:
				return 2, true, nil
			case compacts() >= 2:
				return 1, true, nil
			}
			return 0, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	if err := waitForFinish(d, boundGiveUpParams(), "sess-19"); err != nil {
		t.Fatalf("waitForFinish: %v, want no escalation: the successful second recovery reset the counter", err)
	}
	prompts := r.Prompts("iter-19")
	if got := countPrompts(prompts, "/compact"); got != 3 {
		t.Errorf("/compact prompts = %d, want 3", got)
	}
	if len(prompts) != 4 {
		t.Errorf("prompts = %v, want one finish-up prompt from the successful recovery only", prompts)
	}
}

// TestWaitForFinish_NonGatedRecoveryFailureNeitherCountsNorResets covers the
// discrimination the reset is easy to get backwards on: recoverSmartZoneBreach
// reports a failed finish-up prompt as (false, nil), so a reset keyed on a nil
// error would clear the counter for exactly the failures that say nothing
// about whether compaction is progressing.
func TestWaitForFinish_NonGatedRecoveryFailureNeitherCountsNorResets(t *testing.T) {
	t.Parallel()
	r := boundGiveUpRunner(alwaysTimeout)
	failArmed := false
	d := Deps{
		Runner:        r,
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		// The middle attempt compacts for real but its finish-up prompt fails,
		// so recovery abandons it without completing. The first read after its
		// "/compact" lands arms that failure.
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			if countPrompts(r.Prompts("iter-19"), "/compact") < 2 {
				return 0, true, nil
			}
			if !failArmed {
				failArmed = true
				r.FailNextPrompts("iter-19", 1, errors.New("finish-up never submitted"))
			}
			return 1, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, boundGiveUpParams(), "sess-19")
	if !errors.Is(err, errCompactRecoveryExhausted) {
		t.Fatalf("waitForFinish error = %v, want escalation: the middle failure was not a recovery and must not reset the counter", err)
	}
	prompts := r.Prompts("iter-19")
	if got := countPrompts(prompts, "/compact"); got != 3 {
		t.Errorf("/compact prompts = %d, want 3 (two gated give-ups either side of one non-gated failure)", got)
	}
	if len(prompts) != 3 {
		t.Errorf("prompts = %v, want /compact only: no finish-up prompt on any of the three attempts", prompts)
	}
}

// TestRecoverSmartZoneBreach_FailedFinishUpPromptIsBestEffort verifies that a
// finish-up prompt the runner refuses is logged and abandoned rather than
// failing the iteration: the agent may still finish on its own.
func TestRecoverSmartZoneBreach_FailedFinishUpPromptIsBestEffort(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	var r *blipRunner
	r = breachRunner(func(int) bool {
		r.FailNextPrompts("iter-19", 1, errors.New("finish-up never submitted"))
		return false
	})
	d := Deps{Runner: r, Sleep: func(time.Duration) {}}

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(scratchDir), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=true, want false: the finish-up prompt never went out")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only, no finish-up", prompts)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var sawFailed bool
	for _, e := range events {
		if e.Type == string(eventsc.SmartZoneRecoveryFailed) {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Error("missing smart-zone-recovery-failed event when the finish-up prompt fails")
	}
}

// staleReadingDeps wires a Claude Deps whose transcript reports occupancy
// tokens with the given staleness, plus the minimum needed for waitForFinish
// to poll: one timing-out tick, then idle.
func staleReadingDeps(tokens int, stale bool) Deps {
	return Deps{
		Runner: boundGiveUpRunner(func(wait int) bool { return wait == 1 }),
		ReadOccupancyReading: func(cwd, sessionID string) (transcript.OccupancyReading, error) {
			return transcript.OccupancyReading{
				Usage: transcript.Usage{InputTokens: tokens},
				Found: true,
				Stale: stale,
			}, nil
		},
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) { return tokens, true, nil },
		Sleep:         func(time.Duration) {},
	}
}

func TestWaitForFinish_StaleOccupancyAfterCompactionDoesNotRebreach(t *testing.T) {
	t.Parallel()
	sink := &occupancySink{}
	d := staleReadingDeps(200, true)
	r := d.Runner.(*blipRunner)

	p := boundGiveUpParams()
	p.Sink = sink
	err := waitForFinish(d, p, "sess-19")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if r.interrupts != 0 {
		t.Error("pane interrupted, want the over-budget pre-compaction number treated as unknown for breach purposes")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 0 {
		t.Errorf("prompts = %v, want no second compaction", prompts)
	}
	if len(sink.calls) != 1 || sink.calls[0].tokens != 200 {
		t.Errorf("ContextOccupancy calls = %+v, want the last known 200 still emitted for display", sink.calls)
	}
}

func TestWaitForFinish_FreshOccupancyStillBreaches(t *testing.T) {
	t.Parallel()
	d := staleReadingDeps(200, false)
	r := d.Runner.(*blipRunner)
	// The recovery this breach starts completes normally; the breach itself is
	// what this test is about.
	d.ReadCompactions = func(cwd, sessionID string) (int, bool, error) {
		return countPrompts(r.Prompts("iter-19"), "/compact"), true, nil
	}

	err := waitForFinish(d, boundGiveUpParams(), "sess-19")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if prompts := r.Prompts("iter-19"); r.interrupts != 1 || len(prompts) == 0 || prompts[0] != "/compact" {
		t.Errorf("interruptions = %d, prompts = %v, want one breach recovery", r.interrupts, prompts)
	}
}

func TestContextOccupancy_UnaffectedByStaleness(t *testing.T) {
	t.Parallel()
	d := staleReadingDeps(200, true)
	d.ReadOccupancyReading = func(cwd, sessionID string) (transcript.OccupancyReading, error) {
		t.Error("the general occupancy read reached for the staleness-aware reader, want it left to the smart-zone check")
		return transcript.OccupancyReading{}, nil
	}

	occupancy, ok, err := contextOccupancy(d, AgentClaude, "/repo/iter-19", "sess-19")
	if err != nil || !ok || occupancy != 200 {
		t.Fatalf("contextOccupancy() = %d, %v, %v; want the stamped/displayed value reported regardless", occupancy, ok, err)
	}

	sink := &occupancySink{}
	emitContextOccupancy(d, sink, AgentClaude, "19", "/repo/iter-19", "sess-19")
	if len(sink.calls) != 1 || sink.calls[0].tokens != 200 {
		t.Errorf("ContextOccupancy calls = %+v, want the iteration-started emission unaffected", sink.calls)
	}
}

// occupancySink is a minimal EventSink test double that only records
// ContextOccupancy calls, embedding noopEventSink for the rest.
type occupancySink struct {
	noopEventSink
	mu    sync.Mutex
	calls []occupancyCall
}

type occupancyCall struct {
	identifier string
	tokens     int
}

type quotaEventSink struct {
	noopEventSink
	paused  []quotaPauseEvent
	resumed []quotaResumeEvent
}

type quotaPauseEvent struct {
	identifier string
	kind       PauseKind
	reason     string
}

type quotaResumeEvent struct {
	identifier string
	kind       PauseKind
}

func (s *quotaEventSink) IterationPaused(identifier, label string, kind PauseKind, reason string) {
	s.paused = append(s.paused, quotaPauseEvent{identifier: label, kind: kind, reason: reason})
}

func (s *quotaEventSink) IterationResumed(identifier, label string, kind PauseKind) {
	s.resumed = append(s.resumed, quotaResumeEvent{identifier: label, kind: kind})
}

func (s *occupancySink) ContextOccupancy(identifier string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, occupancyCall{identifier: identifier, tokens: tokens})
}

func TestWaitForFinish_EmitsContextOccupancyOnEachPollTimeout(t *testing.T) {
	t.Parallel()
	sink := &occupancySink{}
	r := &blipRunner{Runner: idleRunner("iter-01"), timeoutIf: func(wait int) bool { return wait <= 2 }}
	d := Deps{
		Runner: r,
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 1000 * r.waits, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentClaude, Pane: "pane-1", Ticket: "01",
		SessionCwd: "/repo/iter-01", SmartZone: 1_000_000, Gate: NewGate(), Sink: sink,
	}, "sess-1")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if len(sink.calls) != 2 {
		t.Fatalf("ContextOccupancy calls = %+v, want 2 (one per poll timeout)", sink.calls)
	}
	for i, c := range sink.calls {
		if c.identifier != "01" {
			t.Errorf("call %d identifier = %q, want 01", i, c.identifier)
		}
	}
}

// TestWaitForFinish_BlockedPaneDwellsThenParks verifies ticket 14's core
// gate: a pane found blocked joins the wait's completion list (so the wait
// returns immediately instead of re-looping to a timeout), a single 15s
// dwell precedes a re-check via Runner.Status (a peek, not another Wait), and
// — the pane still being blocked at the end of that window — the iteration
// ends in a pane-answered park: needs-answer, a reason, and a "## Needs
// Answer" stub, both naming the iteration label rather than the raw pane id.
// It also covers the destructive-interrupt regression: no Prompt call
// is ever made while the pane is blocked, since typing into a pane sitting
// on an operator's own pending dialog would be exactly that.
func TestWaitForFinish_BlockedPaneDwellsThenParks(t *testing.T) {
	t.Parallel()
	for _, agentKind := range []AgentKind{AgentClaude, AgentCodex} {
		t.Run(string(agentKind), func(t *testing.T) {
			t.Parallel()
			ticketPath := writeFrontmatterTicket(t, "claimed")
			scratchDir := epicScratchDir(t, "epic")
			var slept []time.Duration
			r := blockedRunner("iter-01")
			d := Deps{
				Runner: r,
				Sleep:  func(d time.Duration) { slept = append(slept, d) },
			}

			err := waitForFinish(d, launchAndPromptParams{
				Label: "iter-01", Agent: agentKind, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
				ScratchDir: scratchDir, EpicName: "epic", Gate: NewGate(),
			}, "sess-1")
			if !errors.Is(err, errBlockedPaneParked) {
				t.Fatalf("waitForFinish() err = %v, want errBlockedPaneParked", err)
			}
			if len(slept) != 1 || slept[0] != blockedDwellMs*time.Millisecond {
				t.Errorf("Sleep calls = %v, want exactly one %v dwell", slept, blockedDwellMs*time.Millisecond)
			}
			if prompts := r.Prompts("iter-01"); len(prompts) != 0 {
				t.Errorf("prompts = %v, want none; must never interrupt a pending operator prompt", prompts)
			}

			raw, err := os.ReadFile(ticketPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			ticket, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
			if err != nil {
				t.Fatalf("ParseTicketFromRaw: %v", err)
			}
			if ticket.Status != schema.StatusNeedsAnswer {
				t.Errorf("Status = %q, want needs-answer", ticket.Status)
			}
			if ticket.ParkKind != schema.ParkKindBlockedPane {
				t.Errorf("ParkKind = %q, want blocked-pane", ticket.ParkKind)
			}
			body := schema.ParseBody(string(raw))
			if !strings.Contains(body, "## Needs Answer") {
				t.Errorf("body missing ## Needs Answer stub:\n%s", body)
			}
			if !strings.Contains(body, "iter-01") {
				t.Errorf("body does not name the iteration label iter-01:\n%s", body)
			}

			events, ok, err := ReadEvents(scratchDir, "epic")
			if err != nil || !ok || len(events) == 0 {
				t.Fatalf("ReadEvents() = %+v, ok=%v, err=%v", events, ok, err)
			}
			last := events[len(events)-1]
			if last.Type != string(eventsc.NeedsAnswer) || !strings.Contains(last.Reason, "iter-01") {
				t.Errorf("park event = %+v, want type needs-answer with reason naming iter-01", last)
			}
			// Seam B: exactly one park event, kind matching the frontmatter park_kind.
			var parks int
			for _, ev := range events {
				if ev.Type == string(eventsc.NeedsAnswer) {
					parks++
				}
			}
			if parks != 1 || last.Kind != string(ticket.ParkKind) || last.Kind != "blocked-pane" || last.Pane != "pane-1" {
				t.Errorf("parks = %d, last = %+v, want one blocked-pane event matching park_kind with pane context", parks, last)
			}
		})
	}
}

// TestWaitForFinish_BlockedPaneClearsBeforeDwellRecheck_DoesNotPark verifies
// that the dwell's single re-check, not the initial observation, decides the
// park: a pane that is no longer blocked by the time the 15s window ends
// keeps the iteration running instead of parking it.
func TestWaitForFinish_BlockedPaneClearsBeforeDwellRecheck_DoesNotPark(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	r := blockedRunner("iter-01")
	d := Deps{
		Runner: r,
		// The pane clears during the dwell; it then finishes.
		Sleep: func(time.Duration) { r.SetState("iter-01", agentrunner.StateIdle, "") },
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentClaude, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		Gate: NewGate(),
	}, "sess-1")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "needs-answer") {
		t.Errorf("ticket was parked despite the pane clearing before the dwell recheck:\n%s", raw)
	}
}

// TestWaitForFinish_BlockedPaneDwellIsFixedWindow_NotASettleTimer verifies
// the dwell's shape: parkOnBlockedPane sleeps once and re-checks once,
// rather than polling the pane during the window. A pane that left and
// re-entered the blocked state inside the window (indistinguishable from
// this fixture, which never observes anything mid-window) still parks
// purely off the single end-of-window read — proven here by Runner.Wait never
// being asked again once the dwell starts.
func TestWaitForFinish_BlockedPaneDwellIsFixedWindow_NotASettleTimer(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	r := &blipRunner{Runner: blockedRunner("iter-01")}
	d := Deps{
		Runner: r,
		Sleep:  func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentClaude, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		Gate: NewGate(),
	}, "sess-1")
	if !errors.Is(err, errBlockedPaneParked) {
		t.Fatalf("waitForFinish() err = %v, want errBlockedPaneParked", err)
	}
	if r.waits != 1 {
		t.Errorf("Runner.Wait calls = %d, want exactly 1 (dwell must not re-poll the pane)", r.waits)
	}
}

// TestWaitForFinish_InverseGuard_BlockedAfterOwnSmartZoneRecoveryNotParked
// verifies ticket 14's inverse guard: gx's own smart-zone breach recovery
// (ctrl+c, then /compact) can leave a pane reporting blocked as a side
// effect, and the very next poll tick observing that must not be mistaken
// for an operator-raised prompt and parked.
func TestWaitForFinish_InverseGuard_BlockedAfterOwnSmartZoneRecoveryNotParked(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	var readCompactionsCalls int
	fake := idleRunner("iter-01")
	fake.PromptState = agentrunner.StateIdle
	d := Deps{
		Runner: &blipRunner{Runner: fake, finishPollsOnly: true, timeoutIf: func(wait int) bool {
			switch wait {
			case 1:
				// Times out, driving the smart-zone breach branch.
				return true
			case 2:
				// The tick right after recoverSmartZoneBreach returns: blocked
				// as its own artifact, must be guarded against parking.
				fake.SetState("iter-01", agentrunner.StateBlocked, "")
			case 3:
				fake.SetState("iter-01", agentrunner.StateIdle, "")
			}
			return false
		}},
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 2_000_000, true, nil
		},
		// The first two reads are the pre-"/compact" baselines (waitForFinish's
		// own, then recoverSmartZoneBreach's own newStickyBaseline); the third
		// is the advancement check on recovery's first poll tick, so the
		// compaction reads as already confirmed.
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			readCompactionsCalls++
			if readCompactionsCalls <= 2 {
				return 0, true, nil
			}
			return 1, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentClaude, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		SmartZone: 1_000_000, Gate: NewGate(),
	}, "sess-1")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "needs-answer") {
		t.Errorf("ticket was parked off a pane blocked by gx's own smart-zone recovery:\n%s", raw)
	}
}

// TestWaitForFinish_BlockedPane_ParksWithoutResend covers ticket 05: the
// park-on-blocked resend hatch was unreachable dead code and has been
// deleted, so a blocked pane always parks with no prompt attempted,
// whatever its state_change_seq.
func TestWaitForFinish_BlockedPane_ParksWithoutResend(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")

	r := blockedRunner("iter-01")
	d := Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentClaude, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		Gate: NewGate(),
	}, "sess-1")
	if !errors.Is(err, errBlockedPaneParked) {
		t.Fatalf("waitForFinish() err = %v, want errBlockedPaneParked", err)
	}
	if prompts := r.Prompts("iter-01"); len(prompts) != 0 {
		t.Errorf("prompts = %v, want none; a blocked pane must never be prompted", prompts)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "needs-answer") {
		t.Errorf("ticket was not parked despite a blocked pane:\n%s", raw)
	}
	if !strings.Contains(string(raw), "iter-01") {
		t.Errorf("park reason does not name the iteration label:\n%s", raw)
	}
}

// TestWaitForFinish_CodexQuotaDoesNotBecomeNeedsRepair covers a Codex pane
// still blocked once its quota resets: per ticket 04, that must park for a
// human (needs-answer), never a hard failure that stalled-agent detection
// would flag needs-repair, and never a "continue" re-prompt into a pane the
// runner reports blocked.
func TestWaitForFinish_CodexQuotaDoesNotBecomeNeedsRepair(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	scratchDir := t.TempDir()
	gate := NewGate()
	sink := &quotaEventSink{}
	var sawPausedGate bool
	r := &blipRunner{Runner: blockedRunner("iter-01")}
	r.SetState("iter-01", agentrunner.StateBlocked, "approval_prompt")
	r.SetRateLimit("iter-01", time.Now().Add(-time.Second))
	d := Deps{
		Runner: r,
		// The reset's own recheck is the first Sleep: the quota clears there
		// while the pane stays blocked.
		Sleep: func(time.Duration) {
			sawPausedGate = gate.isPaused()
			r.SetRateLimit("iter-01", time.Time{})
		},
		Now: time.Now,
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		ScratchDir: scratchDir, EpicName: "epic", Gate: gate, Sink: sink,
	}, "codex-session-1")
	if !errors.Is(err, errBlockedPaneParked) {
		t.Fatalf("waitForFinish() err = %v, want errBlockedPaneParked", err)
	}
	if prompts := r.Prompts("iter-01"); len(prompts) != 0 {
		t.Errorf("prompts = %v, want none — a blocked pane must never be prompted", prompts)
	}
	if r.interrupts != 0 {
		t.Errorf("pane interruptions = %d, want 0", r.interrupts)
	}
	if !sawPausedGate {
		t.Error("shared gate was not paused while waiting for the Codex quota reset")
	}
	if gate.isPaused() {
		t.Error("gate remains paused after the Codex quota reset")
	}
	if len(sink.paused) != 1 || sink.paused[0].identifier != "iter-01" ||
		sink.paused[0].kind != PauseRateLimit || !strings.Contains(sink.paused[0].reason, "Codex quota exhausted") {
		t.Errorf("paused events = %+v, want one typed rate-limit pause", sink.paused)
	}
	if len(sink.resumed) != 1 || sink.resumed[0] != (quotaResumeEvent{identifier: "iter-01", kind: PauseRateLimit}) {
		t.Errorf("resumed events = %+v, want one typed rate-limit resume", sink.resumed)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "needs-answer") || strings.Contains(string(raw), "needs-repair") {
		t.Errorf("ticket status = %s, want needs-answer without needs-repair", raw)
	}
	if !strings.Contains(string(raw), "approval_prompt") {
		t.Errorf("ticket = %s, want the park reason to name the runner's blocked reason", raw)
	}
}

func TestWaitForFinish_CodexQuotaDetectionErrorPreservesClaimedTicket(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	r := blockedRunner("iter-01")
	r.SetRateLimitErr("iter-01", errors.New("rollout unreadable"))
	d := Deps{Runner: r}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", TicketPath: ticketPath, Gate: NewGate(),
	}, "session-1")
	if err == nil || !strings.Contains(err.Error(), "rollout unreadable") {
		t.Fatalf("waitForFinish error = %v, want rollout read failure", err)
	}
	raw, readErr := os.ReadFile(ticketPath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if !strings.Contains(string(raw), "claimed") || strings.Contains(string(raw), "needs-repair") {
		t.Errorf("ticket status = %s, want claimed without needs-repair", raw)
	}
}

func TestWaitForFinish_CodexQuotaDoesNotBecomeNeedsRepairWhenPaneIdlesAfterReset(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	gate := NewGate()
	sink := &quotaEventSink{}
	r := blockedRunner("iter-01")
	r.SetLimitedUnknownReset("iter-01")
	d := Deps{
		Runner: r,
		// The pane comes back idle once the quota resets.
		Sleep: func(time.Duration) {
			r.SetRateLimit("iter-01", time.Time{})
			r.SetState("iter-01", agentrunner.StateIdle, "")
		},
	}

	if err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", TicketPath: ticketPath, Gate: gate, Sink: sink,
	}, "session-1"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if len(sink.paused) != 1 || sink.paused[0].kind != PauseRateLimit || len(sink.resumed) != 1 {
		t.Errorf("quota events = paused %+v, resumed %+v; want one of each", sink.paused, sink.resumed)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "claimed") || strings.Contains(string(raw), "needs-repair") {
		t.Errorf("ticket status = %s, want claimed without needs-repair", raw)
	}
}

// stickyBaselineDeps is the premature-idle pane the sticky-baseline tests
// share: with r a breachRunner that never times out, "/compact" and every
// subsequent poll report idle immediately, so the only thing that can ever end
// the recovery is the transcript.
func stickyBaselineDeps(
	r *blipRunner, sleeps *int,
	readCompactions func(string, string) (int, bool, error),
	readCompactionsAfter func(string, string, time.Time) (int, bool, error),
) Deps {
	return Deps{
		Runner:               r,
		ReadCompactions:      readCompactions,
		ReadCompactionsAfter: readCompactionsAfter,
		Sleep:                func(time.Duration) { *sleeps++ },
	}
}

// noBoundarySinceSubmission is the after-submission read for a recovery whose
// compaction never lands.
func noBoundarySinceSubmission(string, string, time.Time) (int, bool, error) { return 0, true, nil }

// TestRecoverSmartZoneBreach_UnavailableBaselineConfirmsOnABoundaryAfterSubmission
// verifies that a transcript read failing at "/compact" submission time doesn't
// disable the gate for the whole recovery: with no baseline count to compare
// against, the gate switches to "was a boundary written after submission" and
// still refuses the pane's premature idle report until one is.
func TestRecoverSmartZoneBreach_UnavailableBaselineConfirmsOnABoundaryAfterSubmission(t *testing.T) {
	t.Parallel()
	var sleeps int
	landed := 0
	r := breachRunner(func(wait int) bool {
		if wait == 3 {
			landed = 1
		}
		return false
	})
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			return 0, false, errors.New("transcript read failed")
		},
		func(string, string, time.Time) (int, bool, error) {
			return landed, true, nil
		},
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true: the boundary does land after submission")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up] once a boundary lands", prompts)
	}
	if sleeps == 0 {
		t.Error("gated sleeps = 0, want the gate held closed until a boundary landed after submission")
	}
}

// TestRecoverSmartZoneBreach_FastCompactionUnderAnUnavailableBaselineIsConfirmed
// covers the race the after-submission predicate exists for: the pre-submission
// read blips, the compaction then completes inside the first tick, and every
// count read afterwards already includes the new boundary. Comparing counts
// could only ever report "not advanced" here, turning a successful compaction
// into ten minutes of waiting and a gated give-up.
func TestRecoverSmartZoneBreach_FastCompactionUnderAnUnavailableBaselineIsConfirmed(t *testing.T) {
	t.Parallel()
	var sleeps int
	r := breachRunner(nil)
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			if len(r.Prompts("iter-19")) == 0 {
				return 0, false, errors.New("transcript read failed")
			}
			return 6, true, nil // already includes the boundary this recovery caused
		},
		func(string, string, time.Time) (int, bool, error) { return 1, true, nil },
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true: the compaction genuinely completed")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up]", prompts)
	}
	if r.waits != 1 || sleeps != 0 {
		t.Errorf("Runner.Wait calls = %d, gated sleeps = %d; want 1 and 0: an already-landed boundary confirms on the first poll tick", r.waits, sleeps)
	}
}

// TestRecoverSmartZoneBreach_UnavailableBaselineNeverRebasesOnALaterCount
// pins the invariant that keeps the unavailable case out of a deadlock: a count
// that only becomes readable mid-recovery is never adopted as the baseline. Its
// value is already past the true submission-time one, so "count is greater than
// baseline" would be unsatisfiable forever and a successful compaction would be
// reported as a give-up. With no boundary landing after submission here, the
// recovery must give up on the extended bound — never on that stale comparison.
func TestRecoverSmartZoneBreach_UnavailableBaselineNeverRebasesOnALaterCount(t *testing.T) {
	t.Parallel()
	var sleeps, reads int
	r := breachRunner(nil)
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			reads++
			if reads == 1 { // the submission-time read
				return 0, false, errors.New("transcript read failed")
			}
			return 5, true, nil
		},
		noBoundarySinceSubmission,
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
	}
	if recovered {
		t.Error("recoverSmartZoneBreach returned recovered=true, want false: a late baseline is stale and can only deadlock the gate")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only: no finish-up prompt on a never-confirmed compaction", prompts)
	}
}

// TestRecoverSmartZoneBreach_UnavailableReadsInLoopHoldGateClosed verifies
// that transcript reads failing intermittently mid-recovery are treated as
// "not yet", not as "no baseline, trust the pane" — that second reading is the
// same silent-disable failure in a different disguise. A persistent read
// problem must surface as an ordinary gated give-up, paced by the extended
// bound.
func TestRecoverSmartZoneBreach_UnavailableReadsInLoopHoldGateClosed(t *testing.T) {
	t.Parallel()
	var sleeps, reads int
	r := breachRunner(nil)
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			reads++
			if reads == 1 {
				return 0, true, nil // a readable baseline, then the transcript goes flaky
			}
			if reads%2 == 0 {
				return 0, false, errors.New("transcript read failed")
			}
			return 0, false, nil // exists-but-unreadable and not-yet-existing both hold
		},
		noBoundarySinceSubmission,
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
	}
	if recovered {
		t.Error("recoverSmartZoneBreach returned recovered=true, want false: an unreadable transcript confirms nothing")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only", prompts)
	}
	wantSleeps := smartZoneCompactExtendedTimeoutMs / smartZonePollMs
	if sleeps != wantSleeps {
		t.Errorf("gated sleeps = %d, want %d: failed reads must consume the extended bound, not short-circuit it", sleeps, wantSleeps)
	}
}

// TestRecoverSmartZoneBreach_MissingTranscriptVersusUnsupportedAgent
// discriminates the two states that arrive as the same (0, false, nil) read.
// A Claude transcript that doesn't exist yet is temporarily unavailable and
// holds the gate closed; a Codex session has no boundary signal at all and
// still fails open, never routed through the closed-gate policy.
func TestRecoverSmartZoneBreach_MissingTranscriptVersusUnsupportedAgent(t *testing.T) {
	t.Parallel()
	missing := func(string, string) (int, bool, error) { return 0, false, nil }

	t.Run("Claude transcript not written yet holds the gate closed", func(t *testing.T) {
		t.Parallel()
		var sleeps int
		r := breachRunner(nil)
		d := stickyBaselineDeps(r, &sleeps, missing, noBoundarySinceSubmission)
		recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
		if !errors.Is(err, errCompactNeverConfirmed) {
			t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
		}
		if prompts := r.Prompts("iter-19"); recovered || len(prompts) != 1 {
			t.Errorf("recovered = %v, prompts = %v; want no finish-up prompt for an unavailable transcript", recovered, prompts)
		}
	})

	t.Run("Codex has no boundary signal and fails open", func(t *testing.T) {
		t.Parallel()
		var sleeps int
		r := breachRunner(nil)
		d := stickyBaselineDeps(r, &sleeps, missing, noBoundarySinceSubmission)
		p := gatedBreachParams(t.TempDir())
		p.Agent = AgentCodex
		recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
		if err != nil {
			t.Fatalf("recoverSmartZoneBreach: %v", err)
		}
		if prompts := r.Prompts("iter-19"); !recovered || len(prompts) != 2 {
			t.Errorf("recovered = %v, prompts = %v; want the pre-gate behavior for an agent with no boundary signal", recovered, prompts)
		}
		if r.waits != 1 || sleeps != 0 {
			t.Errorf("Runner.Wait calls = %d, gated sleeps = %d; want 1 and 0: an unsupported agent trusts the first idle tick", r.waits, sleeps)
		}
	})
}

// idleBackgroundTaskDeps is the base Deps for waitForBackgroundTasks tests: a
// pane that reports idle on every AgentWait (including confirmFinished's own
// re-check), so readBackgroundTasks is the only thing that can hold
// waitForFinish's conclusion open.
func idleBackgroundTaskDeps(readBackgroundTasks func(cwd, sessionID string) (transcript.BackgroundTaskReading, error), sleeps *int) Deps {
	return Deps{
		Runner:              idleRunner("iter-30"),
		ReadBackgroundTasks: readBackgroundTasks,
		Sleep:               func(time.Duration) { *sleeps++ },
	}
}

// idleRunner hosts one idle session under label, with ID "pane-1" and
// SessionID "sess-1".
func idleRunner(label string) *runnerfake.Runner {
	return paneRunner(label, "pane-1", "sess-1")
}

// blockedRunner is idleRunner with its session blocked.
func blockedRunner(label string) *runnerfake.Runner {
	r := idleRunner(label)
	r.SetState(label, agentrunner.StateBlocked, "")
	return r
}

// paneRunner hosts one idle session of label on pane, so a Wait aimed at any
// other pane fails with ErrNotFound.
func paneRunner(label, pane, sessionID string) *runnerfake.Runner {
	r := runnerfake.NewRunner()
	r.IDs = func(string) (string, string) { return pane, sessionID }
	if _, err := r.Start(agentrunner.StartOptions{Label: label}); err != nil {
		panic(err)
	}
	return r
}

// idlePromptRunner is paneRunner on pane-1 whose agent reports idle the moment
// it is prompted, so a wait after "/compact" never blocks in real time.
func idlePromptRunner(label, sessionID string) *runnerfake.Runner {
	r := paneRunner(label, "pane-1", sessionID)
	r.PromptState = agentrunner.StateIdle
	return r
}

func backgroundTaskGateParams(scratchDir string) launchAndPromptParams {
	return launchAndPromptParams{
		Label: "iter-30", Agent: AgentClaude, Pane: "pane-1", Ticket: "30",
		SessionCwd: "/repo/iter-30", Gate: NewGate(),
		ScratchDir: scratchDir, EpicName: "epic",
	}
}

// TestWaitForFinish_BackgroundTaskGateHoldsUntilResolved covers the core gate
// contract: a pane that debounce-confirms idle while its transcript still
// shows an outstanding-fresh background task must not be reported finished
// until that task resolves.
func TestWaitForFinish_BackgroundTaskGateHoldsUntilResolved(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		status := transcript.BackgroundTaskOutstandingFresh
		if reads >= 3 {
			status = transcript.BackgroundTaskResolved
		}
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}},
		}, nil
	}
	d := idleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

	if err := waitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if reads != 3 {
		t.Errorf("ReadBackgroundTasks calls = %d, want 3 (held, held, resolved)", reads)
	}
	// One sleep from confirmFinished's own debounce, two more pacing the
	// gate's re-reads while the marker stayed outstanding-fresh, plus one more
	// from the recheck confirmFinished runs once the gate releases.
	if sleeps != 4 {
		t.Errorf("Sleep calls = %d, want 4: the gate must pace re-reads at smartZonePollMs, not spin, and recheck idle once it releases", sleeps)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var held, released int
	for _, ev := range events {
		switch ev.Type {
		case string(eventsc.BackgroundTaskGateHeld):
			held++
			if !strings.Contains(ev.Reason, "task-1") {
				t.Errorf("gate-held reason = %q, want it to name the task id", ev.Reason)
			}
		case string(eventsc.BackgroundTaskGateReleased):
			released++
		}
	}
	if held != 1 {
		t.Errorf("gate-held events = %d, want exactly 1 (once per task id, not once per poll tick)", held)
	}
	if released != 1 {
		t.Errorf("gate-released events = %d, want exactly 1", released)
	}
}

// TestWaitForFinish_RecoveryForceReleasesAHeldGate covers R10's hook: a task
// that never resolves stops holding once GateReleased reports true.
func TestWaitForFinish_RecoveryForceReleasesAHeldGate(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh}},
		}, nil
	}
	d := idleBackgroundTaskDeps(readBackgroundTasks, &sleeps)
	d.GateReleased = func() bool { return reads >= 2 }

	if err := waitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if reads != 2 {
		t.Errorf("ReadBackgroundTasks calls = %d, want 2 (held, released by recovery)", reads)
	}
	events, _, err := ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var types []string
	for _, ev := range events {
		switch ev.Type {
		case string(eventsc.BackgroundTaskGateHeld), string(eventsc.BackgroundTaskGateReleased):
			types = append(types, ev.Type+" "+ev.TaskID+" "+ev.Reason)
		}
	}
	want := []string{
		string(eventsc.BackgroundTaskGateHeld) + " task-1 background task task-1",
		string(eventsc.BackgroundTaskGateReleased) + " task-1 background task task-1: recovery forced the release",
	}
	if !slices.Equal(types, want) {
		t.Errorf("gate events = %q, want %q", types, want)
	}
}

// TestWaitForFinish_BackgroundTaskAgesOutAndFallsThrough covers the ~2h cap:
// once a held marker reads outstanding-aged-out, the gate stops holding on
// it and waitForFinish falls through to a plain finish, logging gate-expired
// (never gate-released) for that marker.
func TestWaitForFinish_BackgroundTaskAgesOutAndFallsThrough(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		status := transcript.BackgroundTaskOutstandingFresh
		if reads >= 2 {
			status = transcript.BackgroundTaskOutstandingAgedOut
		}
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}},
		}, nil
	}
	d := idleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

	if err := waitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("waitForFinish: %v, want the aged-out marker to fall through to a plain finish", err)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var expired, released int
	for _, ev := range events {
		switch ev.Type {
		case string(eventsc.BackgroundTaskGateExpired):
			expired++
		case string(eventsc.BackgroundTaskGateReleased):
			released++
		}
	}
	if expired != 1 {
		t.Errorf("gate-expired events = %d, want exactly 1", expired)
	}
	if released != 0 {
		t.Errorf("gate-released events = %d, want 0: this marker aged out, it never resolved", released)
	}
}

// TestWaitForFinish_BackgroundTaskGateReleaseRechecksIdle covers the false-
// finish this gate can otherwise produce: a background task resolving only
// proves that one task's own notification landed, not that the agent's
// overall turn is over — seeing the result is exactly when it's likely to
// resume real work. If the pane is back to "working" by the time the gate
// releases, waitForFinish must not declare the iteration done off the stale
// idle signal from before the gate started holding; it must keep polling
// until a later idle genuinely holds up.
func TestWaitForFinish_BackgroundTaskGateReleaseRechecksIdle(t *testing.T) {
	t.Parallel()
	scratchDir := epicScratchDir(t, "epic")
	var sleeps, reads int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		reads++
		status := transcript.BackgroundTaskOutstandingFresh
		if reads >= 2 {
			status = transcript.BackgroundTaskResolved
		}
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}},
		}, nil
	}
	// waits: 1 = outer loop's first wait (idle); 2 = that idle's own
	// confirmFinished recheck (still idle); 3 = the gate-release recheck this
	// fix adds (agent resumed work, not idle); 4 = outer loop's second wait,
	// once the agent is genuinely done; 5 = that second idle's own
	// confirmFinished recheck (idle; no gate re-hold since ReadBackgroundTasks
	// already reports resolved).
	r := &blipRunner{Runner: idleRunner("iter-30"), timeoutOn: 3}
	d := Deps{
		Runner:              r,
		ReadBackgroundTasks: readBackgroundTasks,
		Sleep:               func(time.Duration) { sleeps++ },
	}

	if err := waitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if r.waits < 4 {
		t.Errorf("Runner.Wait calls = %d, want at least 4: the gate-release recheck finding the pane busy must send waitForFinish back around its outer poll loop", r.waits)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	var held, released int
	for _, ev := range events {
		switch ev.Type {
		case string(eventsc.BackgroundTaskGateHeld):
			held++
		case string(eventsc.BackgroundTaskGateReleased):
			released++
		}
	}
	if held != 1 || released != 1 {
		t.Errorf("gate-held/-released events = %d/%d, want exactly 1/1 (only the first outstanding task ever gates)", held, released)
	}
}

// TestWaitForFinish_BackgroundTaskUnreadableAndUnsupportedNeverHoldGate covers
// the fail-open requirement: neither a transcript that failed to parse nor an
// agent with no transcript at all (Codex, via ReadBackgroundTasks == nil at
// the call site) is evidence, so waitForFinish must conclude on the first
// debounce-confirmed idle exactly as it would with no reader wired at all.
func TestWaitForFinish_BackgroundTaskUnreadableAndUnsupportedNeverHoldGate(t *testing.T) {
	t.Parallel()

	t.Run("unreadable transcript fails open", func(t *testing.T) {
		t.Parallel()
		scratchDir := t.TempDir()
		var sleeps int
		readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
			return transcript.BackgroundTaskReading{FileStatus: transcript.BackgroundTaskUnreadable}, nil
		}
		d := idleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

		if err := waitForFinish(d, backgroundTaskGateParams(scratchDir), "sess-30"); err != nil {
			t.Fatalf("waitForFinish: %v", err)
		}
		if sleeps != 1 {
			t.Errorf("Sleep calls = %d, want exactly 1 (confirmFinished's debounce only)", sleeps)
		}
	})

	t.Run("unsupported (Codex) agent never even calls the reader", func(t *testing.T) {
		t.Parallel()
		scratchDir := t.TempDir()
		var sleeps, reads int
		readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
			reads++
			return transcript.BackgroundTaskReading{FileStatus: transcript.BackgroundTaskUnsupported}, nil
		}
		d := idleBackgroundTaskDeps(readBackgroundTasks, &sleeps)
		p := backgroundTaskGateParams(scratchDir)
		p.Agent = AgentCodex

		if err := waitForFinish(d, p, "sess-30"); err != nil {
			t.Fatalf("waitForFinish: %v", err)
		}
		if reads != 0 {
			t.Errorf("ReadBackgroundTasks calls = %d, want 0: waitForBackgroundTasks must not call it for Codex", reads)
		}
		if sleeps != 1 {
			t.Errorf("Sleep calls = %d, want exactly 1 (confirmFinished's debounce only)", sleeps)
		}
	})
}

// TestWaitForFinish_BackgroundTaskGateAccumulatesAgainstFinishTimeoutMs covers
// the elapsed-time contract: time spent holding the gate must accumulate
// against a caller-set FinishTimeoutMs (never reset), so a bound shorter than
// the ~2h aged-out cap still fires while the gate is held.
func TestWaitForFinish_BackgroundTaskGateAccumulatesAgainstFinishTimeoutMs(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	var sleeps int
	readBackgroundTasks := func(string, string) (transcript.BackgroundTaskReading, error) {
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: transcript.BackgroundTaskOutstandingFresh}},
		}, nil
	}
	d := idleBackgroundTaskDeps(readBackgroundTasks, &sleeps)

	p := backgroundTaskGateParams(scratchDir)
	p.FinishTimeoutMs = smartZonePollMs / 2

	err := waitForFinish(d, p, "sess-30")
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("waitForFinish error = %v, want a FinishTimeoutMs timeout while the gate held", err)
	}
}
