package menu

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var _ = strings.Contains // keep strings import

func TestMenuState_CreatesNumberedItems(t *testing.T) {
	items := []Item{
		{Label: "First"},
		{Label: "Second", Detail: "detail"},
	}
	state := menuState(items)
	if len(state.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(state.Items))
	}
	if !strings.HasPrefix(state.Items[0].Label, "1.") {
		t.Errorf("item 0 label = %q, expected to start with '1.'", state.Items[0].Label)
	}
	if state.Items[1].Detail != "detail" {
		t.Errorf("item 1 detail = %q, want 'detail'", state.Items[1].Detail)
	}
}

func TestModel_Init(t *testing.T) {
	m := model{items: []Item{{Label: "a"}}, state: menuState([]Item{{Label: "a"}})}
	if m.Init() != nil {
		t.Error("Init() should return nil")
	}
}

func TestModel_View(t *testing.T) {
	items := []Item{{Label: "Option A"}, {Label: "Option B"}}
	m := model{header: "Choose:", items: items, state: menuState(items)}
	v := m.View()
	_ = v // View() returns tea.View; just verify it doesn't panic
}

func TestModel_Update_NumberKey(t *testing.T) {
	items := []Item{{Label: "a"}, {Label: "b"}}
	m := model{items: items, state: menuState(items)}
	next, cmd := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	nm := next.(model)
	if !nm.done {
		t.Error("expected done=true after pressing '1'")
	}
	if nm.state.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", nm.state.Cursor)
	}
	if cmd == nil {
		t.Error("expected non-nil quit cmd")
	}
}

func TestModel_Update_EscAborts(t *testing.T) {
	items := []Item{{Label: "a"}}
	m := model{items: items, state: menuState(items)}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(model)
	if !nm.done || !nm.aborted {
		t.Error("expected done=true, aborted=true after esc")
	}
}

func TestModel_Update_QAborts(t *testing.T) {
	items := []Item{{Label: "a"}}
	m := model{items: items, state: menuState(items)}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	nm := next.(model)
	if !nm.aborted {
		t.Error("expected aborted=true after q")
	}
}

func TestModel_Update_NonKeyMsg(t *testing.T) {
	items := []Item{{Label: "a"}}
	m := model{items: items, state: menuState(items)}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	nm := next.(model)
	if nm.done {
		t.Error("expected done=false for window size msg")
	}
	_ = cmd
}

func TestModel_Update_OutOfRangeNumber(t *testing.T) {
	items := []Item{{Label: "a"}}
	m := model{items: items, state: menuState(items)}
	// Press '9' when only 1 item exists
	next, _ := m.Update(tea.KeyPressMsg{Code: '9', Text: "9"})
	nm := next.(model)
	if nm.done {
		t.Error("expected done=false for out-of-range number key")
	}
}
