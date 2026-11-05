package rowactions

import (
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

func TestTicketMenuItems_DimReasons(t *testing.T) {
	disabled := func(in ticketMenuInput, label string) string {
		return itemByLabel(t, ticketMenuItems(in), label).Disabled
	}
	draft := tickets.Ticket{Status: "draft"}

	if got := disabled(ticketMenuInput{rendered: tickets.StatusDraft, ticket: draft}, "Set done"); got != doneIsSetByServer {
		t.Errorf("loop-epic set done = %q", got)
	}
	if got := disabled(ticketMenuInput{rendered: tickets.StatusDraft, ticket: draft, isMap: true}, "Set done"); got != "" {
		t.Errorf("map-epic set done = %q, want enabled", got)
	}
	if got := disabled(ticketMenuInput{rendered: tickets.StatusDraft, ticket: draft}, "Set draft"); got != "already draft" {
		t.Errorf("set draft on a draft = %q", got)
	}

	resumeDetail := detailWith(wire.ActionResume)
	done := ticketMenuInput{rendered: tickets.StatusDone, ticket: tickets.Ticket{Status: "done"}, isMap: true, detail: &resumeDetail}
	for _, label := range []string{"Pause", "Cancel", resumeLabel, "Unmute & Reopen"} {
		if got := disabled(done, label); got != ticketFinished {
			t.Errorf("%s on a done ticket = %q, want %q", label, got, ticketFinished)
		}
	}
	done.serverReason = ServerNotRunning
	if got := disabled(done, "Pause"); got != ServerNotRunning {
		t.Errorf("pause with no server on a done ticket = %q, want the server reason first", got)
	}

	open := ticketMenuInput{rendered: tickets.StatusOpen, ticket: tickets.Ticket{Status: "open"}}
	if got := disabled(open, "Unmute & Reopen"); got != notMuted {
		t.Errorf("unmute = %q, want %q", got, notMuted)
	}

	paused := ticketMenuInput{rendered: tickets.StatusPausedRepair, ticket: tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: schema.PauseManual}, herdrDown: true, detail: &resumeDetail}
	if got := disabled(paused, "Pause"); got != alreadyPaused {
		t.Errorf("pause on a paused ticket = %q", got)
	}
	if got := disabled(paused, resumeLabel); got != "" {
		t.Errorf("resume on a paused ticket = %q, want enabled", got)
	}
	if got := disabled(paused, "Investigate"); got != "" {
		t.Errorf("investigate with herdr down = %q, want enabled", got)
	}
}

func TestTicketMenuItems_StatusPickCarriesTheStatus(t *testing.T) {
	item := itemByLabel(t, ticketMenuItems(ticketMenuInput{rendered: tickets.StatusOpen}), "Set draft")
	target := TicketTarget{Address: "gx:alpha/01"}
	if got := item.Run(target)(); got != (TicketPickMsg{Target: target, Status: schema.StatusDraft}) {
		t.Errorf("pick = %+v", got)
	}
}

func TestTicketMenuItems_ArchivedKeepsReadActions(t *testing.T) {
	items := ticketMenuItems(ticketMenuInput{
		rendered: tickets.StatusPausedRepair,
		ticket:   tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: schema.PauseSpinning},
		archived: true,
	})
	reads := []string{ActionWatch, ActionTranscript, actionYankSummary, actionYankFilePath}
	for _, e := range ticketMenuEdits {
		reads = append(reads, e.action)
	}
	var enabled []string
	for _, it := range items {
		if it.Disabled != "" {
			continue
		}
		msg, _ := it.Run(TicketTarget{})().(TicketPickMsg)
		if !slices.Contains(reads, msg.Action) {
			t.Errorf("%s (action %q) enabled on an archived row", it.Label, msg.Action)
		}
		enabled = append(enabled, msg.Action)
	}
	slices.Sort(enabled)
	slices.Sort(reads)
	if !slices.Equal(enabled, reads) {
		t.Errorf("enabled actions = %v, want %v", enabled, reads)
	}
}

