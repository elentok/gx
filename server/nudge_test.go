package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/testutil/runnerfake"
)

func claimTicket(t *testing.T, store, epic, id, file string) {
	t.Helper()
	path := filepath.Join(store, "proj", epic, "issues", file)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "status: open", "status: claimed", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNudge_TypesIntoLivePaneLogsAndRateLimits(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	claimTicket(t, store, "epic-a", "01", "01-first.md")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	h := servertest.StartWithStore(t, store)
	var mu sync.Mutex
	var typed []string
	h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if argv[2] != "epic-a-iter-01" && argv[2] != "pane-1" {
			return nil, herdrfake.Identities{}, errors.New("agent not found")
		}
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "tab_id": "tab-1", "agent_status": "idle", "state_change_seq": 1}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		typed = append(typed, argv[2]+": "+argv[3])
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "agent_status": "working", "state_change_seq": 2}}, herdrfake.Identities{}, nil
	})
	ctx := context.Background()

	res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "please continue")
	if err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
	mu.Lock()
	if len(typed) != 1 || typed[0] != "pane-1: please continue" {
		t.Errorf("typed = %v", typed)
	}
	mu.Unlock()

	evs, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "run-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(evs), `"type":"`+string(events.Nudged)+`"`); n != 1 || !strings.Contains(string(evs), `"text":"please continue"`) {
		t.Errorf("run log should hold one nudged event with the text, got:\n%s", evs)
	}

	for i := 0; i < 2; i++ {
		if res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "again"); err != nil || res.Refused {
			t.Fatalf("nudge %d = %+v, %v", i+2, res, err)
		}
	}
	res, err = h.Client.TicketNudge(ctx, "proj:epic-a/01", "once more")
	if err != nil || !res.Refused || res.Reason != server.ReasonRateLimited {
		t.Errorf("over the limit = %+v, %v; want refusal %s", res, err, server.ReasonRateLimited)
	}
	mu.Lock()
	if len(typed) != 3 {
		t.Errorf("a refused nudge must not reach the pane, typed = %v", typed)
	}
	mu.Unlock()
}

func TestNudge_NotReadyRefuses(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	claimTicket(t, store, "epic-a", "01", "01-first.md")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Runner = fake })
	const label = "epic-a-iter-01"
	if _, err := fake.Start(agentrunner.StartOptions{Label: label, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	fake.SetState(label, agentrunner.StateBlocked, "permission to use Bash")
	res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "hi")
	if err != nil || !res.Refused || res.Reason != server.ReasonAgentNotReady {
		t.Errorf("nudge while blocked = %+v, %v; want refusal %s", res, err, server.ReasonAgentNotReady)
	}

	fake.SetState(label, agentrunner.StateIdle, "")
	fake.SetPromptErr(label, errors.New("boom"))
	if res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "hi"); err == nil || res.Refused {
		t.Errorf("other prompt error = %+v, %v; want an error", res, err)
	}
}

func TestNudge_Refusals(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	for _, c := range []struct{ name, addr, text, reason string }{
		{"no text", "proj:epic-a/01", " ", server.ReasonTextRequired},
		{"unknown ticket", "proj:epic-a/77", "hi", server.ReasonUnknownTicket},
		{"no live pane", "proj:epic-a/01", "hi", server.ReasonIterationNotLive},
	} {
		res, err := h.Client.TicketNudge(ctx, c.addr, c.text)
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
}
