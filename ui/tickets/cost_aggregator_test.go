package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// withStubbedCostReads swaps epicLandedCostsFn/sessionCostFn for the
// duration of the test, restoring the real ralphloop-backed implementations
// on cleanup.
func withStubbedCostReads(t *testing.T, epicFn func(scratchDir, epicName string) (float64, map[string]float64, error), sessionFn func(cwd, sessionID string) (float64, bool, error)) {
	t.Helper()
	prevEpic, prevSession := epicLandedCostsFn, sessionCostFn
	epicLandedCostsFn = epicFn
	sessionCostFn = sessionFn
	t.Cleanup(func() {
		epicLandedCostsFn = prevEpic
		sessionCostFn = prevSession
	})
}

// resetCostAgg starts the test from a clean aggregator and leaves a clean one
// behind, since costAgg is a package-level singleton.
func resetCostAgg(t *testing.T) {
	t.Helper()
	costAgg.reset()
	t.Cleanup(costAgg.reset)
}

// costEpic builds one epic's tick input; tickets maps identifier to ticket.
func costEpic(epicName string, tickets map[string]costTicketSnapshot) epicCostSnapshot {
	return epicCostSnapshot{EpicName: epicName, ScratchDir: "/scratch", Tickets: tickets}
}

func runningTicket(cwd, sessionID string, agent ralphloop.AgentKind) costTicketSnapshot {
	return costTicketSnapshot{Running: true, Cwd: cwd, SessionID: sessionID, Agent: agent}
}

func TestCostAggregatorTickBaselinesEpicOnFirstObservation(t *testing.T) {
	resetCostAgg(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 4.0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)

	costAgg.tick([]epicCostSnapshot{costEpic("epic-a", nil)})

	if got := LiveSpend(); got != 0 {
		t.Fatalf("LiveSpend() = %v after baselining tick, want 0 (baseline == current landed cost)", got)
	}
	if got := LiveSpendByEpic()["epic-a"]; got != 0 {
		t.Fatalf("LiveSpendByEpic()[epic-a] = %v, want 0", got)
	}
}

func TestCostAggregatorTickSumsLandedSinceBaselinePlusInFlight(t *testing.T) {
	// not parallel-safe: writeFakeTranscript sets the process HOME env var
	resetCostAgg(t)
	landed := 4.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return landed, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 1.5, true, nil },
	)

	cwd := t.TempDir()
	sessionID := "sess-1"
	writeFakeTranscript(t, cwd, sessionID)

	snap := []epicCostSnapshot{costEpic("epic-a", map[string]costTicketSnapshot{
		"01": runningTicket(cwd, sessionID, ralphloop.AgentClaude),
	})}
	costAgg.tick(snap) // baseline == 4.0

	landed = 6.0 // epic-wide landed cost rose by $2 since baseline
	costAgg.tick(snap)

	want := (6.0 - 4.0) + 1.5
	if got := LiveSpend(); got != want {
		t.Fatalf("LiveSpend() = %v, want %v", got, want)
	}
	if got := LiveSpendByEpic()["epic-a"]; got != want {
		t.Fatalf("LiveSpendByEpic()[epic-a] = %v, want %v", got, want)
	}
}

func TestCostAggregatorRelaunchWithinSessionKeepsOriginalBaseline(t *testing.T) {
	resetCostAgg(t)
	landed := 4.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return landed, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)

	// A second epic keeps the Attach session (and the aggregator's
	// baselines) alive across epic-a's finish/relaunch — the scenario this
	// test is about.
	keepalive := costEpic("epic-keepalive", nil)
	costAgg.tick([]epicCostSnapshot{keepalive, costEpic("epic-a", nil)}) // baseline == 4.0
	costAgg.tick([]epicCostSnapshot{keepalive})                          // epic-a finished

	landed = 5.0 // epic finished this tick's iteration, landed cost rose $1

	costAgg.tick([]epicCostSnapshot{keepalive, costEpic("epic-a", nil)}) // relaunched

	want := 5.0 - 4.0 // baseline stayed at the original 4.0, not reset to 5.0
	if got := LiveSpendByEpic()["epic-a"]; got != want {
		t.Fatalf("LiveSpendByEpic()[epic-a] after relaunch = %v, want %v (original baseline retained)", got, want)
	}
}

