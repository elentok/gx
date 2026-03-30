package stage

import (
	"fmt"
	"strings"

	"gx/git"
	"gx/ui/components"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() tea.View {
	if !m.ready {
		v := tea.NewView("\n  Loading stage UI…")
		v.AltScreen = true
		return v
	}

	if m.err != nil {
		v := tea.NewView("\n  Error: " + m.err.Error())
		v.AltScreen = true
		return v
	}

	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}

	statusW, diffW := m.splitWidth()
	statusH, diffH := m.splitHeight(mainH)

	statusPanel := m.renderStatusPane(statusW, statusH)
	diffPanel := m.renderDiffPane(diffW, diffH)

	var body string
	if m.useStackedLayout() {
		body = lipgloss.JoinVertical(lipgloss.Left, statusPanel, diffPanel)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, statusPanel, diffPanel)
	}

	footer := m.helpLine()
	out := lipgloss.JoinVertical(lipgloss.Left, body, footer)
	if m.runningOpen {
		out = overlayModal(out, m.runningModalView(), m.width, m.height)
	} else if m.confirmOpen {
		out = overlayModal(out, m.confirmModalView(), m.width, m.height)
	} else if m.errorOpen {
		out = overlayModal(out, m.errorModalView(), m.width, m.height)
	} else if m.helpOpen {
		out = overlayModal(out, m.helpModalView(), m.width, m.height)
	}
	v := tea.NewView(out)
	v.AltScreen = true
	v.ReportFocus = true
	return v
}

func (m *Model) showGitError(err error) {
	if err == nil {
		return
	}
	m.setStatus("git command failed")
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
}

func (m Model) splitWidth() (statusW, diffW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}
	statusW = int(float64(m.width) * 0.30)
	if statusW < 20 {
		statusW = 20
	}
	diffW = m.width - statusW
	if diffW < 20 {
		diffW = 20
		statusW = m.width - diffW
	}
	return statusW, diffW
}

func (m Model) splitHeight(total int) (statusH, diffH int) {
	if !m.useStackedLayout() {
		return total, total
	}
	statusH = int(float64(total) * 0.30)
	if statusH < 5 {
		statusH = 5
	}
	diffH = total - statusH
	if diffH < 5 {
		diffH = 5
		statusH = total - diffH
	}
	return statusH, diffH
}

func (m Model) useStackedLayout() bool {
	return m.width <= 100
}

func (m Model) renderStatusPane(width, height int) string {
	innerH := maxInt(1, height-2)
	lines := make([]string, 0, innerH)

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	if len(m.statusEntries) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(catSubtle).Render("clean working tree"))
	} else {
		icons := statusPaneIconsFor(m.settings.UseNerdFontIcons)
		start := m.selected - bodyH/2
		if start < 0 {
			start = 0
		}
		if start > len(m.statusEntries)-bodyH {
			start = len(m.statusEntries) - bodyH
		}
		if start < 0 {
			start = 0
		}
		end := start + bodyH
		if end > len(m.statusEntries) {
			end = len(m.statusEntries)
		}
		for i := start; i < end; i++ {
			entry := m.statusEntries[i]
			mark := " "
			if i == m.selected {
				mark = lipgloss.NewStyle().Foreground(catOrange).Render("▌")
			}
			indent := strings.Repeat("  ", entry.Depth)
			statusColor := statusEntryColor(entry)
			deleted := entry.Kind == statusEntryFile && isDeletedFileStatus(entry.File)
			metaRaw := statusEntryMeta(entry, m.settings.UseNerdFontIcons, icons)
			metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
			if deleted {
				metaStyle = metaStyle.Faint(true)
			}
			meta := metaStyle.Render(metaRaw)
			name := entry.DisplayName
			if entry.Kind == statusEntryDir {
				symbol := icons.folderOpen
				if !entry.Expanded {
					symbol = icons.folderClosed
				}
				name = symbol + " " + name + "/"
			} else {
				if entry.File.IsRenamed() && entry.File.RenameFrom != "" {
					name = entry.File.RenameFrom + " -> " + entry.File.Path
				}
				name = statusFileIcon(entry.File, icons) + " " + name
			}
			if m.searchMatchStatusIndex(i) {
				name = highlightMatchText(name, m.searchQuery)
			}
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
			if deleted {
				nameStyle = nameStyle.Faint(true)
			}
			name = nameStyle.Render(name)
			sep := " "
			if strings.TrimSpace(metaRaw) == "" {
				sep = ""
			}
			line := fmt.Sprintf("%s%s%s%s%s", mark, indent, meta, sep, name)
			if i == m.selected && !deleted {
				line = lipgloss.NewStyle().Bold(true).Render(line)
			}
			lines = append(lines, line)
		}
	}

	for len(lines) < innerH {
		lines = append(lines, "")
	}

	return m.renderPanelWithBorderTitle(width, height, "Status", "", lines, m.focus == focusStatus, sectionUnstaged)
}

