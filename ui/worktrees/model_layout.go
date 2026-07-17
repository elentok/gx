package worktrees

import (
	"github.com/elentok/gx/git"
)

// seamWidth is the 1-cell gap reserved between the table and details panels;
// the panels themselves render edge-to-edge, so the layout - not either
// panel - owns this gap.
func (m Model) seamWidth() int {
	return 1
}

func (m Model) splitWidth() (tableW, previewW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}
	width := m.width - m.seamWidth()
	tableW = int(float64(width) * 0.55)
	previewW = width - tableW
	return
}

func (m Model) splitHeight(total int) (tableH, previewH int) {
	if !m.useStackedLayout() {
		return total, total
	}
	total -= m.seamWidth()
	// Size the table to exactly fit its rows: N rows + 2 (header+border) + 2 (box borders).
	const minPreviewH = 6
	tableH = len(m.worktrees) + 4
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

func (m Model) useStackedLayout() bool {
	return m.width <= 100
}

func (m Model) helpLineCount() int {
	return 1
}

func (m Model) contentHeight() int {
	h := m.height - m.helpLineCount()
	if h < 4 {
		return 4
	}
	return h
}

func (m Model) resized() Model {
	tableW, previewW := m.splitWidth()
	h := m.contentHeight()
	tableH, previewH := m.splitHeight(h)

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
	var rebasedOnMain *bool
	var isMainBranch bool
	if wt != nil {
		rebasedOnMain = m.baseStatus[wt.Branch]
		isMainBranch = wt.Branch == m.repo.MainBranch
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
		rebasedOnMain,
		isMainBranch,
		m.previewChanges,
		spinnerView,
		m.settings.UseNerdFontIcons,
	)
}
