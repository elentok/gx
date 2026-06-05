package stashlist

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
)

func runModelInit(m Model) Model {
	cmd := m.Init()
	if cmd == nil {
		return m
	}
	msg := cmd()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func sendModel(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func newReadyModel(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")
	mustStashFile(t, repo, "stash-a")

	m := runModelInit(NewModel(repo, ui.Settings{}, keys.Manager{}))
	m = sendModel(m, tea.WindowSizeMsg{Width: 200, Height: 40})
	return m
}

func TestQuestionMarkOpensHelpOverlay(t *testing.T) {
	m := newReadyModel(t)
	if m.help.IsOpen {
		t.Fatal("help should start closed")
	}

	m = sendModel(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !m.help.IsOpen {
		t.Fatal("expected help open after ?")
	}

	content := m.View().Content
	if !strings.Contains(content, "Keybindings") {
		t.Fatalf("expected help overlay with Keybindings title, got:\n%s", content)
	}
	for _, want := range []string{"apply stash", "pop stash", "drop stash", "create stash"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected help to list %q, got:\n%s", want, content)
		}
	}

	// esc closes the help overlay.
	m = sendModel(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.help.IsOpen {
		t.Fatal("expected help closed after esc")
	}
}

func TestLFromStashPanelFocusesDetail(t *testing.T) {
	m := newReadyModel(t)
	if !m.split.IsSplit() || !m.split.IsListFocused() {
		t.Fatal("expected stash tab to start split with list focus")
	}

	m = sendModel(m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after l from stash panel")
	}
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after l from stash panel")
	}
}

func TestHFromDetailFileTreeReturnsFocusToList(t *testing.T) {
	m := newReadyModel(t)
	m = sendModel(m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused before h")
	}

	m = sendModel(m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !m.split.IsSplit() {
		t.Fatal("expected split to remain open after h from detail")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after h from detail")
	}
}

func TestQFromDetailReturnsFocusToList(t *testing.T) {
	m := newReadyModel(t)
	m = sendModel(m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused before q")
	}

	m = sendModel(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !m.split.IsSplit() {
		t.Fatal("expected split to remain open after q from detail")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after q from detail")
	}
}

func TestQFromListReturnsNavBack(t *testing.T) {
	m := newReadyModel(t)
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused before q")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected nav.Back cmd from q on stash list")
	}
	if !nav.IsBack(cmd()) {
		t.Fatalf("expected nav.Back msg, got %T", cmd())
	}
}

func TestWindowSizeLeavesFooterRowForTabs(t *testing.T) {
	m := newReadyModel(t)
	m = sendModel(m, tea.WindowSizeMsg{Width: 120, Height: 30})

	if m.stashList.height != 30 {
		t.Fatalf("stash list height = %d, want 30", m.stashList.height)
	}
}

func TestViewAddsFooterRowForAppTabs(t *testing.T) {
	m := newReadyModel(t)
	m = sendModel(m, tea.WindowSizeMsg{Width: 120, Height: 30})

	lines := strings.Split(m.View().Content, "\n")
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}
	// The footer row carries the "? help" hint and is where the app shell
	// injects the tab bar.
	if !strings.Contains(lines[len(lines)-1], "? help") {
		t.Fatalf("expected footer row with ? help, got %q", lines[len(lines)-1])
	}
}

func TestAutoReloadDispatchesLoadAndPreservesSplit(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")
	mustStashFile(t, repo, "stash-a")
	mustStashFile(t, repo, "stash-b")
	m := runModelInit(NewModel(repo, ui.Settings{}, keys.Manager{}))
	m = sendModel(m, tea.WindowSizeMsg{Width: 200, Height: 40})
	splitBefore := m.split.IsSplit()

	// AutoReload must return a non-nil cmd that produces stashAutoReloadMsg.
	autoCmd := m.AutoReload()
	if autoCmd == nil {
		t.Fatal("AutoReload returned nil cmd")
	}
	if _, ok := autoCmd().(stashAutoReloadMsg); !ok {
		t.Fatal("AutoReload cmd did not return stashAutoReloadMsg")
	}

	// Dispatching stashAutoReloadMsg must trigger a list reload (non-nil cmd)
	// without changing split state.
	updated, reloadCmd := m.Update(stashAutoReloadMsg{})
	m = updated.(Model)
	if reloadCmd == nil {
		t.Fatal("stashAutoReloadMsg handler returned nil cmd")
	}
	if m.split.IsSplit() != splitBefore {
		t.Errorf("split state changed: got %v, want %v", m.split.IsSplit(), splitBefore)
	}

	// Deliver the reload result; selection index must survive.
	selBefore := m.stashList.list.Selected()
	loadedMsg := reloadCmd()
	m = sendModel(m, loadedMsg)
	if m.stashList.list.Selected() != selBefore {
		t.Errorf("selection changed after reload: got %d, want %d", m.stashList.list.Selected(), selBefore)
	}
}

func TestAutoReloadSatisfiesPageAutoReloadable(t *testing.T) {
	m := newReadyModel(t)
	type autoReloadable interface {
		AutoReload() tea.Cmd
	}
	if _, ok := any(m).(autoReloadable); !ok {
		t.Fatal("Model does not implement pageAutoReloadable interface")
	}
}
