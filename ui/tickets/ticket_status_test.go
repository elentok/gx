package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
)

func TestTicketStatus_ServerWordWinsOverComputed(t *testing.T) {
	info := server.TicketInfo{
		Address: "p:e/01", Title: "one", Status: "open", RenderedStatus: "waiting-for-children",
	}
	tk := ticketFromInfo(info, "01")
	epic := gxtickets.Epic{Name: "p:e", Tickets: []gxtickets.Ticket{tk}}

	if got := epic.RenderedStatus(tk); got != gxtickets.StatusOpen {
		t.Fatalf("computed status = %v, want open (the test needs the two to differ)", got)
	}
	if got := ticketStatus(epic, tk); got != gxtickets.StatusWaitingForChildren {
		t.Errorf("ticketStatus = %v, want waiting-for-children", got)
	}
	out, _, _ := renderTicketPreview(epic, tk, 80)
	if !strings.Contains(out, "waiting-for-children") {
		t.Errorf("preview does not show the server's status word:\n%s", out)
	}
}

func TestTicketStatus_FallsBackToComputedWithoutServerWord(t *testing.T) {
	tk := gxtickets.Ticket{Number: 1, Identifier: "01", Path: "p:e/01", Status: "open"}
	epic := gxtickets.Epic{Name: "p:e", Tickets: []gxtickets.Ticket{tk}}
	if got := ticketStatus(epic, tk); got != gxtickets.StatusOpen {
		t.Errorf("ticketStatus = %v, want open", got)
	}
}
