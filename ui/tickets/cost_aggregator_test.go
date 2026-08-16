package tickets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// startTestRegistry installs a fresh registry as the package-level singleton
// (the seam every other loop_registry test uses) and returns it.
func startTestRegistry(t *testing.T) *loopRegistry {
	t.Helper()
	r := newLoopRegistry(4)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() { ralphLoopRegistry = previous })
	return r
}

func startRunningTicket(t *testing.T, r *loopRegistry, epicName, identifier, cwd, sessionID string, agent ralphloop.AgentKind) {
	t.Helper()
	r.reduceLiveEvent(epicName, ralphloop.LiveEvent{
		Kind:       ralphloop.LiveEventIterationStarted,
		Identifier: identifier,
		Label:      identifier,
		Cwd:        cwd,
		SessionID:  sessionID,
		AgentKind:  agent,
	})
}

func TestCostAggregatorTickBaselinesEpicOnFirstObservation(t *testing.T) {
	r := startTestRegistry(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 4.0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)

	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })

	costAgg.tick()

	if got := LiveSpend(); got != 0 {
		t.Fatalf("LiveSpend() = %v after baselining tick, want 0 (baseline == current landed cost)", got)
	}
	if got := LiveSpendByEpic()["epic-a"]; got != 0 {
		t.Fatalf("LiveSpendByEpic()[epic-a] = %v, want 0", got)
	}
}

func TestCostAggregatorTickSumsLandedSinceBaselinePlusInFlight(t *testing.T) {
	// not parallel-safe: writeFakeTranscript sets the process HOME env var
	r := startTestRegistry(t)
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

	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	startRunningTicket(t, r, "epic-a", "01", cwd, sessionID, ralphloop.AgentClaude)

	costAgg.tick() // baseline == 4.0

	landed = 6.0 // epic-wide landed cost rose by $2 since baseline
	costAgg.tick()

	want := (6.0 - 4.0) + 1.5
	if got := LiveSpend(); got != want {
		t.Fatalf("LiveSpend() = %v, want %v", got, want)
	}
	if got := LiveSpendByEpic()["epic-a"]; got != want {
		t.Fatalf("LiveSpendByEpic()[epic-a] = %v, want %v", got, want)
	}
}

func TestCostAggregatorRelaunchWithinSessionKeepsOriginalBaseline(t *testing.T) {
	r := startTestRegistry(t)
	landed := 4.0
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return landed, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)

	dir := t.TempDir()
	// A second epic holds the attach lock across epic-a's finish/relaunch so
	// the Attach session (and the aggregator's baselines) never resets — the
	// scenario this test is about.
	if _, ok := r.tryStart("epic-keepalive", 0, 1, dir); !ok {
		t.Fatal("tryStart(epic-keepalive): want success")
	}
	t.Cleanup(func() { r.finish("epic-keepalive", nil) })

	if _, ok := r.tryStart("epic-a", 0, 5, dir); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	costAgg.tick() // baseline == 4.0
	r.finish("epic-a", nil)

	landed = 5.0 // epic finished this tick's iteration, landed cost rose $1
	if _, ok := r.tryStart("epic-a", 0, 5, dir); !ok {
		t.Fatal("relaunch tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	costAgg.tick()

	want := 5.0 - 4.0 // baseline stayed at the original 4.0, not reset to 5.0
	if got := LiveSpendByEpic()["epic-a"]; got != want {
		t.Fatalf("LiveSpendByEpic()[epic-a] after relaunch = %v, want %v (original baseline retained)", got, want)
	}
}

func TestCostAggregatorDoesNotDoubleCountLandedTicketAsInFlight(t *testing.T) {
	r := startTestRegistry(t)
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

	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	startRunningTicket(t, r, "epic-a", "01", "/repo", "sess-1", ralphloop.AgentClaude)

	costAgg.tick()

	if inFlightCalls != 0 {
		t.Fatalf("sessionCostFn called %d times for a ticket whose landed cost is already nonzero, want 0", inFlightCalls)
	}
}

func TestCostAggregatorExcludesCodexFromTotalAndCountsUnpriced(t *testing.T) {
	r := startTestRegistry(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) {
			t.Fatal("sessionCostFn should never be called for a Codex ticket")
			return 0, false, nil
		},
	)

	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	startRunningTicket(t, r, "epic-a", "01", "/repo", "sess-1", ralphloop.AgentCodex)

	costAgg.tick()

	if got := LiveSpend(); got != 0 {
		t.Fatalf("LiveSpend() = %v with only a Codex iteration running, want 0", got)
	}
	if got := UnpricedRunningCount(); got != 1 {
		t.Fatalf("UnpricedRunningCount() = %d, want 1", got)
	}
}

