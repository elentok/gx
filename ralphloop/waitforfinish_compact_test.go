package ralphloop

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
)

func TestWaitForFinish_CodexContextBreachRecovers(t *testing.T) {
	t.Parallel()
	ticketPath := fixture.WriteFrontmatterTicket(t, "claimed")
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
		Agent:      agent.AgentCodex,
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
	if rawIsPaused(t, string(raw), "") {
		t.Errorf("ticket status = %s, a recovered breach must not become paused", raw)
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
	scratchDir := fixture.EpicScratchDir(t, "epic")
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

	events, ok, err := runlog.ReadEvents(scratchDir, "epic")
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
	scratchDir := fixture.EpicScratchDir(t, "epic")
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

	events, ok, err := runlog.ReadEvents(scratchDir, "epic")
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
			// The real compaction merges between the second and third poll
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
		agent     agent.AgentKind
		sessionID string
		read      func(string, string) (int, bool, error)
		want      compactBoundarySnapshot
	}{
		{
			name:  "Codex read count is confirmed",
			agent: agent.AgentCodex, sessionID: "sess-19", read: reads(4, true, nil),
			want: compactBoundarySnapshot{state: compactBoundaryConfirmed, count: 4},
		},
		{
			name:  "Codex read error is unavailable",
			agent: agent.AgentCodex, sessionID: "sess-19", read: reads(0, false, errors.New("read failed")),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "Codex empty session id is unavailable",
			agent: agent.AgentCodex, sessionID: "", read: reads(4, true, nil),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "Codex nil read dependency is unsupported",
			agent: agent.AgentCodex, sessionID: "sess-19", read: nil,
			want: compactBoundarySnapshot{state: compactBoundaryUnsupported},
		},
		{
			name:  "nil read dependency is unsupported",
			agent: agent.AgentClaude, sessionID: "sess-19", read: nil,
			want: compactBoundarySnapshot{state: compactBoundaryUnsupported},
		},
		{
			name:  "empty session id is unavailable, not unsupported",
			agent: agent.AgentClaude, sessionID: "", read: reads(4, true, nil),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "read error is unavailable",
			agent: agent.AgentClaude, sessionID: "sess-19", read: reads(0, false, errors.New("read failed")),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "transcript that does not exist yet is unavailable",
			agent: agent.AgentClaude, sessionID: "sess-19", read: reads(0, false, nil),
			want: compactBoundarySnapshot{state: compactBoundaryUnavailable},
		},
		{
			name:  "a read count is confirmed",
			agent: agent.AgentClaude, sessionID: "sess-19", read: reads(4, true, nil),
			want: compactBoundarySnapshot{state: compactBoundaryConfirmed, count: 4},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := Deps{ReadCompactions: tc.read}
			if tc.agent == agent.AgentCodex {
				d = Deps{ReadCodexCompactions: tc.read}
			}
			got := readCompactBoundaries(d, tc.agent, "/repo/iter-19", tc.sessionID)
			if got != tc.want {
				t.Errorf("readCompactBoundaries() = %+v, want %+v", got, tc.want)
			}
		})
	}
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
// merges part-way through. Recovery must resume the moment the boundary count
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
	events, ok, err := runlog.ReadEvents(scratchDir, "epic")
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
// boundary merged a few ticks later. Nothing about that wait expired, so it
// must log the gated event rather than borrowing the timeout one.
func TestRecoverSmartZoneBreach_GatedCompletionLogsItsOwnEvent(t *testing.T) {
	t.Parallel()
	scratchDir := fixture.EpicScratchDir(t, "epic")
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
	scratchDir := fixture.EpicScratchDir(t, "epic")
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
	scratchDir := fixture.EpicScratchDir(t, "epic")
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
		// The compaction merges for real once recovery has already given up
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
