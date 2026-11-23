package designflows

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/testutil"
)

const (
	launchRejectedTicket  = "demo:launch-rejected-epic/01"
	launchUndecidedTicket = "demo:launch-undecided-epic/01"
)

// waitPaused waits for the ticket to pause and returns what `gx tickets paused` prints.
func (r *running) waitPaused(t *testing.T, addr string) string {
	t.Helper()
	if !testutil.Poll(2*time.Minute, 500*time.Millisecond, func() bool {
		return strings.HasPrefix(r.TicketStatus(t, addr), "needs-")
	}) {
		t.Fatalf("ticket %s never paused:\n%s", addr, r.Gx(t, "server", "tickets", "history", addr))
	}
	return r.Gx(t, "tickets", "paused", addr)
}

// eventTypes is the run log's event types in order, limited to those the launch tests care about.
func (r *running) launchEventTypes(t *testing.T, addr string) []string {
	t.Helper()
	var out []string
	for _, typ := range []events.Type{events.PromptNudged, events.LaunchFailed, events.SkillsRefreshed, events.LaunchRetried} {
		for range r.events(t, addr, string(typ)) {
			out = append(out, string(typ))
		}
	}
	return out
}

func count(types []string, typ events.Type) int {
	n := 0
	for _, s := range types {
		if s == string(typ) {
			n++
		}
	}
	return n
}

func TestLaunchRejectedScenario_ThreeLaunchesARefreshAndThePauseWithRetypesOnlyOnUnknownCommand(t *testing.T) {
	t.Parallel()
	r := start(t, "launch-rejected")
	paused := r.waitPaused(t, launchRejectedTicket)
	if !strings.Contains(paused, "agent_prompt_stalled") {
		t.Errorf("paused kind = %q, want agent_prompt_stalled", paused)
	}
	types := r.launchEventTypes(t, launchRejectedTicket)
	if got := count(types, events.LaunchFailed); got != 3 {
		t.Errorf("launch-failed events = %d, want 3 (%v)", got, types)
	}
	if got := count(types, events.SkillsRefreshed); got != 1 {
		t.Errorf("skills-refreshed events = %d, want 1 (%v)", got, types)
	}
	if got := count(types, events.LaunchRetried); got != 1 {
		t.Errorf("launch-retried events = %d, want 1 (%v)", got, types)
	}
	if count(types, events.PromptNudged) == 0 {
		t.Errorf("no prompt-nudged event: an Unknown command answer should be retyped (%v)", types)
	}
	in := r.agentInput(t)
	if got, nudged := strings.Count(in, skillPromptSnippet), count(types, events.PromptNudged); got < nudged {
		t.Errorf("fake got the command %d times for %d retypes:\n%q", got, nudged, in)
	}
	r.Stop(t)
}

func TestLaunchUndecidedScenario_NothingIsRetypedTwoLaunchesFailAndTheTicketPauses(t *testing.T) {
	t.Parallel()
	r := start(t, "launch-undecided")
	paused := r.waitPaused(t, launchUndecidedTicket)
	if !strings.Contains(paused, "agent_prompt_stalled") {
		t.Errorf("paused kind = %q, want agent_prompt_stalled", paused)
	}
	types := r.launchEventTypes(t, launchUndecidedTicket)
	if got := count(types, events.PromptNudged); got != 0 {
		t.Errorf("prompt-nudged events = %d, want 0: the command may have landed (%v)", got, types)
	}
	if got := count(types, events.LaunchFailed); got != 2 {
		t.Errorf("launch-failed events = %d, want 2 (%v)", got, types)
	}
	in := r.agentInput(t)
	commands := strings.Count(in, skillPromptSnippet)
	if commands != 2 {
		t.Errorf("fake got the command %d times, want once per launch (2):\n%q", commands, in)
	}
	if enters := strings.Count(in, "\r"); enters > 2*(1+3) {
		t.Errorf("fake got %d Enters, want at most 3 settle Enters per launch plus the send:\n%q", enters, in)
	}
	r.Stop(t)
}
