package stashlist

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func runTabInit(t Tab) Tab {
	cmd := t.Init()
	if cmd == nil {
		return t
	}
	msg := cmd()
	updated, _ := t.Update(msg)
	return updated.(Tab)
}

func sendTab(t Tab, msg tea.Msg) Tab {
	updated, _ := t.Update(msg)
	return updated.(Tab)
}

func newReadyTab(t *testing.T) Tab {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")
	mustStashFile(t, repo, "stash-a")

	tab := runTabInit(NewTab(repo, ui.Settings{}, keys.Manager{}))
	tab = sendTab(tab, tea.WindowSizeMsg{Width: 200, Height: 40})
	return tab
}

func TestLFromStashPanelFocusesDetail(t *testing.T) {
	tab := newReadyTab(t)
	if !tab.split.IsSplit() || !tab.split.IsListFocused() {
		t.Fatal("expected stash tab to start split with list focus")
	}

	tab = sendTab(tab, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !tab.split.IsDetailFocused() {
		t.Fatal("expected detail focused after l from stash panel")
	}
	if !tab.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after l from stash panel")
	}
}

func TestHFromDetailFileTreeReturnsFocusToList(t *testing.T) {
	tab := newReadyTab(t)
	tab = sendTab(tab, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !tab.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused before h")
	}

	tab = sendTab(tab, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !tab.split.IsSplit() {
		t.Fatal("expected split to remain open after h from detail")
	}
	if !tab.split.IsListFocused() {
		t.Fatal("expected list focused after h from detail")
	}
}

func TestQFromDetailReturnsFocusToList(t *testing.T) {
	tab := newReadyTab(t)
	tab = sendTab(tab, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !tab.split.IsDetailFocused() {
		t.Fatal("expected detail focused before q")
	}

	tab = sendTab(tab, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !tab.split.IsSplit() {
		t.Fatal("expected split to remain open after q from detail")
	}
	if !tab.split.IsListFocused() {
		t.Fatal("expected list focused after q from detail")
	}
}
