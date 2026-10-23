package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"

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

func TestPause_RefusesWithoutReasonAndWritesNothing(t *testing.T) {
	h, path := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "  ")
	if err != nil || !res.Refused || res.Reason != server.ReasonReasonRequired {
		t.Fatalf("pause = %+v, %v; want %s refusal", res, err, server.ReasonReasonRequired)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "status: paused") {
		t.Errorf("refused pause changed the ticket:\n%s", data)
	}
}

func TestPause_WritesStatusSectionAndOneEvent(t *testing.T) {
	h, path := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "needs a human look")
	if err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"status: paused", "pause_reason: manual", "## Paused", "needs a human look"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("ticket lacks %q:\n%s", want, data)
		}
	}
	hist, err := h.Client.History(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Events) != 1 || hist.Events[0].Type != "paused" || hist.Events[0].Kind != "manual" {
		t.Errorf("events = %+v, want one stuck/manual-pause", hist.Events)
	}
}

func TestPause_RefusesUnknownTicket(t *testing.T) {
	h, _ := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPause(ctx, "proj:epic-a/99", "why")
	if err != nil || !res.Refused || res.Reason != server.ReasonUnknownTicket {
		t.Fatalf("pause = %+v, %v", res, err)
	}
}

func TestPause_RefusesARunningDoneAlreadyPausedOrMapEpicTicket(t *testing.T) {
	h, path := startTicketWrites(t)
	store := h.TicketStore
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "02", "finished", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicket(t, store, "proj", "map-epic", "01", "mapped", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map-epic", "ticket.md"), []byte("---\nkind: map\nstatus: open\n---\n# Map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Server.PutRun("proj:epic-a", server.Run{Address: "proj:epic-a/01", Runner: "herdr", Session: agentrunner.Session{ID: "p1"}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, c := range []struct{ name, address, reason string }{
		{"running", "proj:epic-a/01", server.ReasonIterationRunning},
		{"done", "proj:epic-a/02", server.ReasonTicketDone},
		{"map epic", "proj:map-epic/01", server.ReasonMapEpic},
	} {
		res, err := h.Client.TicketPause(ctx, c.address, "why")
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("pause %s = %+v, %v; want %s refusal", c.name, res, err, c.reason)
		}
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "status: paused") {
		t.Errorf("refused pause changed the running ticket:\n%s", data)
	}
}

func TestPause_RefusesAnAlreadyPausedTicket(t *testing.T) {
	h, _ := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "first"); err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "second")
	if err != nil || !res.Refused || res.Reason != server.ReasonAlreadyPaused {
		t.Fatalf("second pause = %+v, %v; want %s refusal", res, err, server.ReasonAlreadyPaused)
	}
}

const mutedFrontmatter = "mutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n"

func writeMuted(t *testing.T, path, status, body string) {
	t.Helper()
	content := "---\nid: \"01\"\nstatus: " + status + "\ntype: implement\n" + mutedFrontmatter + "---\n# first\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUnmute_ReopensAStuckTicketAndDemotesItsSection(t *testing.T) {
	h, path := startTicketWrites(t)
	writeMuted(t, path, "paused\npause_reason: merge-conflict", "## Needs Repair\n\nnotification storm tripped a mute\n")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/01")
	if err != nil || res.Refused {
		t.Fatalf("unmute = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	raw := string(data)
	for _, want := range []string{"status: open", "## Comments", "retired from `## Needs Repair`", "— unmuted"} {
		if !strings.Contains(raw, want) {
			t.Errorf("ticket lacks %q:\n%s", want, raw)
		}
	}
	for _, unwanted := range []string{"mutes:", "\n## Needs Repair\n", "via Suggested Actions"} {
		if strings.Contains(raw, unwanted) {
			t.Errorf("ticket still has %q:\n%s", unwanted, raw)
		}
	}
}

func TestUnmute_LeavesANonPausedTicketsStatusAlone(t *testing.T) {
	h, path := startTicketWrites(t)
	writeMuted(t, path, "open", "Body text.\n")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("unmute = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	if raw := string(data); !strings.Contains(raw, "status: open") || strings.Contains(raw, "mutes:") {
		t.Errorf("want status kept and mutes cleared:\n%s", raw)
	}
}

func TestUnmute_RefusesATicketWithNoMutes(t *testing.T) {
	h, path := startTicketWrites(t)
	before, _ := os.ReadFile(path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/01")
	if err != nil || !res.Refused || res.Reason != server.ReasonNotMuted {
		t.Fatalf("unmute = %+v, %v; want %s refusal", res, err, server.ReasonNotMuted)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("refused unmute changed the ticket:\n%s", after)
	}
}

func TestUnmute_RefusesUnknownTicket(t *testing.T) {
	h, _ := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/99")
	if err != nil || !res.Refused || res.Reason != server.ReasonUnknownTicket {
		t.Fatalf("unmute = %+v, %v", res, err)
	}
}

func TestRelaunch_StartsAFreshIterationOfAPausedTicket(t *testing.T) {
	h, path := startTicketWrites(t)
	start, prompt, _ := registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false)
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

func TestCancel_OldOneOffAddressResolvesTheArchivedTicket(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", server.OneOffsArchivedEpic, "01", "a", servertest.TicketOpts{Status: "cancelled"})
	path := filepath.Join(store, "proj", server.OneOffsArchivedEpic, "issues", "01-a.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n## Resolution\n\n- original: proj:one-offs/07\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketCancel(ctx, "proj:one-offs/07", false)
	if err != nil || !res.Refused || res.Reason != server.ReasonTicketDone {
		t.Fatalf("cancel = %+v, %v; want %s refusal (not unknown-ticket)", res, err, server.ReasonTicketDone)
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
		data, _ := os.ReadFile(p)
		if !strings.Contains(string(data), "status: "+want) {
			t.Errorf("%s lacks status %s:\n%s", filepath.Base(p), want, data)
		}
		if stamped := strings.Contains(string(data), "cancelled_at:"); stamped != (want == "cancelled") {
			t.Errorf("%s cancelled_at stamped = %v:\n%s", filepath.Base(p), stamped, data)
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
