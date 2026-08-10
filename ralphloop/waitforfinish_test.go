package ralphloop

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

func TestWaitForFinish_CodexNativeContextFailureRecoversDespiteStaleOccupancy(t *testing.T) {
	const failure = "■ stream disconnected before completion: Your input exceeds the context window of this model. Please adjust your input and try again."
	scratchDir := t.TempDir()
	var waits, paneReads, interruptions int
	var prompts []string
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 1 {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(string, ...string) error {
			interruptions++
			return nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			if opts.Text == "/compact" {
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		ReadPaneRecent: func(string) (string, error) {
			paneReads++
			if paneReads == 1 {
				return failure, nil
			}
			return "", nil
		},
		ReadCodexContext: func(string, string) (int, bool, error) {
			return 1_000, true, nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
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
	if interruptions != 1 {
		t.Errorf("pane interruptions = %d, want 1", interruptions)
	}
	if len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want compact then finish-up", prompts)
	}
	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok || len(events) == 0 {
		t.Fatalf("readEvents() = %+v, ok=%v, err=%v", events, ok, err)
	}
	if events[0].Type != eventPausedSmartZone || !strings.Contains(events[0].Reason, "input exceeds the context window") {
		t.Errorf("recovery event = %+v, want native failure evidence", events[0])
	}
}

func TestWaitForFinish_CodexNativeContextFailureDetectedWhenSettled(t *testing.T) {
	var paneReads, interruptions int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(string, ...string) error {
			interruptions++
			return nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			if opts.Text == "/compact" {
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		ReadPaneRecent: func(string) (string, error) {
			paneReads++
			if paneReads == 1 {
				return `Error running remote compact task: {"error":{"code":"context_length_exceeded"}}`, nil
			}
			return "", nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-20", Agent: AgentCodex, Pane: "pane-1", Ticket: "20",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-20")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if interruptions != 1 {
		t.Errorf("pane interruptions = %d, want 1 before accepting settled state", interruptions)
	}
}

func TestWaitForFinish_CodexNativeContextFailureRecoveryFailureIsDurable(t *testing.T) {
	const failure = "■ Codex ran out of room in the model's context window."
	var paneReads, interruptions int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(string, ...string) error {
			interruptions++
			return nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			if opts.Text == "/compact" {
				return herdr.Agent{}, errors.New("compact never landed")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		ReadPaneRecent: func(string) (string, error) {
			paneReads++
			if paneReads == 1 {
				return failure, nil
			}
			return "", nil
		},
		Sleep: func(time.Duration) {},
	}

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
	if interruptions != 1 {
		t.Errorf("pane interruptions = %d, want 1", interruptions)
	}
}

func TestWaitForFinish_CodexNativeContextFailureFailsDurablyWithoutFreshTokenEvent(t *testing.T) {
	// No ReadCodexContext dependency at all: the classification and its
	// recovery-failure path must not depend on a fresh high-token record
	// existing to fire — the native exhaustion text is itself the evidence.
	const failure = `Error running remote compact task: {"error":{"code":"context_length_exceeded"}}`
	var paneReads int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(string, ...string) error { return nil },
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			if opts.Text == "/compact" {
				return herdr.Agent{}, errors.New("compact never landed")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		ReadPaneRecent: func(string) (string, error) {
			paneReads++
			if paneReads == 1 {
				return failure, nil
			}
			return "", nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-21", Agent: AgentCodex, Pane: "pane-1", Ticket: "21",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-21")
	if err == nil || !strings.Contains(err.Error(), "recovery failed") {
		t.Fatalf("waitForFinish() = %v, want a durable recovery-failed error without any ReadCodexContext dependency", err)
	}
}

func TestWaitForFinish_CodexContextDiscussionDoesNotTriggerRecovery(t *testing.T) {
	for _, text := range []string{
		"Error: request failed with status 500",
		`I am adding detection for "Your input exceeds the context window of this model."`,
		"The response included context_length_exceeded, which we should classify.",
	} {
		t.Run(text, func(t *testing.T) {
			var paneReads, interruptions int
			d := Deps{
				AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
					return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
				},
				AgentSendKeys: func(string, ...string) error {
					interruptions++
					return nil
				},
				AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
					if opts.Text == "/compact" {
						return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
					}
					return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
				},
				ReadPaneRecent: func(string) (string, error) {
					paneReads++
					if paneReads == 1 {
						return text, nil
					}
					return "", nil
				},
				Sleep: func(time.Duration) {},
			}

			err := waitForFinish(d, launchAndPromptParams{
				Label: "iter-20", Agent: AgentCodex, Pane: "pane-1", Ticket: "20",
				SmartZone: 150_000, Gate: NewGate(),
			}, "codex-session-20")
			if err != nil {
				t.Fatalf("waitForFinish: %v", err)
			}
			if interruptions != 0 {
				t.Errorf("pane interruptions = %d, want no recovery for discussion/error text", interruptions)
			}
		})
	}
}

func TestWaitForFinish_CodexContextBreachRecoversThroughBlockedCompactConfirmation(t *testing.T) {
	ticketPath := writeFrontmatterTicket(t, "claimed")
	gate := NewGate()
	var waits int
	var sentKeys [][]string
	var observedCwd, observedSession string
	var prompts []string
	var promptUntils [][]string
	var recoveryWaitUntils [][]string
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			switch waits {
			case 1:
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			case 2:
				recoveryWaitUntils = append(recoveryWaitUntils, opts.Until)
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
			case 3:
				recoveryWaitUntils = append(recoveryWaitUntils, opts.Until)
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(target string, keys ...string) error {
			sentKeys = append(sentKeys, keys)
			return nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			promptUntils = append(promptUntils, opts.Until)
			if opts.Text == "/compact" {
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		ReadCodexContext: func(cwd, sessionID string) (int, bool, error) {
			observedCwd, observedSession = cwd, sessionID
			return 150001, true, nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
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
	if len(sentKeys) != 1 || !slices.Equal(sentKeys[0], []string{"ctrl+c"}) {
		t.Errorf("AgentSendKeys calls = %v, want one pre-compact ctrl+c", sentKeys)
	}
	if observedCwd != "/repo/iter-01" || observedSession != "codex-session-1" {
		t.Errorf("ReadCodexContext(%q, %q), want (/repo/iter-01, codex-session-1)", observedCwd, observedSession)
	}
	if len(prompts) != 2 || prompts[0] != "/compact" || !strings.Contains(prompts[1], "150000") {
		t.Errorf("prompts = %v, want [/compact, <finish-up prompt mentioning 150000>]", prompts)
	}
	compactStates := append(append([]string{}, plainFinishStates...), "blocked")
	if len(promptUntils) != 2 || !slices.Equal(promptUntils[0], compactStates) {
		t.Errorf("/compact Until = %v, want %v", promptUntils, compactStates)
	}
	if len(recoveryWaitUntils) != 2 ||
		!slices.Equal(recoveryWaitUntils[0], []string{"working"}) ||
		!slices.Equal(recoveryWaitUntils[1], plainFinishStates) {
		t.Errorf("compact confirmation waits = %v, want [[working] %v]", recoveryWaitUntils, plainFinishStates)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "needs-attention") {
		t.Errorf("ticket status = %s, compact confirmation must not become needs-attention", raw)
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
	scratchDir := t.TempDir()
	var waits, prompts int
	var compactionCount int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			// Ticks past smartZoneCompactTimeoutMs (past the 10th
			// smartZonePollMs-sized tick) before the transcript records the
			// compaction finishing, so recovery must stop polling on its own
			// rather than needing the pane to ever confirm completion.
			if waits == 11 {
				compactionCount = 1
			}
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts++
			if opts.Text == "/compact" {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
		},
		Sleep: func(time.Duration) {},
	}

	p := launchAndPromptParams{
		Label:      "iter-19",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}

	recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true once the transcript confirms compaction completed")
	}
	if prompts != 2 {
		t.Errorf("prompts sent = %d, want 2 (/compact, then finish-up)", prompts)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents() ok=%v err=%v", ok, err)
	}
	var sawFailed, sawExpired, sawResumed bool
	for _, e := range events {
		switch e.Type {
		case eventSmartZoneRecoveryFailed:
			sawFailed = true
		case eventSmartZoneWaitExpired:
			sawExpired = true
		case eventResumed:
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
	scratchDir := t.TempDir()
	var prompts int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts++
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		},
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return 0, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	p := launchAndPromptParams{
		Label:      "iter-19",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}

	recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=true, want false: compaction never completed on either signal")
	}
	if prompts != 1 {
		t.Errorf("prompts sent = %d, want 1 (/compact only, no finish-up after failure)", prompts)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents() ok=%v err=%v", ok, err)
	}
	var sawFailed bool
	for _, e := range events {
		if e.Type == eventSmartZoneRecoveryFailed {
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
	scratchDir := t.TempDir()
	var prompts []string
	var waits int
	var compactionCount int
	d := Deps{
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 1 {
				// The real compaction lands between the first and second poll
				// tick of the fallthrough wait.
				compactionCount = 1
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
		},
		Sleep: func(time.Duration) {},
	}

	p := launchAndPromptParams{
		Label:      "iter-19",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}

	recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true once the transcript confirms compaction completed")
	}
	if waits == 0 {
		t.Error("AgentWait never called, want the premature idle/done report to fall through to waitForCompactionSignal")
	}
	if len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up] with finish-up sent only after the fallthrough poll confirms compaction", prompts)
	}
}

// TestRecoverSmartZoneBreach_ImmediateSuccessTrustedWhenAlreadyAdvanced
// verifies the other half of issue 03's fix: when the compaction count has
// already advanced past baseline by the time the immediate "/compact"
// success comes back, that's a genuine completion and recovery proceeds
// straight to the finish-up prompt with no extra polling.
func TestRecoverSmartZoneBreach_ImmediateSuccessTrustedWhenAlreadyAdvanced(t *testing.T) {
	scratchDir := t.TempDir()
	var prompts []string
	var waits int
	var readCompactionsCalls int
	d := Deps{
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		},
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			readCompactionsCalls++
			if readCompactionsCalls == 1 {
				return 0, true, nil // baseline, taken before "/compact" is sent
			}
			return 1, true, nil // already advanced by the time the prompt returns
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
		},
		Sleep: func(time.Duration) {},
	}

	p := launchAndPromptParams{
		Label:      "iter-19",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}

	recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true")
	}
	if waits != 0 {
		t.Errorf("AgentWait calls = %d, want 0: a genuine immediate success must not fall through to extra polling", waits)
	}
	if len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up]", prompts)
	}
}

func TestCompactSignalUnconfirmed(t *testing.T) {
	p := launchAndPromptParams{Agent: AgentClaude, SessionCwd: "/repo/iter-19"}
	confirmedBaseline := compactBoundarySnapshot{state: compactBoundaryConfirmed}

	t.Run("poll timeout is unconfirmed", func(t *testing.T) {
		d := Deps{}
		unconfirmed := compactSignalUnconfirmed(d, p, "sess-19", errors.New("timed out waiting for agent status"), confirmedBaseline)
		if !unconfirmed {
			t.Error("unconfirmed = false, want true: a poll timeout must always fall through")
		}
	})

	t.Run("non-timeout error is confirmed (not re-polled here)", func(t *testing.T) {
		d := Deps{}
		unconfirmed := compactSignalUnconfirmed(d, p, "sess-19", errors.New("boom"), confirmedBaseline)
		if unconfirmed {
			t.Error("unconfirmed = true, want false: a genuine non-timeout error is handled by the caller, not waitForCompactionSignal")
		}
	})

	t.Run("success with baseline not yet advanced is unconfirmed", func(t *testing.T) {
		d := Deps{
			ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
				return 0, true, nil
			},
		}
		unconfirmed := compactSignalUnconfirmed(d, p, "sess-19", nil, confirmedBaseline)
		if !unconfirmed {
			t.Error("unconfirmed = false, want true: transcript hasn't advanced past baseline yet")
		}
	})

	t.Run("success with baseline already advanced is confirmed", func(t *testing.T) {
		d := Deps{
			ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
				return 1, true, nil
			},
		}
		unconfirmed := compactSignalUnconfirmed(d, p, "sess-19", nil, confirmedBaseline)
		if unconfirmed {
			t.Error("unconfirmed = true, want false: transcript already confirms compaction advanced")
		}
	})

	t.Run("success on an unsupported agent is confirmed", func(t *testing.T) {
		d := Deps{}
		unconfirmed := compactSignalUnconfirmed(d, p, "sess-19", nil, compactBoundarySnapshot{state: compactBoundaryUnsupported})
		if unconfirmed {
			t.Error("unconfirmed = true, want false: no boundary signal exists for this agent, trust the immediate success")
		}
	})

	t.Run("success with a re-fetch error is unconfirmed", func(t *testing.T) {
		d := Deps{
			ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
				return 0, false, errors.New("read failed")
			},
		}
		unconfirmed := compactSignalUnconfirmed(d, p, "sess-19", nil, confirmedBaseline)
		if !unconfirmed {
			t.Error("unconfirmed = false, want true: a read that fails now proves nothing about the compaction")
		}
	})

	t.Run("success on an unavailable baseline is unconfirmed", func(t *testing.T) {
		d := Deps{
			ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
				return 3, true, nil
			},
		}
		unconfirmed := compactSignalUnconfirmed(d, p, "sess-19", nil, compactBoundarySnapshot{state: compactBoundaryUnavailable})
		if !unconfirmed {
			t.Error("unconfirmed = false, want true: with no baseline read, no later count can prove the compaction advanced")
		}
	})
}

