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

	var body string
	if m.diffarea.Fullscreen && m.focus == focusDiff {
		body = m.renderDiffPane(m.width, mainH)
	} else {
		filetreePanel := m.renderLeftPane(filetreeW, filetreeH)
		diffPanel := m.renderDiffPane(diffW, diffH)
		if m.useStackedLayout() {
			body = lipgloss.JoinVertical(lipgloss.Left, filetreePanel, diffPanel)
		} else {
			body = lipgloss.JoinHorizontal(lipgloss.Top, filetreePanel, diffPanel)
		}
	}

	footer := m.helpLine()
	out := lipgloss.JoinVertical(lipgloss.Left, body, footer)
	if m.focus == focusFiletree && m.fileTreeModel.Search().InputFocused() {
		s := m.fileTreeModel.Search()
		s.SetWidth(m.searchOverlayWidth())
		overlay := s.View()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		out = ui.OverlayBottomCenter(out, overlay, m.width, y)
	}
	if m.bump.IsOpen {
		out = ui.OverlayCenter(out, m.bump.View(m.width), m.width, m.height)
	} else if m.push.IsOpen {
		out = ui.OverlayCenter(out, m.push.View(m.width), m.width, m.height)
	} else if m.pull.IsOpen {
		out = ui.OverlayCenter(out, m.pull.View(m.width), m.width, m.height)
	} else if m.credentialOpen {
		out = ui.OverlayCenter(out, m.credentialModalView(), m.width, m.height)
	} else if m.runningOpen {
		out = ui.OverlayCenter(out, m.runningModalView(), m.width, m.height)
	} else if m.output.IsOpen {
		out = ui.OverlayCenter(out, m.output.View(), m.width, m.height)
	} else if m.confirmOpen {
		out = ui.OverlayCenter(out, m.confirmModalView(), m.width, m.height)
	} else if m.errorOpen {
		out = ui.OverlayCenter(out, m.errorModalView(), m.width, m.height)
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
	m.errorVP = vp
	m.errorOpen = true
	m.keys.Reset()
}
