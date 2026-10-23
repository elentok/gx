package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestEpicList_MapProgress(t *testing.T) {
	repo := testutil.TempRepo(t)
	mapMD := func(front, body string) string { return "---\nstatus: open\nkind: map\n" + front + "---\n" + body }
	writeSpec := func(name string) {
		if err := os.MkdirAll(filepath.Join(repo, "docs", "specs"), 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, filepath.Join(repo, "docs", "specs"), name+".md", "# spec\n")
	}
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}
	unchecked := "\n## Review findings\n\n- [ ] a\n- [x] b\n"
	checked := "\n## Review findings\n\n- [x] a\n- [x] b\n"

	h, base := startEpicDetail(t, func(store string) {
		writeEpicFiles(t, store, "m-chart", mapMD("destination_unconfirmed: true\n", ""), done)
		writeEpicFiles(t, store, "m-resolve-open", mapMD("", ""), open)
		writeEpicFiles(t, store, "m-resolve-nys", mapMD("", "\n## Not yet specified\n\n- the schema\n"), done)
		writeEpicFiles(t, store, "m-spec", mapMD("", "\n## Not yet specified\n"), done)
		writeEpicFiles(t, store, "m-review", mapMD("", ""), done)
		writeSpec("m-review")
		writeEpicFiles(t, store, "m-grill", mapMD("", unchecked), done)
		writeSpec("m-grill")
		writeEpicFiles(t, store, "m-create", mapMD("", checked), done)
		writeSpec("m-create")
		writeEpicFiles(t, store, "m-create-impl", "---\nstatus: draft\n---\n", open)
		writeEpicFiles(t, store, "m-branch", mapMD("", checked), done)
		writeSpec("m-branch")
		writeEpicFiles(t, store, "m-branch-impl", "---\nstatus: open\n---\n", open)
		writeEpicFiles(t, store, "m-merged", mapMD("", checked), done)
		writeEpicFiles(t, store, "m-merged-impl", "---\nstatus: open\n---\n", open)
		runGitIn(t, repo, "branch", "m-merged-impl")
		writeEpicFiles(t, store, "m-status-done", "---\nstatus: done\nkind: map\n---\n", done)
		writeEpicFiles(t, store, "m-by-hand", mapMD("completed_at: 2026-01-01T11:00:00Z\n", ""), done)
		writeEpicFiles(t, store, "loop", "---\nstatus: open\n---\n", open)
		servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/epics")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	got := map[string]server.EpicListRow{}
	for _, r := range getEpicList(t, code, body).Epics {
		got[r.Name] = r
	}

	want := map[string]struct {
		step  server.ProgressStep
		phase server.ProgressPhase
	}{
		"m-chart":        {server.ProgressChart, server.ProgressPhaseMap},
		"m-resolve-open": {server.ProgressResolveTickets, server.ProgressPhaseMap},
		"m-resolve-nys":  {server.ProgressResolveTickets, server.ProgressPhaseMap},
		"m-spec":         {server.ProgressSpec, server.ProgressPhaseReview},
		"m-review":       {server.ProgressReview, server.ProgressPhaseReview},
		"m-grill":        {server.ProgressGrillFindings, server.ProgressPhaseReview},
		"m-create":       {server.ProgressCreateTickets, server.ProgressPhaseReview},
		"m-branch":       {server.ProgressBranch, server.ProgressPhaseReview},
		"m-merged":       {server.ProgressDone, server.ProgressPhaseImplement},
		"m-status-done":  {server.ProgressDone, server.ProgressPhaseImplement},
		"m-by-hand":      {server.ProgressDone, server.ProgressPhaseImplement},
		"loop":           {"", ""},
	}
	for name, w := range want {
		r, ok := got[name]
		if !ok {
			t.Errorf("%s is missing from %s", name, body)
			continue
		}
		if r.ProgressStep != w.step || r.ProgressPhase != w.phase {
			t.Errorf("%s = step %q phase %q, want %q %q", name, r.ProgressStep, r.ProgressPhase, w.step, w.phase)
		}
	}
}
