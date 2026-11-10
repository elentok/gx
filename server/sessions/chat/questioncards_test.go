package chat_test

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

func card(source, id string) wire.QuestionCard {
	return wire.QuestionCard{Source: source, ID: id, Questions: []wire.QuestionCardQuestion{}}
}

func TestBuildCards_LiveBlockerHidesTheRest(t *testing.T) {
	t.Parallel()
	all := []chat.CardSlot{chat.SlotLiveBlocker, chat.SlotTicketAsk, chat.SlotRound}
	src := chat.CardSources{
		chat.SlotLiveBlocker: {card("permission", "p")},
		chat.SlotTicketAsk:   {card("ticket-ask", "a")},
		chat.SlotRound:       {card("round", "r")},
	}
	if got := chat.PickCards(all, src); len(got) != 1 || got[0].Source != "permission" {
		t.Fatalf("with a blocker = %+v, want only it", got)
	}
	delete(src, chat.SlotLiveBlocker)
	got := chat.PickCards([]chat.CardSlot{chat.SlotTicketAsk, chat.SlotRound}, src)
	if len(got) != 2 || got[0].Source != "ticket-ask" || got[1].Source != "round" {
		t.Fatalf("no blocker = %+v, want ask then round", got)
	}
}

func TestBuildCards_OrderAndEmpty(t *testing.T) {
	t.Parallel()
	if got := chat.BuildCards(chat.ClassQueueAgent, chat.CardSources{chat.SlotRound: {card("round", "r")}}); got == nil || len(got) != 0 {
		t.Errorf("queue agent = %#v, want empty non-nil", got)
	}
	if got := chat.BuildCards(chat.ClassPlain, nil); got == nil || len(got) != 0 {
		t.Errorf("no sources = %#v, want empty non-nil", got)
	}
}

func TestRoundCard_IDFollowsTheRound(t *testing.T) {
	t.Parallel()
	a := []wire.SessionRoundCard{{Label: "1", Title: "Store", Kind: "options", Recommended: "a",
		Options: []wire.SessionRoundOption{{ID: "a", Label: "SQLite", Consequence: "fast"}},
		Links:   []wire.QuestionLink{{Label: "Mock", URL: "https://x"}}}, {Label: "2", Title: "Go?", Kind: "yes-no"}}
	b := []wire.SessionRoundCard{{Label: "1", Title: "Other", Kind: "text"}}
	ca, ca2, cb := chat.RoundCard(a), chat.RoundCard(a), chat.RoundCard(b)
	if !reflect.DeepEqual(ca, ca2) || ca[0].ID == cb[0].ID || ca[0].Source != wire.CardSourceRound {
		t.Fatalf("ids %q %q", ca[0].ID, cb[0].ID)
	}
	q := ca[0].Questions[0]
	if len(ca[0].Questions) != 2 || q.Kind != "options" || q.Recommended != "a" || q.Options[0].Consequence != "fast" || q.Links[0].URL != "https://x" {
		t.Errorf("question = %+v", q)
	}
	if chat.RoundCard(nil) != nil {
		t.Error("no round should give no card")
	}
}

func TestClassOf(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		m    chat.Meta
		want chat.SessionClass
	}{
		"plain": {chat.Meta{ID: "s1"}, chat.ClassPlain},
		"queue": {chat.Meta{ID: "queue-x"}, chat.ClassQueueAgent},
		"fix":   {chat.Meta{ID: "s2", FixOf: "gx:e/01"}, chat.ClassFixAgent},
	}
	for name, c := range cases {
		if got := chat.ClassOf(c.m); got != c.want {
			t.Errorf("%s: class = %v, want %v", name, got, c.want)
		}
	}
}
