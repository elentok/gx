package server_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets/schema"
)

func TestRetry_ZeroCommitWithALiveAgentPromptsAndKeepsThePause(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	pauseEnvZeroCommit(t, e)

	res, err := e.h.Client.TicketRetry(t.Context(), answerAddr, false)
	if err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	if got := e.fake.Prompts(answerLabel); len(got) != 1 {
		t.Errorf("prompts = %q; want one", got)
	}
	if tk, err := schema.ParseTicket(e.ticket); err != nil || tk.PausedClass() != schema.PauseClassAnswer {
		t.Errorf("ticket = %+v, %v; want still asking", tk, err)
	}
}

func TestRetry_BlockedPaneWithALiveAgentPrompts(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	if err := e.h.Server.PauseAs(answerAddr, events.BlockedPane, "stuck"); err != nil {
		t.Fatal(err)
	}

	if res, err := e.h.Client.TicketRetry(t.Context(), answerAddr, false); err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	if got := e.fake.Prompts(answerLabel); len(got) != 1 {
		t.Errorf("prompts = %q; want one", got)
	}
}

func TestRetry_BudgetKilledDoesNotPromptTheAgent(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	if err := e.h.Server.PauseAs(answerAddr, events.BudgetKilled, "over budget"); err != nil {
		t.Fatal(err)
	}

	// The fixture repo has no trunk branch, so the relaunch itself errors: the
	// error shows the relaunch path ran.
	_, err := e.h.Client.TicketRetry(t.Context(), answerAddr, false)
	if err == nil || !strings.Contains(err.Error(), "trunk") {
		t.Errorf("retry error = %v; want the relaunch's trunk error", err)
	}
	if got := e.fake.Prompts(answerLabel); len(got) != 0 {
		t.Errorf("prompts = %q; want none (relaunch path)", got)
	}
}
