package server_test

import (
	"net/http"
	"testing"

	"github.com/elentok/gx/server"
)

func TestSessionCreate_TierDefaultsToWorker(t *testing.T) {
	t.Parallel()
	h, _, _, id, _ := startFakeSession(t)
	if tier := readMeta(t, h, id)["tier"]; tier != "worker" {
		t.Errorf("meta tier = %v, want worker", tier)
	}
}

func TestSessionCreate_TierIsSavedAndResumeKeepsIt(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, _ := startFakeSessionWith(t, server.CreateSessionRequest{Tier: "strongest"})
	if tier := readMeta(t, h, id)["tier"]; tier != "strongest" {
		t.Fatalf("meta tier = %v, want strongest", tier)
	}
	stopFakeSession(ctx, t, h, evs, id)
	if r, err := h.Client.ResumeSession(ctx, id); err != nil || r.Refused {
		t.Fatalf("resume = %+v, %v", r, err)
	}
	if tier := readMeta(t, h, id)["tier"]; tier != "strongest" {
		t.Errorf("meta tier after resume = %v, want strongest", tier)
	}
}

func TestSessionCreate_UnknownTierIsRefused(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Tier: "huge"})
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionBadRequest {
		t.Errorf("create = %+v, %v; want a bad-request refusal", res, err)
	}
}

func TestWebSessionCreate_TierIsSaved(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	code, res := f.webCreate(t, `{"project":"proj","dir":"`+f.projectRoot(t)+`","tier":"light"}`)
	if code != http.StatusOK || res.Refused || res.Session == nil {
		t.Fatalf("create = %d %+v", code, res)
	}
	if tier := readMeta(t, f.h, res.Session.ID)["tier"]; tier != "light" {
		t.Errorf("meta tier = %v, want light", tier)
	}
}
