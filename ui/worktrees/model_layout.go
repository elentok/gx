package worktrees

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

// layout is the screen geometry of the table and details panels: the page's
// size and how many worktree rows the table must fit when stacked.
type layout struct {
	width, height, rows int
}

func layoutOf(m Model) layout {
	return layout{width: m.width, height: m.height, rows: len(m.worktrees)}
}

// seamWidth is the 1-cell gap reserved between the table and details panels;
// the panels themselves render edge-to-edge, so the layout - not either
// panel - owns this gap.
func (l layout) seamWidth() int {
	return 1
}

func (l layout) splitWidth() (tableW, previewW int) {
	if l.useStackedLayout() {
		return l.width, l.width
	}
	width := l.width - l.seamWidth()
	tableW = int(float64(width) * 0.55)
	previewW = width - tableW
	return
}

func (l layout) splitHeight(total int) (tableH, previewH int) {
	if !l.useStackedLayout() {
		return total, total
	}
	total -= l.seamWidth()
	// Size the table to exactly fit its rows: N rows + 2 (header+border) + 2 (box borders).
	const minPreviewH = 6
	tableH = l.rows + 4
	if tableH > total-minPreviewH {
		tableH = total - minPreviewH
	}
	if tableH < 4 {
		tableH = 4
	}
	previewH = total - tableH
	if previewH < 1 {
		previewH = 1
	}
	return
}

func (l layout) useStackedLayout() bool {
	return l.width <= 100
}

// tableRect and previewRect give the on-screen bounds of the two panes, for
// routing a mouse-wheel event to whichever the cursor is over.
func (l layout) tableRect() ui.Rect {
	tableW, _ := l.splitWidth()
	tableH, _ := l.splitHeight(l.contentHeight())
	return ui.Rect{X: 0, Y: 0, W: tableW, H: tableH}
}

func (l layout) previewRect() ui.Rect {
	tableW, previewW := l.splitWidth()
	tableH, previewH := l.splitHeight(l.contentHeight())
	if l.useStackedLayout() {
		return ui.Rect{X: 0, Y: tableH + l.seamWidth(), W: tableW, H: previewH}
	}
	return ui.Rect{X: tableW + l.seamWidth(), Y: 0, W: previewW, H: tableH}
}

func (l layout) contentHeight() int {
	if l.height < 4 {
		return 4
	}
	return l.height
}

func (m Model) resized() Model {
	l := layoutOf(m)
	tableW, previewW := l.splitWidth()
	h := l.contentHeight()
	tableH, previewH := l.splitHeight(h)

	tableInnerW := tableW - 2
	tableInnerH := tableH - 2
	if tableInnerW < 1 {
		tableInnerW = 1
	}
	if tableInnerH < 1 {
		tableInnerH = 1
	}
	resizeTable(&m.table, tableInnerW, tableInnerH)

	vpW := previewW - 2
	vpH := previewH - 2
	if vpW < 1 {
		vpW = 1
	}
	if vpH < 1 {
		vpH = 1
	}
	m.viewport.SetWidth(vpW)
	m.viewport.SetHeight(vpH)
	m.viewport.SetContent(m.previewContent())

	return m
}

func (m Model) previewContent() string {
	var wt *git.Worktree
	if cursor := m.table.Cursor(); cursor >= 0 && cursor < len(m.worktrees) {
		w := m.worktrees[cursor]
		wt = &w
	}
	base := previewBase{trunk: m.repo.Trunk, problem: m.trunkProblem(), ahead: m.previewTrunkAhead, behind: m.previewTrunkBehind}
	if wt != nil {
		base.rebased = m.baseStatus[wt.Branch]
		base.isTrunk = m.isTrunk(wt.Branch)
	}
	spinnerView := ""
	if m.previewLoading {
		spinnerView = m.spinner.View()
	}
	return renderPreviewContent(
		wt,
		m.previewUpstream,
		m.previewHeadCommit,
		m.previewAheadCommits,
		m.previewBehindCommits,
		base,
		m.previewChanges,
		spinnerView,
		m.settings.UseNerdFontIcons,
	)
}
