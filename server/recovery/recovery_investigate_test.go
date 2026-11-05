package recovery_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func startUnmatched(t *testing.T) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Catalog{Enabled: true}
	})
}

func TestRecovery_UnmatchedPauseForksAQueuedInvestigateChildAndNeverRecurses(t *testing.T) {
	t.Parallel()
	h := startUnmatched(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/02", ""); err != nil {
		t.Fatal(err)
	}

	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitFor(t, func() bool {
		return len(servertest.QueueAddresses(t, h)) > 0 && servertest.QueueAddresses(t, h)[0] == "proj:epic-a/01a"
	})

	epic := epicTickets(t, h)
	var parent, child tickets.Ticket
	for _, tk := range epic.Tickets {
		switch tk.Identifier {
		case "01":
			parent = tk
		case "01a":
			child = tk
		}
	}
	if child.Type != "investigate" || child.Parent == nil || *child.Parent != "01" {
		t.Errorf("child = %+v, want an investigate fork of 01", child)
	}
	if got := epic.RenderedStatus(parent); got != tickets.StatusWaitingForChildren {
		t.Errorf("parent renders %v, want waiting-for-children", got)
	}

	if err := h.Server.PauseAs("proj:epic-a/01a", events.IterationError, "stuck"); err != nil {
		t.Fatalf("pause child: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // negative wait: a forked ticket would show by now
	for _, tk := range epicTickets(t, h).Tickets {
		if tk.Identifier == "01b" || tk.Identifier == "01a1" {
			t.Errorf("investigate ticket failure forked %s, want no recursion", tk.Identifier)
		}
	}
}

// R2 launches disabled, so a zero-commit pause falls through to an investigate
// fork with no R2 event: nothing is applied or proposed on R2's behalf.
func TestRecovery_ZeroCommitPauseWithR2DisabledInvestigatesWithoutR2Events(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
	})
	servertest.RegisterTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.ZeroCommit, "no commits landed"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				return true
			}
		}
		return false
	})
	log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	for _, ev := range log {
		if ev.Reason == "R2" {
			t.Errorf("unexpected %s event for disabled R2", ev.Type)
		}
	}
}

// R3 is an agent entry: an enabled match hands the failure to an investigate
// fork named for R3 and records the recovery as applied. The agent, not the
// server, then proves presence and merges the recoverable branch.
func TestRecovery_EnabledR3MatchForksAnInvestigateTicketNamingR3(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	servertest.RegisterTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.AmbiguousMerge, "done but commits missing from epic-a"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R3")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AmbiguousMerge) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// startR3 starts a server with the default catalog fully enabled.
func startR3(t *testing.T, opts ...func(*server.Config)) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, append([]func(*server.Config){func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	}}, opts...)...)
	servertest.RegisterTurn(h)
	return h
}

// R2 and R3 read the transcript the pause's session left: the matcher sees the
// agent's last words with no test injecting them.
func TestRecovery_ZeroCommitPauseMatchesOnItsTranscriptText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, last, entry string }{
		{"R2", `Bash({"command": "go test ./..."})`, "R2"},
		{"R3", "The feature is already implemented on the feature branch.", "R3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := startR3(t)
			servertest.RunToZeroCommitPause(t, h, tc.last)
			waitFor(t, func() bool {
				for _, tk := range epicTickets(t, h).Tickets {
					if tk.Identifier == "01a" && tk.Type == "investigate" {
						body, _ := os.ReadFile(tk.Path)
						return strings.Contains(string(body), tc.entry)
					}
				}
				return false
			})
		})
	}
}

