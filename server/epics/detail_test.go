package epics_test

import (
	"context"
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

func TestDetail_OpenSelfReportedQuestion(t *testing.T) {
	t.Parallel()
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != string(schema.StatusLegacyAnswer) || d.PauseReason != string(schema.PauseSelfReported) || d.Title == "" {
		t.Errorf("header = %+v", d)
	}
	if d.State != state {
		t.Errorf("state = %q, want %q", d.State, state)
	}
	if d.Latest == nil || d.Latest.Kind != ticketlog.QuestionOptions || len(d.Latest.Options) != 2 || d.Latest.Handoff != "h" || d.Latest.Outcome != wire.RoundOpen {
		t.Errorf("latest = %+v", d.Latest)
	}
	if want := []string{wire.ActionAnswer}; !slices.Equal(d.Actions, want) {
		t.Errorf("live agent actions = %v, want %v", d.Actions, want)
	}
	sess, ok, err := fake.Find("epic-a-iter-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}
	if err := fake.Stop(sess); err != nil {
		t.Fatal(err)
	}
	d, err = h.Client.Reads.Detail(context.Background(), answerAddr)
	if want := []string{wire.ActionAnswer, wire.ActionResume}; err != nil || !slices.Equal(d.Actions, want) {
		t.Errorf("gone agent actions = %v, %v; want %v", d.Actions, err, want)
	}
}

func TestDetail_ZeroCommitOffersRetryOnly(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
		tk.Status, tk.PauseKind = schema.StatusLegacyAnswer, schema.PauseKindZeroCommit
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ticketlog.AppendLog(ticketPath, ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, By: "gx", Summary: "no commits"}); err != nil {
		t.Fatal(err)
	}
	d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
	if want := []string{wire.ActionRetry}; err != nil || !slices.Equal(d.Actions, want) {
		t.Errorf("actions = %v, %v; want %v", d.Actions, err, want)
	}
	if got, want := d.ActionInfo[wire.ActionRetry], (wire.ActionInfo{Label: schema.VerbRetry.Label(), Tooltip: schema.VerbRetry.Tooltip()}); got != want {
		t.Errorf("retry info = %+v, want %+v", got, want)
	}
}

func TestDetail_ActionsFollowTheTable(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	if _, err := ticketlog.AppendLog(ticketPath, ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, By: "gx", Summary: "paused"}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		reason schema.PauseReason
		// tracked is what the run still being tracked leaves; want is the untracked list.
		tracked []string
		want    []string
	}{
		{schema.PauseIterationError, []string{}, []string{wire.ActionRetry, wire.ActionStartOver}},
		{schema.PauseMergeConflict, []string{wire.ActionMerge}, []string{wire.ActionMerge, wire.ActionStartOver}},
		{schema.PauseStrandedCommits, []string{wire.ActionReopenAndRetry}, []string{wire.ActionReopenAndRetry}},
		{schema.PauseAmbiguousBase, []string{wire.ActionChooseBase}, []string{wire.ActionChooseBase}},
		{schema.PauseBlockedPane, []string{wire.ActionRetry}, []string{wire.ActionRetry}},
		{schema.PauseBackgroundTaskGate, []string{wire.ActionRetry, wire.ActionStartOver}, []string{wire.ActionRetry, wire.ActionStartOver}},
		{schema.PauseManual, []string{wire.ActionResume, wire.ActionStartOver}, []string{wire.ActionResume, wire.ActionStartOver}},
	}
	check := func(label string, want func(int) []string, finishing func(int) bool) {
		for i, c := range cases {
			if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
				tk.Status, tk.PauseReason = schema.StatusPaused, c.reason
			}); err != nil {
				t.Fatal(err)
			}
			d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
			if err != nil || !slices.Equal(d.Actions, want(i)) {
				t.Errorf("%s %s: actions = %v, %v; want %v", label, c.reason, d.Actions, err, want(i))
			}
			if d.RunFinishing != finishing(i) {
				t.Errorf("%s %s: run_finishing = %v, want %v", label, c.reason, d.RunFinishing, finishing(i))
			}
			if c.reason == schema.PauseStrandedCommits && d.ActionInfo[wire.ActionReopenAndRetry].Label != "Reopen and retry" {
				t.Errorf("stranded-commits label = %q", d.ActionInfo[wire.ActionReopenAndRetry].Label)
			}
		}
	}
	check("tracked", func(i int) []string { return cases[i].tracked }, func(i int) bool {
		return len(cases[i].tracked) < len(cases[i].want)
	})
	h.Server.Registry().Delete(answerAddr)
	check("untracked", func(i int) []string { return cases[i].want }, func(int) bool { return false })
}