func TestCostAggregatorDoesNotDoubleCountLandedTicketAsInFlight(t *testing.T) {
	resetCostAgg(t)
	inFlightCalls := 0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 3.0, map[string]float64{"01": 3.0}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) {
			inFlightCalls++
			return 99.0, true, nil
		},
	)

	costAgg.tick([]epicCostSnapshot{costEpic("epic-a", map[string]costTicketSnapshot{
		"01": runningTicket("/repo", "sess-1", ralphloop.AgentClaude),
	})})

	if inFlightCalls != 0 {
		t.Fatalf("sessionCostFn called %d times for a ticket whose landed cost is already nonzero, want 0", inFlightCalls)
	}
}

func TestCostAggregatorExcludesCodexFromTotalAndCountsUnpriced(t *testing.T) {
	resetCostAgg(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) {
			t.Fatal("sessionCostFn should never be called for a Codex ticket")
			return 0, false, nil
		},
	)

	costAgg.tick([]epicCostSnapshot{costEpic("epic-a", map[string]costTicketSnapshot{
		"01": runningTicket("/repo", "sess-1", ralphloop.AgentCodex),
	})})

	if got := LiveSpend(); got != 0 {
		t.Fatalf("LiveSpend() = %v with only a Codex iteration running, want 0", got)
	}
	if got := UnpricedRunningCount(); got != 1 {
		t.Fatalf("UnpricedRunningCount() = %d, want 1", got)
	}
}

func TestCostAggregatorClaudeIterationDoesNotCountAsUnpriced(t *testing.T) {
	resetCostAgg(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 1.0, true, nil },
	)

	costAgg.tick([]epicCostSnapshot{costEpic("epic-a", map[string]costTicketSnapshot{
		"01": runningTicket("/repo", "sess-1", ralphloop.AgentClaude),
	})})

	if got := UnpricedRunningCount(); got != 0 {
		t.Fatalf("UnpricedRunningCount() = %d for a running Claude iteration, want 0", got)
	}
}

func TestCostAggregatorMtimeUnchangedSkipsReparse(t *testing.T) {
	// not parallel-safe: writeFakeTranscript sets the process HOME env var
	resetCostAgg(t)
	calls := 0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) {
			calls++
			return 2.0, true, nil
		},
	)

	cwd := t.TempDir()
	sessionID := "sess-mtime"
	writeFakeTranscript(t, cwd, sessionID)

	snap := []epicCostSnapshot{costEpic("epic-a", map[string]costTicketSnapshot{
		"01": runningTicket(cwd, sessionID, ralphloop.AgentClaude),
	})}
	costAgg.tick(snap)
	costAgg.tick(snap)

	if calls != 1 {
		t.Fatalf("sessionCostFn called %d times across two ticks with an unchanged transcript mtime, want 1", calls)
	}
}

func TestCostAggregatorFailingReadContributesZeroWithoutAffectingOtherEpics(t *testing.T) {
	resetCostAgg(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			if epicName == "epic-fail" {
				return 0, nil, os.ErrNotExist
			}
			return 3.0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)

	costAgg.tick([]epicCostSnapshot{costEpic("epic-fail", nil), costEpic("epic-ok", nil)})

	if got := LiveSpendByEpic()["epic-fail"]; got != 0 {
		t.Fatalf("LiveSpendByEpic()[epic-fail] = %v after a failed load, want 0", got)
	}
	if got := LiveSpendByEpic()["epic-ok"]; got != 0 {
		t.Fatalf("LiveSpendByEpic()[epic-ok] = %v, want 0 (baselined on this same tick)", got)
	}
}

// writeFakeTranscript writes a minimal Claude Code transcript file at the
// path transcript.Path(cwd, sessionID) resolves to, so os.Stat inside
// costAggregator.transcriptCost can find a real mtime to guard on. Content
// doesn't matter here since sessionCostFn is stubbed.
func writeFakeTranscript(t *testing.T, cwd, sessionID string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
