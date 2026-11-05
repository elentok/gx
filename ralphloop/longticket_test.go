package ralphloop

import (
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// runLongTicketWait runs waitForFinish through three timed-out polls, each
// reading the next count, and returns the counts the alert was called with.
func runLongTicketWait(t *testing.T, ticketType string, threshold int, counts []int, nilRead bool) []int {
	t.Helper()
	var fired []int
	reads := 0
	d := Deps{Runner: boundGiveUpRunner(func(wait int) bool { return wait <= 3 }), Sleep: func(time.Duration) {}}
	if !nilRead {
		d.ReadTurns = func(_, _ string, _ time.Time) (transcript.Turns, bool, error) {
			n := counts[min(reads, len(counts)-1)]
			reads++
			return transcript.Turns{Model: "claude-sonnet-5-5", Count: n}, true, nil
		}
	}
	p := boundGiveUpParams()
	p.Agent = agent.AgentClaude
	p.Model = "sonnet"
	p.TicketData = tickets.Ticket{Identifier: "19", Type: ticketType}
	p.ScratchDir = fixture.EpicScratchDir(t, "epic")
	p.EpicName = "epic"
	p.LongTicketTurns = threshold
	p.OnLongTicket = func(turns int) { fired = append(fired, turns) }
	if err := waitForFinish(d, p, "sess-19"); err != nil {
		t.Fatalf("waitForFinish: %v", err)
	}
	return fired
}

func TestLongTicket_FiresOnceAtTheThreshold(t *testing.T) {
	t.Parallel()
	got := runLongTicketWait(t, "implement", 100, []int{99, 100, 101}, false)
	if !slices.Equal(got, []int{100}) {
		t.Errorf("alert counts = %v, want [100]", got)
	}
}

func TestLongTicket_ZeroThresholdNeverFires(t *testing.T) {
	t.Parallel()
	if got := runLongTicketWait(t, "implement", 0, []int{500}, false); len(got) != 0 {
		t.Errorf("alert counts = %v, want none", got)
	}
}

func TestLongTicket_NilReadTurnsNeverFires(t *testing.T) {
	t.Parallel()
	if got := runLongTicketWait(t, "implement", 1, nil, true); len(got) != 0 {
		t.Errorf("alert counts = %v, want none", got)
	}
}

func TestLongTicket_OtherTicketTypesAreUntouched(t *testing.T) {
	t.Parallel()
	if got := runLongTicketWait(t, "research", 1, []int{500}, false); len(got) != 0 {
		t.Errorf("alert counts = %v, want none", got)
	}
}
