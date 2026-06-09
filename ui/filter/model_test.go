package filter

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewModel_InitialState(t *testing.T) {
	m := NewModel()
	if m.Query() != "" {
		t.Errorf("Query = %q, want empty", m.Query())
	}
	if m.HasQuery() {
		t.Error("HasQuery should be false initially")
	}
	if m.IsActive() {
		t.Error("IsActive should be false initially")
	}
	if m.InputFocused() {
		t.Error("InputFocused should be false initially")
	}
	if m.Mode() != ModeNone {
		t.Errorf("Mode = %d, want ModeNone", m.Mode())
	}
}

func TestStart_EntersInputMode(t *testing.T) {
	m := NewModel()
	m.Start()
	if m.Mode() != ModeInput {
		t.Errorf("Mode = %d, want ModeInput", m.Mode())
	}
	if !m.IsActive() {
		t.Error("IsActive should be true after Start")
	}
	if !m.InputFocused() {
		t.Error("InputFocused should be true after Start")
	}
}

func TestUpdate_SlashActivates(t *testing.T) {
	m := NewModel()
	next, _, result := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if next.Mode() != ModeInput {
		t.Errorf("Mode = %d, want ModeInput", next.Mode())
	}
	if !result.Activated {
		t.Error("expected Activated=true")
	}
	if !result.Handled {
		t.Error("expected Handled=true")
	}
}

func TestUpdate_TypingChangesQueryAndEmitsMsg(t *testing.T) {
	m := NewModel()
	m.Start()
	next, cmd, result := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if next.Query() != "a" {
		t.Errorf("Query = %q, want 'a'", next.Query())
	}
	if !result.QueryChanged {
		t.Error("expected QueryChanged=true")
	}
	if cmd == nil {
		t.Fatal("expected a command emitting FilterChangedMsg")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected BatchMsg, got %T", msg)
	}
	found := false
	for _, c := range batch {
		if fc, ok := c().(FilterChangedMsg); ok {
			found = true
			if fc.Query != "a" {
				t.Errorf("FilterChangedMsg.Query = %q, want 'a'", fc.Query)
			}
		}
	}
	if !found {
		t.Error("expected a FilterChangedMsg in the batch")
	}
}

func TestUpdate_EscClearsAndDeactivates(t *testing.T) {
	m := NewModel()
	m.Start()
	m, _, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.IsActive() {
		t.Error("esc should deactivate the filter")
	}
	if next.HasQuery() {
		t.Error("esc should clear the query")
	}
	if !result.Handled {
		t.Error("expected Handled=true on esc")
	}
}

func TestUpdate_EnterKeepsQueryAndDefocuses(t *testing.T) {
	m := NewModel()
	m.Start()
	m, _, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	next, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !next.IsActive() {
		t.Error("enter should keep the filter active")
	}
	if next.InputFocused() {
		t.Error("enter should defocus the input")
	}
	if next.Query() != "j" {
		t.Errorf("Query = %q, want 'j' after enter", next.Query())
	}
	if next.Mode() != ModeActive {
		t.Errorf("Mode = %d, want ModeActive", next.Mode())
	}
	if !result.Handled {
		t.Error("expected Handled=true on enter")
	}
}

func TestUpdate_SlashIgnoredWhenInactiveYieldsNotHandled(t *testing.T) {
	m := NewModel()
	_, _, result := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if result.Handled {
		t.Error("non-slash key while inactive should not be handled")
	}
}

func TestClear_ResetsState(t *testing.T) {
	m := NewModel()
	m.Start()
	m, _, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m.Clear()
	if m.IsActive() || m.HasQuery() {
		t.Errorf("Clear should reset: active=%v query=%q", m.IsActive(), m.Query())
	}
}

func TestUpdate_NonKeyMsg(t *testing.T) {
	m := NewModel()
	_, _, result := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if result.Handled {
		t.Error("non-key msg should not be handled")
	}
}

func TestView_NonEmptyWhenActive(t *testing.T) {
	m := NewModel()
	m.Start()
	if m.View() == "" {
		t.Error("expected non-empty view when active")
	}
}

func TestSetWidth_NoPanic(t *testing.T) {
	m := NewModel()
	m.SetWidth(80)
	m.SetWidth(1)
}

func TestInit_ReturnsNil(t *testing.T) {
	if NewModel().Init() != nil {
		t.Error("Init() should return nil")
	}
}
