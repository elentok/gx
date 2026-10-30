package server_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// liveAgentFixture runs ticket 01 on the PTY runner with an agent that keeps
// working until the test ends, so the ticket stays claimed with a live session.
func liveAgentFixture(t *testing.T) (*schedFixture, context.Context) {
	t.Helper()
	started := make(chan struct{}, 1)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, _ string) {
		p.Stall()
		select {
		case started <- struct{}{}:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	f.queueEpic(ctx, t, "01")
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("the agent never took its turn")
	}
	return f, ctx
}

func TestRepairMerge_LiveAgentOnPTYRefusesUnlessIgnored(t *testing.T) {
	t.Parallel()
	f, ctx := liveAgentFixture(t)
	req := server.RepairRequest{Address: "proj:" + schedEpic + "/01"}

	res, err := f.h.Client.Repair(ctx, "merge", req)
	if err != nil || !res.Refused || res.Reason != repair.ReasonLiveAgentOnTab {
		t.Fatalf("merge = %+v, %v; want %s refusal", res, err, repair.ReasonLiveAgentOnTab)
	}

	req.IgnoreLiveTab = true
	res, err = f.h.Client.Repair(ctx, "merge", req)
	if err != nil || res.Reason == repair.ReasonLiveAgentOnTab {
		t.Errorf("merge --ignore-live-tab = %+v, %v; want the live-agent guard skipped", res, err)
	}
}

func TestRepairVerify_ReportsAPTYLeftoverSession(t *testing.T) {
	t.Parallel()
	f, ctx := liveAgentFixture(t)

	res, err := f.h.Client.Repair(ctx, "verify", server.RepairRequest{Address: "proj:" + schedEpic + "/01"})
	if err != nil || res.Refused {
		t.Fatalf("verify = %+v, %v", res, err)
	}
	var out repair.VerifyResult
	if err := json.Unmarshal(res.Data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tickets) != 1 || out.Tickets[0].Leftovers.Tab == nil || !*out.Tickets[0].Leftovers.Tab {
		t.Errorf("verify tickets = %+v; want the PTY session reported as a leftover", out.Tickets)
	}
}
