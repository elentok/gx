package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
)

func TestOlderServer_QueueTabShowsNoticeAndNoTickets(t *testing.T) {
	m := newServerQueueModel(t, nil)
	st := olderServerState()
	next, _ := m.WithServerState(&st, tabapi.ServerLinkUp)
	view := ansi.Strip(next.View().Content)
	if !strings.Contains(view, tabapi.OlderServerNotice) || strings.Contains(view, "First") {
		t.Fatalf("view = %s", view)
	}
	if got := len(next.(QueueModel).rows.Epics); got != 0 {
		t.Fatalf("epics = %d, want none", got)
	}
}

func TestNoServer_QueueSaysServerNotRunning(t *testing.T) {
	m := newServerQueueModel(t, nil)
	next, _ := m.WithServerState(nil, tabapi.ServerLinkDown)
	if view := ansi.Strip(next.View().Content); !strings.Contains(view, "server not running") {
		t.Fatalf("view = %s", view)
	}
}

func TestQueueModel_ClickOnTicketIconTogglesIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-parent.md", "Status: open\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "02-child.md", "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-parent.md"): true,
		tickettest.TicketPath(root, "alpha", "02-child.md"):  true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	parentIdx := -1
	for i, e := range m.rows.Tree.Entries() {
		if e.Value.Kind == NodeTicket && e.HasChildren {
			parentIdx = i
			break
		}
	}
	if parentIdx < 0 {
		t.Fatal("no parent ticket entry")
	}
	parent := m.rows.Tree.Entries()[parentIdx]
	iconX := rowfmt.PanelPaddingX + tree.LabelColumn(parent) + 1
	y := 1 + len(m.header().BodyLines()) + parentIdx
	before := len(m.rows.Tree.Entries())

	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX + 10, Y: y})
	m = updated.(QueueModel)
	if got := len(m.rows.Tree.Entries()); got != before {
		t.Fatalf("click off the icon changed entries: %d -> %d", before, got)
	}

	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX, Y: y})
	m = updated.(QueueModel)
	if got := len(m.rows.Tree.Entries()); got != before-1 {
		t.Fatalf("click on icon should collapse the ticket, entries %d -> %d", before, got)
	}
	if m.rows.Tree.SelectedIndex() != parentIdx {
		t.Fatalf("click should select the parent row %d, got %d", parentIdx, m.rows.Tree.SelectedIndex())
	}

	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX, Y: y})
	m = updated.(QueueModel)
	if got := len(m.rows.Tree.Entries()); got != before {
		t.Fatalf("second click should re-expand, entries %d, want %d", got, before)
	}
}

func TestQueueModel_DeleteWithoutAServerWritesNothing(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-root.md", "Status: open\n\nBody.\n")
	file := tickettest.TicketPath(root, "alpha", "01-root.md")
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), map[string]bool{file: true}))
	m = selectFirstQueueTicketRow(t, m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if next.(QueueModel).confirm.IsOpen {
		t.Fatal("x opened a confirm with no server")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Message != rowactions.ServerNotRunning {
		t.Fatalf("result = %+v, want the %q toast", msg, rowactions.ServerNotRunning)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("ticket file gone: %v", err)
	}
}

func TestQueueModel_EditTargetIsTheAddressFile(t *testing.T) {
	store := t.TempDir()
	file := tickettest.WriteStoreTicket(t, store, "01-first.md", "---\nid: \"01\"\nstatus: open\n---\n\nBody.\n")
	m := newMaintenanceQueue(t, tickettest.FakeServerAPI{})
	m.settings.TicketStore = store
	m.rows.Epics[0].Tickets[0].File = filepath.Join(t.TempDir(), "stale.md")

	if got, ok := m.selectedEditTarget(); !ok || got != file {
		t.Errorf("edit target = %q, %v, want %q", got, ok, file)
	}
}

func TestQueueModel_ScrollbarAppearsOnlyWhenListOverflows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "epic", "01-only-ticket.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "epic", "01-only-ticket.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	shortContent := m.View().Content
	if strings.Contains(shortContent, "┃") {
		t.Fatalf("expected no queue scrollbar thumb when list fits, got:\n%s", shortContent)
	}

	checked2 := map[string]bool{}
	for i := 1; i <= 60; i++ {
		name := fmt.Sprintf("%02d-ticket.md", i)
		tickettest.WriteTicket(t, root, "epic2", name, "Status: open\n\nBody.\n")
		checked2[tickettest.TicketPath(root, "epic2", name)] = true
	}
	m2 := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked2))
	longContent := m2.View().Content
	if !strings.Contains(longContent, "┃") {
		t.Fatalf("expected a queue scrollbar thumb when the list overflows, got:\n%s", longContent)
	}
}

