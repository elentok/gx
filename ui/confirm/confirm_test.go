package confirm

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfirmNew(t *testing.T) {
	m := New()
	if m.IsOpen {
		t.Error("expected IsOpen=false initially")
	}
}

func TestConfirmOpen(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Are you sure?", DefaultYes: true})
	if !m.IsOpen {
		t.Error("expected IsOpen=true after Open")
	}
}

func TestConfirmAccept(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Continue?", DefaultYes: true})

	_, _, result := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Done || !result.Accepted {
		t.Error("expected Done=true, Accepted=true on enter with yes")
	}
}

func TestConfirmReject(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Continue?"})

	next, _, result := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !result.Done || result.Accepted {
		t.Error("expected Done=true, Accepted=false on 'n'")
	}
	if next.IsOpen {
		t.Error("expected IsOpen=false after rejection")
	}
}

func TestConfirmNavigation(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Continue?", DefaultYes: false})

	// 'h' should set yes without deciding
	next, _, result := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if result.Done {
		t.Error("expected not Done after 'h'")
	}
	_ = next

	// unhandled key
	_, _, result = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if result.Done {
		t.Error("expected not Done for unknown key")
	}
}

func TestConfirmView(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Delete?", DefaultYes: true})
	view := m.View(60)
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestConfirmNonKeyMsg(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Confirm?"})
	_, _, result := m.Update("not-a-key")
	if result.Done {
		t.Error("expected not Done for non-key msg")
	}
}
