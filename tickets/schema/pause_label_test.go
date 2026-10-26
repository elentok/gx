package schema

import "testing"

func TestPauseReasonLabelGroups(t *testing.T) {
	groups := map[string][]PauseReason{
		"waiting for a person": {PauseSelfReported, PauseAmbiguousBase},
		"agent stopped or stuck": {
			PauseZeroCommit, PauseMergeUnreported, PauseBlockedPane, PauseSpinning, PauseRetryExhausted,
			PauseHandleMismatch, PauseDuplicateLive, PauseAgentNameTaken, PauseAgentPaneBusy, PauseAgentPromptStalled,
		},
		"background task": {PauseBackgroundTaskGate},
		"merging": {
			PauseMergeConflict, PauseAmbiguousMerge, PauseDirtyTrunk, PauseTrunkMissing,
			PauseStrandedCommits, PauseMergeError, PauseNeedsRebase, PauseConflictMarkers,
		},
		"ticket graph":     {PauseBlockedCycle, PauseParentDefect},
		"limits and other": {PauseBudgetKilled, PauseIterationError, PauseManual},
	}
	seen := 0
	for group, reasons := range groups {
		for _, r := range reasons {
			seen++
			if got := r.Label(); got == "" || got == string(r) {
				t.Errorf("%s: %q label = %q, want a plain label", group, r, got)
			}
		}
	}
	if seen != len(pauseClasses) {
		t.Errorf("groups cover %d reasons, the schema has %d", seen, len(pauseClasses))
	}
}
