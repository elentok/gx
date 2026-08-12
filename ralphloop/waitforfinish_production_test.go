package ralphloop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/transcript"
	"go.uber.org/goleak"
)

// TestMain re-enters the test binary as the fake herdr helper when launched
// under herdrfake's coordinator (see herdrfake.RunHelperProcess), so this
// package's tests can put a hermetic fake `herdr` executable first in PATH.
// goleak.VerifyTestMain fails the specific leaking test at leak time (stack
// trace and all) instead of letting a stuck goroutine surface as an
// unrelated multi-minute package hang.
func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	goleak.VerifyTestMain(m)
}

// agentResult builds the {"agent": {...}} envelope herdr's runAgentJSON
// parses, wrapped by herdrfake.Result into the {"result": ...} shape the real
// CLI also produces.
func agentResult(pane, status string) (any, herdrfake.Identities, error) {
	return map[string]any{
		"agent": map[string]any{
			"pane_id":      pane,
			"agent_status": status,
		},
	}, herdrfake.Identities{PaneID: pane}, nil
}

// parseUntil extracts every "--until VALUE" pair from a herdr agent
// wait/prompt argv tail.
func parseUntil(args []string) []string {
	var until []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--until" && i+1 < len(args) {
			until = append(until, args[i+1])
			i++
		}
	}
	return until
}

// writeOccupancyTranscript writes a minimal single-assistant-turn Claude Code
// transcript at the real path transcript.Path resolves for cwd/sessionID
// (under $HOME, which the caller must have redirected via setHomeEnv), reading
// back occupancy inputTokens.
func writeOccupancyTranscript(t *testing.T, cwd, sessionID string, inputTokens int) {
	t.Helper()
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		t.Fatalf("transcript.Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	line := fmt.Sprintf(
		`{"type":"assistant","message":{"model":"claude","usage":{"input_tokens":%d,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"output_tokens":1}}}`+"\n",
		inputTokens,
	)
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatalf("WriteFile transcript: %v", err)
	}
}

