package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestBadgeCount_ActionableSessionsPlusPausedTickets(t *testing.T) {
	t.Parallel()
	withReason := wire.SessionActionable{Reason: "needs you"}
	blocked := wire.SessionStatus{State: wire.SessionBlocked}
	for _, c := range []struct {
		name     string
		sessions []wire.SessionInfo
		tickets  []wire.TicketInfo
		want     int
	}{
		{"empty", nil, nil, 0},
		{"actionable reason", []wire.SessionInfo{{Actionable: withReason}}, nil, 1},
		{"blocked without reason", []wire.SessionInfo{{Status: blocked}}, nil, 1},
		{"idle session", []wire.SessionInfo{{}}, nil, 0},
		{"queue excluded", []wire.SessionInfo{{Queue: true, Actionable: withReason}}, nil, 0},
		{"design excluded", []wire.SessionInfo{{Design: true, Actionable: withReason}}, nil, 0},
		{"triage excluded", []wire.SessionInfo{{Flow: string(tickets.FlowTriage), Actionable: withReason}}, nil, 0},
		{"paused ticket", nil, []wire.TicketInfo{{Status: string(schema.StatusPaused)}}, 1},
		{"legacy paused", nil, []wire.TicketInfo{
			{Status: string(schema.StatusLegacyAnswer)}, {Status: string(schema.StatusLegacyRepair)},
		}, 2},
		{"map epic ticket excluded", nil, []wire.TicketInfo{{Status: string(schema.StatusPaused), EpicIsMap: true}}, 0},
		{"draft and error excluded", nil, []wire.TicketInfo{{Status: string(schema.StatusDraft)}, {Status: "error"}}, 0},
		{"sum", []wire.SessionInfo{{Actionable: withReason}, {Status: blocked}},
			[]wire.TicketInfo{{Status: string(schema.StatusPaused)}}, 3},
	} {
		if got := badgeCount(c.sessions, c.tickets); got != c.want {
			t.Errorf("%s: badgeCount = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBadge_SnapshotEqualsServerBadgeCount(t *testing.T) {
	t.Parallel()
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var snap wire.Snapshot
	rec := httptest.NewRecorder()
	rootRoutes{s}.snapshot(rec, httptest.NewRequest("GET", "/v1/snapshot", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Badge != s.BadgeCount() {
		t.Fatalf("snapshot badge %d != BadgeCount %d", snap.Badge, s.BadgeCount())
	}
}
