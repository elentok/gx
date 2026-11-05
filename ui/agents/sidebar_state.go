package agents

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ui"
)

// sidebar owns the sidebar's own state: which groups are listed, the pending
// `t` chord, the mouse position and the render memo. The session list and the
// cursor stay in Model and come in as sidebarInput.
type sidebar struct {
	// showQueue and showDesign: the Queue and Design groups are listed, not
	// folded into a count; remembered in groupsPath.
	showQueue, showDesign bool
	groupsPath            string
	// togglePending: `t` was pressed and the next key picks the group.
	togglePending bool
	// hoverAt is the last mouse position; its zero value is on the panel
	// header, so no row is hovered.
	hoverAt tea.Mouse
	// memo lets a frame that changes nothing visible skip the render and its
	// width measuring.
	memo *ui.Memo
}

// sidebarInput is what the sidebar reads from Model on each call.
type sidebarInput struct {
	sessions        []wire.SessionInfo
	cursor          int
	spinner         string
	nerdFont        bool
	height          int
	onScreen        bool
	modalOpen       bool
	terminalFocused bool
	afk             bool
}

func (m Model) sidebarInput() sidebarInput {
	return sidebarInput{
		sessions:        m.sessions,
		cursor:          m.cursor,
		spinner:         m.spinner.View(),
		nerdFont:        m.settings.UseNerdFontIcons,
		height:          m.height,
		onScreen:        m.sidebarOnScreen(),
		modalOpen:       m.ModalOpen(),
		terminalFocused: m.terminalFocused,
		afk:             m.afk,
	}
}
