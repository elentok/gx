package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func TestFinish_ZeroCommitParkRecordsRun(t *testing.T) {
	t.Run("herdr", func(t *testing.T) {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		h := servertest.StartWithStore(t, store)
		runToZeroCommitPark(t, h, "done")
		assertParkRecordsRun(t, store, events.NeedsAnswer, "herdr", agentrunner.Session{Label: "epic-a-iter-01", SessionID: "s1"})
	})
	t.Run("headless", func(t *testing.T) {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		fake := newTurnRunner(func(string, string) {})
		h := servertest.StartWithStore(t, store, func(c *server.Config) {
			c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
		})
		if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		waitStatus(t, h.TicketStore+"/proj/epic-a/issues/01-first.md", schema.StatusNeedsAnswer)
		assertParkRecordsRun(t, store, events.NeedsAnswer, "fake", agentrunner.Session{Label: "epic-a-iter-01", SessionID: "session-1"})
	})
}
