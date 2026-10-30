package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
)

// TestRun_ReattachedSmartZoneBreach_AutoRecoversThenMerges covers ticket 23's
// first requirement: a reattached iteration's wait still proactively observes
// context occupancy against --smart-zone and auto-recovers a breach (Ctrl-C,
// /compact, finish-up re-prompt), the same as a fresh iteration's wait —
// because reattachIteration drives its wait through the same waitForFinish
// call. Ends in an ordinary merging with the reattached worktree/tab/branch
// cleaned up exactly once.
func TestRun_ReattachedSmartZoneBreach_AutoRecoversThenMerges(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if strings.Contains(cwd, "epic-item-01") {
			return 999999, true, nil
		}
		return 0, false, nil
	}
	hostLiveAgent(t, d, "epic-iter-01")
	fakeRunner(d).SetState("epic-iter-01", agentrunner.StateWorking, "")
	r := &blipRunner{Runner: fakeRunner(d), timeoutOn: 1}
	d.Runner = r

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if r.interrupts != 1 {
		t.Errorf("interrupts = %d, want a single interrupt for the smart-zone breach", r.interrupts)
	}
	if prompts := r.Prompts("epic-iter-01"); len(prompts) != 2 || prompts[0] != "/compact" || !strings.Contains(prompts[1], "context window") {
		t.Fatalf("prompts = %v, want [/compact, finish-up prompt]", prompts)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration cleaned up exactly once", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket after reattached smart-zone recovery = %s, want status: done", raw)
	}
}
