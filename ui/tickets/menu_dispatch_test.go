package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

var muted = tickettest.Muted

// writeMenuTicketFile writes a paused (answer) ticket file and returns its path.
func writeMenuTicketFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-first.md")
	body := "---\nid: \"01\"\ntitle: First\nstatus: paused\npause_reason: blocked-pane\n---\n\n# A\n\n" + tickets.LegacyAnswerHeading + "\n\nWhich approach?\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
