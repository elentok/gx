package status

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/sidebar"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	minFiletreePaneWidth  = 25
	maxFiletreePaneWidth  = 45
	minDiffPaneWidth      = 60
	minFiletreePaneHeight = 5
)

func (m Model) splitWidth() (filetreeW, diffW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}

	filetreeW = m.requiredFiletreePaneWidth(m.mainContentHeight())
	filetreeMax := minInt(maxFiletreePaneWidth, int(float64(m.width)*0.45))
	if filetreeMax < minFiletreePaneWidth {
		filetreeMax = minFiletreePaneWidth
	}
	if filetreeW < minFiletreePaneWidth {
		filetreeW = minFiletreePaneWidth
	}
	if filetreeW > filetreeMax {
		filetreeW = filetreeMax
	}
	if m.width-filetreeW < minDiffPaneWidth {
		filetreeW = m.width - minDiffPaneWidth
	}
	if filetreeW < minFiletreePaneWidth {
		filetreeW = minFiletreePaneWidth
	}
	diffW = m.width - filetreeW
	if diffW < minDiffPaneWidth {
		diffW = minDiffPaneWidth
		filetreeW = m.width - diffW
	}
	return filetreeW, diffW
}

func (m Model) splitHeight(total int) (filetreeH, diffH int) {
	if !m.useStackedLayout() {
		return total, total
	}
	filetreeH = int(float64(total) * 0.30)
	if filetreeH < 5 {
		filetreeH = 5
	}
	diffH = total - filetreeH
	if diffH < 5 {
		diffH = 5
		filetreeH = total - diffH
	}
	return filetreeH, diffH
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
	return m.renderFiletreePane(width, maxInt(minFiletreePaneHeight, height))
}

func (m Model) filetreePaneTitle() string {
	title := "Filetree"
	if summary := m.branchSummaryTitleSuffix(); summary != "" {
		title += " (" + summary + ")"
	}
	return title
}

func (m Model) visibleStatusLines(height int) []string {
	innerH := maxInt(1, height-2)
	icons := filetreePaneIconsFor(m.settings.UseNerdFontIcons)
	start, _ := sidebar.VisibleWindow(len(m.page.statusEntries), m.page.selected, innerH)
	rows := sidebar.BuildVisibleRenderableRows(m.page.statusEntries, m.page.selected, innerH, func(i int, entry statusEntry) sidebar.RenderableRow {
		statusColor := statusEntryColor(entry)
		deleted := entry.Kind == statusEntryFile && isDeletedFileStatus(entry.File)
		metaRaw := statusEntryMeta(entry, m.settings.UseNerdFontIcons, icons)
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
			name = search.Highlight(name, m.fileTreeModel.Search().Query(), false)
		}
		return sidebar.RenderableRow{
			Depth:    entry.Depth,
			MetaRaw:  metaRaw,
			NameRaw:  name,
			Color:    statusColor,
			Selected: i == m.page.selected,
			Faint:    deleted,
		}
	})
	lines := sidebar.RenderRows(rows, innerH, lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render("clean working tree"), ui.ColorBlue)
	if m.focus == focusFiletree {
		selectedIdx := m.page.selected - start
		if selectedIdx >= 0 && selectedIdx < len(lines) && lines[selectedIdx] != "" {
			lines[selectedIdx] = ui.RenderRowHighlight(lines[selectedIdx])
		}
	}
	return lines
}

func (m Model) requiredFiletreePaneWidth(height int) int {
	required := ansi.StringWidth(" " + m.filetreePaneTitle() + " ")
	for _, line := range m.visibleStatusLines(height) {
		if w := ansi.StringWidth(line); w > required {
			required = w
		}
	}
	return required + 2
}

func (m Model) renderFiletreePane(width, height int) string {
	lines := m.visibleStatusLines(height)
	return m.renderFiletreePanelWithBorderTitle(width, height, m.filetreePaneTitle(), m.searchCounterForFiletreePane(), lines, m.focus == focusFiletree)
}

func (m Model) branchSummaryTitleSuffix() string {
	if strings.TrimSpace(m.page.branchName) == "" {
		return ""
	}
	branchLabel := "branch"
	if m.settings.UseNerdFontIcons {
		branchLabel = ""
	}
	out := branchLabel + " " + m.page.branchName + " " + m.branchSyncToken()
	base := strings.TrimSpace(m.page.branchBaseRef)
	if shouldShowBranchBaseRef(base) {
		out += " · vs " + base
	}
	return out
}

func (m Model) branchSyncToken() string {
	switch m.page.branchSync.Name {
	case git.StatusSame:
		return "✓"
	case git.StatusAhead:
		return fmt.Sprintf("↑%d", m.page.branchSync.Ahead)
	case git.StatusBehind:
		return fmt.Sprintf("↓%d", m.page.branchSync.Behind)
	case git.StatusDiverged:
		return fmt.Sprintf("↑%d ↓%d", m.page.branchSync.Ahead, m.page.branchSync.Behind)
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
