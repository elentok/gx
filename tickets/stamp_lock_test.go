package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// A `set`/`section` write that lands while the stamp is in flight holds the
// ticket lock; the stamp must wait for it and re-read, not write back stale
// content over it.
func TestStampEpicStarted_TicketMDWaitsForLockAndKeepsConcurrentWrite(t *testing.T) {
	dir := t.TempDir()
	ticketPath := filepath.Join(dir, "epic", "ticket.md")
	writeFile(t, ticketPath, "---\nstatus: open\n---\n\nold body\n")

	unlock, err := schema.LockTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- StampEpicStarted(dir, "epic", time.Now()) }()

	// Let the stamp reach the lock (or, unfixed, read and write straight away).
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(ticketPath, []byte("---\nstatus: claimed\n---\n\nnew body\n"), 0644); err != nil {
		t.Fatal(err)
	}
	unlock()

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "status: claimed") || !strings.Contains(got, "new body") {
		t.Errorf("concurrent write lost:\n%s", got)
	}
	if !strings.Contains(got, "started_at:") {
		t.Errorf("stamp missing:\n%s", got)
	}
}
