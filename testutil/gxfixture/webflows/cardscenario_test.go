package webflows

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/catalog"
)

// TestDeliverCards: each chat of deliver-cards ends in its card Kind after its control and send.
func TestDeliverCards(t *testing.T) {
	t.Parallel()
	r := start(t, "deliver-cards")
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil || len(list) != len(catalog.DeliverCardChats) {
		t.Fatalf("sessions = %d, %v, want %d", len(list), err, len(catalog.DeliverCardChats))
	}
	rows := []struct {
		chat    string
		control string
		send    string
		kind    string
	}{
		{catalog.DeliverCardChats[0], agentfake.ActionUndecided, "/model opus", wire.DeliveryMaybeSent},
		{catalog.DeliverCardChats[1], agentfake.ActionStall, "do the thing", wire.DeliverySendFailed},
		{catalog.DeliverCardChats[2], agentfake.ActionUnknownSkill, "/my-skill", wire.DeliveryUnknownCommand},
	}
	// The sends run side by side: each waits out the grace or the settle on its own chat.
	t.Run("sends", func(t *testing.T) {
		for _, row := range rows {
			id, conv := r.chatNamed(t, list, row.chat)
			t.Run(row.kind, func(t *testing.T) {
				t.Parallel()
				if err := agentfake.SendControl(agentfake.TUIControlPath(r.Home(), conv), agentfake.Control{Action: row.control}); err != nil {
					t.Fatal(err)
				}
				out := r.Gx(t, "agents", "prompt", id, row.send)
				var got wire.SessionActionable
				if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
					var now []wire.SessionInfo
					if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &now); err != nil {
						return false
					}
					i := slices.IndexFunc(now, func(s wire.SessionInfo) bool { return s.ID == id })
					if i < 0 {
						return false
					}
					got = now[i].Actionable
					return got.Kind == row.kind
				}) {
					t.Errorf("chat %q never raised %s, last actionable %+v, prompt said %q", row.chat, row.kind, got, out)
				}
			})
		}
	})
	r.Stop(t)
}

// chatNamed returns the session id and conversation id of the chat whose first prompt is name.
func (r *running) chatNamed(t *testing.T, list []wire.SessionInfo, name string) (id, conv string) {
	t.Helper()
	for _, s := range list {
		var c wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", s.ID, "--json")), &c); err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(c.Messages, func(m wire.SessionChatMessage) bool { return strings.Contains(m.Text, name) }) {
			return s.ID, c.ConversationID
		}
	}
	t.Fatalf("no chat holds %q", name)
	return "", ""
}
