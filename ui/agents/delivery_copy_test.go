package agents

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
)

// withCard sets a delivery card of kind on session id.
func withCard(m Model, id, kind, text string, action ...string) Model {
	return withSession(m, id, func(s *wire.SessionInfo) {
		s.Actionable = wire.SessionActionable{Kind: kind, Reason: "reason", Action: action, CopyText: text, GxRaised: true}
	})
}

func TestDeliveryCopy_LeaderYCopiesTheFullText(t *testing.T) {
	for _, kind := range []string{wire.DeliveryMaybeSent, wire.DeliverySendFailed, wire.DeliveryUnknownCommand} {
		t.Run(kind, func(t *testing.T) {
			m := withCard(actionModel(t, &fakeSessionClient{}, "live"), "live", kind, "/model opus\nline two")
			var got []string
			m.clipWrite = func(s string) error { got = append(got, s); return nil }
			_, cmd := leaderThen(t, m, letter("y"))
			if cmd != nil {
				cmd()
			}
			if !slices.Equal(got, []string{"/model opus\nline two"}) {
				t.Fatalf("clipboard = %q", got)
			}
		})
	}
}

func TestDeliveryCopy_LeaderYWithoutACardCopiesNothing(t *testing.T) {
	m := actionModel(t, &fakeSessionClient{}, "live")
	m.clipWrite = func(s string) error { t.Fatalf("copied %q", s); return nil }
	_, cmd := leaderThen(t, m, letter("y"))
	if cmd == nil {
		t.Fatal("no notice shown")
	}
	cmd()
}

func TestDeliveryCopy_ActionOnMaybeSentSaysNoActionAndCallsNothing(t *testing.T) {
	f := &fakeSessionClient{}
	m := withCard(actionModel(t, f, "live"), "live", wire.DeliveryMaybeSent, "/model opus")
	_, note := act(t, m, letter("a"))
	if note != "no action" {
		t.Fatalf("footer = %q", note)
	}
	if got := calls(f); len(got) != 0 {
		t.Fatalf("calls = %v", got)
	}
}

func TestDeliveryCopy_ActionOnSendFailedConfirmsListingTheText(t *testing.T) {
	f := &fakeSessionClient{}
	m := withCard(actionModel(t, f, "live"), "live", wire.DeliverySendFailed, "/model opus", "/model opus")
	m, _ = act(t, m, letter("a"))
	if view := plainView(m); !strings.Contains(view, "/model opus") {
		t.Fatalf("confirm lacks the text:\n%s", view)
	}
	act(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := calls(f); !slices.Equal(got, []string{"action live"}) {
		t.Fatalf("calls = %v", got)
	}
}
