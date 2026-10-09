package runnerfake

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
)

var _ agentrunner.Runner = (*Runner)(nil)

func start(t *testing.T, r agentrunner.Runner, label string) agentrunner.Session {
	t.Helper()
	s, err := r.Start(agentrunner.StartOptions{Label: label, Epic: "epic", Cwd: "/w", Kind: "claude"})
	if err != nil {
		t.Fatalf("Start(%q): %v", label, err)
	}
	return s
}

func status(t *testing.T, r agentrunner.Runner, s agentrunner.Session) agentrunner.Status {
	t.Helper()
	st, err := r.Status(s)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	return st
}

func TestStart_IdleAndLabelTaken(t *testing.T) {
	r := NewRunner()
	s := start(t, r, "a")
	if s.Label != "a" || s.ID == "" || s.SessionID == "" {
		t.Fatalf("session = %+v, want label a with ids", s)
	}
	if st := status(t, r, s); st.State != agentrunner.StateIdle || st.Turn != 0 || st.SessionID != s.SessionID {
		t.Fatalf("status = %+v, want idle turn 0", st)
	}
	_, err := r.Start(agentrunner.StartOptions{Label: "a", Epic: "epic"})
	if !errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Fatalf("second Start err = %v, want ErrLabelTaken", err)
	}
}

func TestStartAndStopErrs(t *testing.T) {
	r := NewRunner()
	r.SetStartErr("a", agentrunner.ErrLabelTaken)
	if _, err := r.Start(agentrunner.StartOptions{Label: "a"}); !errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Fatalf("Start err = %v, want ErrLabelTaken", err)
	}
	r.SetStartErr("a", nil)
	s := start(t, r, "a")

	failed := errors.New("tab close failed")
	r.SetStopErr("a", failed)
	if err := r.Stop(s); !errors.Is(err, failed) {
		t.Fatalf("Stop err = %v, want %v", err, failed)
	}
	if _, ok, _ := r.Find("a"); !ok {
		t.Fatal("a failed Stop left no session, want it still live")
	}
	r.SetStopErr("a", nil)
	if err := r.Stop(s); err != nil {
		t.Fatalf("Stop after clear = %v", err)
	}
}

func TestPrompt_StartsTurnAndRecordsText(t *testing.T) {
	r := NewRunner()
	s := start(t, r, "a")
	if err := r.Prompt(s, "do it"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if st := status(t, r, s); st.State != agentrunner.StateWorking || st.Turn != 1 {
		t.Fatalf("status = %+v, want working turn 1", st)
	}
	if got := r.Prompts("a"); len(got) != 1 || got[0] != "do it" {
		t.Fatalf("Prompts = %v", got)
	}
}

func TestPrompt_PromptStateAndInjectedError(t *testing.T) {
	r := NewRunner()
	r.PromptState = agentrunner.StateDone
	s := start(t, r, "a")
	if err := r.Prompt(s, "x"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if st := status(t, r, s); st.State != agentrunner.StateDone {
		t.Fatalf("state = %s, want done", st.State)
	}
	r.SetPromptErr("a", agentrunner.ErrNotDelivered)
	if err := r.Prompt(s, "y"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt err = %v, want ErrNotDelivered", err)
	}
	if st := status(t, r, s); st.Turn != 1 {
		t.Fatalf("turn = %d after undelivered prompt, want 1", st.Turn)
	}
}

func TestBlocked_PromptNotReadyAndAnswerUnblocks(t *testing.T) {
	r := NewRunner()
	s := start(t, r, "a")
	if err := r.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow}); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Answer on idle err = %v, want ErrNotReady", err)
	}
	r.SetState("a", agentrunner.StateBlocked, "permission: Bash")
	if st := status(t, r, s); st.State != agentrunner.StateBlocked || st.BlockedReason != "permission: Bash" {
		t.Fatalf("status = %+v", st)
	}
	if err := r.Prompt(s, "x"); !errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Prompt on blocked err = %v, want ErrNotReady", err)
	}
	a := agentrunner.Answer{Decision: agentrunner.DecisionAllow}
	if err := r.Answer(s, a); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if st := status(t, r, s); st.State != agentrunner.StateWorking || st.BlockedReason != "" {
		t.Fatalf("status after answer = %+v, want working", st)
	}
	if got := r.Answers("a"); len(got) != 1 || got[0] != a {
		t.Fatalf("Answers = %v", got)
	}
}

