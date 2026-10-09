package ralphloop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestCommitlessGitOneOff_RunsDetachedAndIsGoneAfterwards(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	one := OneIteration{RepoDir: repo, Epic: "fix-it", Ticket: tickets.Ticket{Identifier: "01"}}
	d := DefaultDeps()

	wt, err := PrepareCommitless(d, one, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	if wt.Path == repo {
		t.Fatalf("ran in the live checkout %s", wt.Path)
	}
	if got := gitIn(t, wt.Path, "rev-parse", "HEAD"); got != gitIn(t, repo, "rev-parse", "main") {
		t.Errorf("HEAD = %s, want the base tip", got)
	}
	if got := gitIn(t, wt.Path, "branch", "--show-current"); got != "" {
		t.Errorf("worktree is on branch %q, want detached", got)
	}
	if err := cleanupCommitless(d, one, wt, agentrunner.Session{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree still exists after the ticket ended: %v", err)
	}
	if got := gitIn(t, repo, "branch", "--list"); strings.Contains(got, "fix-it") {
		t.Errorf("a branch was left behind: %s", got)
	}
}

func TestCommitlessScratchOneOff_KeepsItsSubdirAfterwards(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scratch", "fix-it")
	one := OneIteration{Epic: "fix-it", Ticket: tickets.Ticket{Identifier: "01"}}
	d := DefaultDeps()

	wt, err := PrepareCommitless(d, one, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if wt.Path != dir {
		t.Fatalf("path = %s, want %s", wt.Path, dir)
	}
	if err := cleanupCommitless(d, one, wt, agentrunner.Session{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("scratch subdir gone after the ticket ended: %v", err)
	}
}
