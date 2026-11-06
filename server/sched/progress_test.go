package sched_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// A running ticket gets a Progress message per interval, naming the ticket and
// the epic's counts; the first one comes a full interval after the run starts.
func TestProgress_RunningTicketGetsAMessagePerInterval(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	withInterval := func(c *server.Config) {
		chat(c)
		c.Chat.ProgressInterval = 400 * time.Millisecond
	}
	startBlockable(t, "headless", withInterval)
	if got := waitForChat(count, "01 first"); got < 1 {
		t.Fatalf("no Progress message naming the running ticket")
	}
	waitFor(t, func() bool { return count("Left: 0") >= 2 })
}
