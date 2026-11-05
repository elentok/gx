package status

import (
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Loading status UI…")
	}

	if m.err != nil {
		return ui.NewMainView("\n  Error: " + m.err.Error())
	}

	mainH := m.mainContentHeight()

	filetreeW, diffW := m.splitWidth()
	filetreeH, diffH := m.splitHeight(mainH)

	var out string
	if m.diffarea.Fullscreen && m.focus == focusDiff {
		out = m.renderDiffPane(m.width, mainH)
	} else {
		filetreePanel := m.renderLeftPane(filetreeW, filetreeH)
		diffPanel := m.renderDiffPane(diffW, diffH)
		if m.useStackedLayout() {
			seam := ui.RenderSeamRow(m.width, ui.SeamColor)
			out = lipgloss.JoinVertical(lipgloss.Left, filetreePanel, seam, diffPanel)
		} else {
			seam := ui.RenderSeamColumn(filetreeH, ui.SeamColor)
			out = lipgloss.JoinHorizontal(lipgloss.Top, filetreePanel, seam, diffPanel)
		}
	}

	if gitView, ok := m.git.ViewAction(m.width); ok {
		out = ui.OverlayCenter(out, gitView, m.width, m.height)
	} else if m.modals.credentialOpen {
		out = ui.OverlayCenter(out, m.modals.credentialView(), m.width, m.height)
	} else if m.modals.runningOpen {
		out = ui.OverlayCenter(out, m.modals.runningView(), m.width, m.height)
	} else if m.git.OutputOpen() {
		out = ui.OverlayCenter(out, m.git.ViewOutput(), m.width, m.height)
	} else if m.modals.confirmOpen {
		out = ui.OverlayCenter(out, m.modals.confirmView(m.width), m.width, m.height)
	} else if m.modals.errorOpen {
		out = ui.OverlayCenter(out, m.modals.errorView(), m.width, m.height)
	} else if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	} else if chordHints := m.keys.ChordHints(); len(chordHints) > 0 {
		prefix := strings.Join(m.keys.Prefix(), "")
		bindings := ui.ChordBindingsFromHints(chordHints)
		out = ui.OverlayBottomRight(out, ui.RenderChordOverlay(prefix, bindings), m.width, m.height)
	}
	return ui.NewMainView(out)
}

func (m *Model) showGitError(err error) {
	if err == nil {
		return
	}
	vpW := m.width * 2 / 3
	if vpW < 44 {
		vpW = 44
	}
	if vpW > 96 {
		vpW = 96
	}
	vpH := m.height/2 - 6
	if vpH < 4 {
		vpH = 4
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(err.Error())
	m.modals.errorVP = vp
	m.modals.errorOpen = true
	m.keys.Reset()
}
