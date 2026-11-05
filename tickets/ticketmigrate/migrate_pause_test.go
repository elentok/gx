package ticketmigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/projectlock"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/tickettest"
)

// A migration step runs while its migration holds the lock exclusive. The
// pause unit writes tickets itself, so it must not wait for its own lock.
func TestApplyPauseMigration_RunsWhileProjectLockHeld(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	path := filepath.Join(dir, "epic", "issues", "01-a.md")
	tickettest.WriteFile(t, filepath.Join(dir, "project.json"), "{}")
	tickettest.WriteFile(t, path, "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\nBody.\n")
	unlock, err := projectlock.Exclusive(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	done := make(chan error, 1)
	go func() {
		_, err := ApplyPauseMigration([]PauseStep{{Path: path, Reason: schema.PausedReason("", schema.PauseClassAnswer)}})
		if err == nil {
			_, err = tickets.AppendLogInner(path, tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", Summary: "Which?", Text: "Which?"})
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("migration step waited for its own migration lock")
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "status: paused") {
		t.Errorf("ticket = %q, want status paused", raw)
	}
}
