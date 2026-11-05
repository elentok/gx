package supervise

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
)

func TestWaitForFinish_CodexNativeContextFailureRecoversDespiteStaleOccupancy(t *testing.T) {
	t.Parallel()
	const failure = "■ stream disconnected before completion: Your input exceeds the context window of this model. Please adjust your input and try again."
	scratchDir := fixture.EpicScratchDir(t, "epic")
	r := &fakes.BlipRunner{Runner: fakes.IdlePromptRunner("iter-20", "codex-session-20"), TimeoutOn: 1, ContextExhausted: failure}
	d := deps.Deps{
		Runner: r,
		ReadCodexContext: func(string, string) (int, bool, error) {
			return 1_000, true, nil
		},
		Sleep: func(time.Duration) {},
	}

	err := WaitForFinish(d, LaunchParams{
		Label: "iter-20", Agent: agent.AgentCodex, Pane: "pane-1", Ticket: "20",
		SessionCwd: "/repo/iter-20", SmartZone: 150_000, ScratchDir: scratchDir,
		EpicName: "epic", Gate: NewGate(),
	}, "codex-session-20")
	if err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if r.Interrupts != 1 {
		t.Errorf("pane interruptions = %d, want 1", r.Interrupts)
	}
	if prompts := r.Prompts("iter-20"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want compact then finish-up", prompts)
	}
	events, ok, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil || !ok || len(events) == 0 {
		t.Fatalf("ReadEvents() = %+v, ok=%v, err=%v", events, ok, err)
	}
	if events[0].Type != string(eventsc.PausedSmartZone) || !strings.Contains(events[0].Reason, "input exceeds the context window") {
		t.Errorf("recovery event = %+v, want native failure evidence", events[0])
	}
}

