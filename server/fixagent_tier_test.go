package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func TestFixAgent_RunsOnStrongestAndResumeKeepsIt(t *testing.T) {
	t.Parallel()
	h, addr, _ := startRemoteFix(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.FixAgent(ctx, server.FixAgentRequest{Address: addr})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("fix = %+v, %v", res, err)
	}
	id := res.Session.ID
	want := map[string]any{"tier": "strongest", "model": "opus", "effort": "high"}
	check := func(when string) {
		t.Helper()
		meta := readMeta(t, h, id)
		for k, v := range want {
			if meta[k] != v {
				t.Errorf("meta %s %s = %v, want %v", when, k, meta[k], v)
			}
		}
	}
	check("after start")
	stopFakeSession(ctx, t, h, evs, id)
	if r, err := h.Client.ResumeSession(ctx, id); err != nil || r.Refused {
		t.Fatalf("resume = %+v, %v", r, err)
	}
	check("after resume")
}
