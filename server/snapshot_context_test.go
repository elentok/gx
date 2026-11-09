package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestSnapshot_CarriesTheSmartZoneCeiling(t *testing.T) {
	t.Parallel()
	// A short path: the test name would push the unix socket path past its limit.
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rootRoutes{s}.snapshot(rec, httptest.NewRequest("GET", "/v1/snapshot", nil))
	var snap wire.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.ContextCeiling != agent.SmartZoneCeiling {
		t.Errorf("context_ceiling = %d, want %d", snap.ContextCeiling, agent.SmartZoneCeiling)
	}
}
