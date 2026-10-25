package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestAgentsAutoAdvance_JSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	created, err := h.Client.CreateSession(context.Background(), server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	var out bytes.Buffer
	if err := runAgentsAutoAdvance(context.Background(), h.Client, &out, true, created.Session.ID, false); err != nil {
		t.Fatal(err)
	}
	var res server.SessionResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if !res.Refused || res.Reason != server.ReasonSessionNotDesign {
		t.Errorf("auto-advance off = %+v, want %s", res, server.ReasonSessionNotDesign)
	}
}
