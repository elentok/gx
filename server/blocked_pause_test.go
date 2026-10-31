package server_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets/schema"
)

// An agent error pauses the ticket with the agent's own text, not the
// "prompt gx did not send" wording, and the web offers Retry.
func TestBlockedPause_AgentErrorPausesWithItsTextAndOffersRetry(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"pty", "codex"} {
		t.Run(runner, func(t *testing.T) {
			h, ticketPath, turn := startBlockable(t, runner)
			turn.SetErrored("epic-a-iter-01", "model gone")
			waitStatus(t, ticketPath, schema.StatusLegacyAnswer)

			d, err := h.Client.Detail(context.Background(), answerAddr)
			if err != nil || !slices.Contains(d.Actions, server.ActionRetry) {
				t.Errorf("actions = %v, %v; want Retry offered", d.Actions, err)
			}
			var reason string
			waitFor(t, func() bool {
				log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
				for _, ev := range log {
					if events.Type(ev.Type) == events.Paused && ev.Kind == string(events.BlockedPane) {
						reason = ev.Reason
					}
				}
				return reason != ""
			})
			if !strings.Contains(reason, "hit an error: model gone") || strings.Contains(reason, "did not send") {
				t.Errorf("pause reason does not quote the error: %q", reason)
			}
		})
	}
}

// gateDwell returns a Config option that parks the 15-second dwell until the
// returned release is called, so "under 15 seconds" is not a timing race.
func gateDwell() (func(*server.Config), func()) {
	gate := make(chan struct{})
	opt := func(c *server.Config) {
		pace := c.Sleep
		c.Sleep = func(d time.Duration) {
			if d == 15*time.Second {
				<-gate
				return
			}
			pace(d)
		}
	}
	return opt, func() { close(gate) }
}

// A block still there when the 15-second dwell ends pauses the ticket and the
// reason names the buttons the web offers; one that clears inside it pauses
// nothing.
func TestBlockedPause_PTYDwellPausesOnlyIfStillBlocked(t *testing.T) {
	t.Parallel()
	t.Run("still blocked", func(t *testing.T) {
		gate, release := gateDwell()
		h, ticketPath, turn := startBlockable(t, "pty", gate)
		turn.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
		time.Sleep(200 * time.Millisecond) // negative wait: the dwell has not ended
		if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusClaimed {
			t.Fatalf("ticket = %+v, %v; want claimed before the dwell ends", tk.Status, err)
		}
		release()
		waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
		var reason string
		waitFor(t, func() bool {
			log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
			for _, ev := range log {
				if events.Type(ev.Type) == events.Paused && ev.Kind == string(events.BlockedPane) {
					reason = ev.Reason
				}
			}
			return reason != ""
		})
		for _, want := range []string{"Approve", "Retry"} {
			if !strings.Contains(reason, want) {
				t.Errorf("reason %q lacks %q", reason, want)
			}
		}
	})
	t.Run("clears inside the dwell", func(t *testing.T) {
		gate, release := gateDwell()
		_, ticketPath, turn := startBlockable(t, "pty", gate)
		turn.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
		turn.SetState("epic-a-iter-01", agentrunner.StateWorking, "")
		release()
		// Negative wait: a pause would show once the re-check has run.
		time.Sleep(300 * time.Millisecond)
		if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusClaimed {
			t.Errorf("ticket = %v, %v; want claimed", tk.Status, err)
		}
	})
}

// A usage-limit error holds the run: the limit is read before the blocked
// state, so the ticket never pauses.
func TestBlockedPause_UsageLimitErrorHoldsInsteadOfPausing(t *testing.T) {
	t.Parallel()
	h, ticketPath, turn := startBlockable(t, "pty")
	turn.SetRateLimit("epic-a-iter-01", time.Now().Add(time.Hour))
	turn.SetErrored("epic-a-iter-01", "You've hit your usage limit.")
	waitFor(t, func() bool {
		ex, err := h.Client.Explain(context.Background(), "proj:epic-a/01")
		return err == nil && strings.HasPrefix(ex.Verdict, "rate-limited: resets")
	})
	time.Sleep(200 * time.Millisecond) // negative wait: a pause would show within a few ticks
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status == schema.StatusPaused {
		t.Errorf("ticket = %+v, %v; a held run must not pause", tk.Status, err)
	}
}

// A blocked pause lifts when the agent leaves blocked, on every runner, with the
// same session and no second claude.
func TestBlockedPause_LiftsWhenAgentLeavesBlocked(t *testing.T) {
	t.Parallel()
	exits := map[string]agentrunner.State{"working": agentrunner.StateWorking, "idle": agentrunner.StateIdle}
	for _, runner := range []string{"herdr", "pty", "codex"} {
		for name, state := range exits {
			t.Run(runner+"/"+name, func(t *testing.T) {
				h, ticketPath, turn := startBlockable(t, runner)
				turn.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
				waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
				waitFor(t, func() bool { return runsJSONHas(h, `"waiting_since"`) })

				turn.SetState("epic-a-iter-01", state, "")
				waitStatus(t, ticketPath, schema.StatusClaimed)
				waitFor(t, func() bool { return !runsJSONHas(h, `"waiting_since"`) })
				if _, ok, err := turn.Find("epic-a-iter-01"); err != nil || !ok {
					t.Errorf("session not live: ok=%v err=%v", ok, err)
				}
			})
		}
	}
}