func TestReadCompactBoundaries_ClassifiesEachState(t *testing.T) {
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

// TestRecoverSmartZoneBreach_GateHoldsWhileBoundaryStaysAtBaseline verifies
// the live incident from research ticket 15: a pane that reports idle the
// instant "/compact" is typed, over and over, while the transcript's
// compaction-boundary count never moves. The finish-up prompt must never go
// out — sending it there is what cancelled the compaction — and the give-up
// must be paced by real poll intervals rather than spinning through the
// extended bound instantly.
func TestRecoverSmartZoneBreach_GateHoldsWhileBoundaryStaysAtBaseline(t *testing.T) {
	scratchDir := t.TempDir()
	var prompts []string
	var sleeps []time.Duration
	d := Deps{
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return 0, true, nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
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
	if len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only: the finish-up prompt cancels an in-progress compaction", prompts)
	}
	// recoverSmartZoneBreach hands the wait its first tick already spent, so
	// the gated loop pays for every remaining tick up to the extended bound.
	wantSleeps := smartZoneCompactExtendedTimeoutMs/smartZonePollMs - 1
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
	scratchDir := t.TempDir()
	var prompts []string
	var sleeps []time.Duration
	var waits int
	compactionCount := 0
	d := Deps{
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 3 {
				compactionCount = 1
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			return compactionCount, true, nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
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
	if len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up]", prompts)
	}
	if len(sleeps) != 2 {
		t.Errorf("gated sleeps = %d, want 2 (the two ticks the transcript held the gate)", len(sleeps))
	}
}

// TestRecoverSmartZoneBreach_TimeoutPathIsNotDoublePaced pins the pacing
// asymmetry: a pane wait that times out has already consumed its poll interval
// inside AgentWait, so the gate must not sleep for it too. Sleeping on both
// branches would double every tick and stretch the extended bound to twenty
// minutes of wall clock.
func TestRecoverSmartZoneBreach_TimeoutPathIsNotDoublePaced(t *testing.T) {
	scratchDir := t.TempDir()
	var sleeps []time.Duration
	var waits int
	d := Deps{
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		},
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
	wantWaits := smartZoneCompactExtendedTimeoutMs/smartZonePollMs - 1
	if waits != wantWaits {
		t.Errorf("AgentWait calls = %d, want %d (the extended bound reached in ten minutes of ticks, not twenty)", waits, wantWaits)
	}
}

// TestRecoverSmartZoneBreach_UnsupportedFailsOpenButUnavailableDoesNot covers
// the two states that fail open versus closed on the same underlying
// (0, false, nil) read: a build with no ReadCompactions dependency has no
// boundary signal at all and must behave exactly as it did before the gate,
// while a Claude session whose id isn't known yet is merely unavailable and
// must not be trusted on the pane's word.
func TestRecoverSmartZoneBreach_UnsupportedFailsOpenButUnavailableDoesNot(t *testing.T) {
	newDeps := func(prompts *[]string, readCompactions func(string, string) (int, bool, error)) Deps {
		return Deps{
			AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
				*prompts = append(*prompts, opts.Text)
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			},
			AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			},
			ReadCompactions: readCompactions,
			AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
				return "compaction complete", nil
			},
			Sleep: func(time.Duration) {},
		}
	}

	t.Run("no boundary signal at all trusts the idle pane", func(t *testing.T) {
		var prompts []string
		recovered, err := recoverSmartZoneBreach(newDeps(&prompts, nil), gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
		if err != nil {
			t.Fatalf("recoverSmartZoneBreach: %v", err)
		}
		if !recovered || len(prompts) != 2 {
			t.Errorf("recovered = %v, prompts = %v; want the pre-gate behavior: recovered with a finish-up prompt", recovered, prompts)
		}
	})

	t.Run("unidentified session holds the gate closed", func(t *testing.T) {
		var prompts []string
		d := newDeps(&prompts, func(string, string) (int, bool, error) { return 0, true, nil })
		recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "", "smart-zone breach", 100)
		if !errors.Is(err, errCompactNeverConfirmed) {
			t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
		}
		if recovered || len(prompts) != 1 {
			t.Errorf("recovered = %v, prompts = %v; want no finish-up prompt: an empty session id is unavailable, not unsupported", recovered, prompts)
		}
	})
}