// A gate pause's live-task verbs follow a fresh read of the agent's task: Stop
// task needs a task gx can stop, which only a Claude run has.
func TestDetail_GatePauseVerbsFollowTheLiveTask(t *testing.T) {
	t.Parallel()
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	if _, err := ticketlog.AppendLog(ticketPath, ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, By: "gx", Summary: "task still running"}); err != nil {
		t.Fatal(err)
	}
	if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
		tk.Status, tk.PauseReason = schema.StatusPaused, schema.PauseBackgroundTaskGate
	}); err != nil {
		t.Fatal(err)
	}
	setAgent := func(agent string) {
		run, ok := h.Server.Registry().Tracked(answerAddr)
		if !ok {
			t.Fatal("run not tracked")
		}
		run.Agent = agent
		h.Server.Registry().Put(run)
	}
	cases := []struct {
		name  string
		agent string
		task  bool
		want  []string
	}{
		{"claude, live task", "claude", true, []string{wire.ActionKeepWaiting, wire.ActionStopTask, wire.ActionRetry, wire.ActionStartOver}},
		{"codex, live task", "codex", true, []string{wire.ActionKeepWaiting, wire.ActionRetry, wire.ActionStartOver}},
		{"claude, no live task", "claude", false, []string{wire.ActionRetry, wire.ActionStartOver}},
		{"codex, no live task", "codex", false, []string{wire.ActionRetry, wire.ActionStartOver}},
	}
	for _, c := range cases {
		setAgent(c.agent)
		fake.SetBackgroundTask("epic-a-iter-01", c.task)
		d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
		if err != nil || !slices.Equal(d.Actions, c.want) {
			t.Errorf("%s: actions = %v, %v; want %v", c.name, d.Actions, err, c.want)
		}
		if got := d.ActionInfo[wire.ActionKeepWaiting]; c.task && got.Label != "Keep waiting" {
			t.Errorf("%s: keep-waiting info = %+v", c.name, got)
		}
	}
}

func TestDetail_FinishedCommitlessOffersMergeOnly(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	cases := []struct {
		name       string
		status     schema.Status
		iter       schema.IterationStatus
		commitless bool
		want       []string
	}{
		{"finished commitless", schema.StatusClaimed, schema.IterationStatusFinished, true, []string{wire.ActionMerge}},
		{"finished with commits", schema.StatusClaimed, schema.IterationStatusFinished, false, []string{}},
		{"unfinished commitless", schema.StatusClaimed, schema.IterationStatusWorking, true, []string{}},
		{"done commitless", schema.StatusDone, schema.IterationStatusFinished, true, []string{}},
	}
	for _, c := range cases {
		if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.Status, tk.IterationStatus, tk.Commitless = c.status, c.iter, c.commitless
		}); err != nil {
			t.Fatal(err)
		}
		d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
		if err != nil || !slices.Equal(d.Actions, c.want) {
			t.Errorf("%s: actions = %v, %v; want %v", c.name, d.Actions, err, c.want)
		}
	}
}

func TestDetail_AnsweredAndClosedRounds(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseWithQuestion(t, ticketPath, optionsBlock)
	if _, err := ticketlog.AppendLog(ticketPath, ticketlog.LogEvent{Type: ticketlog.EventPauseAnswer, Question: 1, Source: "web", Choice: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) { tk.Status = schema.StatusOpen }); err != nil {
		t.Fatal(err)
	}
	d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
	if err != nil || !d.Answered || len(d.Actions) != 0 {
		t.Fatalf("answered = %+v, %v", d, err)
	}
	if d.Latest.Answer == nil || d.Latest.Answer.Choice != "b" || len(d.Chat) != 2 {
		t.Errorf("answer = %+v, chat = %+v", d.Latest.Answer, d.Chat)
	}

	// A second round: the first is now one line, and a closed round is read-only.
	if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) { tk.Status = schema.StatusLegacyAnswer }); err != nil {
		t.Fatal(err)
	}
	q, err := ticketlog.AppendLog(ticketPath, ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, By: "agent", Summary: "again", Handoff: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ticketlog.AppendLog(ticketPath, ticketlog.LogEvent{Type: ticketlog.EventPauseClosed, Question: q.Seq, Reason: "resumed"}); err != nil {
		t.Fatal(err)
	}
	d, err = h.Client.Reads.Detail(context.Background(), answerAddr)
	if err != nil || len(d.Rounds) != 1 || d.Rounds[0].Outcome != wire.RoundAnswered || d.Rounds[0].Options != nil {
		t.Fatalf("earlier rounds = %+v, %v", d.Rounds, err)
	}
	if d.Latest.Outcome != wire.RoundClosed || d.Latest.ClosedReason != "resumed" || slices.Contains(d.Actions, wire.ActionAnswer) {
		t.Errorf("closed round = %+v, actions = %v", d.Latest, d.Actions)
	}
}

func TestDetail_StuckHasNoAnswerAction(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseWithQuestion(t, ticketPath, optionsBlock)
	if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) { tk.Status = schema.StatusLegacyRepair }); err != nil {
		t.Fatal(err)
	}
	d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
	if err != nil || slices.Contains(d.Actions, wire.ActionAnswer) {
		t.Errorf("stuck actions = %v, %v", d.Actions, err)
	}
}

func TestDetail_BadBlockReadsAsText(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseWithQuestion(t, ticketPath, "type: nonsense\n")
	d, err := h.Client.Reads.Detail(context.Background(), answerAddr)
	if err != nil || d.Latest == nil || d.Latest.Kind != ticketlog.QuestionText || len(d.Latest.Options) != 0 {
		t.Errorf("latest = %+v, %v; want a text question", d.Latest, err)
	}
}

func TestDetail_UnknownAddressIs404(t *testing.T) {
	t.Parallel()
	h, _, _ := startWaitingRestartFake(t, 0)
	if _, err := h.Client.Reads.Detail(context.Background(), "proj:epic-a/99"); err == nil {
		t.Error("unknown ticket returned no error")
	}
}
