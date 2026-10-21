package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

func TestReads_HistoryLocksAndProjects(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	projectDir := filepath.Join(store, "proj")
	for _, ticket := range []string{"01", "02", "01"} {
		if err := ralphloop.AppendEvent(projectDir, "epic-a", ralphloop.Event{Type: string(events.CherryPicked), Ticket: ticket}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ralphloop.AcquireLandLockFor(filepath.Join(projectDir, "epic-a"), "epic-a", "01"); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()

	hist, err := h.Client.History(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Events) != 2 || hist.Events[0].Address != "proj:epic-a/01" {
		t.Errorf("history = %+v", hist)
	}
	if _, err := h.Client.History(ctx, "nope:epic-a/01"); err == nil {
		t.Error("history of an unknown project should fail")
	}

	locks, err := h.Client.Locks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 1 || locks[0].Kind != "land" || locks[0].Epic != "proj:epic-a" || locks[0].Pid != os.Getpid() || !locks[0].Alive || locks[0].Ticket != "01" {
		t.Errorf("locks = %+v", locks)
	}

	projects, err := h.Client.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[0].Name != "proj" || projects[0].Tickets["open"] != 2 || projects[1].Name != "scratch" {
		t.Errorf("projects = %+v", projects)
	}
}

func TestExplain_StoreDerivableVerdicts(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.WriteTicket(t, store, "proj", "epic-a", "03", "dangling", "99")
	issues := filepath.Join(store, "proj", "epic-a", "issues")
	for name, status := range map[string]string{"04-draft.md": "draft", "05-asking.md": "needs-answer"} {
		body := "---\nid: \"" + name[:2] + "\"\nstatus: " + status + "\ntype: implement\n---\n\n# x\n"
		if err := os.WriteFile(filepath.Join(issues, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store)

	want := map[string]struct{ verdict, reason string }{
		"01": {"not queued", ""},
		"02": {"blocked", "01"},
		"03": {"error", ""},
		"04": {"stalled", "draft"},
		"05": {"stalled", "needs-answer"},
	}
	for id, w := range want {
		ex, err := h.Client.Explain(context.Background(), "proj:epic-a/"+id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if ex.Verdict != w.verdict || (w.reason != "" && ex.Reason != w.reason) {
			t.Errorf("%s: explain = %+v, want %+v", id, ex, w)
		}
		if id == "03" && ex.Reason == "" {
			t.Error("error verdict should carry its reason")
		}
	}
	if _, err := h.Client.Explain(context.Background(), "proj:epic-a/77"); err == nil {
		t.Error("explain of an unknown ticket should fail")
	}
}

func TestExplain_ForkChildNamesItsUndoneParent(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "blocker", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "parent", "")
	issues := filepath.Join(store, "proj", "epic-a", "issues")
	body := "---\nid: \"02a\"\nstatus: open\ntype: implement\nparent: \"02\"\nblocked_by: [\"01\"]\n---\n\n# child\n"
	if err := os.WriteFile(filepath.Join(issues, "02a-child.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	ex, err := h.Client.Explain(context.Background(), "proj:epic-a/02a")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Verdict != "blocked" || ex.Reason != "01, parent 02" {
		t.Errorf("explain = %+v, want blocked by \"01, parent 02\"", ex)
	}
}

func TestReads_IterationsAndQueue(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "status: open", "status: claimed", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if argv[2] != "epic-a-iter-01" && argv[2] != "pane-1" {
			return nil, herdrfake.Identities{}, errors.New("agent not found")
		}
		return map[string]any{"agent": map[string]any{
			"pane_id": "pane-1", "workspace_id": "ws-1", "tab_id": "tab-1", "agent_status": "working",
			"agent_session": map[string]any{"value": "sess-1"},
		}}, herdrfake.Identities{}, nil
	})
	ctx := context.Background()

	its, err := h.Client.Iterations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(its) != 1 || its[0].Address != "proj:epic-a/01" || its[0].Pane != "pane-1" ||
		its[0].Branch != "ralph-loop/epic-a-item-01" || !strings.HasSuffix(its[0].Worktree, "epic-a-item-01") ||
		!strings.HasSuffix(its[0].Transcript, "sess-1.jsonl") {
		t.Errorf("iterations = %+v", its)
	}

	q, err := h.Client.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range q {
		got[e.Address] = e.Decision
	}
	if got["proj:epic-a/01"] != "claimed" || got["proj:epic-a/02"] != "blocked" {
		t.Errorf("queue = %+v", q)
	}
}

func TestSnapshot_CarriesTheLandingMetrics(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	with := strings.Replace(string(raw), "status: open", "status: done\nactual_context_window: 66395\nelapsed_time: 316\ncompactions: 2", 1)
	if err := os.WriteFile(path, []byte(with), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tickets) != 1 || snap.Tickets[0].ActualContextWindow != 66395 || snap.Tickets[0].ElapsedTime != 316 || snap.Tickets[0].Compactions != 2 {
		t.Errorf("tickets = %+v, want the landing metrics", snap.Tickets)
	}
}

func TestSnapshot_CarriesCategoryAndExportedAt(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	with := strings.Replace(string(raw), "status: open", "status: open\ncategory: idea\nexported_at: 2026-10-11T09:30:00Z", 1)
	if err := os.WriteFile(path, []byte(with), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
	if len(snap.Tickets) != 1 || snap.Tickets[0].Category != "idea" || !snap.Tickets[0].ExportedAt.Equal(want) {
		t.Errorf("tickets = %+v, want category and exported_at", snap.Tickets)
	}
}

func TestSnapshot_CarriesMutes(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	with := strings.Replace(string(raw), "status: open", "status: needs-repair\nmutes:\n  - event_type: notification-storm\n    tripped_at: 2026-10-11T09:30:00Z", 1)
	if err := os.WriteFile(path, []byte(with), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
	if len(snap.Tickets) != 1 || len(snap.Tickets[0].Mutes) != 1 ||
		snap.Tickets[0].Mutes[0].EventType != "notification-storm" || !snap.Tickets[0].Mutes[0].TrippedAt.Equal(want) {
		t.Errorf("tickets = %+v, want the mute", snap.Tickets)
	}
}

func TestSnapshot_CarriesTheServerBuild(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Build != "test-build" {
		t.Errorf("build = %q, want the server's build", snap.Build)
	}
}

func TestSnapshot_CarriesTheEpicsMergedAt(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	epicMD := filepath.Join(store, "proj", "epic-a", "ticket.md")
	if err := os.WriteFile(epicMD, []byte("---\nmerged_at: 2026-10-11T09:30:00Z\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
	if len(snap.Tickets) != 2 || !snap.Tickets[0].EpicMergedAt.Equal(want) || !snap.Tickets[1].EpicMergedAt.IsZero() {
		t.Errorf("tickets = %+v, want epic-a's merged_at on its ticket only", snap.Tickets)
	}
}

func TestSnapshot_CarriesTheEpicsMapKind(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	epicMD := filepath.Join(store, "proj", "epic-a", "ticket.md")
	if err := os.WriteFile(epicMD, []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tickets) != 2 || !snap.Tickets[0].EpicIsMap || snap.Tickets[1].EpicIsMap {
		t.Errorf("tickets = %+v, want epic-a's map kind on its ticket only", snap.Tickets)
	}
}

func TestSnapshot_CarriesTheEpicsMergeTargetAndStatus(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	done := servertest.TicketOpts{Status: "done"}
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", done)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "02", "second", done)
	servertest.SetProjectTrunk(t, store, "proj", repo, "main")
	epicMD := filepath.Join(store, "proj", "epic-a", "ticket.md")
	if err := os.WriteFile(epicMD, []byte("---\nstatus: draft\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tickets) != 2 {
		t.Fatalf("tickets = %+v, want 2", snap.Tickets)
	}
	for _, ti := range snap.Tickets {
		if ti.EpicMergeTarget != "main" || ti.EpicStatus != "draft" {
			t.Errorf("%s: merge target %q, status %q, want main and draft", ti.Address, ti.EpicMergeTarget, ti.EpicStatus)
		}
	}
}

// The target only labels a Merge that can run, and each one costs git calls,
// so an epic with open tickets or already merged gets none.
func TestSnapshot_LeavesTheMergeTargetOffEpicsThatCantMerge(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	done := servertest.TicketOpts{Status: "done"}
	servertest.WriteTicketWith(t, store, "proj", "open-epic", "01", "first", done)
	servertest.WriteTicket(t, store, "proj", "open-epic", "02", "second", "")
	servertest.WriteTicketWith(t, store, "proj", "merged-epic", "01", "first", done)
	servertest.SetProjectTrunk(t, store, "proj", repo, "main")
	epicMD := filepath.Join(store, "proj", "merged-epic", "ticket.md")
	if err := os.WriteFile(epicMD, []byte("---\nmerged_at: 2026-10-01T10:00:00Z\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tickets) != 3 {
		t.Fatalf("tickets = %+v, want 3", snap.Tickets)
	}
	for _, ti := range snap.Tickets {
		if ti.EpicMergeTarget != "" {
			t.Errorf("%s: merge target %q, want none", ti.Address, ti.EpicMergeTarget)
		}
	}
}
