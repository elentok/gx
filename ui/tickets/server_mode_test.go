package tickets

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/server/wire"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// withRenderedStatus makes snap look like one from a current server, which
// always sends rendered_status: a ticket without one gets its raw status word.
// A snapshot with none at all reads as an older server's.
func withRenderedStatus(snap wire.Snapshot) wire.Snapshot {
	if snap.Seq == 0 {
		snap.Seq = 1
	}
	snap.Tickets = slices.Clone(snap.Tickets)
	for i := range snap.Tickets {
		if snap.Tickets[i].RenderedStatus == "" {
			snap.Tickets[i].RenderedStatus = snap.Tickets[i].Status
		}
	}
	return snap
}

func applySnapshot(st viewmodel.State, snap wire.Snapshot) viewmodel.State {
	return st.ApplySnapshot(withRenderedStatus(snap))
}

// pausedAnswerInfo and pausedRepairInfo are server tickets paused for each
// class: status paused, rendered as the class's word.
func pausedAnswerInfo(address, title string) wire.TicketInfo {
	return wire.TicketInfo{Address: address, Title: title, Status: "paused", RenderedStatus: gxtickets.StatusPausedAnswer.Word()}
}

func pausedRepairInfo(address, title string) wire.TicketInfo {
	return wire.TicketInfo{Address: address, Title: title, Status: "paused", RenderedStatus: gxtickets.StatusPausedRepair.Word()}
}

func newServerModel(t *testing.T) Model {
	t.Helper()
	return NewModel(t.TempDir(), ui.Settings{}, keys.New(nil)).WithServer(tickettest.FakeServerAPI{}).WithTickets(tickettest.FakeServerAPI{}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{}})
}

// withState hands the tab the state the app shell would deliver for snap,
// then expands its open epics.
func withState(m Model, snap wire.Snapshot) Model {
	st := applySnapshot(viewmodel.State{}, snap)
	next, _ := m.WithServerState(&st, tabapi.ServerLinkUp)
	m = next.(Model)
	expandOpenEpics(&m)
	return m
}

func TestServerMode_SnapshotRendersReducedRows(t *testing.T) {
	snap := wire.Snapshot{Seq: 4, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Fork", Status: "open", Parent: "gx:alpha/01"},
	}}
	m := withState(newServerModel(t), snap)
	if len(m.epics) != 1 || len(m.epics[0].Tickets) != 2 {
		t.Fatalf("epics = %+v", m.epics)
	}
}

func TestServerMode_NoStateKeepsLoading(t *testing.T) {
	m := newServerModel(t)
	if cmd := m.Init(); cmd != nil {
		t.Fatal("Init fetched; the shell's stream delivers the state")
	}
	pending, _ := m.WithServerState(nil, tabapi.ServerLinkUp)
	next, _ := pending.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m := next.(Model); m.loaded || !strings.Contains(m.View().Content, "loading…") {
		t.Fatalf("loaded=%v, want the loading… state", m.loaded)
	}
}

func TestServerMode_ResnapshotRequestFromRefresh(t *testing.T) {
	msg := newServerModel(t).cmdLoad()()
	if _, ok := msg.(tabapi.ResnapshotRequestedMsg); !ok {
		t.Fatalf("refresh msg = %T, want tabapi.ResnapshotRequestedMsg", msg)
	}
}

var scopeSnap = wire.Snapshot{Seq: 4, Tickets: []wire.TicketInfo{
	{Address: "gx:alpha/01", Status: "open"},
	{Address: "other:beta/01", Status: "open"},
}}

func TestServerMode_ScopesToCwdProjectAndToggles(t *testing.T) {
	m := withState(newServerModel(t).WithCwdProject("gx"), scopeSnap)
	if len(m.epics) != 1 || m.epics[0].Name != "gx:alpha" {
		t.Fatalf("scoped epics = %+v", m.epics)
	}
	next, _ := m.toggleProjectScope()
	if got := len(next.(Model).epics); got != 2 {
		t.Fatalf("all-projects epics = %d, want 2", got)
	}
}

func TestServerMode_ScopeToggleSurvivesDeliveredState(t *testing.T) {
	m := withState(newServerModel(t).WithCwdProject("gx"), scopeSnap)
	next, _ := m.toggleProjectScope()

	m = withState(next.(Model), scopeSnap)
	if got := len(m.epics); got != 2 {
		t.Fatalf("epics after delivery = %d, want 2 (still all projects)", got)
	}
}

// The first-load hint comes from the app shell, so the tab stays quiet.
func TestServerMode_OutsideProjectShowsAllWithoutToasting(t *testing.T) {
	m := withState(newServerModel(t).WithCwdProject(""), scopeSnap)
	if len(m.epics) != 2 {
		t.Fatalf("epics = %d, want all", len(m.epics))
	}
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if got := toastsOf(cmd); len(got) != 0 {
		t.Fatalf("tab toasts = %+v", got)
	}
}

