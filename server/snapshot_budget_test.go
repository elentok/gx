package server

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestBudgetStatus_SnapshotAndBudgetRouteAgree(t *testing.T) {
	t.Parallel()
	// A short path: the test name would push the unix socket path past its limit.
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(), Settings: Settings{BudgetSoftLimit: 300, BudgetHardLimit: 350}})
	if err != nil {
		t.Fatal(err)
	}
	s.budget.RecordSpend("a", 4.25)

	var snap wire.Snapshot
	rec := httptest.NewRecorder()
	s.snapshot(rec, httptest.NewRequest("GET", "/v1/snapshot", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	route, ok := findRoute(s.routes, "GET /v1/budget", wire.WebNone)
	if !ok {
		t.Fatal("no GET /v1/budget route")
	}
	var status wire.BudgetStatus
	rec = httptest.NewRecorder()
	route.Handler(rec, httptest.NewRequest("GET", "/v1/budget", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(snap.Budget, status) {
		t.Fatalf("snapshot budget %+v != budget route %+v", snap.Budget, status)
	}
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"total", status.Total, 4.25}, {"soft", status.SoftLimit, 300}, {"hard", status.HardLimit, 350},
	} {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Fatalf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}
