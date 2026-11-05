package log

import (
	"testing"

	"github.com/elentok/gx/ui/gitops"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
)

func sendOpenHelp(m Model) Model {
	updated, _ := m.Update(nav.OpenHelp()())
	return updated.(Model)
}

func TestOpenHelpOpensHelp(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 120, 40

	if m = sendOpenHelp(m); !m.help.IsOpen {
		t.Fatal("expected help open")
	}
}

func TestOpenHelpFromSearchEndsSearch(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 120, 40
	m.search.Start("fix")
	if !m.InputFocused() {
		t.Fatal("expected search input focused")
	}

	m = sendOpenHelp(m)
	if !m.help.IsOpen {
		t.Fatal("expected help open")
	}
	if m.search.Mode() != search.SearchModeNone || m.search.Query() != "" {
		t.Fatalf("search not ended: mode %v query %q", m.search.Mode(), m.search.Query())
	}
}

func TestOpenHelpWithModalOpenDoesNothing(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 120, 40
	m.git.SetOutput("t", "content")
	m.git.OpenOutput(m.width, m.height)

	if m = sendOpenHelp(m); m.help.IsOpen {
		t.Fatal("expected help to stay closed while a modal is open")
	}

	m.git = gitops.New()
	m.rebase.confirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: true}
	if m = sendOpenHelp(m); m.help.IsOpen {
		t.Fatal("expected help to stay closed while the rebase confirm is open")
	}
}
