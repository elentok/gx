package sessions_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// A launch whose agent never turns idle fails Pty.Start (nativerunner covers
// that); here the runner's start fails the same way, so the ticket pauses at
// once and the pause card's Retry relaunches it.
func TestLaunchFailure_PausesAsIterationErrorAndRetryRelaunches(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	servertest.RegisterTurn(h)
	h.FailNextStart(errors.New("agent never turned idle"))
	if res, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	servertest.WaitFor(t, 30*time.Second, "the ticket to pause", func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusPaused
	})
	body := servertest.ReadFile(t, path)
	if !strings.Contains(body, "iteration-error") {
		t.Fatalf("ticket not paused as iteration-error:\n%s", body)
	}

	// Retry on an iteration-error pause is a relaunch, not a resume.
	res, err := h.Client.Tickets.Relaunch(context.Background(), "proj:epic-a/01", false)
	if err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	servertest.WaitFor(t, 30*time.Second, "the retried launch", func() bool { return len(h.Starts()) == 1 })
}
