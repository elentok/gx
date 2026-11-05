package ralphloop

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/testutil/herdrfake"
)

// TestWaitForFinish_ProductionPrematureIdlePaneRecovery drives a smart-zone
// breach against herdrfake.ClaudeCompact's premature-idle pane (the iter-13c
// shape: the pane reports "idle" from the instant "/compact" is submitted,
// while the compaction is still running and its boundary is not yet written)
// through the same real dependency wiring as its siblings above. It is the
// production-level counterpart of the unit coverage for the completion gate:
// before the gate existed, the pane's immediate idle report was believed, the
// finish-up prompt went out mid-compaction, and the fake's AcceptFinishUp
// guard — which refuses a finish-up prompt until the compact_boundary line
// exists — fails the run.
//
// The compact-completion poll handler advances virtual time by one
// smartZonePollMs tick per dispatch, which is what makes the boundary
// eventually merge: ClaudeCompact writes it lazily inside Status once virtual
// time passes its compaction duration, and herdrfake's clock never moves on
// its own. A scenario that left the clock still would never reach the boundary
// and would instead exercise the gated give-up path, so the virtual-time
// assertion below is load-bearing, not decorative.
func TestWaitForFinish_ProductionPrematureIdlePaneRecovery(t *testing.T) {
	// not parallel-safe: setHomeEnv mutates the process-wide $HOME env var.
	const pane = "pane-1"
	const smartZone = 100
	cwd := "/repo/iter-07"
	sessionID := "sess-07"

	home := t.TempDir()
	fixture.SetHomeEnv(t, home)

	s := herdrfake.NewState(t)

	var mu sync.Mutex
	// The scenario's own virtual clock rather than State's: State.dispatch holds
	// State's mutex for the whole handler, so reading or advancing its clock from
	// inside one deadlocks.
	var virtualTime time.Duration
	compact := herdrfake.NewClaudeCompact(t, home, cwd, sessionID, func() time.Duration {
		mu.Lock()
		defer mu.Unlock()
		return virtualTime
	}, smartZone, herdrfake.WithPrematureIdlePane())

	var compactCalls, finishUpCalls, ctrlCCalls, enterCalls int
	var finishedUp bool
	var promptOrder []string

	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target, text := argv[2], argv[3]
		switch {
		case text == "/compact":
			mu.Lock()
			compactCalls++
			promptOrder = append(promptOrder, "/compact")
			mu.Unlock()
			if err := compact.StartCompact(); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			status, err := compact.Status()
			if err != nil {
				return nil, herdrfake.Identities{}, err
			}
			return agentResult(target, status)
		case strings.Contains(text, "please finish up quickly"):
			mu.Lock()
			finishUpCalls++
			promptOrder = append(promptOrder, "finish-up")
			mu.Unlock()
			if err := compact.AcceptFinishUp(); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			mu.Lock()
			finishedUp = true
			mu.Unlock()
			return agentResult(target, "working")
		}
		return agentResult(target, "working")
	})

	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		until := parseUntil(argv[3:])
		if compact.Active() {
			// A compact-completion poll (see waitForCompactionSignal in
			// waitforfinish.go): each dispatch models one smartZonePollMs tick
			// of the compaction actually running. Once "blocked" joined every
			// finish poll's completion states (ticket 14), Until's shape alone
			// no longer told this apart from the ordinary finish poll below —
			// compact.Active() does, since only a compaction actually in
			// flight should advance the virtual clock.
			mu.Lock()
			virtualTime += smartZonePollMs * time.Millisecond
			mu.Unlock()
			status, err := compact.Status()
			if err != nil {
				return nil, herdrfake.Identities{}, err
			}
			if slices.Contains(until, status) {
				return agentResult(target, status)
			}
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		}
		mu.Lock()
		done := finishedUp
		keyed := ctrlCCalls > 0
		mu.Unlock()
		// Before the finish-up prompt merges the agent is mid-turn, so every
		// ordinary finish poll times out — that's the tick the smart-zone
		// breach is detected on.
		if done && (len(until) == 0 || slices.Contains(until, "done")) {
			return agentResult(target, "done")
		}
		// The interrupt's own wait for the pane to go idle.
		if keyed {
			status, err := compact.Status()
			if err != nil {
				return nil, herdrfake.Identities{}, err
			}
			if slices.Contains(until, status) {
				return agentResult(target, status)
			}
		}
		return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
	})

	// Interrupt reads the state to pick its key: the pane is mid-turn until
	// the interrupt merges.
	s.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		keyed := ctrlCCalls > 0
		mu.Unlock()
		if !keyed {
			return agentResult(argv[2], "working")
		}
		status, err := compact.Status()
		if err != nil {
			return nil, herdrfake.Identities{}, err
		}
		return agentResult(argv[2], status)
	})

	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		for _, k := range argv[3:] {
			mu.Lock()
			switch k {
			case "ctrl+c":
				ctrlCCalls++
			case "enter":
				enterCalls++
			}
			mu.Unlock()
			if err := compact.SendKey(k); err != nil {
				return nil, herdrfake.Identities{}, err
			}
		}
		// The pane's own status after the keys: an interrupt leaves it idle.
		status, err := compact.Status()
		if err != nil {
			return nil, herdrfake.Identities{}, err
		}
		return agentResult(target, status)
	})

	s.Register("agent", "read", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})

	hf, fakeEnv := herdrfake.StartStateEnv(t, s)

	scratchDir := fixture.EpicScratchDir(t, "epic")
	deps := testDepsWithOverrides(DepsOverrides{Env: withProcessEnv(fakeEnv)})
	useFakeClock(deps, hf)
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Unix(0, 0) }

	err := waitForFinish(deps, launchAndPromptParams{
		Label:      "iter-07",
		Agent:      agent.AgentClaude,
		Pane:       pane,
		SessionCwd: cwd,
		SmartZone:  smartZone,
		Gate:       NewGate(),
		Ticket:     "07",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}, sessionID)
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}

	if compactCalls != 1 {
		t.Errorf("compact prompts = %d, want exactly 1", compactCalls)
	}
	if finishUpCalls != 1 {
		t.Errorf("finish-up prompts = %d, want exactly 1 (the run must reach a normal finish, not merely withhold the prompt)", finishUpCalls)
	}
	if ctrlCCalls != 2 {
		t.Errorf("ctrl+c sends = %d, want exactly 2 (interrupt, then clear the restored prompt)", ctrlCCalls)
	}
	if enterCalls != 0 {
		t.Errorf("enter nudges = %d, want 0 (a stray Enter cancels an in-progress compaction)", enterCalls)
	}
	wantOrder := []string{"/compact", "finish-up"}
	if !slices.Equal(promptOrder, wantOrder) {
		t.Errorf("promptOrder = %v, want %v (finish-up must follow compact completion)", promptOrder, wantOrder)
	}
	mu.Lock()
	elapsedVirtual := virtualTime
	mu.Unlock()
	if got, want := elapsedVirtual, herdrfake.CompactDurationMs*time.Millisecond; got < want {
		t.Errorf("VirtualTime() = %s, want >= %s (the compact polls must advance the clock far enough for the boundary to merge)", got, want)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Type] = true
	}
	if !seen[string(eventsc.Resumed)] {
		t.Error("missing resumed event after smart-zone recovery")
	}
	if seen[string(eventsc.SmartZoneRecoveryFailed)] {
		t.Error("smart-zone-recovery-failed event emitted, want none")
	}
	if !seen[string(eventsc.SmartZoneGateReleased)] {
		t.Errorf("missing %s event — a premature-idle pane's completion must be attributed to the gate holding until the boundary merged", string(eventsc.SmartZoneGateReleased))
	}
	if seen[string(eventsc.SmartZoneWaitExpired)] {
		t.Errorf("%s logged for a gated completion, want it reserved for the pane-timeout route", string(eventsc.SmartZoneWaitExpired))
	}
}

