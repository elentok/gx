package nativerunner_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
)

func (h *harness) blockedOn(t *testing.T, r *nativerunner.Headless, s agentrunner.Session) agentrunner.Status {
	t.Helper()
	st, err := r.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait blocked: %v", err)
	}
	return st
}

func TestHeadless_StatusPendingFromOpenRequest(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")

	if st, _ := h.runner.Status(s); st.Pending != nil {
		t.Fatalf("Pending on an idle agent = %+v, want nil", st.Pending)
	}
	if err := h.runner.Prompt(s, "permit:Bash"); err != nil {
		t.Fatal(err)
	}
	p := h.blockedOn(t, h.runner, s).Pending
	if p == nil || p.ID != "req-Bash" || p.Tool != "Bash" || string(p.Input) != `{"x":1}` || p.Description != "run it" || p.NotApprovable {
		t.Fatalf("Pending = %+v, want the open Bash request", p)
	}
}

func TestHeadless_PlanAndQuestionAreNotApprovableByToolName(t *testing.T) {
	for _, tool := range []string{"ExitPlanMode", "AskUserQuestion"} {
		h := newHarness(t)
		s := h.start(t, "epic-07")
		if err := h.runner.Prompt(s, "permit:"+tool); err != nil {
			t.Fatal(err)
		}
		if p := h.blockedOn(t, h.runner, s).Pending; p == nil || !p.NotApprovable {
			t.Errorf("%s Pending = %+v, want NotApprovable", tool, p)
		}
	}
}

func TestHeadless_AnswerRefusesRequestIDThatIsNotOpen(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")
	if err := h.runner.Prompt(s, "permit:Bash"); err != nil {
		t.Fatal(err)
	}
	h.blockedOn(t, h.runner, s)

	err := h.runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "req-other"})
	if !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("Answer with a wrong id = %v, want ErrRequestMismatch", err)
	}
	if st, _ := h.runner.Status(s); st.Pending == nil {
		t.Fatal("a refused answer closed the request")
	}
	if err := h.runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "req-Bash"}); err != nil {
		t.Fatalf("Answer with the open id: %v", err)
	}
	if err := h.runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "req-Bash"}); !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("second Answer = %v, want ErrRequestMismatch", err)
	}
}

func TestHeadless_AnswerWithoutRequestIDAnswersTheOpenRequest(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")
	if err := h.runner.Prompt(s, "permit:Bash"); err != nil {
		t.Fatal(err)
	}
	h.blockedOn(t, h.runner, s)
	if err := h.runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny}); err != nil {
		t.Fatal(err)
	}
}

func TestHeadless_ReplayAfterRestartSkipsAnsweredRequest(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-07")
	if err := h.runner.Prompt(s, "permit:Bash"); err != nil {
		t.Fatal(err)
	}
	h.blockedOn(t, h.runner, s)
	if err := h.runner.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "req-Bash"}); err != nil {
		t.Fatal(err)
	}

	// The fake never moves on after the answer, so out.jsonl still ends with
	// the open request, as it does for a real claude that has not written yet.
	restarted := &nativerunner.Headless{
		Root: h.root, Claude: h.runner.Claude, PollInterval: 5 * time.Millisecond,
		StopGrace: 2 * time.Second, PromptTimeout: 5 * time.Second,
	}
	got, verdict, err := restarted.Reattach("epic-07")
	if err != nil || verdict != nativerunner.VerdictLive {
		t.Fatalf("Reattach = %v, %v; want live", verdict, err)
	}
	if st, err := restarted.Status(got); err != nil || st.Pending != nil {
		t.Fatalf("status after replay = %+v, %v; want no pending request", st, err)
	}
	if err := restarted.Answer(got, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "req-Bash"}); !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("Answer of the replayed request = %v, want ErrRequestMismatch", err)
	}
}
