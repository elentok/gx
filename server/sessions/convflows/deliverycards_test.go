package convflows_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// seedCard writes a card into the session's meta and waits for the snapshot to show it.
func seedCard(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan wire.Event, id string, a wire.SessionActionable) {
	t.Helper()
	meta := readMeta(t, h, id)
	meta["actionable"] = a
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.SessionsDir, id, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == a.Reason })
}

func TestDismiss(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{wire.DeliveryMaybeSent, wire.DeliverySendFailed, wire.DeliveryUnknownCommand} {
		t.Run("clears "+kind, func(t *testing.T) {
			t.Parallel()
			h, ctx, evs, id, _ := startFakeSession(t)
			seedCard(ctx, t, h, evs, id, wire.SessionActionable{Kind: kind, Reason: "card", CopyText: "x", GxRaised: true})
			res, err := h.Client.Sessions.Dismiss(ctx, id)
			if err != nil || res.Refused || res.Session == nil || res.Session.Actionable.Reason != "" {
				t.Fatalf("dismiss = %+v, %v; want the cleared session", res, err)
			}
			waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == "" })
			if again, err := h.Client.Sessions.Dismiss(ctx, id); err != nil || again.Reason != wire.ReasonSessionNoCard {
				t.Errorf("second dismiss = %+v, %v; want %s", again, err, wire.ReasonSessionNoCard)
			}
		})
	}

	other := map[string]wire.SessionActionable{
		"blocking draft":    {Reason: "draft", Blocking: wire.BlockingDraft, GxRaised: true},
		"blocking starting": {Reason: "starting", Blocking: wire.BlockingStarting, GxRaised: true},
		"derived design":    {Reason: "done", Action: []string{"/clear"}, Derived: true},
	}
	for name, card := range other {
		t.Run("refuses "+name, func(t *testing.T) {
			t.Parallel()
			h, ctx, evs, id, _ := startFakeSession(t)
			seedCard(ctx, t, h, evs, id, card)
			res, err := h.Client.Sessions.Dismiss(ctx, id)
			if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotDeliveryCard {
				t.Fatalf("dismiss = %+v, %v; want %s", res, err, wire.ReasonSessionNotDeliveryCard)
			}
			if s := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return true }); s.Actionable.Reason != card.Reason {
				t.Errorf("card = %q after refusal, want %q", s.Actionable.Reason, card.Reason)
			}
		})
	}

	t.Run("refuses an agent-raised card", func(t *testing.T) {
		t.Parallel()
		h, ctx, evs, id, conv := startFakeSession(t)
		idleWithAction(ctx, t, h, evs, id, conv, "do it")
		res, err := h.Client.Sessions.Dismiss(ctx, id)
		if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotDeliveryCard {
			t.Fatalf("dismiss = %+v, %v; want %s", res, err, wire.ReasonSessionNotDeliveryCard)
		}
	})

	t.Run("refuses a session with no card", func(t *testing.T) {
		t.Parallel()
		h, ctx, _, id, _ := startFakeSession(t)
		res, err := h.Client.Sessions.Dismiss(ctx, id)
		if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNoCard {
			t.Fatalf("dismiss = %+v, %v; want %s", res, err, wire.ReasonSessionNoCard)
		}
	})

	t.Run("refuses an unknown id", func(t *testing.T) {
		t.Parallel()
		h, ctx, _, _, _ := startFakeSession(t)
		res, err := h.Client.Sessions.Dismiss(ctx, "nope")
		if err != nil || !res.Refused || res.Reason != wire.ReasonSessionUnknown {
			t.Fatalf("dismiss = %+v, %v; want %s", res, err, wire.ReasonSessionUnknown)
		}
	})
}

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
