package launchtest_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

const noteTicket = "proj:epic-a/01"

// verifyStore is a store holding one verify ticket, whose launch always has a
// note, and the paths the launch tests look at.
func verifyStore(t *testing.T) (store, repo, ticketPath, notePath string) {
	t.Helper()
	store, repo = t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "verify", servertest.TicketOpts{Type: "verify"})
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticketPath = filepath.Join(store, "proj", "epic-a", "issues", "01-verify.md")
	// A recorded story from another commit is what gives a verify launch a note.
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	body := schema.AppendVerified(string(raw), schema.VerifiedEntry{Story: "44", Verdict: "pass", Commit: "deadbeef"})
	if err := os.WriteFile(ticketPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return store, repo, ticketPath, filepath.Join(store, "proj", "epic-a", "launch", "01-note.md")
}

func addTicket(t *testing.T, h *servertest.Harness, agent string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, noteTicket, agent); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
}

// The typed launch prompt is the skill command and the address, even when the
// launch has a note, for both agents.
func TestLaunch_TypedPromptIsOnlySkillAndAddress(t *testing.T) {
	for agentName, want := range map[string]string{"claude": "/gx-verify-epic " + noteTicket, "codex": "$gx-verify-epic " + noteTicket} {
		t.Run(agentName, func(t *testing.T) {
			t.Parallel()
			store, _, _, notePath := verifyStore(t)
			fake := runnerfake.NewRunner()
			h := servertest.StartWithStore(t, store, servertest.WithRunner(rigs.NewNamedFake(fake, "pty")))
			addTicket(t, h, agentName)
			t.Cleanup(func() { fake.Finish(rigs.ResumeLabel) })
			servertest.WaitFor(t, 10*time.Second, "the ticket launches", func() bool { return len(fake.Prompts(rigs.ResumeLabel)) > 0 })

			if got := fake.Prompts(rigs.ResumeLabel)[0]; got != want {
				t.Errorf("typed prompt = %q, want %q", got, want)
			}
			if raw, err := os.ReadFile(notePath); err != nil || len(raw) == 0 {
				t.Errorf("note file = %q (%v), want the note", raw, err)
			}
		})
	}
}

// A ticket's note file is outside its worktree, so a commit never holds it, and
// closing the ticket removes it.
func TestLaunch_NoteFileStaysOutOfTheWorktreeAndGoesWhenTheTicketCloses(t *testing.T) {
	t.Parallel()
	store, _, ticketPath, notePath := verifyStore(t)
	h := servertest.StartWithStore(t, store)
	var mu sync.Mutex
	var status string
	var noteDuringRun bool
	h.OnTurn(func(turn *servertest.Turn) {
		testutil.WriteFile(t, turn.Cwd, "work.txt", "work")
		testutil.CommitAll(t, turn.Cwd, "agent work")
		raw, err := exec.Command("git", "-C", turn.Cwd, "status", "--porcelain", "--ignored").Output()
		if err != nil {
			t.Error(err)
		}
		out := string(raw)
		_, noteErr := os.Stat(notePath)
		mu.Lock()
		status, noteDuringRun = out, noteErr == nil
		mu.Unlock()
		if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.IterationStatus = schema.IterationStatusFinished
			tk.IterationFinishedAt = time.Now().UTC()
		}); err != nil {
			t.Error(err)
		}
	})
	addTicket(t, h, "claude")
	servertest.WaitFor(t, 60*time.Second, "the ticket to finish", func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		return err == nil && tk.Status == schema.StatusDone
	})
	mu.Lock()
	defer mu.Unlock()
	if !noteDuringRun {
		t.Error("the note file did not exist while the agent ran")
	}
	if strings.Contains(status, "note") || strings.Contains(status, "launch") || strings.TrimSpace(status) != "" {
		t.Errorf("git status after the commit = %q, want it clean", status)
	}
	servertest.WaitFor(t, 10*time.Second, "the note file to go", func() bool {
		_, err := os.Stat(notePath)
		return os.IsNotExist(err)
	})
}

