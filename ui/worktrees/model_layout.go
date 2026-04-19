package worktrees

import "github.com/elentok/gx/git"

func (m Model) splitWidth() (tableW, sidebarW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}
	tableW = int(float64(m.width) * 0.55)
	sidebarW = m.width - tableW
	return
}

func (m Model) splitHeight(total int) (tableH, sidebarH int) {
	if !m.useStackedLayout() {
		return total, total
	}
	// Size the table to exactly fit its rows: N rows + 2 (header+border) + 2 (box borders).
	const minSidebarH = 6
	tableH = len(m.worktrees) + 4
	if tableH > total-minSidebarH {
		tableH = total - minSidebarH
	}
	if tableH < 4 {
		tableH = 4
	}
	sidebarH = total - tableH
	if sidebarH < 1 {
		sidebarH = 1
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
	m.help.SetWidth(m.width)
	tableW, sidebarW := m.splitWidth()
	h := m.contentHeight()
	tableH, sidebarH := m.splitHeight(h)

	tableInnerW := tableW - 2
	tableInnerH := tableH - 2
	if tableInnerW < 1 {
		tableInnerW = 1
	}
	if tableInnerH < 1 {
		tableInnerH = 1
	}
	resizeTable(&m.table, tableInnerW, tableInnerH)

	vpW := sidebarW - 2
	vpH := sidebarH - 2
	if vpW < 1 {
		vpW = 1
	}
	if vpH < 1 {
		vpH = 1
	}
	m.viewport.SetWidth(vpW)
	m.viewport.SetHeight(vpH)
	m.viewport.SetContent(m.sidebarContent())

	return m
}

func (m Model) sidebarContent() string {
	var wt *git.Worktree
	if len(m.worktrees) > 0 {
		w := m.worktrees[m.table.Cursor()]
		wt = &w
	}
	var rebasedOnMain *bool
	var isMainBranch bool
	if wt != nil {
		rebasedOnMain = m.baseStatus[wt.Branch]
		isMainBranch = wt.Branch == m.repo.MainBranch
	}
	spinnerView := ""
	if m.sidebarLoading {
		spinnerView = m.spinner.View()
	}
	return renderSidebarContent(
		wt,
		m.sidebarUpstream,
		m.sidebarHeadCommit,
		m.sidebarAheadCommits,
		m.sidebarBehindCommits,
		rebasedOnMain,
		isMainBranch,
		m.sidebarChanges,
		spinnerView,
		m.settings.UseNerdFontIcons,
	)
}