func TestOpenHelp_QueueOpensHelpFromSearchInputWithoutTyping(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	updated, _ = updated.Update(nav.OpenHelp()())
	m = updated.(QueueModel)

	if !m.help.IsOpen {
		t.Fatal("OpenHelp did not open help")
	}
	if m.search.Mode() != search.SearchModeNone || m.search.Query() != "" {
		t.Fatalf("search not ended: mode %v query %q", m.search.Mode(), m.search.Query())
	}
}

func TestOpenHelp_QueueIgnoredWhileTicketMenuOpen(t *testing.T) {
	t.Parallel()
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.Manager{})
	m.menus.Menu.IsOpen = true
	if !m.ModalOpen() {
		t.Fatal("the ticket menu does not count as a modal")
	}
	updated, _ := m.Update(nav.OpenHelp()())
	if updated.(QueueModel).help.IsOpen {
		t.Fatal("OpenHelp opened help over the ticket menu")
	}
}

func TestQueueModel_TVHidesPreviewAndGivesListFullWidth(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-solo.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-solo.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = pressKeys(t, updated, 't', 'v').(QueueModel)

	if !m.preview.Hidden {
		t.Fatal("expected \"tv\" to hide the preview")
	}
	if r := m.layout().treeRect(); r.W != 120 {
		t.Fatalf("expected list to fill the full width, got %d", r.W)
	}
	if strings.Contains(m.View().Content, "Preview") {
		t.Fatal("expected no preview panel while hidden")
	}

	m = pressKeys(t, m, 'l').(QueueModel)
	if m.preview.Focus != preview.FocusSidebar {
		t.Fatalf("expected focus to stay on the list while preview hidden, got %v", m.preview.Focus)
	}

	m = pressKeys(t, m, 't', 'v').(QueueModel)
	if m.preview.Hidden || !strings.Contains(m.View().Content, "Preview") {
		t.Fatal("expected a second \"tv\" to show the preview again")
	}
}

func TestQueueWatch_OpensLiveModal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	appendWatchLog(t, path, "hello from the queue")
	fakeNativeLog(t, agentlog.Log{Path: path, Ended: func() bool { return false }}, nil)

	m := newServerQueueModel(t, nil)
	m = selectFirstQueueTicketRow(t, m)
	_, cmd := chooseQueueMenuItem(t, openQueueMenu(t, m), "Watch")
	if cmd == nil {
		t.Fatal("Watch returned no Cmd")
	}
	next, tick := m.Update(cmd())
	m = next.(QueueModel)
	t.Cleanup(func() { m.watch.Close() })
	if view := m.watch.View(); !m.watch.IsOpen() || !strings.Contains(view, "hello from the queue") || !strings.Contains(view, "· live") {
		t.Errorf("Queue Watch did not open the live modal:\n%s", view)
	}
	if tick == nil {
		t.Error("Queue Watch armed no tick")
	}
}

func TestWrites_QueueTabOnlyToasts(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")
	for name, msg := range map[string]tea.Msg{
		"remove": ServerRemovedMsg{Removed: 1},
		"action": ServerDoneMsg{OK: "queue paused"},
	} {
		_, cmd := m.Update(msg)
		assertOnlyToast(t, name, cmd)
	}
}
