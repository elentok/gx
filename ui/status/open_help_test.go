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

func TestOpenHelpFromSearchTypesNothing(t *testing.T) {
	t.Parallel()
	m := newOpenHelpModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if !m.InputFocused() {
		t.Fatal("expected search input focused after /")
	}

	m = sendOpenHelp(m)
	if !m.help.IsOpen {
		t.Fatal("expected help open")
	}
	if s := m.fileTreeModel.Search(); s.Mode() != search.SearchModeInput || s.Query() != "" {
		t.Fatalf("search changed: mode %v query %q", s.Mode(), s.Query())
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
