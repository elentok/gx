package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestRepair_RefusalsCarryTheRepairPackagesCodes(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	const addr = "proj:epic-a/01"

	cases := []struct {
		name   string
		verb   string
		req    server.RepairRequest
		reason string
	}{
		{"land from a ralph-loop branch", "land", server.RepairRequest{Address: addr, Branch: "ralph-loop/epic-a"}, repair.ReasonRalphLoopCwd},
		{"land continue and abort", "land", server.RepairRequest{Address: addr, Continue: true, Abort: true}, repair.ReasonError},
		{"land an open ticket", "land", server.RepairRequest{Address: addr, Cwd: t.TempDir()}, repair.ReasonStatusRefused},
		{"reset without a reason", "reset", server.RepairRequest{Address: addr}, repair.ReasonReasonRequired},
		{"unpark an open ticket", "unpark", server.RepairRequest{Address: addr}, repair.ReasonNotParked},
		{"bad address", "unpark", server.RepairRequest{Address: "nonsense"}, server.ReasonInvalidAddress},
		{"unknown project", "reset", server.RepairRequest{Address: "nope:epic-a/01", Reason: "x"}, server.ReasonUnknownTicket},
	}
	for _, c := range cases {
		res, err := h.Client.Repair(ctx, c.verb, c.req)
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
}

func TestRepair_RalphLoopGuardIsServerSide(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Repair(context.Background(), "land", server.RepairRequest{Address: "proj:epic-a/01", Branch: "ralph-loop/other"})
	if err != nil || !res.Refused || res.Reason != repair.ReasonRalphLoopCwd {
		t.Errorf("land = %+v, %v; want %s", res, err, repair.ReasonRalphLoopCwd)
	}
}
