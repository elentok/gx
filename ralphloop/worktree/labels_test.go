package worktree

import (
	"strings"
	"testing"
)

// TestIterationWorktreePath_ScopedByEpicName guards against the collision
// IterationWorktreePath exists to prevent: worktreeDir is shared across every
// epic running against the same repo, so without the epic-name path segment
// two epics that both happen to reach iteration number "04" would resolve to
// the exact same on-disk directory.
func TestIterationWorktreePath_ScopedByEpicName(t *testing.T) {
	t.Parallel()
	pathA := IterationWorktreePath("/fake/worktrees", "epic-a", "04")
	pathB := IterationWorktreePath("/fake/worktrees", "epic-b", "04")
	if pathA == pathB {
		t.Fatalf("IterationWorktreePath(epic-a, 04) = IterationWorktreePath(epic-b, 04) = %q, want distinct paths", pathA)
	}
}

// TestIterationKey_ScopedByEpicName guards the companion live-tab-tracking
// key: two epics using the same iteration label must not produce the same
// key, or reconcile could mistake one epic's live tab for another's.
func TestIterationKey_ScopedByEpicName(t *testing.T) {
	t.Parallel()
	keyA := IterationKey("epic-a", IterationLabel("epic-a", "04"))
	keyB := IterationKey("epic-b", IterationLabel("epic-b", "04"))
	if keyA == keyB {
		t.Fatalf("IterationKey(epic-a, iter-04) = IterationKey(epic-b, iter-04) = %q, want distinct keys", keyA)
	}
}

// TestIterationLabel_ScopedByEpicName guards the displayed label itself: two
// epics that both happen to reach the same iteration identifier (e.g. both
// having a ticket "06") must produce visibly distinct labels, since this is
// what's actually shown to the user (herdr tab/pane names), unlike
// IterationKey which is only an internal map key.
func TestIterationLabel_ScopedByEpicName(t *testing.T) {
	t.Parallel()
	labelA := IterationLabel("epic-a", "06")
	labelB := IterationLabel("epic-b", "06")
	if labelA == labelB {
		t.Fatalf("IterationLabel(epic-a, 06) = IterationLabel(epic-b, 06) = %q, want distinct labels", labelA)
	}
}

// TestIterationLabel_LongEpicName_StaysWithinHerdrCap guards the herdr agent-name
// budget directly: a long epic name plus "-iter-" plus the identifier must
// never exceed herdr's 32-char "invalid_agent_name" limit, and must still
// leave the identifier intact (that's what a caller actually looks up).
func TestIterationLabel_LongEpicName_StaysWithinHerdrCap(t *testing.T) {
	t.Parallel()
	label := IterationLabel("notification-throttle-impl", "01")
	if len(label) > maxIterLabelLen {
		t.Fatalf("IterationLabel(long name, 01) = %q (%d chars), want <= %d", label, len(label), maxIterLabelLen)
	}
	if !strings.HasSuffix(label, "-iter-01") {
		t.Fatalf("IterationLabel(long name, 01) = %q, want suffix %q preserved", label, "-iter-01")
	}
}

// TestIterationLabel_LongEpicName_StaysDistinctAcrossEpics guards against the
// truncation itself introducing the same collision IterationLabel already
// protects against for short names: two long epic names sharing a common
// prefix must still truncate to different labels.
func TestIterationLabel_LongEpicName_StaysDistinctAcrossEpics(t *testing.T) {
	t.Parallel()
	labelA := IterationLabel("notification-throttle-impl-alpha", "01")
	labelB := IterationLabel("notification-throttle-impl-beta", "01")
	if labelA == labelB {
		t.Fatalf("IterationLabel(alpha, 01) = IterationLabel(beta, 01) = %q, want distinct labels", labelA)
	}
}
