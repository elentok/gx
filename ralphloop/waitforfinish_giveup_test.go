package ralphloop

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/transcript"
)

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
	thirdMerged := false
	r := boundGiveUpRunner(nil)
	compacts := func() int { return countPrompts(r.Prompts("iter-19"), "/compact") }
	r.timeoutIf = func(int) bool {
		// The agent wraps up on its own after the third breach, so the run
		// must reach a normal finish rather than escalating.
		if compacts() < 3 {
			return true
		}
		// The third compaction merges only once recovery has given up on it,
		// so the finish the pane then reports is corroborated.
		thirdMerged = true
		return false
	}
	d := Deps{
		Runner:        r,
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		// Only the second compaction merges while its own recovery is watching.
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			switch {
			case thirdMerged:
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
		// "/compact" merges arms that failure.
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

// TestWaitForFinish_InverseGuard_BlockedAfterOwnSmartZoneRecoveryNotPaused
// verifies ticket 14's inverse guard: gx's own smart-zone breach recovery
// (ctrl+c, then /compact) can leave a pane reporting blocked as a side
// effect, and the very next poll tick observing that must not be mistaken
// for an operator-raised prompt and paused.
func TestWaitForFinish_InverseGuard_BlockedAfterOwnSmartZoneRecoveryNotPaused(t *testing.T) {
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
				// as its own artifact, must be guarded against pausing.
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
	if rawIsPaused(t, string(raw), "") {
		t.Errorf("ticket was paused off a pane blocked by gx's own smart-zone recovery:\n%s", raw)
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
	if !strings.Contains(string(raw), "claimed") || rawIsPaused(t, string(raw), "") {
		t.Errorf("ticket status = %s, want claimed without a pause", raw)
	}
}