// TestWaitForFinish_ProductionPrematureIdlePaneNeverConfirms is the sibling
// scenario to the recovery test above, differing in one variable: virtual time
// never advances, so ClaudeCompact's compaction never reaches its deadline and
// never writes a boundary. Both of the pane's poll kinds consult the same
// Status() — under WithPrematureIdlePane that is "idle" throughout — which is
// the production shape that made the pane-status finish path fire while
// "/compact" was still running: the iteration was declared finished, the ticket
// closed paused (answer) with no commit, and the worktree abandoned mid-compaction.
// The run must instead end at errCompactRecoveryExhausted, which loop.go
// persists as paused (repair) for an operator.
func TestWaitForFinish_ProductionPrematureIdlePaneNeverConfirms(t *testing.T) {
	// not parallel-safe: setHomeEnv mutates the process-wide $HOME env var.
	const pane = "pane-1"
	const smartZone = 100
	cwd := "/repo/iter-08"
	sessionID := "sess-08"

	home := t.TempDir()
	fixture.SetHomeEnv(t, home)

	s := herdrfake.NewState(t)

	var mu sync.Mutex
	compact := herdrfake.NewClaudeCompact(t, home, cwd, sessionID, func() time.Duration {
		// Frozen: this scenario's compaction never completes, so no poll of
		// either kind may ever move the clock past CompactDurationMs.
		return 0
	}, smartZone, herdrfake.WithPrematureIdlePane())

	var compactCalls, finishUpCalls, ctrlCCalls, enterCalls int
	var compactStarted bool

	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target, text := argv[2], argv[3]
		switch {
		case text == "/compact":
			mu.Lock()
			compactCalls++
			compactStarted = true
			mu.Unlock()
			if err := compact.StartCompact(); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			status, err := compact.Status()
			if err != nil {
				return nil, herdrfake.Identities{}, err
			}
			return agentResult(target, status)
		case strings.Contains(text, "please finish up quickly"):
			mu.Lock()
			finishUpCalls++
			mu.Unlock()
			if err := compact.AcceptFinishUp(); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			return agentResult(target, "working")
		}
		return agentResult(target, "working")
	})

	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		until := parseUntil(argv[3:])
		mu.Lock()
		started := compactStarted
		keyed := ctrlCCalls > 0
		mu.Unlock()
		// Before "/compact" the agent is mid-turn, so every poll times out —
		// that's the tick the smart-zone breach is detected on (the interrupt's
		// own wait for idle is the one that gets an answer). Afterwards both
		// poll kinds get the same answer the real pane gave: idle.
		if !started && !keyed {
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		}
		status, err := compact.Status()
		if err != nil {
			return nil, herdrfake.Identities{}, err
		}
		if len(until) == 0 || slices.Contains(until, status) {
			return agentResult(target, status)
		}
		return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
	})

	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		for _, k := range argv[3:] {
			mu.Lock()
			switch k {
			case "ctrl+c":
				ctrlCCalls++
			case "enter":
				enterCalls++
			}
			mu.Unlock()
			if err := compact.SendKey(k); err != nil {
				return nil, herdrfake.Identities{}, err
			}
		}
		// The pane's own status after the keys: an interrupt leaves it idle.
		status, err := compact.Status()
		if err != nil {
			return nil, herdrfake.Identities{}, err
		}
		return agentResult(target, status)
	})

	// Interrupt reads the state to pick its key: the pane is mid-turn until
	// the interrupt merges.
	s.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		keyed := ctrlCCalls > 0
		mu.Unlock()
		if !keyed {
			return agentResult(argv[2], "working")
		}
		status, err := compact.Status()
		if err != nil {
			return nil, herdrfake.Identities{}, err
		}
		return agentResult(argv[2], status)
	})

	s.Register("agent", "read", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})

	hf, fakeEnv := herdrfake.StartStateEnv(t, s)

	scratchDir := fixture.EpicScratchDir(t, "epic")
	deps := testDepsWithOverrides(DepsOverrides{Env: withProcessEnv(fakeEnv)})
	useFakeClock(deps, hf)
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Unix(0, 0) }

	err := waitForFinish(deps, launchAndPromptParams{
		Label:      "iter-08",
		Agent:      agent.AgentClaude,
		Pane:       pane,
		SessionCwd: cwd,
		SmartZone:  smartZone,
		Gate:       NewGate(),
		Ticket:     "08",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}, sessionID)
	if !errors.Is(err, errCompactRecoveryExhausted) {
		t.Fatalf("waitForFinish error = %v, want one wrapping errCompactRecoveryExhausted: an idle pane whose transcript never records a boundary is not a finish", err)
	}
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Errorf("waitForFinish error = %v, want the underlying gated give-up preserved for the operator", err)
	}

	if compactCalls != 1 {
		t.Errorf("compact prompts = %d, want exactly 1", compactCalls)
	}
	if finishUpCalls != 0 {
		t.Errorf("finish-up prompts = %d, want 0 (it would merge as input to a still-running compaction)", finishUpCalls)
	}
	if ctrlCCalls != 2 {
		t.Errorf("ctrl+c sends = %d, want exactly 2 (interrupt, then clear the restored prompt)", ctrlCCalls)
	}
	if enterCalls != 0 {
		t.Errorf("enter nudges = %d, want 0 (a stray Enter cancels an in-progress compaction)", enterCalls)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents() ok=%v err=%v", ok, err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Type] = true
	}
	if !seen[string(eventsc.SmartZoneRecoveryFailed)] {
		t.Errorf("missing %s event for a compaction that never confirmed", string(eventsc.SmartZoneRecoveryFailed))
	}
	if seen[string(eventsc.Resumed)] {
		t.Error("resumed event emitted, want none: nothing recovered")
	}
}
