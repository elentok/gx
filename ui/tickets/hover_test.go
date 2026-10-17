package tickets

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
)

// hoverFixture drives the Tickets and Queue tabs through the same hover cases.
type hoverFixture struct {
	model    tea.Model
	selected int // the selected entry
	other    int // a selectable entry that is not selected
	outside  ui.Rect
	rowY     func(entry int) int
	state    func(tea.Model) (selected, hovered int, hovering bool)
}

func ticketsHoverFixture(t *testing.T) hoverFixture {
	t.Helper()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second.md", "Status: open\n\nBody.\n")
	m := deliverLoad(t, NewModel(root, ui.Settings{}, keys.New(nil)))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	px, py, pw, ph := m.previewRect()
	other := -1
	for i, e := range m.sidebarTree.Entries() {
		if _, ok := rowFromEntry(e); ok && i != m.sidebarTree.SelectedIndex() {
			other = i
		}
	}
	return hoverFixture{
		model:    m,
		selected: m.sidebarTree.SelectedIndex(),
		other:    other,
		outside:  ui.Rect{X: px, Y: py, W: pw, H: ph},
		rowY:     func(entry int) int { return entry + 1 },
		state: func(tm tea.Model) (int, int, bool) {
			tree := tm.(Model).sidebarTree
			hovered, ok := tree.HoveredIndex()
			return tree.SelectedIndex(), hovered, ok
		},
	}
}

func queueHoverFixture(t *testing.T) hoverFixture {
	t.Helper()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"):  true,
		ticketPath(root, "alpha", "02-second.md"): true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	other := -1
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket && i != m.queueTree.SelectedIndex() {
			other = i
		}
	}
	return hoverFixture{
		model:    m,
		selected: m.queueTree.SelectedIndex(),
		other:    other,
		outside:  m.queuePreviewRect(),
		rowY:     func(entry int) int { return 1 + queueHeaderReservedLines + entry },
		state: func(tm tea.Model) (int, int, bool) {
			tree := tm.(QueueModel).queueTree
			hovered, ok := tree.HoveredIndex()
			return tree.SelectedIndex(), hovered, ok
		},
	}
}

func eachHoverTab(t *testing.T, run func(t *testing.T, f hoverFixture)) {
	t.Helper()
	for name, build := range map[string]func(*testing.T) hoverFixture{
		"tickets": ticketsHoverFixture,
		"queue":   queueHoverFixture,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := build(t)
			if f.other < 0 || f.other == f.selected {
				t.Fatalf("fixture has no unselected row (selected %d, other %d)", f.selected, f.other)
			}
			run(t, f)
		})
	}
}

func backgroundSeq(c color.Color) string {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", n.R, n.G, n.B)
}

var (
	hoverBG     = backgroundSeq(ui.ColorHover)
	selectionBG = backgroundSeq(ui.ColorSurface)
)

func viewLines(m tea.Model) []string {
	return strings.Split(m.View().Content, "\n")
}

func move(m tea.Model, x, y int) tea.Model {
	next, _ := m.Update(tea.MouseMotionMsg{X: x, Y: y})
	return next
}

func TestHover_HighlightsRowUnderMouseWithoutMovingSelection(t *testing.T) {
	t.Parallel()
	eachHoverTab(t, func(t *testing.T, f hoverFixture) {
		m := move(f.model, 2, f.rowY(f.other))

		selected, hovered, ok := f.state(m)
		if !ok || hovered != f.other {
			t.Fatalf("hovered = %d (ok %v), want %d", hovered, ok, f.other)
		}
		if selected != f.selected {
			t.Fatalf("selection moved to %d, want %d", selected, f.selected)
		}
		lines := viewLines(m)
		if !strings.Contains(lines[f.rowY(f.other)], hoverBG) {
			t.Fatalf("hovered row has no hover background: %q", lines[f.rowY(f.other)])
		}
		hoveredLines := 0
		for _, line := range lines {
			if strings.Contains(line, hoverBG) {
				hoveredLines++
			}
		}
		if hoveredLines != 1 {
			t.Fatalf("hover background on %d lines, want only the hovered row", hoveredLines)
		}
		if !strings.Contains(lines[f.rowY(f.selected)], selectionBG) {
			t.Fatalf("selected row lost its selection background: %q", lines[f.rowY(f.selected)])
		}
	})
}

func TestHover_SelectionStyleWinsOnSelectedRow(t *testing.T) {
	t.Parallel()
	eachHoverTab(t, func(t *testing.T, f hoverFixture) {
		m := move(f.model, 2, f.rowY(f.selected))

		content := m.View().Content
		if strings.Contains(content, hoverBG) {
			t.Fatalf("hover background drawn while hovering the selected row")
		}
		if !strings.Contains(viewLines(m)[f.rowY(f.selected)], selectionBG) {
			t.Fatalf("selected row lost its selection background")
		}
	})
}

func TestHover_ClearsWhenLeavingTheList(t *testing.T) {
	t.Parallel()
	eachHoverTab(t, func(t *testing.T, f hoverFixture) {
		for name, pos := range map[string][2]int{
			"preview": {f.outside.X + 2, f.outside.Y + 2},
			"chrome":  {2, -1},
		} {
			m := move(f.model, 2, f.rowY(f.other))
			m = move(m, pos[0], pos[1])
			if _, _, ok := f.state(m); ok {
				t.Fatalf("%s: hover kept after leaving the list", name)
			}
			if strings.Contains(m.View().Content, hoverBG) {
				t.Fatalf("%s: hover background kept after leaving the list", name)
			}
		}
	})
}

func TestHover_ClearsWhenModalOpens(t *testing.T) {
	t.Parallel()
	eachHoverTab(t, func(t *testing.T, f hoverFixture) {
		m := move(f.model, 2, f.rowY(f.other))
		m, _ = m.Update(nav.OpenHelp()())

		if _, _, ok := f.state(m); ok {
			t.Fatalf("hover kept after a modal opened")
		}
		if hp := m.(ui.HoverPage); hp.HoverChanges(tea.MouseMotionMsg{X: 2, Y: f.rowY(f.other)}) {
			t.Fatalf("motion under a modal would re-render")
		}
	})
}

func TestHover_OnlyARowChangeReRenders(t *testing.T) {
	t.Parallel()
	eachHoverTab(t, func(t *testing.T, f hoverFixture) {
		m := move(f.model, 2, f.rowY(f.other))
		hp := m.(ui.HoverPage)

		if hp.HoverChanges(tea.MouseMotionMsg{X: 5, Y: f.rowY(f.other)}) {
			t.Fatalf("motion within the hovered row would re-render")
		}
		if !hp.HoverChanges(tea.MouseMotionMsg{X: 2, Y: f.rowY(f.selected)}) {
			t.Fatalf("motion onto another row would not re-render")
		}
		if _, _, ok := f.state(m); !ok {
			t.Fatalf("HoverChanges moved the model's own hover")
		}
	})
}

func TestHover_TabsUseAllMotionMouseMode(t *testing.T) {
	t.Parallel()
	eachHoverTab(t, func(t *testing.T, f hoverFixture) {
		if got := f.model.View().MouseMode; got != tea.MouseModeAllMotion {
			t.Fatalf("mouse mode = %v, want all-motion", got)
		}
	})
}
