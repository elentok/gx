package designflows

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func TestTriageScenario_OldEpicsRouteListsTheMovesButNotTheOthers(t *testing.T) {
	t.Parallel()
	r := startWith(t, "triage", nil, "--shape", "old-epics")
	var v wire.TriageView
	if !testutil.Poll(60*time.Second, 500*time.Millisecond, func() bool {
		if err := json.Unmarshal([]byte(r.Gx(t, "server", "triage", "show", "gx", "--json")), &v); err != nil {
			t.Fatal(err)
		}
		return v.Summary != nil && len(v.Summary.Moves) > 0
	}) {
		t.Fatalf("the Route saved no old-epic move: step %q, summary %+v", currentStep(v), v.Summary)
	}
	got := map[string]string{}
	for _, m := range v.Summary.Moves {
		got[m.OldEpic] = m.Area
	}
	if len(v.Summary.Waiting) != 1 || !strings.Contains(v.Summary.Waiting[0], "stuck-ticket-watchdog") {
		t.Errorf("waiting = %v, want the epic holding a claimed ticket", v.Summary.Waiting)
	}
	want := map[string]string{"test-flakes": "test-health", "agent-guidance": "agent-guidance-and-skills", "ticket-authoring": "ticket-authoring"}
	for epic, area := range want {
		if got[epic] != area {
			t.Errorf("move of %s = %q, want %q", epic, got[epic], area)
		}
	}
	for _, never := range []string{"tickets-cli", "gx-check", "stuck-ticket-watchdog"} {
		if _, ok := got[never]; ok {
			t.Errorf("%s has a move unit", never)
		}
	}
}

func TestTriageScenario_BacklogStartDesignChatReprintsOnceThenMovesTheTickets(t *testing.T) {
	t.Parallel()
	r := startWith(t, "triage", nil, "--shape", "backlog")
	var res wire.SessionResult
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "start-design", project+":sync", "--json")), &res); err != nil || res.Session == nil {
		t.Fatalf("start-design = %+v, %v", res, err)
	}
	id := res.Session.ID
	cards := func() int {
		var chat wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", id, "--json")), &chat); err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, text := range agentTexts(chat) {
			n += strings.Count(text, "Start this Design?")
		}
		return n
	}
	waitCards := func(want int) {
		if !testutil.Poll(30*time.Second, 300*time.Millisecond, func() bool { return cards() == want }) {
			var chat wire.SessionChatResponse
			_ = json.Unmarshal([]byte(r.Gx(t, "agents", "chat", id, "--json")), &chat)
			t.Fatalf("cards printed = %d, want %d; messages %+v; texts %q", cards(), want, chat.Messages, agentTexts(chat))
		}
	}
	waitCards(1)
	r.promptWhenIdle(t, id, "drop 1")
	waitCards(2)
	r.promptWhenIdle(t, id, "yes")
	if !testutil.Poll(30*time.Second, 300*time.Millisecond, func() bool {
		for _, e := range r.epicList(t).Epics {
			if e.Project == project && e.Name == "sync-cleanup" {
				return e.Total == 4 // the five open tickets less the dropped one
			}
		}
		return false
	}) {
		t.Fatal("the yes created no map")
	}
}
