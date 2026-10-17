package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

var muted = []schema.MuteRecord{{EventType: "notification-storm"}}

// chooseAction moves an open menu's cursor to value and accepts it,
// returning the accept's cmd.
func chooseAction(t *testing.T, menu components.MenuState, update func(tea.Msg) tea.Cmd, value string) tea.Cmd {
	t.Helper()
	idx := slices.Index(menuValues(menu), value)
	if idx < 0 {
		t.Fatalf("menu %v has no %q", menuValues(menu), value)
	}
	for range idx {
		update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	cmd := update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("%q issued no cmd", value)
	}
	return cmd
}

func chooseQueueAction(t *testing.T, m QueueModel, value string) tea.Cmd {
	t.Helper()
	m = pressQueue(t, m, 'm')
	return chooseAction(t, m.actionsMenu.state, func(msg tea.Msg) tea.Cmd {
		next, cmd := m.Update(msg)
		m = next.(QueueModel)
		return cmd
	}, value)
}

func queueWithTicket(m QueueModel, status string, mutes []schema.MuteRecord) QueueModel {
	m.epics[0].Tickets[0].Status = status
	m.epics[0].Tickets[0].Mutes = mutes
	m.clampSelected()
	return m
}

func TestQueueMenu_ResumeUnparksByAddress(t *testing.T) {
	var calls []string
	m := queueWithTicket(newMaintenanceQueue(t, fakeServerAPI{calls: &calls}), "needs-answer", nil)
	if msg, ok := chooseQueueAction(t, m, actionResume)().(serverWriteMsg); !ok || msg.problem != "" {
		t.Fatalf("resume result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"unpark gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestQueueMenu_UnmuteByAddress(t *testing.T) {
	var calls []string
	m := queueWithTicket(newMaintenanceQueue(t, fakeServerAPI{calls: &calls}), "needs-repair", muted)
	chooseQueueAction(t, m, actionUnmuteReopen)()
	if !slices.Equal(calls, []string{"unmute gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

// writeTicketFile writes a needs-answer ticket file and returns its path.
func writeTicketFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-first.md")
	body := "---\nid: \"01\"\ntitle: First\nstatus: needs-answer\n---\n\n" + answerTicket
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertServerNotRunning(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Message != serverNotRunning {
		t.Fatalf("result = %+v, want the %q toast", msg, serverNotRunning)
	}
}

func TestQueueMenu_NoServer_WritesNothing(t *testing.T) {
	for _, tt := range []struct {
		action, status string
		mutes          []schema.MuteRecord
	}{
		{actionResume, "needs-answer", nil},
		{actionUnmuteReopen, "needs-repair", muted},
	} {
		t.Run(tt.action, func(t *testing.T) {
			path := writeTicketFile(t)
			before, _ := os.ReadFile(path)
			m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil))
			next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			m = next.(QueueModel)
			m.loaded = true
			m.epics = []tickets.Epic{{Name: "alpha", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Title: "First", Path: path, Status: tt.status, Mutes: tt.mutes}}}}
			m.checked = map[string]bool{path: true}
			m.clampSelected()

			assertServerNotRunning(t, chooseQueueAction(t, m, tt.action))
			if after, _ := os.ReadFile(path); string(after) != string(before) {
				t.Errorf("ticket file changed:\n%s", after)
			}
		})
	}
}

func TestTicketsTab_NoServer_AnswerSavesThenNeedsServer(t *testing.T) {
	path := writeTicketFile(t)
	before := needsAnswerSection(answerTicket)
	raw, _ := os.ReadFile(path)
	answered := strings.Replace(string(raw), "Which approach?", "Which approach?\n\nA.", 1)
	if err := os.WriteFile(path, []byte(answered), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(t.TempDir(), ui.Settings{}, keys.New(nil))
	next, cmd := m.Update(answerEditorFinishedMsg{path: path, before: before})
	if next.(Model).confirm.IsOpen {
		t.Error("resume confirm opened with no server")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Message != answerSavedNeedsServer {
		t.Fatalf("result = %+v, want %q", msg, answerSavedNeedsServer)
	}
	if after, _ := os.ReadFile(path); string(after) != answered {
		t.Errorf("ticket file changed after the answer:\n%s", after)
	}
}
