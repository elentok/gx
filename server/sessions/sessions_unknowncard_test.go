package sessions_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestUnknownCommandCard(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name string
		card wire.SessionActionable
		kept bool
	}{
		{"unknown command survives Stop", wire.SessionActionable{Kind: wire.DeliveryUnknownCommand, Reason: "unknown", Action: []string{"/nope"}, CopyText: "/nope", GxRaised: true}, true},
		{"send failed is cleared", wire.SessionActionable{Kind: wire.DeliverySendFailed, Reason: "failed", Action: []string{"hi"}, CopyText: "hi", GxRaised: true}, false},
		{"may not have been sent is cleared", wire.SessionActionable{Kind: wire.DeliveryMaybeSent, Reason: "maybe", CopyText: "hi", GxRaised: true}, false},
		{"agent-raised card is cleared", wire.SessionActionable{Reason: "agent", Action: []string{"/review"}}, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			h, ctx, evs, id, _ := startFakeSession(t)
			meta := readMeta(t, h, id)
			meta["actionable"] = row.card
			data, err := json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(h.SessionsDir, id, "meta.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}
			waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == row.card.Reason })

			stopFakeSession(ctx, t, h, evs, id)
			got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Stopped })
			if kept := got.Actionable.Reason == row.card.Reason; kept != row.kept {
				t.Errorf("after Stop actionable = %+v, want kept=%v", got.Actionable, row.kept)
			}
			if got.FirstPromptPending {
				t.Errorf("first_prompt_pending after Stop, want false")
			}

			res, err := h.Client.Sessions.Resume(ctx, id)
			if err != nil || res.Refused || res.Session == nil {
				t.Fatalf("resume = %+v, %v", res, err)
			}
			if kept := res.Session.Actionable.Reason == row.card.Reason; kept != row.kept {
				t.Errorf("after Resume actionable = %+v, want kept=%v", res.Session.Actionable, row.kept)
			}
		})
	}
}