// TestWaitForFinish_AbsorbsGatedGiveUpAndKeepsPolling verifies the call site:
// a gated give-up is a failed recovery, not a failed iteration, so
// waitForFinish swallows that one error and returns to polling. Any other
// error around the breach path still aborts the iteration.
func TestWaitForFinish_AbsorbsGatedGiveUpAndKeepsPolling(t *testing.T) {
	var prompts []string
	var waits int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			// The compact-completion polls are the ones waiting on "blocked"
			// too (see compactStates): those report the premature idle the gate
			// exists to distrust.
			if slices.Contains(opts.Until, "blocked") {
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			}
			waits++
			if waits == 1 {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys:   func(string, ...string) error { return nil },
		ReadOccupancy:   func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) { return 0, true, nil },
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
		},
		Sleep: func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-19", Agent: AgentClaude, Pane: "pane-1", Ticket: "19",
		SessionCwd: "/repo/iter-19", SmartZone: 100, Gate: NewGate(),
	}, "sess-19")
	if err != nil {
		t.Fatalf("waitForFinish: %v, want the gated give-up absorbed", err)
	}
	if len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only: no finish-up prompt may be sent on a gated give-up", prompts)
	}
}

func TestWaitForFinish_PropagatesNonGatedRecoveryErrors(t *testing.T) {
	var waits int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 1 {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys:   func(string, ...string) error { return errors.New("pane is gone") },
		ReadOccupancy:   func(cwd, sessionID string) (int, bool, error) { return 200, true, nil },
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) { return 0, true, nil },
		Sleep:           func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-19", Agent: AgentClaude, Pane: "pane-1", Ticket: "19",
		SessionCwd: "/repo/iter-19", SmartZone: 100, Gate: NewGate(),
	}, "sess-19")
	if err == nil || !strings.Contains(err.Error(), "pane is gone") {
		t.Fatalf("waitForFinish error = %v, want the transport failure propagated, not absorbed", err)
	}
}

