package webflows

import (
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

func TestAgentsInterrupted_EachChatHoldsOneSeparator(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-interrupted")
	defer r.Stop(t)
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("sessions = %d, want 4", len(list))
	}
	designs := 0
	harnesses := map[string]int{}
	for _, si := range list {
		if si.Design {
			designs++
		}
		harnesses[si.Harness]++
		var chat wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", si.ID, "--json")), &chat); err != nil {
			t.Fatal(err)
		}
		interrupted := 0
		for _, m := range chat.Messages {
			if m.Kind == transcript.ChatKindInterrupted {
				interrupted++
			}
			if m.Role == "person" && m.Text == "[Request interrupted by user]" {
				t.Errorf("session %s: the marker shows as a person message", si.ID)
			}
		}
		if interrupted != 1 {
			t.Errorf("session %s (%s): interrupted messages = %d, want 1", si.ID, si.Harness, interrupted)
		}
	}
	if designs != 2 {
		t.Errorf("linked design sessions = %d, want 2", designs)
	}
	if harnesses["codex"] != 2 {
		t.Errorf("sessions by harness = %v, want two codex", harnesses)
	}
}
