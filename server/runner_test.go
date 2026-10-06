package server_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// registerLaunch answers the herdr commands a launch makes and records the
// agent start and prompt argv.
func registerLaunch(h *servertest.Harness) (start, prompt *[]string, cwd *string) {
	start, prompt, cwd = new([]string), new([]string), new(string)
	agent := map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}
	h.Herdr.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "w1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "create", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		for i, a := range argv {
			if a == "--cwd" && i+1 < len(argv) {
				*cwd = argv[i+1]
			}
		}
		return map[string]any{"tab": map[string]any{"tab_id": "t1"}, "root_pane": map[string]any{"pane_id": "p1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{}, herdrfake.Identities{}, nil
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
	return start, prompt, cwd
}

func TestRunner_ClaimsWriteTheFileBeforeTheEventAndLaunchWithTheChosenAgent(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	start, prompt, _ := registerLaunch(h)
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

func TestRunner_LandsTheIterationAndClosesTheRoot(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	_, _, cwd := registerLaunch(h)
	// The agent's turn: commit a file in its own worktree.
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, _ []string) (any, herdrfake.Identities, error) {
		testutil.WriteFile(t, *cwd, "agent.txt", "work")
		testutil.CommitAll(t, *cwd, "agent work")
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle"}}, herdrfake.Identities{}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	var seen []string
	for ev := range evs {
		switch ev.Type {
		case server.EventTicketClaimed, server.EventIterationStarted, server.EventTicketDone, server.EventRootCompleted,
			server.EventIterationFailed, server.EventIterationParked:
			seen = append(seen, ev.Type)
		}
		if ev.Type == server.EventRootCompleted || ev.Type == server.EventIterationFailed {
			break
		}
	}
	want := []string{server.EventTicketClaimed, server.EventIterationStarted, server.EventTicketDone, server.EventRootCompleted}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", seen, want)
	}
	data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: done") {
		t.Errorf("ticket file is not done:\n%s", data)
	}
	out, err := exec.Command("git", "-C", repo, "show", "epic-a:agent.txt").CombinedOutput()
	if err != nil || string(out) != "work" {
		t.Errorf("feature branch lacks the agent's commit: %q, %v", out, err)
	}
}
