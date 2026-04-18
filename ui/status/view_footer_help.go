package stage

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) helpLine() string {
	if m.searchMode != searchModeNone {
		prefix := ""
		if m.focus == focusDiff && m.currentSection().visualActive {
			prefix = "VISUAL · "
		}
		line := lipgloss.NewStyle().Foreground(catSubtle).Render("  " + prefix + m.searchFooterText())
		if m.width > 0 {
			line = ansi.Truncate(line, m.width, "")
		}
		return line
	}
	if m.focus == focusStatus {
		hint := m.diffContextLabel() + " · " + m.helpSectionLabel()
		if t := m.terminalLabel(); t != "" {
			hint += " · " + t
		}
		hint += " · ? help"
		if hs := m.footerShortHelp(); hs != "" {
			hint += " · " + hs
		}
		if s := m.searchCounterLabel(); s != "" {
			hint = s + " · " + hint
		}
		return m.renderFooterLine(hint)
	}
	modeLabel := "hunk"
	if m.navMode == navLine {
		modeLabel = "line"
	}
	wrapLabel := "off"
	if m.wrapSoft {
		wrapLabel = "on"
	}
	hint := m.diffContextLabel() + " · diff: mode:" + modeLabel + " · render:" + m.renderModeLabel() + " · wrap:" + wrapLabel
	if t := m.terminalLabel(); t != "" {
		hint += " · " + t
	}
	hint += " · ? help"
	if hs := m.footerShortHelp(); hs != "" {
		hint += " · " + hs
	}
	if s := m.searchCounterLabel(); s != "" {
		hint = s + " · " + hint
	}
	if m.currentSection().visualActive {
		return m.renderFooterLineWithPrefix("VISUAL", hint)
	}
	return m.renderFooterLine(hint)
}

func (m Model) searchCounterLabel() string {
	if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 {
		return ""
	}
	idx := m.searchCursor + 1
	if idx < 1 {
		idx = 1
	}
	if idx > len(m.searchMatches) {
		idx = len(m.searchMatches)
	}
	icon := "*"
	if m.settings.UseNerdFontIcons {
		icon = ui.Icons(true).Search
	}
	return fmt.Sprintf("%s %d/%d", icon, idx, len(m.searchMatches))
}

func (m Model) diffContextLabel() string {
	if m.settings.UseNerdFontIcons {
		return fmt.Sprintf("󰉸 context: %d", m.currentDiffContextLines())
	}
	return fmt.Sprintf("context: %d", m.currentDiffContextLines())
}

func (m Model) renderFooterLine(hint string) string {
	return m.renderFooterLineWithPrefix("", hint)
}

func (m Model) renderFooterLineWithPrefix(prefix, hint string) string {
	hintText := "· " + hint
	hintStyled := lipgloss.NewStyle().Foreground(catSubtle).Render(hintText)
	leftText := ""
	if prefix != "" {
		leftText = prefix
	}
	if m.statusMsg != "" {
		if leftText != "" {
			leftText += " · "
		}
		leftText += m.statusMsg
	}
	lineW := m.width
	if lineW <= 0 {
		if leftText == "" {
			return hintStyled
		}
		return leftText + "  " + hintStyled
	}

	hintW := ansi.StringWidth(hintText)
	if leftText == "" {
		if hintW >= lineW {
			return ansi.Truncate(hintStyled, lineW, "")
		}
		return strings.Repeat(" ", lineW-hintW) + hintStyled
	}

	sep := "  "
	sepW := ansi.StringWidth(sep)
	statusMax := lineW - hintW - sepW
	if statusMax <= 0 {
		if leftText != "" {
			left := leftText + sep
			leftW := ansi.StringWidth(left)
			if leftW >= lineW {
				return ansi.Truncate(leftText, lineW, "...")
			}
			return left + ansi.Truncate(hintStyled, lineW-leftW, "")
		}
		if hintW >= lineW {
			return ansi.Truncate(hintStyled, lineW, "")
		}
		return strings.Repeat(" ", lineW-hintW) + hintStyled
	}

	status := ansi.Truncate(leftText, statusMax, "...")
	left := status + sep
	leftW := ansi.StringWidth(left)
	if leftW+hintW >= lineW {
		return left + hintStyled
	}
	return left + strings.Repeat(" ", lineW-leftW-hintW) + hintStyled
}

func (m Model) terminalLabel() string {
	return m.settings.Terminal.String()
}

func (m *Model) showHelpOverlay() {
	vpW := m.width * 2 / 3
	if vpW < 56 {
		vpW = 56
	}
	if vpW > 104 {
		vpW = 104
	}
	vpH := m.height/2 - 4
	if vpH < 8 {
		vpH = 8
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(m.helpFullView(vpW - 2))
	m.helpVP = vp
	m.helpOpen = true
}

func (m Model) footerShortHelp() string {
	if m.statusMsg != "" || m.width < 110 {
		return ""
	}
	return m.helpShortView()
}
