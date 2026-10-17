package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/viewmodel"
)

// requestQueueMenu presses "m" and returns the ready message its cmd
// resolves, not yet delivered.
func requestQueueMenu(t *testing.T, m QueueModel) (QueueModel, ticketMenuReadyMsg) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	ready, ok := cmd().(ticketMenuReadyMsg)
	if !ok {
		t.Fatalf("m resolved to %T, want ticketMenuReadyMsg", ready)
	}
	return updated.(QueueModel), ready
}

func openQueueMenu(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	m, ready := requestQueueMenu(t, m)
	updated, _ := m.Update(ready)
	m = updated.(QueueModel)
	if !m.ticketMenu.IsOpen {
		t.Fatal("context menu did not open")
	}
	return m
}

// chooseQueueMenuItem filters the open Context menu down to label, presses
// enter and feeds the pick back through Update. cmd is nil when the item is
// dim.
func chooseQueueMenuItem(t *testing.T, m QueueModel, label string) (QueueModel, tea.Cmd) {
	t.Helper()
	updated := tea.Model(m)
	for _, r := range label {
		updated, _ = updated.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		return updated.(QueueModel), nil
	}
	updated, cmd = updated.Update(cmd())
	return updated.(QueueModel), cmd
}

func groupsOf(items []contextmenu.Item[ticketTarget]) []contextmenu.Group {
	var groups []contextmenu.Group
	for _, it := range items {
		if !slices.Contains(groups, it.Group) {
			groups = append(groups, it.Group)
		}
	}
	return groups
}

