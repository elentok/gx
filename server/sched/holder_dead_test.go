package sched_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

const verifiedEntry = "## Verified\n\n- story-1 passed at abc123\n"

// startHeldTicket launches proj:epic-a/01 on a pty fake and returns once it is up.
func startHeldTicket(t *testing.T) (pty rigs.NamedFake, ticketPath string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "held", "")
	ticketPath = store + "/proj/epic-a/issues/01-held.md"
	f, err := os.OpenFile(ticketPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n" + verifiedEntry); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty = rigs.NewNamedFake(runnerfake.NewRunner(), "pty")
	h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
		func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the ticket to launch", func() bool { return len(pty.Prompts(readoptLabel)) > 0 })
	return pty, ticketPath
}

// A dead holder relaunches once, resuming the session, and the ticket body (the
// earlier session's verdicts) is left as it was.
func TestHolderDead_RelaunchesOnceResumingTheSession(t *testing.T) {
	t.Parallel()
	pty, path := startHeldTicket(t)
	pty.SetCallErr(readoptLabel, agentrunner.HolderDied("holder failed: dial"))
	servertest.WaitFor(t, 20*time.Second, "the relaunch", func() bool { return len(pty.ResumeIDs(readoptLabel)) == 2 })
	if ids := pty.ResumeIDs(readoptLabel); ids[1] == "" {
		t.Errorf("resume ids = %q, want the relaunch to resume the first session", ids)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), verifiedEntry) {
		t.Errorf("ticket lost its verified entries:\n%s", body)
	}
	if tk, err := schema.ParseTicket(path); err != nil || tk.Status != schema.StatusClaimed {
		t.Errorf("status = %q, err = %v, want the ticket still claimed", tk.Status, err)
	}
}

// A relaunch that fails pauses iteration-error with the holder-dead cause.
func TestHolderDead_FailedRelaunchPausesNamingHolderDead(t *testing.T) {
	t.Parallel()
	pty, path := startHeldTicket(t)
	pty.SetStartErr(readoptLabel, errors.New("no pane"))
	pty.SetCallErr(readoptLabel, agentrunner.HolderDied("holder failed: dial"))
	servertest.WaitFor(t, 20*time.Second, "the ticket to pause", func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusPaused
	})
	tk, _ := schema.ParseTicket(path)
	body, _ := os.ReadFile(path)
	if string(tk.PauseReason) != "iteration-error" || !strings.Contains(string(body), "holder dead") {
		t.Errorf("pause reason = %q, want iteration-error naming holder dead:\n%s", tk.PauseReason, body)
	}
}
