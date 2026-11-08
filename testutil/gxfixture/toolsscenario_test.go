package main

import (
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

func TestToolsScenario_ChatListsTheCallsInFlight(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-tools")
	list := r.seededSessions(t)
	if len(list) != 3 {
		t.Fatalf("%d sessions, want 3", len(list))
	}
	for i, s := range list {
		if s.Status.State != wire.SessionWorking {
			t.Errorf("session %d state = %s, want working", i, s.Status.State)
		}
	}
	if !list[2].Design {
		t.Errorf("third session is not a design session: %+v", list[2])
	}

	// A single call is Current alone; the newest of several is Current and the
	// older ones follow newest first.
	for i, wantOthers := range []int{0, 2, 0} {
		chat := r.waitChat(t, list[i].ID, func(c wire.SessionChatResponse) bool { return c.Current != nil })
		if chat.Current.Kind != transcript.InFlightShell && wantOthers == 0 {
			t.Errorf("session %d current = %+v, want the shell call", i, chat.Current)
		}
		if len(chat.Others) != wantOthers {
			t.Errorf("session %d others = %+v, want %d", i, chat.Others, wantOthers)
		}
		if wantOthers > 0 && (chat.Current.Kind != transcript.InFlightSearch || chat.Others[0].Kind != transcript.InFlightAgent || chat.Others[1].Kind != transcript.InFlightShell) {
			t.Errorf("session %d current %+v others %+v, want search, then agent, shell", i, chat.Current, chat.Others)
		}
	}
}
