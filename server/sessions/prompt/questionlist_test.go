package prompt_test

import (
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f promptFixture) cardSources(t *testing.T) []string {
	t.Helper()
	chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range chat.Questions {
		out = append(out, c.Source)
	}
	return out
}

// A Fix session of a ticket with an open ask lists the ask and the printed
// round while idle, and each live blocker (permission, then plan) alone.
func TestQuestionList_FixSessionListsPermissionPlanRoundAndTicketAsk(t *testing.T) {
	t.Parallel()
	f := fromPrompt(servertest.StartPromptSessionAs(t, 10*time.Second, wire.CreateSessionRequest{FixOf: "proj:plain/02"}))
	servertest.WriteTicketWith(t, f.h.TicketStore, "proj", "plain", "02", "asking", servertest.TicketOpts{PauseReason: "self-reported"})
	ask := "## Question\n\nWhich option?\n\n```gx-question\n" + servertest.OptionsBlock + "```\n\n## Handoff\n\nHalf done.\n"
	if res, err := f.h.Client.Tickets.Ask(f.ctx, "proj:plain/02", ask); err != nil || res.Refused {
		t.Fatalf("ask = %+v, %v", res, err)
	}
	writeRounds(t, f.transcript, 1)
	idle := func() {
		report(t, f.conv, agentfake.Control{State: "idle"})
		waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	}
	for name, step := range map[string]struct {
		rows []string
		want []string
	}{
		"idle":       {nil, []string{wire.CardSourceTicketAsk, wire.CardSourceRound}},
		"permission": {agentfake.BashPermissionDialog("ls"), []string{wire.CardSourcePermission}},
		"plan":       {agentfake.PlanDialog(planFile(t, "fix-plan", "# Plan"), planOptions...), []string{wire.CardSourcePermission}},
	} {
		if step.rows != nil {
			f.blockOn(t, step.rows)
		}
		if got := f.cardSources(t); !slices.Equal(got, step.want) {
			t.Errorf("%s: sources = %v, want %v", name, got, step.want)
		}
		if step.rows != nil {
			idle()
		}
	}
}

// A live blocker is the only card while it is open; the printed round returns
// once it clears.
func TestQuestionList_LiveBlockerHidesThePrintedRoundUntilItClears(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	writeRounds(t, f.transcript, 1)
	f.blockOn(t, agentfake.BashPermissionDialog("ls"))
	if got := f.cardSources(t); !slices.Equal(got, []string{wire.CardSourcePermission}) {
		t.Fatalf("blocked: sources = %v, want only permission", got)
	}
	report(t, f.conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if got := f.cardSources(t); !slices.Equal(got, []string{wire.CardSourceRound}) {
		t.Errorf("cleared: sources = %v, want the round", got)
	}
}
