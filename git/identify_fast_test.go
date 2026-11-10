package git_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// IdentifyDir's fast path must answer exactly what the git-forking path
// answers, for every layout it claims, and must decline the rest.
func TestIdentifyDir_FastPathMatchesGit(t *testing.T) {
	t.Parallel()

	plain := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(plain, "a", "b"))
	linkedRepo, linkedWT := testutil.TempRepoWithLinkedWorktree(t, "feat")
	bareWT := testutil.TempBareRepoWithWorktrees(t, "feature")
	dotBare := testutil.TempDotBareRepoWithWorktrees(t, "feat")
	testutil.Mkdir(t, filepath.Join(dotBare, "feat", "sub"))
	bare := testutil.TempBareRepo(t)

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(plain, link); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		dir  string
		fast bool
	}{
		{"plain root", plain, true},
		{"plain subdir", filepath.Join(plain, "a", "b"), true},
		{"plain via symlink", link, true},
		{"plain subdir via symlink", filepath.Join(link, "a", "b"), true},
		{"plain inside .git", filepath.Join(plain, ".git"), false},
		{"linked worktree of plain repo", linkedWT, true},
		{"plain repo with linked worktree", linkedRepo, true},
		{"bare repo worktree", filepath.Join(bareWT, "feature"), true},
		{"bare repo root", bare, false},
		{".bare layout worktree", filepath.Join(dotBare, "feat"), true},
		{".bare layout worktree subdir", filepath.Join(dotBare, "feat", "sub"), true},
		{".bare layout root", dotBare, false},
		{".bare layout .bare dir", filepath.Join(dotBare, ".bare"), false},
		{"not a repo", t.TempDir(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := git.FastIdentified(tc.dir); got != tc.fast {
				t.Errorf("fast path taken = %v, want %v", got, tc.fast)
			}
			got, gotErr := git.IdentifyDir(tc.dir)
			want, wantErr := git.IdentifyDirSlow(tc.dir)
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("err = %v, git path err = %v", gotErr, wantErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("IdentifyDir = %+v, git path = %+v", got, want)
			}
		})
	}
}

func TestIdentifyDir_EnvOverrideSkipsFastPath(t *testing.T) { // not parallel: sets env
	dir := testutil.TempRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(dir, ".git"))
	if git.FastIdentified(dir) {
		t.Error("fast path taken with GIT_DIR set")
	}
}
