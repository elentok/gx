package queue_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/servertest"
)

func TestQueueAgentRows_PtyRunListsAWorkingRowWithoutUnreadOrActionable(t *testing.T) {
	t.Parallel()
	h, fake := servertest.StartQueueRows(t, "pty")
	fake.SetState(servertest.ResumeLabel, agentrunner.StateWorking, "")

	servertest.WaitFor(t, 10*time.Second, "a working row", func() bool {
		rows := servertest.QueueRows(t, h)
		return len(rows) == 1 && rows[0].Status.State == wire.SessionWorking
	})
	row := servertest.QueueRows(t, h)[0]
	if row.Ticket != servertest.QueueRowTicket || row.Holder == "" || row.Unread != (wire.SessionUnread{}) || row.Actionable.Reason != "" {
		t.Errorf("row = %+v, want the ticket, a holder dir and no Unread or Actionable", row)
	}
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil || len(snap.Sessions) == 0 {
		t.Errorf("snapshot sessions = %+v, %v; want the Queue row too", snap.Sessions, err)
	}
}

func TestQueueAgentRows_IterationsNameTheRunnerAndTheRow(t *testing.T) {
	t.Parallel()
	h, fake := servertest.StartQueueRows(t, "pty")
	fake.SetState(servertest.ResumeLabel, agentrunner.StateWorking, "")

	servertest.WaitFor(t, 10*time.Second, "a live iteration", func() bool {
		its, err := h.Client.Reads.Iterations(context.Background())
		return err == nil && len(its) == 1
	})
	its, _ := h.Client.Reads.Iterations(context.Background())
	rows := servertest.QueueRows(t, h)
	if its[0].Runner != "pty" || len(rows) != 1 || its[0].QueueAgent != rows[0].ID {
		t.Errorf("iteration = %+v, Queue rows = %+v; want runner pty and the row's id", its[0], rows)
	}
}

func TestQueueAgentRows_HeldRunShowsRateLimited(t *testing.T) {
	t.Parallel()
	h, fake := servertest.StartQueueRows(t, "pty")
	fake.SetLimitedUnknownReset(servertest.ResumeLabel)
	fake.SetState(servertest.ResumeLabel, agentrunner.StateDone, "")

	servertest.WaitFor(t, 10*time.Second, "a rate-limited row", func() bool {
		rows := servertest.QueueRows(t, h)
		return len(rows) == 1 && rows[0].Status.State == wire.SessionRateLimited
	})
}

func TestQueueAgentRows_BlockedAgentKeepsItsRow(t *testing.T) {
	t.Parallel()
	h, fake := servertest.StartQueueRows(t, "pty")
	fake.SetState(servertest.ResumeLabel, agentrunner.StateBlocked, "permission")

	servertest.WaitFor(t, 10*time.Second, "a blocked row", func() bool {
		rows := servertest.QueueRows(t, h)
		return len(rows) == 1 && rows[0].Status.State == wire.SessionBlocked
	})
}

func TestQueueAgentRows_RowLeavesWhenTheRunFinishes(t *testing.T) {
	t.Parallel()
	h, fake := servertest.StartQueueRows(t, "pty")
	servertest.WaitFor(t, 10*time.Second, "a row", func() bool { return len(servertest.QueueRows(t, h)) == 1 })

	fake.SetState(servertest.ResumeLabel, agentrunner.StateDone, "")
	servertest.WaitFor(t, 20*time.Second, "the row to leave", func() bool { return len(servertest.QueueRows(t, h)) == 0 })
}

func TestQueueAgentChat_LivePtyAgentServesItsChatAndScreen(t *testing.T) {
	t.Parallel()
	h, row, release := servertest.StartQueueChat(t, "queue agent says hi")
	defer release()
	ctx := context.Background()

	servertest.WaitFor(t, 10*time.Second, "the agent's message in the chat", func() bool {
		chat, err := h.Client.Sessions.Chat(ctx, row.ID)
		return err == nil && slices.ContainsFunc(chat.Messages, func(m wire.SessionChatMessage) bool {
			return strings.Contains(m.Text, "queue agent says hi")
		})
	})
	screen, err := h.Client.Sessions.Screen(ctx, row.ID)
	if err != nil || len(screen.Rows) == 0 {
		t.Errorf("screen = %+v, %v; want the agent's rows", screen, err)
	}
}

func TestQueueAgentChat_GoneAgentReadsAsUnknownSession(t *testing.T) {
	t.Parallel()
	h, row, release := servertest.StartQueueChat(t, "bye")
	release()
	servertest.WaitFor(t, 20*time.Second, "the row to leave", func() bool { return len(servertest.QueueRows(t, h)) == 0 })
	ctx := context.Background()

	if _, err := h.Client.Sessions.Chat(ctx, row.ID); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("chat err = %v, want a 404", err)
	}
	if _, err := h.Client.Sessions.Screen(ctx, row.ID); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("screen err = %v, want a 404", err)
	}
}

func TestQueueAgentChat_WriteRoutesRefuseAQueueAgentAsUnknown(t *testing.T) {
	t.Parallel()
	h, row, release := servertest.StartQueueChat(t, "hello")
	defer release()
	ctx := context.Background()

	for name, call := range map[string]func() (wire.SessionResult, error){
		"stop":      func() (wire.SessionResult, error) { return h.Client.Sessions.Stop(ctx, row.ID) },
		"interrupt": func() (wire.SessionResult, error) { return h.Client.Sessions.Interrupt(ctx, row.ID) },
		"remove":    func() (wire.SessionResult, error) { return h.Client.Sessions.Remove(ctx, row.ID) },
		"prompt": func() (wire.SessionResult, error) {
			return h.Client.Sessions.Prompt(ctx, wire.SessionPromptRequest{ID: row.ID, Text: "hi"})
		},
	} {
		res, err := call()
		if err != nil || !res.Refused || res.Reason != wire.ReasonSessionUnknown {
			t.Errorf("%s = %+v, %v; want a refusal as unknown session", name, res, err)
		}
	}
}

func TestQueueAgentRows_HerdrAndHeadlessRunsAreAbsent(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"herdr", "headless"} {
		h, _ := servertest.StartQueueRows(t, runner)
		if rows := servertest.QueueRows(t, h); len(rows) != 0 {
			t.Errorf("%s rows = %+v, want none", runner, rows)
		}
	}
}
