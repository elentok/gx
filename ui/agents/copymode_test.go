package agents

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/nativerunner"
)

var copyRowsText = []string{"$ claude", "foo bar.baz  qux"}

// copyModel is session a attached, its terminal focused, showing
// copyRowsText at offset with scrollMax lines of scrollback, in copy mode.
// It returns the attach's pending read.
func copyModel(t *testing.T, offset, scrollMax int) (Model, *fakeAttach, tea.Cmd) {
	t.Helper()
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	fa := h.attaches["a"]
	fa.scrolledFrame(offset, scrollMax, 0, copyRowsText...)
	m, next = read(t, m, next)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = leaderThen(t, m, letter("["))
	if m.copy == nil || !m.InputFocused() {
		t.Fatal("<leader> [ did not enter copy mode")
	}
	fa.log, fa.keys = nil, nil
	return m, fa, next
}

func pressKeys(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		m = update1(t, m, k)
	}
	return m
}

func letters(s string) []tea.KeyPressMsg {
	var keys []tea.KeyPressMsg
	for _, r := range s {
		keys = append(keys, letter(string(r)))
	}
	return keys
}

func TestCopyMode_StartsOnTheLastNonBlankRow(t *testing.T) {
	m, _, _ := copyModel(t, 0, 100)
	if want := (nativerunner.LinePos{Line: 101}); m.copy.cursor != want {
		t.Fatalf("cursor = %+v, want %+v", m.copy.cursor, want)
	}
}

func TestCopyMode_Moves(t *testing.T) {
	ctrl := func(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want nativerunner.LinePos
	}{
		{"l l h", letters("llh"), nativerunner.LinePos{Line: 101, Col: 1}},
		{"arrows", []tea.KeyPressMsg{{Code: tea.KeyRight}, {Code: tea.KeyRight}, {Code: tea.KeyUp}, {Code: tea.KeyLeft}}, nativerunner.LinePos{Line: 100, Col: 1}},
		{"k j j", letters("kjj"), nativerunner.LinePos{Line: 102}},
		{"$", letters("$"), nativerunner.LinePos{Line: 101, Col: 15}},
		{"0", letters("$0"), nativerunner.LinePos{Line: 101}},
		{"w over words and punctuation", letters("www"), nativerunner.LinePos{Line: 101, Col: 8}},
		{"w over a double space", letters("wwww"), nativerunner.LinePos{Line: 101, Col: 13}},
		{"e", letters("e"), nativerunner.LinePos{Line: 101, Col: 2}},
		{"e to the end of a word", letters("wwwe"), nativerunner.LinePos{Line: 101, Col: 10}},
		{"b", letters("$b"), nativerunner.LinePos{Line: 101, Col: 13}},
		{"b back across a line", letters("b"), nativerunner.LinePos{Line: 100, Col: 2}},
		{"w forward across a line", letters("kww"), nativerunner.LinePos{Line: 101}},
		{"ctrl+u", []tea.KeyPressMsg{ctrl('u')}, nativerunner.LinePos{Line: 96}},
		{"ctrl+d", []tea.KeyPressMsg{ctrl('d')}, nativerunner.LinePos{Line: 106}},
		{"PgUp", []tea.KeyPressMsg{{Code: tea.KeyPgUp}}, nativerunner.LinePos{Line: 91}},
		{"PgDn clamps to the last line", []tea.KeyPressMsg{{Code: tea.KeyPgDown}}, nativerunner.LinePos{Line: 109}},
		{"g g", letters("lgg"), nativerunner.LinePos{}},
		{"G", letters("lkG"), nativerunner.LinePos{Line: 109}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, _ := copyModel(t, 0, 100)
			m = pressKeys(t, m, tt.keys...)
			if m.copy.cursor != tt.want {
				t.Fatalf("cursor = %+v, want %+v", m.copy.cursor, tt.want)
			}
		})
	}
}

