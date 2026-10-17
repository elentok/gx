package status

import (
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func newOpenHelpModel(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "apple.txt", "one\n")
	m := newTestModelDefault(repo)
	m.ready = true
	m.width, m.height = 120, 40
	m.focus = focusFiletree
	return m
}

func sendOpenHelp(m Model) Model {
	updated, _ := m.Update(nav.OpenHelp()())
	return updated.(Model)
}

func TestOpenHelpOpensHelp(t *testing.T) {
	t.Parallel()
	if m := sendOpenHelp(newOpenHelpModel(t)); !m.help.IsOpen {
		t.Fatal("expected help open")
	}
}

func TestOpenHelpFromSearchEndsSearch(t *testing.T) {
	t.Parallel()
	m := newOpenHelpModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if !m.InputFocused() {
		t.Fatal("expected search input focused after /")
	}

	m = sendOpenHelp(m)
	if !m.help.IsOpen {
		t.Fatal("expected help open")
	}
	if s := m.fileTreeModel.Search(); s.Mode() != search.SearchModeNone || s.Query() != "" {
		t.Fatalf("search not ended: mode %v query %q", s.Mode(), s.Query())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.help.IsOpen || m.InputFocused() {
		t.Fatalf("closing help: help open %v, input focused %v", m.help.IsOpen, m.InputFocused())
	}
}

func TestOpenHelpWithModalOpenDoesNothing(t *testing.T) {
	t.Parallel()
	m := newOpenHelpModel(t)
	m.confirmOpen = true

	if m = sendOpenHelp(m); m.help.IsOpen {
		t.Fatal("expected help to stay closed while a modal is open")
	}
}
