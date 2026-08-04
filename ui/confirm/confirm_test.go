package confirm

import (
	"testing"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

func TestConfirmMouseClickYes(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Continue?", DefaultYes: false})
	bounds := components.LocateConfirmButtons("Continue?")

	_, _, result := m.Update(tea.MouseClickMsg{
		X: bounds.YesX0, Y: bounds.Row, Button: tea.MouseLeft,
	})
	if !result.Done || !result.Accepted {
		t.Fatalf("expected clicking Yes to accept, got %+v", result)
	}
}

func TestConfirmMouseClickNo(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Continue?", DefaultYes: true})
	bounds := components.LocateConfirmButtons("Continue?")

	next, _, result := m.Update(tea.MouseClickMsg{
		X: bounds.NoX0, Y: bounds.Row, Button: tea.MouseLeft,
	})
	if !result.Done || result.Accepted {
		t.Fatalf("expected clicking No to reject, got %+v", result)
	}
	if next.IsOpen {
		t.Error("expected IsOpen=false after clicking No")
	}
}

func TestConfirmMouseClickOutsideButtons(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Continue?", DefaultYes: true})

	_, _, result := m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if result.Done {
		t.Error("expected no action for a click outside any button")
	}
}

func TestConfirmUpdateMouseTranslatesScreenCoords(t *testing.T) {
	m := New()
	m = m.Open(Options{Prompt: "Continue?", DefaultYes: false})

	const width, screenW, screenH = 60, 100, 20
	view := m.View(width)
	ox, oy := ui.OverlayCenterOrigin(lipgloss.Width(view), lipgloss.Height(view), screenW, screenH)
	bounds := components.LocateConfirmButtons("Continue?")

	_, _, result := m.UpdateMouse(tea.MouseClickMsg{
		X: ox + bounds.YesX0, Y: oy + bounds.Row, Button: tea.MouseLeft,
	}, width, screenW, screenH)
	if !result.Done || !result.Accepted {
		t.Fatalf("expected clicking Yes at translated screen coords to accept, got %+v", result)
	}
}
