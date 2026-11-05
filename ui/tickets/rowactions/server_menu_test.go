package rowactions

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/server/wire"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// pausedAnswerTab is gx:alpha/01 paused for an answer, as the server's
// snapshot renders it, served by api.
func pausedAnswerTab(api tickettest.FakeServerAPI) fakeTab {
	vm := viewmodel.State{}.ApplySnapshot(wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "paused", RenderedStatus: gxtickets.StatusPausedAnswer.Word()},
	}})
	epic := tabapi.EpicsFromViewModel(vm, viewmodel.Scope{})[0]
	tab := serverTab("paused", api)
	tab.sel = Selection{OK: true, Epic: epic, Ticket: epic.Tickets[0]}
	return tab
}

// openMenu presses "m" and delivers the menu its cmd resolves.
func openMenu(t *testing.T, rm *Menus, tab Tab) {
	t.Helper()
	rm.HandleReady(requestTicketMenu(t, rm, tab), tab)
}

// Seam D: "enter" on a paused row issues the detail's first verb (resume here).
func TestResumeEnter_ResumesAPausedRow(t *testing.T) {
	var calls []string
	api := tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionResume)}
	tab := serverTab("open", api)
	// The wire carries no pause reason, so only the retired repair status tells
	// the client this pause is a repair.
	tab.sel.Ticket = gxtickets.Ticket{Path: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair)}

	cmd, handled := ResumeEnter(tab)
	if !handled || cmd == nil {
		t.Fatal("enter on a paused row issued nothing")
	}
	cmd()
	if want := []string{"resume gx:alpha/02"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestMenus_KeyOpensOnEveryStatus(t *testing.T) {
	for _, status := range []string{"open", "claimed", "done", "blocked", "draft", "paused"} {
		var rm Menus
		openMenu(t, &rm, serverTab(status, tickettest.FakeServerAPI{}))
		if !rm.Menu.IsOpen {
			t.Errorf("status %s: menu closed, want open", status)
		}
	}
}

// Seam D: on herdr, Go to pane jumps to the tab from the iterations read, and
// Resume issues resume.
func TestMenus_GoToPaneAndResume(t *testing.T) {
	focused := fakeFocus(t)
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)

	var calls []string
	api := tickettest.FakeServerAPI{
		Calls: &calls, DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume),
		IterationsRes: []wire.IterationInfo{{Address: "gx:alpha/01", Pane: "p1", Tab: "t1"}},
	}
	tab := pausedAnswerTab(api)
	var rm Menus
	ready := requestTicketMenu(t, &rm, tab)

	pickTicketMenuItem(t, tab, ready, "Go to pane")()
	if !slices.Equal(*focused, []string{"t1"}) {
		t.Fatalf("focused = %v", *focused)
	}

	cmd := pickTicketMenuItem(t, tab, ready, resumeLabel)
	if msg, ok := cmd().(ServerWriteMsg); !ok || msg.Verb != "resume" {
		t.Fatalf("resume result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"resume gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

// A paused (answer) ticket's menu has no Answer… action: answering is the web
// UI's job, and the TUI only offers Resume.
func TestMenus_PausedAnswerMenuHasNoAnswerAction(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	var rm Menus
	openMenu(t, &rm, pausedAnswerTab(tickettest.FakeServerAPI{DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume)}))
	view := rm.Menu.View()
	if strings.Contains(view, "Answer…") {
		t.Fatalf("menu offers Answer…:\n%s", view)
	}
	if !strings.Contains(view, resumeLabel) {
		t.Fatalf("menu lacks %s:\n%s", resumeLabel, view)
	}
}
