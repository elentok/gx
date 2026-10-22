package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const answerAddr = "proj:epic-a/01"

// pauseWithQuestion turns the waiting run's ticket into a self-reported pause
// with one open question, and returns the state token a client would hold.
func pauseWithQuestion(t *testing.T, ticketPath, block string) string {
	t.Helper()
	err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
		tk.Status = schema.StatusLegacyAnswer
		tk.PauseKind = schema.PauseKindSelfReported
	})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := tickets.AppendLog(ticketPath, tickets.LogEvent{
		Type: tickets.EventPauseQuestion, By: "agent", Summary: "which?", Handoff: "h", Block: block,
	})
	if err != nil {
		t.Fatal(err)
	}
	return answerToken(ev.Seq, schema.StatusLegacyAnswer)
}

func answerToken(seq int, status schema.Status) string {
	return strings.Join([]string{string(rune('0' + seq)), string(status)}, ":")
}

func answerCount(t *testing.T, ticketPath string) int {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range evs {
		if ev.Type == tickets.EventPauseAnswer {
			n++
		}
	}
	return n
}

const optionsBlock = "type: options\noptions:\n  - {id: a, label: A}\n  - {id: b, label: B}\n"

func TestAnswer_DeliversTheFrameAndClaimsTheTicket(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{
		Address: answerAddr, State: state, Choice: "b", Note: "line1\r\nline2", Source: "cli",
	})
	if err != nil || res.Refused || res.Outcome != server.OutcomeDelivered {
		t.Fatalf("answer = %+v, %v; want delivered", res, err)
	}
	prompts := fake.Prompts("epic-a-iter-01")
	want := "Reply from the user to your question 1: b\nline1\nline2"
	if len(prompts) != 1 || prompts[0] != want {
		t.Errorf("prompts = %q, want [%q]", prompts, want)
	}
	if n := answerCount(t, ticketPath); n != 1 {
		t.Errorf("pause.answer lines = %d, want 1", n)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Status != schema.StatusClaimed {
		t.Errorf("ticket = %+v, %v; want claimed", tk, err)
	}
}

func TestAnswer_NotAnswerable(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	for name, pause := range map[string]func(*schema.Ticket){
		"zero-commit pause": func(tk *schema.Ticket) {
			tk.Status, tk.PauseKind = schema.StatusLegacyAnswer, schema.PauseKindZeroCommit
		},
		"repair pause": func(tk *schema.Ticket) {
			tk.Status, tk.PauseKind = schema.StatusLegacyRepair, schema.PauseKindSelfReported
		},
	} {
		if err := schema.UpdateTicket(ticketPath, pause); err != nil {
			t.Fatal(err)
		}
		res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, Choice: "a"})
		if err != nil || res.Reason != server.ReasonNotAnswerable {
			t.Errorf("%s: %+v, %v; want %s", name, res, err, server.ReasonNotAnswerable)
		}
	}
	if n := answerCount(t, ticketPath); n != 0 {
		t.Errorf("refusals logged %d pause.answer lines, want 0", n)
	}
	if got := fake.Prompts("epic-a-iter-01"); len(got) != 0 {
		t.Errorf("refusals prompted the agent: %q", got)
	}
}

func TestAnswer_BudgetRefusalLogsAndPromptsNothing(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 5)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	h.Server.RecordSpend("spent", 6)
	res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
	if err != nil || res.Reason != server.ReasonBudgetLimit {
		t.Fatalf("answer = %+v, %v; want %s", res, err, server.ReasonBudgetLimit)
	}
	if n := answerCount(t, ticketPath); n != 0 {
		t.Errorf("pause.answer lines = %d, want 0", n)
	}
	if got := fake.Prompts("epic-a-iter-01"); len(got) != 0 {
		t.Errorf("budget refusal prompted the agent: %q", got)
	}
}

// A finish that arrived after a zero-commit pause is adopted by the answer
// verb, as it is by resume: the reply is not delivered.
func TestAnswer_LateFinishIsAdoptedAndWritesNoAnswer(t *testing.T) {
	p := pauseZeroCommit(t, false)
	commitAgentWork(t, p.wtPath)
	if err := schema.UpdateTicket(p.ticketPath, p.finishedLate(false)); err != nil {
		t.Fatal(err)
	}
	res, err := p.h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, Choice: "a"})
	if err != nil || res.Refused || res.Outcome != server.OutcomeAdopted {
		t.Fatalf("answer = %+v, %v; want adopted", res, err)
	}
	if n := answerCount(t, p.ticketPath); n != 0 {
		t.Errorf("pause.answer lines = %d, want 0", n)
	}
}

