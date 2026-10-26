package tickets

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/terminalrun"
)

// requestTicketMenu presses "m" and returns the ready message its cmd
// resolves, not yet delivered.
func requestTicketMenu(t *testing.T, m Model) (Model, ticketMenuReadyMsg) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	ready, ok := cmd().(ticketMenuReadyMsg)
	if !ok {
		t.Fatalf("m resolved to %T, want ticketMenuReadyMsg", ready)
	}
	return updated.(Model), ready
}

// chooseTicketMenuItem filters the open Context menu down to label, presses
// enter and feeds the pick back through Update. cmd is nil when the item is
// dim.
func chooseTicketMenuItem(t *testing.T, m Model, label string) (Model, tea.Cmd) {
	t.Helper()
	if !m.ticketMenu.IsOpen {
		t.Fatal("context menu is not open")
	}
	updated := tea.Model(m)
	for _, r := range label {
		updated, _ = updated.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		return updated.(Model), nil
	}
	updated, cmd = updated.Update(cmd())
	return updated.(Model), cmd
}

func itemByLabel(t *testing.T, items []contextmenu.Item[ticketTarget], label string) contextmenu.Item[ticketTarget] {
	t.Helper()
	i := slices.IndexFunc(items, func(it contextmenu.Item[ticketTarget]) bool { return it.Label == label })
	if i < 0 {
		t.Fatalf("no %q item in %v", label, itemLabels(items))
	}
	return items[i]
}

// menuHasReason reports whether an item of the open menu is dim for reason.
func menuHasReason(m Model, reason string) bool {
	return slices.ContainsFunc(m.ticketMenu.Items(), func(it contextmenu.Item[ticketTarget]) bool { return it.Disabled == reason })
}

func itemLabels(items []contextmenu.Item[ticketTarget]) []string {
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.Group.String() + "/" + it.Label
	}
	return labels
}

func orchestrationLabels() []string {
	return []string{"Unmute & Reopen", "Pause", "Cancel"}
}

func withMutes(info server.TicketInfo, mutes []schema.MuteRecord) server.TicketInfo {
	info.Mutes = mutes
	return info
}

func TestTicketMenu_GroupsAndPauseByAddress(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(fakeServerAPI{calls: &calls, detail: detailWith(server.ActionAnswer, server.ActionResume)})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		pausedAnswerInfo("gx:alpha/01", "First"),
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = pressMenuKey(t, selectTicketRow(t, updated.(Model)))

	view := m.ticketMenu.View()
	for _, want := range []string{"Suggested", "Status", "Server", `Ticket "First"`, resumeLabel, "Set draft", "Pause", "Transcript"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu view lacks %q:\n%s", want, view)
		}
	}
	if !strings.Contains(m.View().Content, resumeLabel) {
		t.Error("tab view does not overlay the menu")
	}

	m, cmd := chooseTicketMenuItem(t, m, "Pause")
	if cmd == nil {
		t.Fatal("pause issued no cmd")
	}
	if msg, ok := cmd().(serverWriteMsg); !ok || msg.problem != "" {
		t.Fatalf("pause result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"pause gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
	if m.ticketMenu.IsOpen {
		t.Error("menu still open after a pick")
	}
}

func TestTicketMenu_UnmuteByAddress(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(fakeServerAPI{calls: &calls})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		withMutes(pausedRepairInfo("gx:alpha/01", "First"), muted),
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	_, cmd := chooseTicketMenuItem(t, pressMenuKey(t, m), "Unmute")
	if cmd == nil {
		t.Fatal("unmute is dim for a muted server ticket")
	}
	cmd()
	if !slices.Equal(calls, []string{"unmute gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestTicketMenu_ServerTicketMutesDecideUnmute(t *testing.T) {
	for name, tt := range map[string]struct {
		mutes []schema.MuteRecord
		want  string
	}{
		"muted":   {muted, ""},
		"unmuted": {nil, notMuted},
	} {
		t.Run(name, func(t *testing.T) {
			m := newServerModel(t).WithServer(fakeServerAPI{})
			m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
				withMutes(pausedRepairInfo("gx:alpha/01", "First"), tt.mutes),
			}})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			_, ready := requestTicketMenu(t, selectTicketRow(t, updated.(Model)))
			item := itemByLabel(t, ready.items, "Unmute & Reopen")
			if item.Disabled != tt.want {
				t.Errorf("unmute disabled = %q, want %q", item.Disabled, tt.want)
			}
			if tt.mutes != nil && item.Group != contextmenu.Suggested {
				t.Errorf("unmute group = %v, want Suggested", item.Group)
			}
		})
	}
}

