package ralphloop

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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

// finishedCommitlessOneOff prepares a detached one-off for a ticket that has
// reported finished, commitless.
func finishedCommitlessOneOff(t *testing.T) (Deps, OneIteration, IterationWorktree) {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	scratchDir := fixture.WriteEpic(t, "fix-it", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: investigate\niteration_status: finished\ncommitless: true\n---\n# A\n",
	})
	path := fixture.TicketPath(scratchDir, "fix-it", "01-a.md")
	one := OneIteration{RepoDir: repo, Epic: "fix-it", ScratchDir: scratchDir, Ticket: tickets.Ticket{Identifier: "01", Path: path}}
	d := DefaultDeps()
	d.Runner = runnerfake.NewRunner()
	wt, err := PrepareCommitless(d, one, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	return d, one, wt
}

func TestFinishCommitless_NoCommitsMarksDoneAndRemovesWorktree(t *testing.T) {
	d, one, wt := finishedCommitlessOneOff(t)

	out, err := FinishCommitless(d, one, wt, "")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Merged {
		t.Errorf("outcome = %+v, want merged", out)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree still exists: %v", err)
	}
}

func TestFinishCommitless_AskOnNonKeepingRunKeepsWorktreeAndStopsSession(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	scratchDir := fixture.WriteEpic(t, "fix-it", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: investigate\niteration_status: asking\ncommitless: true\n---\n# A\n",
	})
	path := fixture.TicketPath(scratchDir, "fix-it", "01-a.md")
	one := OneIteration{RepoDir: repo, Epic: "fix-it", ScratchDir: scratchDir, Ticket: tickets.Ticket{Identifier: "01", Path: path}}
	d := DefaultDeps()
	runner := runnerfake.NewRunner()
	d.Runner = runner
	wt, err := PrepareCommitless(d, one, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	s, err := runner.Start(agentrunner.StartOptions{Label: wt.Label, Epic: "fix-it", Cwd: wt.Path})
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(wt.Path, "work.txt")
	if err := os.WriteFile(work, []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := FinishCommitless(d, one, wt, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(work); err != nil {
		t.Errorf("uncommitted file gone after the ask: %v", err)
	}
	if _, found, _ := runner.Find(wt.Label); found {
		t.Errorf("session still live after the ask")
	}
}

// Spec story 17: no timer or age check removes a kept worktree. The clock is
// moved a year ahead and every removal and sleep is trapped; the ask must
// neither remove the worktree nor schedule any wait that could.
func TestFinishCommitless_AskKeepsWorktreeWhateverTheClockSays(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	scratchDir := fixture.WriteEpic(t, "fix-it", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: investigate\niteration_status: asking\ncommitless: true\n---\n# A\n",
	})
	path := fixture.TicketPath(scratchDir, "fix-it", "01-a.md")
	one := OneIteration{RepoDir: repo, Epic: "fix-it", ScratchDir: scratchDir, Ticket: tickets.Ticket{Identifier: "01", Path: path}}
	d := DefaultDeps()
	d.Runner = runnerfake.NewRunner()
	wt, err := PrepareCommitless(d, one, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	var removals, sleeps atomic.Int32
	d.RemoveWorktree = func(string, string, bool) error {
		removals.Add(1)
		return nil
	}
	d.Sleep = func(time.Duration) { sleeps.Add(1) }
	d.Now = func() time.Time { return time.Now().AddDate(1, 0, 0) }

	if _, err := FinishCommitless(d, one, wt, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("kept worktree gone: %v", err)
	}
	if n := removals.Load(); n != 0 {
		t.Errorf("RemoveWorktree called %d times for a kept worktree", n)
	}
	if n := sleeps.Load(); n != 0 {
		t.Errorf("Sleep called %d times: the ask scheduled a wait", n)
	}
}

func TestFinishCommitless_StrandedCommitsPauseAndKeepWorktree(t *testing.T) {
	d, one, wt := finishedCommitlessOneOff(t)
	gitIn(t, wt.Path, "commit", "--allow-empty", "-m", "the fix")
	sha := gitIn(t, wt.Path, "rev-parse", "--short", "HEAD")

	out, err := FinishCommitless(d, one, wt, "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != schema.StatusLegacyAnswer || out.Kind != events.StrandedCommits {
		t.Errorf("outcome = %+v, want paused answer/%s", out, events.StrandedCommits)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
	evs, _, _ := runlog.ReadEvents(one.ScratchDir, "fix-it")
	evs = slices.DeleteFunc(evs, func(e runlog.Event) bool { return e.Type == string(events.DepsInstalled) })
	if len(evs) != 1 || !strings.Contains(evs[0].Reason, sha+" the fix") {
		t.Errorf("events = %+v, want one pause naming %s", evs, sha)
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
