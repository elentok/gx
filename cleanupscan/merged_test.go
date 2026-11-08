package cleanupscan

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// branchWithCommits creates branch from main with one commit per file, then
// returns to main.
func branchWithCommits(t *testing.T, dir, branch string, files ...string) {
	t.Helper()
	testutil.MustGitExported(t, dir, "checkout", "-b", branch, "main")
	for _, f := range files {
		testutil.WriteFile(t, dir, f, f+"\n")
		testutil.CommitAll(t, dir, "add "+f)
	}
	testutil.MustGitExported(t, dir, "checkout", "main")
}

func TestCheckMerged(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		setup      func(t *testing.T, dir string)
		branch     string
		epic       string
		wantMerged bool
		wantMode   string
	}{
		{
			name: "cherry-picked",
			setup: func(t *testing.T, dir string) {
				branchWithCommits(t, dir, "feat", "a.txt", "b.txt")
				// Diverge main so the picks get fresh hashes.
				testutil.WriteFile(t, dir, "other.txt", "o\n")
				testutil.CommitAll(t, dir, "other")
				testutil.MustGitExported(t, dir, "cherry-pick", "main~1..feat")
			},
			wantMerged: true, wantMode: DeleteForce,
		},
		{
			name: "one extra unmatched commit",
			setup: func(t *testing.T, dir string) {
				branchWithCommits(t, dir, "feat", "a.txt")
				testutil.MustGitExported(t, dir, "cherry-pick", "feat")
				testutil.MustGitExported(t, dir, "checkout", "feat")
				testutil.WriteFile(t, dir, "extra.txt", "x\n")
				testutil.CommitAll(t, dir, "extra")
				testutil.MustGitExported(t, dir, "checkout", "main")
			},
			wantMerged: false, wantMode: DeleteNone,
		},
		{
			name: "ancestry",
			setup: func(t *testing.T, dir string) {
				branchWithCommits(t, dir, "feat", "a.txt")
				testutil.MustGitExported(t, dir, "merge", "--ff-only", "feat")
			},
			wantMerged: true, wantMode: DeleteSafe,
		},
		{
			name: "one-commit squash",
			setup: func(t *testing.T, dir string) {
				branchWithCommits(t, dir, "feat", "a.txt")
				testutil.MustGitExported(t, dir, "merge", "--squash", "feat")
				testutil.CommitAll(t, dir, "squash")
			},
			wantMerged: true, wantMode: DeleteForce,
		},
		{
			name: "multi-commit squash",
			setup: func(t *testing.T, dir string) {
				branchWithCommits(t, dir, "feat", "a.txt", "b.txt", "c.txt")
				testutil.MustGitExported(t, dir, "merge", "--squash", "feat")
				testutil.CommitAll(t, dir, "squash")
			},
			wantMerged: true, wantMode: DeleteForce,
		},
		{
			name: "not merged at all",
			setup: func(t *testing.T, dir string) {
				branchWithCommits(t, dir, "feat", "a.txt")
			},
			wantMerged: false, wantMode: DeleteNone,
		},
		{
			name: "iteration merged by ancestry into epic not on trunk",
			setup: func(t *testing.T, dir string) {
				branchWithCommits(t, dir, "ralph-loop/bugs-item-01", "a.txt")
				testutil.MustGitExported(t, dir, "branch", "bugs", "ralph-loop/bugs-item-01")
			},
			branch: "ralph-loop/bugs-item-01", epic: "bugs",
			wantMerged: true, wantMode: DeleteForce,
		},
	}

	for _, tc := range cases {
		for _, withWorktree := range []bool{false, true} {
			name := tc.name
			if withWorktree {
				name += " with worktree"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				dir := testutil.TempRepo(t)
				tc.setup(t, dir)
				branch := tc.branch
				if branch == "" {
					branch = "feat"
				}
				if withWorktree {
					testutil.MustGitExported(t, dir, "worktree", "add", filepath.Join(t.TempDir(), "wt"), branch)
				}

				repo := git.Repo{Root: dir, Trunk: "main"}
				got, err := CheckMerged(repo, branch, tc.epic)
				if err != nil {
					t.Fatalf("CheckMerged: %v", err)
				}
				if got.Merged != tc.wantMerged || got.DeleteMode != tc.wantMode {
					t.Fatalf("CheckMerged = %+v, want merged=%v mode=%q", got, tc.wantMerged, tc.wantMode)
				}
				if tc.wantMode == DeleteNone {
					return
				}
				if !withWorktree {
					testutil.MustGitExported(t, dir, "branch", got.DeleteMode, branch)
				}
			})
		}
	}
}
