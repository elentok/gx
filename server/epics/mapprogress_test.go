package epics_test

import (
	"context"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest/rigs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestProgressStepsMatchTickets(t *testing.T) {
	t.Parallel()
	design := []wire.ProgressStep{
		wire.ProgressChart, wire.ProgressResolveTickets, wire.ProgressSpec, wire.ProgressReview,
		wire.ProgressGrillFindings, wire.ProgressCreateTickets, wire.ProgressBranch, wire.ProgressDone,
	}
	triage := []wire.ProgressStep{
		wire.ProgressVerify, wire.ProgressRoute, wire.ProgressQuestions, wire.ProgressWrite, wire.ProgressDone,
	}
	asStrings := func(steps []wire.ProgressStep) []string {
		out := make([]string, len(steps))
		for i, s := range steps {
			out[i] = string(s)
		}
		return out
	}
	for flow, got := range map[tickets.Flow][]wire.ProgressStep{tickets.FlowDesign: design, tickets.FlowTriage: triage} {
		spec, err := tickets.FlowSpecFor(flow)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(asStrings(got), spec.Steps) {
			t.Fatalf("%s: wire.Progress* = %v, want flow table = %v", flow, got, spec.Steps)
		}
	}
	if !slices.Equal(asStrings(design), tickets.DesignStepNames()) {
		t.Fatalf("wire design = %v, want tickets.DesignSteps = %v", design, tickets.DesignSteps)
	}
}

func TestEpicList_MapProgress(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mapMD := func(front string) string { return "---\nstatus: open\nkind: map\n" + front + "---\n" }
	stepMD := func(step string) string { return mapMD("design_step: " + step + "\n") }
	open := map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": rigs.IssueMD("01", "done", "", "A")}

	steps := []wire.ProgressStep{
		wire.ProgressChart, wire.ProgressResolveTickets, wire.ProgressSpec, wire.ProgressReview,
		wire.ProgressGrillFindings, wire.ProgressCreateTickets, wire.ProgressBranch, wire.ProgressDone,
	}

	h, base := rigs.StartEpicDetail(t, func(store string) {
		for _, s := range steps {
			rigs.WriteEpicFiles(t, store, "m-"+string(s), stepMD(string(s)), open)
		}
		rigs.WriteEpicFiles(t, store, "m-no-field", mapMD(""), done)
		rigs.WriteEpicFiles(t, store, "m-bad-step", stepMD("not-a-step"), open)
		rigs.WriteEpicFiles(t, store, "m-status-done", "---\nstatus: done\nkind: map\n---\n", done)

		rigs.WriteEpicFiles(t, store, "m-ignored", stepMD("resolve-tickets"), open)
		rigs.WriteEpicFiles(t, store, "m-ignored-impl", "---\nstatus: open\n---\n", open)
		runGitIn(t, repo, "branch", "m-ignored-impl")
		if err := os.MkdirAll(filepath.Join(repo, "docs", "specs"), 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, filepath.Join(repo, "docs", "specs"), "m-ignored.md", "# spec\n")
		rigs.WriteEpicFiles(t, store, "loop", "---\nstatus: open\n---\n", open)
		servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	})

	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/epics")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	got := map[string]wire.EpicListRow{}
	for _, r := range rigs.GetEpicList(t, code, body).Epics {
		got[r.Name] = r
	}

	want := map[string]wire.ProgressStep{
		"m-no-field":    wire.ProgressChart,
		"m-bad-step":    wire.ProgressChart,
		"m-status-done": wire.ProgressDone,
		"m-ignored":     wire.ProgressResolveTickets,
		"loop":          "",
	}
	for _, s := range steps {
		want["m-"+string(s)] = s
	}
	for name, w := range want {
		r, ok := got[name]
		if !ok {
			t.Errorf("%s is missing from %s", name, body)
			continue
		}
		if r.ProgressStep != w {
			t.Errorf("%s = step %q, want %q", name, r.ProgressStep, w)
		}
	}
}

// handMergeEpic hand-merges the complete epic blocker into a temp repo while a
// map named mapName sits in the store, and returns a loader for the map once
// the server stamped the blocker's merged_at.
func handMergeEpic(t *testing.T, blocker, mapName string) func() tickets.Epic {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, blocker)
	writeCompleteEpic(t, store, blocker)
	if err := os.MkdirAll(filepath.Join(store, "proj", mapName), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Join(store, "proj", mapName), "ticket.md", "---\nkind: map\nstatus: open\n---\n# Map\n")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{})
	servertest.WriteEpicBlockedBy(t, store, "proj", "dep", blocker)
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	if res, err := h.Client.Queue.Add(context.Background(), "proj:dep/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	runGitIn(t, repo, "merge", "-q", "--ff-only", blocker)

	blockerPath := filepath.Join(store, "proj", blocker)
	waitUntil(t, "merged_at on "+blocker, func() bool { return !tickets.LoadEpicTicketMD(blockerPath).MergedAt.IsZero() })
	return func() tickets.Epic { return tickets.LoadEpicTicketMD(filepath.Join(store, "proj", mapName)) }
}

func TestMapDone_MergingTheImplEpicMarksItsMapDone(t *testing.T) {
	t.Parallel()
	load := handMergeEpic(t, "feat-impl", "feat")
	waitUntil(t, "map done", func() bool { m := load(); return m.Status == "done" && !m.MergedAt.IsZero() })
}

func TestMapDone_MergingAnotherEpicLeavesTheMapAlone(t *testing.T) {
	t.Parallel()
	load := handMergeEpic(t, "feat-other", "feat")
	time.Sleep(300 * time.Millisecond) // a negative: give a wrong stamp time to land
	if m := load(); m.Status != "open" || !m.MergedAt.IsZero() {
		t.Errorf("map status %q, merged_at %v, want untouched", m.Status, m.MergedAt)
	}
}
