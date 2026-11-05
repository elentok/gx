package supervise

import (
	"errors"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

const verifyCapTestZone = 130_000

func runVerifyCapWait(t *testing.T, ticketType string, agent agent.AgentKind, smartZone, polls int, occupancy func(prompts []string) int, tweak ...func(*fakes.BlipRunner)) fakes.VerifyCapFixture {
	t.Helper()
	r := fakes.BoundGiveUpRunner(func(wait int) bool { return wait <= polls })
	for _, f := range tweak {
		f(r)
	}
	scratchDir := fixture.EpicScratchDir(t, "epic")
	read := func(string, string) (int, bool, error) { return occupancy(r.Prompts("iter-19")), true, nil }
	d := deps.Deps{
		Runner:           r,
		ReadOccupancy:    read,
		ReadCodexContext: read,
		Sleep:            func(time.Duration) {},
	}
	p := boundGiveUpParams()
	p.Agent = agent
	p.SmartZone = smartZone
	p.TicketData = tickets.Ticket{Identifier: "19", Type: ticketType}
	p.VerifyCap = new(VerifyCapState)
	p.ScratchDir = scratchDir
	p.EpicName = "epic"
	err := WaitForFinish(d, p, "sess-19")
	return fakes.VerifyCapFixture{R: r, ScratchDir: scratchDir, Err: err}
}

func constOccupancy(n int) func([]string) int { return func([]string) int { return n } }

func TestWaitForFinish_VerifyCap_StopsVerifyButNotImplement(t *testing.T) {
	t.Parallel()
	above := constOccupancy(verifyCapTestZone - VerifyCapMargin + 1)

	v := runVerifyCapWait(t, "verify", agent.AgentClaude, verifyCapTestZone, 2, above)
	if v.Err != nil {
		t.Fatal(v.Err)
	}
	if v.R.Interrupts != 1 {
		t.Errorf("verify interrupts = %d, want 1", v.R.Interrupts)
	}
	prompts := v.R.Prompts("iter-19")
	if len(prompts) != 1 || !strings.Contains(prompts[0], "cap-stop step") || !strings.Contains(prompts[0], agent.VerifySkill) {
		t.Errorf("verify prompts = %q, want one finish-up prompt naming the %s cap-stop step", prompts, agent.VerifySkill)
	}
	if got := fakes.CapStopEvents(t, v.ScratchDir); got != 1 {
		t.Errorf("verify-cap-stopped events = %d, want 1", got)
	}

	i := runVerifyCapWait(t, "implement", agent.AgentClaude, verifyCapTestZone, 2, above)
	if i.Err != nil {
		t.Fatal(i.Err)
	}
	if i.R.Interrupts != 0 || len(i.R.Prompts("iter-19")) != 0 {
		t.Errorf("implement got interrupts=%d prompts=%q, want neither", i.R.Interrupts, i.R.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_FiredStateSurvivesASecondWait(t *testing.T) {
	t.Parallel()
	r := fakes.BoundGiveUpRunner(func(wait int) bool { return wait == 1 || wait == 3 })
	read := func(string, string) (int, bool, error) { return verifyCapTestZone, true, nil }
	d := deps.Deps{Runner: r, ReadOccupancy: read, ReadCodexContext: read, Sleep: func(time.Duration) {}}
	p := boundGiveUpParams()
	p.SmartZone = verifyCapTestZone
	p.TicketData = tickets.Ticket{Identifier: "19", Type: "verify"}
	p.ScratchDir = fixture.EpicScratchDir(t, "epic")
	p.EpicName = "epic"
	p.VerifyCap = new(VerifyCapState)
	for i := range 2 {
		if err := WaitForFinish(d, p, "sess-19"); err != nil {
			t.Fatalf("wait %d: %v", i, err)
		}
	}
	if got := len(r.Prompts("iter-19")); got != 1 {
		t.Errorf("cap prompts across two waits = %d, want 1", got)
	}
}

func TestWaitForFinish_VerifyCap_FailedPromptSendLogsCapStop(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", agent.AgentClaude, verifyCapTestZone, 3, constOccupancy(verifyCapTestZone),
		func(r *fakes.BlipRunner) { r.PromptErr = errors.New("send failed") })
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	evs, _, err := runlog.ReadEvents(got.ScratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	var capStops, recoveryFailed int
	for _, ev := range evs {
		switch ev.Type {
		case string(events.VerifyCapStopped):
			capStops++
		case string(events.SmartZoneRecoveryFailed):
			recoveryFailed++
		}
	}
	if recoveryFailed != 0 {
		t.Errorf("smart-zone-recovery-failed events = %d, want 0", recoveryFailed)
	}
	if capStops != 2 {
		t.Errorf("verify-cap-stopped events = %d, want the stop plus the failed send, with no retry", capStops)
	}
}

func TestWaitForFinish_VerifyCap_FollowsSmartZone(t *testing.T) {
	t.Parallel()
	const zone = 50_000
	got := runVerifyCapWait(t, "verify", agent.AgentClaude, zone, 1, constOccupancy(zone-VerifyCapMargin+1))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.R.Interrupts != 1 || len(got.R.Prompts("iter-19")) != 1 {
		t.Errorf("interrupts=%d prompts=%q, want the cap to sit at smart zone minus the margin", got.R.Interrupts, got.R.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_AbsentAtSmallSmartZone(t *testing.T) {
	t.Parallel()
	// Occupancy is below the smart zone, so only a cap could act.
	got := runVerifyCapWait(t, "verify", agent.AgentClaude, VerifyCapMargin, 2, constOccupancy(VerifyCapMargin-1))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.R.Interrupts != 0 || len(got.R.Prompts("iter-19")) != 0 {
		t.Errorf("interrupts=%d prompts=%q, want no cap stop", got.R.Interrupts, got.R.Prompts("iter-19"))
	}

	// Past the smart zone, normal recovery still runs.
	got = runVerifyCapWait(t, "verify", agent.AgentClaude, VerifyCapMargin, 1, func(prompts []string) int {
		if slices.Contains(prompts, "/compact") {
			return 1
		}
		return VerifyCapMargin + 1
	})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if !slices.Contains(got.R.Prompts("iter-19"), "/compact") || fakes.CapStopEvents(t, got.ScratchDir) != 0 {
		t.Errorf("prompts = %q, want smart-zone recovery and no cap stop", got.R.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_Codex(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", agent.AgentCodex, verifyCapTestZone, 2, constOccupancy(verifyCapTestZone-VerifyCapMargin+1))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.R.Interrupts != 1 || len(got.R.Prompts("iter-19")) != 1 {
		t.Errorf("interrupts=%d prompts=%q, want one interrupt and one prompt", got.R.Interrupts, got.R.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_FiresOncePerSession(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", agent.AgentClaude, verifyCapTestZone, 4, constOccupancy(verifyCapTestZone-VerifyCapMargin+1))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	prompts := got.R.Prompts("iter-19")
	if got.R.Interrupts != 1 || len(prompts) != 1 || slices.Contains(prompts, "/compact") {
		t.Errorf("interrupts=%d prompts=%q, want one interrupt, one prompt and no /compact", got.R.Interrupts, prompts)
	}
	if n := fakes.CapStopEvents(t, got.ScratchDir); n != 1 {
		t.Errorf("verify-cap-stopped events = %d, want 1", n)
	}
}

func TestWaitForFinish_VerifyCap_SmartZoneStillAppliesAfterwards(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", agent.AgentClaude, verifyCapTestZone, 3, func(prompts []string) int {
		switch {
		case slices.Contains(prompts, "/compact"):
			return 10_000
		case len(prompts) == 0:
			return verifyCapTestZone - VerifyCapMargin + 1
		case len(prompts) == 1:
			return verifyCapTestZone + 1
		}
		return 10_000
	})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	prompts := got.R.Prompts("iter-19")
	if len(prompts) < 3 || !strings.Contains(prompts[0], "cap-stop step") || prompts[1] != "/compact" || !strings.Contains(prompts[2], "gx-implement") {
		t.Errorf("prompts = %q, want cap finish-up, /compact, smart-zone finish-up", prompts)
	}
}

func TestVerifyCap_OptInOnly(t *testing.T) {
	t.Parallel()
	p := boundGiveUpParams()
	p.TicketData = tickets.Ticket{Identifier: "19", Type: "verify"}
	if NewVerifyCap(p, verifyCapTestZone) != nil {
		t.Error("a pane that did not opt in got a cap")
	}
	p.VerifyCap = new(VerifyCapState)
	if NewVerifyCap(p, verifyCapTestZone) == nil {
		t.Error("an opted-in verify pane got no cap")
	}
}
