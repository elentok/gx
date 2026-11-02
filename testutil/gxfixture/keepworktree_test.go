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
)

const (
	keepFirst  = "demo:keep-epic/01"
	keepSecond = "demo:keep-epic/02"
)

// worktrees lists `git worktree list --porcelain` of the fixture repo.
func (r *running) worktrees(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	out, err := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").CombinedOutput()
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

func (r *running) waitStatus(t *testing.T, addr, want string) {
	t.Helper()
	if !testutil.Poll(90*time.Second, time.Second, func() bool { return r.ticketStatus(t, addr) == want }) {
		t.Logf("history:\n%s", r.gx(t, "server", "tickets", "history", addr))
		t.Fatalf("ticket %s is %q, want %s", addr, r.ticketStatus(t, addr), want)
	}
}

func TestKeepWorktreeScenario_TheAskedFileSurvivesTheAnswerAndASecondTicketRuns(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-worktree")
	r.waitStatus(t, keepFirst, "needs-answer")
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
	r.waitStatus(t, keepFirst, "needs-answer")
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	r.waitStatus(t, keepSecond, "done")
	r.gx(t, "tickets", "answer", keepFirst, "--body", "go on")
	r.waitStatus(t, keepFirst, "done")
	r.stop(t)
}
