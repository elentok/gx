package status

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	minStatusPaneWidth   = 30
	maxStatusPaneWidth   = 72
	minDiffPaneWidth     = 60
	minCommitsPaneHeight = 5
	maxCommitsPaneHeight = 20
	minStatusPaneHeight  = 5
)

func (m Model) splitWidth() (statusW, diffW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}

	statusH, _ := m.leftPaneHeights(m.mainContentHeight(), minStatusPaneWidth)
	statusW = m.requiredStatusPaneWidth(statusH)
	statusMax := minInt(maxStatusPaneWidth, int(float64(m.width)*0.45))
	if statusMax < minStatusPaneWidth {
		statusMax = minStatusPaneWidth
	}
	if statusW < minStatusPaneWidth {
		statusW = minStatusPaneWidth
	}
	if statusW > statusMax {
		statusW = statusMax
	}
	if m.width-statusW < minDiffPaneWidth {
		statusW = m.width - minDiffPaneWidth
	}
	if statusW < minStatusPaneWidth {
		statusW = minStatusPaneWidth
	}
	diffW = m.width - statusW
	if diffW < minDiffPaneWidth {
		diffW = minDiffPaneWidth
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

func (m Model) mainContentHeight() int {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	return mainH
}

func (m Model) renderLeftPane(width, height int) string {
	statusH, commitsH := m.leftPaneHeights(height, width)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderStatusPane(width, statusH),
		m.renderBranchCommitsPane(width, commitsH),
	)
}

func (m Model) leftPaneHeights(total, width int) (statusH, commitsH int) {
	commitsH = m.requiredBranchCommitsPaneHeight(width)
	if commitsH > total-2 {
		commitsH = maxInt(2, total/2)
	}
	statusH = total - commitsH
	if statusH < minStatusPaneHeight {
		commitsH -= minStatusPaneHeight - statusH
		if commitsH < 2 {
			commitsH = 2
		}
		statusH = total - commitsH
	}
	if statusH < 2 {
		statusH = 2
		commitsH = total - statusH
	}
	if commitsH < 2 {
		commitsH = 2
		statusH = total - commitsH
	}
	return statusH, commitsH
}

func (m Model) statusPaneTitle() string {
	title := "Status"
	if summary := m.branchSummaryTitleSuffix(); summary != "" {
		title += " (" + summary + ")"
	}
	return title
}

func (m Model) visibleStatusLines(height int) []string {
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
				name = statusFileIcon(entry.File, isWorktreeSymlink(m.worktreeRoot, entry.File.Path), icons) + " " + name
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
	return lines
}

func (m Model) requiredStatusPaneWidth(height int) int {
	required := ansi.StringWidth(" " + m.statusPaneTitle() + " ")
	for _, line := range m.visibleStatusLines(height) {
		if w := ansi.StringWidth(line); w > required {
			required = w
		}
	}
	return required + 2
}

func (m Model) renderStatusPane(width, height int) string {
	lines := m.visibleStatusLines(height)
	return m.renderPanelWithBorderTitle(width, height, m.statusPaneTitle(), "", lines, m.focus == focusStatus, sectionUnstaged)
}

func (m Model) branchSummaryTitleSuffix() string {
	if strings.TrimSpace(m.branchName) == "" {
		return ""
	}
	branchLabel := "branch"
	if m.settings.UseNerdFontIcons {
		branchLabel = ""
	}
	out := branchLabel + " " + m.branchName + " " + m.branchSyncToken()
	base := strings.TrimSpace(m.branchBaseRef)
	if shouldShowBranchBaseRef(base) {
		out += " · vs " + base
	}
	return out
}

func (m Model) branchSyncToken() string {
	switch m.branchSync.Name {
	case git.StatusSame:
		return "✓"
	case git.StatusAhead:
		return fmt.Sprintf("↑%d", m.branchSync.Ahead)
	case git.StatusBehind:
		return fmt.Sprintf("↓%d", m.branchSync.Behind)
	case git.StatusDiverged:
		return fmt.Sprintf("↑%d ↓%d", m.branchSync.Ahead, m.branchSync.Behind)
	}
	return "?"
}

func shouldShowBranchBaseRef(base string) bool {
	base = strings.TrimSpace(base)
	if base == "" {
		return false
	}
	return base != "origin/main" && base != "origin/master"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
