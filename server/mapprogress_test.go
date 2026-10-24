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
	mapMD := func(front string) string { return "---\nstatus: open\nkind: map\n" + front + "---\n" }
	stepMD := func(step string) string { return mapMD("design_step: " + step + "\n") }
	open := map[string]string{"01-a.md": issueMD("01", "open", "", "A")}
	done := map[string]string{"01-a.md": issueMD("01", "done", "", "A")}

	steps := []server.ProgressStep{
		server.ProgressChart, server.ProgressResolveTickets, server.ProgressSpec, server.ProgressReview,
		server.ProgressGrillFindings, server.ProgressCreateTickets, server.ProgressBranch, server.ProgressDone,
	}

	h, base := startEpicDetail(t, func(store string) {
		for _, s := range steps {
			writeEpicFiles(t, store, "m-"+string(s), stepMD(string(s)), open)
		}
		writeEpicFiles(t, store, "m-no-field", mapMD(""), done)
		writeEpicFiles(t, store, "m-status-done", "---\nstatus: done\nkind: map\n---\n", done)
		// Files and refs that the old guess read must change nothing.
		writeEpicFiles(t, store, "m-ignored", stepMD("resolve-tickets"), open)
		writeEpicFiles(t, store, "m-ignored-impl", "---\nstatus: open\n---\n", open)
		runGitIn(t, repo, "branch", "m-ignored-impl")
		if err := os.MkdirAll(filepath.Join(repo, "docs", "specs"), 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, filepath.Join(repo, "docs", "specs"), "m-ignored.md", "# spec\n")
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

	want := map[string]server.ProgressStep{
		"m-no-field":    server.ProgressChart,
		"m-status-done": server.ProgressDone,
		"m-ignored":     server.ProgressResolveTickets,
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
