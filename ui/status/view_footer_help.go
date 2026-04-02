package stage

import (
	"fmt"
	"strings"

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
		hint := "status · ? help"
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
	hint := "diff: mode:" + modeLabel + " · render:" + m.renderModeLabel() + " · wrap:" + wrapLabel + " · ? help"
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
		icon = "󰍉"
	}
	return fmt.Sprintf("%s %d/%d", icon, idx, len(m.searchMatches))
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
	vp.SetContent(stageHelpText())
	m.helpVP = vp
	m.helpOpen = true
}

func stageHelpText() string {
	return strings.Join([]string{
		"Global",
		"  ?       toggle this help",
		"  q       quit",
		"  cc      open git commit",
		"  ol      open lazygit log",
		"  yy/yl/ya/yf yank content/location/all/filename",
		"  p/P     pull / push",
		"  b       rebase on origin/master",
		"  A       amend last commit (confirm)",
		"",
		"Status Focus",
		"  j / k   move selection",
		"  gg / G  jump top / bottom",
		"  ctrl+u/d scroll half page",
		"  h       collapse open directory",
		"  l       expand directory / open diff on file",
		"  space   stage/unstage file",
		"  d       discard file change (confirm)",
		"  e       edit current file in $EDITOR",
		"  enter   open diff view",
		"  r       refresh",
		"",
		"Diff Focus",
		"  esc/h   return to status",
		"  gg / G  jump top / bottom",
		"  ctrl+u/d scroll half page",
		"  tab     switch unstaged/staged section",
		"  a       toggle hunk/line mode",
		"  v       toggle visual line-range mode",
		"  j / k   move active hunk/line",
		"  , / .   previous/next file",
		"  J / K   scroll diff viewport",
		"  s       toggle unified/side-by-side (hunk-only)",
		"  space   stage/unstage active hunk/line",
		"  d       discard (unstaged) / unstage (staged)",
		"  e       edit current file in $EDITOR",
		"  f       toggle fullscreen diff",
		"  w       toggle soft wrap",
		"  r       refresh",
	}, "\n")
}
