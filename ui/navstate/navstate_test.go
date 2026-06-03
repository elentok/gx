package navstate_test

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"
)

const defaultWT = "/repo"

func newState() navstate.State {
	return navstate.NewState(defaultWT)
}

func TestBackOnEmptyStackReturnsQuit(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	_, quit := s.Back()
	if !quit {
		t.Fatalf("expected quit=true on empty stack back")
	}
}

func TestOpenPushesAndSetsActiveTab(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tabVS := s.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	if tabVS.Tab != nav.TabStatus {
		t.Fatalf("expected returned tab status, got %q", tabVS.Tab)
	}
	if s.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected state active tab status, got %q", s.ActiveTab())
	}
}

func TestBackPopsAndRestoresPreviousTab(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	active, quit := s.Back()
	if quit {
		t.Fatalf("expected quit=false after popping non-empty stack")
	}
	if active.Tab != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", active.Tab)
	}
	if s.ActiveTab() != nav.TabLog {
		t.Fatalf("expected state active tab log after back, got %q", s.ActiveTab())
	}
}

func TestSwitchClearsStack(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	tabVS := s.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	if tabVS.Tab != nav.TabStatus {
		t.Fatalf("expected returned tab status, got %q", tabVS.Tab)
	}
	if s.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", s.ActiveTab())
	}
	// Verify stack is cleared by doing a Back which should quit.
	_, quit := s.Back()
	if !quit {
		t.Fatalf("expected quit=true after Switch+Back (stack should be cleared)")
	}
}

func TestApplyViewStateChangedReturnsResolvedViewState(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tabVS := s.ApplyViewStateChanged(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "abc"})
	if tabVS.Ref != "abc" {
		t.Fatalf("expected returned Ref abc, got %q", tabVS.Ref)
	}
}

func TestApplyViewStateChangedUpdatesSamTab(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "HEAD"})
	newVS := nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "abc123"}
	s.ApplyViewStateChanged(newVS)
	active := s.Active()
	if active.Ref != "abc123" {
		t.Fatalf("expected active ref abc123 after ViewStateChanged, got %q", active.Ref)
	}
}

func TestApplyViewStateChangedIgnoresDifferentTab(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "HEAD"})
	s.ApplyViewStateChanged(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	// Stack top should still be the log tab entry.
	active := s.Active()
	if active.Tab != nav.TabLog {
		t.Fatalf("expected stack top still TabLog, got %q", active.Tab)
	}
}

// When WorktreeRoot is absent but other context fields are present (so the
// memory-restore branch is skipped), the default worktree is applied.
func TestEmptyWorktreeRootFallsBackToDefault(t *testing.T) {
	s := newState()
	// Ref is non-empty → all-empty-context check fails → memory restore skipped → defaultWT applies.
	tabVS := s.Open(nav.ViewState{Tab: nav.TabLog, Ref: "main"})
	if tabVS.WorktreeRoot != defaultWT {
		t.Fatalf("expected WorktreeRoot %q, got %q", defaultWT, tabVS.WorktreeRoot)
	}
}

func TestEmptyContextRestoresFromTabMemory(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	// Seed log tab memory with a specific ref.
	s.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "main"})
	// Now switch to log with empty context — should restore "main".
	tabVS := s.Switch(nav.ViewState{Tab: nav.TabLog})
	if tabVS.Ref != "main" {
		t.Fatalf("expected restored ref %q, got %q", "main", tabVS.Ref)
	}
}

func TestSwitchToSeededTabUsesDefaultWorktree(t *testing.T) {
	// Mirrors `gx log` (or `gx wt`): the status tab is only seeded by
	// initMissingTabs, never visited. Switching to it must carry the default
	// worktree, not a blank path — otherwise a commit there runs in the wrong
	// directory.
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tabVS := s.Switch(nav.ViewState{Tab: nav.TabStatus})
	if tabVS.WorktreeRoot != defaultWT {
		t.Fatalf("expected switched status WorktreeRoot %q, got %q", defaultWT, tabVS.WorktreeRoot)
	}
}

func TestStashTabSeededWithDefaultWorktree(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tabVS := s.Switch(nav.ViewState{Tab: nav.TabStash})
	if tabVS.WorktreeRoot != defaultWT {
		t.Fatalf("expected stash WorktreeRoot %q, got %q", defaultWT, tabVS.WorktreeRoot)
	}
}

func TestSwitchFromLogToStatusCarriesWorktree(t *testing.T) {
	const worktreeY = "/worktrees/feature"
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: worktreeY})
	// Switch to status with no explicit worktree — should inherit log's worktree.
	tabVS := s.Switch(nav.ViewState{Tab: nav.TabStatus})
	if tabVS.WorktreeRoot != worktreeY {
		t.Fatalf("expected status WorktreeRoot %q (from log), got %q", worktreeY, tabVS.WorktreeRoot)
	}
}

