package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func keyText(s string) tea.KeyPressMsg {
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func keyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func typeFilter(t *testing.T, state MenuState, text string) MenuState {
	t.Helper()
	for _, r := range text {
		next, decided, _, handled := UpdateMenu(keyText(string(r)), state)
		if !handled || decided {
			t.Fatalf("typing %q: handled=%v decided=%v", r, handled, decided)
		}
		state = next
	}
	return state
}

func labels(items []MenuItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}

func TestUpdateMenuNavigationAndSelection(t *testing.T) {
	state := MenuState{Items: []MenuItem{{Label: "A"}, {Label: "B"}, {Label: "C"}}, Cursor: 0}

	next, decided, accepted, handled := UpdateMenu(keyCtrl('n'), state)
	if !handled || decided || accepted || next.Cursor != 1 {
		t.Fatalf("expected ctrl+n to move cursor, got %+v decided=%v accepted=%v handled=%v", next, decided, accepted, handled)
	}
	next, _, _, _ = UpdateMenu(tea.KeyPressMsg{Code: tea.KeyDown}, next)
	if next.Cursor != 2 {
		t.Fatalf("expected down to move cursor to 2, got %d", next.Cursor)
	}
	next, _, _, _ = UpdateMenu(keyCtrl('p'), next)
	if next.Cursor != 1 {
		t.Fatalf("expected ctrl+p to move cursor to 1, got %d", next.Cursor)
	}
	next, _, _, _ = UpdateMenu(tea.KeyPressMsg{Code: tea.KeyUp}, next)
	if next.Cursor != 0 {
		t.Fatalf("expected up to move cursor to 0, got %d", next.Cursor)
	}
	next, _, _, _ = UpdateMenu(keyCtrl('n'), next)

	next, decided, accepted, handled = UpdateMenu(tea.KeyPressMsg{Code: tea.KeyEnter}, next)
	if !handled || !decided || !accepted {
		t.Fatalf("expected enter to accept selection")
	}
	if it, ok := next.Selected(); !ok || it.Label != "B" {
		t.Fatalf("expected B selected, got %+v ok=%v", it, ok)
	}

	_, decided, accepted, handled = UpdateMenu(tea.KeyPressMsg{Code: tea.KeyEsc}, next)
	if !handled || !decided || accepted {
		t.Fatalf("expected esc to cancel selection")
	}
}

func TestUpdateMenuTypingFiltersIncludingFormerShortcuts(t *testing.T) {
	state := MenuState{Items: []MenuItem{
		{Label: "jump"},
		{Label: "kick"},
		{Label: "quit", Detail: "leave now"},
		{Label: "1 one"},
	}}

	for _, tc := range []struct {
		text string
		want []string
	}{
		{"j", []string{"jump"}},
		{"k", []string{"kick"}},
		{"q", []string{"quit"}},
		{"1", []string{"1 one"}},
		{"qnow", []string{"quit"}},
		{"U", []string{"jump", "quit"}},
	} {
		got := typeFilter(t, state, tc.text)
		if got.Filter != tc.text {
			t.Fatalf("typing %q: filter = %q", tc.text, got.Filter)
		}
		if strings.Join(labels(got.Visible()), ",") != strings.Join(tc.want, ",") {
			t.Fatalf("typing %q: visible = %v, want %v", tc.text, labels(got.Visible()), tc.want)
		}
	}
}

func TestUpdateMenuBackspaceWidensFilter(t *testing.T) {
	state := typeFilter(t, MenuState{Items: []MenuItem{{Label: "push"}, {Label: "pull"}}}, "pus")
	if len(state.Visible()) != 1 {
		t.Fatalf("expected one visible item, got %v", labels(state.Visible()))
	}
	state, decided, _, handled := UpdateMenu(tea.KeyPressMsg{Code: tea.KeyBackspace}, state)
	if !handled || decided || state.Filter != "pu" || len(state.Visible()) != 2 {
		t.Fatalf("expected backspace to widen filter, got filter=%q visible=%v", state.Filter, labels(state.Visible()))
	}
}

func TestUpdateMenuCursorClampsToVisibleItems(t *testing.T) {
	state := MenuState{Items: []MenuItem{{Label: "alpha"}, {Label: "beta"}, {Label: "gamma"}}, Cursor: 2}
	state = typeFilter(t, state, "b")
	if state.Cursor != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", state.Cursor)
	}
	state, _, _, _ = UpdateMenu(keyCtrl('n'), state)
	if state.Cursor != 0 {
		t.Fatalf("expected cursor to stay on the only visible item, got %d", state.Cursor)
	}
	if it, ok := state.Selected(); !ok || it.Label != "beta" {
		t.Fatalf("expected beta selected, got %+v ok=%v", it, ok)
	}
}