func TestAnswer_RunSettlesWhenTheAgentFinishes(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
	if err != nil || res.Outcome != server.OutcomeDelivered {
		t.Fatalf("answer = %+v, %v; want delivered", res, err)
	}
	fake.SetState("epic-a-iter-01", agentrunner.StateIdle, "")
	waitFor(t, func() bool { return len(h.Server.Runs()) == 0 })
	waitStatus(t, ticketPath, schema.StatusDone)
}

// requireQueued checks the outcome of an answer to a gone agent: logged,
// reopened, answered, and nothing launched.
func requireQueued(t *testing.T, h *servertest.Harness, ticketPath string, fake *headlessFake, res server.QueueResult, err error) {
	t.Helper()
	if err != nil || res.Refused || res.Outcome != server.OutcomeQueued {
		t.Fatalf("answer = %+v, %v; want queued", res, err)
	}
	if n := answerCount(t, ticketPath); n != 1 {
		t.Errorf("pause.answer lines = %d, want 1", n)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil || !tickets.Answered(tk.Status, evs) {
		t.Errorf("Answered = false (%v), want true", err)
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("Runs() = %+v, want none", runs)
	}
	if got := fake.Prompts("epic-a-iter-01"); len(got) > 1 {
		t.Errorf("prompts = %q, want at most the one racing prompt", got)
	}
}

func TestAnswer_GoneAgentQueues(t *testing.T) {
	// The hard limit is spent: a queued answer skips the budget refusal.
	h, ticketPath, fake := startWaitingRestartFake(t, 5)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	h.Server.RecordSpend("spent", 6)
	sess, ok, err := fake.Find("epic-a-iter-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	if err := fake.Stop(sess); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
	requireQueued(t, h, ticketPath, fake, res, err)
}

func TestAnswer_DeathAfterPromptQueues(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	fake.DieAfterPrompt("epic-a-iter-01", agentrunner.SessionDied("claude exited (code 1)"))
	res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
	requireQueued(t, h, ticketPath, fake, res, err)
}

func TestAnswer_Refusals(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	ctx := context.Background()
	ask := func(req server.QueueRequest) server.QueueResult {
		t.Helper()
		req.Address = answerAddr
		res, err := h.Client.TicketAnswer(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := ask(server.QueueRequest{Choice: "a"}); res.Reason != server.ReasonNotPaused {
		t.Errorf("claimed ticket: %+v, want %s", res, server.ReasonNotPaused)
	}
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	cases := []struct {
		name   string
		req    server.QueueRequest
		reason string
	}{
		{"stale", server.QueueRequest{State: "99:paused", Choice: "a"}, server.ReasonStale},
		{"bad choice", server.QueueRequest{State: state, Choice: "zzz"}, server.ReasonBadChoice},
		{"empty", server.QueueRequest{State: state}, server.ReasonEmptyAnswer},
		{"too long", server.QueueRequest{State: state, Choice: "a", Note: strings.Repeat("x", 4001)}, server.ReasonTooLong},
	}
	for _, c := range cases {
		if res := ask(c.req); !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, want %s", c.name, res, c.reason)
		}
	}
	if res := ask(server.QueueRequest{State: "99:paused", Choice: "a"}); res.Question == nil || res.State != state {
		t.Errorf("stale carries question %v state %q, want the current one (%q)", res.Question, res.State, state)
	}
	fake.SetState("epic-a-iter-01", agentrunner.StateBlocked, "permission")
	if res := ask(server.QueueRequest{State: state, Choice: "a"}); res.Reason != server.ReasonAgentNotReady {
		t.Errorf("blocked agent: %+v, want %s", res, server.ReasonAgentNotReady)
	}
	if n := answerCount(t, ticketPath); n != 0 {
		t.Errorf("refusals logged %d pause.answer lines, want 0", n)
	}
	if got := fake.Prompts("epic-a-iter-01"); len(got) != 0 {
		t.Errorf("refusals prompted the agent: %q", got)
	}
}
