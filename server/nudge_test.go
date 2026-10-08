package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
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
		if argv[2] != "epic-a-iter-01" {
			return nil, herdrfake.Identities{}, errors.New("agent not found")
		}
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "tab_id": "tab-1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		typed = append(typed, argv[2]+": "+argv[3])
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1"}}, herdrfake.Identities{}, nil
	})
	ctx := context.Background()

	res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "please continue")
	if err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
	mu.Lock()
	if len(typed) != 1 || typed[0] != "epic-a-iter-01: please continue" {
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