func TestRecovery_R3CommitlessDoneIsProposedAndOnlyApprovalMarksItDone(t *testing.T) {
	t.Parallel()
	h := startR3(t)
	servertest.RunToZeroCommitPause(t, h, "Nothing to do: this is already implemented in a sibling ticket.")
	dir := filepath.Join(h.TicketStore, "proj")
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryProposed && ev.Reason == "R3" && ev.Text == "commitless-done proj:epic-a/01" {
				return true
			}
		}
		return false
	})
	tk := epicTickets(t, h).Tickets[0]
	if tk.IsDone() || tk.Commitless {
		t.Fatalf("proposal acted: status %s, commitless %v", tk.Status, tk.Commitless)
	}

	// The paused iteration may still be unwinding, and approve refuses while it runs.
	waitFor(t, func() bool { return len(h.Server.Runs()) == 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.Tickets.Approve(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("approve = %+v, %v", res, err)
	}
	if tk := epicTickets(t, h).Tickets[0]; !tk.IsDone() || !tk.Commitless {
		t.Errorf("approved ticket: status %s, commitless %v, want done and commitless", tk.Status, tk.Commitless)
	}
}

func TestRecovery_R3LostCommitsEscalateWithoutAnInvestigation(t *testing.T) {
	t.Parallel()
	h := startR3(t)
	reason := "done but commits missing from epic-a and iteration branch ralph-loop/epic-a-item-01 no longer exists to recover them"
	if err := h.Server.PauseAs("proj:epic-a/01", events.AmbiguousMerge, reason); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.TicketStore, "proj")
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryEscalated && strings.Contains(ev.Reason, "R3") {
				return true
			}
		}
		return false
	})
	log, _, _ := runlog.ReadEvents(dir, "epic-a")
	for _, ev := range log {
		if typ := events.Type(ev.Type); typ == events.RecoveryApplied || typ == events.RecoveryProposed {
			t.Errorf("unexpected %s for lost commits", ev.Type)
		}
	}
	if n := len(epicTickets(t, h).Tickets); n != 1 {
		t.Errorf("tickets = %d, want no investigate fork", n)
	}
}

// R4 is an agent entry: an enabled match hands the blocked pane to an
// investigate fork named for R4, which reads the pane and answers only an
// allow-listed dialog.
func TestRecovery_EnabledR4MatchForksAnInvestigateTicketNamingR4(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	servertest.RegisterTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.BlockedPane, "epic-a-01 is blocked on a prompt gx did not send; answer it in the pane"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R4")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.BlockedPane) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// pauseLaunchCollision starts a server with the default catalog fully enabled,
// seeds ticket 01's earlier events and its launch-failed event of kind as the
// loop would, and pauses it with the same kind.
func pauseLaunchCollision(t *testing.T, kind events.Kind, earlier ...events.Type) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	seeds := []runlog.Event{}
	for _, typ := range earlier {
		seeds = append(seeds, runlog.Event{Type: string(typ), Ticket: "01"})
	}
	seeds = append(seeds, runlog.Event{Type: string(events.LaunchFailed), Ticket: "01", Kind: string(kind), Attempt: 1, Reason: string(kind)})
	for _, seed := range seeds {
		if err := runlog.AppendEvent(filepath.Join(store, "proj"), "epic-a", seed); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	servertest.RegisterTurn(h)
	if err := h.Server.PauseAs("proj:epic-a/01", kind, "launch failed: "+string(kind)); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRecovery_R6BusyPaneIsRelaunchedByARule(t *testing.T) {
	t.Parallel()
	h := pauseLaunchCollision(t, events.AgentPaneBusy)
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AgentPaneBusy) && ev.Reason == "R6" && ev.Outcome == "ok" {
				return true
			}
		}
		return false
	})
}

// A taken name needs herdr's candidate block read, so R6 hands it to an
// investigate fork rather than acting.
func TestRecovery_R6TakenNameForksAnInvestigateTicketNamingR6(t *testing.T) {
	t.Parallel()
	h := pauseLaunchCollision(t, events.AgentNameTaken)
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R6")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AgentNameTaken) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// A taken name while the ticket's own iteration is still live is a clobbered
// claim, so R11 (not R6) forks the investigation that restores it.
func TestRecovery_R11ClobberedClaimForksAnInvestigateTicketNamingR11(t *testing.T) {
	t.Parallel()
	h := pauseLaunchCollision(t, events.AgentNameTaken, events.IterationStarted)
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R11")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AgentNameTaken) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// A reclaim that pauses zero-commit at once after an earlier prompt stall is a
// stalled pane read as finished, so R12 forks the investigation that closes it
// and relaunches.
func TestRecovery_R12StalledReclaimForksAnInvestigateTicketNamingR12(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	for _, seed := range []runlog.Event{
		{Type: string(events.LaunchFailed), Ticket: "01", Kind: string(events.AgentPromptStalled), Attempt: 1, Reason: "prompt stalled"},
		{Type: string(events.Reclaimed), Ticket: "01"},
	} {
		if err := runlog.AppendEvent(filepath.Join(store, "proj"), "epic-a", seed); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	servertest.RegisterTurn(h)
	if err := h.Server.PauseAs("proj:epic-a/01", events.ZeroCommit, "no commits landed"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R12")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.ZeroCommit) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// iterationAgent is a runner answering to name that hosts ticket 01's
// iteration agent in state.
