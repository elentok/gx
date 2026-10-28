package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// startRemoteFix starts a server whose fake harness takes the remote-control
// setting, with the key as given in the config file.
func startRemoteFix(t *testing.T, on bool) (*servertest.Harness, string, func(bool)) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "loop", "01", "broken", servertest.TicketOpts{PauseReason: "merge-conflict"})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), servertest.WithTestHolder(),
		servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithRemoteControlHarness(string(agentfake.HarnessKind)))
	set := func(on bool) {
		t.Helper()
		servertest.WriteConfig(t, path, store, map[string]any{"agents": map[string]any{"fix-agent-remote-control": on}})
		if res := reload(t, h); res.Refused {
			t.Fatalf("reload = %+v", res)
		}
	}
	set(on)
	return h, "proj:loop/01", set
}

func TestFixAgent_RemoteControlKeyOn(t *testing.T) {
	t.Parallel()
	h, addr, _ := startRemoteFix(t, true)
	res, err := h.Client.FixAgent(context.Background(), server.FixAgentRequest{Address: addr})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("fix = %+v, %v", res, err)
	}
	if !res.RemoteControl {
		t.Error("remote_control = false, want true with the key on")
	}
	if got := readMeta(t, h, res.Session.ID)["remote_control"]; got != addr {
		t.Errorf("meta remote_control = %v, want %s", got, addr)
	}
}

func TestFixAgent_RemoteControlKeyOff(t *testing.T) {
	t.Parallel()
	h, addr, _ := startRemoteFix(t, false)
	res, err := h.Client.FixAgent(context.Background(), server.FixAgentRequest{Address: addr})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("fix = %+v, %v", res, err)
	}
	if res.RemoteControl {
		t.Error("remote_control = true, want false with the key off")
	}
	if got, ok := readMeta(t, h, res.Session.ID)["remote_control"]; ok {
		t.Errorf("meta remote_control = %v, want none", got)
	}
}

// The name lives in the meta, so a resume keeps it after the key is turned off.
func TestFixAgent_ResumeKeepsRemoteControlAfterKeyOff(t *testing.T) {
	t.Parallel()
	h, addr, set := startRemoteFix(t, true)
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
	stopFakeSession(ctx, t, h, evs, id)
	set(false)

	if r, err := h.Client.ResumeSession(ctx, id); err != nil || r.Refused {
		t.Fatalf("resume = %+v, %v", r, err)
	}
	if got := readMeta(t, h, id)["remote_control"]; got != addr {
		t.Errorf("meta remote_control after resume = %v, want %s", got, addr)
	}
}
