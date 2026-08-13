package ralphloop

import (
	"errors"
	"sync"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// stuckSubmissionRetryDeps builds a Deps whose git/worktree/deps-install
// calls are all no-ops (this test only cares about runIteration's
// TabCreate/TabClose/launch retry orchestration, not real git state), and
// whose herdr calls are driven by the given per-attempt AgentPrompt/AgentWait
// stubs. tabIDs records the TabID handed out by each TabCreate call in
// order; closedTabIDs records every TabID passed to TabClose in order.
func stuckSubmissionRetryDeps(t *testing.T, agentPrompt func(attempt int) (herdr.Agent, error)) (d Deps, tabIDs *[]string, closedTabIDs *[]string) {
	t.Helper()
	tabIDs = &[]string{}
	closedTabIDs = &[]string{}
	tabAttempt := 0

	d = Deps{
		RevParse: func(dir, ref string) (string, error) {
			if ref == "feature" {
				return "feature-tip", nil
			}
			// The iteration branch doesn't exist yet (branchExists' check).
			return "", errors.New("no such ref")
		},
		AddWorktree: func(repoDir, path, branch, base string) error { return nil },
		InstallDeps: func(path string) (string, error) { return "", nil },
		TabCreate: func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
			tabAttempt++
			tab := herdr.CreatedTab{
				Tab:        herdr.Tab{TabID: "tab-" + string(rune('0'+tabAttempt))},
				RootPaneID: "pane-" + string(rune('0'+tabAttempt)),
			}
			*tabIDs = append(*tabIDs, tab.TabID)
			return tab, nil
		},
		TabClose: func(tabID string) error {
			*closedTabIDs = append(*closedTabIDs, tabID)
			return nil
		},
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			// Only used here for "wait for idle after launch" (launch.go)
			// before the initial prompt; waitForFinish's own polling isn't
			// reached in this test since every attempt fails at the prompt
			// step itself.
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			return agentPrompt(tabAttempt)
		},
	}
	return d, tabIDs, closedTabIDs
}

func testIterationParams() iterationParams {
	return iterationParams{
		WorkspaceID:     "ws-1",
		RepoDir:         "/fake/repo",
		WorktreeDir:     "/fake/worktrees",
		FeatureWorktree: "/fake/repo",
		FeatureBranch:   "feature",
		Agent:           AgentClaude,
		Ticket:          tickets.Ticket{Identifier: "04"},
		WorktreeLock:    &sync.Mutex{},
		Gate:            NewGate(),
		Sink:            noopEventSink{},
	}
}

// TestRunIteration_StuckSubmission_ClosesPaneAndRetriesFresh is a regression
// test for the fix-spinner/04 incident: a launch whose initial prompt never
// reaches the pane at all (errStuckSubmission) must not bounce the ticket to
// needs-repair while leaking a live, never-prompted pane behind it. Instead
// runIteration should close that pane and retry against a fresh one.
func TestRunIteration_StuckSubmission_ClosesPaneAndRetriesFresh(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom: some unrelated failure once the prompt actually lands")
	d, tabIDs, closedTabIDs := stuckSubmissionRetryDeps(t, func(attempt int) (herdr.Agent, error) {
		if attempt == 1 {
			return herdr.Agent{}, errStuckSubmission
		}
		// The retry's prompt lands fine; a distinct, non-retryable error
		// further down the pipeline ends the test here so it doesn't have to
		// stub waitForFinish's full polling machinery too.
		return herdr.Agent{}, boom
	})

	err := runIteration(d, testIterationParams())
	if err == nil {
		t.Fatal("runIteration() error = nil, want the second attempt's error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("runIteration() error = %v, want it to wrap the second attempt's error %v", err, boom)
	}
	if errors.Is(err, errStuckSubmission) {
		t.Errorf("runIteration() error = %v, want no trace of errStuckSubmission (only the first attempt hit it)", err)
	}

	if len(*tabIDs) != 2 {
		t.Fatalf("TabCreate called %d times, want 2 (initial + one retry)", len(*tabIDs))
	}
	if len(*closedTabIDs) != 1 || (*closedTabIDs)[0] != (*tabIDs)[0] {
		t.Errorf("TabClose calls = %v, want exactly [%s] (only the first, stuck-submission pane)", *closedTabIDs, (*tabIDs)[0])
	}
}

// TestRunIteration_StuckSubmission_ExhaustsRetries_ClosesBothPanes covers the
// case where every launch attempt hits errStuckSubmission: runIteration
// should stop retrying once maxLaunchAttempts is spent (not loop forever)
// and must not leak the final attempt's pane either, even though it's the
// one the ticket ultimately fails against.
func TestRunIteration_StuckSubmission_ExhaustsRetries_ClosesBothPanes(t *testing.T) {
	t.Parallel()
	d, tabIDs, closedTabIDs := stuckSubmissionRetryDeps(t, func(attempt int) (herdr.Agent, error) {
		return herdr.Agent{}, errStuckSubmission
	})

	err := runIteration(d, testIterationParams())
	if !errors.Is(err, errStuckSubmission) {
		t.Fatalf("runIteration() error = %v, want it to wrap errStuckSubmission", err)
	}

	if len(*tabIDs) != maxLaunchAttempts {
		t.Fatalf("TabCreate called %d times, want maxLaunchAttempts (%d)", len(*tabIDs), maxLaunchAttempts)
	}
	if len(*closedTabIDs) != maxLaunchAttempts {
		t.Fatalf("TabClose called %d times, want maxLaunchAttempts (%d): every stuck pane, including the last, should be closed", len(*closedTabIDs), maxLaunchAttempts)
	}
	for i, tabID := range *tabIDs {
		if (*closedTabIDs)[i] != tabID {
			t.Errorf("closedTabIDs[%d] = %q, want %q (closed in the same order they were created)", i, (*closedTabIDs)[i], tabID)
		}
	}
}

// TestRunIteration_SucceedsFirstAttempt_NeverRetriesOrClosesTab is the
// sanity check that ordinary launch failures unrelated to errStuckSubmission
// (or a clean first-attempt failure) don't get the new retry/cleanup
// treatment at all.
func TestRunIteration_UnrelatedFailure_NeverRetriesOrClosesTab(t *testing.T) {
	t.Parallel()
	plainErr := errors.New("agent_pane_busy")
	d, tabIDs, closedTabIDs := stuckSubmissionRetryDeps(t, func(attempt int) (herdr.Agent, error) {
		return herdr.Agent{}, plainErr
	})

	err := runIteration(d, testIterationParams())
	if !errors.Is(err, plainErr) {
		t.Fatalf("runIteration() error = %v, want it to wrap %v", err, plainErr)
	}
	if len(*tabIDs) != 1 {
		t.Errorf("TabCreate called %d times, want 1 (no retry for a non-errStuckSubmission failure)", len(*tabIDs))
	}
	if len(*closedTabIDs) != 0 {
		t.Errorf("TabClose called %d times, want 0 (this failure mode's pane is left for needs-repair inspection, as before)", len(*closedTabIDs))
	}
}
