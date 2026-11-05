package status

import (
	"math"

	"github.com/elentok/gx/ui"

	"github.com/charmbracelet/x/ansi"
)

const (
	minFiletreePaneWidth  = 25
	maxFiletreePaneWidth  = 45
	minDiffPaneWidth      = 60
	minFiletreePaneHeight = 5
)

func (m Model) splitWidth() (filetreeW, diffW int) {
	seamW := 1
	if m.useStackedLayout() {
		return m.width, m.width
	}

	width := m.width - seamW
	filetreeW = m.requiredFiletreePaneWidth(m.mainContentHeight())
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

func (m Model) splitHeight(total int) (filetreeH, diffH int) {
	if !m.useStackedLayout() {
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

func (m Model) useStackedLayout() bool {
	return m.width <= 100
}

func (m Model) mainContentHeight() int {
	mainH := m.height
	if mainH < 4 {
		mainH = 4
	}
	return mainH
}

func (m Model) renderLeftPane(width, height int) string {
	return m.renderFiletreePane(width, maxInt(minFiletreePaneHeight, height))
}

func (m Model) filetreePaneTitle(maxWidth int) string {
	prefix := "Changes in "
	if summary := m.filetree.branchSummaryTitleSuffix(maxWidth-ansi.StringWidth(prefix), m.settings.UseNerdFontIcons); summary != "" {
		return prefix + summary
	}
	return "Filetree"
}

func (m Model) filetreeView() filetreeView {
	return filetreeView{
		worktreeRoot: m.worktreeRoot,
		nerdIcons:    m.settings.UseNerdFontIcons,
		active:       m.focus == focusFiletree,
	}
}

func (m Model) visibleStatusLines(width, height int) []string {
	return m.filetree.renderLines(width, height, m.filetreeView())
}

func (m Model) requiredFiletreePaneWidth(height int) int {
	required := ansi.StringWidth(" " + m.filetreePaneTitle(math.MaxInt32) + " ")
	if w := m.filetree.requiredWidth(height, m.filetreeView()); w > required {
		required = w
	}
	return required + 2
}

// renderFiletreePane puts the branch summary in the title and the worktree
// name as a dim line at the top of the content, ahead of the file list.
func (m Model) renderFiletreePane(width, height int) string {
	info := m.filetreeInfoLines()
	contentHeight := height - len(info)
	if contentHeight < minFiletreePaneHeight {
		contentHeight = height
		info = nil
	}
	lines := append(info, m.visibleStatusLines(width, contentHeight)...)
	return m.renderFiletreePanelWithBorderTitle(width, height, m.filetreePaneTitle(width-2), m.searchCounterForFiletreePane(), lines, m.focus == focusFiletree)
}

// filetreeInnerHeight returns the number of file rows actually rendered
// inside the filetree pane at the given outer height. It mirrors
// renderFiletreePane's reservation of filetreeInfoLines() (e.g. the
// "worktree: X" line shown for bare-repo checkouts), so callers that clamp
// scroll/selection via fileTreeModel.SetVisibleHeight never diverge from what
// is actually rendered.
func (m Model) filetreeInnerHeight(height int) int {
	info := m.filetreeInfoLines()
	contentHeight := height - len(info)
	if contentHeight < minFiletreePaneHeight {
		contentHeight = height
	}
	return maxInt(1, contentHeight-2)
}

// filetreeInfoLines shows which worktree is active, but only when the repo
// actually uses multiple worktrees (the .bare technique) — for a plain
// single-checkout clone the worktree name adds no information.
func (m Model) filetreeInfoLines() []string {
	if !m.isBareRepo {
		return nil
	}
	name := ui.WorktreeName(m.worktreeRoot)
	if name == "" {
		return nil
	}
	icon := ""
	if m.settings.UseNerdFontIcons {
		icon = ui.Icons(true).Worktree + " "
	}
	return []string{ui.StyleMuted.Faint(true).Render(icon + "worktree: " + name)}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
