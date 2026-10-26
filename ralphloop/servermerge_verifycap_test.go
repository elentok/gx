package ralphloop

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
)

// serverVerifyCapWait runs WaitIterationFinished on a pane whose first polls
// time out, with the occupancy reader fixed at occupancy.
func serverVerifyCapWait(t *testing.T, ticketType string, agent AgentKind, polls, occupancy int) verifyCapFixture {
	t.Helper()
	r := boundGiveUpRunner(func(wait int) bool { return wait <= polls })
	// The server's wait does not stop on blocked here, so every Wait is a poll.
	r.finishPollsOnly = false
	scratchDir := epicScratchDir(t, "epic")
	read := func(string, string) (int, bool, error) { return occupancy, true, nil }
	d := Deps{Runner: r, ReadOccupancy: read, ReadCodexContext: read, Sleep: func(time.Duration) {}}
	one := OneIteration{
		Agent: agent, Epic: "epic", ScratchDir: scratchDir,
		Ticket: tickets.Ticket{Identifier: "19", Type: ticketType},
	}
	err := WaitIterationFinished(d, one, IterationWorktree{Label: "iter-19", Path: "/repo/iter-19"}, agentrunner.Session{Label: "iter-19", ID: "pane-1"})
	return verifyCapFixture{r: r, scratchDir: scratchDir, err: err}
}

func TestWaitIterationFinished_VerifyCap_StopsVerifyOnceButNotImplement(t *testing.T) {
	t.Parallel()
	above := defaultSmartZone - verifyCapMargin + 1

	v := serverVerifyCapWait(t, "verify", AgentClaude, 3, above)
	if v.err != nil {
		t.Fatal(v.err)
	}
	prompts := v.r.Prompts("iter-19")
	if v.r.interrupts != 1 || len(prompts) != 1 || !strings.Contains(prompts[0], "cap-stop step") || slices.Contains(prompts, "/compact") {
		t.Errorf("interrupts=%d prompts=%q, want one interrupt and one finish-up prompt", v.r.interrupts, prompts)
	}
	if n := capStopEvents(t, v.scratchDir); n != 1 {
		t.Errorf("verify-cap-stopped events = %d, want 1", n)
	}

	i := serverVerifyCapWait(t, "implement", AgentClaude, 3, above)
	if i.err != nil {
		t.Fatal(i.err)
	}
	if i.r.interrupts != 0 || len(i.r.Prompts("iter-19")) != 0 {
		t.Errorf("implement got interrupts=%d prompts=%q, want neither", i.r.interrupts, i.r.Prompts("iter-19"))
	}
}

func TestWaitIterationFinished_VerifyCap_BelowCapAndCodex(t *testing.T) {
	t.Parallel()
	below := serverVerifyCapWait(t, "verify", AgentClaude, 2, defaultSmartZone-verifyCapMargin)
	if below.err != nil {
		t.Fatal(below.err)
	}
	if below.r.interrupts != 0 || len(below.r.Prompts("iter-19")) != 0 {
		t.Errorf("below the cap: interrupts=%d prompts=%q, want neither", below.r.interrupts, below.r.Prompts("iter-19"))
	}

	codex := serverVerifyCapWait(t, "verify", AgentCodex, 2, defaultSmartZone-verifyCapMargin+1)
	if codex.err != nil {
		t.Fatal(codex.err)
	}
	if codex.r.interrupts != 1 || len(codex.r.Prompts("iter-19")) != 1 {
		t.Errorf("codex interrupts=%d prompts=%q, want one interrupt and one prompt", codex.r.interrupts, codex.r.Prompts("iter-19"))
	}
}
