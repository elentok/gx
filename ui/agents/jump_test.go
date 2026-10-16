package agents

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
)

var (
	ctrlN     = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	ctrlP     = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	enter     = tea.KeyPressMsg{Code: tea.KeyEnter}
	backspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

// jumpModel is the sidebar fixture with the jump popup open.
func jumpModel(t *testing.T) Model {
	t.Helper()
	m := sidebarModel(t, false, fixtureSessions(), 110, 16)
	m, _ = leaderThen(t, m, letter("j"))
	if m.jump == nil {
		t.Fatal("leader j did not open the jump popup")
	}
	return m
}

func jumpIDs(m Model) string {
	var ids []string
	for _, i := range m.jumpMatches() {
		ids = append(ids, m.sessions[i].ID)
	}
	return strings.Join(ids, " ")
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = update1(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestJump_Golden(t *testing.T) {
	assertGolden(t, "jump_popup", jumpModel(t).View().Content)
}

func TestJump_AttentionOrderBucketsAreStable(t *testing.T) {
	m := jumpModel(t)
	// Actionable s4 (Blocked) and s2 (reason), then the Unread s1, then the
	// rest, each bucket in sidebar order.
	if got := jumpIDs(m); got != "s4 s2 s1 s3 s5 s6 s7" {
		t.Fatalf("attention order = %q", got)
	}
	if !strings.Contains(plainView(m), "attention · 7/7") {
		t.Fatalf("border missing the order and count:\n%s", plainView(m))
	}
}

func TestJump_TabTogglesProjectOrder(t *testing.T) {
	m := update1(t, jumpModel(t), tab)
	if got := jumpIDs(m); got != "s1 s3 s4 s2 s5 s6 s7" {
		t.Fatalf("project order = %q", got)
	}
	if !strings.Contains(plainView(m), "project · 7/7") {
		t.Fatalf("border missing the project order:\n%s", plainView(m))
	}
	if m = update1(t, m, tab); !m.jump.attention() {
		t.Fatal("a second tab did not go back to attention order")
	}
}

func TestJump_TypingFiltersOverNameProjectAndDir(t *testing.T) {
	m := jumpModel(t)
	m.sessions[1].Dir = "/src/website" // s3
	for _, tc := range []struct{ query, want string }{
		{"RVW", "s4"},          // name, case-insensitive subsequence
		{"dtf", "s2 s5 s6 s7"}, // project
		{"wbst", "s3"},         // dir
	} {
		m.jump.query = ""
		m = typeText(t, m, tc.query)
		if got := jumpIDs(m); got != tc.want {
			t.Fatalf("%q matched %q, want %q", tc.query, got, tc.want)
		}
	}
	if !strings.Contains(plainView(m), "attention · 1/7") {
		t.Fatalf("count not updated by the filter:\n%s", plainView(m))
	}
	m = update1(t, m, backspace)
	if m.jump.query != "wbs" {
		t.Fatalf("backspace left query %q", m.jump.query)
	}
}

func TestJump_MovementClampsToMatches(t *testing.T) {
	m := jumpModel(t)
	for _, step := range []struct {
		key  tea.KeyPressMsg
		want int
	}{
		{ctrlN, 1}, {tea.KeyPressMsg{Code: tea.KeyDown}, 2}, {ctrlP, 1},
		{tea.KeyPressMsg{Code: tea.KeyUp}, 0}, {ctrlP, 0},
	} {
		if m = update1(t, m, step.key); m.jump.cursor != step.want {
			t.Fatalf("after %s: cursor %d, want %d", step.key, m.jump.cursor, step.want)
		}
	}
	m = typeText(t, update1(t, update1(t, m, ctrlN), ctrlN), "rvw")
	if m.jump.cursor != 0 {
		t.Fatalf("cursor %d after filtering to one match, want 0", m.jump.cursor)
	}
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

func TestJump_EnterWithNoMatchKeepsThePopup(t *testing.T) {
	m := update1(t, typeText(t, jumpModel(t), "zzz"), enter)
	if m.jump == nil || m.selectedID() != "s1" {
		t.Fatal("enter with no match closed the popup or moved the selection")
	}
}

func TestJump_EscCloses(t *testing.T) {
	m := update1(t, jumpModel(t), esc)
	if m.jump != nil || strings.Contains(plainView(m), "Jump to agent") {
		t.Fatal("esc did not close the popup")
	}
}

func TestJump_SlashInSidebarOpensIt(t *testing.T) {
	m := press(t, sidebarModel(t, false, fixtureSessions(), 110, 16), "/")
	if m.jump == nil || !m.InputFocused() {
		t.Fatal("/ in the sidebar did not open the popup and take input")
	}
	m = typeText(t, m, "jk")
	if m.jump.query != "jk" || m.selectedID() != "s1" {
		t.Fatal("letters typed in the popup were not filter text")
	}
}

func TestJump_RowShowsReasonOrDetail(t *testing.T) {
	m := jumpModel(t)
	m.sessions[5].Status = server.SessionStatus{State: server.SessionRateLimited, Kind: "limit", Message: "resets 5pm"}
	view := plainView(m)
	for _, want := range []string{"crashed", "limit: resets 5pm", "dotfiles"} {
		if !strings.Contains(view, want) {
			t.Fatalf("popup missing %q:\n%s", want, view)
		}
	}
}
