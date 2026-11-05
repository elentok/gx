package ralphloop

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// serverGuardsWait runs WaitIterationFinished through three timed-out polls on
// a ticket of ticketType, reading the next entry of counts at each turn read.
// A nil counts leaves ReadTurns nil.
type serverGuardsFixture struct {
	err   error
	reads int
	fired []int
}

func serverGuardsWait(t *testing.T, ticketType, expected, ranModel string, threshold int, counts []int) serverGuardsFixture {
	t.Helper()
	var f serverGuardsFixture
	r := boundGiveUpRunner(func(wait int) bool { return wait <= 3 })
	// The server's wait does not stop on blocked here, so every Wait is a poll.
	r.finishPollsOnly = false
	d := deps.Deps{Runner: r, Sleep: func(time.Duration) {}}
	if counts != nil {
		d.ReadTurns = func(_, _ string, _ time.Time) (transcript.Turns, bool, error) {
			n := counts[min(f.reads, len(counts)-1)]
			f.reads++
			return transcript.Turns{Model: ranModel, Count: n}, true, nil
		}
	}
	one := OneIteration{
		Agent: agent.AgentClaude, Epic: "epic", ScratchDir: fixture.EpicScratchDir(t, "epic"), Model: expected,
		Ticket:          tickets.Ticket{Identifier: "19", Type: ticketType},
		LongTicketTurns: threshold,
		OnLongTicket:    func(turns int) { f.fired = append(f.fired, turns) },
	}
	f.err = WaitIterationFinished(d, one, IterationWorktree{Label: "iter-19", Path: "/repo/iter-19"}, agentrunner.Session{Label: "iter-19", ID: "pane-1"})
	return f
}

func TestWaitIterationFinished_ModelMismatch_WrongModelReturnsTypedError(t *testing.T) {
	t.Parallel()
	f := serverGuardsWait(t, "implement", "sonnet", "claude-opus-5-5", 0, []int{1})
	var mm *ModelMismatchError
	if !errors.As(f.err, &mm) {
		t.Fatalf("err = %v, want *ModelMismatchError", f.err)
	}
	if mm.Ran != "claude-opus-5-5" || mm.Expected != "sonnet" {
		t.Errorf("error = %+v, want ran claude-opus-5-5 expected sonnet", mm)
	}
}

func TestWaitIterationFinished_ModelMismatch_ChangesNothingWhenItShouldNot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                    string
		ticketType, ran, expect string
		counts                  []int
		wantReads               bool
	}{
		{"matching model", "implement", "claude-sonnet-5-5", "sonnet", []int{1}, true},
		{"nil ReadTurns", "implement", "claude-opus-5-5", "sonnet", nil, false},
		{"non-implement ticket", "research", "claude-opus-5-5", "sonnet", []int{1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := serverGuardsWait(t, c.ticketType, c.expect, c.ran, 0, c.counts)
			if f.err != nil {
				t.Fatalf("err = %v, want none", f.err)
			}
			if (f.reads > 0) != c.wantReads {
				t.Errorf("reads = %d, wantReads %v", f.reads, c.wantReads)
			}
		})
	}
}

func TestWaitIterationFinished_LongTicket_FiresOnceAtTheThreshold(t *testing.T) {
	t.Parallel()
	f := serverGuardsWait(t, "implement", "sonnet", "claude-sonnet-5-5", 100, []int{99, 100, 101})
	if f.err != nil {
		t.Fatal(f.err)
	}
	if !slices.Equal(f.fired, []int{100}) {
		t.Errorf("alert counts = %v, want [100]", f.fired)
	}
}

func TestWaitIterationFinished_LongTicket_ChangesNothingWhenItShouldNot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		ticketType string
		threshold  int
		counts     []int
	}{
		{"zero threshold", "implement", 0, []int{500}},
		{"nil ReadTurns", "implement", 1, nil},
		{"non-implement ticket", "research", 1, []int{500}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := serverGuardsWait(t, c.ticketType, "sonnet", "claude-sonnet-5-5", c.threshold, c.counts)
			if f.err != nil || len(f.fired) != 0 {
				t.Errorf("err = %v, alert counts = %v, want none", f.err, f.fired)
			}
		})
	}
}
