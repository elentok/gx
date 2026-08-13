package ralphloop

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// TestLaunchAndPrompt_IterationStartedCarriesCwdAndSessionIDPlusImmediateOccupancy
// covers ticket 02's two start-time requirements: IterationStarted must carry
// cwd/sessionID (so a consumer can resolve the transcript itself), and one
// extra immediate ContextOccupancy read/emit must fire right away, rather
// than waiting up to smartZonePollMs for the first poll tick to report it.
func TestLaunchAndPrompt_IterationStartedCarriesCwdAndSessionIDPlusImmediateOccupancy(t *testing.T) {
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
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-1"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 4200, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	sessionID, err := launchAndPrompt(d, launchAndPromptParams{
		Label:      "iter-01",
		Agent:      AgentClaude,
		Pane:       "pane-1",
		Prompt:     "go",
		SessionCwd: "/repo/iter-01",
		Ticket:     "01",
		TicketData: tickets.Ticket{Identifier: "01"},
		StartEvent: "iteration-started",
		Sink:       sink,
	})
	if err != nil {
		t.Fatalf("launchAndPrompt: %v", err)
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
// first started. Herdr discovers it after the first prompt begins working, so
// launchAndPrompt must adopt the AgentPrompt result rather than carrying the
// empty AgentStart session through monitoring, logging, and landing.
func TestLaunchAndPrompt_CodexAdoptsSessionIDFromInitialPrompt(t *testing.T) {
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
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working", AgentSession: "codex-session-1"}, nil
		},
		ReadCodexContext: func(_, sessionID string) (int, bool, error) {
			observedSessionID = sessionID
			return 4200, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	sessionID, err := launchAndPrompt(d, launchAndPromptParams{
		Label:      "iter-09",
		Agent:      AgentCodex,
		Pane:       "pane-9",
		Prompt:     "go",
		SessionCwd: "/repo/iter-09",
		Ticket:     "09",
		StartEvent: eventIterationStarted,
		Sink:       sink,
	})
	if err != nil {
		t.Fatalf("launchAndPrompt: %v", err)
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

// TestLaunchAndPrompt_AgentNameTakenByOwnWorktree_AttachesInsteadOfFailing is
// a regression test for the iter-06b agent_name_taken incident's secondary
// defense (fix 2, on top of claimNext's own launched-set de-dup in
// loop.go): if AgentStart fails with agent_name_taken and the reported
// candidate's cwd is this iteration's own SessionCwd, launchAndPrompt must
// not hard-fail the ticket to needs-repair — it attaches to the live
// pane (via AgentGet) and waits it out, without sending a second, redundant
// initial prompt.
func TestLaunchAndPrompt_AgentNameTakenByOwnWorktree_AttachesInsteadOfFailing(t *testing.T) {
	t.Parallel()
	var promptCalls int
	var waitTargets []string
	sink := &recordingSinkWithArgs{occupancySink: &occupancySink{}}

	d := Deps{
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{}, &herdr.AgentNameTakenError{
				Message:      "agent name iter-01 is already used; candidates: cwd=/repo/iter-01 status=Working",
				CandidateCwd: "/repo/iter-01",
			}
		},
		AgentGet: func(target string) (herdr.Agent, error) {
			if target != "iter-01" {
				t.Errorf("AgentGet target = %q, want the agent label %q", target, "iter-01")
			}
			return herdr.Agent{PaneID: "live-pane", AgentStatus: "working", AgentSession: "sess-live"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			waitTargets = append(waitTargets, opts.Target)
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			promptCalls++
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		Sleep: func(time.Duration) {},
	}

	sessionID, err := launchAndPrompt(d, launchAndPromptParams{
		Label:      "iter-01",
		Agent:      AgentClaude,
		Pane:       "fresh-pane",
		Prompt:     "go",
		SessionCwd: "/repo/iter-01",
		Ticket:     "01",
		StartEvent: eventIterationStarted,
		Sink:       sink,
	})
	if err != nil {
		t.Fatalf("launchAndPrompt: %v", err)
	}
	if sessionID != "sess-live" {
		t.Errorf("sessionID = %q, want the live agent's own session sess-live", sessionID)
	}
	if promptCalls != 0 {
		t.Errorf("AgentPrompt calls = %d, want 0 (attaching must not re-send the initial prompt)", promptCalls)
	}
	for _, target := range waitTargets {
		if target != "live-pane" {
			t.Errorf("AgentWait target = %q, want the live pane %q, not the fresh pane we failed to launch in", target, "live-pane")
		}
	}
}

// TestLaunchAndPrompt_AgentNameTakenByUnrelatedWorktree_StillFails covers the
// flip side: a candidate cwd that does NOT match this iteration's own
// worktree is a genuine, unrelated name collision, not our own already-
// running launch — it must still hard-fail rather than silently attaching to
// someone else's pane.
func TestLaunchAndPrompt_AgentNameTakenByUnrelatedWorktree_StillFails(t *testing.T) {
	t.Parallel()
	d := Deps{
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{}, &herdr.AgentNameTakenError{
				Message:      "agent name iter-01 is already used; candidates: cwd=/repo/some-other-worktree status=Working",
				CandidateCwd: "/repo/some-other-worktree",
			}
		},
		AgentGet: func(target string) (herdr.Agent, error) {
			t.Fatalf("AgentGet called for an unrelated name collision, want a hard failure instead")
			return herdr.Agent{}, nil
		},
	}

	if _, err := launchAndPrompt(d, launchAndPromptParams{
		Label:      "iter-01",
		Agent:      AgentClaude,
		Pane:       "fresh-pane",
		Prompt:     "go",
		SessionCwd: "/repo/iter-01",
		Ticket:     "01",
	}); err == nil {
		t.Fatal("launchAndPrompt() error = nil, want a hard failure for an unrelated agent_name_taken collision")
	}
}

// TestLaunchAndPrompt_StuckSubmission_PropagatesAsErrStuckSubmission is a
// regression test for the fix-spinner/04 incident: when the initial prompt
// never reaches the pane at all (AgentPrompt's promptWithNudge wrapper
// exhausts its retypes and returns errStuckSubmission), launchAndPrompt's
// "sending initial prompt" wrap must still be unwrappable back to
// errStuckSubmission via errors.Is, so runIteration's caller-side retry
// logic can tell it apart from an ordinary launch failure.
func TestLaunchAndPrompt_StuckSubmission_PropagatesAsErrStuckSubmission(t *testing.T) {
	t.Parallel()
	d := Deps{
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			return herdr.Agent{}, errStuckSubmission
		},
	}

	_, err := launchAndPrompt(d, launchAndPromptParams{
		Label:      "iter-04",
		Agent:      AgentClaude,
		Pane:       "pane-4",
		Prompt:     "go",
		SessionCwd: "/repo/iter-04",
		Ticket:     "04",
	})
	if !errors.Is(err, errStuckSubmission) {
		t.Fatalf("launchAndPrompt() error = %v, want it to wrap errStuckSubmission", err)
	}
}

// TestLaunchAndPrompt_AttachToLiveAgent_StalledSinceLaunchSendsPrompt covers
// ticket 03: a collided reattach onto a pane that reports idle but never
// advanced past its own launch-time state_change_seq baseline (its first
// prompt-send stalled) must send/nudge a prompt instead of short-circuiting
// to "already finished".
func TestLaunchAndPrompt_AttachToLiveAgent_StalledSinceLaunchSendsPrompt(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicName := "fix-spinner"

	if err := logEvent(scratchDir, epicName, Event{
		Type:           eventIterationStarted,
		Ticket:         "03",
		AgentSession:   "sess-live",
		StateChangeSeq: 946,
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var promptCalls int
	sink := &recordingSinkWithArgs{occupancySink: &occupancySink{}}

	d := Deps{
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{}, &herdr.AgentNameTakenError{
				Message:      "agent name iter-01 is already used; candidates: cwd=/repo/iter-01 status=Idle",
				CandidateCwd: "/repo/iter-01",
			}
		},
		AgentGet: func(target string) (herdr.Agent, error) {
			return herdr.Agent{PaneID: "live-pane", AgentStatus: "idle", AgentSession: "sess-live", StateChangeSeq: 946}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			promptCalls++
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working", AgentSession: "sess-live"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		Sleep: func(time.Duration) {},
	}

	sessionID, err := launchAndPrompt(d, launchAndPromptParams{
		Label:      "iter-01",
		Agent:      AgentClaude,
		Pane:       "fresh-pane",
		Prompt:     "go",
		SessionCwd: "/repo/iter-01",
		Ticket:     "03",
		ScratchDir: scratchDir,
		EpicName:   epicName,
		StartEvent: eventIterationStarted,
		Sink:       sink,
	})
	if err != nil {
		t.Fatalf("launchAndPrompt: %v", err)
	}
	if sessionID != "sess-live" {
		t.Errorf("sessionID = %q, want sess-live", sessionID)
	}
	if promptCalls != 1 {
		t.Errorf("AgentPrompt calls = %d, want 1 (stalled-since-launch pane must be prompted)", promptCalls)
	}
}

// TestLaunchAndPrompt_AttachToLiveAgent_GenuinelyFinishedStaysFinished covers
// the flip side of ticket 03: a collided reattach onto a pane whose
// state_change_seq has advanced past its launch-time baseline and reports
// idle genuinely finished a turn — it must keep parking as "already
// finished", unchanged from today, with no extra prompt sent.
func TestLaunchAndPrompt_AttachToLiveAgent_GenuinelyFinishedStaysFinished(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicName := "fix-spinner"

	if err := logEvent(scratchDir, epicName, Event{
		Type:           eventIterationStarted,
		Ticket:         "03",
		AgentSession:   "sess-live",
		StateChangeSeq: 946,
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	var promptCalls int
	sink := &recordingSinkWithArgs{occupancySink: &occupancySink{}}

	d := Deps{
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{}, &herdr.AgentNameTakenError{
				Message:      "agent name iter-01 is already used; candidates: cwd=/repo/iter-01 status=Idle",
				CandidateCwd: "/repo/iter-01",
			}
		},
		AgentGet: func(target string) (herdr.Agent, error) {
			return herdr.Agent{PaneID: "live-pane", AgentStatus: "idle", AgentSession: "sess-live", StateChangeSeq: 950}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			promptCalls++
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working", AgentSession: "sess-live"}, nil
		},
		Sleep: func(time.Duration) {},
	}

	sessionID, err := launchAndPrompt(d, launchAndPromptParams{
		Label:      "iter-01",
		Agent:      AgentClaude,
		Pane:       "fresh-pane",
		Prompt:     "go",
		SessionCwd: "/repo/iter-01",
		Ticket:     "03",
		ScratchDir: scratchDir,
		EpicName:   epicName,
		StartEvent: eventIterationStarted,
		Sink:       sink,
	})
	if err != nil {
		t.Fatalf("launchAndPrompt: %v", err)
	}
	if sessionID != "sess-live" {
		t.Errorf("sessionID = %q, want sess-live", sessionID)
	}
	if promptCalls != 0 {
		t.Errorf("AgentPrompt calls = %d, want 0 (genuinely finished pane must not be re-prompted)", promptCalls)
	}
}

// recordingSinkWithArgs embeds occupancySink (itself embedding
// noopEventSink) and additionally hooks IterationStarted, for tests that
// need both start-time signals asserted together.
type recordingSinkWithArgs struct {
	*occupancySink
	onIterationStarted func(ticket tickets.Ticket, label, cwd, sessionID string)
}

func (s *recordingSinkWithArgs) IterationStarted(ticket tickets.Ticket, label, cwd, sessionID string) {
	if s.onIterationStarted != nil {
		s.onIterationStarted(ticket, label, cwd, sessionID)
	}
}
