package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func waitForFile(t *testing.T, path string, present bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, err := os.Stat(path)
		if (err == nil) == present {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s: present = %v never reached", path, present)
}

func startSweep(t *testing.T, store string) *servertest.Harness {
	t.Helper()
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
	})
}

func TestSweep_ArchivesDoneAndCancelledOneOffsAndKeepsBrokenOnes(t *testing.T) {
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	for id, status := range map[string]string{"01": "done", "02": "cancelled", "03": "needs-repair", "04": "open"} {
		servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, id, "t"+id, servertest.TicketOpts{Type: "prompt", Status: status})
	}
	servertest.WriteTicketWith(t, store, "proj", "feature", "01", "after", servertest.TicketOpts{BlockedBy: []string{"one-offs/01"}})
	startSweep(t, store)

	archived := filepath.Join(proj, server.OneOffsArchivedEpic, "issues")
	waitForFile(t, filepath.Join(proj, server.OneOffsEpic, "issues", "01-t01.md"), false)
	waitForFile(t, filepath.Join(proj, server.OneOffsEpic, "issues", "02-t02.md"), false)

	done := readFile(t, filepath.Join(archived, "01-t01.md"))
	if !strings.Contains(done, "completed_at:") {
		t.Errorf("done one-off has no completed_at:\n%s", done)
	}
	if !strings.Contains(done, "- original: proj:one-offs/01") {
		t.Errorf("done one-off does not name its original address:\n%s", done)
	}
	cancelled := readFile(t, filepath.Join(archived, "02-t02.md"))
	if strings.Contains(cancelled, "completed_at:") {
		t.Errorf("cancelled one-off has a completed_at:\n%s", cancelled)
	}
	if !strings.Contains(cancelled, "- original: proj:one-offs/02") {
		t.Errorf("cancelled one-off does not name its original address:\n%s", cancelled)
	}
	for _, name := range []string{"03-t03.md", "04-t04.md"} {
		if _, err := os.Stat(filepath.Join(proj, server.OneOffsEpic, "issues", name)); err != nil {
			t.Errorf("%s left one-offs: %v", name, err)
		}
	}
	if blocker := readFile(t, filepath.Join(proj, "feature", "issues", "01-after.md")); !strings.Contains(blocker, "one-offs-archived/01") {
		t.Errorf("blocked_by was not rewritten to the archive:\n%s", blocker)
	}
}

func TestSweep_CrashBetweenArchiveWriteAndRemovalLeavesOneCopy(t *testing.T) {
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsArchivedEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	archivedCopy := filepath.Join(proj, server.OneOffsArchivedEpic, "issues", "01-a.md")
	if err := os.WriteFile(archivedCopy, []byte(readFile(t, archivedCopy)+"\n## Resolution\n\n- original: proj:one-offs/01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicketWith(t, store, "proj", "feature", "01", "after", servertest.TicketOpts{BlockedBy: []string{"one-offs/01"}})
	startSweep(t, store)

	waitForFile(t, filepath.Join(proj, server.OneOffsEpic, "issues", "01-a.md"), false)
	entries, err := os.ReadDir(filepath.Join(proj, server.OneOffsArchivedEpic, "issues"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("archive holds %d files (err %v), want 1", len(entries), err)
	}
	if blocker := readFile(t, filepath.Join(proj, "feature", "issues", "01-after.md")); !strings.Contains(blocker, "one-offs-archived/01") {
		t.Errorf("blocked_by of the half-moved ticket still dangles:\n%s", blocker)
	}
}

func TestSweep_CancelArchivesTheOneOff(t *testing.T) {
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt"})
	h := startSweep(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The first scan may not have indexed the ticket yet.
	for {
		res, err := h.Client.TicketCancel(ctx, "proj:one-offs/01", false)
		if err == nil && !res.Refused {
			break
		}
		if err != nil || res.Reason != "unknown-ticket" || ctx.Err() != nil {
			t.Fatalf("cancel: %+v, %v", res, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	waitForFile(t, filepath.Join(proj, server.OneOffsEpic, "issues", "01-a.md"), false)
	got := readFile(t, filepath.Join(proj, server.OneOffsArchivedEpic, "issues", "01-a.md"))
	if !strings.Contains(got, "status: cancelled") || strings.Contains(got, "completed_at:") {
		t.Errorf("archived cancel:\n%s", got)
	}
}
