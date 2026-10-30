package server_test

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
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

// Retry on a blocked pause with a live PTY or herdr agent ends the pause once,
// interrupts the agent and sends one retry prompt; the unblock watch seeing the
// agent leave blocked afterwards adds neither a second end nor a second prompt.
func TestRetry_BlockedPauseOnAQuestionMenuInterruptsAndPrompts(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"pty", "herdr"} {
		t.Run(runner, func(t *testing.T) {
			h, ticketPath, turn := startBlockable(t, runner)
			turn.SetState("epic-a-iter-01", agentrunner.StateBlocked, "waiting on a prompt")
			waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
			waitFor(t, func() bool { return runsJSONHas(h, `"waiting_since"`) })
			before := len(turn.Prompts("epic-a-iter-01"))

			if res, err := h.Client.TicketRetry(t.Context(), answerAddr, false); err != nil || res.Refused {
				t.Fatalf("retry = %+v, %v", res, err)
			}
			waitStatus(t, ticketPath, schema.StatusClaimed)
			waitFor(t, func() bool { return !runsJSONHas(h, `"waiting_since"`) })
			// Give the watch time to run on the interrupted agent before counting.
			time.Sleep(300 * time.Millisecond)
			if got := turn.Interrupts("epic-a-iter-01"); got != 1 {
				t.Errorf("interrupts = %d; want 1", got)
			}
			if got := len(turn.Prompts("epic-a-iter-01")) - before; got != 1 {
				t.Errorf("retry prompts = %d; want 1", got)
			}
		})
	}
}

// A headless blocked pause keeps refusing Retry's plain resume.
func TestRetry_HeadlessBlockedPauseStillRefuses(t *testing.T) {
	t.Parallel()
	h, _, turn := pauseBlocked(t)
	res, err := h.Client.TicketRetry(t.Context(), answerAddr, false)
	if err != nil || !res.Refused || res.Reason != server.ReasonAgentWaiting {
		t.Fatalf("retry = %+v, %v; want %s", res, err, server.ReasonAgentWaiting)
	}
	if got := turn.Interrupts("epic-a-iter-01"); got != 0 {
		t.Errorf("interrupts = %d; want 0", got)
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
