package ralphloop

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets/schema"
)

// TestRecoverCodexRateLimit_BlockedAfterReset_ParksInsteadOfPrompting covers
// ticket 04: under herdr 0.8.2, a Codex pane that comes back blocked once its
// quota resets must never be prompted with "continue" (a hard-rejected
// agent_blocked submission before this fix, and the wrong recovery even if it
// weren't — the pane is sitting on its own dialog, not waiting for the
// iteration to resume). It must park for a human instead, naming the
// unanswered dialog by its matched_rule.id.
func TestRecoverCodexRateLimit_BlockedAfterReset_ParksInsteadOfPrompting(t *testing.T) {
	t.Parallel()
	ticketPath := writeFrontmatterTicket(t, "claimed")
	scratchDir := t.TempDir()

	d := Deps{
		Now:   func() time.Time { return time.Unix(0, 0) },
		Sleep: func(time.Duration) {},
		ReadCodexRateLimit: func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
			return codexsession.RateLimit{}, false, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
		},
		AgentExplain: func(target string) (herdr.AgentExplainResult, error) {
			return herdr.AgentExplainResult{State: "blocked", MatchedRuleID: "trust_directory"}, nil
		},
		AgentPrompt: func(herdr.AgentPromptOptions) (herdr.Agent, error) {
			t.Fatal("AgentPrompt was called on a pane herdr reports blocked; must never prompt it")
			return herdr.Agent{}, nil
		},
	}

	err := recoverCodexRateLimit(d, launchAndPromptParams{
		Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
		ScratchDir: scratchDir, EpicName: "epic", Gate: NewGate(),
	}, "sess-1", codexsession.RateLimit{Quota: "usage"})
	if !errors.Is(err, errBlockedPaneParked) {
		t.Fatalf("recoverCodexRateLimit() err = %v, want errBlockedPaneParked", err)
	}

	raw, readErr := os.ReadFile(ticketPath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	ticket, parseErr := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if parseErr != nil {
		t.Fatalf("ParseTicketFromRaw: %v", parseErr)
	}
	if ticket.Status != schema.StatusNeedsAnswer {
		t.Errorf("Status = %q, want needs-answer", ticket.Status)
	}
	if ticket.ParkKind != schema.ParkKindBlockedPane {
		t.Errorf("ParkKind = %q, want blocked-pane", ticket.ParkKind)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok || len(events) == 0 {
		t.Fatalf("ReadEvents() = %+v, ok=%v, err=%v", events, ok, err)
	}
	last := events[len(events)-1]
	if last.Type != eventNeedsAnswer {
		t.Fatalf("last event type = %q, want needs-answer", last.Type)
	}
	if !strings.Contains(last.Reason, "trust_directory") {
		t.Errorf("park reason = %q, want it to name the matched_rule.id trust_directory", last.Reason)
	}

	// No trust_directory (or any other) auto-answer branch: parking is the
	// only outcome, regardless of which rule herdr reports matched.
	if strings.Contains(strings.ToLower(last.Reason), "answered") {
		t.Errorf("park reason = %q, suggests an auto-answer happened; this path must never answer a dialog itself", last.Reason)
	}
}

// TestRecoverCodexRateLimit_NonBlockedAfterReset_ReturnsWithoutSendingAnything
// covers this recovery's unaffected branch: a pane that comes back idle,
// done, or working after the reset must return with no prompt or park at
// all.
func TestRecoverCodexRateLimit_NonBlockedAfterReset_ReturnsWithoutSendingAnything(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"idle", "done", "working"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			ticketPath := writeFrontmatterTicket(t, "claimed")
			scratchDir := t.TempDir()

			d := Deps{
				Now:   func() time.Time { return time.Unix(0, 0) },
				Sleep: func(time.Duration) {},
				ReadCodexRateLimit: func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
					return codexsession.RateLimit{}, false, nil
				},
				AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
					return herdr.Agent{PaneID: opts.Target, AgentStatus: status}, nil
				},
				AgentExplain: func(target string) (herdr.AgentExplainResult, error) {
					t.Fatal("AgentExplain was called for a non-blocked pane")
					return herdr.AgentExplainResult{}, nil
				},
				AgentPrompt: func(herdr.AgentPromptOptions) (herdr.Agent, error) {
					t.Fatal("AgentPrompt was called on the unaffected non-blocked branch")
					return herdr.Agent{}, nil
				},
			}

			err := recoverCodexRateLimit(d, launchAndPromptParams{
				Label: "iter-01", Agent: AgentCodex, Pane: "pane-1", Ticket: "01", TicketPath: ticketPath,
				ScratchDir: scratchDir, EpicName: "epic", Gate: NewGate(),
			}, "sess-1", codexsession.RateLimit{Quota: "usage"})
			if err != nil {
				t.Fatalf("recoverCodexRateLimit() err = %v, want nil", err)
			}

			raw, readErr := os.ReadFile(ticketPath)
			if readErr != nil {
				t.Fatalf("ReadFile: %v", readErr)
			}
			if strings.Contains(string(raw), "needs-answer") {
				t.Errorf("ticket was parked on the unaffected non-blocked branch:\n%s", raw)
			}
		})
	}
}
