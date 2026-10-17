package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

var muted = []schema.MuteRecord{{EventType: "notification-storm"}}

func queueWithTicket(t *testing.T, m QueueModel, status string, mutes []schema.MuteRecord) QueueModel {
	t.Helper()
	m.epics[0].Tickets[0].Status = status
	m.epics[0].Tickets[0].Mutes = mutes
	m.clampSelected()
	m = selectFirstQueueTicketRow(t, m)
	return m
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

// With the link down the answer has no resume step, so it is saved and the
// toast follows (see TestTicketsTab_NoServer_AnswerSavesThenNeedsServer)
// without an unpark; with the link up the unpark is sent as before.
func TestAnswerResume_LinkDownSkipsTheUnpark(t *testing.T) {
	var calls []string
	api := fakeServerAPI{calls: &calls}
	if cmd := answerResume(api, ServerLinkDown, "gx:alpha/01"); cmd != nil {
		cmd()
		t.Fatalf("link down: resume = non-nil, calls = %v", calls)
	}
	if cmd := answerResume(nil, ServerLinkUp, "gx:alpha/01"); cmd != nil {
		t.Fatal("no api: resume = non-nil")
	}

	cmd := answerResume(api, ServerLinkUp, "gx:alpha/01")
	if cmd == nil {
		t.Fatal("link up: resume = nil")
	}
	cmd()
	if !slices.Equal(calls, []string{"changed gx:alpha/01", "unpark gx:alpha/01"}) {
		t.Fatalf("link up: calls = %v", calls)
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
