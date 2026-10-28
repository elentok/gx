package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestServerChecksSave_JSONPrintsTheRouteResult(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	if err := servertest.WriteEpic(h.TicketStore, "demo", ""); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	req := server.SaveChecksRequest{Project: "demo", Target: server.SaveTargetProjectJSON, Checks: json.RawMessage(`{"test":false}`)}
	if err := runServerChecksSave(context.Background(), h.Client, &out, req, true); err != nil {
		t.Fatal(err)
	}
	var got server.SaveChecksResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Refused || got.Path == "" || got.Target != server.SaveTargetProjectJSON {
		t.Errorf("--json = %s", out.String())
	}
}

func TestServerChecks_JSONMatchesTheRoute(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	ctx := context.Background()

	want, err := h.Client.ChecksStatus(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runServerChecks(ctx, h.Client, &out, "", true); err != nil {
		t.Fatal(err)
	}
	var got server.ChecksStatus
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(want)
	b, _ := json.Marshal(got)
	if len(got.Projects) == 0 || !bytes.Equal(a, b) {
		t.Errorf("--json = %s, route = %s", b, a)
	}
}
