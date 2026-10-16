package menu

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var _ = strings.Contains // keep strings import

func TestMenuState_CreatesItems(t *testing.T) {
	items := []Item{
		{Label: "First"},
		{Label: "Second", Detail: "detail"},
	}
	state := menuState(items)
	if len(state.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(state.Items))
	}
	if state.Items[0].Label != "First" {
		t.Errorf("item 0 label = %q, want 'First' (no number prefix to pollute filtering)", state.Items[0].Label)
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

func TestModel_Update_FilterThenEnterPicksOriginalIndex(t *testing.T) {
	items := []Item{{Label: "pull"}, {Label: "push"}, {Label: "force push"}}
	var m tea.Model = model{items: items, state: menuState(items)}
	for _, r := range "fo" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm := next.(model)
	if !nm.done || nm.aborted {
		t.Fatalf("expected done and accepted, got done=%v aborted=%v", nm.done, nm.aborted)
	}
	if got := nm.selectedIndex(); got != 2 {
		t.Errorf("selectedIndex = %d, want 2", got)
	}
	if cmd == nil {
		t.Error("expected non-nil quit cmd")
	}
}

func TestModel_Update_DigitIsFilterText(t *testing.T) {
	items := []Item{{Label: "a"}, {Label: "b"}}
	m := model{items: items, state: menuState(items)}
	next, _ := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	nm := next.(model)
	if nm.done || nm.state.Filter != "1" {
		t.Errorf("expected '1' to be filter text, got done=%v filter=%q", nm.done, nm.state.Filter)
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

func TestModel_Update_QIsFilterText(t *testing.T) {
	items := []Item{{Label: "a"}}
	m := model{items: items, state: menuState(items)}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	nm := next.(model)
	if nm.done || nm.aborted || nm.state.Filter != "q" {
		t.Errorf("expected q to be filter text, got done=%v aborted=%v filter=%q", nm.done, nm.aborted, nm.state.Filter)
	}
}

func TestModel_Update_CtrlCAborts(t *testing.T) {
	items := []Item{{Label: "a"}}
	m := model{items: items, state: menuState(items)}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	nm := next.(model)
	if !nm.done || !nm.aborted {
		t.Error("expected done=true, aborted=true after ctrl+c")
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
