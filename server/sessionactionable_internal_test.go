package server

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/agentfake"
)

// The fake agent and the server agree on the run-end text and the design
// prompt prefix only by convention. This fails when either side drifts.
func TestRunEndLine_MatchesAgentfakeDesignRunEnd(t *testing.T) {
	got, ok := runEndNext("Thanks, that settles it.\n\n"+agentfake.DesignRunEnd, "gx:epic/03", false)
	if !ok {
		t.Fatalf("runEndLine does not match agentfake.DesignRunEnd %q", agentfake.DesignRunEnd)
	}
	if got.Reason != agentfake.DesignRunEnd {
		t.Errorf("Reason = %q, want %q", got.Reason, agentfake.DesignRunEnd)
	}
}

func TestDesignContinueAction_UsesAgentfakeDesignPromptPrefix(t *testing.T) {
	action := designContinueAction("gx:epic/03")
	if len(action) != 2 || !strings.HasPrefix(action[1], agentfake.DesignPromptPrefix) {
		t.Errorf("action = %q, want second entry to start with %q", action, agentfake.DesignPromptPrefix)
	}
}
