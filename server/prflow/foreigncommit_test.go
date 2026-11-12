package prflow_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// Story 36 proves the push step only: the gx-ci-fix skill's fetch, rebase and
// `--force-with-lease` commands, run against a real bare origin.

func gitOut(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "maintenance.auto=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func mustGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOut(t, dir, args...)
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return out
}

// foreignCommitRig pushes branch feature-a, then has another clone push a
// foreign commit to it, and leaves the agent worktree with a local fix commit.
func foreignCommitRig(t *testing.T) (agent, origin string) {
	t.Helper()
	repo := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	agent = filepath.Join(repo, "feature-a")
	testutil.PushBranchWithUpstream(t, agent, "origin", "feature-a")
	origin = mustGitOut(t, agent, "remote", "get-url", "origin")

	foreign := filepath.Join(t.TempDir(), "foreign")
	mustGitOut(t, t.TempDir(), "clone", "--branch", "feature-a", origin, foreign)
	mustGitOut(t, foreign, "config", "user.email", "other@example.com")
	mustGitOut(t, foreign, "config", "user.name", "Other")
	testutil.WriteFile(t, foreign, "foreign.txt", "from someone else")
	testutil.CommitAll(t, foreign, "foreign commit")
	mustGitOut(t, foreign, "push", "origin", "feature-a")

	testutil.WriteFile(t, agent, "fix.txt", "ci fix")
	testutil.CommitAll(t, agent, "ci fix")
	return agent, origin
}

func originLog(t *testing.T, origin string) string {
	t.Helper()
	return mustGitOut(t, origin, "log", "--format=%s", "feature-a")
}

func TestCIFixPush_ForeignCommitSurvivesFetchRebasePush(t *testing.T) {
	t.Parallel()
	agent, origin := foreignCommitRig(t)

	mustGitOut(t, agent, "fetch", "origin")
	mustGitOut(t, agent, "rebase", "origin/feature-a")
	mustGitOut(t, agent, "push", "--force-with-lease", "origin", "feature-a")

	log := originLog(t, origin)
	for _, want := range []string{"foreign commit", "ci fix"} {
		if !strings.Contains(log, want) {
			t.Fatalf("origin log missing %q:\n%s", want, log)
		}
	}
}

func TestCIFixPush_WithoutFetchTheLeaseRefusesAndForeignCommitSurvives(t *testing.T) {
	t.Parallel()
	agent, origin := foreignCommitRig(t)

	out, err := gitOut(t, agent, "push", "--force-with-lease", "origin", "feature-a")
	if err == nil {
		t.Fatalf("push without a fetch should be refused, got success:\n%s", out)
	}
	if log := originLog(t, origin); !strings.Contains(log, "foreign commit") {
		t.Fatalf("foreign commit lost from origin:\n%s", log)
	}
	if log := originLog(t, origin); strings.Contains(log, "ci fix") {
		t.Fatalf("refused push still landed the fix:\n%s", log)
	}
}
