package ralphloop

import (
	"strings"
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
