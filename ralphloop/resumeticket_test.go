package ralphloop

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

func TestResumeTicket_DemotesPausedSection(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: paused\npause_reason: zero-commit\ntype: implement\n---\n# A\n\n## Paused\n\nwaiting on a person\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")

	if err := ResumeTicket(path, time.Now()); err != nil {
		t.Fatalf("ResumeTicket: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "\n## Paused\n") {
		t.Errorf("Paused section left behind:\n%s", raw)
	}
	if !strings.Contains(string(raw), "retired from `## Paused`") {
		t.Errorf("Paused section not demoted into Comments:\n%s", raw)
	}
}

// TestUnmuteTicket_Paused_ClearsMutesDemotesReopens covers a mute that
// actually paused its ticket (status: paused (repair), written by muteSource's
// pauseTicket callback): UnmuteTicket clears Mutes, demotes the "## Needs
// Repair" section into "## Comments", reopens the ticket, and appends its
// own unmute audit note.
func TestUnmuteTicket_Paused_ClearsMutesDemotesReopens(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-repair\ntype: implement\nmutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n---\n# A\n\n## Needs Repair\n\nnotification storm tripped a mute\n")

	if err := UnmuteTicket(path, time.Now()); err != nil {
		t.Fatalf("UnmuteTicket: %v", err)
	}

	ticket := mustParse(t, path)
	if ticket.Status != schema.StatusOpen {
		t.Errorf("Status = %q, want %q", ticket.Status, schema.StatusOpen)
	}
	if len(ticket.Mutes) != 0 {
		t.Errorf("Mutes = %v, want empty", ticket.Mutes)
	}

	raw := mustRead(t, path)
	if strings.Contains(raw, "\n## Needs Repair\n") {
		t.Errorf("Needs Repair section not demoted:\n%s", raw)
	}
	if !strings.Contains(raw, "## Comments") || !strings.Contains(raw, "retired from `## Needs Repair`") {
		t.Errorf("Needs Repair not retired into Comments:\n%s", raw)
	}
	if !strings.Contains(raw, "— unmuted") {
		t.Errorf("unmute audit note missing:\n%s", raw)
	}
}

// TestUnmuteTicket_Resumed_ClearsMutesLeavesStatus covers a mute that
// tripped with no pauseTicket callback available (see muteSource) — the
// ticket's status was never touched, so there's no pause to undo: clearing
// Mutes (plus the audit note) is the whole action.
func TestUnmuteTicket_Resumed_ClearsMutesLeavesStatus(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: open\ntype: implement\nmutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n---\n# A\n\nBody text.\n")

	if err := UnmuteTicket(path, time.Now()); err != nil {
		t.Fatalf("UnmuteTicket: %v", err)
	}

	ticket := mustParse(t, path)
	if ticket.Status != schema.StatusOpen {
		t.Errorf("Status = %q, want unchanged %q", ticket.Status, schema.StatusOpen)
	}
	if len(ticket.Mutes) != 0 {
		t.Errorf("Mutes = %v, want empty", ticket.Mutes)
	}

	raw := mustRead(t, path)
	if !strings.Contains(raw, "— unmuted") {
		t.Errorf("unmute audit note missing:\n%s", raw)
	}
}

// waitForFileContains polls path until its content contains want, failing
// the test if that never happens within a few seconds.
func waitForFileContains(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(raw), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to contain %q", path, want)
}
