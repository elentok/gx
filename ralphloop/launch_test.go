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
