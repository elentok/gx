package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// openDesignWorktree opens the map before Branch and returns its design
// worktree and session id; the design branch has one commit when commit is set.
func (f designsFixture) openDesignWorktree(t *testing.T, commit bool) (dir, id string) {
	t.Helper()
	res := f.open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	dir, _ = readMeta(t, f.h, res.Session.ID)["dir"].(string)
	// The open's stamp write is rescanned after a debounce; a rescan that lands
	// after the test moves the step would take the new step as its baseline.
	if err := f.h.Client.TicketChanged(f.ctx, "proj:map"); err != nil {
		t.Fatal(err)
	}
	if commit {
		if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte("design\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitOut(t, dir, "add", "design.md")
		gitOut(t, dir, "commit", "-m", "design work")
	}
	return dir, res.Session.ID
}

func (f designsFixture) stop(t *testing.T, id string) {
	t.Helper()
	if _, err := f.h.Client.StopSession(f.ctx, id); err != nil {
		t.Fatal(err)
	}
}

func (f designsFixture) serverLog(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(server.LogPath(f.h.StateDir))
	return string(b)
}

func (f designsFixture) waitForCleanupWarning(t *testing.T) {
	t.Helper()
	waitFor(t, func() bool {
		log := f.serverLog(t)
		return strings.Contains(log, "design cleanup kept") && strings.Contains(log, "proj:map")
	})
}

func (f designsFixture) designGone(t *testing.T, dir string) bool {
	t.Helper()
	_, err := os.Stat(dir)
	return os.IsNotExist(err) && gitOut(t, f.repo, "branch", "--list", "map-design") == ""
}

func (f designsFixture) designKept(t *testing.T, dir string) bool {
	t.Helper()
	_, err := os.Stat(dir)
	return err == nil && gitOut(t, f.repo, "branch", "--list", "map-design") != ""
}

func TestDesignCleanup_SessionExitAfterBranchRemovesWorktreeAndBranch(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	f.stop(t, id)
	waitFor(t, func() bool { return f.designGone(t, dir) })
}

func TestDesignCleanup_RebasedImplBranchCountsAsMerged(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	if err := os.WriteFile(filepath.Join(f.repo, "t.txt"), []byte("trunk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.repo, "add", "t.txt")
	gitOut(t, f.repo, "commit", "-m", "trunk change")
	trunk := gitOut(t, f.repo, "rev-parse", "--abbrev-ref", "HEAD")
	gitOut(t, f.repo, "checkout", "-b", "map-impl")
	gitOut(t, f.repo, "cherry-pick", "map-design")
	gitOut(t, f.repo, "checkout", trunk)
	f.stop(t, id)
	waitFor(t, func() bool { return f.designGone(t, dir) })
}

func TestDesignCleanup_DirtyWorktreeKeepsBothAndWarns(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	if err := os.WriteFile(filepath.Join(dir, "wip.md"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.stop(t, id)
	f.waitForCleanupWarning(t)
	if !f.designKept(t, dir) {
		t.Errorf("design worktree or branch removed")
	}
}

func TestDesignCleanup_UnmergedDesignCommitKeepsBothAndWarns(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, false)
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	if err := os.WriteFile(filepath.Join(dir, "late.md"), []byte("late\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "add", "late.md")
	gitOut(t, dir, "commit", "-m", "late design work")
	f.stop(t, id)
	f.waitForCleanupWarning(t)
	if !f.designKept(t, dir) {
		t.Errorf("design worktree or branch removed")
	}
}

func TestDesignCleanup_GitErrorKeepsBothAndWarns(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	ticketMD := filepath.Join(f.h.TicketStore, "proj", "map", "ticket.md")
	if err := os.WriteFile(ticketMD, []byte("---\nkind: map\ndesign_step: branch\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.stop(t, id)
	f.waitForCleanupWarning(t)
	if !f.designKept(t, dir) {
		t.Errorf("design worktree or branch removed")
	}
}

func TestDesignCleanup_DesignOpenAfterBranchRunsTheCheck(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	f.stop(t, id)
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	if !f.designKept(t, dir) {
		t.Fatalf("design worktree or branch removed before the open")
	}
	if res := f.open(t, "proj:map", false); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	waitFor(t, func() bool { return f.designGone(t, dir) })
}

func (f designsFixture) setDesignStepBranch(t *testing.T) {
	t.Helper()
	ticketMD := filepath.Join(f.h.TicketStore, "proj", "map", "ticket.md")
	// Rename, not WriteFile: a truncating write can be rescanned half-written
	// as an empty step, and the next rescan then sees a fresh transition.
	tmp := ticketMD + ".tmp"
	if err := os.WriteFile(tmp, []byte("---\nkind: map\ndesign_step: branch\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, ticketMD); err != nil {
		t.Fatal(err)
	}
}

func TestDesignCleanup_StepMovingToBranchRemovesWithoutSessionExit(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	f.stop(t, id)
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	if !f.designKept(t, dir) {
		t.Fatalf("design worktree or branch removed before the step moved")
	}
	f.setDesignStepBranch(t)
	waitFor(t, func() bool { return f.designGone(t, dir) })
}

func TestDesignCleanup_StepMovingToBranchWithDirtyWorktreeKeepsBothAndWarns(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	f.stop(t, id)
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	if err := os.WriteFile(filepath.Join(dir, "wip.md"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.setDesignStepBranch(t)
	f.waitForCleanupWarning(t)
	if !f.designKept(t, dir) {
		t.Errorf("design worktree or branch removed")
	}
}

func TestDesignCleanup_RescanWithoutStepChangeDoesNotCheckAgain(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, id := f.openDesignWorktree(t, true)
	f.stop(t, id)
	// The session-exit cleanup runs once the stop is saved. Let it finish
	// before the branch and the edit exist, or it could be the one that warns.
	waitFor(t, func() bool { return len(f.live(t)) == 0 })
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	wip := filepath.Join(dir, "wip.md")
	if err := os.WriteFile(wip, []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.setDesignStepBranch(t)
	f.waitForCleanupWarning(t)
	if err := os.Remove(wip); err != nil {
		t.Fatal(err)
	}
	f.setDesignStepBranch(t) // rescans with the step unchanged
	time.Sleep(500 * time.Millisecond)
	if !f.designKept(t, dir) {
		t.Errorf("an unchanged step ran the cleanup check again")
	}
}

func TestDesignCleanup_LiveSessionKeepsTheWorktree(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	dir, _ := f.openDesignWorktree(t, true)
	gitOut(t, f.repo, "branch", "map-impl", "map-design")
	if res := f.open(t, "proj:map", false); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if !f.designKept(t, dir) {
		t.Errorf("design worktree or branch removed under a live session")
	}
}