func TestCopyMode_CursorPastTheViewScrollsIt(t *testing.T) {
	m, fa, _ := copyModel(t, 0, 100)
	m = pressKeys(t, m, letters("kk")...)
	m = pressKeys(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = pressKeys(t, m, letters("gg")...)
	pressKeys(t, m, letters("G")...)
	if want := []string{"scroll:1", "scroll:11", "scroll:100", "scroll:0"}; !slices.Equal(fa.log, want) {
		t.Fatalf("sent %v, want %v", fa.log, want)
	}
}

func TestCopyMode_RendersTheSelection(t *testing.T) {
	m, _, _ := copyModel(t, 0, 100)
	m = pressKeys(t, m, letters("vll")...)
	if view := m.View().Content; !strings.Contains(view, styleCopySelection.Render("fo")) {
		t.Fatalf("v selection not drawn:\n%q", view)
	}

	m = pressKeys(t, m, letters("V")...)
	cols, _ := m.screenSize()
	// The cursor, still on the third cell, splits the line's highlight.
	selected := " bar.baz  qux" + strings.Repeat(" ", cols-len(copyRowsText[1]))
	if view := m.View().Content; !strings.Contains(view, styleCopySelection.Render(selected)) {
		t.Fatalf("V selection not drawn over the whole line:\n%q", view)
	}
}

func TestCopyMode_YankCopiesTheRangeAndWritesTheReply(t *testing.T) {
	m, fa, next := copyModel(t, 0, 100)
	var clipped []string
	m.clipWrite = func(s string) error { clipped = append(clipped, s); return nil }
	m = pressKeys(t, m, letters("wve")...)
	m = pressKeys(t, m, letters("y")...)
	if want := []string{"copy:101.4-101.7"}; !slices.Equal(fa.log, want) {
		t.Fatalf("y sent %v, want %v", fa.log, want)
	}
	if m.copy != nil {
		t.Fatal("y did not leave copy mode")
	}

	fa.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamCopy, Text: "bar"}
	read(t, m, next)
	if !slices.Equal(clipped, []string{"bar"}) {
		t.Fatalf("clipboard got %v, want [bar]", clipped)
	}
}

func TestCopyMode_EnterCopiesALineSelection(t *testing.T) {
	m, fa, _ := copyModel(t, 0, 100)
	m = pressKeys(t, m, letters("kVj")...)
	pressKeys(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if want := []string{"copy:100.0-101.1073741824"}; !slices.Equal(fa.log, want) {
		t.Fatalf("enter sent %v, want %v", fa.log, want)
	}
}

func TestCopyMode_EscClearsTheSelectionThenLeaves(t *testing.T) {
	m, _, _ := copyModel(t, 0, 100)
	m = pressKeys(t, m, letters("v")...)
	m = pressKeys(t, m, esc)
	if m.copy == nil || m.copy.selecting {
		t.Fatal("esc with a selection did not just clear it")
	}
	m = pressKeys(t, m, esc)
	if m.copy != nil {
		t.Fatal("esc without a selection did not leave")
	}

	m, _, _ = copyModel(t, 0, 100)
	if m = pressKeys(t, m, letters("q")...); m.copy != nil {
		t.Fatal("q did not leave")
	}
}

func TestCopyMode_LeavingReturnsToTheBottom(t *testing.T) {
	m, fa, _ := copyModel(t, 5, 100)
	m = pressKeys(t, m, letters("q")...)
	if want := []string{"scroll:0"}; !slices.Equal(fa.log, want) {
		t.Fatalf("leaving sent %v, want %v", fa.log, want)
	}
	if view := plainView(m); strings.Contains(view, "scrolled") || strings.Contains(view, "copy") {
		t.Fatalf("header still shows scrolled or copy after leaving:\n%s", view)
	}
}

func TestCopyMode_SendsNoInputToTheSession(t *testing.T) {
	m, fa, _ := copyModel(t, 0, 100)
	m = pressKeys(t, m, letters("xaiz:")...)
	m = pressKeys(t, m, ctrlA, letter("["), tea.KeyPressMsg{Code: tea.KeyTab})
	update1(t, m, tea.PasteMsg{Content: "p"})
	if len(fa.keys) != 0 || len(fa.pastes) != 0 {
		t.Fatalf("copy mode sent keys %v and pastes %v", fa.keys, fa.pastes)
	}
	if m.leaderPending || m.copy == nil {
		t.Fatal("the leader escaped copy mode")
	}
}