func (m *Model) renderDiffPane(width, height int) string {
	hasUnstaged := len(m.unstaged.viewLines) > 0
	hasStaged := len(m.staged.viewLines) > 0

	if !hasUnstaged && !hasStaged {
		content := lipgloss.NewStyle().Foreground(catSubtle).Render("No file selected")
		return m.panelStyle(m.focus == focusDiff).
			Width(width).
			Height(height).
			Render(content)
	}

	if hasUnstaged && !hasStaged {
		return m.renderSectionPane(width, height, "Unstaged", &m.unstaged, sectionUnstaged)
	}
	if hasStaged && !hasUnstaged {
		return m.renderSectionPane(width, height, "Staged", &m.staged, sectionStaged)
	}
	if m.diffFullscreen {
		if m.section == sectionStaged {
			return m.renderSectionPane(width, height, "Staged", &m.staged, sectionStaged)
		}
		return m.renderSectionPane(width, height, "Unstaged", &m.unstaged, sectionUnstaged)
	}

	topH := height / 2
	if topH < 5 {
		topH = 5
	}
	bottomH := height - topH
	if bottomH < 5 {
		bottomH = 5
		topH = height - bottomH
	}

	top := m.renderSectionPane(width, topH, "Unstaged", &m.unstaged, sectionUnstaged)
	bottom := m.renderSectionPane(width, bottomH, "Staged", &m.staged, sectionStaged)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m *Model) renderSectionPane(width, height int, title string, sec *sectionState, section diffSection) string {
	innerW := maxInt(1, width-2)
	innerH := maxInt(1, height-2)

	activeSection := m.focus == focusDiff && m.section == section

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	active := m.activeRawLineIndex(*sec)
	accent := catOrange
	if section == sectionStaged {
		accent = catGreen
	}
	hunkStart, hunkEnd := -1, -1
	if m.navMode == navHunk && sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
		hunkStart = sec.parsed.Hunks[sec.activeHunk].StartLine
		hunkEnd = sec.parsed.Hunks[sec.activeHunk].EndLine
	}
	sec.viewport.SetHeight(maxInt(0, bodyH))
	sec.viewport.SetWidth(innerW)

	titleText := title
	if file, ok := m.selectedFile(); ok && file.IsRenamed() && file.RenameFrom != "" {
		titleText += " [moved: " + file.RenameFrom + " -> " + file.Path + "]"
	}
	if m.diffFullscreen {
		titleText += " [fullscreen]"
	}
	rightTitleText := ""
	if sec.viewport.TotalLineCount() > sec.viewport.VisibleLineCount() && sec.viewport.VisibleLineCount() > 0 {
		pct := int(sec.viewport.ScrollPercent()*100 + 0.5)
		rightTitleText = fmt.Sprintf("%d%%", pct)
	}

	overflowTopDisplay := -1
	overflowBottomDisplay := -1
	if m.navMode == navHunk && activeSection && sec.activeHunk >= 0 {
		if start, end, ok := hunkDisplayBounds(*sec, sec.activeHunk); ok && sec.viewport.VisibleLineCount() > 0 {
			vpTop := sec.viewport.YOffset()
			vpBottom := vpTop + sec.viewport.VisibleLineCount() - 1
			if start < vpTop {
				overflowTopDisplay = vpTop
			}
			if end > vpBottom {
				overflowBottomDisplay = vpBottom
			}
		}
	}
	overflowTopMark, overflowBottomMark, overflowBothMark := m.hunkOverflowMarkers()

	lines := make([]string, 0, bodyH)

	for i := 0; i < bodyH; i++ {
		displayIdx := sec.viewport.YOffset() + i
		if displayIdx >= len(sec.viewLines) {
			lines = append(lines, "")
			continue
		}
		rawIdx := -1
		if displayIdx >= 0 && displayIdx < len(sec.displayToRaw) {
			rawIdx = sec.displayToRaw[displayIdx]
		}
		mark := "  "
		if m.navMode == navLine && sec.visualActive && m.visualMatchDiffDisplay(*sec, displayIdx) {
			mark = lipgloss.NewStyle().Foreground(accent).Render("▎ ")
		}
		inActiveHunk := rawIdx >= 0 && m.navMode == navHunk && rawIdx >= hunkStart && rawIdx <= hunkEnd
		if inActiveHunk && activeSection {
			mark = lipgloss.NewStyle().Foreground(accent).Render("▌ ")
		}
		if rawIdx >= 0 && rawIdx == active && activeSection {
			mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▌ ")
		}
		if inActiveHunk {
			if displayIdx == overflowTopDisplay && displayIdx == overflowBottomDisplay {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBothMark)
			} else if displayIdx == overflowTopDisplay {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowTopMark)
			} else if displayIdx == overflowBottomDisplay {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBottomMark)
			}
		}
		if rawIdx >= 0 && m.flashMarker(section, rawIdx, sec) {
			mark = lipgloss.NewStyle().Foreground(catGreen).Bold(true).Render("◆ ")
		}

		indicator := "  "
		if matched, current := m.searchMatchDiffDisplay(section, displayIdx); matched {
			icon := "* "
			if m.settings.UseNerdFontIcons {
				icon = "󰍉 "
			}
			style := lipgloss.NewStyle().Foreground(catYellow).Bold(true)
			if current {
				style = style.Foreground(catGreen)
			}
			indicator = style.Render(icon)
		}

		markW := ansi.StringWidth(mark)
		indicatorW := ansi.StringWidth(indicator)
		bodyW := innerW - markW - indicatorW
		if bodyW < 0 {
			bodyW = 0
		}
		body := ansi.Truncate(sec.viewLines[displayIdx], bodyW, "")
		body += strings.Repeat(" ", maxInt(0, bodyW-ansi.StringWidth(body)))
		lines = append(lines, mark+body+indicator)
	}
	return m.renderPanelWithBorderTitle(width, height, titleText, rightTitleText, lines, activeSection, section)
}

