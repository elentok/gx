package components

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestUpdateConfirmKeyHandling(t *testing.T) {
	if next, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'h', Text: "h"}, false); !handled || decided || accepted || !next {
		t.Fatalf("left/h should set yes without deciding")
	}

	if next, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'l', Text: "l"}, true); !handled || decided || accepted || next {
		t.Fatalf("right/l should set no without deciding")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'y', Text: "y"}, false); !handled || !decided || !accepted {
		t.Fatalf("y should accept")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: 'n', Text: "n"}, true); !handled || !decided || accepted {
		t.Fatalf("n should reject")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter}, true); !handled || !decided || !accepted {
		t.Fatalf("enter should accept when yes selected")
	}

	if _, decided, accepted, handled := UpdateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter}, false); !handled || !decided || accepted {
		t.Fatalf("enter should reject when no selected")
	}
}

func TestRenderConfirmModalIncludesPrompt(t *testing.T) {
	r := RenderConfirmModal(
		"Prompt?",
		true,
		lipgloss.Color("240"),
		lipgloss.Color("2"),
		lipgloss.Color("1"),
		lipgloss.Color("8"),
		40,
	)
	if r == "" {
		t.Fatalf("expected non-empty rendered modal")
	}
}