// TestWaitForFinish_ProductionSlowCompactRegression reproduces the
// production bug (promptWithNudge overriding a caller's /compact timeout
// with its short nudge grace, then canceling an in-progress compaction with
// a stray Enter) through the real dependency wiring: DefaultDeps with only
// Sleep/Now swapped for deterministic ones, every herdr call (prompt, wait,
// send-keys, read) going out via the real herdr client functions to a fake
// `herdr` executable reached through PATH (testutil/herdrfake), backed by a
// deterministic State whose virtual clock is advanced explicitly instead of
// sleeping three real minutes. The pane's immediate idle report is corroborated
// by a compaction boundary appended to the transcript as "/compact" is
// submitted — a compaction that genuinely completed inside the three virtual
// minutes already advanced before waitForFinish is called — since an idle
// report the transcript never backs up is now held by the completion gate.
func TestWaitForFinish_ProductionSlowCompactRegression(t *testing.T) {
	const pane = "pane-1"
	const smartZone = 100
	cwd := "/repo/iter-05"
	sessionID := "sess-05"

	s := herdrfake.NewState(t)

	var mu sync.Mutex
	status := "working" // agent starts mid-turn, already over smartZone
	var compactCalls, ctrlCCalls, enterCalls int
	var promptOrder []string

	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target, text := argv[2], argv[3]
		mu.Lock()
		defer mu.Unlock()
		var respond string
		switch {
		case text == "/compact":
			compactCalls++
			promptOrder = append(promptOrder, "/compact")
			// The submit call observes the compaction starting; by the time
			// anything queries status again, the (virtual) compaction has
			// finished, so the completion wait finds it done — and the
			// transcript corroborates that idle report with a real compaction
			// boundary, which is what the completion gate requires before the
			// finish-up prompt may be sent.
			appendCompactBoundaryLine(t, cwd, sessionID)
			respond = "working"
			status = "idle"
		case strings.Contains(text, "please finish up quickly"):
			promptOrder = append(promptOrder, "finish-up")
			respond = "working"
			status = "done"
		default:
			respond = "working"
			status = "working"
		}
		return agentResult(target, respond)
	})

	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		until := parseUntil(argv[3:])
		mu.Lock()
		cur := status
		mu.Unlock()
		if len(until) == 0 || slices.Contains(until, cur) {
			return agentResult(target, cur)
		}
		return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
	})

	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		mu.Lock()
		for _, k := range argv[3:] {
			switch k {
			case "ctrl+c":
				ctrlCCalls++
			case "enter":
				enterCalls++
			}
		}
		cur := status
		mu.Unlock()
		return agentResult(target, cur)
	})

	s.Register("agent", "read", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})

	herdrfake.StartState(t, s)

	setHomeEnv(t, t.TempDir())
	writeOccupancyTranscript(t, cwd, sessionID, smartZone+100)

	// Models the whole scenario's ~3 real minutes of compaction without
	// sleeping for them.
	s.AdvanceVirtualTime(3 * time.Minute)

	scratchDir := t.TempDir()
	deps := testDeps()
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Unix(0, 0) }

	err := waitForFinish(deps, launchAndPromptParams{
		Label:      "iter-05",
		Agent:      AgentClaude,
		Pane:       pane,
		SessionCwd: cwd,
		SmartZone:  smartZone,
		Gate:       NewGate(),
		Ticket:     "05",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}, sessionID)
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}

	if compactCalls != 1 {
		t.Errorf("compact prompts = %d, want exactly 1", compactCalls)
	}
	if ctrlCCalls != 1 {
		t.Errorf("ctrl+c sends = %d, want exactly 1", ctrlCCalls)
	}
	if enterCalls != 0 {
		t.Errorf("enter nudges = %d, want 0 (a stray Enter cancels an in-progress compaction)", enterCalls)
	}
	wantOrder := []string{"/compact", "finish-up"}
	if !slices.Equal(promptOrder, wantOrder) {
		t.Errorf("promptOrder = %v, want %v (finish-up must follow compact completion)", promptOrder, wantOrder)
	}
	if got := s.VirtualTime(); got < 3*time.Minute {
		t.Errorf("VirtualTime() = %s, want >= 3m (scenario must advance virtual time, not sleep)", got)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents() ok=%v err=%v", ok, err)
	}
	var sawResumed, sawFailed bool
	for _, e := range events {
		switch e.Type {
		case eventResumed:
			sawResumed = true
		case eventSmartZoneRecoveryFailed:
			sawFailed = true
		}
	}
	if !sawResumed {
		t.Error("missing resumed event after smart-zone recovery")
	}
	if sawFailed {
		t.Error("smart-zone-recovery-failed event emitted, want none")
	}

	// No send-keys (Enter or another Ctrl-C) dispatched once compaction is
	// known to be running, i.e. after the /compact submit call.
	var compactionRunning bool
	for _, e := range s.Trace() {
		if len(e.Argv) > 3 && e.Argv[0] == "agent" && e.Argv[1] == "prompt" && e.Argv[3] == "/compact" {
			compactionRunning = true
			continue
		}
		if compactionRunning && len(e.Argv) >= 2 && e.Argv[0] == "agent" && e.Argv[1] == "send-keys" {
			t.Errorf("send-keys dispatched after compaction started: %v", e.Argv)
		}
	}
}

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
// eventually land: ClaudeCompact writes it lazily inside Status once virtual
// time passes its compaction duration, and herdrfake's clock never moves on
// its own. A scenario that left the clock still would never reach the boundary
// and would instead exercise the gated give-up path, so the virtual-time
// assertion below is load-bearing, not decorative.
func TestWaitForFinish_ProductionPrematureIdlePaneRecovery(t *testing.T) {
	const pane = "pane-1"
	const smartZone = 100
	cwd := "/repo/iter-07"
	sessionID := "sess-07"

	setHomeEnv(t, t.TempDir())

	s := herdrfake.NewState(t)

	var mu sync.Mutex
	// The scenario's own virtual clock rather than State's: State.dispatch holds
	// State's mutex for the whole handler, so reading or advancing its clock from
	// inside one deadlocks.
	var virtualTime time.Duration
	compact := herdrfake.NewClaudeCompact(t, cwd, sessionID, func() time.Duration {
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
			// A compact-completion poll (see compactStates in
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
		mu.Unlock()
		// Before the finish-up prompt lands the agent is mid-turn, so every
		// ordinary finish poll times out — that's the tick the smart-zone
		// breach is detected on.
		if done && (len(until) == 0 || slices.Contains(until, "done")) {
			return agentResult(target, "done")
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
		return agentResult(target, "working")
	})

	s.Register("agent", "read", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})

	herdrfake.StartState(t, s)

	scratchDir := t.TempDir()
	deps := testDeps()
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Unix(0, 0) }

	err := waitForFinish(deps, launchAndPromptParams{
		Label:      "iter-07",
		Agent:      AgentClaude,
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
	if ctrlCCalls != 1 {
		t.Errorf("ctrl+c sends = %d, want exactly 1", ctrlCCalls)
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
		t.Errorf("VirtualTime() = %s, want >= %s (the compact polls must advance the clock far enough for the boundary to land)", got, want)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents() ok=%v err=%v", ok, err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Type] = true
	}
	if !seen[eventResumed] {
		t.Error("missing resumed event after smart-zone recovery")
	}
	if seen[eventSmartZoneRecoveryFailed] {
		t.Error("smart-zone-recovery-failed event emitted, want none")
	}
	if !seen[eventSmartZoneGateReleased] {
		t.Errorf("missing %s event — a premature-idle pane's completion must be attributed to the gate holding until the boundary landed", eventSmartZoneGateReleased)
	}
	if seen[eventSmartZoneWaitExpired] {
		t.Errorf("%s logged for a gated completion, want it reserved for the pane-timeout route", eventSmartZoneWaitExpired)
	}
}

// TestWaitForFinish_ProductionPrematureIdlePaneNeverConfirms is the sibling
// scenario to the recovery test above, differing in one variable: virtual time
// never advances, so ClaudeCompact's compaction never reaches its deadline and
// never writes a boundary. Both of the pane's poll kinds consult the same
// Status() — under WithPrematureIdlePane that is "idle" throughout — which is
// the production shape that made the pane-status finish path fire while
// "/compact" was still running: the iteration was declared finished, the ticket
// closed needs-answer with no commit, and the worktree abandoned mid-compaction.
// The run must instead end at errCompactRecoveryExhausted, which loop.go
// persists as needs-repair for an operator.
func TestWaitForFinish_ProductionPrematureIdlePaneNeverConfirms(t *testing.T) {
	const pane = "pane-1"
	const smartZone = 100
	cwd := "/repo/iter-08"
	sessionID := "sess-08"

	setHomeEnv(t, t.TempDir())

	s := herdrfake.NewState(t)

	var mu sync.Mutex
	compact := herdrfake.NewClaudeCompact(t, cwd, sessionID, func() time.Duration {
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
		mu.Unlock()
		// Before "/compact" the agent is mid-turn, so every poll times out —
		// that's the tick the smart-zone breach is detected on. Afterwards both
		// poll kinds get the same answer the real pane gave: idle.
		if !started {
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
		return agentResult(target, "working")
	})

	s.Register("agent", "read", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})

	herdrfake.StartState(t, s)

	scratchDir := t.TempDir()
	deps := testDeps()
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Unix(0, 0) }

	err := waitForFinish(deps, launchAndPromptParams{
		Label:      "iter-08",
		Agent:      AgentClaude,
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
		t.Errorf("finish-up prompts = %d, want 0 (it would land as input to a still-running compaction)", finishUpCalls)
	}
	if ctrlCCalls != 1 {
		t.Errorf("ctrl+c sends = %d, want exactly 1", ctrlCCalls)
	}
	if enterCalls != 0 {
		t.Errorf("enter nudges = %d, want 0 (a stray Enter cancels an in-progress compaction)", enterCalls)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents() ok=%v err=%v", ok, err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Type] = true
	}
	if !seen[eventSmartZoneRecoveryFailed] {
		t.Errorf("missing %s event for a compaction that never confirmed", eventSmartZoneRecoveryFailed)
	}
	if seen[eventResumed] {
		t.Error("resumed event emitted, want none: nothing recovered")
	}
}

// compactBoundaryConfirmTick is the "agent wait" dispatch count (counting
// only compact-completion polls, i.e. those whose --until includes
// "blocked" — see waitforfinish.go's compactStates) at which this test's
// fake transport finally writes the transcript's compaction-boundary line.
// Each such dispatch models one smartZonePollMs (30s) tick, and
// waitForCompactionSignal's loop is entered with startElapsedMs already at
// one tick (see recoverSmartZoneBreach), so the Nth dispatch corresponds to
// N*smartZonePollMs of elapsed virtual time: 16 ticks lands completion at
// 480s (8 minutes) — comfortably past smartZoneCompactTimeoutMs (5 minutes,
// tick 10) where the transcript check first engages, and comfortably short
// of smartZoneCompactExtendedTimeoutMs (10 minutes, tick 20) where an
// unconfirmed compact would be reported as a genuine failure.
const compactBoundaryConfirmTick = 16

// appendCompactBoundaryLine appends a minimal Claude Code compact_boundary
// system line to the transcript at cwd/sessionID, indistinguishable from a
// real compaction's completion marker to transcript.Compactions.
func appendCompactBoundaryLine(t *testing.T, cwd, sessionID string) {
	t.Helper()
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		t.Fatalf("transcript.Path: %v", err)
	}
	line := map[string]any{
		"type":            "system",
		"subtype":         "compact_boundary",
		"timestamp":       time.Unix(0, 0).UTC().Format(time.RFC3339Nano),
		"compactMetadata": map[string]any{"trigger": "manual"},
	}
	b, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("marshal compact boundary line: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open transcript for append: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		t.Fatalf("append compact boundary line: %v", err)
	}
}

