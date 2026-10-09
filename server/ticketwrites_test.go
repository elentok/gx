package server_test

import (
	"context"
	"github.com/elentok/gx/ralphloop"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

func startTicketWrites(t *testing.T) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	return h, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
}

func TestPark_RefusesWithoutReasonAndWritesNothing(t *testing.T) {
	h, path := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "  ")
	if err != nil || !res.Refused || res.Reason != server.ReasonReasonRequired {
		t.Fatalf("park = %+v, %v; want %s refusal", res, err, server.ReasonReasonRequired)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "needs-repair") {
		t.Errorf("refused park changed the ticket:\n%s", data)
	}
}

func TestPark_WritesStatusSectionAndOneEvent(t *testing.T) {
	h, path := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "needs a human look")
	if err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"status: needs-repair", "## Needs Repair", "needs a human look"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("ticket lacks %q:\n%s", want, data)
		}
	}
	hist, err := h.Client.History(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Events) != 1 || hist.Events[0].Type != "needs-repair" || hist.Events[0].Kind != "manual-park" {
		t.Errorf("events = %+v, want one needs-repair/manual-park", hist.Events)
	}
}

func TestPark_RefusesUnknownTicket(t *testing.T) {
	h, _ := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPark(ctx, "proj:epic-a/99", "why")
	if err != nil || !res.Refused || res.Reason != server.ReasonUnknownTicket {
		t.Fatalf("park = %+v, %v", res, err)
	}
}

func TestRelaunch_StartsAFreshIterationOfAParkedTicket(t *testing.T) {
	h, path := startTicketWrites(t)
	start, prompt, _ := registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01")
	if err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: claimed") {
		t.Errorf("relaunched ticket is not claimed:\n%s", data)
	}
	if len(*start) == 0 || !strings.Contains(strings.Join(*prompt, " "), "proj:epic-a/01") {
		t.Errorf("agent start = %v, prompt = %v", *start, *prompt)
	}
}

func TestCancel_LiveTicketRefusesUntilStop(t *testing.T) {
	h, path := startTicketWrites(t)
	var closed atomic.Int32
	h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "tab_id": "t1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		closed.Add(1)
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	h.Server.PutRun("proj:epic-a", server.Run{Address: "proj:epic-a/01", Runner: "herdr", Session: agentrunner.Session{ID: "p1"}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false)
	if err != nil || !res.Refused || res.Reason != server.ReasonTicketLive {
		t.Fatalf("cancel = %+v, %v; want %s refusal", res, err, server.ReasonTicketLive)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "status: cancelled") || closed.Load() != 0 {
		t.Fatalf("refused cancel changed things (closed=%d):\n%s", closed.Load(), data)
	}
	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("cancel --stop = %+v, %v", res, err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "status: cancelled") {
		t.Errorf("ticket not cancelled:\n%s", data)
	}
	if closed.Load() != 1 {
		t.Errorf("tab close calls = %d, want 1", closed.Load())
	}
}

func TestCancel_CascadesToNonTerminalDescendants(t *testing.T) {
	h, path := startTicketWrites(t)
	issues := filepath.Dir(path)
	child := func(id, status string) string {
		p := filepath.Join(issues, id+"-kid.md")
		body := "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: implement\nparent: \"01\"\n---\n\n# kid\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	open, done := child("02", "open"), child("03", "done")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	for p, want := range map[string]string{path: "cancelled", open: "cancelled", done: "done"} {
		if data, _ := os.ReadFile(p); !strings.Contains(string(data), "status: "+want) {
			t.Errorf("%s lacks status %s:\n%s", filepath.Base(p), want, data)
		}
	}
}

func TestSnapshot_ClaimedTicketCarriesLaunchTime(t *testing.T) {
	h, path := startTicketWrites(t)
	if err := ralphloop.Claim(path); err != nil {
		t.Fatal(err)
	}
	launched := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	h.Server.PutRunAt("proj:epic-a", server.Run{Address: "proj:epic-a/01", Runner: "herdr", Session: agentrunner.Session{ID: "p1"}}, launched)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deadline := time.Now().Add(5 * time.Second)
	for {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range snap.Tickets {
			if tk.Address == "proj:epic-a/01" && tk.Status == "claimed" {
				if !tk.ClaimedAt.Equal(launched) {
					t.Fatalf("ClaimedAt = %v, want %v", tk.ClaimedAt, launched)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("ticket never showed as claimed: %+v", snap.Tickets)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
