package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
)

func endedRounds(t *testing.T, ticketPath string) []tickets.PauseRound {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	return tickets.AllRounds(evs)
}

func TestResume_ZeroCommitEndsTheRoundWithTheClientsSource(t *testing.T) {
	for name, origin := range map[string]string{"cli": server.OriginCLI, "tui": server.OriginTUI} {
		t.Run(name, func(t *testing.T) {
			e := startAnswerEnv(t, nil)
			pauseEnvZeroCommit(t, e)
			if err := e.fake.Stop(mustFind(t, e.fake)); err != nil {
				t.Fatal(err)
			}

			res, err := e.h.Server.ResumeAs(server.RepairRequest{Address: answerAddr}, origin)
			if err != nil {
				t.Fatal(err)
			}
			resumeResult(t, res)
			rounds := endedRounds(t, e.ticket)
			if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowResume || rounds[0].End.Source != origin {
				t.Errorf("rounds = %+v; want one ended by %s resume", rounds, origin)
			}
		})
	}
}

func TestResume_ZeroCommitKeptByALiveAgentWritesNoEnd(t *testing.T) {
	e := startAnswerEnv(t, nil)
	pauseEnvZeroCommit(t, e)

	res, err := e.h.Client.Repair(context.Background(), "resume", server.RepairRequest{Address: answerAddr})
	if err != nil {
		t.Fatal(err)
	}
	if out := resumeResult(t, res); out.Status != repair.ResumePrompted {
		t.Fatalf("result = %+v; want the live agent prompted", out)
	}
	rounds := endedRounds(t, e.ticket)
	if len(rounds) != 1 || rounds[0].End != nil {
		t.Errorf("rounds = %+v; want the round still open", rounds)
	}
}

func TestUnmute_EndsTheRepairPausesRound(t *testing.T) {
	h, path := startTicketWrites(t)
	writeMuted(t, path, "open", "Body text.\n")
	if err := h.Server.PauseAs("proj:epic-a/01", events.MergeConflict, "stuck"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("unmute = %+v, %v", res, err)
	}
	rounds := endedRounds(t, path)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowResume || rounds[0].End.Source != server.OriginCLI {
		t.Errorf("rounds = %+v; want one ended by cli resume", rounds)
	}
}