func TestUpdateMenuEnterReturnsFilteredItem(t *testing.T) {
	state := MenuState{Items: []MenuItem{{Label: "Patch", Value: "p"}, {Label: "Minor", Value: "mi"}, {Label: "Major", Value: "ma"}}}
	state = typeFilter(t, state, "maj")
	state, decided, accepted, handled := UpdateMenu(tea.KeyPressMsg{Code: tea.KeyEnter}, state)
	if !handled || !decided || !accepted {
		t.Fatalf("expected enter to accept, got decided=%v accepted=%v handled=%v", decided, accepted, handled)
	}
	if it, ok := state.Selected(); !ok || it.Value != "ma" {
		t.Fatalf("expected Major selected, got %+v ok=%v", it, ok)
	}
}

func TestUpdateMenuEnterWithNoMatchesDoesNothing(t *testing.T) {
	state := typeFilter(t, MenuState{Items: []MenuItem{{Label: "push"}}}, "zz")
	_, decided, _, handled := UpdateMenu(tea.KeyPressMsg{Code: tea.KeyEnter}, state)
	if !handled || decided {
		t.Fatalf("expected enter with no matches to be swallowed, got decided=%v handled=%v", decided, handled)
	}
	if _, ok := state.Selected(); ok {
		t.Fatalf("expected no selection with no matches")
	}
}

func TestRenderMenuModalIncludesPromptAndItems(t *testing.T) {
	r := RenderMenuModal(
		"Title",
		"Choose:",
		MenuState{Items: []MenuItem{{Label: "One"}, {Label: "Two"}}, Cursor: 0},
		"",
		lipgloss.Color("8"),
		lipgloss.Color("7"),
		lipgloss.Color("8"),
		lipgloss.Color("10"),
		60,
	)
	if !strings.Contains(r, "Choose:") || !strings.Contains(r, "One") || !strings.Contains(r, "Two") {
		t.Fatalf("menu modal missing content: %q", r)
	}
	for _, want := range []string{"navigate", "filter", "select", "cancel", "ctrl+n"} {
		if !strings.Contains(r, want) {
			t.Fatalf("menu modal hint missing %q: %q", want, r)
		}
	}
}

func TestRenderMenuListIncludesDetails(t *testing.T) {
	r := RenderMenuList(
		MenuState{Items: []MenuItem{{Label: "One", Detail: "first"}, {Label: "Two"}}, Cursor: 0},
		lipgloss.Color("8"),
		lipgloss.Color("10"),
	)
	if !strings.Contains(r, "One") || !strings.Contains(r, "first") || !strings.Contains(r, "Two") {
		t.Fatalf("menu list missing content: %q", r)
	}
}

func TestRenderMenuListShowsFilterAndOnlyVisibleItems(t *testing.T) {
	r := RenderMenuList(MenuState{Items: []MenuItem{{Label: "One"}, {Label: "Two"}}, Filter: "tw"}, lipgloss.Color("8"), lipgloss.Color("10"))
	if !strings.Contains(r, "tw") || !strings.Contains(r, "Two") || strings.Contains(r, "One") {
		t.Fatalf("filtered list wrong: %q", r)
	}
	r = RenderMenuList(MenuState{Items: []MenuItem{{Label: "One"}}, Filter: "zz"}, lipgloss.Color("8"), lipgloss.Color("10"))
	if !strings.Contains(r, "no matches") {
		t.Fatalf("expected no matches line: %q", r)
	}
}
