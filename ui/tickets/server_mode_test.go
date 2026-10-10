package tickets

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/viewmodel"
)

type fakeServerAPI struct {
	adds *[]server.QueueItem
	// calls records the ticket write verbs as "verb address" and the
	// maintenance verbs by name.
	calls *[]string
	// replaced records the QueueReplace items; replaceRes is what it returns.
	replaced   *[]server.QueueItem
	replaceRes server.QueueResult
	removeRes  server.QueueResult
	budget     server.BudgetStatus
	iterations []server.IterationInfo
	// snap and queue are what Snapshot and QueueItems return.
	snap  server.Snapshot
	queue []server.QueueItem
}

// verb records the maintenance verbs ("drain", "pause", "resume", "override",
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

func (f fakeServerAPI) QueueDrain(context.Context) (server.QueueResult, error) {
	f.verb("drain")
	return server.QueueResult{Mode: "draining"}, nil
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
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketPark(_ context.Context, address, _ string) (server.QueueResult, error) {
	f.record("park " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketCancel(_ context.Context, address string, _ bool) (server.QueueResult, error) {
	f.record("cancel " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketRelaunch(_ context.Context, address string) (server.QueueResult, error) {
	f.record("relaunch " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) TicketApprove(_ context.Context, address string) (server.QueueResult, error) {
	f.record("approve " + address)
	return server.QueueResult{}, nil
}

func (f fakeServerAPI) Repair(_ context.Context, verb string, req server.RepairRequest) (server.RepairResult, error) {
	f.record(verb + " " + req.Address)
	return server.RepairResult{}, nil
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

func newServerModel(t *testing.T) Model {
	t.Helper()
	return NewModel(t.TempDir(), ui.Settings{}, keys.New(nil)).WithServer(fakeServerAPI{})
}

// withState hands the tab the state the app shell would deliver for snap.
func withState(m Model, snap server.Snapshot) Model {
	st := viewmodel.State{}.ApplySnapshot(snap)
	next, _ := m.WithServerState(&st, ServerLinkUp)
	return next.(Model)
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
	st := viewmodel.State{}.ApplySnapshot(scopeSnap)
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

// Seam D: the "a" confirm renders the agent picker and the client call carries
// the chosen agent.
func TestServerMode_EnqueueKeyPicksAgent(t *testing.T) {
	var adds []server.QueueItem
	m := newServerModel(t).WithServer(fakeServerAPI{adds: &adds})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Second", Status: "done"},
	}})
	m.checked = map[string]bool{"gx:alpha/01": true, "gx:alpha/02": true}

	next, _ := m.handleServerEnqueueKey()
	m = next.(Model)
	if !m.confirm.IsOpen {
		t.Fatal("confirm not open")
	}
	if view := m.confirm.View(80); !strings.Contains(view, "Agent: claude") || !strings.Contains(view, "Enqueue 1 ticket") {
		t.Fatalf("confirm view = %q", view)
	}

	m.confirm, _, _ = m.confirm.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.confirm.Choice(); got != "codex" {
		t.Fatalf("choice after tab = %q", got)
	}
	var cmd tea.Cmd
	m.confirm, cmd, _ = m.confirm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept produced no command")
	}
	if _, ok := cmd().(serverEnqueuedMsg); !ok {
		t.Fatal("accept did not enqueue")
	}
	want := []server.QueueItem{{Address: "gx:alpha/01", Agent: "codex"}}
	if !slices.Equal(adds, want) {
		t.Fatalf("adds = %+v, want %+v", adds, want)
	}
}

// Seam D: "r" replaces with the chosen agent; a ticket-live refusal becomes an
// error toast.
func TestServerMode_ReplaceKeyPicksAgentAndShowsRefusal(t *testing.T) {
	var replaced []server.QueueItem
	api := fakeServerAPI{replaced: &replaced}
	m := newServerModel(t).WithServer(api)
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}})
	m.checked = map[string]bool{"gx:alpha/01": true}

	next, _ := m.handleServerReplaceKey()
	m = next.(Model)
	if !m.confirm.IsOpen {
		t.Fatal("confirm not open")
	}
	m.confirm, _, _ = m.confirm.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	_, cmd, _ := m.confirm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept produced no command")
	}
	if _, ok := cmd().(serverReplacedMsg); !ok {
		t.Fatal("accept did not replace")
	}
	want := []server.QueueItem{{Address: "gx:alpha/01", Agent: "codex"}}
	if !slices.Equal(replaced, want) {
		t.Fatalf("replaced = %+v, want %+v", replaced, want)
	}

	api.replaceRes = server.QueueResult{Refused: true, Reason: server.ReasonTicketLive, Message: "gx:alpha/01 has a live iteration"}
	msg := m.WithServer(api).cmdServerReplace("gx", []string{"gx:alpha/01"}, "claude")()
	if got := msg.(serverReplacedMsg).problem; !strings.Contains(got, "live iteration") {
		t.Fatalf("problem = %q", got)
	}
}

