package agents

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ui/nav"
)

func TestAttach_LiveViewKeepsTheFrameColours(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	h.attaches["a"].frame("\x1b[31mred")
	m, _ = read(t, m, next)

	content := m.View().Content
	i := strings.Index(content, "\x1b[31m")
	if i < 0 || !strings.Contains(content[i:], "red") {
		t.Fatalf("the red SGR did not reach the live view:\n%q", content)
	}
}

// cursorFrame is a frame carrying a cursor and a scroll offset.
func cursorFrame(x, y, offset int) holder.AttachMessage {
	return holder.AttachMessage{Type: holder.StreamFrame, Frame: holder.Frame{
		Rows:         []holder.FrameRow{{Index: 0, ANSI: "$"}},
		Cursor:       holder.FrameCursor{X: x, Y: y, Visible: true},
		ScrollOffset: offset,
	}}
}

func TestAttach_CursorFollowsTheFrameWhileTheTerminalIsFocused(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	fa := h.attaches["a"]
	fa.msgs <- cursorFrame(4, 2, 0)
	m, next = read(t, m, next)

	if c := m.View().Cursor; c != nil {
		t.Fatalf("cursor %+v shown while the sidebar has focus", c)
	}

	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	ox, oy := m.termView().ScreenOrigin()
	c := m.View().Cursor
	if c == nil || c.X != ox+4 || c.Y != oy+2 {
		t.Fatalf("cursor = %+v, want (%d,%d)", c, ox+4, oy+2)
	}

	fa.msgs <- cursorFrame(4, 2, 3)
	scrolled, _ := read(t, m, next)
	if c := scrolled.View().Cursor; c != nil {
		t.Fatalf("cursor %+v shown while scrolled up", c)
	}

	covered := update1(t, m, nav.OpenHelp()())
	if !covered.ModalOpen() {
		t.Fatal("help did not open")
	}
	if c := covered.View().Cursor; c != nil {
		t.Fatalf("cursor %+v shown under a modal", c)
	}
}
