package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func resumeEvents(t *testing.T, ticketPath string) (closed []tickets.LogEvent, answers int) {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		switch ev.Type {
		case tickets.EventPauseClosed:
			closed = append(closed, ev)
		case tickets.EventPauseAnswer:
			answers++
		}
	}
	return closed, answers
}

func TestResume_LiveWaitingAgentRefusesAndIsUntouched(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseWithQuestion(t, ticketPath, optionsBlock)

	res, err := h.Client.Repair(context.Background(), "resume", server.RepairRequest{Address: answerAddr})
	if err != nil || !res.Refused || res.Reason != server.ReasonAgentWaiting {
		t.Fatalf("resume = %+v, %v; want refusal %s", res, err, server.ReasonAgentWaiting)
	}
	if got := fake.Prompts("epic-a-iter-01"); len(got) != 0 {
		t.Errorf("resume prompted the agent: %q", got)
	}
	if _, ok, err := fake.Find("epic-a-iter-01"); err != nil || !ok {
		t.Errorf("session alive = %v, %v; want true", ok, err)
	}
	if closed, answers := resumeEvents(t, ticketPath); len(closed) != 0 || answers != 0 {
		t.Errorf("refusal logged closed=%+v answers=%d", closed, answers)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.PausedClass() != schema.PauseClassAnswer {
		t.Errorf("ticket = %+v, %v; want still asking", tk, err)
	}
}

func TestResume_GoneAgentClosesTheQuestionAndReopens(t *testing.T) {
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseWithQuestion(t, ticketPath, optionsBlock)
	sess, ok, err := fake.Find("epic-a-iter-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	if err := fake.Stop(sess); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.Repair(context.Background(), "resume", server.RepairRequest{Address: answerAddr})
	if err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v; want reopened", res, err)
	}
	closed, answers := resumeEvents(t, ticketPath)
	if len(closed) != 1 || closed[0].Reason != ralphloop.CloseResumed || answers != 0 {
		t.Errorf("closed=%+v answers=%d; want one pause.closed(%s) and no pause.answer", closed, answers, ralphloop.CloseResumed)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
}
