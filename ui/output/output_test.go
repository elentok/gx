package output

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestOutputNew(t *testing.T) {
	m := New()
	if m.IsOpen || m.HasContent() {
		t.Error("expected empty model")
	}
}

func TestOutputSet(t *testing.T) {
	m := New()
	m.Set("title", "some output")
	if !m.HasContent() {
		t.Error("expected HasContent=true after Set")
	}

	// Empty content is ignored
	m2 := New()
	m2.Set("title", "   ")
	if m2.HasContent() {
		t.Error("expected HasContent=false for whitespace-only content")
	}
}

func TestOutputOpen(t *testing.T) {
	m := New()
	m.Set("title", "output")
	m.Open(100, 40)
	if !m.IsOpen {
		t.Error("expected IsOpen=true after Open")
	}
}

func TestOutputUpdate_CloseKeys(t *testing.T) {
	tests := []tea.KeyPressMsg{
		{Code: tea.KeyEscape},
		{Code: tea.KeyEnter},
		{Code: 'q', Text: "q"},
	}
	for _, msg := range tests {
		m := New()
		m.Set("title", "output")
		m.Open(100, 40)
		next, _ := m.Update(msg)
		if next.IsOpen {
			t.Errorf("key %v: expected IsOpen=false after close key", msg)
		}
	}
}

func TestOutputView_ClosedIsEmpty(t *testing.T) {
	m := New()
	if got := m.View(); got != "" {
		t.Errorf("View() on closed model = %q, want empty", got)
	}
}

func TestOutputView_Open(t *testing.T) {
	m := New()
	m.Set("", "some output content") // empty title → default title
	m.Open(100, 40)
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view when open")
	}
}