func (m Model) hunkOverflowMarkers() (top, bottom, both string) {
	if m.settings.UseNerdFontIcons {
		return " ", " ", "↕ "
	}
	return "↑ ", "↓ ", "↕ "
}

func (m Model) visualMatchDiffDisplay(sec sectionState, displayIdx int) bool {
	if !sec.visualActive || m.navMode != navLine {
		return false
	}
	if displayIdx < 0 || displayIdx >= len(sec.displayToRaw) {
		return false
	}
	rawIdx := sec.displayToRaw[displayIdx]
	if rawIdx < 0 {
		return false
	}
	start, end := visualLineBounds(sec)
	for i := start; i <= end && i < len(sec.parsed.Changed); i++ {
		if i >= 0 && sec.parsed.Changed[i].LineIndex == rawIdx {
			return true
		}
	}
	return false
}

func (m Model) activeRawLineIndex(sec sectionState) int {
	if m.navMode == navHunk {
		if sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
			return sec.parsed.Hunks[sec.activeHunk].StartLine
		}
		return -1
	}
	if sec.activeLine >= 0 && sec.activeLine < len(sec.parsed.Changed) {
		return sec.parsed.Changed[sec.activeLine].LineIndex
	}
	return -1
}

func (m *Model) syncDiffViewports() {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	_, diffH := m.splitHeight(mainH)
	vpW := maxInt(1, diffW-4)
	wrapWidth := maxInt(1, vpW-2)
	reflowSectionLines(&m.unstaged, wrapWidth, m.wrapSoft)
	reflowSectionLines(&m.staged, wrapWidth, m.wrapSoft)

	hasUnstaged := len(m.unstaged.viewLines) > 0
	hasStaged := len(m.staged.viewLines) > 0
	if m.diffFullscreen {
		if m.section == sectionUnstaged {
			m.unstaged.viewport.SetHeight(maxInt(0, diffH-3))
			m.staged.viewport.SetHeight(0)
		} else {
			m.staged.viewport.SetHeight(maxInt(0, diffH-3))
			m.unstaged.viewport.SetHeight(0)
		}
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
		m.unstaged.viewport.SetContentLines(m.unstaged.viewLines)
		m.staged.viewport.SetContentLines(m.staged.viewLines)
		return
	}

	if hasUnstaged && hasStaged {
		topH := diffH / 2
		if topH < 5 {
			topH = 5
		}
		bottomH := diffH - topH
		if bottomH < 5 {
			bottomH = 5
			topH = diffH - bottomH
		}
		m.unstaged.viewport.SetHeight(maxInt(0, topH-3))
		m.staged.viewport.SetHeight(maxInt(0, bottomH-3))
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
	} else if hasUnstaged {
		m.unstaged.viewport.SetHeight(maxInt(0, diffH-3))
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetHeight(0)
		m.staged.viewport.SetWidth(vpW)
	} else if hasStaged {
		m.staged.viewport.SetHeight(maxInt(0, diffH-3))
		m.staged.viewport.SetWidth(vpW)
		m.unstaged.viewport.SetHeight(0)
		m.unstaged.viewport.SetWidth(vpW)
	} else {
		m.unstaged.viewport.SetHeight(0)
		m.staged.viewport.SetHeight(0)
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
	}
	// Ensure content is set and clamped.
	m.unstaged.viewport.SetContentLines(m.unstaged.viewLines)
	m.staged.viewport.SetContentLines(m.staged.viewLines)
}

