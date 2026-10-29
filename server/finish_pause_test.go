package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func TestFinish_ZeroCommitPauseRecordsRun(t *testing.T) {
	t.Parallel()
	t.Run("pty", func(t *testing.T) {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		h := servertest.StartWithStore(t, store)
		runToZeroCommitPause(t, h, "done")
		// The PTY agent's session id is generated, so any non-empty one will do.
		assertPauseRecordsRun(t, store, events.Paused, "pty", agentrunner.Session{Label: "epic-a-iter-01"})
	})
	t.Run("headless", func(t *testing.T) {
		store, repo := t.TempDir(), testutil.TempRepo(t)
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, "proj", repo)
		fake := newTurnRunner(func(string, string) {})
		h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
		if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
		waitStatus(t, h.TicketStore+"/proj/epic-a/issues/01-first.md", schema.StatusLegacyAnswer)
		assertPauseRecordsRun(t, store, events.Paused, "fake", agentrunner.Session{Label: "epic-a-iter-01", SessionID: "session-1"})
	})
}