func TestWaitForFinish_CodexNativeContextFailureDetectedWhenSettled(t *testing.T) {
	t.Parallel()
	r := &fakes.BlipRunner{
		Runner:           fakes.IdlePromptRunner("iter-20", "codex-session-20"),
		ContextExhausted: `Error running remote compact task: {"error":{"code":"context_length_exceeded"}}`,
	}
	d := deps.Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := WaitForFinish(d, LaunchParams{
		Label: "iter-20", Agent: agent.AgentCodex, Pane: "pane-1", Ticket: "20",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-20")
	if err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if r.Interrupts != 1 {
		t.Errorf("pane interruptions = %d, want 1 before accepting settled state", r.Interrupts)
	}
}

func TestWaitForFinish_CodexNativeContextFailureRecoveryFailureIsDurable(t *testing.T) {
	t.Parallel()
	const failure = "■ Codex ran out of room in the model's context window."
	r := &fakes.BlipRunner{Runner: fakes.IdlePromptRunner("iter-21", "codex-session-21"), ContextExhausted: failure}
	r.FailNextPrompts("iter-21", 1, errors.New("compact never merged"))
	d := deps.Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := WaitForFinish(d, LaunchParams{
		Label: "iter-21", Agent: agent.AgentCodex, Pane: "pane-1", Ticket: "21",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-21")
	if err == nil {
		t.Fatal("WaitForFinish: want durable error after failed recovery, got nil")
	}
	if !strings.Contains(err.Error(), "recovery failed") || !strings.Contains(err.Error(), "context window") {
		t.Errorf("WaitForFinish error = %q, want it to name the failed recovery and evidence", err.Error())
	}
	if r.Interrupts != 1 {
		t.Errorf("pane interruptions = %d, want 1", r.Interrupts)
	}
}

func TestWaitForFinish_CodexNativeContextFailureFailsDurablyWithoutFreshTokenEvent(t *testing.T) {
	t.Parallel()
	// No ReadCodexContext dependency at all: the classification and its
	// recovery-failure path must not depend on a fresh high-token record
	// existing to fire — the native exhaustion text is itself the evidence.
	const failure = `Error running remote compact task: {"error":{"code":"context_length_exceeded"}}`
	r := &fakes.BlipRunner{Runner: fakes.IdlePromptRunner("iter-21", "codex-session-21"), ContextExhausted: failure}
	r.FailNextPrompts("iter-21", 1, errors.New("compact never merged"))
	d := deps.Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := WaitForFinish(d, LaunchParams{
		Label: "iter-21", Agent: agent.AgentCodex, Pane: "pane-1", Ticket: "21",
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-21")
	if err == nil || !strings.Contains(err.Error(), "recovery failed") {
		t.Fatalf("WaitForFinish() = %v, want a durable recovery-failed error without any ReadCodexContext dependency", err)
	}
}

func TestWaitForFinish_CodexBlockedPaneContextExhaustionRecoversInsteadOfPausing(t *testing.T) {
	t.Parallel()
	ticketPath := fixture.WriteFrontmatterTicket(t, "claimed")
	r := &fakes.BlipRunner{
		Runner:           fakes.IdlePromptRunner("iter-22", "codex-session-22"),
		ContextExhausted: "■ Codex ran out of room in the model's context window.",
	}
	r.SetState("iter-22", agentrunner.StateBlocked, "")
	d := deps.Deps{Runner: r, Sleep: func(time.Duration) {}}

	err := WaitForFinish(d, LaunchParams{
		Label: "iter-22", Agent: agent.AgentCodex, Pane: "pane-1", Ticket: "22", TicketPath: ticketPath,
		SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-22")
	if err != nil {
		t.Fatalf("WaitForFinish: %v", err)
	}
	if r.Interrupts != 1 {
		t.Errorf("pane interruptions = %d, want 1", r.Interrupts)
	}
	if prompts := r.Prompts("iter-22"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want compact then finish-up", prompts)
	}
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if fakes.RawIsPaused(t, string(raw), "") {
		t.Errorf("ticket was paused despite recoverable context exhaustion:\n%s", raw)
	}
}

// refuseFirstInterrupt answers the first Interrupt with ErrNotReady. A real
// pane shows its context-exhaustion banner on every poll, so the refusal
// re-arms fakes.BlipRunner's one-shot evidence for the retry.
type refuseFirstInterrupt struct {
	*fakes.BlipRunner
	evidence string
	refused  bool
}

func (r *refuseFirstInterrupt) Interrupt(s agentrunner.Session) error {
	if !r.refused {
		r.refused = true
		r.BlipRunner.Interrupts++
		r.BlipRunner.ContextExhausted = r.evidence
		return agentrunner.ErrNotReady
	}
	return r.BlipRunner.Interrupt(s)
}

// TestWaitForFinish_InterruptNotReadyRetriesNextPoll covers every Interrupt
// caller in recovery: ErrNotReady skips the poll, the next poll retries, and
// recovery then completes.
func TestWaitForFinish_InterruptNotReadyRetriesNextPoll(t *testing.T) {
	t.Parallel()
	const banner = "■ Codex ran out of room in the model's context window."
	cases := []struct {
		name  string
		agent agent.AgentKind
		setup func(r *fakes.BlipRunner)
		zone  bool
	}{
		{name: "codex blocked", agent: agent.AgentCodex, setup: func(r *fakes.BlipRunner) {
			r.ContextExhausted = banner
			r.SetState("iter-30", agentrunner.StateBlocked, "")
		}},
		{name: "codex idle", agent: agent.AgentCodex, setup: func(r *fakes.BlipRunner) { r.ContextExhausted = banner }},
		{name: "codex timeout while working", agent: agent.AgentCodex, setup: func(r *fakes.BlipRunner) {
			r.ContextExhausted = banner
			r.TimeoutIf = func(n int) bool { return n <= 2 }
		}},
		{name: "smart zone", agent: agent.AgentCodex, zone: true, setup: func(r *fakes.BlipRunner) { r.TimeoutIf = func(n int) bool { return n <= 2 } }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ticketPath := fixture.WriteFrontmatterTicket(t, "claimed")
			r := &fakes.BlipRunner{Runner: fakes.IdlePromptRunner("iter-30", "codex-session-30")}
			tc.setup(r)
			d := deps.Deps{Runner: &refuseFirstInterrupt{BlipRunner: r, evidence: r.ContextExhausted}, Sleep: func(time.Duration) {}}
			if tc.zone {
				d.ReadCodexContext = func(string, string) (int, bool, error) { return 150001, true, nil }
			}

			err := WaitForFinish(d, LaunchParams{
				Label: "iter-30", Agent: tc.agent, Pane: "pane-1", Ticket: "30", TicketPath: ticketPath,
				SmartZone: 150_000, Gate: NewGate(),
			}, "codex-session-30")
			if err != nil {
				t.Fatalf("WaitForFinish: %v", err)
			}
			if r.Interrupts != 2 {
				t.Errorf("interrupts = %d, want a refused one then a retry", r.Interrupts)
			}
			if prompts := r.Prompts("iter-30"); len(prompts) == 0 || prompts[0] != "/compact" {
				t.Errorf("prompts = %v, want /compact after the retry", prompts)
			}
		})
	}
}

func TestWaitForFinish_InterruptOtherErrorStillFails(t *testing.T) {
	t.Parallel()
	r := &fakes.BlipRunner{Runner: fakes.IdlePromptRunner("iter-31", "codex-session-31"), TimeoutOn: 1, InterruptErr: errors.New("pane gone")}
	d := deps.Deps{
		Runner:           r,
		ReadCodexContext: func(string, string) (int, bool, error) { return 150001, true, nil },
		Sleep:            func(time.Duration) {},
	}
	err := WaitForFinish(d, LaunchParams{
		Label: "iter-31", Agent: agent.AgentCodex, Pane: "pane-1", Ticket: "31", SmartZone: 150_000, Gate: NewGate(),
	}, "codex-session-31")
	if err == nil || !strings.Contains(err.Error(), "pane gone") {
		t.Fatalf("WaitForFinish() = %v, want the interrupt error", err)
	}
}
