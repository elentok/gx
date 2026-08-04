package ralphloop

import (
	"errors"
	"fmt"
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
		ResumeSignaled: func(path string) (bool, error) { return false, nil },
		Sleep:          func(time.Duration) {},
	}

	err := waitForFinish(d, launchAndPromptParams{
		Label:            "iter-01",
		Agent:            AgentCodex,
		Pane:             "pane-1",
		Ticket:           "01",
		TicketPath:       ticketPath,
		SessionCwd:       "/repo/iter-01",
		SmartZone:        150000,
		Gate:             gate,
		ResumeSignalPath: "unused",
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

func TestWaitForFinish_ManualAttentionRecheckKeepsBlockedTicketPaused(t *testing.T) {
	ticketPath := writeFrontmatterTicket(t, "claimed")
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
