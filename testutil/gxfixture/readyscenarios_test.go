package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
)

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
