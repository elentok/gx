package prs

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func sendModel(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestModelRendersEmptyPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	content := m.View().Content
	if !strings.Contains(content, "no PRs") {
		t.Fatalf("expected placeholder content, got:\n%s", content)
	}
}

func TestQuestionMarkOpensHelpOverlay(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
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
}

func TestQKeyReturnsBackCmd(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a nav.Back() cmd from q")
	}
}