func TestTicketMenu_SetStatusWritesTheFile(t *testing.T) {
	for _, tt := range []struct{ label, want string }{
		{"Set open", "status: open"},
		{"Set done", "status: done"},
	} {
		t.Run(tt.label, func(t *testing.T) {
			var calls []string
			m, file := serverModelWithStoreTicket(t, fakeServerAPI{calls: &calls}, true)
			_, cmd := chooseTicketMenuItem(t, pressMenuKey(t, m), tt.label)
			if cmd == nil {
				t.Fatalf("%s issued nothing", tt.label)
			}
			cmd()
			if raw, _ := os.ReadFile(file); !strings.Contains(string(raw), tt.want) {
				t.Errorf("ticket file = %s, want %s", raw, tt.want)
			}
		})
	}
}

func TestTicketMenu_NoServer_OrchestrationIsDim(t *testing.T) {
	for name, cut := range map[string]func(*Model){
		"no server api": func(m *Model) { m.serverAPI = nil },
		"link down":     func(m *Model) { m.link = ServerLinkDown },
	} {
		t.Run(name, func(t *testing.T) {
			var calls []string
			m, file := serverModelWithStoreTicket(t, fakeServerAPI{calls: &calls}, false)
			r, _ := m.selectedRow()
			m.epics[r.epicIdx].Tickets[r.ticketIdx].Mutes = muted
			cut(&m)

			m, ready := requestTicketMenu(t, m)
			for _, label := range orchestrationLabels() {
				if got := itemByLabel(t, ready.items, label).Disabled; got != serverNotRunning {
					t.Errorf("%s disabled = %q, want %q", label, got, serverNotRunning)
				}
			}
			updated, _ := m.Update(ready)
			m = updated.(Model)
			if !menuHasReason(m, serverNotRunning) {
				t.Errorf("open menu has no item dim for %q", serverNotRunning)
			}

			before, _ := os.ReadFile(file)
			for _, label := range []string{"Pause", "Cancel", "Unmute"} {
				next, cmd := chooseTicketMenuItem(t, m, label)
				if cmd != nil {
					t.Errorf("%s issued a cmd", label)
				}
				if !next.ticketMenu.IsOpen {
					t.Errorf("%s closed the menu", label)
				}
			}
			if after, _ := os.ReadFile(file); string(after) != string(before) {
				t.Errorf("ticket file changed:\n%s", after)
			}
			if len(calls) != 0 {
				t.Errorf("calls = %v, want none", calls)
			}

			_, cmd := chooseTicketMenuItem(t, m, "Set open")
			if cmd == nil {
				t.Fatal("set open issued nothing")
			}
			cmd()
			if raw, _ := os.ReadFile(file); !strings.Contains(string(raw), "status: open") {
				t.Errorf("ticket file = %s, want status: open", raw)
			}
		})
	}
}

func TestTicketMenu_ReadOnlyLinkDimsWithItsReason(t *testing.T) {
	m, _ := serverModelWithStoreTicket(t, fakeServerAPI{}, false)
	m.link = ServerLinkReadOnly
	_, ready := requestTicketMenu(t, m)
	if got, want := itemByLabel(t, ready.items, "Pause").Disabled, ServerLinkReadOnly.disabledReason(); got != want {
		t.Errorf("pause disabled =%q, want %q", got, want)
	}
}

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

	resumeDetail := detailWith(server.ActionResume)
	done := ticketMenuInput{rendered: tickets.StatusDone, ticket: tickets.Ticket{Status: "done"}, isMap: true, detail: &resumeDetail}
	for _, label := range []string{"Pause", "Cancel", resumeLabel, "Unmute & Reopen"} {
		if got := disabled(done, label); got != ticketFinished {
			t.Errorf("%s on a done ticket = %q, want %q", label, got, ticketFinished)
		}
	}
	done.serverReason = serverNotRunning
	if got := disabled(done, "Pause"); got != serverNotRunning {
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
	if got := disabled(paused, "Investigate"); got != herdrDownText {
		t.Errorf("investigate with herdr down = %q", got)
	}
}

func TestTicketMenuItems_StatusPickCarriesTheStatus(t *testing.T) {
	item := itemByLabel(t, ticketMenuItems(ticketMenuInput{rendered: tickets.StatusOpen}), "Set draft")
	target := ticketTarget{address: "gx:alpha/01"}
	if got := item.Run(target)(); got != (ticketMenuPickMsg{target: target, status: schema.StatusDraft}) {
		t.Errorf("pick = %+v", got)
	}
}