func TestConfirmCompactSubmitted(t *testing.T) {
	t.Run("trailing /compact line reports not yet submitted", func(t *testing.T) {
		d := Deps{
			AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
				return "some earlier output\n/compact", nil
			},
		}
		submitted, err := confirmCompactSubmitted(d, "pane-1")
		if err != nil {
			t.Fatalf("confirmCompactSubmitted: %v", err)
		}
		if submitted {
			t.Error("submitted = true, want false while /compact is still the trailing line")
		}
	})

	t.Run("rendered output past /compact reports submitted", func(t *testing.T) {
		d := Deps{
			AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
				return "/compact\nCompacting conversation...\nworking", nil
			},
		}
		submitted, err := confirmCompactSubmitted(d, "pane-1")
		if err != nil {
			t.Fatalf("confirmCompactSubmitted: %v", err)
		}
		if !submitted {
			t.Error("submitted = false, want true once the pane has rendered past /compact")
		}
	})
}

// TestConfirmCompactSubmittedWithRetry_PacesWithSleep verifies the retry loop
// paces itself via d.Sleep, not AgentWait: a sequence of not-yet-submitted
// AgentRead results should drive exactly one Sleep call per retry, each of
// smartZoneCompactSubmitPollMs, with no AgentWait call at all.
func TestConfirmCompactSubmittedWithRetry_PacesWithSleep(t *testing.T) {
	var reads int
	var sleeps []time.Duration
	d := Deps{
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			reads++
			if reads <= 3 {
				return "/compact", nil
			}
			return "/compact\nCompacting conversation...", nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			t.Fatal("confirmCompactSubmittedWithRetry must not call AgentWait")
			return herdr.Agent{}, nil
		},
		Sleep: func(d time.Duration) {
			sleeps = append(sleeps, d)
		},
	}

	if err := confirmCompactSubmittedWithRetry(d, "pane-1"); err != nil {
		t.Fatalf("confirmCompactSubmittedWithRetry: %v", err)
	}

	wantSleeps := []time.Duration{
		smartZoneCompactSubmitPollMs * time.Millisecond,
		smartZoneCompactSubmitPollMs * time.Millisecond,
		smartZoneCompactSubmitPollMs * time.Millisecond,
	}
	if !slices.Equal(sleeps, wantSleeps) {
		t.Errorf("sleeps = %v, want %v", sleeps, wantSleeps)
	}
}

