package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func recommendAs(t *testing.T, h *servertest.Harness, investigator, address, verb string) server.QueueResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := apiclient.New(server.SocketPath(h.StateDir)).AsInvestigator(investigator).TicketRecommend(ctx, address, verb, "the branch is fine, the agent just crashed")
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestRecommend_StoredRecommendationShowsInDetail(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	if res := recommendAs(t, h, investigateAddr, "proj:epic-a/01", server.ActionRetry); res.Refused {
		t.Fatalf("recommend refused: %+v", res)
	}
	d, err := h.Client.Detail(t.Context(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if d.Recommendation == nil || d.Recommendation.Verb != server.ActionRetry || d.Recommendation.Diagnosis != "the branch is fine, the agent just crashed" {
		t.Errorf("recommendation = %+v, want retry with the diagnosis", d.Recommendation)
	}
}

func TestRecommend_VerbOutsideActionsIsRefused(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	if res := recommendAs(t, h, investigateAddr, "proj:epic-a/01", server.ActionApprove); !res.Refused || res.Reason != server.ReasonVerbNotOffered {
		t.Errorf("recommend = %+v, want %s refusal", res, server.ReasonVerbNotOffered)
	}
	d, err := h.Client.Detail(t.Context(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if d.Recommendation != nil {
		t.Errorf("recommendation = %+v, want none", d.Recommendation)
	}
}

func TestRecommend_NewPauseRoundClearsIt(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	if res := recommendAs(t, h, investigateAddr, "proj:epic-a/01", server.ActionRetry); res.Refused {
		t.Fatalf("recommend refused: %+v", res)
	}
	if err := h.Server.PauseAs("proj:epic-a/01", events.ZeroCommit, "again"); err != nil {
		t.Fatal(err)
	}
	d, err := h.Client.Detail(t.Context(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if d.Recommendation != nil {
		t.Errorf("recommendation = %+v, want none after a new round", d.Recommendation)
	}
}

func TestRecommend_DeadlockInvestigationWritesNone(t *testing.T) {
	t.Parallel()
	h := startDeadlock(t, recovery.Catalog{Enabled: true})
	h.Server.RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllPaused, Reason: "every remaining ticket is paused"})
	waitFor(t, func() bool { return len(runLogOf(t, h, events.RecoveryApplied)) > 0 })
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom"); err != nil {
		t.Fatal(err)
	}
	if res := recommendAs(t, h, "proj:epic-a/03", "proj:epic-a/01", server.ActionRetry); !res.Refused || res.Reason != server.ReasonInvestigateStop {
		t.Errorf("recommend = %+v, want %s refusal", res, server.ReasonInvestigateStop)
	}
	d, err := h.Client.Detail(t.Context(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if d.Recommendation != nil {
		t.Errorf("recommendation = %+v, want none", d.Recommendation)
	}
}
