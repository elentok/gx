package splitview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/commit"
)

// ListScroller is implemented by a tab's own list-panel type (e.g. the log
// or stash list), letting RouteWheel scroll it without depending on any
// tab-specific list type.
type ListScroller[T any] interface {
	ScrollViewport(delta int) T
}

// RouteWheel hit-tests msg against m's hover side and forwards it: on the
// detail side, translated into the embedded commit-detail panel's own local
// coordinate space and forwarded to its Update; on the list side, scrolled
// via the list panel's ScrollViewport. A cursor over neither region, or a
// region with nothing to scroll, returns detail/list unchanged and a nil
// cmd.
func RouteWheel[L ListScroller[L]](m Model, detail commit.Model, list L, msg tea.MouseWheelMsg) (commit.Model, L, tea.Cmd) {
	mouse := msg.Mouse()
	switch m.HoverSideAt(mouse.X, mouse.Y) {
	case HoverDetail:
		col, row, visible := m.DetailOrigin()
		if !visible {
			return detail, list, nil
		}
		translated := tea.MouseWheelMsg{X: mouse.X - col, Y: mouse.Y - row, Button: mouse.Button, Mod: mouse.Mod}
		updated, cmd := detail.Update(translated)
		return updated.(commit.Model), list, cmd
	case HoverList:
		dir, ok := ui.WheelDirection(msg)
		if !ok {
			return detail, list, nil
		}
		return detail, list.ScrollViewport(dir * ui.WheelScrollLines), nil
	default:
		return detail, list, nil
	}
}
