package ralphloop

import (
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
)

// TestMain re-enters the test binary as the fake herdr helper when launched
// under herdrfake's coordinator (see herdrfake.RunHelperProcess), so this
// package's tests can put a hermetic fake `herdr` executable first in PATH.
func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
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
// (under $HOME, which the caller must have redirected via t.Setenv), reading
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
// sleeping three real minutes.
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
			// finished, so the completion wait finds it done.
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

	t.Setenv("HOME", t.TempDir())
	writeOccupancyTranscript(t, cwd, sessionID, smartZone+100)

	// Models the whole scenario's ~3 real minutes of compaction without
	// sleeping for them.
	s.AdvanceVirtualTime(3 * time.Minute)

	scratchDir := t.TempDir()
	deps := DefaultDeps()
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
