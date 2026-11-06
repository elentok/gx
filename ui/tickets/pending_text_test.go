package tickets

import (
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestPendingText_CapVerdictReadsWaitingForASlot(t *testing.T) {
	cases := []struct {
		name string
		row  wire.PendingRow
		want string
	}{
		{"cap with reason", wire.PendingRow{Address: "gx:map/01", Verdict: wire.VerdictConcurrencyCap, Reason: "global cap 1/1"}, "Waiting for a slot: global cap 1/1"},
		{"cap without reason", wire.PendingRow{Address: "gx:map/01", Verdict: wire.VerdictConcurrencyCap}, "Waiting for a slot"},
		{"other verdict unchanged", wire.PendingRow{Address: "gx:map/01", Verdict: "blocked", Reason: "by 02"}, "blocked: by 02"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newServerModel(t)
			m = withState(m, wire.Snapshot{
				Tickets: []wire.TicketInfo{{Address: "gx:map/01", Title: "Open", Status: "open", EpicIsMap: true}},
				Pending: []wire.PendingRow{c.row},
			})
			got, ok := sidebarEnvFor(m).PendingText("gx:map/01")
			if !ok || got != c.want {
				t.Errorf("PendingText = %q, %v, want %q", got, ok, c.want)
			}
		})
	}
}
