package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/output"
)

// WorktreeDiffCapFiles is how many untracked files the capped worktree holds:
// one more than the server lists.
const WorktreeDiffCapFiles = 501

// edgeWorktrees makes the plain worktrees of the edge states, off main. g is
// the project repo's git.
func edgeWorktrees(git Git, g *epicDiffGit, root string) error {
	add := func(dir string) *epicDiffGit {
		g.run("worktree", "add", "-q", "-b", dir, filepath.Join(root, dir), "main")
		return &epicDiffGit{git: git, repo: filepath.Join(root, dir), err: g.err}
	}
	capped := add(wtdCappedDir)
	capped.write("src/capped.txt", "tracked change\n")
	capped.commit("Change a tracked file")
	for i := range WorktreeDiffCapFiles {
		capped.write(fmt.Sprintf("untracked/f%03d.txt", i), fmt.Sprintf("file %d\n", i))
	}
	big := add(wtdOversizeDir)
	if big.err == nil {
		// Line-shaped text, so only its size keeps it out of the diff.
		line := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde\n"
		data := make([]byte, 0, rangediff.MaxBlobBytes+1)
		for len(data) < rangediff.MaxBlobBytes+1 {
			data = append(data, line...)
		}
		big.write("big/huge.txt", string(data[:rangediff.MaxBlobBytes+1]))
		big.write("big/small.txt", "small\n")
	}
	clean := add(wtdCleanDir)
	return firstErr(g.err, capped.err, big.err, clean.err)
}

// waitFile polls until path exists.
func waitFile(path string) error {
	for deadline := time.Now().Add(wtdWaitLimit); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	return fmt.Errorf("%s did not appear", path)
}

// edgePostStart seeds the targets that need the server: the Queue agents, the
// Triage and the paused tickets. wtDir is the directory the iteration worktrees live in.
func edgePostStart(env PostStartEnv, out *output.WorktreeDiffOutput, wtDir string) error {
	target := func(addr string, si wire.SessionInfo) output.WorktreeTarget {
		return output.WorktreeTarget{ID: si.ID, Path: si.Dir, Address: addr}
	}
	si, err := env.waitSession("triage", func(si wire.SessionInfo) bool { return si.Flow == "triage" })
	if err != nil {
		return err
	}
	out.Triage = target(project+":"+"triage", si)

	// The asking agent stays up: its ticket pauses with the run still tracked.
	askAddr := wtdAddr(wtdAsk, "01")
	si, err = env.waitSession(askAddr, func(si wire.SessionInfo) bool { return si.Queue && si.Ticket == askAddr })
	if err != nil {
		return err
	}
	if err := waitFile(filepath.Join(si.Dir, agentfake.UncommittedFile)); err != nil {
		return err
	}
	out.PausedRun = target(askAddr, si)

	// A paused ticket with no run entry has only its iteration worktree: the
	// base is derived from its blocker's branch, or from the epic's feature branch.
	out.FeatureBranch = wtdPause
	out.PausedFeature = output.WorktreeTarget{
		Path: worktree.IterationWorktreePath(wtDir, wtdPause, "01"), Address: wtdAddr(wtdPause, "01"),
	}
	if err := env.Git(env.Repo, "branch", wtdPause, "main"); err != nil {
		return err
	}
	if err := env.Git(env.Repo, "worktree", "add", "-q", "-b", worktree.IterationBranch(wtdPause, "01"), out.PausedFeature.Path, wtdPause); err != nil {
		return err
	}
	if err := commitAndEdit(env.Git, out.PausedFeature.Path, wtdPause); err != nil {
		return err
	}

	out.PausedBlockerBranch = worktree.IterationBranch(wtdPauseStack, "01")
	blockerDir := filepath.Join(wtDir, "wtd-pstack-blocker")
	for _, args := range [][]string{
		{"branch", wtdPauseStack, "main"},
		{"worktree", "add", "-q", "-b", out.PausedBlockerBranch, blockerDir, wtdPauseStack},
	} {
		if err := env.Git(env.Repo, args...); err != nil {
			return err
		}
	}
	if err := commitAndEdit(env.Git, blockerDir, wtdPauseStack+"-01"); err != nil {
		return err
	}
	// The blocker keeps its branch only.
	if err := env.Git(env.Repo, "worktree", "remove", "--force", blockerDir); err != nil {
		return err
	}
	out.PausedStacked = output.WorktreeTarget{
		Path: worktree.IterationWorktreePath(wtDir, wtdPauseStack, "02"), Address: wtdAddr(wtdPauseStack, "02"),
	}
	if err := env.Git(env.Repo, "worktree", "add", "-q", "-b", worktree.IterationBranch(wtdPauseStack, "02"), out.PausedStacked.Path, out.PausedBlockerBranch); err != nil {
		return err
	}
	return commitAndEdit(env.Git, out.PausedStacked.Path, wtdPauseStack+"-02")
}