// TestRecoverSmartZoneBreach_FinishUpGatedOnCompactSubmitConfirmation verifies
// the prompt-submission race from research ticket 03: a compact-completion
// signal sampled before Enter's effect has rendered "/compact" as submitted
// must not let the finish-up prompt go out on top of it. The finish-up
// prompt must wait until confirmCompactSubmitted confirms.
func TestRecoverSmartZoneBreach_FinishUpGatedOnCompactSubmitConfirmation(t *testing.T) {
	scratchDir := t.TempDir()
	var prompts []string
	var sentKeys [][]string
	var reads int
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(target string, keys ...string) error {
			sentKeys = append(sentKeys, keys)
			return nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			reads++
			if reads <= 2 {
				return "/compact", nil
			}
			return "/compact\nCompacting conversation...", nil
		},
		Sleep: func(time.Duration) {},
	}

	p := launchAndPromptParams{
		Label:      "iter-19",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}

	recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true once submission is confirmed")
	}
	if len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up]", prompts)
	}
	if reads < 3 {
		t.Errorf("AgentRead calls = %d, want at least 3 (unsubmitted polls before confirmation)", reads)
	}
	if len(sentKeys) != 0 {
		t.Errorf("AgentSendKeys calls = %v, want none: the gate must never nudge or resubmit", sentKeys)
	}
}

