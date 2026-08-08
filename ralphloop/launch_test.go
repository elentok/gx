package ralphloop

import (
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
)

// TestLaunchAndPrompt_IterationStartedCarriesCwdAndSessionIDPlusImmediateOccupancy
// covers ticket 02's two start-time requirements: IterationStarted must carry
// cwd/sessionID (so a consumer can resolve the transcript itself), and one
// extra immediate ContextOccupancy read/emit must fire right away, rather
// than waiting up to smartZonePollMs for the first poll tick to report it.
func TestLaunchAndPrompt_IterationStartedCarriesCwdAndSessionIDPlusImmediateOccupancy(t *testing.T) {
	var started struct {
		identifier, label, cwd, sessionID string
	}
	occSink := &occupancySink{}
	sink := &recordingSinkWithArgs{
		occupancySink: occSink,
		onIterationStarted: func(identifier, label, cwd, sessionID string) {
			started.identifier, started.label, started.cwd, started.sessionID = identifier, label, cwd, sessionID
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
	var startedSessionID string
	var observedSessionID string
	sink := &recordingSinkWithArgs{
		occupancySink: &occupancySink{},
		onIterationStarted: func(_, _, _, sessionID string) {
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
// not hard-fail the ticket to needs-attention — it attaches to the live
// pane (via AgentGet) and waits it out, without sending a second, redundant
// initial prompt.
func TestLaunchAndPrompt_AgentNameTakenByOwnWorktree_AttachesInsteadOfFailing(t *testing.T) {
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

// recordingSinkWithArgs embeds occupancySink (itself embedding
// noopEventSink) and additionally hooks IterationStarted, for tests that
// need both start-time signals asserted together.
type recordingSinkWithArgs struct {
	*occupancySink
	onIterationStarted func(identifier, label, cwd, sessionID string)
}

func (s *recordingSinkWithArgs) IterationStarted(identifier, label, cwd, sessionID string) {
	if s.onIterationStarted != nil {
		s.onIterationStarted(identifier, label, cwd, sessionID)
	}
}
