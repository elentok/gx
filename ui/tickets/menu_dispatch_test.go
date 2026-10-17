package tickets

import (
	"os"
	"path/filepath"
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
