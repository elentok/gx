package ralphloop

import (
	"strings"
	"sync"
	"testing"
)

// TestIterationWorktreePath_ScopedByEpicName guards against the collision
// iterationWorktreePath exists to prevent: worktreeDir is shared across every
// epic running against the same repo, so without the epic-name path segment
// two epics that both happen to reach iteration number "04" would resolve to
// the exact same on-disk directory.
func TestIterationWorktreePath_ScopedByEpicName(t *testing.T) {
	t.Parallel()
	pathA := iterationWorktreePath("/fake/worktrees", "epic-a", "04")
	pathB := iterationWorktreePath("/fake/worktrees", "epic-b", "04")
	if pathA == pathB {
		t.Fatalf("iterationWorktreePath(epic-a, 04) = iterationWorktreePath(epic-b, 04) = %q, want distinct paths", pathA)
	}
}

// TestIterationKey_ScopedByEpicName guards the companion live-tab-tracking
// key: two epics using the same iteration label must not produce the same
// key, or reconcile could mistake one epic's live tab for another's.
func TestIterationKey_ScopedByEpicName(t *testing.T) {
	t.Parallel()
	keyA := iterationKey("epic-a", iterLabel("epic-a", "04"))
	keyB := iterationKey("epic-b", iterLabel("epic-b", "04"))
	if keyA == keyB {
		t.Fatalf("iterationKey(epic-a, iter-04) = iterationKey(epic-b, iter-04) = %q, want distinct keys", keyA)
	}
}

// TestIterLabel_ScopedByEpicName guards the displayed label itself: two
// epics that both happen to reach the same iteration identifier (e.g. both
// having a ticket "06") must produce visibly distinct labels, since this is
// what's actually shown to the user (herdr tab/pane names), unlike
// iterationKey which is only an internal map key.
func TestIterLabel_ScopedByEpicName(t *testing.T) {
	t.Parallel()
	labelA := iterLabel("epic-a", "06")
	labelB := iterLabel("epic-b", "06")
	if labelA == labelB {
		t.Fatalf("iterLabel(epic-a, 06) = iterLabel(epic-b, 06) = %q, want distinct labels", labelA)
	}
}

// TestIterLabel_LongEpicName_StaysWithinHerdrCap guards the herdr agent-name
// budget directly: a long epic name plus "-iter-" plus the identifier must
// never exceed herdr's 32-char "invalid_agent_name" limit, and must still
// leave the identifier intact (that's what a caller actually looks up).
func TestIterLabel_LongEpicName_StaysWithinHerdrCap(t *testing.T) {
	t.Parallel()
	label := iterLabel("notification-throttle-impl", "01")
	if len(label) > maxIterLabelLen {
		t.Fatalf("iterLabel(long name, 01) = %q (%d chars), want <= %d", label, len(label), maxIterLabelLen)
	}
	if !strings.HasSuffix(label, "-iter-01") {
		t.Fatalf("iterLabel(long name, 01) = %q, want suffix %q preserved", label, "-iter-01")
	}
}

// TestIterLabel_LongEpicName_StaysDistinctAcrossEpics guards against the
// truncation itself introducing the same collision iterLabel already
// protects against for short names: two long epic names sharing a common
// prefix must still truncate to different labels.
func TestIterLabel_LongEpicName_StaysDistinctAcrossEpics(t *testing.T) {
	t.Parallel()
	labelA := iterLabel("notification-throttle-impl-alpha", "01")
	labelB := iterLabel("notification-throttle-impl-beta", "01")
	if labelA == labelB {
		t.Fatalf("iterLabel(alpha, 01) = iterLabel(beta, 01) = %q, want distinct labels", labelA)
	}
}

// TestRun_TwoEpicsSameIterationNumber_DontCollideOnWorktreePath exercises
// ticket 01's regression scenario end to end: two separate epics that both
// reach an iteration numbered "04" must land on distinct worktree
// directories rather than the second epic silently reusing (or corrupting)
// the first's.
func TestRun_TwoEpicsSameIterationNumber_DontCollideOnWorktreePath(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var addedPaths []string

	instrumented := func() Deps {
		d, _, _ := fakeDeps()
		next := d.AddWorktree
		d.AddWorktree = func(repoDir, path, branch, base string) error {
			mu.Lock()
			addedPaths = append(addedPaths, path)
			mu.Unlock()
			return next(repoDir, path, branch, base)
		}
		return d
	}

	scratchA := writeEpic(t, "epic-a", map[string]string{
		"04-x.md": "---\nid: \"04\"\nstatus: open\ntype: task\n---\n# X\n",
	})
	if err := Run(RunOptions{EpicName: "epic-a", Skill: "implement", ScratchDir: scratchA, RepoDir: "/fake/repo"}, instrumented(), noopEventSink{}); err != nil {
		t.Fatalf("Run(epic-a) error = %v", err)
	}

	scratchB := writeEpic(t, "epic-b", map[string]string{
		"04-y.md": "---\nid: \"04\"\nstatus: open\ntype: task\n---\n# Y\n",
	})
	if err := Run(RunOptions{EpicName: "epic-b", Skill: "implement", ScratchDir: scratchB, RepoDir: "/fake/repo"}, instrumented(), noopEventSink{}); err != nil {
		t.Fatalf("Run(epic-b) error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	var iterPaths []string
	for _, p := range addedPaths {
		if strings.Contains(p, "item-04") {
			iterPaths = append(iterPaths, p)
		}
	}
	if len(iterPaths) != 2 {
		t.Fatalf("iteration worktree paths = %v, want exactly 2 (one per epic)", iterPaths)
	}
	if iterPaths[0] == iterPaths[1] {
		t.Fatalf("both epics' iter-04 worktree used the same path %q, want distinct paths scoped by epic name", iterPaths[0])
	}
	if !strings.Contains(iterPaths[0], "epic-a") && !strings.Contains(iterPaths[1], "epic-a") {
		t.Errorf("iteration worktree paths = %v, want one to be scoped under epic-a", iterPaths)
	}
	if !strings.Contains(iterPaths[0], "epic-b") && !strings.Contains(iterPaths[1], "epic-b") {
		t.Errorf("iteration worktree paths = %v, want one to be scoped under epic-b", iterPaths)
	}
}
