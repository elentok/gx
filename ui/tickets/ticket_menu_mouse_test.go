package tickets

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

// openedTicketMenu is a server-backed tab with one ticket selected and its
// Context menu open, on a 120x40 screen.
func openedTicketMenu(t *testing.T, calls *[]string) Model {
	t.Helper()
	api := tickettest.FakeServerAPI{Calls: calls, DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume)}
	m := newServerModel(t).WithServer(api).WithTickets(api).
		WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).
		WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).
		WithReads(tickettest.FakeReads{FakeServerAPI: api})
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{pausedAnswerInfo("gx:alpha/01", "First")}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return pressMenuKey(t, selectTicketRow(t, updated.(Model)))
}

// cellOf finds text in the rows of content at or below row minY.
func cellOf(t *testing.T, content, text string, minY int) (x, y int) {
	t.Helper()
	for y, row := range strings.Split(ansi.Strip(content), "\n") {
		if before, _, ok := strings.Cut(row, text); ok && y >= minY {
			return ansi.StringWidth(before), y
		}
	}
	t.Fatalf("%q not drawn:\n%s", text, ansi.Strip(content))
	return 0, 0
}

func TestTicketMenu_OpensAtTheAnchorOrigin(t *testing.T) {
	m := openedTicketMenu(t, new([]string))

	frame := strings.Split(m.menus.Menu.View(), "\n")
	w := 0
	for _, l := range frame {
		w = max(w, ansi.StringWidth(l))
	}
	ox, oy := contextmenu.Origin(m.menuAnchor(), w, len(frame), m.width, m.height)

	// The title sits in the top border: "╭ …" starting at the origin.
	x, y := cellOf(t, m.View().Content, `Actions · Ticket "First"`, 0)
	if x != ox+2 || y != oy {
		t.Fatalf("menu title at (%d,%d), want (%d,%d) from contextmenu.Origin", x, y, ox+2, oy)
	}
}

func TestTicketMenu_ClickOnAnItemRunsItAndClickOutsideCloses(t *testing.T) {
	var calls []string
	m := openedTicketMenu(t, &calls)

	_, oy := cellOf(t, m.View().Content, `Actions · Ticket "First"`, 0)
	x, y := cellOf(t, m.View().Content, "Pause", oy)

	outside, _ := m.Update(tea.MouseClickMsg{X: m.width - 3, Y: m.height - 3, Button: tea.MouseLeft})
	if outside.(Model).menus.Menu.IsOpen {
		t.Fatal("a click over the preview left the menu open")
	}

	updated, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on an item issued no cmd")
	}
	updated, cmd = updated.Update(cmd())
	if cmd == nil {
		t.Fatal("the picked item issued no cmd")
	}
	if msg, ok := cmd().(rowactions.ServerWriteMsg); !ok || msg.Problem != "" {
		t.Fatalf("pause result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"pause gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
	if updated.(Model).menus.Menu.IsOpen {
		t.Error("menu still open after a click on an item")
	}
}
