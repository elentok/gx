package server_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

func TestSessionStatus_FollowsHolderWatch(t *testing.T) {
	h, ctx, evs, id, conv := startFakeSession(t)

	var last time.Time
	for _, step := range []struct {
		report agentfake.Control
		want   server.SessionStatus
	}{
		{agentfake.Control{State: "done"}, server.SessionStatus{State: server.SessionIdle}},
		{agentfake.Control{State: "working"}, server.SessionStatus{State: server.SessionWorking}},
		{agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"},
			server.SessionStatus{State: server.SessionBlocked, Kind: "permission", Message: "run ls?"}},
		{agentfake.Control{State: "done"}, server.SessionStatus{State: server.SessionIdle}},
	} {
		step.report.Action = agentfake.ActionReport
		if err := agentfake.SendProgramControl(conv, step.report); err != nil {
			t.Fatal(err)
		}
		got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status == step.want })
		if !got.StatusChangedAt.After(last) {
			t.Errorf("%s: status changed at %v, not after %v", step.want.State, got.StatusChangedAt, last)
		}
		last = got.StatusChangedAt
		if saved := readMeta(t, h, id); saved["status"].(map[string]any)["state"] != step.want.State {
			t.Errorf("%s: saved status = %v", step.want.State, saved["status"])
		}
	}

	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionConversation, ConversationID: "conv-2"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(server.SessionInfo) bool {
		return readMeta(t, h, id)["conversation_id"] == "conv-2"
	})
}

func TestSessionStatus_ExitedCleanOrCrashed(t *testing.T) {
	for name, code := range map[string]int{server.SessionExitClean: 0, server.SessionExitCrashed: 1} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv := startFakeSession(t)
			if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionExit, Code: code}); err != nil {
				t.Fatal(err)
			}
			got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
			if got.Exit != name {
				t.Errorf("exit = %q, want %q", got.Exit, name)
			}
			if saved := readMeta(t, h, id); saved["exit"] != name {
				t.Errorf("saved exit = %v, want %s", saved["exit"], name)
			}
		})
	}
}

func TestSessionStatus_HolderDeathIsCrashed(t *testing.T) {
	h, ctx, evs, id, _ := startFakeSession(t)
	child := waitForHolder(t, filepath.Join(h.SessionsDir, id)).ChildPID()
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(child)).Output()
	if err != nil {
		t.Fatal(err)
	}
	holder, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-child, syscall.SIGKILL) })
	if err := syscall.Kill(holder, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	if got.Exit != server.SessionExitCrashed {
		t.Errorf("exit = %q, want crashed", got.Exit)
	}
}

// startFakeSession creates a fake-harness session and waits for its first
// report, returning its id and conversation id.
func startFakeSession(t *testing.T) (*servertest.Harness, context.Context, <-chan server.Event, string, string) {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	return h, ctx, evs, id, conv
}

// waitForSession waits for a session-changed event after which the
// snapshot's session id satisfies ok.
func waitForSession(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan server.Event, id string, ok func(server.SessionInfo) bool) server.SessionInfo {
	t.Helper()
	for {
		servertest.WaitForEvent(ctx, t, evs, server.EventSessionChanged, id)
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range snap.Sessions {
			if s.ID == id && ok(s) {
				return s
			}
		}
	}
}

func readMeta(t *testing.T, h *servertest.Harness, id string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.SessionsDir, id, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
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
