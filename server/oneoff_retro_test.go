package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestOneOff_RetroSubmitCreatesEvolveTicketAndRefusesALiveDuplicate(t *testing.T) {
	t.Parallel()
	h, _ := servertest.StartOneOffHarness(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 10, 14, 5, 0, 0, time.UTC)
	req := ticketwrite.RetroSubmit("proj", "epic-a", at)

	first, err := h.Client.OneOff(ctx, req)
	if err != nil || first.Refused {
		t.Fatalf("retro: %+v, %v", first, err)
	}
	if first.Address != "proj:one-offs/01" {
		t.Errorf("address = %q, want proj:one-offs/01", first.Address)
	}
	files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "01-evolve-epic-a-20261010-1405.md"))
	if len(files) != 1 {
		t.Fatalf("ticket files = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	for _, want := range []string{"type: evolve", "notify: true", "unique: evolve:proj/epic-a"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ticket lacks %q:\n%s", want, raw)
		}
	}
	if strings.Contains(string(raw), "agent:") {
		t.Errorf("retro ticket pins an agent:\n%s", raw)
	}

	again := ticketwrite.RetroSubmit("proj", "epic-a", at.Add(time.Hour))
	res, err := h.Client.OneOff(ctx, again)
	if err != nil || !res.Refused || res.Reason != wire.ReasonDuplicateLive || res.Address != first.Address {
		t.Errorf("second retro: %+v, %v; want duplicate-live naming %s", res, err, first.Address)
	}
}

// A plain one-off and the evolve retro pin no harness, so with the default on
// codex both claim on codex.
func TestRunner_UnpinnedOneOffAndRetroClaimOnTheDefaultHarness(t *testing.T) {
	t.Parallel()
	retro := ticketwrite.RetroSubmit("proj", "epic-a", time.Date(2026, 10, 10, 14, 5, 0, 0, time.UTC))
	for name, req := range map[string]wire.OneOffRequest{
		"plain": {Prompt: "plain job", Name: "plain-job", Project: "proj"},
		"retro": retro,
	} {
		// A harness per request: the epic's root runs one ticket at a time.
		store := t.TempDir()
		servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
		fake := runnerfake.NewRunner()
		h := servertest.StartOneOffsHarness(t, store, servertest.WithRunner(fake), func(c *server.Config) { c.AgentsDefaultHarness = "codex" })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		res, err := h.Client.OneOff(ctx, req)
		if err != nil || res.Refused {
			t.Fatalf("%s: %+v, %v", name, res, err)
		}
		for label, kind := range servertest.WaitForAgentKinds(t, fake, "01") {
			if kind != "codex" {
				t.Errorf("%s: %s started as %q, want the default codex", name, label, kind)
			}
		}
		cancel()
	}
}