func reflowSectionLines(sec *sectionState, wrapWidth int, wrapSoft bool) {
	if len(sec.baseLines) == 0 {
		sec.viewLines = nil
		sec.displayToRaw = nil
		sec.rawToDisplay = buildRawToDisplayMap(sec.parsed, nil)
		sec.viewport.SetContent("")
		sec.viewport.SetYOffset(0)
		return
	}

	prevOffset := sec.viewport.YOffset()
	view := make([]string, 0, len(sec.baseLines))
	mapRaw := make([]int, 0, len(sec.baseDisplayToRaw))

	for i, line := range sec.baseLines {
		rawIdx := -1
		if i < len(sec.baseDisplayToRaw) {
			rawIdx = sec.baseDisplayToRaw[i]
		}
		if !wrapSoft || rawIdx < 0 {
			view = append(view, line)
			mapRaw = append(mapRaw, rawIdx)
			continue
		}
		parts := wrapANSI(line, wrapWidth)
		for _, p := range parts {
			view = append(view, p)
			mapRaw = append(mapRaw, rawIdx)
		}
	}

	sec.viewLines = view
	sec.displayToRaw = mapRaw
	sec.rawToDisplay = buildRawToDisplayMap(sec.parsed, sec.displayToRaw)
	sec.viewport.SetContentLines(sec.viewLines)
	sec.viewport.SetYOffset(prevOffset)
}