func TestWait_ReturnsOnStateChangeOrTimesOut(t *testing.T) {
	r := NewRunner()
	s := start(t, r, "a")
	if _, err := r.Wait(s, []agentrunner.State{agentrunner.StateDone}, 10*time.Millisecond); !errors.Is(err, agentrunner.ErrTimeout) {
		t.Fatalf("Wait err = %v, want ErrTimeout", err)
	}
	st, err := r.Wait(s, []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}, time.Millisecond)
	if err != nil || st.State != agentrunner.StateIdle {
		t.Fatalf("Wait on already idle = %+v, %v; want idle", st, err)
	}
	go func() {
		time.Sleep(5 * time.Millisecond)
		r.SetState("a", agentrunner.StateDone, "")
	}()
	st, err = r.Wait(s, []agentrunner.State{agentrunner.StateDone}, time.Second)
	if err != nil || st.State != agentrunner.StateDone {
		t.Fatalf("Wait = %+v, %v; want done", st, err)
	}
}

func TestInterruptKeepsSessionAndStopIsIdempotent(t *testing.T) {
	r := NewRunner()
	s := start(t, r, "a")
	if err := r.Prompt(s, "x"); err != nil {
		t.Fatal(err)
	}
	if err := r.Interrupt(s); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if st := status(t, r, s); st.State != agentrunner.StateIdle {
		t.Fatalf("state = %s, want idle", st.State)
	}
	if err := r.Stop(s); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := r.Stop(s); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if _, err := r.Status(s); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Fatalf("Status after Stop err = %v, want ErrNotFound", err)
	}
	if _, found, _ := r.Find("a"); found {
		t.Fatal("Find found stopped session")
	}
	start(t, r, "a") // label is free again
}

func TestFindAndListByEpic(t *testing.T) {
	r := NewRunner()
	a := start(t, r, "a")
	start(t, r, "b")
	if _, err := r.Start(agentrunner.StartOptions{Label: "c", Epic: "other"}); err != nil {
		t.Fatal(err)
	}
	got, found, err := r.Find("a")
	if err != nil || !found || got != a {
		t.Fatalf("Find = %+v, %v, %v; want %+v", got, found, err, a)
	}
	list, err := r.List("epic")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Label != "a" || list[1].Label != "b" {
		t.Fatalf("List = %+v, want a, b", list)
	}
}

func TestRateLimitAndHealth(t *testing.T) {
	r := NewRunner()
	s := start(t, r, "a")
	if _, limited, err := r.RateLimit(s); err != nil || limited {
		t.Fatalf("RateLimit = %v, %v; want not limited", limited, err)
	}
	reset := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r.SetRateLimit("a", reset)
	if at, limited, err := r.RateLimit(s); err != nil || !limited || !at.Equal(reset) {
		t.Fatalf("RateLimit = %v, %v, %v; want %v", at, limited, err, reset)
	}
	r.SetLimitedUnknownReset("a")
	if at, limited, err := r.RateLimit(s); err != nil || !limited || !at.IsZero() {
		t.Fatalf("RateLimit = %v, %v, %v; want limited, unknown reset", at, limited, err)
	}
	r.SetRateLimit("a", time.Time{})
	if _, limited, err := r.RateLimit(s); err != nil || limited {
		t.Fatalf("RateLimit after clear = %v, %v; want not limited", limited, err)
	}
	exhausted := fmt.Errorf("%w: ran out of room", agentrunner.ErrContextExhausted)
	r.SetRateLimitErr("a", exhausted)
	if _, _, err := r.RateLimit(s); !errors.Is(err, agentrunner.ErrContextExhausted) {
		t.Fatalf("RateLimit err = %v, want ErrContextExhausted", err)
	}

	if err := agentrunner.Healthy(r); err != nil {
		t.Fatalf("Healthy = %v, want nil", err)
	}
	down := errors.New("host down")
	r.SetHealthErr(down)
	if err := agentrunner.Healthy(r); !errors.Is(err, down) {
		t.Fatalf("Healthy = %v, want %v", err, down)
	}
}
