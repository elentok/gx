package agents

import (
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ui/contextmenu"
)

// selected is the session under the cursor; ok is false when there is none.
func (m Model) selected() (wire.SessionInfo, bool) {
	if m.actions.client == nil || m.cursor >= len(m.sessions) {
		return wire.SessionInfo{}, false
	}
	return m.sessions[m.cursor], true
}

// menuAnchor is the selected session's sidebar row, under the panel's
// header line; the zero Anchor (a centered menu) while the sidebar is
// hidden.
func (m Model) menuAnchor() contextmenu.Anchor {
	if !m.sidebarOnScreen() {
		return contextmenu.Anchor{}
	}
	for i, r := range m.sidebar.visibleRows(m.sidebarInput()) {
		if r.session == m.cursor {
			return contextmenu.Anchor{X: 1, Y: 1 + i, H: 1}
		}
	}
	return contextmenu.Anchor{}
}
