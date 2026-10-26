package ralphloop

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

const verifyCapTestZone = 130_000

// verifyCapFixture runs waitForFinish on a pane whose first polls time out,
// reading occupancy from occupancy(prompts so far).
type verifyCapFixture struct {
	r          *blipRunner
	scratchDir string
	err        error
}

func runVerifyCapWait(t *testing.T, ticketType string, agent AgentKind, smartZone, polls int, occupancy func(prompts []string) int, tweak ...func(*blipRunner)) verifyCapFixture {
	t.Helper()
	r := boundGiveUpRunner(func(wait int) bool { return wait <= polls })
	for _, f := range tweak {
		f(r)
	}
	scratchDir := epicScratchDir(t, "epic")
	read := func(string, string) (int, bool, error) { return occupancy(r.Prompts("iter-19")), true, nil }
	d := Deps{
		Runner:           r,
		ReadOccupancy:    read,
		ReadCodexContext: read,
		Sleep:            func(time.Duration) {},
	}
	p := boundGiveUpParams()
	p.Agent = agent
	p.SmartZone = smartZone
	p.TicketData = tickets.Ticket{Identifier: "19", Type: ticketType}
	p.ScratchDir = scratchDir
	p.EpicName = "epic"
	err := waitForFinish(d, p, "sess-19")
	return verifyCapFixture{r: r, scratchDir: scratchDir, err: err}
}

func constOccupancy(n int) func([]string) int { return func([]string) int { return n } }

func capStopEvents(t *testing.T, scratchDir string) int {
	t.Helper()
	evs, _, err := ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range evs {
		if ev.Type == string(events.VerifyCapStopped) {
			n++
		}
	}
	return n
}

func TestWaitForFinish_VerifyCap_StopsVerifyButNotImplement(t *testing.T) {
	t.Parallel()
	above := constOccupancy(verifyCapTestZone - verifyCapMargin + 1)

	v := runVerifyCapWait(t, "verify", AgentClaude, verifyCapTestZone, 2, above)
	if v.err != nil {
		t.Fatal(v.err)
	}
	if v.r.interrupts != 1 {
		t.Errorf("verify interrupts = %d, want 1", v.r.interrupts)
	}
	prompts := v.r.Prompts("iter-19")
	if len(prompts) != 1 || !strings.Contains(prompts[0], "cap-stop step") || !strings.Contains(prompts[0], verifySkill) {
		t.Errorf("verify prompts = %q, want one finish-up prompt naming the %s cap-stop step", prompts, verifySkill)
	}
	if got := capStopEvents(t, v.scratchDir); got != 1 {
		t.Errorf("verify-cap-stopped events = %d, want 1", got)
	}

	i := runVerifyCapWait(t, "implement", AgentClaude, verifyCapTestZone, 2, above)
	if i.err != nil {
		t.Fatal(i.err)
	}
	if i.r.interrupts != 0 || len(i.r.Prompts("iter-19")) != 0 {
		t.Errorf("implement got interrupts=%d prompts=%q, want neither", i.r.interrupts, i.r.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_FailedPromptSendLogsCapStop(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", AgentClaude, verifyCapTestZone, 3, constOccupancy(verifyCapTestZone),
		func(r *blipRunner) { r.promptErr = errors.New("send failed") })
	if got.err != nil {
		t.Fatal(got.err)
	}
	evs, _, err := ReadEvents(got.scratchDir, "epic")
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
	got := runVerifyCapWait(t, "verify", AgentClaude, zone, 1, constOccupancy(zone-verifyCapMargin+1))
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.r.interrupts != 1 || len(got.r.Prompts("iter-19")) != 1 {
		t.Errorf("interrupts=%d prompts=%q, want the cap to sit at smart zone minus the margin", got.r.interrupts, got.r.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_AbsentAtSmallSmartZone(t *testing.T) {
	t.Parallel()
	// Occupancy is below the smart zone, so only a cap could act.
	got := runVerifyCapWait(t, "verify", AgentClaude, verifyCapMargin, 2, constOccupancy(verifyCapMargin-1))
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.r.interrupts != 0 || len(got.r.Prompts("iter-19")) != 0 {
		t.Errorf("interrupts=%d prompts=%q, want no cap stop", got.r.interrupts, got.r.Prompts("iter-19"))
	}

	// Past the smart zone, normal recovery still runs.
	got = runVerifyCapWait(t, "verify", AgentClaude, verifyCapMargin, 1, func(prompts []string) int {
		if slices.Contains(prompts, "/compact") {
			return 1
		}
		return verifyCapMargin + 1
	})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !slices.Contains(got.r.Prompts("iter-19"), "/compact") || capStopEvents(t, got.scratchDir) != 0 {
		t.Errorf("prompts = %q, want smart-zone recovery and no cap stop", got.r.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_Codex(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", AgentCodex, verifyCapTestZone, 2, constOccupancy(verifyCapTestZone-verifyCapMargin+1))
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.r.interrupts != 1 || len(got.r.Prompts("iter-19")) != 1 {
		t.Errorf("interrupts=%d prompts=%q, want one interrupt and one prompt", got.r.interrupts, got.r.Prompts("iter-19"))
	}
}

func TestWaitForFinish_VerifyCap_FiresOncePerSession(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", AgentClaude, verifyCapTestZone, 4, constOccupancy(verifyCapTestZone-verifyCapMargin+1))
	if got.err != nil {
		t.Fatal(got.err)
	}
	prompts := got.r.Prompts("iter-19")
	if got.r.interrupts != 1 || len(prompts) != 1 || slices.Contains(prompts, "/compact") {
		t.Errorf("interrupts=%d prompts=%q, want one interrupt, one prompt and no /compact", got.r.interrupts, prompts)
	}
	if n := capStopEvents(t, got.scratchDir); n != 1 {
		t.Errorf("verify-cap-stopped events = %d, want 1", n)
	}
}

func TestWaitForFinish_VerifyCap_SmartZoneStillAppliesAfterwards(t *testing.T) {
	t.Parallel()
	got := runVerifyCapWait(t, "verify", AgentClaude, verifyCapTestZone, 3, func(prompts []string) int {
		switch {
		case slices.Contains(prompts, "/compact"):
			return 10_000
		case len(prompts) == 0:
			return verifyCapTestZone - verifyCapMargin + 1
		case len(prompts) == 1:
			return verifyCapTestZone + 1
		}
		return 10_000
	})
	if got.err != nil {
		t.Fatal(got.err)
	}
	prompts := got.r.Prompts("iter-19")
	if len(prompts) < 3 || !strings.Contains(prompts[0], "cap-stop step") || prompts[1] != "/compact" || !strings.Contains(prompts[2], "gx-implement") {
		t.Errorf("prompts = %q, want cap finish-up, /compact, smart-zone finish-up", prompts)
	}
}

func TestVerifyCap_NotAppliedToConflictResolutionPane(t *testing.T) {
	t.Parallel()
	p := boundGiveUpParams()
	p.TicketData = tickets.Ticket{Identifier: "19", Type: "verify"}
	if newVerifyCap(p, verifyCapTestZone) == nil {
		t.Fatal("verify pane got no cap")
	}
	p.noVerifyCap = true
	if newVerifyCap(p, verifyCapTestZone) != nil {
		t.Error("conflict-resolution pane got a cap")
	}
}
