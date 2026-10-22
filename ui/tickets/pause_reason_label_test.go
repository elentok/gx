package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

func TestPauseReasonLabel_NeverFallsBackToTheRawSpelling(t *testing.T) {
	for _, reason := range []schema.PauseReason{
		schema.PauseSelfReported, schema.PauseAmbiguousBase, schema.PauseZeroCommit, schema.PauseMergeUnreported,
		schema.PauseBlockedPane, schema.PauseSpinning, schema.PauseRetryExhausted, schema.PauseHandleMismatch,
		schema.PauseDuplicateLive, schema.PauseAgentNameTaken, schema.PauseAgentPaneBusy, schema.PauseAgentPromptStalled,
		schema.PauseBackgroundTaskGate, schema.PauseMergeConflict, schema.PauseAmbiguousMerge, schema.PauseDirtyTrunk,
		schema.PauseTrunkMissing, schema.PauseStrandedCommits, schema.PauseMergeError, schema.PauseBlockedCycle,
		schema.PauseParentDefect, schema.PauseBudgetKilled, schema.PauseIterationError, schema.PauseManual,
	} {
		if got, want := pauseReasonLabel(reason), reason.Label(); got != want || got == "" {
			t.Errorf("pauseReasonLabel(%q) = %q, want the schema label %q", reason, got, want)
		}
	}
}
