package webflows

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/output"
	"github.com/elentok/gx/testutil/gxfixture/spec/wtd"
)

// pausedStatus is how the CLI reports a ticket paused with a question (file status: paused).
const pausedStatus = "needs-answer" // oldword:ok

// hasRun reports whether runs.json has an entry for addr.
func hasRun(t *testing.T, r *running, addr string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.StateDir, "runs.json"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(data), `"address":"`+addr+`"`)
}

func assertEdgeStates(t *testing.T, r *running, w *output.WorktreeDiffOutput, sessions []wire.SessionInfo) {
	t.Helper()

	// The capped worktree has one untracked file more than the cap, and a committed tracked change.
	untracked := strings.Fields(gitIn(t, w.Capped.Path, "ls-files", "-o", "--exclude-standard"))
	if len(untracked) != wtd.WorktreeDiffCapFiles {
		t.Errorf("capped: %d untracked files, want %d", len(untracked), wtd.WorktreeDiffCapFiles)
	}
	if n := commitsPast(t, w.Capped.Path, "main"); n == 0 {
		t.Error("capped: no committed tracked change past main")
	}

	// The oversize worktree has one file over the blob cap and a small one.
	if st, err := os.Stat(filepath.Join(w.Oversize.Path, "big/huge.txt")); err != nil || st.Size() <= rangediff.MaxBlobBytes {
		t.Errorf("oversize: huge.txt = %v, %v; want a file over %d bytes", st, err, rangediff.MaxBlobBytes)
	}
	if st, err := os.Stat(filepath.Join(w.Oversize.Path, "big/small.txt")); err != nil || st.Size() == 0 || st.Size() > 1024 {
		t.Errorf("oversize: small.txt = %v, %v; want a small file", st, err)
	}
	if got := strings.Fields(gitIn(t, w.Oversize.Path, "ls-files", "-o", "--exclude-standard")); len(got) != 2 {
		t.Errorf("oversize: untracked files = %v, want 2", got)
	}

	// The clean worktree has no changes and HEAD at its base.
	if got := gitIn(t, w.Clean.Path, "status", "--porcelain"); got != "" {
		t.Errorf("clean: status is not empty:\n%s", got)
	}
	if head, base := gitIn(t, w.Clean.Path, "rev-parse", "HEAD"), gitIn(t, w.Clean.Path, "merge-base", "HEAD", "main"); head != base {
		t.Errorf("clean: HEAD %s is not its merge-base %s", head, base)
	}

	// Both paused tickets have an iteration worktree and no run entry.
	for name, target := range map[string]output.WorktreeTarget{"paused feature": w.PausedFeature, "paused stacked": w.PausedStacked} {
		r.WaitStatus(t, target.Address, pausedStatus)
		if _, err := os.Stat(target.Path); err != nil {
			t.Errorf("%s: no iteration worktree: %v", name, err)
		}
		if hasRun(t, r, target.Address) {
			t.Errorf("%s: runs.json has an entry for %s", name, target.Address)
		}
	}
	gitIn(t, r.Repo(), "rev-parse", "--verify", w.PausedBlockerBranch)
	if out := gitIn(t, w.PausedStacked.Path, "merge-base", "--is-ancestor", w.PausedBlockerBranch, "HEAD"); out != "" {
		t.Errorf("blocker branch is not an ancestor of the stacked HEAD: %s", out)
	}
	if got, want := gitIn(t, w.PausedFeature.Path, "merge-base", "HEAD", w.FeatureBranch), gitIn(t, r.Repo(), "rev-parse", w.FeatureBranch); got != want {
		t.Errorf("paused feature base = %s, want the feature tip %s", got, want)
	}

	// The asking ticket keeps its run while it waits.
	r.WaitStatus(t, w.PausedRun.Address, pausedStatus)
	if !hasRun(t, r, w.PausedRun.Address) {
		t.Errorf("paused run: no runs.json entry for %s", w.PausedRun.Address)
	}
	if _, err := os.Stat(filepath.Join(w.PausedRun.Path, agentfake.UncommittedFile)); err != nil {
		t.Errorf("paused run: %v", err)
	}

	// The Triage chat is managed, so it has no worktree actions.
	if i := slices.IndexFunc(sessions, func(si wire.SessionInfo) bool { return si.ID == w.Triage.ID }); i < 0 || sessions[i].Flow != "triage" {
		t.Errorf("triage: no triage session with id %q", w.Triage.ID)
	}
}
