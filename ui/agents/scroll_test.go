package agents

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/nativerunner"
)

// scrolledFrame queues a full frame at a scroll offset.
func (f *fakeAttach) scrolledFrame(offset, scrollMax, newLines int, rows ...string) {
	fr := nativerunner.Frame{ScrollOffset: offset, ScrollMax: scrollMax, NewLines: newLines}
	for i, r := range rows {
		fr.Rows = append(fr.Rows, nativerunner.FrameRow{Index: i, ANSI: r})
	}
	f.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamFrame, Frame: fr}
}

// scrollModel is attachModel with session a attached and one frame read,
// which has scrollMax lines of scrollback at offset.
func scrollModel(t *testing.T, offset, scrollMax, newLines int) (Model, *fakeAttach) {
	t.Helper()
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	fa := h.attaches["a"]
	fa.scrolledFrame(offset, scrollMax, newLines, "$ claude")
	m, _ = read(t, m, next)
	return m, fa
}

func wheel(x, y int, button tea.MouseButton) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: button}
}

func TestScroll_WheelOverThePanelScrollsInEitherFocus(t *testing.T) {
	m, fa := scrollModel(t, 0, 100, 0)
	panelX := sidebarWidth + 5
	m = update1(t, m, wheel(panelX, 4, tea.MouseWheelUp))
	if want := []string{"scroll:3"}; !slices.Equal(fa.log, want) {
		t.Fatalf("wheel up over the panel with the sidebar focused sent %v, want %v", fa.log, want)
	}

	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fa.log = nil
	m = update1(t, m, wheel(panelX, 4, tea.MouseWheelUp))
	m = update1(t, m, wheel(panelX, 4, tea.MouseWheelDown))
	if want := []string{"scroll:6", "scroll:3"}; !slices.Equal(fa.log, want) {
		t.Fatalf("wheel over the focused panel sent %v, want %v", fa.log, want)
	}
}

func TestScroll_WheelGoesToAChildThatReportsTheMouse(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	fa := h.attaches["a"]
	fa.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamFrame, Frame: nativerunner.Frame{
		Mouse: true, ScrollMax: 100, Rows: []nativerunner.FrameRow{{Index: 0, ANSI: "claude"}},
	}}
	m, _ = read(t, m, next)

	ox, oy := m.screenOrigin()
	m = update1(t, m, wheel(ox+3, oy+2, tea.MouseWheelUp))
	m = update1(t, m, wheel(ox+3, oy+2, tea.MouseWheelDown))
	// Over the panel's header, off the screen: nowhere to send it.
	update1(t, m, wheel(ox+3, 0, tea.MouseWheelUp))
	if want := []string{"wheel:3,2,false", "wheel:3,2,true"}; !slices.Equal(fa.log, want) {
		t.Fatalf("wheel sent %v, want %v", fa.log, want)
	}
}

func TestScroll_WheelStaysInsideTheScrollback(t *testing.T) {
	m, fa := scrollModel(t, 0, 4, 0)
	panelX := sidebarWidth + 5
	m = update1(t, m, wheel(panelX, 4, tea.MouseWheelDown))
	m = update1(t, m, wheel(panelX, 4, tea.MouseWheelUp))
	update1(t, m, wheel(panelX, 4, tea.MouseWheelUp))
	if want := []string{"scroll:3", "scroll:4"}; !slices.Equal(fa.log, want) {
		t.Fatalf("wheel sent %v, want %v", fa.log, want)
	}
}

func TestScroll_WheelOverTheSidebarDoesNotScroll(t *testing.T) {
	m, fa := scrollModel(t, 0, 100, 0)
	update1(t, m, wheel(2, 4, tea.MouseWheelUp))
	if len(fa.log) != 0 {
		t.Fatalf("wheel over the sidebar sent %v", fa.log)
	}
}

func TestScroll_HeaderShowsScrolledAndNewBadges(t *testing.T) {
	m, _ := scrollModel(t, 0, 100, 0)
	if view := plainView(m); strings.Contains(view, "scrolled") || strings.Contains(view, "new") {
		t.Fatalf("badge shown at the bottom:\n%s", view)
	}

	m, _ = scrollModel(t, 5, 100, 0)
	if view := plainView(m); !strings.Contains(view, "scrolled ↑ 5") || strings.Contains(view, "new") {
		t.Fatalf("want only the scrolled badge:\n%s", view)
	}

	m, _ = scrollModel(t, 8, 100, 3)
	if view := plainView(m); !strings.Contains(view, "scrolled ↑ 8") || !strings.Contains(view, "+3 new") {
		t.Fatalf("want the scrolled and new badges:\n%s", view)
	}
}

func TestScroll_KeyWhileScrolledSnapsToTheBottomThenTypes(t *testing.T) {
	m, fa := scrollModel(t, 5, 100, 2)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fa.log = nil
	m = update1(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = update1(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if want := []string{"scroll:0", "key:x", "key:y"}; !slices.Equal(fa.log, want) {
		t.Fatalf("typing while scrolled sent %v, want %v", fa.log, want)
	}
	if view := plainView(m); strings.Contains(view, "scrolled") {
		t.Fatalf("badge kept after snapping to the bottom:\n%s", view)
	}

	update1(t, m, tea.PasteMsg{Content: "p"})
	if want := []string{"scroll:0", "key:x", "key:y"}; !slices.Equal(fa.log, want) {
		t.Fatalf("a paste at the bottom scrolled again: %v", fa.log)
	}
}

func TestScroll_PasteWhileScrolledSnapsToTheBottom(t *testing.T) {
	m, fa := scrollModel(t, 5, 100, 0)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fa.log = nil
	update1(t, m, tea.PasteMsg{Content: "p"})
	if want := []string{"scroll:0"}; !slices.Equal(fa.log, want) || !slices.Equal(fa.pastes, []string{"p"}) {
		t.Fatalf("paste while scrolled sent %v and pastes %v", fa.log, fa.pastes)
	}
}