func TestTicketMenuItems_ResumeEnterHintOnlyWhenEnterResumes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		reason   schema.PauseReason
		rendered tickets.RenderedStatus
		wantKey  string
	}{
		{"paused (answer)", "", tickets.StatusPausedAnswer, ""},
		{"paused (repair)", schema.PauseManual, tickets.StatusPausedRepair, "enter"},
	} {
		d := detailWith(wire.ActionResume)
		items := ticketMenuItems(ticketMenuInput{
			rendered: tt.rendered,
			ticket:   tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: tt.reason},
			detail:   &d,
		})
		if got := itemByLabel(t, items, resumeLabel).Key; got != tt.wantKey {
			t.Errorf("%s: Resume key = %q, want %q", tt.name, got, tt.wantKey)
		}
	}
}

func TestTicketMenuItems_GoToPane(t *testing.T) {
	if hasItem(ticketMenuItems(ticketMenuInput{}), "Go to pane") {
		t.Error("Go to pane shown without a herdr pane")
	}
	for _, tt := range []struct {
		name string
		in   ticketMenuInput
		want string
	}{
		{"live pane", ticketMenuInput{paneLive: true}, ""},
		{"herdr down", ticketMenuInput{paneLive: true, herdrDown: true}, herdrDownText},
		{"pty live", ticketMenuInput{queueAgent: "queue-a", ptyLive: true}, ""},
		{"pty live, herdr down", ticketMenuInput{queueAgent: "queue-a", ptyLive: true, herdrDown: true}, ""},
		{"pty holder gone", ticketMenuInput{queueAgent: "queue-a"}, holderGoneText},
	} {
		item := itemByLabel(t, ticketMenuItems(tt.in), "Go to pane")
		if item.Disabled != tt.want || item.Group != contextmenu.Agent {
			t.Errorf("%s: Go to pane = %q in %v, want %q in Agent", tt.name, item.Disabled, item.Group, tt.want)
		}
	}
}

func TestTicketMenuItems_GoToPanePTYOpensTheAgentsRow(t *testing.T) {
	item := itemByLabel(t, ticketMenuItems(ticketMenuInput{queueAgent: "queue-a", ptyLive: true}), "Go to pane")
	if got := item.Run(TicketTarget{})(); got != (tabapi.GoToSessionMsg{ID: "queue-a"}) {
		t.Fatalf("Go to pane sent %v, want tabapi.GoToSessionMsg{queue-a}", got)
	}
}

func TestCmdTicketMenu_FetchesIterationsOnce(t *testing.T) {
	calls := 0
	api := tickettest.CountingIterationsAPI{FakeReads: tickettest.FakeReads{}, Calls: &calls}
	in := newTicketMenuInput(tickets.Epic{}, tickets.Ticket{Path: "gx:alpha/01", Title: "First"}, false, "")

	cmdTicketMenu(api, config.Dirs{}, in)()

	if calls != 1 {
		t.Errorf("Iterations calls = %d, want 1", calls)
	}
}

func TestCmdTicketMenu_PTYIterationOffersTheAgentRow(t *testing.T) {
	api := tickettest.FakeServerAPI{IterationsRes: []wire.IterationInfo{{Address: "gx:alpha/01", Runner: "pty", QueueAgent: "queue-gx-alpha-01"}}}
	in := newTicketMenuInput(tickets.Epic{}, tickets.Ticket{Path: "gx:alpha/01"}, true, "")
	msg := cmdTicketMenu(tickettest.FakeReads{FakeServerAPI: api}, config.Dirs{}, in)().(TicketReadyMsg)
	item := itemByLabel(t, msg.Items, "Go to pane")
	if item.Disabled != "" {
		t.Fatalf("Go to pane dimmed %q with a live PTY holder and herdr down", item.Disabled)
	}
}
