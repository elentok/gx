package agents

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var enter = tea.KeyPressMsg{Code: tea.KeyEnter}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = update1(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestJump_EnterSwitchesSessionAndFocusesTheTerminal(t *testing.T) {
	m, _ := attachModel(t, &fakeHolders{attaches: map[string]*fakeAttach{}})
	m, _ = leaderThen(t, m, letter("j"))
	m = typeText(t, m, "beta")
	next, cmd := m.Update(enter)
	m = next.(Model)
	if m.jump != nil || m.selectedID() != "b" || !m.terminalFocused {
		t.Fatalf("jump=%v selected=%q focused=%v, want closed, b, focused", m.jump, m.selectedID(), m.terminalFocused)
	}
	if cmd == nil {
		t.Fatal("jumping did not dial the new session")
	}
}

func TestJump_SlashInSidebarOpensIt(t *testing.T) {
	m := press(t, sidebarModel(t, false, fixtureSessions(), 110, 16), "/")
	if m.jump == nil || !m.InputFocused() {
		t.Fatal("/ in the sidebar did not open the popup and take input")
	}
	m = typeText(t, m, "jk")
	if !strings.Contains(plainView(m), "> jk") || m.selectedID() != "s1" {
		t.Fatal("letters typed in the popup were not filter text")
	}
}