// TestWaitForFinish_ProductionSlowButSuccessfulCompactRegression proves
// ticket 19's fix: a "/compact" that genuinely takes longer than
// smartZoneCompactTimeoutMs (5 minutes) — here, 8 minutes — but eventually
// completes must not be misreported as a failed recovery. It's a sibling of
// TestWaitForFinish_ProductionSlowCompactRegression above, sharing the same
// fake-transport/virtual-time pattern through real dependency wiring
// (DefaultDeps with only Sleep/Now swapped), but where that test's fake
// "/compact" resolves the completion wait synchronously on submission (the
// pane flips to "idle" the instant "/compact" is typed, so the real herdr
// wait call it makes never actually observes a pending state), this test's
// pane deliberately never reports the compact as finished — a herdr
// pane-status observation gap, per waitForCompactionSignal's doc comment —
// and only the transcript's compaction-boundary line, appended after
// compactBoundaryConfirmTick polls, ever confirms completion. Against
// pre-ticket-19 code (no transcript fallback), the pane-status wait alone
// would time out at the 5-minute mark and this scenario would be reported as
// a failed recovery; against the fix, it's confirmed successful instead.
func TestWaitForFinish_ProductionSlowButSuccessfulCompactRegression(t *testing.T) {
	const pane = "pane-1"
	const smartZone = 100
	cwd := "/repo/iter-06"
	sessionID := "sess-06"

	s := herdrfake.NewState(t)

	var mu sync.Mutex
	status := "working" // agent starts mid-turn, already over smartZone
	var compactCalls, ctrlCCalls, enterCalls, compactWaitTicks int
	var boundaryWritten, compacting bool
	var promptOrder []string

	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target, text := argv[2], argv[3]
		mu.Lock()
		defer mu.Unlock()
		var respond string
		switch {
		case text == "/compact":
			compactCalls++
			promptOrder = append(promptOrder, "/compact")
			// Deliberately does NOT flip status to "idle": the pane never
			// confirms the compact finishing, so only the transcript can.
			compacting = true
			respond = "working"
		case strings.Contains(text, "please finish up quickly"):
			promptOrder = append(promptOrder, "finish-up")
			respond = "working"
			status = "done"
		default:
			respond = "working"
			status = "working"
		}
		return agentResult(target, respond)
	})

	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		until := parseUntil(argv[3:])
		mu.Lock()
		defer mu.Unlock()
		if compacting {
			// A compact-completion poll (see compactStates in
			// waitforfinish.go): the pane never reports completion, so
			// every one of these times out. Once enough ticks have passed,
			// the transcript is updated as a side effect, still without
			// ever making this call itself succeed. Gated on compacting
			// rather than Until's shape — once "blocked" joined every finish
			// poll's completion states (ticket 14), Until alone no longer
			// told a compact-completion poll apart from the ordinary finish
			// poll below.
			compactWaitTicks++
			if compactWaitTicks >= compactBoundaryConfirmTick && !boundaryWritten {
				boundaryWritten = true
				compacting = false
				mu.Unlock()
				appendCompactBoundaryLine(t, cwd, sessionID)
				mu.Lock()
			}
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		}
		cur := status
		if len(until) == 0 || slices.Contains(until, cur) {
			return agentResult(target, cur)
		}
		return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
	})

	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		target := argv[2]
		mu.Lock()
		for _, k := range argv[3:] {
			switch k {
			case "ctrl+c":
				ctrlCCalls++
			case "enter":
				enterCalls++
			}
		}
		cur := status
		mu.Unlock()
		return agentResult(target, cur)
	})

	s.Register("agent", "read", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})

	herdrfake.StartState(t, s)

	setHomeEnv(t, t.TempDir())
	writeOccupancyTranscript(t, cwd, sessionID, smartZone+100)

	// Models the whole scenario's ~8 real minutes of compaction without
	// sleeping for them.
	s.AdvanceVirtualTime(8 * time.Minute)

	scratchDir := t.TempDir()
	deps := testDeps()
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Unix(0, 0) }

	err := waitForFinish(deps, launchAndPromptParams{
		Label:      "iter-06",
		Agent:      AgentClaude,
		Pane:       pane,
		SessionCwd: cwd,
		SmartZone:  smartZone,
		Gate:       NewGate(),
		Ticket:     "06",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}, sessionID)
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}

	if compactCalls != 1 {
		t.Errorf("compact prompts = %d, want exactly 1", compactCalls)
	}
	if ctrlCCalls != 1 {
		t.Errorf("ctrl+c sends = %d, want exactly 1", ctrlCCalls)
	}
	if enterCalls != 0 {
		t.Errorf("enter nudges = %d, want 0 (a stray Enter cancels an in-progress compaction)", enterCalls)
	}
	wantOrder := []string{"/compact", "finish-up"}
	if !slices.Equal(promptOrder, wantOrder) {
		t.Errorf("promptOrder = %v, want %v (finish-up must follow compact completion)", promptOrder, wantOrder)
	}
	if got := s.VirtualTime(); got < 8*time.Minute {
		t.Errorf("VirtualTime() = %s, want >= 8m (scenario must advance virtual time, not sleep)", got)
	}
	if !boundaryWritten {
		t.Fatal("test bug: compact boundary line was never written")
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents() ok=%v err=%v", ok, err)
	}
	var sawResumed, sawFailed, sawWaitExpired bool
	for _, e := range events {
		switch e.Type {
		case eventResumed:
			sawResumed = true
		case eventSmartZoneRecoveryFailed:
			sawFailed = true
		case eventSmartZoneWaitExpired:
			sawWaitExpired = true
		}
	}
	if !sawResumed {
		t.Error("missing resumed event after smart-zone recovery")
	}
	if sawFailed {
		t.Error("smart-zone-recovery-failed event emitted, want none — a merely slow but successful compact must not be misreported as a failed recovery")
	}
	if !sawWaitExpired {
		t.Error("missing smart-zone-wait-expired event — the transcript's compaction-boundary signal, not the pane status, should have confirmed this compact")
	}

	// No send-keys (Enter or another Ctrl-C) dispatched once compaction is
	// known to be running, i.e. after the /compact submit call.
	var compactionRunning bool
	for _, e := range s.Trace() {
		if len(e.Argv) > 3 && e.Argv[0] == "agent" && e.Argv[1] == "prompt" && e.Argv[3] == "/compact" {
			compactionRunning = true
			continue
		}
		if compactionRunning && len(e.Argv) >= 2 && e.Argv[0] == "agent" && e.Argv[1] == "send-keys" {
			t.Errorf("send-keys dispatched after compaction started: %v", e.Argv)
		}
	}
}
