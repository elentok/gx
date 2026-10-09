// Package runnertest is the conformance suite every agentrunner.Runner
// passes. Each runner supplies a Harness that drives its fake host; the
// scenarios only go through the Runner interface plus those hooks.
package runnertest

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// Harness drives the agent behind a Runner. The hooks stand in for what a
// real agent does on its own.
type Harness struct {
	Runner agentrunner.Runner
	// FinishTurn makes the agent behind s end its current turn.
	FinishTurn func(t *testing.T, s agentrunner.Session)
	// Block makes the agent behind s wait on a prompt gx did not send.
	Block func(t *testing.T, s agentrunner.Session, reason string)
	// RateLimit makes the agent behind s report a limit that resets at resetAt.
	RateLimit func(t *testing.T, s agentrunner.Session, resetAt time.Time)
	// Restart returns a fresh Runner over the same host, as after a gx
	// restart.
	Restart func(t *testing.T) agentrunner.Runner
}

// waitTimeout bounds every Wait expected to succeed.
const waitTimeout = 5 * time.Second

var idleOrDone = []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}

// Run runs every scenario against a fresh harness from newHarness.
func Run(t *testing.T, newHarness func(t *testing.T) Harness) {
	scenarios := []struct {
		name string
		fn   func(t *testing.T, h Harness)
	}{
		{"StartIsIdle", startIsIdle},
		{"StartLabelTaken", startLabelTaken},
		{"PromptStartsTurn", promptStartsTurn},
		{"WaitTimeout", waitTimesOut},
		{"InterruptKeepsSession", interruptKeepsSession},
		{"StopIsIdempotent", stopIsIdempotent},
		{"FindAndList", findAndList},
		{"StoppedSessionNotFound", stoppedSessionNotFound},
		{"BlockedThenAnswer", blockedThenAnswer},
		{"AnswerNotBlocked", answerNotBlocked},
		{"RateLimit", rateLimit},
		{"RestartThenFind", restartThenFind},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { sc.fn(t, newHarness(t)) })
	}
}

func start(t *testing.T, r agentrunner.Runner, label, epic string) agentrunner.Session {
	t.Helper()
	s, err := r.Start(agentrunner.StartOptions{Label: label, Epic: epic, Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatalf("Start(%q): %v", label, err)
	}
	return s
}

func status(t *testing.T, r agentrunner.Runner, s agentrunner.Session) agentrunner.Status {
	t.Helper()
	st, err := r.Status(s)
	if err != nil {
		t.Fatalf("Status(%q): %v", s.Label, err)
	}
	return st
}

func prompt(t *testing.T, r agentrunner.Runner, s agentrunner.Session, text string) {
	t.Helper()
	if err := r.Prompt(s, text); err != nil {
		t.Fatalf("Prompt(%q): %v", s.Label, err)
	}
}

func startIsIdle(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	if s.Label != "e-01" || s.ID == "" {
		t.Fatalf("session = %+v, want label e-01 and an ID", s)
	}
	if st := status(t, h.Runner, s); !slices.Contains(idleOrDone, st.State) || st.Turn != 0 {
		t.Fatalf("status after Start = %+v, want idle at turn 0", st)
	}
}

func startLabelTaken(t *testing.T, h Harness) {
	start(t, h.Runner, "e-01", "e")
	_, err := h.Runner.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if !errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Fatalf("second Start err = %v, want ErrLabelTaken", err)
	}
}

func promptStartsTurn(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	if st := status(t, h.Runner, s); st.Turn != 1 {
		t.Fatalf("Turn after Prompt = %d, want 1", st.Turn)
	}
	h.FinishTurn(t, s)
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after FinishTurn: %v", err)
	}
	prompt(t, h.Runner, s, "again")
	if st := status(t, h.Runner, s); st.Turn != 2 {
		t.Fatalf("Turn after second Prompt = %d, want 2", st.Turn)
	}
}

func waitTimesOut(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	st, err := h.Runner.Wait(s, idleOrDone, 50*time.Millisecond)
	if !errors.Is(err, agentrunner.ErrTimeout) {
		t.Fatalf("Wait on a working agent err = %v, want ErrTimeout", err)
	}
	if st.State != agentrunner.StateWorking {
		t.Fatalf("Wait timeout status = %+v, want working", st)
	}
}

