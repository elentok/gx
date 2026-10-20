package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const answerAddr = "proj:epic-a/01"

// parkWithQuestion turns the waiting run's ticket into a self-reported park
// with one open question, and returns the state token a client would hold.
func parkWithQuestion(t *testing.T, ticketPath, block string) string {
	t.Helper()
	err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
		tk.Status = schema.StatusNeedsAnswer
		tk.ParkKind = schema.ParkKindSelfReported
	})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := tickets.AppendLog(ticketPath, tickets.LogEvent{
		Type: tickets.EventParkQuestion, By: "agent", Summary: "which?", Handoff: "h", Block: block,
	})
	if err != nil {
		t.Fatal(err)
	}
	return answerToken(ev.Seq, schema.StatusNeedsAnswer)
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
		if ev.Type == tickets.EventParkAnswer {
			n++
		}
	}
	return n
}

const optionsBlock = "type: options\noptions:\n  - {id: a, label: A}\n  - {id: b, label: B}\n"

func TestAnswer_DeliversTheFrameAndClaimsTheTicket(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := parkWithQuestion(t, ticketPath, optionsBlock)
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
		t.Errorf("park.answer lines = %d, want 1", n)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Status != schema.StatusClaimed {
		t.Errorf("ticket = %+v, %v; want claimed", tk, err)
	}
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
	if res := ask(server.QueueRequest{Choice: "a"}); res.Reason != server.ReasonNotParked {
		t.Errorf("claimed ticket: %+v, want %s", res, server.ReasonNotParked)
	}
	state := parkWithQuestion(t, ticketPath, optionsBlock)
	cases := []struct {
		name   string
		req    server.QueueRequest
		reason string
	}{
		{"stale", server.QueueRequest{State: "99:needs-answer", Choice: "a"}, server.ReasonStale},
		{"bad choice", server.QueueRequest{State: state, Choice: "zzz"}, server.ReasonBadChoice},
		{"empty", server.QueueRequest{State: state}, server.ReasonEmptyAnswer},
		{"too long", server.QueueRequest{State: state, Choice: "a", Note: strings.Repeat("x", 4001)}, server.ReasonTooLong},
	}
	for _, c := range cases {
		if res := ask(c.req); !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, want %s", c.name, res, c.reason)
		}
	}
	if res := ask(server.QueueRequest{State: "99:needs-answer", Choice: "a"}); res.Question == nil || res.State != state {
		t.Errorf("stale carries question %v state %q, want the current one (%q)", res.Question, res.State, state)
	}
	fake.SetState("epic-a-iter-01", agentrunner.StateBlocked, "permission")
	if res := ask(server.QueueRequest{State: state, Choice: "a"}); res.Reason != server.ReasonAgentNotReady {
		t.Errorf("blocked agent: %+v, want %s", res, server.ReasonAgentNotReady)
	}
	if n := answerCount(t, ticketPath); n != 0 {
		t.Errorf("refusals logged %d park.answer lines, want 0", n)
	}
	if got := fake.Prompts("epic-a-iter-01"); len(got) != 0 {
		t.Errorf("refusals prompted the agent: %q", got)
	}
}
