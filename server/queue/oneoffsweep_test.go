package queue_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
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

// flakyHost fails EnsureDraftEpic (the first step of an archive) while failing is set.
type flakyHost struct {
	queue.Host
	failing  atomic.Bool
	attempts atomic.Int32
}

func (h *flakyHost) EnsureDraftEpic(path string) error {
	h.attempts.Add(1)
	if h.failing.Load() {
		return errors.New("archive blocked")
	}
	return os.MkdirAll(filepath.Join(path, "issues"), 0o755)
}

func TestSweep_StuckOneOffBacksOffToTheCapAndWarnsOncePerTicket(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	host := &flakyHost{}
	host.failing.Store(true)
	o := queue.NewOneOffs(queue.OneOffsDeps{
		Queue:       &queue.Queue{},
		Index:       index.New(store, broker.New(16), log),
		Registry:    &runs.Registry{},
		Log:         log,
		Host:        host,
		TicketStore: store,
	})

	clock := time.Now()
	o.SweepAt(clock)
	if got := host.attempts.Load(); got != 1 {
		t.Fatalf("first sweep attempts = %d, want 1", got)
	}
	// 1m, 2m, 4m ... 64m: the last step is clamped to the 1 hour cap.
	for k := range 8 {
		delay := min(time.Minute<<k, time.Hour)
		before := host.attempts.Load()
		o.SweepAt(clock.Add(delay - time.Second))
		if host.attempts.Load() != before {
			t.Fatalf("retried before the %v back-off", delay)
		}
		clock = clock.Add(delay)
		o.SweepAt(clock)
		if got := host.attempts.Load(); got != before+1 {
			t.Fatalf("attempts after the %v back-off = %d, want %d", delay, got, before+1)
		}
	}
	if n := strings.Count(logs.String(), "level=WARN"); n != 1 {
		t.Errorf("warn lines = %d, want exactly 1:\n%s", n, logs.String())
	}

	host.failing.Store(false)
	o.SweepAt(clock.Add(time.Hour))
	waitForFile(t, filepath.Join(store, "proj", queue.OneOffsEpic, "issues", "01-a.md"), false)

	// The back-off is cleared: a fresh failure for the same address retries after 1 minute.
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	host.failing.Store(true)
	later := clock.Add(2 * time.Hour)
	o.SweepAt(later)
	before := host.attempts.Load()
	o.SweepAt(later.Add(time.Minute))
	if host.attempts.Load() != before+1 {
		t.Errorf("a cleared back-off did not retry after 1 minute")
	}
}

func TestSweep_ArchivesDoneAndCancelledOneOffsAndKeepsBrokenOnes(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	for id, status := range map[string]string{"01": "done", "02": "cancelled", "03": "paused", "04": "open"} {
		servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, id, "t"+id, servertest.TicketOpts{Type: "prompt", Status: status})
	}
	servertest.WriteTicketWith(t, store, "proj", "feature", "01", "after", servertest.TicketOpts{BlockedBy: []string{"one-offs/01"}})
	startSweep(t, store)

	archived := filepath.Join(proj, queue.OneOffsArchivedEpic, "issues")
	waitForFile(t, filepath.Join(proj, queue.OneOffsEpic, "issues", "01-t01.md"), false)
	waitForFile(t, filepath.Join(proj, queue.OneOffsEpic, "issues", "02-t02.md"), false)

	done := servertest.ReadFile(t, filepath.Join(archived, "01-t01.md"))
	if !strings.Contains(done, "completed_at:") {
		t.Errorf("done one-off has no completed_at:\n%s", done)
	}
	if !strings.Contains(done, "- original: proj:one-offs/01") {
		t.Errorf("done one-off does not name its original address:\n%s", done)
	}
	cancelled := servertest.ReadFile(t, filepath.Join(archived, "02-t02.md"))
	if strings.Contains(cancelled, "completed_at:") {
		t.Errorf("cancelled one-off has a completed_at:\n%s", cancelled)
	}
	if !strings.Contains(cancelled, "- original: proj:one-offs/02") {
		t.Errorf("cancelled one-off does not name its original address:\n%s", cancelled)
	}
	for _, name := range []string{"03-t03.md", "04-t04.md"} {
		if _, err := os.Stat(filepath.Join(proj, queue.OneOffsEpic, "issues", name)); err != nil {
			t.Errorf("%s left one-offs: %v", name, err)
		}
	}
	if blocker := servertest.ReadFile(t, filepath.Join(proj, "feature", "issues", "01-after.md")); !strings.Contains(blocker, "one-offs-archived/01") {
		t.Errorf("blocked_by was not rewritten to the archive:\n%s", blocker)
	}
}