// stubTicketEditor records the path and read-only flag each edit launches.
func stubTicketEditor(t *testing.T) *[]string {
	t.Helper()
	var launched []string
	orig := launchTicketEditor
	launchTicketEditor = func(_ string, _ ui.Settings, path string, _ terminalrun.SplitType, readOnly bool) tea.Cmd {
		launched = append(launched, fmt.Sprintf("%s ro=%v", path, readOnly))
		return func() tea.Msg { return nil }
	}
	t.Cleanup(func() { launchTicketEditor = orig })
	return &launched
}

func TestTicketMenu_AgentAndEditCopyGroups(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, fakeServerAPI{}, false)
	m = pressMenuKey(t, m)
	view := m.ticketMenu.View()
	for _, want := range []string{"Agent", "Edit/copy", "Watch", "Transcript", "Investigate",
		"Edit file (vsplit)", "e v", "Yank epic - ticket", "y y", "Yank file path", "y f"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu view lacks %q:\n%s", want, view)
		}
	}

	launched := stubTicketEditor(t)
	_, cmd := chooseTicketMenuItem(t, m, "Edit file (tab)")
	if cmd == nil {
		t.Fatal("edit issued no cmd")
	}
	if want := []string{file + " ro=false"}; !slices.Equal(*launched, want) {
		t.Errorf("launched = %v, want %v", *launched, want)
	}

	for _, tt := range []struct{ label, chord string }{
		{"Yank epic - ticket", "yy"},
		{"Yank file path", "yf"},
	} {
		got := stubClipboard(t)
		closed, _ := chooseTicketMenuItem(t, m, tt.label)
		fromMenu := *got
		*got = ""
		next := tea.Model(closed)
		for _, r := range tt.chord {
			next, _ = next.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if fromMenu == "" || fromMenu != *got {
			t.Errorf("%s copied %q, %s copies %q", tt.label, fromMenu, tt.chord, *got)
		}
	}
}

func TestTicketMenu_ArchivedRowOnlyReads(t *testing.T) {
	root := t.TempDir()
	writeArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: paused\nPause reason: spinning\n\nBody.\n")
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectArchivedTicketRow(t, updated.(Model))

	m, ready := requestTicketMenu(t, m)
	var enabled []string
	for _, it := range ready.items {
		switch {
		case it.Disabled == "":
			enabled = append(enabled, it.Label)
		case it.Label != "Watch" && it.Disabled != archivedText:
			t.Errorf("%s disabled = %q, want %q", it.Label, it.Disabled, archivedText)
		}
	}
	want := []string{"Transcript", "Edit file", "Edit file (split)", "Edit file (vsplit)", "Edit file (tab)", "Yank epic - ticket", "Yank file path"}
	if !slices.Equal(enabled, want) {
		t.Errorf("enabled = %v, want %v", enabled, want)
	}

	launched := stubTicketEditor(t)
	updated, _ = m.Update(ready)
	chooseTicketMenuItem(t, updated.(Model), "Edit file")
	if len(*launched) != 1 || !strings.HasSuffix((*launched)[0], "01-old-ticket.md ro=true") {
		t.Errorf("launched = %v, want the archived file read-only", *launched)
	}
}

func TestTicketMenuItems_ArchivedKeepsReadActions(t *testing.T) {
	items := ticketMenuItems(ticketMenuInput{
		rendered: tickets.StatusPausedRepair,
		ticket:   tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: schema.PauseSpinning},
		archived: true,
	})
	reads := []string{actionWatch, actionTranscript, actionYankSummary, actionYankFilePath}
	for _, e := range ticketMenuEdits {
		reads = append(reads, e.action)
	}
	var enabled []string
	for _, it := range items {
		if it.Disabled != "" {
			continue
		}
		msg, _ := it.Run(ticketTarget{})().(ticketMenuPickMsg)
		if !slices.Contains(reads, msg.action) {
			t.Errorf("%s (action %q) enabled on an archived row", it.Label, msg.action)
		}
		enabled = append(enabled, msg.action)
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
		d := detailWith(server.ActionResume)
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
	} {
		item := itemByLabel(t, ticketMenuItems(tt.in), "Go to pane")
		if item.Disabled != tt.want || item.Group != contextmenu.Agent {
			t.Errorf("%s: Go to pane = %q in %v, want %q in Agent", tt.name, item.Disabled, item.Group, tt.want)
		}
	}
}

func hasItem(items []contextmenu.Item[ticketTarget], label string) bool {
	return slices.ContainsFunc(items, func(it contextmenu.Item[ticketTarget]) bool { return it.Label == label })
}

func TestTicketsTab_StatusKeyOpensNothing(t *testing.T) {
	m, _ := serverModelWithStoreTicket(t, fakeServerAPI{}, false)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if updated.(Model).ModalOpen() || cmd != nil {
		t.Errorf("s opened a modal (%v) or issued a cmd (%v)", updated.(Model).ModalOpen(), cmd != nil)
	}
}
