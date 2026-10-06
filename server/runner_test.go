package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

// registerLaunch answers the herdr commands a launch makes and records the
// agent start and prompt argv.
func registerLaunch(h *servertest.Harness) (start, prompt *[]string) {
	start, prompt = new([]string), new([]string)
	agent := map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}
	h.Herdr.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "w1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"tab": map[string]any{"tab_id": "t1"}, "root_pane": map[string]any{"pane_id": "p1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "start", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		*start = argv
		return agent, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return agent, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		*prompt = argv
		return agent, herdrfake.Identities{}, nil
	})
	return start, prompt
}

func TestRunner_ClaimsWriteTheFileBeforeTheEventAndLaunchWithTheChosenAgent(t *testing.T) {
	store, repo := t.TempDir(), t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	start, prompt := registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	claimedOnDisk := false
	for ev := range evs {
		if ev.Type == server.EventTicketClaimed {
			data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
			if err != nil {
				t.Fatal(err)
			}
			claimedOnDisk = strings.Contains(string(data), "status: claimed")
			if ev.Address != "proj:epic-a/01" {
				t.Errorf("claim event address = %q", ev.Address)
			}
		}
		if ev.Type == server.EventIterationStarted {
			break
		}
	}
	if !claimedOnDisk {
		t.Error("ticket file did not say claimed when the claim event streamed")
	}
	if !strings.Contains(strings.Join(*start, " "), "--kind codex") {
		t.Errorf("agent start = %v, want kind codex", *start)
	}
	if !strings.Contains(strings.Join(*prompt, " "), "$gx-implement proj:epic-a/01") {
		t.Errorf("agent prompt = %v, want the codex skill prompt", *prompt)
	}
	if runs := h.Server.Runs(); len(runs) != 1 || runs[0].Address != "proj:epic-a/01" || runs[0].Agent != "codex" {
		t.Errorf("runs = %+v", runs)
	}
}
