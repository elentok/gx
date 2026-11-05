package ticketops_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func TestResume_ZeroCommitEndsTheRoundWithTheClientsSource(t *testing.T) {
	t.Parallel()
	for name, origin := range map[string]string{"cli": wire.OriginCLI, "tui": wire.OriginTUI} {
		t.Run(name, func(t *testing.T) {
			e := startAnswerEnv(t, nil)
			pauseEnvZeroCommit(t, e)
			if err := e.Fake.Stop(mustFind(t, e.Fake)); err != nil {
				t.Fatal(err)
			}

			res, err := e.H.Server.TicketOps().ResumeAs(wire.RepairRequest{Address: answerAddr}, origin)
			if err != nil {
				t.Fatal(err)
			}
			resumeResult(t, res)
			rounds := endedRounds(t, e.Ticket)
			if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowResume || rounds[0].End.Source != origin {
				t.Errorf("rounds = %+v; want one ended by %s resume", rounds, origin)
			}
		})
	}
}

func TestResume_ZeroCommitPromptedLiveAgentKeepsTheRoundOpen(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	pauseEnvZeroCommit(t, e)

	res, err := e.H.Client.Repair(context.Background(), "resume", wire.RepairRequest{Address: answerAddr})
	if err != nil {
		t.Fatal(err)
	}
	if out := resumeResult(t, res); out.Status != repair.ResumePrompted {
		t.Fatalf("result = %+v; want the live agent prompted", out)
	}
	rounds := endedRounds(t, e.Ticket)
	if len(rounds) != 1 || rounds[0].End != nil {
		t.Errorf("rounds = %+v; want the round still open after the prompt", rounds)
	}
}
