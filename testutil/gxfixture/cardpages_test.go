package main

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/bgtx"
	"github.com/elentok/gx/tickets"
)

func TestCardPages(t *testing.T) {
	t.Parallel()
	r := start(t, "card-pages")

	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	var links, asking []string
	for _, si := range list {
		var link string
		switch {
		case si.Flow == string(tickets.FlowDesign):
			link = "design " + si.Ticket
		case si.Flow == string(tickets.FlowTriage):
			link = "triage " + si.FlowKey
		case si.Ticket == "" && si.Flow == "":
			link = "none"
		default:
			link = "unexpected " + si.Flow + " " + si.Ticket
		}
		links = append(links, link)

		if si.Status.State != wire.SessionWorking {
			t.Errorf("%s: state = %q, want working", link, si.Status.State)
		}
		var chat wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", si.ID, "--json")), &chat); err != nil {
			t.Fatal(err)
		}
		tasks := map[string]bool{}
		for _, task := range chat.Tasks {
			tasks[task.Label] = task.Finished
		}
		if len(chat.Tasks) != 2 || tasks[bgtx.SubBackgroundDesc] || !tasks[bgtx.SubFinishedDesc] {
			t.Errorf("%s: tasks = %+v, want %q running and %q finished", link, chat.Tasks, bgtx.SubBackgroundDesc, bgtx.SubFinishedDesc)
		}
		if len(chat.Questions) > 0 {
			asking = append(asking, link)
		}
	}

	slices.Sort(links)
	if want := []string{"design demo:design-epic", "none", "triage demo"}; !slices.Equal(links, want) {
		t.Errorf("links = %q, want %q", links, want)
	}
	if want := []string{"triage demo"}; !slices.Equal(asking, want) {
		t.Errorf("sessions with questions = %q, want %q", asking, want)
	}
}
