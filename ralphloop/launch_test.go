package ralphloop

import (
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

// TestPromptedLaunch_IterationStartedCarriesCwdAndSessionIDPlusImmediateOccupancy
// covers ticket 02's two start-time requirements: IterationStarted must carry
// cwd/sessionID (so a consumer can resolve the transcript itself), and one
// extra immediate ContextOccupancy read/emit must fire right away, rather
// than waiting up to smartZonePollMs for the first poll tick to report it.
func TestPromptedLaunch_IterationStartedCarriesCwdAndSessionIDPlusImmediateOccupancy(t *testing.T) {
	t.Parallel()
	var started struct {
		identifier, label, cwd, sessionID string
	}
	occSink := &occupancySink{}
	sink := &recordingSinkWithArgs{
		occupancySink: occSink,
		onIterationStarted: func(ticket tickets.Ticket, label, cwd, sessionID string) {
			started.identifier, started.label, started.cwd, started.sessionID = ticket.Identifier, label, cwd, sessionID
		},
	}

	d := Deps{
		Runner: paneRunner("iter-01", "pane-1", "sess-1"),
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 4200, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	sessionID, err := promptedLaunch(d, launchAndPromptParams{
		Label:      "iter-01",
		Agent:      AgentClaude,
		Session:    agentrunner.Session{Label: "iter-01", ID: "pane-1"},
		Pane:       "pane-1",
		Prompt:     "go",
		SessionCwd: "/repo/iter-01",
		Ticket:     "01",
		TicketData: tickets.Ticket{Identifier: "01"},
		StartEvent: "iteration-started",
		Sink:       sink,
	}, 0)
	if err != nil {
		t.Fatalf("promptedLaunch: %v", err)
	}
	if sessionID != "sess-1" {
		t.Fatalf("sessionID = %q, want sess-1", sessionID)
	}
	if started.identifier != "01" || started.label != "iter-01" || started.cwd != "/repo/iter-01" || started.sessionID != "sess-1" {
		t.Errorf("IterationStarted args = %+v, want {01 iter-01 /repo/iter-01 sess-1}", started)
	}
	if len(occSink.calls) != 1 || occSink.calls[0].identifier != "01" || occSink.calls[0].tokens != 4200 {
		t.Errorf("ContextOccupancy calls = %+v, want one {01 4200} for the immediate start-time read", occSink.calls)
	}
}

// Codex does not expose its native session ID when the interactive process is
// first started, only once the first prompt begins working, so promptedLaunch
// must read the session ID from the post-prompt status for monitoring,
// logging, and landing.
func TestPromptedLaunch_CodexAdoptsSessionIDFromStatusAfterPrompt(t *testing.T) {
	t.Parallel()
	var startedSessionID string
	var observedSessionID string
	sink := &recordingSinkWithArgs{
		occupancySink: &occupancySink{},
		onIterationStarted: func(_ tickets.Ticket, _, _, sessionID string) {
			startedSessionID = sessionID
		},
	}

	d := Deps{
		Runner: paneRunner("iter-09", "pane-9", "codex-session-1"),
		ReadCodexContext: func(_, sessionID string) (int, bool, error) {
			observedSessionID = sessionID
			return 4200, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	sessionID, err := promptedLaunch(d, launchAndPromptParams{
		Label:      "iter-09",
		Agent:      AgentCodex,
		Session:    agentrunner.Session{Label: "iter-09", ID: "pane-9"},
		Pane:       "pane-9",
		Prompt:     "go",
		SessionCwd: "/repo/iter-09",
		Ticket:     "09",
		StartEvent: string(events.IterationStarted),
		Sink:       sink,
	}, 0)
	if err != nil {
		t.Fatalf("promptedLaunch: %v", err)
	}
	if sessionID != "codex-session-1" {
		t.Fatalf("sessionID = %q, want codex-session-1", sessionID)
	}
	if startedSessionID != "codex-session-1" {
		t.Errorf("IterationStarted sessionID = %q, want codex-session-1", startedSessionID)
	}
	if observedSessionID != "codex-session-1" {
		t.Errorf("ReadCodexContext sessionID = %q, want codex-session-1", observedSessionID)
	}
}

// TestNoActivitySinceLaunch covers ticket 07: a matching string(events.IterationStarted)
// with a zero/unset StateChangeSeq is never treated as a real launch-time
// baseline — the scan must keep looking past it rather than short-circuit on
// it.
func TestNoActivitySinceLaunch(t *testing.T) {
	t.Parallel()

	t.Run("zero-seq match is not a real baseline", func(t *testing.T) {
		t.Parallel()
		scratchDir := epicScratchDir(t, "fix-spinner")
		epicName := "fix-spinner"
		if err := logEvent(scratchDir, epicName, Event{
			Type:         string(events.IterationStarted),
			AgentSession: "sess-live",
		}); err != nil {
			t.Fatalf("logEvent: %v", err)
		}

		if got := noActivitySinceLaunch(scratchDir, epicName, "sess-live", 0); got {
			t.Errorf("noActivitySinceLaunch() = true, want false (zero-seq event must not be treated as a real baseline)")
		}
	})

	t.Run("no matching event falls through to false", func(t *testing.T) {
		t.Parallel()
		scratchDir := epicScratchDir(t, "fix-spinner")
		epicName := "fix-spinner"
		if err := logEvent(scratchDir, epicName, Event{
			Type:         string(events.IterationStarted),
			AgentSession: "sess-other",
		}); err != nil {
			t.Fatalf("logEvent: %v", err)
		}

		if got := noActivitySinceLaunch(scratchDir, epicName, "sess-live", 946); got {
			t.Errorf("noActivitySinceLaunch() = true, want false (no matching event)")
		}
	})

	t.Run("genuine non-zero baseline still matches", func(t *testing.T) {
		t.Parallel()
		scratchDir := epicScratchDir(t, "fix-spinner")
		epicName := "fix-spinner"
		if err := logEvent(scratchDir, epicName, Event{
			Type:           string(events.IterationStarted),
			AgentSession:   "sess-live",
			StateChangeSeq: 946,
		}); err != nil {
			t.Fatalf("logEvent: %v", err)
		}

		if got := noActivitySinceLaunch(scratchDir, epicName, "sess-live", 946); !got {
			t.Errorf("noActivitySinceLaunch() = false, want true (current seq matches genuine baseline)")
		}
		if got := noActivitySinceLaunch(scratchDir, epicName, "sess-live", 950); got {
			t.Errorf("noActivitySinceLaunch() = true, want false (current seq has moved past genuine baseline)")
		}
	})

	t.Run("skips zero-seq event to find a later genuine baseline", func(t *testing.T) {
		t.Parallel()
		scratchDir := epicScratchDir(t, "fix-spinner")
		epicName := "fix-spinner"
		if err := logEvent(scratchDir, epicName, Event{
			Type:         string(events.IterationStarted),
			AgentSession: "sess-live",
		}); err != nil {
			t.Fatalf("logEvent: %v", err)
		}
		if err := logEvent(scratchDir, epicName, Event{
			Type:           string(events.IterationStarted),
			AgentSession:   "sess-live",
			StateChangeSeq: 946,
		}); err != nil {
			t.Fatalf("logEvent: %v", err)
		}

		if got := noActivitySinceLaunch(scratchDir, epicName, "sess-live", 946); !got {
			t.Errorf("noActivitySinceLaunch() = false, want true (must skip the zero-seq event and match the later genuine baseline)")
		}
	})
}

// recordingSinkWithArgs embeds occupancySink (itself embedding
// noopEventSink) and additionally hooks IterationStarted, for tests that
// need both start-time signals asserted together.
type recordingSinkWithArgs struct {
	*occupancySink
	onIterationStarted func(ticket tickets.Ticket, label, cwd, sessionID string)
}

func (s *recordingSinkWithArgs) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string, agent AgentKind, paneID, tabID string) {
	if s.onIterationStarted != nil {
		s.onIterationStarted(ticket, label, cwd, sessionID)
	}
}
