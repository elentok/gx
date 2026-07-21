package prs

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type openPRsLoadedMsg struct {
	prs    []git.PR
	anyPRs bool
	err    error
}

type closedPRsLoadedMsg struct {
	closedPRs []git.ClosedPR
}

// cmdLoad kicks off the open-PR and closed-PR fetches concurrently, each
// completing (and rendering) independently of the other — see
// issues/09-load-time-batched-fetch.md.
func (m Model) cmdLoad() tea.Cmd {
	return tea.Batch(m.cmdLoadOpen(), m.cmdLoadClosed())
}

func (m Model) cmdLoadOpen() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	allRepos := m.allRepos
	return func() tea.Msg {
		prs, err := git.ListOpenPRs(worktreeRoot, allRepos)
		if err != nil {
			return openPRsLoadedMsg{err: err}
		}
		anyPRs := len(prs) > 0
		if !anyPRs {
			// If the probe itself fails (transient network blip, rate limit),
			// default to the less alarming "no open PRs" rather than falsely
			// claiming the user has no PRs at all.
			var probeErr error
			anyPRs, probeErr = git.AnyPRsExist(worktreeRoot, allRepos)
			if probeErr != nil {
				anyPRs = true
			}
		}
		return openPRsLoadedMsg{prs: prs, anyPRs: anyPRs}
	}
}

func (m Model) cmdLoadClosed() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	allRepos := m.allRepos
	return func() tea.Msg {
		// The closed-PR section is independent of the open-PR pipeline (no
		// facets, no actionable marker), so a closed-fetch failure is treated
		// as "no recently-closed PRs" rather than surfacing its own error UI.
		closedPRs, _ := git.ListClosedPRs(worktreeRoot, allRepos)
		return closedPRsLoadedMsg{closedPRs: closedPRs}
	}
}

type gotoPRMsg struct {
	url string
}

// navigateSelection moves the selection by delta across the combined
// open+closed list. Scroll-viewport math only ever applies to the open
// section (closed rows always render in full below it), so
// EnsureSelectionVisible is skipped once the selection lands on a closed
// row — see issues/10-closed-pr-selectable.md.
func (m Model) navigateSelection(delta int) Model {
	m.list.SetSelected(m.list.Selected()+delta, m.totalItems())
	if m.list.Selected() < len(m.prs) {
		m.list.EnsureSelectionVisible(len(m.prs), m.visibleH())
	}
	return m
}

func (m Model) cmdOpenSelected() tea.Cmd {
	sel := m.list.Selected()
	if sel < 0 || sel >= m.totalItems() {
		return nil
	}
	var url string
	if sel < len(m.prs) {
		url = m.prs[sel].URL
	} else {
		url = m.closedPRs[sel-len(m.prs)].URL
	}
	return func() tea.Msg { return gotoPRMsg{url: url} }
}

func (m Model) handleGotoPR(msg gotoPRMsg) (Model, tea.Cmd) {
	if msg.url == "" {
		return m, notify.Warning("no PR URL found")
	}
	return m, ui.CmdOpenURL(msg.url)
}

type commentsLoadedMsg struct {
	comments []git.PRComment
	err      error
}

// cmdOpenComments opens the comments popup for the selected row's on-demand
// comment fetch. A no-op when the selection is on a closed row (or invalid)
// — see issues/13-comments-popup.md.
func (m *Model) cmdOpenComments() tea.Cmd {
	sel := m.list.Selected()
	if sel < 0 || sel >= len(m.prs) {
		return nil
	}
	pr := m.prs[sel]
	m.comments.open(m.width)

	worktreeRoot := m.worktreeRoot
	repo := pr.Repo
	number := pr.Number
	return func() tea.Msg {
		comments, err := git.FetchPRComments(worktreeRoot, repo, number)
		return commentsLoadedMsg{comments: comments, err: err}
	}
}