func TestQueueTicketMenu_SameGroupsAsTicketsTabAndResumeByAddress(t *testing.T) {
	var calls []string
	m := queueWithTicket(t, newMaintenanceQueue(t, fakeServerAPI{calls: &calls}), "needs-answer", nil)
	m, ready := requestQueueMenu(t, m)

	tabs := ticketMenuItems(newTicketMenuInput(m.epics[0], m.epics[0].Tickets[0], false, ""))
	want := slices.Insert(groupsOf(tabs), slices.Index(groupsOf(tabs), contextmenu.EditCopy), contextmenu.Queue)
	if got := groupsOf(ready.items); !slices.Equal(got, want) {
		t.Errorf("groups = %v, want %v", got, want)
	}

	updated, _ := m.Update(ready)
	m = updated.(QueueModel)
	if !strings.Contains(m.View().Content, `Ticket "First"`) {
		t.Error("tab view does not overlay the menu")
	}
	m, cmd := chooseQueueMenuItem(t, m, resumeLabel)
	if cmd == nil {
		t.Fatal("resume issued no cmd")
	}
	if msg, ok := cmd().(serverWriteMsg); !ok || msg.problem != "" {
		t.Fatalf("resume result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"unpark gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
	if m.ticketMenu.IsOpen {
		t.Error("menu still open after a pick")
	}
}

func TestQueueTicketMenu_UnmuteByAddress(t *testing.T) {
	var calls []string
	m := queueWithTicket(t, newMaintenanceQueue(t, fakeServerAPI{calls: &calls}), "needs-repair", muted)
	_, cmd := chooseQueueMenuItem(t, openQueueMenu(t, m), "Unmute")
	cmd()
	if !slices.Equal(calls, []string{"unmute gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestQueueTicketMenu_ServerTicketMutesEnableUnmute(t *testing.T) {
	m := newMaintenanceQueue(t, fakeServerAPI{})
	m, _ = m.applyServerState(&viewmodel.State{
		Tickets: []server.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "needs-repair", Mutes: muted}},
		Queue:   []string{"gx:alpha/01"},
	})
	m = selectFirstQueueTicketRow(t, m)
	_, ready := requestQueueMenu(t, m)
	item := itemByLabel(t, ready.items, "Unmute & Reopen")
	if item.Disabled != "" || item.Group != contextmenu.Suggested {
		t.Errorf("unmute = %+v, want enabled in Suggested", item)
	}
}

func TestQueueTicketMenu_ApproveRecoveryProposal(t *testing.T) {
	const label = "Approve recovery proposal"
	var calls []string
	m := newMaintenanceQueue(t, fakeServerAPI{calls: &calls})
	m.epics[0].Tickets[0].Body = "## Proposed Remedy\n\nrun it"
	m.clampSelected()
	m = selectFirstQueueTicketRow(t, m)
	_, ready := requestQueueMenu(t, m)
	item := itemByLabel(t, ready.items, label)
	if item.Group != contextmenu.Queue || item.Key != "A" || item.Disabled != "" {
		t.Errorf("approve = group %v key %q disabled %q", item.Group, item.Key, item.Disabled)
	}
	m, _ = chooseQueueMenuItem(t, openQueueMenu(t, m), label)
	accept(t, m)
	if !slices.Equal(calls, []string{"approve gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}

	m = newMaintenanceQueue(t, fakeServerAPI{})
	m.epics[0].Tickets[0].Body = "## Notes\n\nnothing"
	m.clampSelected()
	m = selectFirstQueueTicketRow(t, m)
	_, ready = requestQueueMenu(t, m)
	if got := itemByLabel(t, ready.items, label).Disabled; got != noProposal {
		t.Errorf("approve with no proposal disabled = %q, want %q", got, noProposal)
	}
}

func TestQueueTicketMenu_NoServer_OrchestrationIsDim(t *testing.T) {
	path := writeTicketFile(t)
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(QueueModel)
	m.loaded = true
	m.epics = []tickets.Epic{{Name: "alpha", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Title: "First", Path: path, Status: "needs-repair", Mutes: muted}}}}
	m.checked = map[string]bool{path: true}
	m.clampSelected()
	m = selectFirstQueueTicketRow(t, m)

	m, ready := requestQueueMenu(t, m)
	for _, label := range append(orchestrationLabels(), "Approve recovery proposal") {
		if got := itemByLabel(t, ready.items, label).Disabled; got != serverNotRunning {
			t.Errorf("%s disabled = %q, want %q", label, got, serverNotRunning)
		}
	}
	updated, _ := m.Update(ready)
	before, _ := os.ReadFile(path)
	if _, cmd := chooseQueueMenuItem(t, updated.(QueueModel), "Park"); cmd != nil {
		t.Error("park issued a cmd")
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("ticket file changed:\n%s", after)
	}
}

func TestQueueTicketMenu_StaleLivenessOpensNothing(t *testing.T) {
	for name, stale := range map[string]func(QueueModel) QueueModel{
		"selection moved": func(m QueueModel) QueueModel {
			next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			return next.(QueueModel)
		},
		"another modal opened": func(m QueueModel) QueueModel {
			return pressQueue(t, m, 'x')
		},
		"escaped": func(m QueueModel) QueueModel {
			next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			return next.(QueueModel)
		},
	} {
		t.Run(name, func(t *testing.T) {
			snap := server.Snapshot{Tickets: []server.TicketInfo{
				{Address: "gx:alpha/01", Title: "First", Status: "open"},
				{Address: "gx:alpha/02", Title: "Second", Status: "open"},
			}}
			m, _ := deliverQueue(t, snap, []server.QueueItem{{Address: "gx:alpha/01"}, {Address: "gx:alpha/02"}})
			m = selectFirstQueueTicketRow(t, m)
			m, ready := requestQueueMenu(t, m)
			m = stale(m)
			updated, _ := m.Update(ready)
			if updated.(QueueModel).ticketMenu.IsOpen {
				t.Error("a stale liveness result opened the menu")
			}
		})
	}
}

// openQueueTranscript chooses Transcript from the Queue tab menu opened on
// path, and returns what the action's Cmd produced.
func openQueueTranscript(t *testing.T, store, path string) tea.Msg {
	t.Helper()
	m := NewQueueModel(t.TempDir(), ui.Settings{TicketStore: store}, keys.Manager{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(QueueModel)
	m.loaded = true
	m.epics = []tickets.Epic{{Name: "alpha", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Title: "First", Path: path, Status: "done"}}}}
	m.checked = map[string]bool{path: true}
	m.clampSelected()
	m = selectFirstQueueTicketRow(t, m)
	_, cmd := chooseQueueMenuItem(t, openQueueMenu(t, m), "Transcript")
	if cmd == nil {
		t.Fatal("Transcript returned no Cmd")
	}
	return cmd()
}

func TestQueueTicketMenu_TranscriptResolvesAnAddress(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"s1": "hello from the agent"})
	store := t.TempDir()
	dir := filepath.Join(store, "gx", "alpha", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nid: 01\nstatus: done\ntype: implement\nsession_ids:\n  - s1\n---\n\n# 01 — x\n"
	if err := os.WriteFile(filepath.Join(dir, "01-first.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, ok := openQueueTranscript(t, store, "gx:alpha/01").(watchLoadedMsg)
	if !ok || !strings.Contains(ansi.Strip(strings.Join(msg.lines, "\n")), "hello from the agent") {
		t.Errorf("msg = %#v, want the transcript of the address's ticket file", msg)
	}
}

func TestQueueTicketMenu_TranscriptAcceptsAFilePath(t *testing.T) {
	fakeSessionLookup(t, map[string]string{"s1": "hello from the agent"})
	path := writeTranscriptTicket(t, "session_ids:\n  - s1\n")

	if _, ok := openQueueTranscript(t, t.TempDir(), path).(watchLoadedMsg); !ok {
		t.Error("a file-shaped Path did not open its transcript")
	}
}

func TestResolveTicketPath(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	dir := filepath.Join(store, "gx", "alpha", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "01-first.md")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if file, address := resolveTicketPath(store, "gx:alpha/01"); file != want || address != "gx:alpha/01" {
		t.Errorf("address: got (%q, %q)", file, address)
	}
	if file, address := resolveTicketPath(store, "/some/01-x.md"); file != "/some/01-x.md" || address != "" {
		t.Errorf("file: got (%q, %q)", file, address)
	}
}
