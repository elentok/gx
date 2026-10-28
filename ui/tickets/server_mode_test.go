package tickets

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/viewmodel"
)

type fakeServerAPI struct {
	adds *[]server.QueueItem
	// addRes is what QueueAdd returns per address.
	addRes map[string]server.QueueResult
	// calls records the ticket write verbs as "verb address" and the
	// maintenance verbs by name.
	calls *[]string
	// replaced records the QueueReplace items; replaceRes is what it returns.
	replaced   *[]server.QueueItem
	replaceRes server.QueueResult
	removeRes  server.QueueResult
	mergeRes   server.QueueResult
	// detail and detailErr are what Detail returns for any address.
	detail     server.TicketDetail
	detailErr  error
	budget     server.BudgetStatus
	iterations []server.IterationInfo
	// designWorktree is what DesignWorktree returns.
	designWorktree    server.DesignWorktreeResult
	designWorktreeErr error
	// snap and queue are what Snapshot and QueueItems return.
	snap  server.Snapshot
	queue []server.QueueItem
}

// verb records the maintenance verbs ("pause", "resume", "override",
// "remove <addr>") so a test can assert which one a key reached; removeRes and
// budget are what QueueRemove and Budget return.
func (f fakeServerAPI) verb(v string) {
	if f.calls != nil {
		*f.calls = append(*f.calls, v)
	}
}

func (f fakeServerAPI) QueueRemove(_ context.Context, a string) (server.QueueResult, error) {
	f.verb("remove " + a)
	return f.removeRes, nil
}

func (f fakeServerAPI) QueuePause(context.Context) (server.QueueResult, error) {
	f.verb("pause")
	return server.QueueResult{Mode: "paused"}, nil
}

func (f fakeServerAPI) QueueResume(context.Context) (server.QueueResult, error) {
	f.verb("resume")
	return server.QueueResult{Mode: "running"}, nil
}

func (f fakeServerAPI) Budget(context.Context) (server.BudgetStatus, error) { return f.budget, nil }

func (f fakeServerAPI) BudgetOverride(context.Context) (server.BudgetResult, error) {
	f.verb("override")
	return server.BudgetResult{}, nil
}

func (f fakeServerAPI) QueueReplace(_ context.Context, _ string, items []server.QueueItem) (server.QueueResult, error) {
	if f.replaced != nil {
		*f.replaced = append(*f.replaced, items...)
	}
	return f.replaceRes, nil
}

func (f fakeServerAPI) QueueAdd(_ context.Context, address, agent string) (server.QueueResult, error) {
	if f.adds != nil {
		*f.adds = append(*f.adds, server.QueueItem{Address: address, Agent: agent})
	}
	return f.addRes[address], nil
}

