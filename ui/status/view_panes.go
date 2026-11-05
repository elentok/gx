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
	if useStackedLayout(m.width) {
		return m.width, m.width
	}

	width := m.width - seamW
	filetreeW = m.filetree.requiredPaneWidth(mainContentHeight(m.height), filetreeViewOf(m))
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

func (p filetreePane) title(maxWidth int, v filetreeView) string {
	prefix := "Changes in "
	if summary := p.branchSummaryTitleSuffix(maxWidth-ansi.StringWidth(prefix), v.nerdIcons); summary != "" {
		return prefix + summary
	}
	return "Filetree"
}

func filetreeViewOf(m Model) filetreeView {
	return filetreeView{
		worktreeRoot: m.worktreeRoot,
		isBareRepo:   m.isBareRepo,
		nerdIcons:    m.settings.UseNerdFontIcons,
		active:       m.focus == focusFiletree,
	}
}

func (m Model) visibleStatusLines(width, height int) []string {
	return m.filetree.renderLines(width, height, filetreeViewOf(m))
}

func (p filetreePane) requiredPaneWidth(height int, v filetreeView) int {
	required := ansi.StringWidth(" " + p.title(math.MaxInt32, v) + " ")
	if w := p.requiredWidth(height, v); w > required {
		required = w
	}
	return required + 2
}

// renderFiletreePane puts the branch summary in the title and the worktree
// name as a dim line at the top of the content, ahead of the file list.
func (m Model) renderFiletreePane(width, height int) string {
	v := filetreeViewOf(m)
	info := m.filetree.infoLines(v)
	contentHeight := height - len(info)
	if contentHeight < minFiletreePaneHeight {
		contentHeight = height
		info = nil
	}
	lines := append(info, m.visibleStatusLines(width, contentHeight)...)
	return m.renderFiletreePanelWithBorderTitle(width, height, m.filetree.title(width-2, v), m.searchCounterForFiletreePane(), lines, m.focus == focusFiletree)
}

// innerHeight returns the number of file rows actually rendered
// inside the filetree pane at the given outer height. It mirrors
// renderFiletreePane's reservation of infoLines() (e.g. the
// "worktree: X" line shown for bare-repo checkouts), so callers that clamp
// scroll/selection via fileTreeModel.SetVisibleHeight never diverge from what
// is actually rendered.
func (p filetreePane) innerHeight(height int, v filetreeView) int {
	info := p.infoLines(v)
	contentHeight := height - len(info)
	if contentHeight < minFiletreePaneHeight {
		contentHeight = height
	}
	return maxInt(1, contentHeight-2)
}

// infoLines shows which worktree is active, but only when the repo
// actually uses multiple worktrees (the .bare technique) — for a plain
// single-checkout clone the worktree name adds no information.
func (p filetreePane) infoLines(v filetreeView) []string {
	if !v.isBareRepo {
		return nil
	}
	name := ui.WorktreeName(v.worktreeRoot)
	if name == "" {
		return nil
	}
	icon := ""
	if v.nerdIcons {
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
