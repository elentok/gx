package designflows

import (
	"encoding/json"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/cleanupscan"
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
	out, err := exec.Command("git", "-C", r.Repo(), "worktree", "list", "--porcelain").CombinedOutput()
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
	r.WaitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s while the ticket waits:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	if !strings.Contains(r.worktrees(t), "branch refs/heads/ralph-loop/") {
		t.Errorf("no iteration branch while the ticket waits:\n%s", r.worktrees(t))
	}
	// The second ticket starts while the first waits.
	r.WaitStatus(t, keepSecond, "done")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the waiting ticket's worktree is gone after the second ticket ran: %v", err)
	}
	r.Gx(t, "tickets", "answer", keepFirst, "--body", "go on")
	// The relaunch finishes only when it still sees the file.
	r.WaitStatus(t, keepFirst, "done")
	r.Stop(t)
}

func TestKeepAgentScenario_TheUnmodifiedPtyFixtureKeepsTheAskedWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-agent")
	r.WaitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	r.WaitStatus(t, keepSecond, "done")
	r.Gx(t, "tickets", "answer", keepFirst, "--body", "go on")
	r.WaitStatus(t, keepFirst, "done")
	r.Stop(t)
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
	r.WaitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s after the ask:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	r.WaitStatus(t, keepSecond, "done")
	if !r.worktreeListed(t, path) {
		t.Fatalf("the asked worktree %s left the list:\n%s", path, r.worktrees(t))
	}
	r.Stop(t)
}

func TestKeepCleanupScenario_CancellingThePausedTicketRemovesItsWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-cleanup")
	r.WaitStatus(t, keepFirst, askedStatus)
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s before the cancel:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	r.Gx(t, "server", "tickets", "cancel", keepFirst)
	r.waitWorktreeGone(t, path, "after the cancel")
	r.Stop(t)
}

func TestKeepCleanupScenario_MergingTheEpicRemovesALeftoverIterationWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "keep-cleanup")
	r.WaitStatus(t, keepFirst, askedStatus)
	r.WaitStatus(t, keepSecond, "done")
	path := r.askedWorktree(t)
	if path == "" {
		t.Fatalf("no worktree holds %s:\n%s", agentfake.UncommittedFile, r.worktrees(t))
	}
	// The cancel removes the worktree, so the leftover is put back by hand where
	// the epic merge looks for it.
	r.Gx(t, "server", "tickets", "cancel", keepFirst)
	r.waitWorktreeGone(t, path, "after the cancel")
	if out, err := exec.Command("git", "-C", r.Repo(), "worktree", "add", "--detach", path).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	if !r.worktreeListed(t, path) {
		t.Fatalf("the leftover %s is not listed before the merge:\n%s", path, r.worktrees(t))
	}
	r.Gx(t, "server", "epics", "merge", "demo:keep-epic")
	r.waitWorktreeGone(t, path, "after the epic merge")
	r.Stop(t)
}

func TestCleanupScenario_EveryCaseIsInGitAndTheScanClassifiesIt(t *testing.T) {
	t.Parallel()
	r := start(t, "cleanup")
	for _, branch := range []string{"merged-wt", "idle-wt", "dirty-wt"} {
		if !strings.Contains(r.worktrees(t), "branch refs/heads/"+branch+"\n") {
			t.Errorf("no worktree on %s:\n%s", branch, r.worktrees(t))
		}
	}
	cmd := exec.Command(fixturetest.GxBin, "cleanup", "scan", "--json")
	cmd.Dir = r.Repo()
	// r.Env has no PATH, and the scan shells out to git.
	cmd.Env = append(slices.Clone(r.Env), "PATH="+os.Getenv("PATH"))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("gx cleanup scan: %v\n%s", err, stderr.String())
	}
	var scan cleanupscan.Result
	if err := json.Unmarshal(out, &scan); err != nil {
		t.Fatal(err)
	}
	branches := map[string]cleanupscan.WorktreeScan{}
	for _, w := range scan.Worktrees {
		branches[w.Branch] = w
	}
	const pick, unmatched = "ralph-loop/cleanup-epic-item-01", "ralph-loop/cleanup-epic-item-02"
	for branch, want := range map[string]struct {
		rec   string
		dirty bool
	}{
		"merged-wt": {rec: "delete"}, pick: {rec: "delete"}, "squash-one": {rec: "delete"},
		unmatched: {}, "idle-wt": {}, "dirty-wt": {dirty: true}, "unmerged-epic": {},
	} {
		got, ok := branches[branch]
		if !ok {
			t.Errorf("scan lacks %s: %s", branch, out)
			continue
		}
		if got.Recommendation != want.rec || got.Dirty != want.dirty {
			t.Errorf("%s = recommendation %q dirty %t, want %q %t", branch, got.Recommendation, got.Dirty, want.rec, want.dirty)
		}
	}
	epics := map[string]cleanupscan.EpicScan{}
	for _, e := range scan.Epics {
		epics[e.Name] = e
	}
	if e := epics["merged-epic"]; !e.AllDone || !e.MergedToTrunk {
		t.Errorf("merged-epic = %+v, want done and merged", e)
	}
	if e := epics["unmerged-epic"]; !e.AllDone || e.MergedToTrunk {
		t.Errorf("unmerged-epic = %+v, want done and unmerged", e)
	}
	r.Stop(t)
}
