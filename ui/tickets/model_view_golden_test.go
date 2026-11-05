package tickets

import (
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestModelViewGolden_SidebarExpandedEpicAndDoneEpic(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "open-epic", "01-first-ticket.md", "Status: done\n\n# First ticket\n\nShipped.\n")
	tickettest.WriteTicket(t, root, "open-epic", "02-second-ticket.md", "Status: open\n\n# Second ticket\n\nStill to do.\n")
	tickettest.WriteTicket(t, root, "done-epic", "01-only-ticket.md", "Status: done\n\n# Only ticket\n\nDone.\n")

	m := goldenModel(t, root)
	expandOpenEpics(&m)
	m = selectGoldenTicket(t, m)
	assertGolden(t, "model_sidebar_epics", goldenModelView(m))
}

func TestModelViewGolden_PausedRowWithRepairPreview(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-stuck-ticket.md",
		"Status: paused\nPause reason: blocked-pane\n\n# Stuck ticket\n\nBody.\n\n"+tickets.LegacyRepairHeading+"\n\nThe agent pane blocked.\n")

	m := goldenModel(t, root)
	expandOpenEpics(&m)
	m = selectGoldenTicket(t, m)
	assertGolden(t, "model_paused_repair", goldenModelView(m))
}

func TestModelViewGolden_ServerModeClaimedTicketMetrics(t *testing.T) {
	snap := wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "Claimed ticket", Status: "claimed", ElapsedTime: 754, ActualContextWindow: 45_200, ActualCost: 1.5},
		{Address: "gx:alpha/02", Title: "Open ticket", Status: "open"},
	}}
	m := withState(newServerModel(t), snap)
	m = selectGoldenTicket(t, sizeGoldenModel(m))
	assertGolden(t, "model_server_claimed", goldenModelView(m))
}