func TestServerMode_StaleStateIsIgnoredWhileOnFallback(t *testing.T) {
	m := newServerModel(t)
	st := applySnapshot(viewmodel.State{}, scopeSnap)
	next, _ := m.WithServerState(&st, tabapi.ServerLinkDown)
	m = next.(Model)
	if m.loaded || len(m.epics) != 0 {
		t.Fatalf("fallback tab took the shell's stale state: loaded=%v epics=%d", m.loaded, len(m.epics))
	}
}

// toastsOf runs cmd (flattening batches) and returns the toasts it emits.
func toastsOf(cmd tea.Cmd) []notify.NotifyMsg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case notify.NotifyMsg:
		return []notify.NotifyMsg{msg}
	case tea.BatchMsg:
		var out []notify.NotifyMsg
		for _, c := range msg {
			out = append(out, toastsOf(c)...)
		}
		return out
	}
	return nil
}

func TestServerMode_PendingRowRendersVerdictSubtext(t *testing.T) {
	m := newServerModel(t)
	snap := wire.Snapshot{Seq: 1,
		Tickets: []wire.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "open"}},
		Pending: []wire.PendingRow{{Address: "gx:alpha/01", Verdict: "blocked", Reason: "waiting on 00"}},
	}
	m = withState(m, snap)

	var body []string
	for _, e := range m.buildSidebarEntries() {
		if e.Value.kind == nodeTicket {
			body = e.Body
		}
	}
	if len(body) != 1 || !strings.Contains(body[0], "blocked: waiting on 00") {
		t.Fatalf("subtext = %q", body)
	}
}

// A whole queued epic gives one row, for its next ticket; only that ticket
// shows a verdict.
func TestServerMode_WholeQueuedEpicShowsTheVerdictOnItsNextTicketOnly(t *testing.T) {
	m := newServerModel(t)
	snap := wire.Snapshot{Seq: 1,
		Tickets: []wire.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: "open"},
			{Address: "gx:alpha/02", Title: "Second", Status: "open"},
		},
		Pending: []wire.PendingRow{{Address: "gx:alpha/01", Verdict: "disk low", Reason: "2 GB free"}},
	}
	m = withState(m, snap)

	var bodies [][]string
	for _, e := range m.buildSidebarEntries() {
		if e.Value.kind == nodeTicket {
			bodies = append(bodies, e.Body)
		}
	}
	if len(bodies) != 2 || len(bodies[0]) != 1 || !strings.Contains(bodies[0][0], "disk low: 2 GB free") || len(bodies[1]) != 0 {
		t.Fatalf("subtexts = %q", bodies)
	}
}

// Seam D: "enter" on a paused row issues the detail's first verb (resume here).
func TestServerMode_EnterResumesAPausedRow(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionResume)}).WithTickets(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionResume)}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionResume)}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionResume)}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionResume)}})
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		// The wire carries no pause reason, so only the retired repair status
		// tells the client this pause is a repair.
		{Address: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair), RenderedStatus: gxtickets.StatusPausedRepair.Word()},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a paused row issued nothing")
	}
	cmd()
	if want := []string{"resume gx:alpha/02"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

// In server mode a row's Path is the ticket's address, so the menu's status
// writes and the edit chords must reach the ticket's file instead.
func serverModelWithTicketFile(t *testing.T) (Model, string) {
	t.Helper()
	return serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, false)
}

// serverModelWithStoreTicket serves a draft gx:alpha/20 whose file lives in a
// temp store, in a map epic when isMap is set.
func serverModelWithStoreTicket(t *testing.T, api tickettest.FakeServerAPI, isMap bool) (Model, string) {
	t.Helper()
	store := t.TempDir()
	file := tickettest.WriteStoreTicket(t, store, "20-a.md", "---\nid: \"20\"\nstatus: draft\ntype: research\n---\n\n# 20 — A\n\nBody.\n")
	m := newServerModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api})
	m.settings.TicketStore = store
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/20", Title: "A", Status: "draft", File: file, EpicIsMap: isMap},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return selectTicketRow(t, updated.(Model)), file
}

func TestServerMode_EditTargetIsTheTicketFile(t *testing.T) {
	m, file := serverModelWithTicketFile(t)

	target, ok, warning := m.selectedEditTarget()
	if !ok || target != file {
		t.Errorf("edit target = %q, %v (%s), want the ticket file %q", target, ok, warning, file)
	}
}