// Retry and Start over each write the note file again.
func TestLaunch_RetryAndStartOverRewriteTheNoteFile(t *testing.T) {
	t.Parallel()
	store, _, ticketPath, notePath := verifyStore(t)
	h := servertest.StartWithStore(t, store, servertest.WithLaunchGrace(3*time.Second),
		func(c *server.Config) { c.StalledNudgeDelay = 2 * time.Second })
	hang := func() { h.FailNextPrompts(2, &claudeui.HungPaneError{Screen: "> /gx-verify-epic " + noteTicket}) }
	hang()
	addTicket(t, h, "claude")
	stale := func() {
		t.Helper()
		servertest.WaitStatus(t, ticketPath, schema.StatusPaused)
		servertest.WaitFor(t, 10*time.Second, "the run to leave the registry", func() bool { return len(h.Server.Runs()) == 0 })
		if raw, err := os.ReadFile(notePath); err != nil || len(raw) == 0 {
			t.Fatalf("note file = %q (%v), want the launch note", raw, err)
		}
		if err := os.WriteFile(notePath, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rewritten := func(what string) {
		t.Helper()
		servertest.WaitFor(t, 30*time.Second, what+" to rewrite the note file", func() bool {
			raw, err := os.ReadFile(notePath)
			return err == nil && string(raw) != "stale"
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stale()
	hang()
	if res, err := h.Client.Tickets.Retry(ctx, noteTicket, false); err != nil || res.Refused {
		t.Fatalf("retry: %+v, %v", res, err)
	}
	rewritten("Retry")

	servertest.WaitFor(t, 30*time.Second, "the ticket to pause again", func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		return err == nil && tk.Status == schema.StatusPaused && len(h.Server.Runs()) == 0
	})
	stale()
	res, err := h.Client.Admin.Repair(ctx, "start-over", wire.RepairRequest{Address: noteTicket, Reason: "again"})
	if err != nil || res.Refused {
		t.Fatalf("start over: %+v, %v", res, err)
	}
	rewritten("Start over")
}

// A Claude whose typed launch command becomes a paste fails the launch with the
// pasted-command reason (not the hung-pane one), is relaunched once, then the
// ticket pauses with Retry and Start over, and Retry ends the pause.
func TestLaunch_PastedCommandRelaunchesOnceThenPausesAndRetryEndsIt(t *testing.T) {
	t.Parallel()
	store, _, ticketPath, _ := verifyStore(t)
	// The launch and its one relaunch paste; the launch after Retry does not.
	h := servertest.StartWithStore(t, store,
		servertest.WithEnv(agentfake.TUIPasteOverEnv+"=5:2:"+filepath.Join(t.TempDir(), "launches")),
		servertest.WithLaunchGrace(3*time.Second))
	addTicket(t, h, "claude")
	servertest.WaitStatus(t, ticketPath, schema.StatusPaused)

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || string(tk.PauseReason) != string(events.AgentPromptStalled) {
		t.Fatalf("pause reason = %q (%v), want agent_prompt_stalled", tk.PauseReason, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the paused event", func() bool { return len(ofType(runLog(t, h), events.Paused)) > 0 })
	logged := runLog(t, h)
	failed := ofType(logged, events.LaunchFailed)
	if len(failed) != 2 {
		t.Fatalf("launch-failed events = %+v, want the launch and one relaunch", failed)
	}
	for _, f := range failed {
		if !strings.Contains(f.Reason, "the launch command was pasted as plain text and the skill did not run") || strings.Contains(f.Reason, "pane stopped taking keys") {
			t.Errorf("launch-failed reason = %q, want the pasted-command reason", f.Reason)
		}
	}
	if n := len(ofType(logged, events.IterationStarted)); n != 0 {
		t.Errorf("iteration-started events = %d, want none: no launch succeeded", n)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	d, err := h.Client.Reads.Detail(ctx, noteTicket)
	if err != nil || !slices.Equal(d.Actions, []string{wire.ActionRetry, wire.ActionStartOver}) {
		t.Errorf("actions = %v (%v), want Retry and Start over", d.Actions, err)
	}
	if res, err := h.Client.Tickets.Retry(ctx, noteTicket, false); err != nil || res.Refused {
		t.Fatalf("retry: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 30*time.Second, "Retry to end the pause", func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		return err == nil && tk.Status != schema.StatusPaused
	})
}
