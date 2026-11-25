package webflows

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/output"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// commonDir is the repository a checkout belongs to.
func commonDir(t *testing.T, dir string) string {
	t.Helper()
	p := gitIn(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	return filepath.Clean(p)
}

// assertUncommitted checks the edits every full worktree target holds.
func assertFullDirtyState(t *testing.T, dir string) {
	t.Helper()
	status := gitIn(t, dir, "status", "--porcelain", "--ignored", "-uall")
	for want, how := range map[string]string{
		spec.WorktreeDiffModified: " M ",
		spec.WorktreeDiffRenamed:  " M ",
		spec.WorktreeDiffDeleted:  " D ",
		spec.WorktreeDiffNew:      "?? ",
		spec.WorktreeDiffPNG:      "?? ",
		spec.WorktreeDiffIgnored:  "!! ",
	} {
		// gitIn trims the output, which drops the first line's leading space.
		if !strings.Contains(status, strings.TrimSpace(how)+" "+want) {
			t.Errorf("%s: status has no %q:\n%s", dir, how+want, status)
		}
	}
	if untracked := gitIn(t, dir, "ls-files", "-o", "--exclude-standard"); strings.Contains(untracked, spec.WorktreeDiffIgnored) {
		t.Errorf("%s: ignored file is listed as untracked:\n%s", dir, untracked)
	}
	if got := gitIn(t, dir, "log", "--diff-filter=R", "--name-status", "--format=", "-M", "main..HEAD"); !strings.Contains(got, spec.WorktreeDiffRenamedOld+"\t"+spec.WorktreeDiffRenamed) {
		t.Errorf("%s: no rename commit past main:\n%s", dir, got)
	}
}

func commitsPast(t *testing.T, dir, base string) int {
	t.Helper()
	return len(strings.Fields(gitIn(t, dir, "rev-list", base+"..HEAD")))
}

func TestWorktreeDiffScenario(t *testing.T) {
	t.Parallel()
	r := start(t, "worktree-diff")
	w := r.WorktreeDiff
	if w == nil {
		t.Fatal("no worktree_diff in the output")
	}
	repoCommon := commonDir(t, r.Repo())

	sessions, err := apiclient.New(server.SocketPath(r.StateDir)).Sessions.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// session finds a chat by id: its directory is the printed path.
	session := func(name string, target output.WorktreeTarget) {
		t.Helper()
		i := slices.IndexFunc(sessions, func(si wire.SessionInfo) bool { return si.ID == target.ID })
		if target.ID == "" || i < 0 {
			t.Errorf("%s: no session with id %q", name, target.ID)
			return
		}
		if sessions[i].Dir != target.Path {
			t.Errorf("%s: session dir = %q, want %q", name, sessions[i].Dir, target.Path)
		}
	}
	for name, target := range map[string]output.WorktreeTarget{"chat": w.Chat, "ticket": w.Ticket, "design": w.Design, "fix agent": w.FixAgent, "clone": w.Clone} {
		session(name, target)
	}

	// The worktrees belong to the project repo and hold commits past their base.
	for name, target := range map[string]output.WorktreeTarget{"chat": w.Chat, "ticket": w.Ticket, "stacked": w.Stacked, "design": w.Design} {
		if commonDir(t, target.Path) != repoCommon {
			t.Errorf("%s: %s is not a worktree of the project repo", name, target.Path)
		}
		if gitIn(t, target.Path, "status", "--porcelain") == "" {
			t.Errorf("%s: no uncommitted edits", name)
		}
	}
	assertFullDirtyState(t, w.Chat.Path)
	for name, target := range map[string]output.WorktreeTarget{"chat": w.Chat, "ticket": w.Ticket, "design": w.Design} {
		if n := commitsPast(t, target.Path, gitIn(t, target.Path, "merge-base", "HEAD", "main")); n == 0 {
			t.Errorf("%s: no commits past the trunk merge-base in %s\n%s", name, target.Path, gitIn(t, r.Repo(), "worktree", "list"))
		}
	}

	// The stacked ticket starts from the blocker's branch, not from the epic tip.
	blockerTip := gitIn(t, r.Repo(), "rev-parse", w.BlockerBranch)
	if got := gitIn(t, w.Stacked.Path, "merge-base", "HEAD", w.BlockerBranch); got != blockerTip {
		t.Errorf("stacked base = %s, want the blocker tip %s", got, blockerTip)
	}
	if n := commitsPast(t, w.Stacked.Path, blockerTip); n == 0 {
		t.Error("stacked: no commits past the blocker tip")
	}
	if n := commitsPast(t, w.Stacked.Path, "main"); n < 2 {
		t.Errorf("stacked: %d commits past main, want the blocker's before its own", n)
	}

	// The Fix agent runs in the trunk checkout: edits only.
	if w.FixAgent.Path != r.Repo() {
		t.Errorf("fix agent dir = %q, want the trunk checkout %q", w.FixAgent.Path, r.Repo())
	}
	if gitIn(t, w.FixAgent.Path, "status", "--porcelain") == "" {
		t.Error("fix agent: no uncommitted edits in the trunk checkout")
	}

	// The clone is a repository of its own.
	if commonDir(t, w.Clone.Path) == repoCommon {
		t.Errorf("clone shares the repo's git dir %s", repoCommon)
	}

	assertEdgeStates(t, r, w, sessions)

	// The unclaimed ticket has no worktree and no iteration branch.
	if w.Unclaimed == "" {
		t.Fatal("no unclaimed ticket address")
	}
	if out, _ := exec.Command("git", "-C", r.Repo(), "worktree", "list", "--porcelain").CombinedOutput(); strings.Contains(string(out), "wtd-idle") {
		t.Errorf("the unclaimed ticket has a worktree:\n%s", out)
	}
}
