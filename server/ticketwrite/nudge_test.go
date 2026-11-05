package ticketwrite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestNudge_TypesIntoLivePaneLogsAndRateLimits(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.ClaimTicket(t, store, "epic-a", "01", "01-first.md")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	const label = "epic-a-iter-01"
	if _, err := fake.Start(agentrunner.StartOptions{Label: label, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "please continue")
	if err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
	if typed := fake.Prompts(label); len(typed) != 1 || typed[0] != "please continue" {
		t.Errorf("typed = %v", typed)
	}

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
	if err != nil || !res.Refused || res.Reason != wire.ReasonRateLimited {
		t.Errorf("over the limit = %+v, %v; want refusal %s", res, err, wire.ReasonRateLimited)
	}
	if typed := fake.Prompts(label); len(typed) != 3 {
		t.Errorf("a refused nudge must not reach the pane, typed = %v", typed)
	}
}

func TestNudge_NotReadyRefuses(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.ClaimTicket(t, store, "epic-a", "01", "01-first.md")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	const label = "epic-a-iter-01"
	if _, err := fake.Start(agentrunner.StartOptions{Label: label, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	fake.SetState(label, agentrunner.StateBlocked, "permission to use Bash")
	res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "hi")
	if err != nil || !res.Refused || res.Reason != wire.ReasonAgentNotReady {
		t.Errorf("nudge while blocked = %+v, %v; want refusal %s", res, err, wire.ReasonAgentNotReady)
	}

	fake.SetState(label, agentrunner.StateIdle, "")
	fake.SetPromptErr(label, errors.New("boom"))
	if res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "hi"); err == nil || res.Refused {
		t.Errorf("other prompt error = %+v, %v; want an error", res, err)
	}
}

func TestNudge_Refusals(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	for _, c := range []struct{ name, addr, text, reason string }{
		{"no text", "proj:epic-a/01", " ", wire.ReasonTextRequired},
		{"unknown ticket", "proj:epic-a/77", "hi", wire.ReasonUnknownTicket},
		{"no live pane", "proj:epic-a/01", "hi", wire.ReasonIterationNotLive},
	} {
		res, err := h.Client.TicketNudge(ctx, c.addr, c.text)
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
}
