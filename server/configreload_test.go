package server_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

func reload(t *testing.T, h *servertest.Harness) server.ReloadResult {
	t.Helper()
	res, err := h.Client.ReloadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func maxAgents(n int) map[string]any {
	return map[string]any{"execution-queue": map[string]any{"max-agents": n}}
}

// Seam A: a raised cap starts a queued root without waiting for the poll
// tick; a lowered one kills no running agent.
func TestConfigReload_MaxAgentsAppliesWithoutKillingAgents(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.MaxAgents = 1
		c.PollInterval = time.Hour
		c.Sleep = time.Sleep
	})
	registerLaunch(h)
	// Started agents keep their slot: an iteration's finish wait never returns.
	idle := map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle", "state_change_seq": 1}}
	h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if slices.Contains(argv, "blocked") {
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		}
		return idle, herdrfake.Identities{}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, addr := range []string{"proj:epic-a/01", "proj:epic-b/01"} {
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	waitRuns(ctx, t, h, 1)

	servertest.WriteConfig(t, path, store, maxAgents(2))
	if res := reload(t, h); res.Refused || !slices.Contains(res.Changed, "execution-queue.max-agents") {
		t.Fatalf("raise = %+v, want max-agents changed", res)
	}
	waitRuns(ctx, t, h, 2)

	servertest.WriteConfig(t, path, store, maxAgents(1))
	if res := reload(t, h); res.Refused {
		t.Fatalf("lower = %+v", res)
	}
	time.Sleep(100 * time.Millisecond)
	if runs := h.Server.Runs(); len(runs) != 2 {
		t.Errorf("runs after lowering = %+v, want both agents still running", runs)
	}
}

func waitRuns(ctx context.Context, t *testing.T, h *servertest.Harness, n int) {
	t.Helper()
	for len(h.Server.Runs()) != n {
		if ctx.Err() != nil {
			t.Fatalf("runs = %+v, want %d", h.Server.Runs(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Seam A: an invalid or missing file is refused and leaves the live settings
// as they were.
func TestConfigReload_RefusesInvalidAndMissingFiles(t *testing.T) {
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, maxAgents(3))
	if res := reload(t, h); res.Refused {
		t.Fatalf("baseline = %+v", res)
	}

	if err := os.WriteFile(path, []byte(`{"execution-queue": {"max-agents": 5}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := reload(t, h); res.Reason != server.ReasonInvalidConfig || res.Message == "" {
		t.Errorf("bad json = %+v, want %s with the parse error", res, server.ReasonInvalidConfig)
	}
	servertest.WriteConfig(t, path, store, map[string]any{"execution-queue": map[string]any{"max-agents": 5}, "agent-runner": "pty"})
	if res := reload(t, h); res.Reason != server.ReasonInvalidConfig {
		t.Errorf("bad agent-runner = %+v, want %s", res, server.ReasonInvalidConfig)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if res := reload(t, h); res.Reason != server.ReasonConfigMissing {
		t.Errorf("missing = %+v, want %s", res, server.ReasonConfigMissing)
	}

	servertest.WriteConfig(t, path, store, maxAgents(3))
	if res := reload(t, h); res.Refused || len(res.Changed) != 0 {
		t.Errorf("after refusals = %+v, want no changes", res)
	}
}

// Seam A: a startup-only key warns on every reload until restart, and the
// rest of the file still applies.
func TestConfigReload_StartupOnlyKeyWarnsUntilRestart(t *testing.T) {
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) { c.MaxAgents = 3 })
	servertest.WriteConfig(t, path, store, map[string]any{
		"execution-queue": map[string]any{"max-agents": 4},
		"server":          map[string]any{"tcp-listen": true},
	})
	want := []string{"restart needed for server.tcp-listen"}
	res := reload(t, h)
	if res.Refused || !slices.Contains(res.Changed, "execution-queue.max-agents") || !slices.Equal(res.Warnings, want) {
		t.Fatalf("first = %+v, want max-agents changed and warnings %v", res, want)
	}
	if res := reload(t, h); slices.Contains(res.Changed, "execution-queue.max-agents") || !slices.Equal(res.Warnings, want) {
		t.Errorf("second = %+v, want no max-agents change and warnings %v", res, want)
	}
}

func TestConfigReload_UnavailableWithoutConfigPath(t *testing.T) {
	h := servertest.Start(t)
	if res := reload(t, h); res.Reason != server.ReasonReloadUnavailable {
		t.Errorf("reload = %+v, want %s", res, server.ReasonReloadUnavailable)
	}
}
