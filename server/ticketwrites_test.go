package server_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"

	"github.com/elentok/gx/ralphloop"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestCancel_EndsThePausedTicketsRoundWithTheRequestsSource(t *testing.T) {
	t.Parallel()
	h, path := startTicketWrites(t)
	if err := h.Server.PauseAs("proj:epic-a/01", events.MergeConflict, "stuck"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	rounds := endedRounds(t, path)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowCancel || rounds[0].End.Source != wire.OriginCLI {
		t.Errorf("rounds = %+v; want one ended by cli cancel", rounds)
	}
}

func TestCancel_WithoutAnOpenRoundWritesNoEnd(t *testing.T) {
	t.Parallel()
	h, path := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	if rounds := endedRounds(t, path); len(rounds) != 0 {
		t.Errorf("rounds = %+v; want none", rounds)
	}
}

func branchListed(t *testing.T, repo, branch string) bool {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "branch", "--list", branch).CombinedOutput()
	if err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out)) != ""
}

func TestCancel_RemovesTheKeptWorktreeAndKeepsTheBranch(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	wt := keepWorktree(t, repo, "epic-a", "01")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree still there: %v", err)
	}
	if !branchListed(t, repo, ralphloop.IterationBranch("epic-a", "01")) {
		t.Error("iteration branch gone")
	}
}

func TestCancel_RemovesTheForkDescendantsWorktreeToo(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	kid := "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\n\n# kid\n"
	if err := os.WriteFile(filepath.Join(store, "proj", "epic-a", "issues", "02-kid.md"), []byte(kid), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	wts := []string{keepWorktree(t, repo, "epic-a", "01"), keepWorktree(t, repo, "epic-a", "02")}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	for _, wt := range wts {
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Errorf("worktree %s still there: %v", wt, err)
		}
	}
}

func TestCancel_FailedWorktreeRemovalDoesNotStopTheCancel(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	kidPath := filepath.Join(store, "proj", "epic-a", "issues", "02-kid.md")
	kid := "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\n\n# kid\n"
	if err := os.WriteFile(kidPath, []byte(kid), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	// A locked worktree makes a single forced removal fail.
	wts := []string{keepWorktree(t, repo, "epic-a", "01"), keepWorktree(t, repo, "epic-a", "02")}
	for _, wt := range wts {
		if out, err := exec.Command("git", "-C", repo, "worktree", "lock", wt).CombinedOutput(); err != nil {
			t.Fatalf("git worktree lock: %v\n%s", err, out)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventTicketCancelled, "proj:epic-a/01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventTicketCancelled, "proj:epic-a/02")
	for _, p := range []string{filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"), kidPath} {
		if b, err := os.ReadFile(p); err != nil || !strings.Contains(string(b), "status: cancelled") {
			t.Errorf("%s not cancelled: %v\n%s", p, err, b)
		}
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	h, path := startTicketWrites(t)
	before, _ := os.ReadFile(path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/01")
	if err != nil || !res.Refused || res.Reason != wire.ReasonNotMuted {
		t.Fatalf("unmute = %+v, %v; want %s refusal", res, err, wire.ReasonNotMuted)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("refused unmute changed the ticket:\n%s", after)
	}
}

func TestUnmute_RefusesUnknownTicket(t *testing.T) {
	t.Parallel()
	h, _ := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/99")
	if err != nil || !res.Refused || res.Reason != wire.ReasonUnknownTicket {
		t.Fatalf("unmute = %+v, %v", res, err)
	}
}

func TestCancel_LiveTicketRefusesUntilStop(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	_, path := h.LeavePtyRun(t, store, repo, epicA01, true, nil)
	h.Restart(t)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", false)
	if err != nil || !res.Refused || res.Reason != wire.ReasonTicketLive {
		t.Fatalf("cancel = %+v, %v; want %s refusal", res, err, wire.ReasonTicketLive)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "status: cancelled") || h.Stops() != 0 {
		t.Fatalf("refused cancel changed things (stops=%d):\n%s", h.Stops(), data)
	}
	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("cancel --stop = %+v, %v", res, err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "status: cancelled") {
		t.Errorf("ticket not cancelled:\n%s", data)
	}
	if h.Stops() != 1 {
		t.Errorf("agent stops = %d, want 1", h.Stops())
	}
}

func TestCancel_OldOneOffAddressResolvesTheArchivedTicket(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsArchivedEpic, "01", "a", servertest.TicketOpts{Status: "cancelled"})
	path := filepath.Join(store, "proj", queue.OneOffsArchivedEpic, "issues", "01-a.md")
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
	if err != nil || !res.Refused || res.Reason != wire.ReasonTicketDone {
		t.Fatalf("cancel = %+v, %v; want %s refusal (not unknown-ticket)", res, err, wire.ReasonTicketDone)
	}
}

func TestCancel_CascadesToNonTerminalDescendants(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	h, path := startTicketWrites(t)
	if err := ralphloop.Claim(path); err != nil {
		t.Fatal(err)
	}
	launched := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	h.Server.Registry().PutRunAt("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "pty", Session: agentrunner.Session{ID: "p1"}}, launched)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var claimed wire.TicketInfo
	servertest.WaitFor(t, 5*time.Second, "the ticket to show as claimed", func() bool {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range snap.Tickets {
			if tk.Address == "proj:epic-a/01" && tk.Status == "claimed" {
				claimed = tk
				return true
			}
		}
		return false
	})
	if !claimed.ClaimedAt.Equal(launched) {
		t.Fatalf("ClaimedAt = %v, want %v", claimed.ClaimedAt, launched)
	}
}