func TestSweep_CrashBetweenArchiveWriteAndRemovalLeavesOneCopy(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsArchivedEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	archivedCopy := filepath.Join(proj, queue.OneOffsArchivedEpic, "issues", "01-a.md")
	if err := os.WriteFile(archivedCopy, []byte(servertest.ReadFile(t, archivedCopy)+"\n## Resolution\n\n- original: proj:one-offs/01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicketWith(t, store, "proj", "feature", "01", "after", servertest.TicketOpts{BlockedBy: []string{"one-offs/01"}})
	startSweep(t, store)

	waitForFile(t, filepath.Join(proj, queue.OneOffsEpic, "issues", "01-a.md"), false)
	entries, err := os.ReadDir(filepath.Join(proj, queue.OneOffsArchivedEpic, "issues"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("archive holds %d files (err %v), want 1", len(entries), err)
	}
	if blocker := servertest.ReadFile(t, filepath.Join(proj, "feature", "issues", "01-after.md")); !strings.Contains(blocker, "one-offs-archived/01") {
		t.Errorf("blocked_by of the half-moved ticket still dangles:\n%s", blocker)
	}
}

func TestSweep_DependentOfAnArchivedOneOffIsUnblockedLikeAnyTerminalBlocker(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	for id, status := range map[string]string{"01": "done", "02": "cancelled", "03": "open"} {
		servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, id, "t"+id, servertest.TicketOpts{Type: "prompt", Status: status})
	}
	for id, blocker := range map[string]string{"01": "one-offs/01", "02": "one-offs/02", "03": "one-offs/03"} {
		servertest.WriteTicketWith(t, store, "proj", "feature", id, "after", servertest.TicketOpts{BlockedBy: []string{blocker}})
	}
	h := startSweep(t, store)
	waitForFile(t, filepath.Join(proj, queue.OneOffsEpic, "issues", "01-t01.md"), false)
	waitForFile(t, filepath.Join(proj, queue.OneOffsEpic, "issues", "02-t02.md"), false)

	// "blocked" is the scheduler's word for a ticket with an unresolved blocker;
	// an unblocked one falls through to the queue verdicts.
	for id, want := range map[string]string{"01": "not queued", "02": "not queued", "03": "blocked"} {
		ex, err := h.Client.Reads.Explain(context.Background(), "proj:feature/"+id)
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
	q, err := h.Client.Queue.Items(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range q {
		if strings.HasPrefix(it.Address, "proj:"+queue.OneOffsEpic+"/") {
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
	if _, err := h.Client.Queue.Pause(ctx); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "first", Project: "proj"})
	if err != nil || res.Refused {
		t.Fatalf("submit: %+v, %v", res, err)
	}
	waitOneOffsQueued(t, h, true)
	if cres, err := h.Client.Tickets.Cancel(ctx, res.Address, false); err != nil || cres.Refused {
		t.Fatalf("cancel: %+v, %v", cres, err)
	}
	waitOneOffsQueued(t, h, false)

	res, err = h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "second", Project: "proj"})
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
			servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: status})
			h := startSweep(t, store)
			ctx := context.Background()
			if _, err := h.Client.Queue.Pause(ctx); err != nil {
				t.Fatal(err)
			}
			var res wire.QueueResult
			var err error
			if !servertest.Poll(5*time.Second, func() bool {
				res, err = h.Client.Queue.Add(ctx, "proj:one-offs/01", "")
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
	if _, err := h.Client.Queue.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		res, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "p" + string(rune('a'+i)), Project: "proj"})
		if err != nil || res.Refused {
			t.Fatalf("submit %d: %+v, %v", i, res, err)
		}
		if cres, err := h.Client.Tickets.Cancel(ctx, res.Address, false); err != nil || cres.Refused {
			t.Fatalf("cancel %d: %+v, %v", i, cres, err)
		}
	}
	last, err := h.Client.Queue.OneOff(ctx, wire.OneOffRequest{Prompt: "last", Project: "proj"})
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
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt"})
	h := startSweep(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The first scan may not have indexed the ticket yet.
	servertest.WaitFor(t, 5*time.Second, "the cancel to be accepted", func() bool {
		res, err := h.Client.Tickets.Cancel(ctx, "proj:one-offs/01", false)
		if err == nil && !res.Refused {
			return true
		}
		if err != nil || res.Reason != "unknown-ticket" {
			t.Fatalf("cancel: %+v, %v", res, err)
		}
		return false
	})
	waitForFile(t, filepath.Join(proj, queue.OneOffsEpic, "issues", "01-a.md"), false)
	got := servertest.ReadFile(t, filepath.Join(proj, queue.OneOffsArchivedEpic, "issues", "01-a.md"))
	if !strings.Contains(got, "status: cancelled") || strings.Contains(got, "completed_at:") {
		t.Errorf("archived cancel:\n%s", got)
	}
}
