package servertest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
)

const longTicketTurns = 3

// longSession is a held implement session whose transcript already has n assistant turns.
func longSession(h *Harness, n int) {
	h.OnTurn(func(turn *Turn) {
		turn.Stall()
		// The count starts at the wait, after the prompt went in: write later.
		go func(turn Turn) {
			time.Sleep(2 * time.Second)
			for range n {
				turn.Transcript("working")
			}
		}(*turn)
	})
}

func ticketFile(t *testing.T, store, epic string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(store, "proj", epic, "issues", "*.md"))
	var all []string
	for _, f := range files {
		body, _ := os.ReadFile(f)
		all = append(all, string(body))
	}
	return strings.Join(all, "\n")
}

func TestLongTicket_AlertsOnceMarksTheTicketAndKeepsTheSessionRunning(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo, func(c *server.Config) {
		c.LongTicketTurns = longTicketTurns
		c.RecoverySettings.FollowUps = "proj:follow-ups"
	})
	longSession(h, longTicketTurns)
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	// The guards tick once per 30s poll.
	WaitFor(t, 90*time.Second, "the long-ticket follow-up", func() bool {
		return strings.Contains(ticketFile(t, store, "follow-ups"), "long-ticket")
	})
	if !strings.Contains(ticketFile(t, store, "epic-a"), "long_ticket_turns: 3") {
		t.Errorf("ticket 01 has no long_ticket_turns mark:\n%s", ticketFile(t, store, "epic-a"))
	}
	if !strings.Contains(ticketFile(t, store, "epic-a"), "status: claimed") {
		t.Errorf("ticket 01 is not still claimed:\n%s", ticketFile(t, store, "epic-a"))
	}
	banners := 0
	for _, b := range h.Banners.Sent() {
		if strings.Contains(b.Title, "Long ticket") {
			banners++
		}
	}
	if banners != 1 {
		t.Errorf("long-ticket banners = %d, want 1", banners)
	}
}

func TestLongTicket_ALaunchClearsTheMarkOfAnEarlierOne(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicketWith(t, store, "proj", "epic-a", "01", "first", TicketOpts{LongTicketTurns: 5})
	SetProjectRepo(t, store, "proj", repo)
	h := StartWithStore(t, store, func(c *server.Config) { c.LongTicketTurns = 0 })
	h.OnTurn(func(turn *Turn) { turn.Stall() })
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	WaitFor(t, 30*time.Second, "the launch to clear the mark", func() bool {
		f := ticketFile(t, store, "epic-a")
		return strings.Contains(f, "status: claimed") && !strings.Contains(f, "long_ticket_turns")
	})
}

func TestLongTicket_ZeroThresholdProducesNoAlert(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	h := startPty(t, store, repo, func(c *server.Config) { c.LongTicketTurns = 0 })
	longSession(h, 5)
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	WaitFor(t, 30*time.Second, "the session to start", func() bool {
		return strings.Contains(ticketFile(t, store, "epic-a"), "status: claimed")
	})
	// The guards tick once per 30s poll; wait past the first tick.
	time.Sleep(35 * time.Second)
	if strings.Contains(ticketFile(t, store, "epic-a"), "long_ticket_turns") {
		t.Error("ticket 01 was marked with the threshold at 0")
	}
	for _, b := range h.Banners.Sent() {
		if strings.Contains(b.Title, "Long ticket") {
			t.Errorf("banner %+v with the threshold at 0", b)
		}
	}
}
