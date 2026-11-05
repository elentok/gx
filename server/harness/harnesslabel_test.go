package harness_test

import (
	"context"
	"github.com/elentok/gx/server/servertest/rigs"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// labelsOf is the label of the ticket at address from the epic detail and from
// the snapshot; both must agree.
func labelsOf(t *testing.T, h *servertest.Harness, base, epic, address string) wire.HarnessLabel {
	t.Helper()
	code, d, body := rigs.GetEpicDetail(t, h, base, epic)
	if code != 200 {
		t.Fatalf("detail status %d: %s", code, body)
	}
	fromDetail := rigs.RowOf(t, d, address).HarnessLabel
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, ti := range snap.Tickets {
		if ti.Address == address {
			if ti.HarnessLabel != fromDetail {
				t.Errorf("snapshot label %+v != detail label %+v", ti.HarnessLabel, fromDetail)
			}
			return fromDetail
		}
	}
	t.Fatalf("no snapshot ticket %s", address)
	return wire.HarnessLabel{}
}

func TestHarnessLabel_WaitingNotes(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": rigs.IssueMD("01", "open", "", "A"),
			"02-b.md": rigs.IssueMD("02", "open", "agent: codex\n", "B"),
			"03-c.md": rigs.IssueMD("03", "open", "", "C"),
			"04-d.md": rigs.IssueMD("04", "open", "agent: codex\n", "D"),
			"05-e.md": rigs.IssueMD("05", "open", "", "E"),
			"06-f.md": rigs.IssueMD("06", "done", "", "F"),
		})

	})

	rigs.SeedEarlierClaudeRun(t, h, h.TicketStore, "proj", "work", "03", "sess-3", true)
	rigs.SeedEarlierClaudeRun(t, h, h.TicketStore, "proj", "work", "04", "sess-4", true)
	rigs.SeedEarlierClaudeRun(t, h, h.TicketStore, "proj", "work", "05", "sess-5", false)
	ctx := context.Background()
	if _, err := h.Client.Queue.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:work/02", ""); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	for address, want := range map[string]wire.HarnessLabel{
		"proj:work/01": {HarnessNote: "follows default"},
		"proj:work/02": {Harness: "codex", HarnessNote: "pinned"},
		"proj:work/03": {Harness: "claude", HarnessNote: "keeps Claude (ran before)"},
		"proj:work/04": {Harness: "codex", HarnessNote: "pinned"},
		"proj:work/05": {HarnessNote: "follows default"},
		"proj:work/06": {},
	} {
		if got := labelsOf(t, h, base, "proj:work", address); got != want {
			t.Errorf("%s label = %+v, want %+v", address, got, want)
		}
	}
}

func TestHarnessLabel_QueueItemAgentPins(t *testing.T) {
	t.Parallel()
	h, base := rigs.StartEpicDetail(t, func(store string) {
		rigs.WriteEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
			"01-a.md": rigs.IssueMD("01", "open", "", "A"),
		})

	})

	ctx := context.Background()
	if _, err := h.Client.Queue.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:work/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	want := wire.HarnessLabel{Harness: "codex", HarnessNote: "pinned"}
	if got := labelsOf(t, h, base, "proj:work", "proj:work/01"); got != want {
		t.Errorf("label = %+v, want %+v", got, want)
	}
}

func TestHarnessLabel_RunningRowCarriesTheRunsConfig(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{})
	h, fake, _ := startHarnessClaim(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	rigs.WaitKind(t, fake, "epic-a-iter-01")
	var got wire.HarnessLabel
	servertest.WaitFor(t, 10*time.Second, "running label", func() bool {
		got = labelsOf(t, h, hs.WebURL, "proj:epic-a", "proj:epic-a/01")
		return got.Tier != ""
	})
	if got.Harness != "claude" || got.Tier != "worker" || got.HarnessNote != "" {
		t.Errorf("running label = %+v, want claude/worker and no note", got)
	}
}