func wrapANSI(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	total := ansi.StringWidth(s)
	if total <= width {
		return []string{s}
	}
	out := make([]string, 0, total/width+1)
	for start := 0; start < total; start += width {
		end := start + width
		if end > total {
			end = total
		}
		part := ansi.Cut(s, start, end)
		if part == "" {
			break
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

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
	hint := "diff: mode:" + modeLabel + " · wrap:" + wrapLabel + " · ? help"
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
		"  yc/yf   yank context / filename",
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
		"  J / K   scroll diff viewport",
		"  space   stage/unstage active hunk/line",
		"  d       discard (unstaged) / unstage (staged)",
		"  f       toggle fullscreen diff",
		"  w       toggle soft wrap",
		"  r       refresh",
	}, "\n")
}

func (m Model) errorModalView() string {
	return components.RenderOutputModal(
		"Error",
		m.errorVP.View(),
		"esc / enter dismiss · j/k scroll",
		catRed,
		catRed,
		catSubtle,
		m.errorVP.Width(),
	)
}

func (m Model) helpModalView() string {
	titleStyle := lipgloss.NewStyle().Foreground(catBlue).Bold(true)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(catBlue).
		Padding(0, 1).
		Width(m.helpVP.Width())

	hint := lipgloss.NewStyle().Foreground(catSubtle).Render("? / esc / enter dismiss · j/k scroll")
	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Keyboard Help"),
		"",
		m.helpVP.View(),
		"",
		hint,
	)
	return borderStyle.Render(inner)
}

func overlayModal(bg, modal string, screenW, screenH int) string {
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	x := (screenW - modalW) / 2
	y := (screenH - modalH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return placeOverlay(bg, modal, x, y)
}

func placeOverlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		bgY := y + i
		if bgY < 0 || bgY >= len(bgLines) {
			continue
		}
		bgLine := bgLines[bgY]
		fgW := ansi.StringWidth(fgLine)

		left := ansi.Truncate(bgLine, x, "")
		if leftW := ansi.StringWidth(left); leftW < x {
			left += strings.Repeat(" ", x-leftW)
		}
		right := ansi.TruncateLeft(bgLine, x+fgW, "")
		bgLines[bgY] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}

func (m Model) panelStyle(active bool) lipgloss.Style {
	borderColor := catSubtle
	if active {
		borderColor = catOrange
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(catBase0)
}

func (m Model) renderPanelWithBorderTitle(width, height int, title, rightTitle string, lines []string, active bool, section diffSection) string {
	if width < 2 || height < 2 {
		return ""
	}
	innerW := width - 2
	innerH := height - 2

	borderColor := catSubtle
	titleStyle := lipgloss.NewStyle().Foreground(catBlue)
	if section == sectionStaged {
		borderColor = catGreen
		titleStyle = lipgloss.NewStyle().Foreground(catGreen)
		if active {
			titleStyle = titleStyle.Bold(true)
		}
	} else if active {
		borderColor = catOrange
		titleStyle = lipgloss.NewStyle().Foreground(catOrange).Bold(true)
	}
	border := lipgloss.NewStyle().Foreground(borderColor)

	titleSeg := titleStyle.Render(" " + title + " ")
	rightSeg := ""
	if rightTitle != "" {
		rightSeg = titleStyle.Render(" " + rightTitle + " ")
	}
	titleW := ansi.StringWidth(titleSeg)
	rightW := ansi.StringWidth(rightSeg)
	topInner := ""
	if rightW >= innerW {
		topInner = ansi.Truncate(rightSeg, innerW, "")
	} else if titleW+rightW >= innerW {
		titleSeg = ansi.Truncate(titleSeg, innerW-rightW, "")
		titleW = ansi.StringWidth(titleSeg)
		topInner = titleSeg + rightSeg
	} else if titleW >= innerW {
		topInner = ansi.Truncate(titleSeg, innerW, "")
		titleW = ansi.StringWidth(topInner)
	} else {
		topInner = titleSeg + border.Render(strings.Repeat("─", innerW-titleW-rightW)) + rightSeg
	}
	if titleW+rightW < innerW && !strings.Contains(topInner, "─") {
		topInner += border.Render(strings.Repeat("─", innerW-titleW-rightW))
	}

	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	body := make([]string, 0, innerH)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], innerW, "")
		}
		line = line + strings.Repeat(" ", maxInt(0, innerW-ansi.StringWidth(line)))
		body = append(body, border.Render("│")+line+ansiReset+border.Render("│"))
	}

	bottom := border.Render("╰" + strings.Repeat("─", innerW) + "╯")
	top := border.Render("╭") + topInner + border.Render("╮")
	return strings.Join(append([]string{top}, append(body, bottom)...), "\n")
}

