package server_test

import (
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets/schema"
)

// A blocked pause lifts when the agent leaves blocked, on every runner, with the
// same session and no second claude.
func TestBlockedPause_LiftsWhenAgentLeavesBlocked(t *testing.T) {
	t.Parallel()
	exits := map[string]agentrunner.State{"working": agentrunner.StateWorking, "idle": agentrunner.StateIdle}
	for _, runner := range []string{"herdr", "pty", "codex"} {
		for name, state := range exits {
			t.Run(runner+"/"+name, func(t *testing.T) {
				h, ticketPath, turn := startBlockable(t, runner)
				turn.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
				waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
				waitFor(t, func() bool { return runsJSONHas(h, `"waiting_since"`) })

				turn.SetState("epic-a-iter-01", state, "")
				waitStatus(t, ticketPath, schema.StatusClaimed)
				waitFor(t, func() bool { return !runsJSONHas(h, `"waiting_since"`) })
				if _, ok, err := turn.Find("epic-a-iter-01"); err != nil || !ok {
					t.Errorf("session not live: ok=%v err=%v", ok, err)
				}
			})
		}
	}
}
