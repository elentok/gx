package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestUpdateMenuNavigationAndSelection(t *testing.T) {
	state := MenuState{Items: []MenuItem{{Label: "A"}, {Label: "B"}, {Label: "C"}}, Cursor: 0}

	next, decided, accepted, handled := UpdateMenu(tea.KeyPressMsg{Code: 'j', Text: "j"}, state)
	if !handled || decided || accepted || next.Cursor != 1 {
		t.Fatalf("expected j to move cursor, got %+v decided=%v accepted=%v handled=%v", next, decided, accepted, handled)
	}

	next, decided, accepted, handled = UpdateMenu(tea.KeyPressMsg{Code: tea.KeyEnter}, next)
	if !handled || !decided || !accepted {
		t.Fatalf("expected enter to accept selection")
	}

	_, decided, accepted, handled = UpdateMenu(tea.KeyPressMsg{Code: tea.KeyEsc}, next)
	if !handled || !decided || accepted {
		t.Fatalf("expected esc to cancel selection")
	}
}

func TestRenderMenuModalIncludesPromptAndItems(t *testing.T) {
	r := RenderMenuModal(
		"Title",
		"Choose:",
		MenuState{Items: []MenuItem{{Label: "One"}, {Label: "Two"}}, Cursor: 0},
		"hint",
		lipgloss.Color("8"),
		lipgloss.Color("7"),
		lipgloss.Color("8"),
		lipgloss.Color("10"),
		60,
	)
	if !strings.Contains(r, "Choose:") || !strings.Contains(r, "One") || !strings.Contains(r, "Two") {
		t.Fatalf("menu modal missing content: %q", r)
	}
}
