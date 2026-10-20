package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets/schema"
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
	body := "---\nid: \"01\"\ntitle: First\nstatus: needs-answer\n---\n\n# A\n\n## Needs Answer\n\nWhich approach?\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
