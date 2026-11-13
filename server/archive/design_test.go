package archive_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// startPair is a store whose Design foo is at step, with a done foo-impl epic
// unless noImpl.
func startPair(t *testing.T, step string, noImpl bool, opts ...func(*server.Config)) rig {
	t.Helper()
	store := t.TempDir()
	servertest.WriteCompleteEpic(t, store, "foo")
	md := "---\nkind: map\ndesign_step: " + step + "\n---\n"
	if err := os.WriteFile(filepath.Join(store, "proj", "foo", "ticket.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if !noImpl {
		servertest.WriteCompleteEpic(t, store, "foo-impl")
	}
	return rig{rigs.StartDesignsIn(t, store, opts...), store}
}

func TestArchiveDesign_PairArchivesAsOneRow(t *testing.T) {
	r := startPair(t, "done", false)
	got := r.archive(t, false, false, "proj:foo")[0]
	if got.Status != wire.TargetDone || got.Target != "proj:foo" {
		t.Fatalf("archive = %+v", got)
	}
	if a := r.archived(t); !a["foo"] || !a["foo-impl"] {
		t.Errorf("archived = %v, want foo and foo-impl", a)
	}
	design, impl := r.epic(t, "foo"), r.epic(t, "foo-impl")
	if design.ArchivedAt.IsZero() || !design.ArchivedAt.Equal(impl.ArchivedAt) || design.ArchivedWorkspace != impl.ArchivedWorkspace {
		t.Errorf("stamps design = %v %q, impl = %v %q, want identical", design.ArchivedAt, design.ArchivedWorkspace, impl.ArchivedAt, impl.ArchivedWorkspace)
	}
}

func TestArchiveDesign_BeforeDoneIsNotFinished(t *testing.T) {
	r := startPair(t, "chart", false)
	for _, addr := range []string{"proj:foo", "proj:foo-impl"} {
		if got := r.archive(t, true, false, addr)[0]; got.Reason != wire.ReasonArchiveNotFinished {
			t.Errorf("%s = %+v, want not-finished", addr, got)
		}
	}
	if len(r.archived(t)) != 0 {
		t.Fatal("a refused row moved a folder")
	}
}

func TestArchiveDesign_WithoutImplArchivesAlone(t *testing.T) {
	r := startPair(t, "done", true)
	if got := r.archive(t, false, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v", got)
	}
	if a := r.archived(t); !a["foo"] || len(a) != 1 {
		t.Errorf("archived = %v, want only foo", a)
	}
}

func TestArchiveDesign_ImplAddressResolvesToTheDesignRowAndKeepsItsKey(t *testing.T) {
	r := startPair(t, "done", false)
	if err := os.MkdirAll(filepath.Join(r.store, "proj", ".archive", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	dry := r.archive(t, false, true, "proj:foo-impl")[0]
	if dry.Target != "proj:foo-impl" || dry.NewAddress != "proj:foo-2-impl" {
		t.Fatalf("dry run = %+v, want target proj:foo-impl and new_address proj:foo-2-impl", dry)
	}
	got := r.archive(t, false, false, "proj:foo-impl")[0]
	if got.Status != wire.TargetDone || got.Target != "proj:foo-impl" {
		t.Fatalf("archive = %+v", got)
	}
	if a := r.archived(t); !a["foo-2"] || !a["foo-2-impl"] {
		t.Errorf("archived = %v, want foo-2 and foo-2-impl", a)
	}
	if e := r.epic(t, "foo-2"); e.BranchName != "foo" {
		t.Errorf("branch_name = %q, want foo on the Design", e.BranchName)
	}
}

func TestArchiveDesign_ClashOfImplNameAloneTakesTheNextSuffix(t *testing.T) {
	r := startPair(t, "done", false)
	if err := os.MkdirAll(filepath.Join(r.store, "proj", ".archive", "foo-impl"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := r.archive(t, false, false, "proj:foo")[0]
	if got.NewAddress != "proj:foo-2" {
		t.Fatalf("archive = %+v, want new_address proj:foo-2", got)
	}
}

func TestArchiveDesign_ImplNotDoneIsRefused(t *testing.T) {
	r := startPair(t, "done", true)
	servertest.WriteTicket(t, r.store, "proj", "foo-impl", "01", "first", "")
	got := r.archive(t, true, false, "proj:foo")[0]
	if got.Reason != wire.ReasonArchiveImplNotArchivable {
		t.Fatalf("archive = %+v, want impl-not-archivable", got)
	}
	if len(r.archived(t)) != 0 {
		t.Fatal("a refused row moved a folder")
	}
}

// withDesignWorktree gives the repo a foo-design worktree and branch, and a
// foo-impl branch that is on trunk unless diverged.
func (r rig) withDesignWorktree(t *testing.T, diverged bool) string {
	t.Helper()
	// The startup rescan cleans a Design that is already done in the background;
	// wait for it, or it would race the worktree made below.
	servertest.WaitUntil(t, "startup design cleanup", func() bool {
		b, _ := os.ReadFile(server.LogPath(r.H.StateDir))
		return strings.Contains(string(b), "design cleanup")
	})
	wt := filepath.Join(r.Repo, ".worktrees", "foo-design")
	servertest.RunGitIn(t, r.Repo, "worktree", "add", "-q", "-b", "foo-design", wt)
	if diverged {
		servertest.CommitOnBranch(t, r.Repo, "foo-impl")
	} else {
		servertest.RunGitIn(t, r.Repo, "branch", "foo-impl")
	}
	return wt
}

func (r rig) branchExists(t *testing.T, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", r.Repo, "rev-parse", "--verify", "-q", "refs/heads/"+branch)
	return cmd.Run() == nil
}

func TestArchiveDesign_OnTrunkPairLosesItsDesignWorktreeAndBranch(t *testing.T) {
	r := startPair(t, "done", false)
	wt := r.withDesignWorktree(t, false)

	if got := r.archive(t, false, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v", got)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("design worktree kept (stat err %v)", err)
	}
	if r.branchExists(t, "foo-design") {
		t.Error("design branch kept")
	}
}

func TestArchiveDesign_OnTrunkPairWithLiveDesignSessionLosesItsDesignWorktreeAndBranch(t *testing.T) {
	// The exit-driven cleanup is held back, so only Archive's own can remove them in time.
	r := startPair(t, "done", false, func(c *server.Config) { c.SessionExitCleanupDelay = 5 * time.Second })
	wt := r.withDesignWorktree(t, false)
	if res := r.Open(t, "proj:foo", false); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}

	if got := r.archive(t, true, false, "proj:foo")[0]; got.Status != wire.TargetDone || len(got.Sessions) == 0 {
		t.Fatalf("archive = %+v, want done after stopping the Design Session", got)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("design worktree kept (stat err %v)", err)
	}
	if r.branchExists(t, "foo-design") {
		t.Error("design branch kept")
	}
}

func TestArchiveDesign_NotOnTrunkPairKeepsItsDesignWorktreeAndBranch(t *testing.T) {
	r := startPair(t, "done", false)
	wt := r.withDesignWorktree(t, true)

	if got := r.archive(t, false, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v", got)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("design worktree removed: %v", err)
	}
	if !r.branchExists(t, "foo-design") {
		t.Error("design branch removed")
	}
}

func TestArchiveDesign_SecondRenameFailureRollsBack(t *testing.T) {
	r := startPair(t, "done", false)
	impl := filepath.Join(r.store, "proj", "foo-impl")
	// A directory cannot change parent without write access to itself.
	if err := os.Chmod(impl, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(impl, 0o755) })

	got := r.archive(t, true, false, "proj:foo")[0]
	if got.Reason != wire.ReasonArchiveRenameFailed {
		t.Fatalf("archive = %+v, want rename-failed", got)
	}
	if len(r.archived(t)) != 0 {
		t.Errorf("archived = %v, want the Design rolled back", r.archived(t))
	}
	if _, err := os.Stat(filepath.Join(r.store, "proj", "foo", "ticket.md")); err != nil {
		t.Errorf("Design not restored: %v", err)
	}
}

func TestArchiveDesign_StuckBeforeDoneArchivesOnceItsImplMerged(t *testing.T) {
	r := startPair(t, "chart", false)
	if err := tickets.StampEpicFields(filepath.Join(r.store, "proj", "foo-impl"), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	got := r.archive(t, false, false, "proj:foo")[0]
	if got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v, want done", got)
	}
	if a := r.archived(t); !a["foo"] || !a["foo-impl"] {
		t.Errorf("archived = %v, want foo and foo-impl", a)
	}
}

func TestArchiveDesign_StuckBeforeDoneWithoutImplArchivesOnceMerged(t *testing.T) {
	r := startPair(t, "chart", true)
	if err := tickets.StampEpicFields(filepath.Join(r.store, "proj", "foo"), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if got := r.archive(t, false, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v, want done", got)
	}
}

func TestArchiveDesign_MergedMapWithoutTicketsOrImplArchives(t *testing.T) {
	r := startPair(t, "branch", true)
	if err := os.RemoveAll(filepath.Join(r.store, "proj", "foo", "issues")); err != nil {
		t.Fatal(err)
	}
	if err := tickets.StampEpicFields(filepath.Join(r.store, "proj", "foo"), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if got := r.archive(t, false, false, "proj:foo")[0]; got.Status != wire.TargetDone {
		t.Fatalf("archive = %+v, want done", got)
	}
}

func TestArchiveDesign_DryRunWithoutImplReportsTrunkFromTheMap(t *testing.T) {
	for _, merged := range []bool{true, false} {
		r := startPair(t, "done", true)
		if merged {
			if err := tickets.StampEpicFields(filepath.Join(r.store, "proj", "foo"), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
				t.Fatal(err)
			}
		}
		got := r.archive(t, false, true, "proj:foo")[0]
		if got.OnTrunk == nil || *got.OnTrunk != merged {
			t.Errorf("merged=%v: on_trunk = %v", merged, got.OnTrunk)
		}
	}
}
