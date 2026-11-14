package epics_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

const (
	rangeBaseSHA = "1111111111111111111111111111111111111111"
	rangeTipSHA  = "2222222222222222222222222222222222222222"
)

// viewDiffRig is a project over a temp repo, started once build has run. extra
// is raw project.json members; "landing" on-done makes a blocker's branch the
// base of its dependents, which on-landed (the default) does not.
func viewDiffRig(t *testing.T, extra string, build func(store, repo string)) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	build(store, repo)
	if err := servertest.WriteProjectRepo(store, "proj", repo, extra); err != nil {
		t.Fatal(err)
	}
	return servertest.StartWithStore(t, store), store
}

func stampEpic(t *testing.T, store, epic string, fields map[string]any) {
	t.Helper()
	if err := tickets.StampEpicFields(filepath.Join(store, "proj", epic), fields); err != nil {
		t.Fatal(err)
	}
}

// mergedFields is the stamp of a merged epic; no base leaves the range unstamped.
func mergedFields(base, tip string) map[string]any {
	f := map[string]any{"merged_at": time.Now().UTC()}
	if base != "" {
		f["range_base"], f["range_tip"] = base, tip
	}
	return f
}

// setDone marks the epic's ticket 01 done, optionally commitless.
func setDone(t *testing.T, store, epic string, commitless bool) {
	t.Helper()
	p := filepath.Join(store, "proj", epic, "issues", "01-t.md")
	body := strings.Replace(servertest.ReadFile(t, p), "status: open", "status: done", 1)
	if commitless {
		body = strings.Replace(body, "status: done", "status: done\ncommitless: true", 1)
	}
	testutil.WriteFile(t, filepath.Dir(p), filepath.Base(p), body)
}

type viewDiffCase struct {
	epic         string
	ahead, shows bool
	base, tip    string
}

func checkEpicDetail(t *testing.T, h *servertest.Harness, archived bool, c viewDiffCase) {
	t.Helper()
	get := h.Client.Reads.EpicDetail
	if archived {
		get = h.Client.Reads.ArchivedEpicDetail
	}
	d, err := get(context.Background(), "proj:"+c.epic)
	if err != nil {
		t.Fatalf("%s: %v", c.epic, err)
	}
	if d.HasCommitsAhead != c.ahead || d.ShowsViewDiff != c.shows || d.DiffBase != c.base || d.DiffTip != c.tip {
		t.Errorf("%s: got ahead=%v shows=%v refs %q..%q, want ahead=%v shows=%v refs %q..%q",
			c.epic, d.HasCommitsAhead, d.ShowsViewDiff, d.DiffBase, d.DiffTip, c.ahead, c.shows, c.base, c.tip)
	}
}

func TestEpicDetail_ViewDiffUnmerged(t *testing.T) {
	t.Parallel()
	h, _ := viewDiffRig(t, `,"landing":"on-done"`, func(store, repo string) {
		for _, e := range []string{"bare", "ahead", "after-merge-blocker"} {
			servertest.WriteTicketWith(t, store, "proj", e, "01", "t", servertest.TicketOpts{})
		}
		servertest.WriteTicketWith(t, store, "proj", "stacked", "01", "t", servertest.TicketOpts{BlockedBy: []string{"ahead/01"}})
		servertest.WriteTicketWith(t, store, "proj", "after-merge", "01", "t", servertest.TicketOpts{BlockedBy: []string{"after-merge-blocker/01"}})
		servertest.WriteTicketWith(t, store, "proj", "explicit", "01", "t", servertest.TicketOpts{Base: "ahead"})
		stampEpic(t, store, "after-merge-blocker", mergedFields(rangeBaseSHA, rangeTipSHA))
		for _, b := range []string{"ahead", "stacked", "after-merge", "explicit"} {
			commitOnBranch(t, repo, b)
		}
	})
	for _, c := range []viewDiffCase{
		{epic: "bare"},
		{epic: "ahead", ahead: true, shows: true, base: "main", tip: "ahead"},
		{epic: "stacked", ahead: true, shows: true, base: "ahead", tip: "stacked"},
		{epic: "after-merge", ahead: true, shows: true, base: "main", tip: "after-merge"},
		{epic: "explicit", ahead: true, shows: true, base: "ahead", tip: "explicit"},
	} {
		checkEpicDetail(t, h, false, c)
	}
}

