package server_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

const queueRowTicket = "proj:epic-a/01"

func queueRows(t *testing.T, h *servertest.Harness) []server.SessionInfo {
	t.Helper()
	list, err := h.Client.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(list, func(si server.SessionInfo) bool { return !si.Queue })
}

func startQueueRows(t *testing.T, runner string) (*servertest.Harness, namedFake) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	fake := namedFake{runnerfake.NewRunner(), runner}
	h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(fake, func(string, string) agentrunner.Runner { return fake }))
	t.Cleanup(func() { fake.SetState(resumeLabel, agentrunner.StateDone, "") })
	if res, err := h.Client.QueueAdd(context.Background(), queueRowTicket, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the ticket launches", func() bool { return len(fake.Prompts(resumeLabel)) > 0 })
	return h, fake
}

func TestQueueAgentRows_PtyRunListsAWorkingRowWithoutUnreadOrActionable(t *testing.T) {
	t.Parallel()
	h, fake := startQueueRows(t, "pty")
	fake.SetState(resumeLabel, agentrunner.StateWorking, "")

	servertest.WaitFor(t, 10*time.Second, "a working row", func() bool {
		rows := queueRows(t, h)
		return len(rows) == 1 && rows[0].Status.State == server.SessionWorking
	})
	row := queueRows(t, h)[0]
	if row.Ticket != queueRowTicket || row.Holder == "" || row.Unread != (server.SessionUnread{}) || row.Actionable.Reason != "" {
		t.Errorf("row = %+v, want the ticket, a holder dir and no Unread or Actionable", row)
	}
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil || len(snap.Sessions) == 0 {
		t.Errorf("snapshot sessions = %+v, %v; want the Queue row too", snap.Sessions, err)
	}
}

func TestQueueAgentRows_HeldRunShowsRateLimited(t *testing.T) {
	t.Parallel()
	h, fake := startQueueRows(t, "pty")
	fake.SetLimitedUnknownReset(resumeLabel)
	fake.SetState(resumeLabel, agentrunner.StateDone, "")

	servertest.WaitFor(t, 10*time.Second, "a rate-limited row", func() bool {
		rows := queueRows(t, h)
		return len(rows) == 1 && rows[0].Status.State == server.SessionRateLimited
	})
}

func TestQueueAgentRows_BlockedAgentKeepsItsRow(t *testing.T) {
	t.Parallel()
	h, fake := startQueueRows(t, "pty")
	fake.SetState(resumeLabel, agentrunner.StateBlocked, "permission")

	servertest.WaitFor(t, 10*time.Second, "a blocked row", func() bool {
		rows := queueRows(t, h)
		return len(rows) == 1 && rows[0].Status.State == server.SessionBlocked
	})
}

func TestQueueAgentRows_RowLeavesWhenTheRunFinishes(t *testing.T) {
	t.Parallel()
	h, fake := startQueueRows(t, "pty")
	servertest.WaitFor(t, 10*time.Second, "a row", func() bool { return len(queueRows(t, h)) == 1 })

	fake.SetState(resumeLabel, agentrunner.StateDone, "")
	servertest.WaitFor(t, 20*time.Second, "the row to leave", func() bool { return len(queueRows(t, h)) == 0 })
}

func TestQueueAgentRows_HerdrAndHeadlessRunsAreAbsent(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"herdr", "headless"} {
		h, _ := startQueueRows(t, runner)
		if rows := queueRows(t, h); len(rows) != 0 {
			t.Errorf("%s rows = %+v, want none", runner, rows)
		}
	}
}