func (f fakeServerAPI) TicketPause(_ context.Context, address, _ string) (server.QueueResult, error) {
	f.record("pause " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketCancel(_ context.Context, address string, _ bool) (server.QueueResult, error) {
	f.record("cancel " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketRelaunch(_ context.Context, address string, _ bool) (server.QueueResult, error) {
	f.record("relaunch " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketUnmute(_ context.Context, address string) (server.QueueResult, error) {
	f.record("unmute " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketApprove(_ context.Context, address string) (server.QueueResult, error) {
	f.record("approve " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) EpicMerge(_ context.Context, address string) (server.QueueResult, error) {
	f.record("merge " + address)
	return f.mergeRes, nil
}

func (f fakeServerAPI) EpicStashMerge(_ context.Context, address string) (server.QueueResult, error) {
	f.record("stash-merge " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) Detail(context.Context, string) (server.TicketDetail, error) {
	return f.detail, f.detailErr
}

func (f fakeServerAPI) Repair(_ context.Context, verb string, req server.RepairRequest) (server.RepairResult, error) {
	f.record(verb + " " + req.Address)
	return server.RepairResult{}, nil
}

func (f fakeServerAPI) DesignWorktree(_ context.Context, req server.DesignWorktreeRequest) (server.DesignWorktreeResult, error) {
	f.record("design-worktree " + req.Epic)
	return f.designWorktree, f.designWorktreeErr
}

func (f fakeServerAPI) record(call string) {
	if f.calls != nil {
		*f.calls = append(*f.calls, call)
	}
}

func (f fakeServerAPI) Snapshot(context.Context) (server.Snapshot, error) {
	return f.snap, nil
}
func (fakeServerAPI) Events(context.Context, uint64) (<-chan server.Event, error) { return nil, nil }
func (f fakeServerAPI) QueueItems(context.Context) ([]server.QueueItem, error)    { return f.queue, nil }

// withRenderedStatus makes snap look like one from a current server, which
// always sends rendered_status: a ticket without one gets its raw status word.
// A snapshot with none at all reads as an older server's.
func withRenderedStatus(snap server.Snapshot) server.Snapshot {
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

func applySnapshot(st viewmodel.State, snap server.Snapshot) viewmodel.State {
	return st.ApplySnapshot(withRenderedStatus(snap))
}

// pausedAnswerInfo and pausedRepairInfo are server tickets paused for each
// class: status paused, rendered as the class's word.
func pausedAnswerInfo(address, title string) server.TicketInfo {
	return server.TicketInfo{Address: address, Title: title, Status: "paused", RenderedStatus: gxtickets.StatusPausedAnswer.Word()}
}

func pausedRepairInfo(address, title string) server.TicketInfo {
	return server.TicketInfo{Address: address, Title: title, Status: "paused", RenderedStatus: gxtickets.StatusPausedRepair.Word()}
}

func newServerModel(t *testing.T) Model {
	t.Helper()
	return NewModel(t.TempDir(), ui.Settings{}, keys.New(nil)).WithServer(fakeServerAPI{})
}

// withState hands the tab the state the app shell would deliver for snap,
// then expands its open epics.
func withState(m Model, snap server.Snapshot) Model {
	st := applySnapshot(viewmodel.State{}, snap)
	next, _ := m.WithServerState(&st, ServerLinkUp)
	m = next.(Model)
	expandOpenEpics(&m)
	return m
}

func TestServerMode_SnapshotRendersReducedRows(t *testing.T) {
	snap := server.Snapshot{Seq: 4, Tickets: []server.TicketInfo{
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
	pending, _ := m.WithServerState(nil, ServerLinkUp)
	next, _ := pending.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m := next.(Model); m.loaded || !strings.Contains(m.View().Content, "loading…") {
		t.Fatalf("loaded=%v, want the loading… state", m.loaded)
	}
}

func TestServerMode_ResnapshotRequestFromRefresh(t *testing.T) {
	msg := newServerModel(t).cmdLoad()()
	if _, ok := msg.(ResnapshotRequestedMsg); !ok {
		t.Fatalf("refresh msg = %T, want ResnapshotRequestedMsg", msg)
	}
}

var scopeSnap = server.Snapshot{Seq: 4, Tickets: []server.TicketInfo{
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
	next, _ := m.WithServerState(&st, ServerLinkDown)
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
	snap := server.Snapshot{Seq: 1,
		Tickets: []server.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "open"}},
		Pending: []server.PendingRow{{Address: "gx:alpha/01", Verdict: "blocked", Reason: "waiting on 00"}},
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
	snap := server.Snapshot{Seq: 1,
		Tickets: []server.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: "open"},
			{Address: "gx:alpha/02", Title: "Second", Status: "open"},
		},
		Pending: []server.PendingRow{{Address: "gx:alpha/01", Verdict: "disk low", Reason: "2 GB free"}},
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
	m := newServerModel(t).WithServer(fakeServerAPI{calls: &calls, detail: detailWith(server.ActionResume)})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
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
	return serverModelWithStoreTicket(t, fakeServerAPI{}, false)
}

// serverModelWithStoreTicket serves a draft gx:alpha/20 whose file lives in a
// temp store, in a map epic when isMap is set.
func serverModelWithStoreTicket(t *testing.T, api fakeServerAPI, isMap bool) (Model, string) {
	t.Helper()
	store := t.TempDir()
	file := writeStoreTicket(t, store, "20-a.md", "---\nid: \"20\"\nstatus: draft\ntype: research\n---\n\n# 20 — A\n\nBody.\n")
	m := newServerModel(t).WithServer(api)
	m.settings.TicketStore = store
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
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

func (f fakeServerAPI) Iterations(context.Context) ([]server.IterationInfo, error) {
	return f.iterations, nil
}

type countingIterationsAPI struct {
	fakeServerAPI
	calls *int
}

func (c countingIterationsAPI) Iterations(ctx context.Context) ([]server.IterationInfo, error) {
	*c.calls++
	return c.fakeServerAPI.Iterations(ctx)
}

func TestCmdTicketMenu_FetchesIterationsOnce(t *testing.T) {
	calls := 0
	api := countingIterationsAPI{fakeServerAPI: fakeServerAPI{}, calls: &calls}
	in := newTicketMenuInput(gxtickets.Epic{}, gxtickets.Ticket{Path: "gx:alpha/01", Title: "First"}, false, "")

	cmdTicketMenu(api, in)()

	if calls != 1 {
		t.Errorf("Iterations calls = %d, want 1", calls)
	}
}

func (f fakeServerAPI) TicketChanged(_ context.Context, address string) error {
	f.record("changed " + address)
	return nil
}

func openAnswerMenu(t *testing.T, api fakeServerAPI) Model {
	t.Helper()
	if len(api.detail.Actions) == 0 {
		api.detail = detailWith(server.ActionAnswer, server.ActionResume)
	}
	m := newServerModel(t).WithServer(api)
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
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
		m := newServerModel(t).WithServer(fakeServerAPI{})
		m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: status},
			// Keeps the epic open: a fully done epic starts collapsed.
			{Address: "gx:alpha/02", Title: "Second", Status: "open"},
		}})
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = pressMenuKey(t, selectTicketRow(t, updated.(Model)))
		if !m.ticketMenu.IsOpen {
			t.Errorf("status %s: menu closed, want open", status)
		}

		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if updated.(Model).ticketMenu.IsOpen {
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
	api := fakeServerAPI{calls: &calls, iterations: []server.IterationInfo{{Address: "gx:alpha/01", Pane: "p1", Tab: "t1"}}}
	m, cmd := chooseTicketMenuItem(t, openAnswerMenu(t, api), "Go to pane")
	cmd()
	if !slices.Equal(*focused, []string{"t1"}) {
		t.Fatalf("focused = %v", *focused)
	}

	_, cmd = chooseTicketMenuItem(t, pressMenuKey(t, m), resumeLabel)
	if msg, ok := cmd().(serverWriteMsg); !ok || msg.verb != serverVerbResume {
		t.Fatalf("resume result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"resume gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

// A paused (answer) ticket's menu has no Answer… action: answering is the web
// UI's job, and the TUI only offers Resume.
func TestServerMode_PausedAnswerMenuHasNoAnswerAction(t *testing.T) {
	m := openAnswerMenu(t, fakeServerAPI{})
	view := m.ticketMenu.View()
	if strings.Contains(view, "Answer…") {
		t.Fatalf("menu offers Answer…:\n%s", view)
	}
	if !strings.Contains(view, resumeLabel) {
		t.Fatalf("menu lacks %s:\n%s", resumeLabel, view)
	}
}

func TestEpicsFromViewModel_KeepsTheDoneMetrics(t *testing.T) {
	vm := applySnapshot(viewmodel.State{}, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "proj:epic-a/01", Status: "done", ActualContextWindow: 66395, ElapsedTime: 316, Compactions: 2},
	}})
	epics := epicsFromViewModel(vm, viewmodel.Scope{})
	if len(epics) != 1 || len(epics[0].Tickets) != 1 {
		t.Fatalf("epics = %+v", epics)
	}
	got := epics[0].Tickets[0]
	if got.ActualContextWindow != 66395 || got.ElapsedTime != 316 || got.Compactions != 2 {
		t.Errorf("ticket = %+v, want the done metrics so the summary can add them up", got)
	}
}

func TestEpicsFromViewModel_MultiLevelForkKeepsItsNumber(t *testing.T) {
	vm := applySnapshot(viewmodel.State{}, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "proj:epic-a/27", Status: "done"},
		{Address: "proj:epic-a/27a", Status: "done", Parent: "proj:epic-a/27"},
		{Address: "proj:epic-a/27a1", Status: "done", Parent: "proj:epic-a/27a"},
		{Address: "proj:epic-a/27a3", Status: "claimed", Parent: "proj:epic-a/27a1"},
	}})
	epics := epicsFromViewModel(vm, viewmodel.Scope{})
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