func TestEpicDetail_ViewDiffMerged(t *testing.T) {
	t.Parallel()
	h, _ := viewDiffRig(t, "", func(store, _ string) {
		for _, e := range []string{"stamped", "unstamped", "empty"} {
			servertest.WriteTicketWith(t, store, "proj", e, "01", "t", servertest.TicketOpts{})
		}
		stampEpic(t, store, "stamped", mergedFields(rangeBaseSHA, rangeTipSHA))
		stampEpic(t, store, "unstamped", mergedFields("", ""))
		stampEpic(t, store, "empty", mergedFields(rangeBaseSHA, rangeBaseSHA))
	})
	for _, c := range []viewDiffCase{
		{epic: "stamped", shows: true, base: rangeBaseSHA, tip: rangeTipSHA},
		{epic: "unstamped", shows: true},
		{epic: "empty"},
	} {
		checkEpicDetail(t, h, false, c)
	}
}

func TestArchivedEpicDetail_ViewDiff(t *testing.T) {
	t.Parallel()
	h, _ := viewDiffRig(t, "", func(store, _ string) {
		for _, e := range []string{"merged", "unmerged"} {
			servertest.WriteTicketWith(t, store, "proj", e, "01", "t", servertest.TicketOpts{})
		}
		stampEpic(t, store, "merged", mergedFields(rangeBaseSHA, rangeTipSHA))
		archive := tickets.ArchiveDir(filepath.Join(store, "proj"))
		if err := os.MkdirAll(archive, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, e := range []string{"merged", "unmerged"} {
			if err := os.Rename(filepath.Join(store, "proj", e), filepath.Join(archive, e)); err != nil {
				t.Fatal(err)
			}
		}
	})
	checkEpicDetail(t, h, true, viewDiffCase{epic: "merged", shows: true, base: rangeBaseSHA, tip: rangeTipSHA})
	checkEpicDetail(t, h, true, viewDiffCase{epic: "unmerged"})
}

func TestEpicDetail_MapEpicNeverShowsViewDiff(t *testing.T) {
	t.Parallel()
	h, _ := viewDiffRig(t, "", func(store, _ string) {
		dir := filepath.Join(store, "proj", "design")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, dir, "ticket.md", "---\nkind: map\nstatus: open\nmerged_at: 2026-01-01T00:00:00Z\n---\n# Map\n")
	})
	checkEpicDetail(t, h, false, viewDiffCase{epic: "design"})
}

func TestEpicDetail_NoRepoReturnsFalse(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic", "01", "t", servertest.TicketOpts{})
	h := servertest.StartWithStore(t, store)
	checkEpicDetail(t, h, false, viewDiffCase{epic: "epic"})
}

func TestDetail_ShowsViewDiff(t *testing.T) {
	t.Parallel()
	h, _ := viewDiffRig(t, "", func(store, repo string) {
		for _, e := range []string{"ahead", "bare"} {
			servertest.WriteTicketWith(t, store, "proj", e, "01", "t", servertest.TicketOpts{})
			servertest.WriteTicketWith(t, store, "proj", e, "02", "u", servertest.TicketOpts{})
			setDone(t, store, e, false)
		}
		servertest.WriteTicketWith(t, store, "proj", "commitless", "01", "t", servertest.TicketOpts{})
		setDone(t, store, "commitless", true)
		commitOnBranch(t, repo, "ahead")
		commitOnBranch(t, repo, "commitless")
	})
	for addr, want := range map[string]bool{
		"proj:ahead/01":      true,
		"proj:ahead/02":      false,
		"proj:bare/01":       false,
		"proj:commitless/01": false,
	} {
		d, err := h.Client.Reads.Detail(context.Background(), addr)
		if err != nil {
			t.Fatal(err)
		}
		if d.ShowsViewDiff != want {
			t.Errorf("%s: ShowsViewDiff = %v, want %v", addr, d.ShowsViewDiff, want)
		}
		wantBase, wantTip := "", ""
		if want {
			wantBase, wantTip = "main", "ahead"
		}
		if d.DiffBase != wantBase || d.DiffTip != wantTip {
			t.Errorf("%s: diff %q..%q, want %q..%q", addr, d.DiffBase, d.DiffTip, wantBase, wantTip)
		}
	}
}
