package epics_test

import (
	"github.com/elentok/gx/server/servertest/rigs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestProgressStepsMatchTickets(t *testing.T) {
	t.Parallel()
	got := []wire.ProgressStep{
		wire.ProgressChart, wire.ProgressResolveTickets, wire.ProgressSpec, wire.ProgressReview,
		wire.ProgressGrillFindings, wire.ProgressCreateTickets, wire.ProgressBranch, wire.ProgressDone,
	}
	if !slices.Equal(got, tickets.DesignSteps) {
		t.Fatalf("server.Progress* = %v, want tickets.DesignSteps = %v", got, tickets.DesignSteps)
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
