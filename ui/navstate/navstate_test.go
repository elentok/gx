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
	// Stack top should still be the commit tab entry
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
