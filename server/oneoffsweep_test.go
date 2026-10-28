package server_test

import (
	"context"
	"fmt"
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
	servertest.WaitFor(t, 5*time.Second, fmt.Sprintf("%s: present = %v", path, present), func() bool {
		_, err := os.Stat(path)
		return (err == nil) == present
	})
}

func startSweep(t *testing.T, store string) *servertest.Harness {
	t.Helper()
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
	})
}

func TestSweep_ArchivesDoneAndCancelledOneOffsAndKeepsBrokenOnes(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	for id, status := range map[string]string{"01": "done", "02": "cancelled", "03": "paused", "04": "open"} {
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
	t.Parallel()
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

func TestSweep_DependentOfAnArchivedOneOffIsUnblockedLikeAnyTerminalBlocker(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	for id, status := range map[string]string{"01": "done", "02": "cancelled", "03": "open"} {
		servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, id, "t"+id, servertest.TicketOpts{Type: "prompt", Status: status})
	}
	for id, blocker := range map[string]string{"01": "one-offs/01", "02": "one-offs/02", "03": "one-offs/03"} {
		servertest.WriteTicketWith(t, store, "proj", "feature", id, "after", servertest.TicketOpts{BlockedBy: []string{blocker}})
	}
	h := startSweep(t, store)
	waitForFile(t, filepath.Join(proj, server.OneOffsEpic, "issues", "01-t01.md"), false)
	waitForFile(t, filepath.Join(proj, server.OneOffsEpic, "issues", "02-t02.md"), false)

	// "blocked" is the scheduler's word for a ticket with an unresolved blocker;
	// an unblocked one falls through to the queue verdicts.
	for id, want := range map[string]string{"01": "not queued", "02": "not queued", "03": "blocked"} {
		ex, err := h.Client.Explain(context.Background(), "proj:feature/"+id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if ex.Verdict != want {
			t.Errorf("feature/%s: explain = %+v, want verdict %q", id, ex, want)
		}
	}
}

func oneOffsQueued(t *testing.T, h *servertest.Harness) bool {
	t.Helper()
	q, err := h.Client.QueueItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range q {
		if strings.HasPrefix(it.Address, "proj:"+server.OneOffsEpic+"/") {
			return true
		}
	}
	return false
}

func waitOneOffsQueued(t *testing.T, h *servertest.Harness, want bool) {
	t.Helper()
	servertest.WaitFor(t, 5*time.Second, fmt.Sprintf("one-offs queued = %v", want), func() bool { return oneOffsQueued(t, h) == want })
}

func TestSweep_LastArchiveDequeuesTheRootAndASubmitRestoresIt(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	h := startSweep(t, store)
	ctx := context.Background()
	if _, err := h.Client.QueuePause(ctx); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "first", Project: "proj"})
	if err != nil || res.Refused {
		t.Fatalf("submit: %+v, %v", res, err)
	}
	waitOneOffsQueued(t, h, true)
	if cres, err := h.Client.TicketCancel(ctx, res.Address, false); err != nil || cres.Refused {
		t.Fatalf("cancel: %+v, %v", cres, err)
	}
	waitOneOffsQueued(t, h, false)

	res, err = h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "second", Project: "proj"})
	if err != nil || res.Refused {
		t.Fatalf("second submit: %+v, %v", res, err)
	}
	if !oneOffsQueued(t, h) {
		t.Fatal("second submit did not restore the one-offs root")
	}
}

func TestSweep_BrokenOneOffsKeepTheRootQueued(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"paused", "failed"} {
		t.Run(status, func(t *testing.T) {
			store := t.TempDir()
			servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: status})
			h := startSweep(t, store)
			ctx := context.Background()
			if _, err := h.Client.QueuePause(ctx); err != nil {
				t.Fatal(err)
			}
			var res server.QueueResult
			var err error
			if !servertest.Poll(5*time.Second, func() bool {
				res, err = h.Client.QueueAdd(ctx, "proj:one-offs/01", "")
				return err == nil && !res.Refused
			}) {
				t.Fatalf("queue add: %+v, %v", res, err)
			}
			time.Sleep(200 * time.Millisecond) // negative wait: several 50ms rescans pass; none may drop the root
			if !oneOffsQueued(t, h) {
				t.Fatalf("%s one-off lost the queue entry", status)
			}
		})
	}
}

func TestSweep_RacingSubmitsNeverLoseTheQueueItem(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	h := startSweep(t, store)
	ctx := context.Background()
	if _, err := h.Client.QueuePause(ctx); err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		res, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "p" + string(rune('a'+i)), Project: "proj"})
		if err != nil || res.Refused {
			t.Fatalf("submit %d: %+v, %v", i, res, err)
		}
		if cres, err := h.Client.TicketCancel(ctx, res.Address, false); err != nil || cres.Refused {
			t.Fatalf("cancel %d: %+v, %v", i, cres, err)
		}
	}
	last, err := h.Client.OneOff(ctx, server.OneOffRequest{Prompt: "last", Project: "proj"})
	if err != nil || last.Refused {
		t.Fatalf("last submit: %+v, %v", last, err)
	}
	time.Sleep(200 * time.Millisecond) // negative wait: several 50ms rescans pass; none may drop the root
	if !oneOffsQueued(t, h) {
		t.Fatal("the live one-off lost its queue entry")
	}
}

func TestSweep_CancelArchivesTheOneOff(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt"})
	h := startSweep(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The first scan may not have indexed the ticket yet.
	servertest.WaitFor(t, 5*time.Second, "the cancel to be accepted", func() bool {
		res, err := h.Client.TicketCancel(ctx, "proj:one-offs/01", false)
		if err == nil && !res.Refused {
			return true
		}
		if err != nil || res.Reason != "unknown-ticket" {
			t.Fatalf("cancel: %+v, %v", res, err)
		}
		return false
	})
	waitForFile(t, filepath.Join(proj, server.OneOffsEpic, "issues", "01-a.md"), false)
	got := readFile(t, filepath.Join(proj, server.OneOffsArchivedEpic, "issues", "01-a.md"))
	if !strings.Contains(got, "status: cancelled") || strings.Contains(got, "completed_at:") {
		t.Errorf("archived cancel:\n%s", got)
	}
}