func TestSwitchFromStatusToLogCarriesWorktree(t *testing.T) {
	const worktreeY = "/worktrees/feature"
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: worktreeY})
	// Switch to log with no explicit worktree — should inherit status's worktree.
	tabVS := s.Switch(nav.ViewState{Tab: nav.TabLog})
	if tabVS.WorktreeRoot != worktreeY {
		t.Fatalf("expected log WorktreeRoot %q (from status), got %q", worktreeY, tabVS.WorktreeRoot)
	}
}

func TestUnknownTabResolvesToWorktrees(t *testing.T) {
	s := newState()
	tabVS := s.Switch(nav.ViewState{Tab: nav.TabID("unknown")})
	if tabVS.Tab != nav.TabWorktrees {
		t.Fatalf("expected unknown tab to resolve to worktrees, got %q", tabVS.Tab)
	}
}

func TestResolveTabIDStashIsFirstClass(t *testing.T) {
	if got := navstate.ResolveTabID(nav.TabStash); got != nav.TabStash {
		t.Fatalf("expected stash to resolve to itself, got %q", got)
	}
}

func TestResolveTabIDCommitAbsent(t *testing.T) {
	if got := navstate.ResolveTabID(nav.TabID("commit")); got != nav.TabWorktrees {
		t.Fatalf("expected unknown commit tab to resolve to worktrees, got %q", got)
	}
}

func TestSetInitialTabSeedsStateWithoutPushingStack(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "main"})
	if s.ActiveTab() != nav.TabLog {
		t.Fatalf("expected activeTab log after SetInitialTab, got %q", s.ActiveTab())
	}
	if s.LiveTab() != nav.TabLog {
		t.Fatalf("expected liveTab log after SetInitialTab, got %q", s.LiveTab())
	}
	if got := s.Active().Ref; got != "main" {
		t.Fatalf("expected seeded ref %q, got %q", "main", got)
	}
	// Back on an empty stack should quit, not pop.
	_, quit := s.Back()
	if !quit {
		t.Fatalf("expected quit=true (SetInitialTab leaves stack empty)")
	}
}

func TestSetInitialTabCarriesFilterOptions(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{
		Tab:             nav.TabLog,
		WorktreeRoot:    defaultWT,
		FilterPath:      "go.mod",
		FilterStartLine: 3,
		FilterEndLine:   9,
	})
	active := s.Active()
	if active.FilterPath != "go.mod" {
		t.Fatalf("expected seeded FilterPath %q, got %q", "go.mod", active.FilterPath)
	}
	if active.FilterStartLine != 3 || active.FilterEndLine != 9 {
		t.Fatalf("expected seeded filter range 3-9, got %d-%d", active.FilterStartLine, active.FilterEndLine)
	}
}

func TestBackWithStackDepthTwoPopsToMiddleEntry(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabStash, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	// Stack: [stash, status]. First Back pops status → stash.
	active1, quit1 := s.Back()
	if quit1 {
		t.Fatalf("expected quit=false on first back")
	}
	if active1.Tab != nav.TabStash {
		t.Fatalf("expected active tab stash after first back, got %q", active1.Tab)
	}
	// Second Back pops stash → empty stack, falls back to liveTab (log).
	active2, quit2 := s.Back()
	if quit2 {
		t.Fatalf("expected quit=false on second back")
	}
	if active2.Tab != nav.TabLog {
		t.Fatalf("expected active tab log after second back, got %q", active2.Tab)
	}
	// Third Back: stack empty → quit.
	_, quit3 := s.Back()
	if !quit3 {
		t.Fatalf("expected quit=true after stack exhausted")
	}
}

func TestOpenPropagatesViewOptionsToPushedEntry(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	vs := nav.ViewState{
		Tab:             nav.TabLog,
		WorktreeRoot:    defaultWT,
		Ref:             "HEAD",
		FocusSubject:    "main.go",
		FilterPath:      "cmd/",
		FilterStartLine: 5,
		FilterEndLine:   20,
	}
	tabVS := s.Open(vs)
	if tabVS.FocusSubject != "main.go" {
		t.Fatalf("expected FocusSubject %q, got %q", "main.go", tabVS.FocusSubject)
	}
	if tabVS.FilterPath != "cmd/" {
		t.Fatalf("expected FilterPath %q, got %q", "cmd/", tabVS.FilterPath)
	}
	if tabVS.FilterStartLine != 5 {
		t.Fatalf("expected FilterStartLine 5, got %d", tabVS.FilterStartLine)
	}
	if tabVS.FilterEndLine != 20 {
		t.Fatalf("expected FilterEndLine 20, got %d", tabVS.FilterEndLine)
	}
}
