package log

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/splitview"
)

func (m Model) View() tea.View {
	if !m.ready || m.listPanel.Rows() == nil {
		return ui.NewMainView("\n  Loading log…")
	}
	if m.err != nil {
		return ui.NewMainView("\n  Error: " + m.err.Error())
	}

	// When detail panel is fullscreened, render it exclusively.
	if m.split.IsFullscreen() && m.split.IsDetailFocused() {
		return m.commitDetail.WithContainerFocus(true).View()
	}

	panel := m.listPanel.
		WithContainerFocus(m.isLogPaneActive()).
		WithHints(m.buildHints())

	listOut := panel.View().Content
	if m.search.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth()
		m.search.SetWidth(overlayW)
		overlay := m.search.View()
		lh := m.listHeight()
		y := m.settings.InputModalBottom.ResolveY(lh, lipgloss.Height(overlay))
		listOut = ui.OverlayBottomCenter(listOut, overlay, m.listWidth(), y)
	}
	if prefix := m.keys.Prefix(); len(prefix) > 0 {
		hints := ui.ChordBindingsFromHints(m.keys.ChordHints())
		if len(hints) > 0 {
			listOut = ui.OverlayBottomRight(listOut, ui.RenderChordOverlay(prefix[0], hints), m.listWidth(), m.listHeight())
		}
	}

	// Compose with detail panel when in split mode.
	var out string
	if m.split.IsSplit() {
		detailContent := m.commitDetail.WithContainerFocus(m.split.IsDetailFocused()).View().Content
		if m.split.EffectiveOrientation() == splitview.Vertical {
			out = lipgloss.JoinHorizontal(lipgloss.Top, listOut, detailContent)
		} else {
			out = lipgloss.JoinVertical(lipgloss.Left, listOut, detailContent)
		}
	} else {
		out = listOut
	}

	if m.rebaseConfirm.isOpen() {
		out = ui.OverlayCenter(out, m.rebaseConfirmView(m.width), m.width, m.height)
	}
	if m.amendConfirm.IsOpen {
		out = ui.OverlayCenter(out, m.amendConfirm.View(m.width), m.width, m.height)
	}
	if m.bump.IsOpen {
		out = ui.OverlayCenter(out, m.bump.View(m.width), m.width, m.height)
	}
	if m.push.IsOpen {
		out = ui.OverlayCenter(out, m.push.View(m.width), m.width, m.height)
	}
	if m.pull.IsOpen {
		out = ui.OverlayCenter(out, m.pull.View(m.width), m.width, m.height)
	}
	if m.output.IsOpen {
		out = ui.OverlayCenter(out, m.output.View(), m.width, m.height)
	}
	if m.reword.IsOpen {
		out = ui.OverlayCenter(out, m.reword.View(m.width), m.width, m.height)
	}
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	return ui.NewMainView(out)
}

func (m Model) logPaneTitleColor() color.Color {
	if m.isLogPaneActive() {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func (m Model) logPaneBorderColor() color.Color {
	if m.isLogPaneActive() {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func (m Model) isLogPaneActive() bool {
	return m.split.IsSplit() && m.split.IsListFocused()
}

func (m Model) frameRightTitle() string {
	searchStatus := m.searchMatchStatus()
	context := m.startRef
	if m.filter.IsActive() {
		if m.filter.StartLine > 0 {
			context = fmt.Sprintf("%s L%d-%d", m.filter.Path, m.filter.StartLine, m.filter.EndLine)
		} else {
			context = m.filter.Path
		}
	}
	return ui.JoinStatus(context, searchStatus)
}

func (m Model) searchMatchStatus() string {
	if m.search.HasQuery() && m.search.MatchesCount() > 0 {
		return fmt.Sprintf("%d/%d matches", m.search.Cursor()+1, m.search.MatchesCount())
	}
	return ""
}

func (m Model) highlightSearch(text string) string {
	query := strings.TrimSpace(m.search.Query())
	if query == "" {
		return text
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if !strings.Contains(lowerText, lowerQuery) {
		return text
	}

	var out strings.Builder
	start := 0
	for start < len(text) {
		idx := strings.Index(lowerText[start:], lowerQuery)
		if idx < 0 {
			out.WriteString(text[start:])
			break
		}
		idx += start
		out.WriteString(text[start:idx])
		end := idx + len(query)
		out.WriteString(logSearchStyle.Render(text[idx:end]))
		start = end
	}
	return out.String()
}

// buildHints assembles the render hints the list panel needs from current page state.
func (m Model) buildHints() listPanelHints {
	title := "Log"
	if m.worktreeRoot != "" {
		title = fmt.Sprintf("Log (%s)", m.worktreeRoot)
	}
	return listPanelHints{
		title:            title,
		rightTitle:       m.frameRightTitle(),
		highlight:        m.highlightSearch,
		flashSubject:     m.flashSubject,
		flashUntil:       m.flashUntil,
		branchDiverged:   m.branchDiverged,
		compiledRefRules: m.compiledRefRules,
		compiledHideRefs: m.compiledHideRefs,
		nerdFont:         m.settings.UseNerdFontIcons,
	}
}

func (m Model) searchOverlayWidth() int {
	outerW := m.width * 80 / 100
	if outerW > 50 {
		outerW = 50
	}
	if outerW < 20 {
		outerW = 20
	}
	return outerW
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
