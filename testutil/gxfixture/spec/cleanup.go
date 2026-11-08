package spec

import (
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// cleanupOld is old enough that no seeded branch tip counts as active work.
const cleanupOld = "2020-01-01T00:00:00Z"

// Cleanup seeds every case the Cleanup page groups: a merged worktree, a
// cherry-picked iteration branch, an iteration branch with an unmatched
// commit, a clean idle unmerged worktree, a dirty worktree, a squash-merged
// branch, a done merged epic and a done unmerged epic.
var Cleanup = scenario{
	Epics: []epic{
		{project, "cleanup-epic", []ticket{
			{"01", "picked", servertest.TicketOpts{Status: "done"}},
			{"02", "unmatched", servertest.TicketOpts{Status: "done"}},
		}},
		{project, "merged-epic", []ticket{{"01", "built", servertest.TicketOpts{Status: "done"}}}},
		{project, "unmerged-epic", []ticket{{"01", "built", servertest.TicketOpts{Status: "done"}}}},
	},
	EpicMeta: map[string]string{
		project + ":merged-epic": "status: open\nmerged_at: 2026-01-01T10:00:00Z\n",
	},
	// The Start cleanup agent one-off is the first ticket in one-offs. Its fake
	// asks at the /gx-cleanup confirm instead of finishing, so the Answer card
	// has a question to show.
	Behaviour: map[string]string{project + ":one-offs/01": agentfake.BehaviourAsk},
	GitSetup:  cleanupGit,
}

// oldCommit commits file on a new branch from main with an old committer
// date, then returns to main. The rebase is what rewrites the committer date,
// which the scan's activity window reads; the hook cannot set the environment.
func oldCommit(git Git, repo, branch, file string) error {
	if err := git(repo, "checkout", "-b", branch, "main"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repo, file), []byte(branch), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"add", file},
		{"commit", "-m", file, "--date", cleanupOld},
		{"rebase", "--force-rebase", "--committer-date-is-author-date", "main"},
		{"checkout", "main"},
	} {
		if err := git(repo, args...); err != nil {
			return err
		}
	}
	return nil
}

func cleanupGit(git Git, repo string) error {
	worktree := func(branch string) error {
		return git(repo, "worktree", "add", filepath.Join(repo, ".worktrees", branch), branch)
	}
	for branch, file := range map[string]string{
		"merged-wt":                       "merged.txt",
		"ralph-loop/cleanup-epic-item-01": "picked.txt",
		"ralph-loop/cleanup-epic-item-02": "unmatched.txt",
		"idle-wt":                         "idle.txt",
		"dirty-wt":                        "dirty.txt",
		"squash-one":                      "squash1.txt",
		"unmerged-epic":                   "epic.txt",
	} {
		if err := oldCommit(git, repo, branch, file); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		{"merge", "--ff-only", "merged-wt"},
		{"cherry-pick", "ralph-loop/cleanup-epic-item-01"},
		{"merge", "--squash", "squash-one"},
		{"commit", "-m", "squash"},
	} {
		if err := git(repo, args...); err != nil {
			return err
		}
	}
	for _, branch := range []string{"merged-wt", "idle-wt", "dirty-wt"} {
		if err := worktree(branch); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(repo, ".worktrees", "dirty-wt", "scratch.txt"), []byte("x"), 0o644)
}
