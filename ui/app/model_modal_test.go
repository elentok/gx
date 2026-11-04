package app

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

// modalKeyRecorder is a page stub with ModalOpen=true that records key presses it receives.
type modalKeyRecorder struct {
	received []string
}

func (s *modalKeyRecorder) Init() tea.Cmd { return nil }
func (s *modalKeyRecorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		s.received = append(s.received, key.String())
	}
	return s, nil
}
func (s *modalKeyRecorder) View() tea.View  { return tea.NewView("stub") }
func (s *modalKeyRecorder) ModalOpen() bool { return true }

func TestNumberKeysDoNotSwitchTabsWhenModalOpen(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	stub := &modalOpenStub{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected tab to stay on status when modal is open, got %q", m.navState.ActiveTab())
	}
}

func TestGChordDoesNotSwitchTabsWhenModalOpen(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	stub := &modalOpenStub{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	// 'g' sets the prefix even when a modal is open — the block fires in switchTab on the second key.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if m.keyPrefix != "g" {
		t.Fatalf("expected keyPrefix to be 'g', got %q", m.keyPrefix)
	}

	// 'l' with prefix 'g' would switch to log, but modal blocks it — tab stays on status.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected tab to stay on status after g+l when modal is open, got %q", m.navState.ActiveTab())
	}
}

// TestRealPullModalBlocksTabSwitch verifies the full stack: a real status model
// with its pull modal open must block number-key tab switching at the app shell level.
func TestRealPullModalBlocksTabSwitch(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	// Give it a window size so the model can render.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m = runAppCmd(m, m.Init())

	// Press 'p' to open pull on the real status model.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(Model)

	// Verify the status model itself reports modal open.
	type modalOpener interface{ ModalOpen() bool }
	statusModel := m.livePageByTab[nav.TabStatus].model
	if mo, ok := statusModel.(modalOpener); !ok {
		t.Fatal("status model does not implement ModalOpen()")
	} else if !mo.ModalOpen() {
		t.Fatal("expected status model ModalOpen()=true after pressing 'p'")
	}

	// Process the background command (runs git status check).
	m = runAppCmd(m, cmd)

	// After background check, modal should still be open.
	statusModel = m.livePageByTab[nav.TabStatus].model
	if mo, ok := statusModel.(modalOpener); ok && !mo.ModalOpen() {
		t.Fatal("expected status model ModalOpen()=true after background check completes")
	}

	// Now press '2' — should be blocked, tab must stay on status.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected tab to stay on status when pull modal is open, got %q", m.navState.ActiveTab())
	}
}

// TestRealPullStashConfirmBlocksTabSwitch exercises the phaseStashConfirm path
// (repo has uncommitted changes, so pull asks "Stash and pull?"). Pressing 2
// must be blocked even in this phase.
// TestNavSwitchMsgIsBlockedWhenModalOpen verifies that a nav.Switch message is
// also blocked when a modal is open (belt-and-suspenders guard on the message path).
func TestNavSwitchMsgIsBlockedWhenModalOpen(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	stub := &modalOpenStub{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	// Send a nav.Switch message directly — should be blocked by the modal guard.
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir})())
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected nav.Switch to be blocked when modal is open, tab switched to %q", m.navState.ActiveTab())
	}
}

func TestRealPullStashConfirmBlocksTabSwitch(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	// Create an uncommitted change so pull needs to stash.
	testutil.WriteFile(t, repoDir, "dirty.txt", "uncommitted")

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Press 'p' to open pull.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(Model)

	// Process the background git status check — with dirty tree, pull model
	// transitions to phaseStashConfirm.
	m = runAppCmd(m, cmd)

	// Pull modal must still be open (stash confirm dialog).
	type modalOpener interface{ ModalOpen() bool }
	statusModel := m.livePageByTab[nav.TabStatus].model
	if mo, ok := statusModel.(modalOpener); !ok {
		t.Fatal("status model does not implement ModalOpen()")
	} else if !mo.ModalOpen() {
		t.Fatal("expected ModalOpen()=true in stash-confirm phase")
	}

	// Press '2' — must be blocked.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected tab to stay on status in stash-confirm phase, got %q", m.navState.ActiveTab())
	}
}

// TestModalReceivesNonTabSwitchKeysWhenModalOpen covers the regression where
// ANY key showed "close the modal first", making the modal impossible to interact with.
// Non-tab-switch keys (y, n, enter, esc) must reach the child model even when ModalOpen=true.
func TestModalReceivesNonTabSwitchKeysWhenModalOpen(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	rec := &modalKeyRecorder{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = rec
	m.livePageByTab[nav.TabStatus] = live

	for _, msg := range []tea.KeyPressMsg{
		{Code: tea.KeyEnter},
		{Code: 'y', Text: "y"},
		{Code: 'n', Text: "n"},
		{Code: tea.KeyEscape},
	} {
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}

	if len(rec.received) == 0 {
		t.Fatal("non-tab-switch keys were not forwarded to the child model when modal is open")
	}
}