// TestRecoverSmartZoneBreach_FinishUpGateGivesUpAfterTimeout verifies the
// gate's bound: if the pane never renders /compact as submitted, the retry
// loop gives up after smartZoneCompactSubmitTimeoutMs without ever nudging or
// resubmitting, and no finish-up prompt is sent.
func TestRecoverSmartZoneBreach_FinishUpGateGivesUpAfterTimeout(t *testing.T) {
	scratchDir := t.TempDir()
	var prompts []string
	var sentKeys [][]string
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(target string, keys ...string) error {
			sentKeys = append(sentKeys, keys)
			return nil
		},
		AgentRead: func(string, herdr.AgentReadOptions) (string, error) {
			return "/compact", nil
		},
		Sleep: func(time.Duration) {},
	}

	p := launchAndPromptParams{
		Label:      "iter-19",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}

	recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=true, want false: /compact never confirmed submitted")
	}
	if len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only, no finish-up", prompts)
	}
	if len(sentKeys) != 0 {
		t.Errorf("AgentSendKeys calls = %v, want none: the gate must never nudge or resubmit", sentKeys)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents() ok=%v err=%v", ok, err)
	}
	var sawFailed bool
	for _, e := range events {
		if e.Type == eventSmartZoneRecoveryFailed {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Error("missing smart-zone-recovery-failed event when submission never confirms")
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

func (s *quotaEventSink) IterationPaused(identifier string, kind PauseKind, reason string) {
	s.paused = append(s.paused, quotaPauseEvent{identifier: identifier, kind: kind, reason: reason})
}

func (s *quotaEventSink) IterationResumed(identifier string, kind PauseKind) {
	s.resumed = append(s.resumed, quotaResumeEvent{identifier: identifier, kind: kind})
}

func (s *occupancySink) ContextOccupancy(identifier string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, occupancyCall{identifier: identifier, tokens: tokens})
}

func TestWaitForFinish_EmitsContextOccupancyOnEachPollTimeout(t *testing.T) {
	var waits int
	sink := &occupancySink{}
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits <= 2 {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 1000 * waits, true, nil
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

func TestWaitForFinish_CodexBlockedMarksNeedsAttentionThenRecovers(t *testing.T) {
	ticketPath := writeFrontmatterTicket(t, "claimed")
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
				raw, err := os.ReadFile(ticketPath)
				if err == nil {
					sawNeedsAttention = strings.Contains(string(raw), "needs-attention")
				}
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			default:
				return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
			}
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
	for _, quota := range []string{"primary", "secondary"} {
		t.Run(quota, func(t *testing.T) {
			ticketPath := writeFrontmatterTicket(t, "claimed")
			gate := NewGate()
			sink := &quotaEventSink{}
			var waits, prompts, quotaChecks, interruptions int
			var sawPausedGate bool
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
				AgentSendKeys: func(string, ...string) error {
					interruptions++
					return nil
				},
				ReadCodexRateLimit: func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
					quotaChecks++
					if quotaChecks > 1 {
						sawPausedGate = gate.isPaused()
						return codexsession.RateLimit{}, false, nil
					}
					return codexsession.RateLimit{Quota: quota, ResetAt: time.Now().Add(-time.Second)}, true, nil
				},
				Sleep: func(time.Duration) {},
				Now:   time.Now,
			}

			if err := waitForFinish(d, launchAndPromptParams{
				Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
				Gate: gate, Sink: sink,
			}, "codex-session-1"); err != nil {
				t.Fatalf("waitForFinish: %v", err)
			}
			if prompts != 1 {
				t.Errorf("continue prompts = %d, want 1", prompts)
			}
			if interruptions != 0 {
				t.Errorf("pane interruptions = %d, want 0", interruptions)
			}
			if !sawPausedGate {
				t.Error("shared gate was not paused while waiting for the Codex quota reset")
			}
			if gate.isPaused() {
				t.Error("gate remains paused after the Codex quota reset")
			}
			wantPaused := quotaPauseEvent{
				identifier: "iter-01",
				kind:       PauseRateLimit,
			}
			if len(sink.paused) != 1 || sink.paused[0].identifier != wantPaused.identifier ||
				sink.paused[0].kind != wantPaused.kind || !strings.Contains(sink.paused[0].reason, "Codex "+quota+" quota exhausted") {
				t.Errorf("paused events = %+v, want one typed %s rate-limit pause", sink.paused, quota)
			}
			if len(sink.resumed) != 1 || sink.resumed[0] != (quotaResumeEvent{identifier: "iter-01", kind: PauseRateLimit}) {
				t.Errorf("resumed events = %+v, want one typed rate-limit resume", sink.resumed)
			}
			raw, err := os.ReadFile(ticketPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !strings.Contains(string(raw), "claimed") || strings.Contains(string(raw), "needs-attention") {
				t.Errorf("ticket status = %s, want claimed without needs-attention", raw)
			}
		})
	}
}

func TestCodexRateLimit_UsesPaneOnlyWhenStructuredQuotaCannotIdentifyBlock(t *testing.T) {
	structuredErr := errors.New("rollout unreadable")
	paneErr := errors.New("pane unreadable")
	cases := []struct {
		name          string
		structured    func(string, string) (codexsession.RateLimit, bool, error)
		pane          func(string) (string, error)
		wantQuota     string
		wantExhausted bool
		wantErr       error
		wantPaneReads int
	}{
		{
			name: "structured quota wins",
			structured: func(string, string) (codexsession.RateLimit, bool, error) {
				return codexsession.RateLimit{Quota: "primary"}, true, nil
			},
			pane:          func(string) (string, error) { return "You've hit your usage limit.", nil },
			wantQuota:     "primary",
			wantExhausted: true,
		},
		{
			name: "null rollout falls back to pane",
			structured: func(string, string) (codexsession.RateLimit, bool, error) {
				return codexsession.RateLimit{}, false, nil
			},
			pane:          func(string) (string, error) { return "You've hit your usage limit.", nil },
			wantQuota:     "usage",
			wantExhausted: true,
			wantPaneReads: 1,
		},
		{
			name: "incidental pane text is not quota",
			structured: func(string, string) (codexsession.RateLimit, bool, error) {
				return codexsession.RateLimit{}, false, nil
			},
			pane:          func(string) (string, error) { return "blocked: approve this command", nil },
			wantPaneReads: 1,
		},
		{
			name: "structured error stops classification",
			structured: func(string, string) (codexsession.RateLimit, bool, error) {
				return codexsession.RateLimit{}, false, structuredErr
			},
			pane:    func(string) (string, error) { return "You've hit your usage limit.", nil },
			wantErr: structuredErr,
		},
		{
			name: "pane error is returned",
			structured: func(string, string) (codexsession.RateLimit, bool, error) {
				return codexsession.RateLimit{}, false, nil
			},
			pane:          func(string) (string, error) { return "", paneErr },
			wantErr:       paneErr,
			wantPaneReads: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paneReads := 0
			d := Deps{
				ReadCodexRateLimit: tc.structured,
				ReadPaneRecent: func(pane string) (string, error) {
					paneReads++
					return tc.pane(pane)
				},
				Now: func() time.Time { return time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC) },
			}

			limit, exhausted, err := codexRateLimit(d, "/repo/iter-01", "session-1", "pane-1")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if exhausted != tc.wantExhausted || limit.Quota != tc.wantQuota {
				t.Errorf("limit = %+v, exhausted = %v; want quota %q, exhausted %v", limit, exhausted, tc.wantQuota, tc.wantExhausted)
			}
			if paneReads != tc.wantPaneReads {
				t.Errorf("pane reads = %d, want %d", paneReads, tc.wantPaneReads)
			}
		})
	}
}

