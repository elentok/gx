package components

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func viewItems() []PickerItem {
	return []PickerItem{
		{Label: "worktrees", Value: "worktrees", Group: "git", Current: true},
		{Label: "log", Value: "log", Group: "git"},
		{Label: "status", Value: "status", Group: "git"},
		{Label: "tickets", Value: "tickets", Group: "agents"},
		{Label: "agents", Value: "agents", Group: "agents", Badge: "2"},
	}
}

func typePicker(t *testing.T, state PickerState, text string) PickerState {
	t.Helper()
	for _, r := range text {
		next, decided, _, handled := UpdatePicker(keyText(string(r)), state)
		if !handled || decided {
			t.Fatalf("typing %q: handled=%v decided=%v", r, handled, decided)
		}
		state = next
	}
	return state
}

func matchLabels(state PickerState) []string {
	var out []string
	for _, m := range state.Matches() {
		out = append(out, m.Item.Label)
	}
	return out
}

func renderLines(state PickerState, width int) []string {
	return strings.Split(ansi.Strip(RenderPickerList(state, width)), "\n")
}

func TestPickerEmptyQueryKeepsOrderWithGroupHeaders(t *testing.T) {
	state := PickerState{Items: viewItems()}
	want := []string{"worktrees", "log", "status", "tickets", "agents"}
	if got := matchLabels(state); !slices.Equal(got, want) {
		t.Fatalf("matches = %v, want %v", got, want)
	}
	lines := renderLines(state, 30)
	if len(lines) != 7 || strings.TrimSpace(lines[0]) != "git" || strings.TrimSpace(lines[4]) != "agents" {
		t.Fatalf("rows = %q", lines)
	}
}

func TestPickerRanksWithinGroupsAndOrdersGroupsByBestScore(t *testing.T) {
	items := []PickerItem{
		{Label: "stash", Group: "git"},
		{Label: "log", Group: "git"},
		{Label: "trace", Group: "git"},
		{Label: "tasks", Group: "agents"},
	}
	// "tasks" matches consecutively at a word start, beating every git item,
	// so its group leads; inside git, "trace" (word start) beats "stash".
	state := typePicker(t, PickerState{Items: items}, "ta")
	want := []string{"tasks", "trace", "stash"}
	if got := matchLabels(state); !slices.Equal(got, want) {
		t.Fatalf("matches = %v, want %v", got, want)
	}
	if it, _ := state.Selected(); it.Label != "tasks" {
		t.Fatalf("filtering should select the best match, got %q", it.Label)
	}
	lines := renderLines(state, 30)
	if strings.TrimSpace(lines[0]) != "agents" || strings.TrimSpace(lines[2]) != "git" {
		t.Fatalf("rows = %q", lines)
	}
}

func TestPickerCursorSkipsHeadersAndClamps(t *testing.T) {
	state := PickerState{Items: viewItems()}
	for range 3 {
		state, _, _, _ = UpdatePicker(tea.KeyPressMsg{Code: tea.KeyDown}, state)
	}
	if it, ok := state.Selected(); !ok || it.Label != "tickets" {
		t.Fatalf("selected = %+v %v, want tickets", it, ok)
	}
	for range 5 {
		state, _, _, _ = UpdatePicker(keyCtrl('n'), state)
	}
	if it, _ := state.Selected(); it.Label != "agents" {
		t.Fatalf("down past the end: selected = %q", it.Label)
	}
	for range 10 {
		state, _, _, _ = UpdatePicker(keyCtrl('p'), state)
	}
	if it, _ := state.Selected(); it.Label != "worktrees" {
		t.Fatalf("up past the start: selected = %q", it.Label)
	}
}

func TestPickerEnterAndEsc(t *testing.T) {
	state := PickerState{Items: viewItems()}
	state, _, _, _ = UpdatePicker(tea.KeyPressMsg{Code: tea.KeyDown}, state)
	next, decided, accepted, handled := UpdatePicker(tea.KeyPressMsg{Code: tea.KeyEnter}, state)
	if !decided || !accepted || !handled {
		t.Fatalf("enter: decided=%v accepted=%v handled=%v", decided, accepted, handled)
	}
	if it, _ := next.Selected(); it.Value != "log" {
		t.Fatalf("enter selected %q", it.Value)
	}

	_, decided, accepted, handled = UpdatePicker(tea.KeyPressMsg{Code: tea.KeyEscape}, state)
	if !decided || accepted || !handled {
		t.Fatalf("esc: decided=%v accepted=%v handled=%v", decided, accepted, handled)
	}

	_, _, _, handled = UpdatePicker(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt}, state)
	if handled {
		t.Fatal("alt+x should not be handled")
	}
}