func TestCostAggregatorClaudeIterationDoesNotCountAsUnpriced(t *testing.T) {
	r := startTestRegistry(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 1.0, true, nil },
	)

	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	startRunningTicket(t, r, "epic-a", "01", "/repo", "sess-1", ralphloop.AgentClaude)

	costAgg.tick()

	if got := UnpricedRunningCount(); got != 0 {
		t.Fatalf("UnpricedRunningCount() = %d for a running Claude iteration, want 0", got)
	}
}

func TestCostAggregatorMtimeUnchangedSkipsReparse(t *testing.T) {
	// not parallel-safe: writeFakeTranscript sets the process HOME env var
	r := startTestRegistry(t)
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

	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}
	t.Cleanup(func() { r.finish("epic-a", nil) })
	startRunningTicket(t, r, "epic-a", "01", cwd, sessionID, ralphloop.AgentClaude)

	costAgg.tick()
	costAgg.tick()

	if calls != 1 {
		t.Fatalf("sessionCostFn called %d times across two ticks with an unchanged transcript mtime, want 1", calls)
	}
}

func TestCostAggregatorFailingReadContributesZeroWithoutAffectingOtherEpics(t *testing.T) {
	r := startTestRegistry(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			if epicName == "epic-fail" {
				return 0, nil, os.ErrNotExist
			}
			return 3.0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)

	if _, ok := r.tryStart("epic-fail", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-fail): want success")
	}
	t.Cleanup(func() { r.finish("epic-fail", nil) })
	if _, ok := r.tryStart("epic-ok", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-ok): want success")
	}
	t.Cleanup(func() { r.finish("epic-ok", nil) })

	costAgg.tick()

	if got := LiveSpendByEpic()["epic-fail"]; got != 0 {
		t.Fatalf("LiveSpendByEpic()[epic-fail] = %v after a failed load, want 0", got)
	}
	if got := LiveSpendByEpic()["epic-ok"]; got != 0 {
		t.Fatalf("LiveSpendByEpic()[epic-ok] = %v, want 0 (baselined on this same tick)", got)
	}
}

func TestCostAggregatorPollerStartsAndStopsWithAttachTransitions(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry
	r := startTestRegistry(t)
	withStubbedCostReads(t,
		func(scratchDir, epicName string) (float64, map[string]float64, error) {
			return 0, map[string]float64{}, nil
		},
		func(cwd, sessionID string) (float64, bool, error) { return 0, false, nil },
	)

	costAgg.mu.Lock()
	costAgg.tickInterval = time.Millisecond
	costAgg.mu.Unlock()

	if _, ok := r.tryStart("epic-a", 0, 5, t.TempDir()); !ok {
		t.Fatal("tryStart(epic-a): want success")
	}

	costAgg.mu.Lock()
	running := costAgg.running
	doneCh := costAgg.doneCh
	costAgg.mu.Unlock()
	if !running {
		t.Fatal("costAgg.running = false after the zero-to-one attach transition, want true")
	}

	r.finish("epic-a", nil)

	costAgg.mu.Lock()
	stillRunning := costAgg.running
	costAgg.mu.Unlock()
	if stillRunning {
		t.Fatal("costAgg.running = true after the one-to-zero attach transition, want false")
	}
	select {
	case <-doneCh:
	default:
		t.Fatal("poller goroutine's doneCh not closed after finish returned, want the goroutine to have exited")
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
