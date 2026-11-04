package app

import (
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func TestSwitchMsgChangesTabWithoutHistory(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	if cmd == nil {
		t.Fatalf("expected resize cmd on Switch")
	}
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", m.navState.ActiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack after tab switch, got %d", len(m.history))
	}
}

func TestSwitchLastMsgReturnsToTheTabLeft(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})
	if _, cmd := m.Update(nav.SwitchLast()()); cmd == nil {
		t.Fatal("expected a notice when there is no last tab")
	}

	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	updated, _ = updated.(Model).Update(nav.SwitchLast()())
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected active tab worktrees, got %q", m.navState.ActiveTab())
	}
	updated, _ = m.Update(nav.SwitchLast()())
	if tab := updated.(Model).navState.ActiveTab(); tab != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", tab)
	}
}

func TestLeaderOSwitchesToTheLastTabOutsideAgents(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})
	updated, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	updated, cmd := updated.(Model).Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if cmd != nil || updated.(Model).keyPrefix != "ctrl+a" {
		t.Fatalf("expected ctrl+a to start the leader, prefix=%q", updated.(Model).keyPrefix)
	}
	_, cmd = updated.(Model).Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil || !nav.IsSwitchLast(cmd()) {
		t.Fatal("expected leader o to ask for the last tab")
	}
}

func TestLeaderDigitSwitchesTabOutsideAgents(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	updated, _ = updated.(Model).Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	got := updated.(Model)
	if got.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected leader 3 to open status, got %q", got.navState.ActiveTab())
	}
	if got.keyPrefix != "" {
		t.Fatalf("expected the leader to clear, prefix=%q", got.keyPrefix)
	}
}

func TestShellChordDirectTabSwitchClearsHistory(t *testing.T) {
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd on first g")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with gw")
	}
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected active tab worktrees, got %q", m.navState.ActiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack after gw, got %d", len(m.history))
	}
}

func TestShellChordSwitchesRelativeTabs(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with g,")
	}
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected g, to move left to worktrees, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabLog {
		t.Fatalf("expected g. to move right to log, got %q", m.navState.ActiveTab())
	}
}

func TestNumberKeysSwitchTabsGlobally(t *testing.T) {
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching to worktrees with 1")
	}
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected 1 to switch to worktrees, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabLog {
		t.Fatalf("expected 2 to switch to log, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected 3 to switch to status, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected 4 to switch to stash, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '5', Text: "5"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabPRs {
		t.Fatalf("expected 5 to switch to prs, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '6', Text: "6"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabTickets {
		t.Fatalf("expected 6 to switch to tickets, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '7', Text: "7"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabQueue {
		t.Fatalf("expected 7 to switch to queue, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '8', Text: "8"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabAgents {
		t.Fatalf("expected 8 to switch to agents, got %q", m.navState.ActiveTab())
	}

	// The Agents page starts in sidebar focus, so tab keys still reach the shell.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected 1 to leave agents for worktrees, got %q", m.navState.ActiveTab())
	}
}

func TestGTMnemonicSwitchesToTicketsTab(t *testing.T) {
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabTickets {
		t.Fatalf("expected gt to switch to tickets, got %q", m.navState.ActiveTab())
	}
}

func TestGQMnemonicSwitchesToQueueTab(t *testing.T) {
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabQueue {
		t.Fatalf("expected gq to switch to queue, got %q", m.navState.ActiveTab())
	}
}

func TestGAMnemonicSwitchesToAgentsTab(t *testing.T) {
	t.Parallel()
	m, _ := newAppModel(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabAgents {
		t.Fatalf("expected ga to switch to agents, got %q", m.navState.ActiveTab())
	}
}

func TestAgentsTabLabelNeverCarriesTheCount(t *testing.T) {
	t.Parallel()
	m, _ := newAppModel(t)
	m.stream.vm.Sessions = []wire.SessionInfo{{ID: "s1", Actionable: wire.SessionActionable{Reason: "blocked"}}}

	for _, spec := range m.tabSpecs() {
		if spec.id == nav.TabAgents && spec.label != "agents" {
			t.Fatalf("agents tab label = %q, want plain %q", spec.label, "agents")
		}
	}
}

func TestGPMnemonicSwitchesToPRsTab(t *testing.T) {
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabPRs {
		t.Fatalf("expected gp to switch to prs, got %q", m.navState.ActiveTab())
	}
}

func TestNumberKeysDoNotSwitchTabsWhenInputFocused(t *testing.T) {
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

	stub := &inputFocusedStub{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected tab to stay on status when input is focused, got %q", m.navState.ActiveTab())
	}
}

func TestSwitchToUninitializedTabRunsInit(t *testing.T) {
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

	worktreesPage := m.livePageByTab[nav.TabWorktrees]
	if worktreesPage.didInit {
		t.Fatalf("expected cached worktrees tab to start uninitialized")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if cmd == nil {
		t.Fatalf("expected init/resize cmd on gw into uninitialized tab")
	}
	m = updated.(Model)
	if !m.livePageByTab[nav.TabWorktrees].didInit {
		t.Fatalf("expected gw to mark worktrees tab initialized")
	}
}

func TestQueueTabLabelPlain(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabQueue, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	var queueLabel string
	for _, spec := range m.tabSpecs() {
		if spec.id == nav.TabQueue {
			queueLabel = spec.label
		}
	}
	if queueLabel != "queue" {
		t.Fatalf("queue tab label = %q, want plain %q", queueLabel, "queue")
	}
}
