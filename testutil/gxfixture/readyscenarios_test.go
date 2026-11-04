package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil"
)

const (
	readyTicket  = "demo:ready-epic/01"
	missingReady = "ready signal missing, sending the launch prompt anyway"
)

func (r *running) serverLogHas(t *testing.T, text string) bool {
	t.Helper()
	return strings.Contains(r.gx(t, "server", "logs"), text)
}

func TestRejectedFirstPromptScenario_TheSessionShowsAnUnknownCommandFailure(t *testing.T) {
	t.Parallel()
	r := start(t, "rejected-first-prompt")
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("sessions = %d, want 1", len(list))
	}
	got := list[0].Actionable
	if !got.GxRaised || !strings.Contains(got.Reason, "Unknown command:") {
		t.Errorf("actionable = %+v, want a gx-raised Unknown command reason", got)
	}
	r.stop(t)
}

func TestReadyDelayedScenario_TheLaunchPromptWaitsForTheReadySignal(t *testing.T) {
	t.Parallel()
	r := start(t, "ready-delayed")
	r.waitStatus(t, readyTicket, "done")
	if r.serverLogHas(t, missingReady) {
		t.Errorf("server log has %q, but the ready signal came in time", missingReady)
	}
	r.stop(t)
}

func TestReadyNeverScenario_TheLaunchPromptIsSentWhenTheWaitRunsOut(t *testing.T) {
	t.Parallel()
	r := start(t, "ready-never")
	r.waitStatus(t, readyTicket, "done")
	if !testutil.Poll(10*time.Second, 200*time.Millisecond, func() bool { return r.serverLogHas(t, missingReady) }) {
		t.Fatalf("server log never says %q:\n%s", missingReady, r.gx(t, "server", "logs"))
	}
	r.stop(t)
}
