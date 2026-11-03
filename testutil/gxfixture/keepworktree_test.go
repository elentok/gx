package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

const (
	keepFirst  = "demo:keep-epic/01"
	keepSecond = "demo:keep-epic/02"
)

// askedStatus is the rendered status word of a ticket waiting on an answer.
var askedStatus = tickets.StatusPausedAnswer.Word()

// worktrees lists `git worktree list --porcelain` of the fixture repo.
func (r *running) worktrees(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "-C", r.repo(), "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}
	return string(out)
}

// askedWorktree returns the path of the worktree holding the uncommitted file,
// or "" while there is none.
func (r *running) askedWorktree(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(r.worktrees(t), "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			if _, err := os.Stat(filepath.Join(path, agentfake.UncommittedFile)); err == nil {
				return path
			}
		}
	}
	return ""
}

func TestKeepWorktreeScenario_TheAskedFileSurvivesTheAnswerAndASecondTicketRuns(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-worktree")
	r.waitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s while the ticket waits:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	if !strings.Contains(r.worktrees(t), "branch refs/heads/ralph-loop/") {
		t.Errorf("no iteration branch while the ticket waits:\n%s", r.worktrees(t))
	}
	// The second ticket starts while the first waits.
	r.waitStatus(t, keepSecond, "done")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the waiting ticket's worktree is gone after the second ticket ran: %v", err)
	}
	r.gx(t, "tickets", "answer", keepFirst, "--body", "go on")
	// The relaunch finishes only when it still sees the file.
	r.waitStatus(t, keepFirst, "done")
	r.stop(t)
}

func TestKeepAgentScenario_TheUnmodifiedPtyFixtureKeepsTheAskedWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-agent")
	r.waitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	r.waitStatus(t, keepSecond, "done")
	r.gx(t, "tickets", "answer", keepFirst, "--body", "go on")
	r.waitStatus(t, keepFirst, "done")
	r.stop(t)
}

// worktreeListed reports whether `git worktree list` names path.
func (r *running) worktreeListed(t *testing.T, path string) bool {
	t.Helper()
	return strings.Contains(r.worktrees(t), "worktree "+path+"\n")
}

func (r *running) waitWorktreeGone(t *testing.T, path, after string) {
	t.Helper()
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool { return !r.worktreeListed(t, path) }) {
		t.Fatalf("the worktree %s is still listed %s:\n%s", path, after, r.worktrees(t))
	}
}

func TestKeepCleanupScenario_ACommitlessAskKeepsItsWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-cleanup")
	r.waitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s after the ask:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	r.waitStatus(t, keepSecond, "done")
	if !r.worktreeListed(t, path) {
		t.Fatalf("the asked worktree %s left the list:\n%s", path, r.worktrees(t))
	}
	r.stop(t)
}

func TestKeepCleanupScenario_CancellingThePausedTicketRemovesItsWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-cleanup")
	r.waitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s before the cancel:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	r.gx(t, "server", "tickets", "cancel", keepFirst)
	r.waitWorktreeGone(t, path, "after the cancel")
	r.stop(t)
}

func TestKeepCleanupScenario_MergingTheEpicRemovesALeftoverIterationWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-cleanup")
	r.waitStatus(t, keepFirst, askedStatus)
	r.waitStatus(t, keepSecond, "done")
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	// The cancel removes the worktree, so the leftover is put back by hand where
	// the epic merge looks for it.
	r.gx(t, "server", "tickets", "cancel", keepFirst)
	r.waitWorktreeGone(t, path, "after the cancel")
	if out, err := exec.Command("git", "-C", r.repo(), "worktree", "add", "--detach", path).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	if !r.worktreeListed(t, path) {
		t.Fatalf("the leftover %s is not listed before the merge:\n%s", path, r.worktrees(t))
	}
	r.gx(t, "server", "epics", "merge", "demo:keep-epic")
	r.waitWorktreeGone(t, path, "after the epic merge")
	r.stop(t)
}
