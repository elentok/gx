package ralphloop

import (
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/supervise"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/tickets"
)

// serverVerifyCapWait runs WaitIterationFinished on a pane whose first polls
// time out, with the occupancy reader fixed at occupancy.
func serverVerifyCapWait(t *testing.T, ticketType string, agent agent.AgentKind, polls, occupancy int) fakes.VerifyCapFixture {
	t.Helper()
	r := fakes.BoundGiveUpRunner(func(wait int) bool { return wait <= polls })
	// The server's wait does not stop on blocked here, so every Wait is a poll.
	r.FinishPollsOnly = false
	scratchDir := fixture.EpicScratchDir(t, "epic")
	read := func(string, string) (int, bool, error) { return occupancy, true, nil }
	d := deps.Deps{Runner: r, ReadOccupancy: read, ReadCodexContext: read, Sleep: func(time.Duration) {}}
	one := OneIteration{
		Agent: agent, Epic: "epic", ScratchDir: scratchDir,
		Ticket: tickets.Ticket{Identifier: "19", Type: ticketType},
	}
	err := WaitIterationFinished(d, one, IterationWorktree{Label: "iter-19", Path: "/repo/iter-19"}, agentrunner.Session{Label: "iter-19", ID: "pane-1"})
	return fakes.VerifyCapFixture{R: r, ScratchDir: scratchDir, Err: err}
}

func TestWaitIterationFinished_VerifyCap_StopsVerifyOnceButNotImplement(t *testing.T) {
	t.Parallel()
	above := agent.DefaultSmartZone - supervise.VerifyCapMargin + 1

	v := serverVerifyCapWait(t, "verify", agent.AgentClaude, 3, above)
	if v.Err != nil {
		t.Fatal(v.Err)
	}
	prompts := v.R.Prompts("iter-19")
	if v.R.Interrupts != 1 || len(prompts) != 1 || !strings.Contains(prompts[0], "cap-stop step") || slices.Contains(prompts, "/compact") {
		t.Errorf("interrupts=%d prompts=%q, want one interrupt and one finish-up prompt", v.R.Interrupts, prompts)
	}
	if n := fakes.CapStopEvents(t, v.ScratchDir); n != 1 {
		t.Errorf("verify-cap-stopped events = %d, want 1", n)
	}

	i := serverVerifyCapWait(t, "implement", agent.AgentClaude, 3, above)
	if i.Err != nil {
		t.Fatal(i.Err)
	}
	if i.R.Interrupts != 0 || len(i.R.Prompts("iter-19")) != 0 {
		t.Errorf("implement got interrupts=%d prompts=%q, want neither", i.R.Interrupts, i.R.Prompts("iter-19"))
	}
}

func TestWaitIterationFinished_VerifyCap_BelowCapAndCodex(t *testing.T) {
	t.Parallel()
	below := serverVerifyCapWait(t, "verify", agent.AgentClaude, 2, agent.DefaultSmartZone-supervise.VerifyCapMargin)
	if below.Err != nil {
		t.Fatal(below.Err)
	}
	if below.R.Interrupts != 0 || len(below.R.Prompts("iter-19")) != 0 {
		t.Errorf("below the cap: interrupts=%d prompts=%q, want neither", below.R.Interrupts, below.R.Prompts("iter-19"))
	}

	codex := serverVerifyCapWait(t, "verify", agent.AgentCodex, 2, agent.DefaultSmartZone-supervise.VerifyCapMargin+1)
	if codex.Err != nil {
		t.Fatal(codex.Err)
	}
	if codex.R.Interrupts != 1 || len(codex.R.Prompts("iter-19")) != 1 {
		t.Errorf("codex interrupts=%d prompts=%q, want one interrupt and one prompt", codex.R.Interrupts, codex.R.Prompts("iter-19"))
	}
}

func TestWaitIterationFinished_VerifyCap_FollowsOneIterationSmartZone(t *testing.T) {
	t.Parallel()
	zone := agent.DefaultSmartZone + 40_000
	r := fakes.BoundGiveUpRunner(func(wait int) bool { return wait <= 3 })
	r.FinishPollsOnly = false
	scratchDir := fixture.EpicScratchDir(t, "epic")
	read := func(string, string) (int, bool, error) { return agent.DefaultSmartZone, true, nil }
	d := deps.Deps{Runner: r, ReadOccupancy: read, ReadCodexContext: read, Sleep: func(time.Duration) {}}
	one := OneIteration{
		Agent: agent.AgentClaude, Epic: "epic", ScratchDir: scratchDir, SmartZone: zone,
		Ticket: tickets.Ticket{Identifier: "19", Type: "verify"},
	}
	err := WaitIterationFinished(d, one, IterationWorktree{Label: "iter-19", Path: "/repo/iter-19"}, agentrunner.Session{Label: "iter-19", ID: "pane-1"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Interrupts != 0 {
		t.Errorf("interrupts = %d, want 0: occupancy is under the raised cap", r.Interrupts)
	}
}
