package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/servertest"
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
		if argv[2] != "epic-a-iter-01" {
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
