package launchtest_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets/schema"
)

const finishBlockAddr = "proj:epic-a/01"

// startVerifyAgent queues a verify ticket with one Stories unit. Its scripted
// agent commits and reports finished with nothing recorded. When
// recordAfterBlock is set it records the unit once the server's block message
// arrives; otherwise it just reports finished again.
func startVerifyAgent(t *testing.T, recordAfterBlock bool) (h *servertest.Harness, ticketPath string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "verify", servertest.TicketOpts{Type: "verify"})
	ticketPath = filepath.Join(store, "proj", "epic-a", "issues", "01-verify.md")
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	body := schema.SetSection(string(raw), schema.StoriesHeading, schema.StoriesSection([]string{schema.StoryKey(1)}))
	if err := os.WriteFile(ticketPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h = servertest.StartWithStore(t, store)
	var launches atomic.Int32
	report := func() {
		if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.IterationStatus = schema.IterationStatusFinished
			tk.IterationFinishedAt = time.Now().UTC()
		}); err != nil {
			t.Error(err)
		}
	}
	h.OnTurn(func(turn *servertest.Turn) {
		switch {
		case !strings.HasPrefix(turn.Text, agentfake.FinishBlockPrefix):
			// A launch after Retry finds the first one's commit, so each commits its own file.
			testutil.WriteFile(t, turn.Cwd, fmt.Sprintf("work-%d.txt", launches.Add(1)), "work")
			testutil.CommitAll(t, turn.Cwd, "agent work")
		case recordAfterBlock:
			head, err := git.RevParse(turn.Cwd, "HEAD")
			if err != nil {
				t.Error(err)
				return
			}
			cur, err := os.ReadFile(ticketPath)
			if err != nil {
				t.Error(err)
				return
			}
			entry := schema.VerifiedEntry{Story: schema.StoryKey(1), Verdict: "pass", Commit: head}
			if err := os.WriteFile(ticketPath, []byte(schema.AppendVerified(string(cur), entry)), 0o644); err != nil {
				t.Error(err)
				return
			}
		}
		report()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, finishBlockAddr, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	return h, ticketPath
}

func finishBlocks(t *testing.T, h *servertest.Harness) int {
	t.Helper()
	n := 0
	for _, ev := range ofType(runLog(t, h), events.FinishBlocked) {
		if strings.Contains(ev.Reason, schema.StoryKey(1)) {
			n++
		}
	}
	return n
}

// A verify finish with an unchecked unit is blocked once; the agent records the
// unit, reports again and the ticket merges.
func TestFinishBlock_VerifyRecordsUncheckedUnitAfterOneBlock(t *testing.T) {
	t.Parallel()
	h, path := startVerifyAgent(t, true)
	servertest.WaitFor(t, 60*time.Second, "the ticket to finish", func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusDone
	})
	if n := finishBlocks(t, h); n != 1 {
		t.Errorf("finish-blocked events naming the unit = %d, want 1", n)
	}
	raw, _ := os.ReadFile(path)
	if got := schema.ParseVerified(schema.ParseBody(string(raw))); len(got) == 0 {
		t.Errorf("ticket has no Verified entries:\n%s", raw)
	}
}

// An agent that never records is blocked at the cap, then the ticket pauses
// iteration-error with the unit in its reason; Retry starts the count again.
func TestFinishBlock_CapPausesIterationErrorAndRetryResetsTheCount(t *testing.T) {
	t.Parallel()
	h, path := startVerifyAgent(t, false)
	paused := func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusPaused && string(tk.PauseReason) == "iteration-error"
	}
	servertest.WaitFor(t, 60*time.Second, "the ticket to pause", paused)
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "unchecked units: "+schema.StoryKey(1)) {
		t.Errorf("pause reason lacks the unchecked unit:\n%s", raw)
	}
	if n := finishBlocks(t, h); n != 2 {
		t.Fatalf("finish-blocked events before Retry = %d, want 2 (the cap)", n)
	}
	servertest.WaitFor(t, 10*time.Second, "the run to leave the registry", func() bool { return len(h.Server.Runs()) == 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.Tickets.Retry(ctx, finishBlockAddr, false); err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v; want accepted", res, err)
	}
	// A count carried over would pause at once; a reset one blocks twice more.
	servertest.WaitFor(t, 60*time.Second, "two more blocks after Retry", func() bool { return finishBlocks(t, h) == 4 })
	servertest.WaitFor(t, 60*time.Second, "the ticket to pause again", paused)
}