func TestPickerDigitsAndBackspaceEditFilter(t *testing.T) {
	items := []PickerItem{{Label: "view 1"}, {Label: "view 2"}}
	state := typePicker(t, PickerState{Items: items}, "2")
	if got := matchLabels(state); !slices.Equal(got, []string{"view 2"}) {
		t.Fatalf("matches = %v", got)
	}
	state, _, _, _ = UpdatePicker(tea.KeyPressMsg{Code: tea.KeyBackspace}, state)
	if state.Filter != "" || len(state.Matches()) != 2 {
		t.Fatalf("after backspace: filter=%q matches=%v", state.Filter, matchLabels(state))
	}
}

func TestPickerNoMatches(t *testing.T) {
	state := typePicker(t, PickerState{Items: viewItems()}, "zzz")
	lines := renderLines(state, 30)
	if len(lines) != 1 || strings.TrimSpace(lines[0]) != "no matches" {
		t.Fatalf("rows = %q", lines)
	}
	_, decided, accepted, handled := UpdatePicker(tea.KeyPressMsg{Code: tea.KeyEnter}, state)
	if decided || accepted || !handled {
		t.Fatalf("enter with no matches: decided=%v accepted=%v handled=%v", decided, accepted, handled)
	}
	if _, ok := state.HitRow(0); ok {
		t.Fatal("no-matches row should not hit")
	}
}

func TestPickerRenderMarksBadgeCurrentAndCursor(t *testing.T) {
	state := PickerState{Items: viewItems()}
	lines := renderLines(state, 30)
	if !strings.Contains(lines[1], "worktrees") || !strings.Contains(lines[1], "current") {
		t.Fatalf("current row = %q", lines[1])
	}
	if strings.Contains(lines[2], "current") {
		t.Fatalf("non-current row = %q", lines[2])
	}
	if !strings.Contains(lines[6], "agents 2") {
		t.Fatalf("badge row = %q", lines[6])
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 30 {
			t.Fatalf("row %d is %d wide: %q", i, w, l)
		}
	}
	raw := strings.Split(RenderPickerList(state, 30), "\n")
	if raw[1] == lines[1] {
		t.Fatal("cursor row should be styled")
	}
}

func TestPickerHitRowMapsBodyLinesToMatches(t *testing.T) {
	state := PickerState{Items: viewItems()}
	cases := map[int]struct {
		match int
		ok    bool
	}{0: {0, false}, 1: {0, true}, 3: {2, true}, 4: {0, false}, 5: {3, true}, 6: {4, true}, 7: {0, false}}
	for line, want := range cases {
		got, ok := state.HitRow(line)
		if ok != want.ok || ok && got != want.match {
			t.Errorf("HitRow(%d) = %d, %v; want %d, %v", line, got, ok, want.match, want.ok)
		}
	}
}

func TestPickerScrollsKeepingTheGroupHeader(t *testing.T) {
	state := PickerState{Items: viewItems(), Height: 3}
	if lines := renderLines(state, 30); len(lines) != 3 {
		t.Fatalf("rows = %q", lines)
	}
	for range 3 {
		state, _, _, _ = UpdatePicker(tea.KeyPressMsg{Code: tea.KeyDown}, state)
	}
	// The cursor is on tickets, the agents group's first item: its header shows too.
	lines := renderLines(state, 30)
	if strings.TrimSpace(lines[1]) != "agents" || !strings.Contains(lines[2], "tickets") {
		t.Fatalf("rows = %q", lines)
	}
	if got, ok := state.HitRow(2); !ok || got != 3 {
		t.Fatalf("HitRow(2) after scroll = %d, %v", got, ok)
	}
	for range 10 {
		state, _, _, _ = UpdatePicker(tea.KeyPressMsg{Code: tea.KeyUp}, state)
	}
	if lines := renderLines(state, 30); strings.TrimSpace(lines[0]) != "git" {
		t.Fatalf("rows after scrolling back = %q", lines)
	}
}
