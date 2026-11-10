package chat_test

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/ticketlog"
)

func card(source, id string) wire.QuestionCard {
	return wire.QuestionCard{Source: source, ID: id, Questions: []wire.QuestionCardQuestion{}}
}

func TestBuildCards_LiveBlockerHidesTheRest(t *testing.T) {
	t.Parallel()
	src := chat.CardSources{
		chat.SlotLiveBlocker: {card("permission", "p")},
		chat.SlotTicketAsk:   {card("ticket-ask", "a")},
		chat.SlotRound:       {card("round", "r")},
	}
	if got := chat.BuildCards(chat.ClassFlow, src); len(got) != 1 || got[0].Source != "permission" {
		t.Fatalf("with a blocker = %+v, want only it", got)
	}
	delete(src, chat.SlotLiveBlocker)
	got := chat.BuildCards(chat.ClassFlow, src)
	if len(got) != 2 || got[0].Source != "ticket-ask" || got[1].Source != "round" {
		t.Fatalf("no blocker = %+v, want ask then round", got)
	}
}

func TestBuildCards_DialogAndCodexQuestionAreLiveBlockers(t *testing.T) {
	t.Parallel()
	rest := chat.CardSources{
		chat.SlotTicketAsk: {card(wire.CardSourceTicketAsk, "a")},
		chat.SlotRound:     {card(wire.CardSourceRound, "r")},
	}
	for slot, source := range map[chat.CardSlot]string{
		chat.SlotClaudeDialog:  wire.CardSourceClaudeDialog,
		chat.SlotCodexQuestion: wire.CardSourceCodex,
	} {
		src := chat.CardSources{slot: {card(source, "x")}}
		for k, v := range rest {
			src[k] = v
		}
		if got := chat.BuildCards(chat.ClassPlain, src); len(got) != 1 || got[0].Source != source {
			t.Errorf("%s with ask and round = %+v, want only it", source, got)
		}
		delete(src, slot)
		if got := chat.BuildCards(chat.ClassPlain, src); len(got) != 2 {
			t.Errorf("%s gone = %+v, want ask and round", source, got)
		}
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

func TestTicketAskCard_QueueAgentListsOnlyTheAsk(t *testing.T) {
	t.Parallel()
	q := ticketlog.Question{
		Summary: "Ship it?", Kind: ticketlog.QuestionOptions, Detail: "more", Recommended: "a",
		Options: []ticketlog.QuestionOption{{ID: "a", Label: "A", Consequence: "fast"}},
		Links:   []ticketlog.QuestionLink{{Label: "Mock", URL: "https://x"}},
	}
	ask := chat.TicketAskCard("gx:e/01", q, "3:answer")
	got := chat.BuildCards(chat.ClassQueueAgent, chat.CardSources{
		chat.SlotLiveBlocker: {card("permission", "p")}, chat.SlotTicketAsk: ask, chat.SlotRound: {card("round", "r")},
	})
	if len(got) != 1 || got[0].Source != wire.CardSourceTicketAsk || got[0].ID != "gx:e/01#3:answer" {
		t.Fatalf("queue agent cards = %+v, want only the ask", got)
	}
	cq := got[0].Questions[0]
	if cq.Title != "Ship it?" || cq.Body != "more" || cq.Recommended != "a" || cq.Options[0].Consequence != "fast" || cq.Links[0].URL != "https://x" {
		t.Errorf("question = %+v", cq)
	}
	addr, state, ok := chat.SplitTicketAskID(got[0].ID)
	if !ok || addr != "gx:e/01" || state != "3:answer" {
		t.Errorf("split = %q %q %v", addr, state, ok)
	}
	fix := chat.BuildCards(chat.ClassFixAgent, chat.CardSources{chat.SlotTicketAsk: ask, chat.SlotRound: {card("round", "r")}})
	if len(fix) != 2 || fix[0].Source != wire.CardSourceTicketAsk {
		t.Errorf("fix agent cards = %+v, want ask then round", fix)
	}
}

func TestClaudeDialogCard(t *testing.T) {
	t.Parallel()
	req := wire.AskHookRequest{Session: "s1", ToolUseID: "toolu_9", Questions: []wire.AskHookQuestion{
		{Header: "Scope", Question: "Which scope?", Options: []wire.AskHookOption{{Label: "A", Description: "first"}, {Label: "B", Description: "second"}}},
		{Header: "Extras", Question: "Which extras?", MultiSelect: true, Options: []wire.AskHookOption{{Label: "X", Description: "x desc"}}},
		{Header: "Ship", Question: "Ship it?", Options: []wire.AskHookOption{{Label: "Yes"}}},
	}}
	got := chat.ClaudeDialogCard(req)
	if len(got) != 1 || got[0].Source != wire.CardSourceClaudeDialog || got[0].ID != "toolu_9" || len(got[0].Questions) != 3 {
		t.Fatalf("card = %+v", got)
	}
	other := wire.QuestionCardOption{ID: "other", Label: "Other", Text: true}
	want := []wire.QuestionCardQuestion{
		{Kind: "options", Label: "Scope", Title: "Which scope?", Options: []wire.QuestionCardOption{{ID: "1", Label: "A", Description: "first"}, {ID: "2", Label: "B", Description: "second"}, other}},
		{Kind: "multi-select", Label: "Extras", Title: "Which extras?", Options: []wire.QuestionCardOption{{ID: "1", Label: "X", Description: "x desc"}, other}},
		{Kind: "options", Label: "Ship", Title: "Ship it?", Options: []wire.QuestionCardOption{{ID: "1", Label: "Yes"}, other}},
	}
	if !reflect.DeepEqual(got[0].Questions, want) {
		t.Errorf("questions = %+v, want %+v", got[0].Questions, want)
	}

	src := chat.CardSources{chat.SlotClaudeDialog: got}
	for class, wantN := range map[chat.SessionClass]int{
		chat.ClassPlain: 1, chat.ClassFlow: 0, chat.ClassQueueAgent: 0, chat.ClassFixAgent: 0,
	} {
		if n := len(chat.BuildCards(class, src)); n != wantN {
			t.Errorf("class %v: %d cards, want %d", class, n, wantN)
		}
	}
}

func withoutSlots(src chat.CardSources, slots ...chat.CardSlot) chat.CardSources {
	out := chat.CardSources{}
	for k, v := range src {
		out[k] = v
	}
	for _, s := range slots {
		delete(out, s)
	}
	return out
}

func TestBuildCards_ClassTableAcrossSources(t *testing.T) {
	t.Parallel()
	all := chat.CardSources{
		chat.SlotLiveBlocker:   {card(wire.CardSourcePermission, "p")},
		chat.SlotClaudeDialog:  {card(wire.CardSourceClaudeDialog, "d")},
		chat.SlotCodexQuestion: {card(wire.CardSourceCodex, "c")},
		chat.SlotTicketAsk:     {card(wire.CardSourceTicketAsk, "a")},
		chat.SlotRound:         {card(wire.CardSourceRound, "r")},
	}
	noBlocker := chat.CardSources{}
	for k, v := range all {
		if k != chat.SlotLiveBlocker {
			noBlocker[k] = v
		}
	}
	sources := func(cards []wire.QuestionCard) []string {
		var out []string
		for _, c := range cards {
			out = append(out, c.Source)
		}
		return out
	}
	cases := []struct {
		name  string
		class chat.SessionClass
		src   chat.CardSources
		want  []string
	}{
		{"plain with a blocker", chat.ClassPlain, all, []string{wire.CardSourcePermission}},
		{"plain with a dialog", chat.ClassPlain, noBlocker, []string{wire.CardSourceClaudeDialog}},
		{"plain with a codex question", chat.ClassPlain, withoutSlots(noBlocker, chat.SlotClaudeDialog), []string{wire.CardSourceCodex}},
		{"plain without", chat.ClassPlain, withoutSlots(noBlocker, chat.SlotClaudeDialog, chat.SlotCodexQuestion), []string{wire.CardSourceTicketAsk, wire.CardSourceRound}},
		{"flow with a blocker", chat.ClassFlow, all, []string{wire.CardSourcePermission}},
		{"flow without", chat.ClassFlow, noBlocker, []string{wire.CardSourceTicketAsk, wire.CardSourceRound}},
		{"queue", chat.ClassQueueAgent, all, []string{wire.CardSourceTicketAsk}},
		{"fix with a blocker", chat.ClassFixAgent, all, []string{wire.CardSourcePermission}},
		{"fix without", chat.ClassFixAgent, noBlocker, []string{wire.CardSourceTicketAsk, wire.CardSourceRound}},
	}
	for _, c := range cases {
		if got := sources(chat.BuildCards(c.class, c.src)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: sources = %v, want %v", c.name, got, c.want)
		}
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