// Seam D: the "s" menu lists the server actions for a ticket's state and
// issues the chosen one; "enter" on a parked row issues unpark.
func TestServerMode_StatusMenuAndEnterUnpark(t *testing.T) {
	var calls []string
	api := fakeServerAPI{calls: &calls}
	m := newServerModel(t).WithServer(api)
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Second", Status: "needs-repair"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if got, want := menuValues(m.statusMenu), []string{"server:park", "server:cancel", "server:relaunch"}; !slices.Equal(got, want) {
		t.Fatalf("open ticket menu = %v, want %v", got, want)
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if msg, ok := cmd().(serverWriteMsg); !ok || msg.problem != "" {
		t.Fatalf("park result = %+v", msg)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if got, want := menuValues(m.statusMenu), []string{"server:unpark", "server:cancel", "server:relaunch"}; !slices.Equal(got, want) {
		t.Fatalf("parked ticket menu = %v, want %v", got, want)
	}
	m.statusMenuOpen = false

	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a parked row issued nothing")
	}
	cmd()
	if want := []string{"park gx:alpha/01", "unpark gx:alpha/02"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

// In server mode a row's Path is the ticket's address, so the "s" menu's
// direct statuses and the edit chords must reach the ticket's file instead.
func serverModelWithTicketFile(t *testing.T) (Model, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "20-a.md")
	if err := os.WriteFile(file, []byte("---\nid: \"20\"\nstatus: draft\ntype: research\n---\n\n# 20 — A\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newServerModel(t)
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/20", Title: "A", Status: "draft", File: file},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return selectTicketRow(t, updated.(Model)), file
}

func TestServerMode_StatusMenuOffersDirectStatusesAndWritesTheFile(t *testing.T) {
	m, file := serverModelWithTicketFile(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if got, want := menuValues(m.statusMenu), []string{"open", "done", "server:park", "server:cancel", "server:relaunch"}; !slices.Equal(got, want) {
		t.Fatalf("draft ticket menu = %v, want %v", got, want)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("choosing done issued nothing")
	}
	cmd()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket file = %s, want status: done", raw)
	}
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

func (f fakeServerAPI) TicketChanged(_ context.Context, address string) error {
	f.record("changed " + address)
	return nil
}

func openAnswerMenu(t *testing.T, api fakeServerAPI) Model {
	t.Helper()
	m := newServerModel(t).WithServer(api)
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "needs-answer"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	return updated.(Model)
}

// Seam D: Answer in pane targets the tab from the iterations read, and Resume
// issues unpark.
func TestServerMode_AnswerInPaneAndResume(t *testing.T) {
	var focused []string
	defer func(prev func(string) error) { focusIterationTab = prev }(focusIterationTab)
	focusIterationTab = func(tab string) error { focused = append(focused, tab); return nil }

	var calls []string
	api := fakeServerAPI{calls: &calls, iterations: []server.IterationInfo{{Address: "gx:alpha/01", Pane: "p1", Tab: "t1"}}}
	m := openAnswerMenu(t, api)
	if got, want := menuValues(m.actionsMenu.state), []string{actionAnswerInPane, actionResumeAnswered, actionInvestigate}; !slices.Equal(got, want) {
		t.Fatalf("menu = %v, want %v", got, want)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cmd()
	if !slices.Equal(focused, []string{"t1"}) {
		t.Fatalf("focused = %v", focused)
	}

	m = updated.(Model)
	m, _ = func() (Model, tea.Cmd) { u, c := m.handleSuggestedActionsKey(); return u.(Model), c }()
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updated, cmd = updated.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if msg, ok := cmd().(serverWriteMsg); !ok || msg.verb != serverVerbUnpark {
		t.Fatalf("resume result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"unpark gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

// Seam D: with no live pane the menu offers Answer…, which writes the ticket
// file directly; its resume pings the server and then unparks.
func TestServerMode_AnswerEditsFileThenPingsAndUnparks(t *testing.T) {
	var calls []string
	m := openAnswerMenu(t, fakeServerAPI{calls: &calls})
	if got := menuValues(m.actionsMenu.state)[0]; got != actionAnswer {
		t.Fatalf("first item = %q", got)
	}
	store := t.TempDir()
	dir := filepath.Join(store, "gx", "alpha", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "01-first.md")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ticketFile(store, "gx:alpha/01"); err != nil || got != want {
		t.Fatalf("ticketFile = %q, %v", got, err)
	}

	m = m.WithTicketStore(store)
	if msg, ok := m.cmdServerResumeAnswered("gx:alpha/01")().(serverWriteMsg); !ok || msg.problem != "" {
		t.Fatalf("resume result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"changed gx:alpha/01", "unpark gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestEpicsFromViewModel_KeepsTheLandingMetrics(t *testing.T) {
	vm := viewmodel.State{}.ApplySnapshot(server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "proj:epic-a/01", Status: "done", ActualContextWindow: 66395, ElapsedTime: 316, Compactions: 2},
	}})
	epics := epicsFromViewModel(vm, viewmodel.Scope{})
	if len(epics) != 1 || len(epics[0].Tickets) != 1 {
		t.Fatalf("epics = %+v", epics)
	}
	got := epics[0].Tickets[0]
	if got.ActualContextWindow != 66395 || got.ElapsedTime != 316 || got.Compactions != 2 {
		t.Errorf("ticket = %+v, want the landing metrics so the summary can add them up", got)
	}
}

func TestEpicsFromViewModel_MultiLevelForkKeepsItsNumber(t *testing.T) {
	vm := viewmodel.State{}.ApplySnapshot(server.Snapshot{Tickets: []server.TicketInfo{
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

func TestServerMode_EnqueueConfirmShowsExtraUsageBanner(t *testing.T) {
	m := newServerModel(t)
	open := func(extra bool) string {
		m.vm.ExtraUsage = extra
		m.confirm = m.openAgentConfirm("Enqueue 1 ticket(s)?", func(string) tea.Cmd { return nil })
		return m.confirm.View(200)
	}
	if got := open(true); !strings.Contains(got, "auto-purchase extra usage") {
		t.Fatalf("confirm lacks the banner:\n%s", got)
	}
	if got := open(false); strings.Contains(got, "extra usage") {
		t.Fatalf("confirm shows the banner while extra usage is off:\n%s", got)
	}
}
