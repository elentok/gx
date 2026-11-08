package sched_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/sched"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

const readoptLabel = "epic-a-iter-01"

// startUnreachable launches proj:epic-a/01 on a fake whose holder does not
// answer, and returns what the tests need to watch it.
func startUnreachable(t *testing.T) (h *servertest.Harness, base string, pty rigs.NamedFake, ticketPath string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "held", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty = rigs.NewNamedFake(runnerfake.NewRunner(), "pty")
	h = servertest.StartWithStore(t, store, servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
		func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the ticket to launch", func() bool { return len(pty.Prompts(readoptLabel)) > 0 })
	t.Cleanup(func() { pty.SetCallErr(readoptLabel, nil) })
	pty.SetCallErr(readoptLabel, fmt.Errorf("status: %w", holder.ErrHolderUnreachable))
	return h, hs.WebURL, pty, store + "/proj/epic-a/issues/01-held.md"
}

func holderRetrying(t *testing.T, h *servertest.Harness, base string) bool {
	t.Helper()
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address=proj:epic-a/01")
	if code != http.StatusOK {
		t.Fatalf("detail %d: %s", code, body)
	}
	var d wire.TicketDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	return d.Stats != nil && d.Stats.HolderUnreachable
}

// A holder that answers late keeps the run tracked: the card shows the retry
// while it holds, clears after, and nothing pauses or relaunches.
func TestHolderReadopt_SlowHolderKeepsTheRun(t *testing.T) {
	t.Parallel()
	h, base, pty, path := startUnreachable(t)
	servertest.WaitFor(t, 10*time.Second, "the card to show the retry", func() bool { return holderRetrying(t, h, base) })
	pty.SetCallErr(readoptLabel, nil)
	servertest.WaitFor(t, 10*time.Second, "the retry state to clear", func() bool { return !holderRetrying(t, h, base) })
	if n := len(pty.Prompts(readoptLabel)); n != 1 {
		t.Errorf("prompts = %d, want 1: the run was relaunched", n)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusClaimed {
		t.Errorf("status = %q, err = %v, want the ticket still claimed", tk.Status, err)
	}
}

// A holder that never answers is retried up to the cap, then pauses with the
// holder-alive cause; it is never relaunched.
func TestHolderReadopt_NeverAnswersPausesAtTheCap(t *testing.T) {
	t.Parallel()
	_, _, pty, path := startUnreachable(t)
	servertest.WaitFor(t, 30*time.Second, "the ticket to pause", func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusPaused
	})
	body, _ := os.ReadFile(path)
	want := fmt.Sprintf("dial failed %d times with the holder alive", sched.HolderReadoptCap)
	if !strings.Contains(string(body), want) {
		t.Errorf("ticket lacks %q:\n%s", want, body)
	}
	if n := len(pty.Prompts(readoptLabel)); n != 1 {
		t.Errorf("prompts = %d, want 1", n)
	}
}
