package status

import (
	"github.com/elentok/gx/ui/status/filetree"
)

const (
	minFiletreePaneWidth = 25
	maxFiletreePaneWidth = 45
	minDiffPaneWidth     = 60
)

func (m Model) splitWidth() (filetreeW, diffW int) {
	seamW := 1
	if useStackedLayout(m.width) {
		return m.width, m.width
	}

	width := m.width - seamW
	filetreeW = m.filetree.RequiredPaneWidth(mainContentHeight(m.height), filetreeViewOf(m))
	filetreeMax := minInt(maxFiletreePaneWidth, int(float64(width)*0.45))
	if filetreeMax < minFiletreePaneWidth {
		filetreeMax = minFiletreePaneWidth
	}
	if filetreeW < minFiletreePaneWidth {
		filetreeW = minFiletreePaneWidth
	}
	if filetreeW > filetreeMax {
		filetreeW = filetreeMax
	}
	if width-filetreeW < minDiffPaneWidth {
		filetreeW = width - minDiffPaneWidth
	}
	if filetreeW < minFiletreePaneWidth {
		filetreeW = minFiletreePaneWidth
	}
	diffW = width - filetreeW
	if diffW < minDiffPaneWidth {
		diffW = minDiffPaneWidth
		filetreeW = width - diffW
	}
	return filetreeW, diffW
}

func splitHeight(width, total int) (filetreeH, diffH int) {
	if !useStackedLayout(width) {
		return total, total
	}
	total--
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

func useStackedLayout(width int) bool {
	return width <= 100
}

func mainContentHeight(height int) int {
	mainH := height
	if mainH < 4 {
		mainH = 4
	}
	return mainH
}

func filetreeViewOf(m Model) filetree.View {
	return filetree.View{
		WorktreeRoot: m.worktreeRoot,
		IsBareRepo:   m.isBareRepo,
		NerdIcons:    m.settings.UseNerdFontIcons,
		Active:       m.focus == focusFiletree,
	}
}

func (m Model) visibleStatusLines(width, height int) []string {
	return m.filetree.RenderLines(width, height, filetreeViewOf(m))
}

// renderFiletreePane puts the branch summary in the title and the worktree
// name as a dim line at the top of the content, ahead of the file list.
func (m Model) renderFiletreePane(width, height int) string {
	v := filetreeViewOf(m)
	info := m.filetree.InfoLines(v)
	contentHeight := height - len(info)
	if contentHeight < filetree.MinPaneHeight {
		contentHeight = height
		info = nil
	}
	lines := append(info, m.visibleStatusLines(width, contentHeight)...)
	return m.renderFiletreePanelWithBorderTitle(width, height, m.filetree.Title(width-2, v), m.searchCounterForFiletreePane(), lines, m.focus == focusFiletree)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