func TestWaitForFinish_CodexQuotaDetectionErrorPreservesClaimedTicket(t *testing.T) {
	ticketPath := writeFrontmatterTicket(t, "claimed")
	d := Deps{
		AgentWait: func(herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{AgentStatus: "blocked"}, nil
		},
		ReadCodexRateLimit: func(string, string) (codexsession.RateLimit, bool, error) {
			return codexsession.RateLimit{}, false, errors.New("rollout unreadable")
		},
	}

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
	if !strings.Contains(string(raw), "claimed") || strings.Contains(string(raw), "needs-attention") {
		t.Errorf("ticket status = %s, want claimed without needs-attention", raw)
	}
}

func TestWaitForFinish_CodexPaneQuotaDoesNotBecomeNeedsAttention(t *testing.T) {
	ticketPath := writeFrontmatterTicket(t, "claimed")
	gate := NewGate()
	sink := &quotaEventSink{}
	waits := 0
	structuredReads := 0
	d := Deps{
		AgentWait: func(herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 1 {
				return herdr.Agent{AgentStatus: "blocked"}, nil
			}
			return herdr.Agent{AgentStatus: "idle"}, nil
		},
		ReadCodexRateLimit: func(string, string) (codexsession.RateLimit, bool, error) {
			structuredReads++
			return codexsession.RateLimit{}, false, nil
		},
		ReadPaneRecent: func(string) (string, error) {
			return "■ You've hit your usage limit.", nil
		},
		Sleep: func(time.Duration) {},
	}

	if err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", TicketPath: ticketPath, Gate: gate, Sink: sink,
	}, "session-1"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if structuredReads != 2 {
		t.Errorf("structured quota reads = %d, want classification and reset recheck", structuredReads)
	}
	if len(sink.paused) != 1 || sink.paused[0].kind != PauseRateLimit || len(sink.resumed) != 1 {
		t.Errorf("quota events = paused %+v, resumed %+v; want one of each", sink.paused, sink.resumed)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "claimed") || strings.Contains(string(raw), "needs-attention") {
		t.Errorf("ticket status = %s, want claimed without needs-attention", raw)
	}
}

func TestWaitForFinish_CodexPaneReadErrorPreservesClaimedTicket(t *testing.T) {
	ticketPath := writeFrontmatterTicket(t, "claimed")
	d := Deps{
		AgentWait: func(herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{AgentStatus: "blocked"}, nil
		},
		ReadCodexRateLimit: func(string, string) (codexsession.RateLimit, bool, error) {
			return codexsession.RateLimit{}, false, nil
		},
		ReadPaneRecent: func(string) (string, error) { return "", errors.New("pane unreadable") },
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", TicketPath: ticketPath, Gate: NewGate(),
	}, "session-1")
	if err == nil || !strings.Contains(err.Error(), "pane unreadable") {
		t.Fatalf("waitForFinish error = %v, want pane read failure", err)
	}
	raw, readErr := os.ReadFile(ticketPath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if !strings.Contains(string(raw), "claimed") || strings.Contains(string(raw), "needs-attention") {
		t.Errorf("ticket status = %s, want claimed without needs-attention", raw)
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
