package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestCreateSession_StartsHolderAndListsSession(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{
		Project: "proj", Dir: dir, Name: "spike", Ticket: "proj:epic/01",
		Harness: string(agentfake.HarnessKind), Cols: 90, Rows: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Refused || res.Session == nil || res.Session.ID == "" {
		t.Fatalf("create = %+v, want a session", res)
	}
	id := res.Session.ID
	servertest.WaitForEvent(ctx, t, evs, server.EventSessionChanged, id)

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 1 {
		t.Fatalf("snapshot sessions = %+v, want one", snap.Sessions)
	}
	got := snap.Sessions[0]
	want := server.SessionInfo{
		ID: id, Name: "spike", Project: "proj", Dir: dir, Harness: string(agentfake.HarnessKind),
		Ticket: "proj:epic/01", Status: server.SessionStatus{State: server.SessionStarting},
	}
	got.StatusChangedAt = time.Time{}
	if got.ID != want.ID || got.Name != want.Name || got.Project != want.Project || got.Dir != want.Dir ||
		got.Harness != want.Harness || got.Ticket != want.Ticket || got.Status != want.Status {
		t.Errorf("snapshot session = %+v, want %+v", got, want)
	}
	list, err := h.Client.Sessions(ctx)
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Errorf("sessions = %+v, %v; want %s", list, err, id)
	}

	sessionDir := filepath.Join(h.SessionsDir, id)
	data, err := os.ReadFile(filepath.Join(sessionDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]any{"project": "proj", "dir": dir, "name": "spike", "ticket": "proj:epic/01", "harness": string(agentfake.HarnessKind), "cols": 90.0, "rows": 30.0} {
		if meta[k] != v {
			t.Errorf("meta[%s] = %v, want %v", k, meta[k], v)
		}
	}
	if meta["conversation_id"] == "" || meta["conversation_id"] == nil {
		t.Error("meta has no conversation id")
	}

	if c := waitForHolder(t, sessionDir); c.ChildPID() == 0 {
		t.Error("holder reports no child")
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want none", runs)
	}
	if _, err := os.Stat(filepath.Join(h.StateDir, "agents")); !os.IsNotExist(err) {
		t.Errorf("loop agents root exists (%v); a session must not use it", err)
	}
}

func TestCreateSession_RefusesBadRequests(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	for name, tc := range map[string]struct {
		req    server.CreateSessionRequest
		reason string
	}{
		"no project":      {server.CreateSessionRequest{Dir: t.TempDir()}, server.ReasonSessionBadRequest},
		"missing dir":     {server.CreateSessionRequest{Project: "p", Dir: filepath.Join(t.TempDir(), "gone")}, server.ReasonSessionBadRequest},
		"unknown harness": {server.CreateSessionRequest{Project: "p", Dir: t.TempDir(), Harness: "nope"}, server.ReasonSessionUnknownHarness},
	} {
		res, err := h.Client.CreateSession(ctx, tc.req)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s: create = %+v, want refused %s", name, res, tc.reason)
		}
	}
	if list, _ := h.Client.Sessions(ctx); len(list) != 0 {
		t.Errorf("sessions = %+v, want none", list)
	}
}

func waitForHolder(t *testing.T, dir string) *nativerunner.HolderClient {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		c, err := nativerunner.DialHolder(dir)
		if err == nil {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("no holder in %s: %v", dir, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