type statusPaneIcons struct {
	folderClosed string
	folderOpen   string
	fileModified string
	fileNew      string
	fileDeleted  string
	fileRenamed  string
	partial      string
	staged       string
}

func statusPaneIconsFor(useNerdFontIcons bool) statusPaneIcons {
	if !useNerdFontIcons {
		return statusPaneIcons{
			folderClosed: "▸",
			folderOpen:   "▾",
			fileModified: "M",
			fileNew:      "N",
			fileDeleted:  "D",
			fileRenamed:  "R",
			partial:      "+",
			staged:       "✓",
		}
	}
	return statusPaneIcons{
		folderClosed: "",
		folderOpen:   "",
		fileModified: "",
		fileNew:      "",
		fileDeleted:  "",
		fileRenamed:  "󰁔",
		partial:      "",
		staged:       "",
	}
}

func statusEntryColor(entry statusEntry) string {
	if entry.Kind == statusEntryFile && isDeletedFileStatus(entry.File) {
		return "#a6adc8"
	}
	if entry.Kind == statusEntryFile && entry.File.IsRenamed() {
		return "#89b4fa"
	}
	if entry.HasStaged && entry.HasUnstaged {
		return "#fab387"
	}
	if entry.HasStaged {
		return "#a6e3a1"
	}
	return "#cdd6f4"
}

func statusEntryMeta(entry statusEntry, useNerdFontIcons bool, icons statusPaneIcons) string {
	if entry.HasStaged && entry.HasUnstaged {
		return icons.partial
	}
	if entry.HasStaged {
		return icons.staged
	}
	if useNerdFontIcons {
		return "  "
	}
	if entry.Kind == statusEntryDir {
		return "-"
	}
	return entry.File.XY()
}

func statusFileIcon(file git.StageFileStatus, icons statusPaneIcons) string {
	if isDeletedFileStatus(file) {
		return icons.fileDeleted
	}
	if file.IsRenamed() {
		return icons.fileRenamed
	}
	if file.IsUntracked() || file.IndexStatus == 'A' {
		return icons.fileNew
	}
	return icons.fileModified
}

func isDeletedFileStatus(file git.StageFileStatus) bool {
	return file.IndexStatus == 'D' || file.WorktreeCode == 'D'
}

func (m Model) flashMarker(section diffSection, rawIdx int, sec *sectionState) bool {
	if !m.flash.active || m.flash.section != section {
		return false
	}
	if m.flash.navMode == navHunk {
		if m.flash.hunk < 0 || m.flash.hunk >= len(sec.parsed.Hunks) {
			return false
		}
		h := sec.parsed.Hunks[m.flash.hunk]
		return rawIdx >= h.StartLine && rawIdx <= h.EndLine
	}
	if m.flash.line < 0 || m.flash.line >= len(sec.parsed.Changed) {
		return false
	}
	return sec.parsed.Changed[m.flash.line].LineIndex == rawIdx
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