func openAnswerMenu(t *testing.T, api tickettest.FakeServerAPI) Model {
	t.Helper()
	if len(api.DetailRes.Actions) == 0 {
		api.DetailRes = detailWith(wire.ActionAnswer, wire.ActionResume)
	}
	m := newServerModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api})
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		pausedAnswerInfo("gx:alpha/01", "First"),
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return pressMenuKey(t, selectTicketRow(t, updated.(Model)))
}

// pressMenuKey presses "m" and feeds back the menu its cmd resolves.
func pressMenuKey(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	updated, _ = updated.(Model).Update(cmd())
	return updated.(Model)
}

func TestServerMode_MenuKeyOpensOnEveryStatus(t *testing.T) {
	for _, status := range []string{"open", "claimed", "done", "blocked", "draft", "paused"} {
		m := newServerModel(t).WithServer(tickettest.FakeServerAPI{}).WithTickets(tickettest.FakeServerAPI{}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{}})
		m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: status},
			// Keeps the epic open: a fully done epic starts collapsed.
			{Address: "gx:alpha/02", Title: "Second", Status: "open"},
		}})
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = pressMenuKey(t, selectTicketRow(t, updated.(Model)))
		if !m.menus.Menu.IsOpen {
			t.Errorf("status %s: menu closed, want open", status)
		}

		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if updated.(Model).menus.Menu.IsOpen {
			t.Errorf("status %s: menu still open after esc", status)
		}
	}
}

// Seam D: on herdr, Go to pane jumps to the tab from the iterations read, and
// Resume issues resume.
func TestServerMode_GoToPaneAndResume(t *testing.T) {
	focused := fakeFocus(t)
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)

	var calls []string
	api := tickettest.FakeServerAPI{Calls: &calls, IterationsRes: []wire.IterationInfo{{Address: "gx:alpha/01", Pane: "p1", Tab: "t1"}}}
	m, cmd := chooseTicketMenuItem(t, openAnswerMenu(t, api), "Go to pane")
	cmd()
	if !slices.Equal(*focused, []string{"t1"}) {
		t.Fatalf("focused = %v", *focused)
	}

	_, cmd = chooseTicketMenuItem(t, pressMenuKey(t, m), resumeLabel)
	if msg, ok := cmd().(rowactions.ServerWriteMsg); !ok || msg.Verb != "resume" {
		t.Fatalf("resume result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"resume gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

// A paused (answer) ticket's menu has no Answer… action: answering is the web
// UI's job, and the TUI only offers Resume.
func TestServerMode_PausedAnswerMenuHasNoAnswerAction(t *testing.T) {
	m := openAnswerMenu(t, tickettest.FakeServerAPI{})
	view := m.menus.Menu.View()
	if strings.Contains(view, "Answer…") {
		t.Fatalf("menu offers Answer…:\n%s", view)
	}
	if !strings.Contains(view, resumeLabel) {
		t.Fatalf("menu lacks %s:\n%s", resumeLabel, view)
	}
}

func TestEpicsFromViewModel_KeepsTheDoneMetrics(t *testing.T) {
	vm := applySnapshot(viewmodel.State{}, wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "proj:epic-a/01", Status: "done", ActualContextWindow: 66395, ElapsedTime: 316, Compactions: 2},
	}})
	epics := tabapi.EpicsFromViewModel(vm, viewmodel.Scope{})
	if len(epics) != 1 || len(epics[0].Tickets) != 1 {
		t.Fatalf("epics = %+v", epics)
	}
	got := epics[0].Tickets[0]
	if got.ActualContextWindow != 66395 || got.ElapsedTime != 316 || got.Compactions != 2 {
		t.Errorf("ticket = %+v, want the done metrics so the summary can add them up", got)
	}
}

func TestEpicsFromViewModel_MultiLevelForkKeepsItsNumber(t *testing.T) {
	vm := applySnapshot(viewmodel.State{}, wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "proj:epic-a/27", Status: "done"},
		{Address: "proj:epic-a/27a", Status: "done", Parent: "proj:epic-a/27"},
		{Address: "proj:epic-a/27a1", Status: "done", Parent: "proj:epic-a/27a"},
		{Address: "proj:epic-a/27a3", Status: "claimed", Parent: "proj:epic-a/27a1"},
	}})
	epics := tabapi.EpicsFromViewModel(vm, viewmodel.Scope{})
	if len(epics) != 1 || len(epics[0].Tickets) != 4 {
		t.Fatalf("epics = %+v", epics)
	}
	epic := epics[0]
	for _, tk := range epic.Tickets {
		if tk.Number != 27 {
			t.Errorf("%s: Number = %d, want 27", tk.Identifier, tk.Number)
		}
	}
	if got := epic.RenderedStatus(epic.Tickets[3]); got != gxtickets.StatusClaimed {
		t.Errorf("27a3 RenderedStatus = %v, want claimed: its parent 27a1 is done", got)
	}
}