func interruptKeepsSession(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	if err := h.Runner.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if _, err := h.Runner.Wait(s, idleOrDone, waitTimeout); err != nil {
		t.Fatalf("Wait idle after Interrupt: %v", err)
	}
	if _, ok, err := h.Runner.Find("e-01"); err != nil || !ok {
		t.Fatalf("Find after Interrupt = %v, %v; want found", ok, err)
	}
}

func stopIsIdempotent(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	for i := range 2 {
		if err := h.Runner.Stop(s); err != nil {
			t.Fatalf("Stop #%d: %v", i+1, err)
		}
	}
	if _, ok, err := h.Runner.Find("e-01"); err != nil || ok {
		t.Fatalf("Find after Stop = %v, %v; want not found", ok, err)
	}
}

func findAndList(t *testing.T, h Harness) {
	a1 := start(t, h.Runner, "a-01", "a")
	a2 := start(t, h.Runner, "a-02", "a")
	start(t, h.Runner, "b-01", "b")

	got, ok, err := h.Runner.Find("a-02")
	if err != nil || !ok || got.ID != a2.ID {
		t.Fatalf("Find(a-02) = %+v, %v, %v; want %+v", got, ok, err, a2)
	}
	if _, ok, err := h.Runner.Find("missing"); err != nil || ok {
		t.Fatalf("Find(missing) = %v, %v; want not found", ok, err)
	}

	list, err := h.Runner.List("a")
	if err != nil {
		t.Fatalf("List(a): %v", err)
	}
	var labels []string
	for _, s := range list {
		labels = append(labels, s.Label)
	}
	slices.Sort(labels)
	if !slices.Equal(labels, []string{a1.Label, a2.Label}) {
		t.Fatalf("List(a) labels = %v, want [a-01 a-02]", labels)
	}
}

func stoppedSessionNotFound(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	if err := h.Runner.Stop(s); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := h.Runner.Prompt(s, "hi"); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Fatalf("Prompt after Stop err = %v, want ErrNotFound", err)
	}
	if _, err := h.Runner.Status(s); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Fatalf("Status after Stop err = %v, want ErrNotFound", err)
	}
}

func blockedThenAnswer(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	prompt(t, h.Runner, s, "do it")
	h.Block(t, s, "allow Bash?")
	st, err := h.Runner.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, waitTimeout)
	if err != nil {
		t.Fatalf("Wait blocked: %v", err)
	}
	if st.BlockedReason == "" {
		t.Fatalf("blocked status = %+v, want a reason", st)
	}
	if err := h.Runner.Prompt(s, "more"); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Prompt while blocked err = %v, want ErrNotReady", err)
	}
	if err := h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow}); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if st := status(t, h.Runner, s); st.State == agentrunner.StateBlocked {
		t.Fatalf("status after Answer = %+v, want not blocked", st)
	}
}

func answerNotBlocked(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	err := h.Runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow})
	if !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Answer on idle agent err = %v, want ErrNotReady", err)
	}
}

func rateLimit(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	if _, limited, err := h.Runner.RateLimit(s); err != nil || limited {
		t.Fatalf("RateLimit on fresh agent = %v, %v; want not limited", limited, err)
	}
	resetAt := time.Now().Add(2 * time.Hour).Truncate(time.Minute)
	h.RateLimit(t, s, resetAt)
	got, limited, err := h.Runner.RateLimit(s)
	if err != nil || !limited {
		t.Fatalf("RateLimit = %v, %v; want limited", limited, err)
	}
	// Scraped reset times are only minute-precise.
	if d := got.Sub(resetAt).Abs(); d > time.Minute {
		t.Fatalf("RateLimit resetAt = %v, want ~%v", got, resetAt)
	}
}

func restartThenFind(t *testing.T, h Harness) {
	s := start(t, h.Runner, "e-01", "e")
	r := h.Restart(t)
	got, ok, err := r.Find("e-01")
	if err != nil || !ok || got.ID != s.ID {
		t.Fatalf("Find after restart = %+v, %v, %v; want %+v", got, ok, err, s)
	}
	status(t, r, got)
}
