package tickets

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tickets/sidebar"
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

func loadedModelWithTicket(t *testing.T) Model {
	t.Helper()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

// withLink delivers link with no state, as the shell does on a link change
// before any snapshot.
func withLink(m Model, link tabapi.ServerLink) (Model, tea.Cmd) {
	next, cmd := m.WithServerState(nil, link)
	return next.(Model), cmd
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

func TestServerMode_ToggleScopeRebuildsRows(t *testing.T) {
	m := withState(newServerModel(t).WithCwdProject("gx"), scopeSnap)
	if len(m.sidebar.Epics) != 1 {
		t.Fatalf("scoped epics = %+v", m.sidebar.Epics)
	}
	next, _ := m.toggleProjectScope()
	if got := len(next.(Model).sidebar.Epics); got != 2 {
		t.Fatalf("all-projects epics = %d, want 2", got)
	}
}

// The first-load hint comes from the app shell, so the tab stays quiet.
func TestServerMode_OutsideProjectShowsAllWithoutToasting(t *testing.T) {
	m := withState(newServerModel(t).WithCwdProject(""), scopeSnap)
	if len(m.sidebar.Epics) != 2 {
		t.Fatalf("epics = %d, want all", len(m.sidebar.Epics))
	}
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if got := toastsOf(cmd); len(got) != 0 {
		t.Fatalf("tab toasts = %+v", got)
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
	for _, e := range m.sidebar.BuildSidebarEntries(sidebarEnvFor(m)) {
		if e.Value.Kind == sidebar.NodeTicket {
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
	for _, e := range m.sidebar.BuildSidebarEntries(sidebarEnvFor(m)) {
		if e.Value.Kind == sidebar.NodeTicket {
			bodies = append(bodies, e.Body)
		}
	}
	if len(bodies) != 2 || len(bodies[0]) != 1 || !strings.Contains(bodies[0][0], "disk low: 2 GB free") || len(bodies[1]) != 0 {
		t.Fatalf("subtexts = %q", bodies)
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

func TestServerMode_FinishAndPauseKeyIsFinishAndPause(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls}).WithTickets(tickettest.FakeServerAPI{Calls: &calls}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}})
	next, _ := m.handleServerFinishAndPauseKey()
	m = next.(Model)
	if !m.confirm.IsOpen {
		t.Fatal("confirm not open")
	}
	_, cmd, _ := m.confirm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept produced no command")
	}
	if _, ok := cmd().(queue.ServerDoneMsg); !ok || !slices.Equal(calls, []string{"pause"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestRefresh_NoDiskPollWhileConnected(t *testing.T) {
	m := newServerModel(t)
	_, cmd := m.Update(epicsLoadedMsg{})
	if cmd != nil {
		t.Fatalf("a connected load scheduled %v; want no poll timer", cmd)
	}
}

// The shell tells the tab the store changed; a connected tab ignores it, and a
// down tab reloads. The tab keeps no loop of its own.
func TestRefresh_StoreChangedReloadsOnlyWhenDown(t *testing.T) {
	m := newServerModel(t)
	if _, cmd := m.Update(tabapi.StoreChangedMsg{}); cmd != nil {
		t.Fatal("a connected tab reloaded on a store change")
	}

	down, _ := withLink(m, tabapi.ServerLinkDown)
	_, cmd := down.Update(tabapi.StoreChangedMsg{})
	if cmd == nil {
		t.Fatal("a down tab must reload on a store change")
	}
	if _, ok := cmd().(epicsLoadedMsg); !ok {
		t.Fatal("a store change must be a single disk read, not a loop")
	}
}

func TestRefresh_ActivationReloadsOnlyWhenDown(t *testing.T) {
	m := newServerModel(t)
	if m.OnPageActivated() != nil {
		t.Fatal("a connected tab reloaded on activation")
	}
	down, _ := withLink(m, tabapi.ServerLinkDown)
	if down.OnPageActivated() == nil {
		t.Fatal("a down tab must reload on activation")
	}

	// The shell delivers the down link when the tab has no client at all.
	noClient, _ := withLink(NewModel(t.TempDir(), ui.Settings{}, keys.Manager{}), tabapi.ServerLinkDown)
	if noClient.Init() == nil {
		t.Fatal("a tab with no client must load from disk on Init")
	}
	if _, blocked := noClient.keyGuard(bindingTicketsFinishAndPause); !blocked {
		t.Fatal(`"D" is not blocked without a server`)
	}
}

func TestOlderServer_TicketsTabShowsNoticeAndNoTickets(t *testing.T) {
	m := newServerModel(t)
	st := viewmodel.State{}.ApplySnapshot(wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}}).SetQueue([]wire.QueueItem{{Address: "gx:alpha/01"}})
	next, _ := m.WithServerState(&st, tabapi.ServerLinkUp)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := ansi.Strip(next.View().Content)
	if !strings.Contains(view, tabapi.OlderServerNotice) || strings.Contains(view, "First") {
		t.Fatalf("view = %s", view)
	}
	if got := len(next.(Model).sidebar.Epics); got != 0 {
		t.Fatalf("epics = %d, want none (no ticket data for menu, preview or search)", got)
	}
}

func TestServerLink_ServerKeysDisabledWithReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		link tabapi.ServerLink
		want string
	}{
		{tabapi.ServerLinkDown, "server is down"},
		{tabapi.ServerLinkReadOnly, "read-only"},
	}
	for _, tc := range cases {
		m, _ := withLink(loadedModelWithTicket(t), tc.link)
		for _, key := range []string{"D"} {
			updated, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
			m = updated.(Model)
			if cmd == nil {
				t.Fatalf("link %v key %q: expected a disabled-with-reason notification", tc.link, key)
			}
			nm, ok := cmd().(notify.NotifyMsg)
			if !ok || !strings.Contains(nm.Message, tc.want) {
				t.Fatalf("link %v key %q: got %#v, want reason containing %q", tc.link, key, nm, tc.want)
			}
		}
	}
}

func TestServerLink_DownKeepsMarkdownRow(t *testing.T) {
	m, _ := withLink(loadedModelWithTicket(t), tabapi.ServerLinkDown)
	content := m.View().Content
	if !strings.Contains(content, "First ticket") {
		t.Fatalf("markdown row should stay, got:\n%s", content)
	}
}
