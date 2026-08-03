package ralphloop

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

func TestWaitForFinish_CodexContextBreachRecoversViaCompactAndFinishPrompt(t *testing.T) {
	gate := NewGate()
	var waits int
	var interrupted bool
	var observedCwd, observedSession string
	var prompts []string
	var promptUntils [][]string
	d := Deps{
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waits++
			if waits == 1 {
				return herdr.Agent{}, errors.New("timed out waiting for agent status")
			}
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentSendKeys: func(target string, keys ...string) error {
			interrupted = slices.Equal(keys, []string{"ctrl+c"})
			return nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			prompts = append(prompts, opts.Text)
			promptUntils = append(promptUntils, opts.Until)
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
		SessionCwd:       "/repo/iter-01",
		SmartZone:        150000,
		Gate:             gate,
		ResumeSignalPath: "unused",
	}, "codex-session-1")
	if err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if !interrupted {
		t.Error("AgentSendKeys did not interrupt the Codex pane with ctrl-c")
	}
	if observedCwd != "/repo/iter-01" || observedSession != "codex-session-1" {
		t.Errorf("ReadCodexContext(%q, %q), want (/repo/iter-01, codex-session-1)", observedCwd, observedSession)
	}
	if len(prompts) != 2 || prompts[0] != "/compact" || !strings.Contains(prompts[1], "150000") {
		t.Errorf("prompts = %v, want [/compact, <finish-up prompt mentioning 150000>]", prompts)
	}
	if len(promptUntils) != 2 || !slices.Equal(promptUntils[0], plainFinishStates) {
		t.Errorf("/compact Until = %v, want %v (must wait for compaction to finish, not just start, "+
			"or the finish-up prompt lands mid-compaction and cancels it)", promptUntils, plainFinishStates)
	}
	if gate.isPaused() {
		t.Error("gate.isPaused() = true, want smart-zone recovery to never pause the Gate")
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
	ticketPath := writeFrontmatterTicket(t, "claimed")
	gate := NewGate()
	var waits, prompts, quotaChecks int
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
		ReadCodexRateLimit: func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
			quotaChecks++
			if quotaChecks > 1 {
				return codexsession.RateLimit{}, false, nil
			}
			return codexsession.RateLimit{Quota: "primary", ResetAt: time.Now().Add(-time.Second)}, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	if err := waitForFinish(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		Gate: gate,
	}, "codex-session-1"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	if prompts != 1 {
		t.Errorf("continue prompts = %d, want 1", prompts)
	}
	if gate.isPaused() {
		t.Error("gate remains paused after the Codex quota reset")
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "needs-attention") {
		t.Errorf("ticket status = %s, quota exhaustion must not become needs-attention", raw)
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
