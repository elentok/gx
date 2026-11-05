package servertest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// WriteCompleteEpic writes epic with one done ticket and completed_at, but no merged_at.
func WriteCompleteEpic(t *testing.T, store, epic string) {
	t.Helper()
	WriteTicketWith(t, store, "proj", epic, "01", "t", TicketOpts{})
	path := filepath.Join(store, "proj", epic, "issues", "01-t.md")
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), strings.Replace(ReadFile(t, path), "status: open", "status: done", 1))
	if err := tickets.StampEpicFields(filepath.Join(store, "proj", epic), map[string]any{"completed_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

// CommitOnBranch commits one file on a new branch, leaving main checked out.
func CommitOnBranch(t *testing.T, repo, branch string) {
	t.Helper()
	RunGitIn(t, repo, "checkout", "-q", "-b", branch)
	testutil.WriteFile(t, repo, branch+".txt", branch)
	testutil.CommitAll(t, repo, "work "+branch)
	RunGitIn(t, repo, "checkout", "-q", "main")
}

func WaitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	WaitFor(t, 10*time.Second, what, cond)
}

// AtMergeCap adds three done merge tickets to an epic's tickets: its run has
// used them all, so its next non-ff pauses the root instead of forking.
func AtMergeCap(opts map[string]TicketOpts) map[string]TicketOpts {
	opts["02"] = TicketOpts{Type: "merge", Status: "done"}
	opts["03"] = TicketOpts{Type: "merge", Status: "done"}
	opts["04"] = TicketOpts{Type: "merge", Status: "done"}
	return opts
}

// ReportIteration is the agent writing iteration_status onto its claimed ticket.
func ReportIteration(t *testing.T, path, status string) {
	t.Helper()
	body := strings.Replace(ReadFile(t, path), "status: claimed\n", "status: claimed\niteration_status: "+status+"\n", 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Error(err)
	}
}

// KeepWorktree leaves an iteration worktree and branch for epic/id, as a paused
// iteration does.
func KeepWorktree(t *testing.T, repo, epic, id string) string {
	t.Helper()
	wtDir, err := deps.DefaultDeps().WorktreeDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	path := worktree.IterationWorktreePath(wtDir, epic, id)
	out, err := exec.Command("git", "-C", repo, "worktree", "add", "-b", worktree.IterationBranch(epic, id), path).CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	return path
}

// CommitAgentWork commits agent.txt in the iteration worktree.
func CommitAgentWork(t *testing.T, wtPath string) {
	testutil.WriteFile(t, wtPath, "agent.txt", "work")
	testutil.CommitAll(t, wtPath, "agent work")
}

// PutRunIn registers a run of address in root on a session that no longer
// exists, with the harness's configured runner.
func (h *Harness) PutRunIn(address, root, cwd string, launching bool) {
	h.Server.Registry().PutRunIn(address, root, cwd, h.cfg.Runner.Name(), launching)
}

// RetroFixture is a one-ticket epic whose agent commits work; a submitted
// retro launches too, but it is commitless and commits nothing.
func RetroFixture(t *testing.T, opts ...func(*server.Config)) *SchedFixture {
	t.Helper()
	return NewSchedFixture(t, map[string]TicketOpts{"01": {}}, func(_ *SchedFixture, p *Turn, id string) {
		if strings.Contains(p.Address, ":one-offs/") {
			return
		}
		CommitWork(t, p, id)
	}, opts...)
}

// RetrosAfter waits for the one retro the hook submits after a root event: the
// submit follows the pause/fail event, so the glob is polled.
func (f *SchedFixture) RetrosAfter(t *testing.T) []string {
	t.Helper()
	var retros []string
	Poll(10*time.Second, func() bool {
		var err error
		retros, err = filepath.Glob(filepath.Join(f.Store, "proj", tickets.OneOffsEpic, "issues", "*-evolve-"+SchedEpic+"-*.md"))
		if err != nil {
			t.Fatal(err)
		}
		return len(retros) > 0
	})
	return retros
}

// RunnerShapes runs test once per runner shape: pty and headless.
func RunnerShapes(t *testing.T, test func(t *testing.T, headless bool)) {
	for name, headless := range map[string]bool{"pty": false, "headless": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			test(t, headless)
		})
	}
}
