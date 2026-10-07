package tickets

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

type fakeServerAPI struct {
	adds *[]server.QueueItem
	// replaced records the QueueReplace items; replaceRes is what it returns.
	replaced   *[]server.QueueItem
	replaceRes server.QueueResult
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

func (fakeServerAPI) Snapshot(context.Context) (server.Snapshot, error) {
	return server.Snapshot{}, nil
}
func (fakeServerAPI) Events(context.Context, uint64) (<-chan server.Event, error) { return nil, nil }
func (fakeServerAPI) QueueItems(context.Context) ([]server.QueueItem, error)      { return nil, nil }

func newServerModel(t *testing.T) Model {
	t.Helper()
	return NewModelWithStore(t.TempDir(), ui.Settings{}, keys.New(nil), loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))).WithServer(fakeServerAPI{})
}

func TestServerMode_SnapshotRendersReducedRows(t *testing.T) {
	m := newServerModel(t)
	snap := server.Snapshot{Seq: 4, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Fork", Status: "open", Parent: "gx:alpha/01"},
	}}
	next, cmd, ok := m.updateServer(serverSnapshotMsg{snap: snap})
	if !ok || cmd == nil {
		t.Fatalf("snapshot not handled: ok=%v cmd=%v", ok, cmd)
	}
	if len(next.epics) != 1 || len(next.epics[0].Tickets) != 2 {
		t.Fatalf("epics = %+v", next.epics)
	}

	// An event the reducer applies changes the rows without a disk read.
	next, _, _ = next.updateServer(serverEventMsg{ev: server.Event{Seq: 5, Type: server.EventTicketDone, Address: "gx:alpha/01"}})
	if got := next.epics[0].Tickets[0].Status; got != "done" {
		t.Fatalf("status after event = %q", got)
	}
}

func TestServerMode_GapAndReconnectResnapshot(t *testing.T) {
	m := newServerModel(t)
	m, _, _ = m.updateServer(serverSnapshotMsg{snap: server.Snapshot{Seq: 4}})

	// A seq gap asks for a re-snapshot.
	_, cmd, _ := m.updateServer(serverEventMsg{ev: server.Event{Seq: 9, Type: server.EventQueueChanged}})
	if cmd == nil {
		t.Fatal("gap produced no command")
	}

	// The stream ending (reconnect) re-snapshots.
	_, cmd, ok := m.updateServer(serverStreamEndedMsg{})
	if !ok || cmd == nil {
		t.Fatal("stream end produced no command")
	}
	if _, isSnap := cmd().(serverSnapshotMsg); !isSnap {
		t.Fatalf("stream end cmd did not fetch a snapshot")
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

func closedEvents() <-chan server.Event {
	ch := make(chan server.Event)
	close(ch)
	return ch
}

func TestServerMode_ToastsFromLiveEventsOnly(t *testing.T) {
	m := newServerModel(t)
	snap := server.Snapshot{Seq: 4, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "needs-repair"},
	}}

	// A snapshot holding a parked ticket is state, not news.
	m, cmd, _ := m.updateServer(serverSnapshotMsg{snap: snap})
	if got := toastsOf(cmd); len(got) != 0 {
		t.Fatalf("snapshot toasted: %+v", got)
	}

	// A live park event toasts once.
	park := server.Event{Seq: 5, Type: server.EventIterationParked, Address: "gx:alpha/01"}
	m, cmd, _ = m.updateServer(serverEventMsg{ev: park, events: closedEvents()})
	if got := toastsOf(cmd); len(got) != 1 || got[0].Kind != notify.KindWarning {
		t.Fatalf("live park toasts = %+v", got)
	}

	// The same event replayed after a reconnect is a duplicate: no toast.
	_, cmd, _ = m.updateServer(serverEventMsg{ev: park, events: closedEvents()})
	if got := toastsOf(cmd); len(got) != 0 {
		t.Fatalf("replayed event toasted: %+v", got)
	}

	// A reconnect re-snapshot toasts nothing either.
	_, cmd, _ = m.updateServer(serverSnapshotMsg{snap: snap})
	if got := toastsOf(cmd); len(got) != 0 {
		t.Fatalf("re-snapshot toasted: %+v", got)
	}
}

func TestServerMode_PendingRowRendersVerdictSubtext(t *testing.T) {
	m := newServerModel(t)
	snap := server.Snapshot{Seq: 1,
		Tickets: []server.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "open"}},
		Pending: []server.PendingRow{{Address: "gx:alpha/01", Verdict: "blocked", Reason: "waiting on 00"}},
	}
	m, _, _ = m.updateServer(serverSnapshotMsg{snap: snap})

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

func TestServerMode_StartImplementSendsNoChat(t *testing.T) {
	m := newServerModel(t)
	m.settings.Notifications.Telegram = config.TelegramConfig{BotToken: "tok", ChatID: "42"}
	// The server sends chat once; the TUI path must not wire a sink.
	if got := m.notificationsForRun(); got.Telegram.BotToken != "" {
		t.Fatalf("server mode kept chat sink: %+v", got)
	}
}

// Seam D: the "a" confirm renders the agent picker and the client call carries
// the chosen agent.
func TestServerMode_EnqueueKeyPicksAgent(t *testing.T) {
	var adds []server.QueueItem
	m := newServerModel(t).WithServer(fakeServerAPI{adds: &adds})
	m, _, _ = m.updateServer(serverSnapshotMsg{snap: server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Second", Status: "done"},
	}}})
	m.checked = map[string]bool{"gx:alpha/01": true, "gx:alpha/02": true}

	next, _ := m.handleAddToQueueKey()
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
	m, _, _ = m.updateServer(serverSnapshotMsg{snap: server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}}})
	m.checked = map[string]bool{"gx:alpha/01": true}

	next, _ := m.handleReplaceQueueKey()
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

func TestServerMode_QuitNotGuarded(t *testing.T) {
	if !newServerModel(t).CanQuit() {
		t.Fatal("server mode must not guard quit")
	}
}
