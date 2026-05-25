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
	tr := s.Back()
	if tr.Kind != navstate.TransitionQuit {
		t.Fatalf("expected TransitionQuit, got %v", tr.Kind)
	}
}

func TestOpenPushesAndSetsActiveTab(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tr := s.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: defaultWT, Ref: "HEAD"})
	if tr.Kind != navstate.TransitionPushed {
		t.Fatalf("expected TransitionPushed, got %v", tr.Kind)
	}
	if tr.ActiveTab != nav.TabCommit {
		t.Fatalf("expected active tab commit, got %q", tr.ActiveTab)
	}
	if s.ActiveTab() != nav.TabCommit {
		t.Fatalf("expected state active tab commit, got %q", s.ActiveTab())
	}
}

func TestBackPopsAndRestoresPreviousTab(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: defaultWT, Ref: "HEAD"})
	tr := s.Back()
	if tr.Kind != navstate.TransitionPopped {
		t.Fatalf("expected TransitionPopped, got %v", tr.Kind)
	}
	if tr.ActiveTab != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", tr.ActiveTab)
	}
	if tr.PoppedEntry.Tab != nav.TabCommit {
		t.Fatalf("expected popped entry tab commit, got %q", tr.PoppedEntry.Tab)
	}
}

func TestSwitchClearsStack(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: defaultWT, Ref: "HEAD"})
	tr := s.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	if tr.Kind != navstate.TransitionSwitched {
		t.Fatalf("expected TransitionSwitched, got %v", tr.Kind)
	}
	if tr.ActiveTab != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", tr.ActiveTab)
	}
	// Verify stack is cleared by doing a Back which should return Quit.
	tr2 := s.Back()
	if tr2.Kind != navstate.TransitionQuit {
		t.Fatalf("expected TransitionQuit after Switch+Back, got %v", tr2.Kind)
	}
}

func TestSwitchCarriesPrevViewState(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tr := s.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	if tr.PrevViewState.Tab != nav.TabLog {
		t.Fatalf("expected PrevViewState.Tab log, got %q", tr.PrevViewState.Tab)
	}
}

func TestApplyViewStateChangedReturnsTransitionUpdated(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tr := s.ApplyViewStateChanged(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "abc"})
	if tr.Kind != navstate.TransitionUpdated {
		t.Fatalf("expected TransitionUpdated, got %v", tr.Kind)
	}
	if tr.ViewState.Ref != "abc" {
		t.Fatalf("expected ViewState.Ref abc, got %q", tr.ViewState.Ref)
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
	tr := s.Open(nav.ViewState{Tab: nav.TabLog, Ref: "main"})
	if tr.ViewState.WorktreeRoot != defaultWT {
		t.Fatalf("expected WorktreeRoot %q, got %q", defaultWT, tr.ViewState.WorktreeRoot)
	}
}

func TestEmptyContextRestoresFromTabMemory(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	// Seed log tab memory with a specific ref.
	s.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT, Ref: "main"})
	// Now switch to log with empty context — should restore "main".
	tr := s.Switch(nav.ViewState{Tab: nav.TabLog})
	if tr.ViewState.Ref != "main" {
		t.Fatalf("expected restored ref %q, got %q", "main", tr.ViewState.Ref)
	}
}

func TestCommitTabDefaultsRefToHEAD(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	tr := s.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: defaultWT})
	if tr.ViewState.Ref != "HEAD" {
		t.Fatalf("expected commit ref HEAD, got %q", tr.ViewState.Ref)
	}
}

func TestUnknownTabResolvesToWorktrees(t *testing.T) {
	s := newState()
	tr := s.Switch(nav.ViewState{Tab: nav.TabID("unknown")})
	if tr.ActiveTab != nav.TabWorktrees {
		t.Fatalf("expected unknown tab to resolve to worktrees, got %q", tr.ActiveTab)
	}
}

func TestResolveTabIDCommitIsFirstClass(t *testing.T) {
	if got := navstate.ResolveTabID(nav.TabCommit); got != nav.TabCommit {
		t.Fatalf("expected commit to resolve to itself, got %q", got)
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
	tr := s.Back()
	if tr.Kind != navstate.TransitionQuit {
		t.Fatalf("expected TransitionQuit (SetInitialTab leaves stack empty), got %v", tr.Kind)
	}
}

func TestBackWithStackDepthTwoPopsToMiddleEntry(t *testing.T) {
	s := newState()
	s.SetInitialTab(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: defaultWT})
	s.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: defaultWT, Ref: "HEAD"})
	s.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: defaultWT})
	// Stack: [commit, status]. First Back pops status → commit.
	tr1 := s.Back()
	if tr1.Kind != navstate.TransitionPopped {
		t.Fatalf("expected TransitionPopped on first back, got %v", tr1.Kind)
	}
	if tr1.ActiveTab != nav.TabCommit {
		t.Fatalf("expected active tab commit after first back, got %q", tr1.ActiveTab)
	}
	if tr1.PoppedEntry.Tab != nav.TabStatus {
		t.Fatalf("expected popped entry status, got %q", tr1.PoppedEntry.Tab)
	}
	// Second Back pops commit → empty stack, falls back to liveTab (log).
	tr2 := s.Back()
	if tr2.Kind != navstate.TransitionPopped {
		t.Fatalf("expected TransitionPopped on second back, got %v", tr2.Kind)
	}
	if tr2.ActiveTab != nav.TabLog {
		t.Fatalf("expected active tab log after second back, got %q", tr2.ActiveTab)
	}
	// Third Back: stack empty → quit.
	tr3 := s.Back()
	if tr3.Kind != navstate.TransitionQuit {
		t.Fatalf("expected TransitionQuit after stack exhausted, got %v", tr3.Kind)
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
	tr := s.Open(vs)
	if tr.ViewState.FocusSubject != "main.go" {
		t.Fatalf("expected FocusSubject %q, got %q", "main.go", tr.ViewState.FocusSubject)
	}
	if tr.ViewState.FilterPath != "cmd/" {
		t.Fatalf("expected FilterPath %q, got %q", "cmd/", tr.ViewState.FilterPath)
	}
	if tr.ViewState.FilterStartLine != 5 {
		t.Fatalf("expected FilterStartLine 5, got %d", tr.ViewState.FilterStartLine)
	}
	if tr.ViewState.FilterEndLine != 20 {
		t.Fatalf("expected FilterEndLine 20, got %d", tr.ViewState.FilterEndLine)
	}
}
