package agents

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
)

func backgroundSeq(c color.Color) string {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", n.R, n.G, n.B)
}

var hoverBG = backgroundSeq(ui.ColorHover)

// Row 0 is the header, then the gx project's header, s1 (selected), s3, s4.
const (
	selectedRowY = 2
	otherRowY    = 4
)

func hoverModel(t *testing.T) Model {
	return update1(t, sidebarModel(t, false, fixtureSessions(), 70, 14), tea.MouseMotionMsg{X: 5, Y: otherRowY})
}

func hoverLines(m Model) []string {
	var lines []string
	for i, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, hoverBG) {
			lines = append(lines, fmt.Sprint(i))
		}
	}
	return lines
}

func TestHover_HighlightsSessionUnderMouseWithoutMovingSelection(t *testing.T) {
	m := hoverModel(t)
	if got := m.selectedID(); got != "s1" {
		t.Fatalf("selection moved to %s, want s1", got)
	}
	if got := strings.Join(hoverLines(m), " "); got != fmt.Sprint(otherRowY) {
		t.Fatalf("hover background on lines %q, want only line %d", got, otherRowY)
	}
}

func TestHover_SelectionStyleWinsOnSelectedRow(t *testing.T) {
	m := update1(t, hoverModel(t), tea.MouseMotionMsg{X: 5, Y: selectedRowY})
	if got := hoverLines(m); len(got) != 0 {
		t.Fatalf("hover background on lines %v while hovering the selected row", got)
	}
}

func TestHover_ClearsWhenLeavingTheList(t *testing.T) {
	for name, pos := range map[string][2]int{
		"terminal":       {40, otherRowY},
		"project header": {5, 1},
		"chrome":         {5, -1},
	} {
		m := update1(t, hoverModel(t), tea.MouseMotionMsg{X: pos[0], Y: pos[1]})
		if _, ok := m.hoveredSession(); ok {
			t.Fatalf("%s: hover kept after leaving the list", name)
		}
		if got := hoverLines(m); len(got) != 0 {
			t.Fatalf("%s: hover background kept on lines %v", name, got)
		}
	}
}

func TestHover_ClearsWhenModalOpens(t *testing.T) {
	m := update1(t, hoverModel(t), nav.OpenHelp()())
	if _, ok := m.hoveredSession(); ok {
		t.Fatal("hover kept after a modal opened")
	}
	if m.HoverChanges(tea.MouseMotionMsg{X: 5, Y: otherRowY}) {
		t.Fatal("motion under a modal would re-render")
	}

	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.ModalOpen() {
		t.Fatal("help still open after esc")
	}
	if _, ok := m.hoveredSession(); ok {
		t.Fatal("hover came back when the modal closed, before the mouse moved")
	}
}

func TestHover_OnlyARowChangeReRenders(t *testing.T) {
	m := hoverModel(t)
	if m.HoverChanges(tea.MouseMotionMsg{X: 9, Y: otherRowY}) {
		t.Fatal("motion within the hovered row would re-render")
	}
	if !m.HoverChanges(tea.MouseMotionMsg{X: 40, Y: 9}) {
		t.Fatal("motion off the list would not re-render")
	}
	if !m.HoverChanges(tea.MouseMotionMsg{X: 5, Y: selectedRowY}) {
		t.Fatal("motion onto another row would not re-render")
	}
	if got, ok := m.hoveredSession(); !ok || m.sessions[got].ID != "s4" {
		t.Fatal("HoverChanges moved the model's own hover")
	}
}

func TestHover_TabUsesAllMotionMouseMode(t *testing.T) {
	if got := hoverModel(t).View().MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("mouse mode = %v, want all-motion", got)
	}
}

// An agent that reports the mouse gets only the wheel, so all-motion changes
// nothing it sees: motion stays with gx and the wheel still reaches it.
func TestHover_AttachedMouseChildStillGetsOnlyTheWheel(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	fa := h.attaches["a"]
	fa.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamFrame, Frame: nativerunner.Frame{Mouse: true}}
	m, _ = read(t, m, next)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	fa.log = nil

	if got := m.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("mouse mode = %v with a mouse child attached, want all-motion", got)
	}
	m = update1(t, m, tea.MouseMotionMsg{X: 40, Y: 4})
	m = update1(t, m, tea.MouseWheelMsg{X: 40, Y: 4, Button: tea.MouseWheelDown})
	if len(fa.log) != 1 || !strings.HasPrefix(fa.log[0], "wheel:") {
		t.Fatalf("agent got %v, want only the wheel", fa.log)
	}
}

// A click over the screen of an agent that reports the mouse reaches it, with
// its release, and focuses the panel.
func TestClick_ReachesAnAgentThatReportsTheMouse(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	fa := h.attaches["a"]
	fa.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamFrame, Frame: nativerunner.Frame{Mouse: true}}
	m, _ = read(t, m, next)
	fa.log = nil

	ox, oy := m.screenOrigin()
	m = update1(t, m, tea.MouseClickMsg{X: ox + 3, Y: oy + 2, Button: tea.MouseLeft})
	m = update1(t, m, tea.MouseMotionMsg{X: ox + 4, Y: oy + 2, Button: tea.MouseLeft})
	m = update1(t, m, tea.MouseReleaseMsg{X: ox + 5, Y: oy + 2, Button: tea.MouseLeft})
	m = update1(t, m, tea.MouseMotionMsg{X: ox + 6, Y: oy + 2})
	want := []string{"mouse:3,2,0,0", "mouse:4,2,2,0", "mouse:5,2,1,0"}
	got := slices.DeleteFunc(slices.Clone(fa.log), func(s string) bool { return !strings.HasPrefix(s, "mouse:") })
	if !slices.Equal(got, want) {
		t.Fatalf("agent got %v, want %v", got, want)
	}
	if !m.terminalFocused {
		t.Error("click on the screen did not focus the terminal")
	}
}

// A click on the sidebar stays with gx even for an agent that reports the mouse.
func TestClick_SidebarStaysWithGxForAMouseAgent(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, next := dialed(t, m, cmd)
	fa := h.attaches["a"]
	fa.msgs <- nativerunner.AttachMessage{Type: nativerunner.StreamFrame, Frame: nativerunner.Frame{Mouse: true}}
	m, _ = read(t, m, next)
	fa.log = nil

	m = update1(t, m, tea.MouseClickMsg{X: 2, Y: 1, Button: tea.MouseLeft})
	for _, l := range fa.log {
		if strings.HasPrefix(l, "mouse:") {
			t.Fatalf("sidebar click reached the agent: %v", fa.log)
		}
	}
}
