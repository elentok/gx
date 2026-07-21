package prs

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type prsLoadedMsg struct {
	prs       []git.PR
	anyPRs    bool
	err       error
	closedPRs []git.ClosedPR
}

func (m Model) cmdLoad() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	allRepos := m.allRepos
	return func() tea.Msg {
		// The closed-PR section is independent of the open-PR pipeline (no
		// facets, no actionable marker), so a closed-fetch failure is treated
		// as "no recently-closed PRs" rather than surfacing its own error UI.
		closedPRs, _ := git.ListClosedPRs(worktreeRoot, allRepos)

		prs, err := git.ListOpenPRs(worktreeRoot, allRepos)
		if err != nil {
			return prsLoadedMsg{err: err, closedPRs: closedPRs}
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
		return prsLoadedMsg{prs: prs, anyPRs: anyPRs, closedPRs: closedPRs}
	}
}

type gotoPRMsg struct {
	url string
}

func (m Model) cmdOpenSelected() tea.Cmd {
	sel := m.list.Selected()
	if sel < 0 || sel >= len(m.prs) {
		return nil
	}
	url := m.prs[sel].URL
	return func() tea.Msg { return gotoPRMsg{url: url} }
}

func (m Model) handleGotoPR(msg gotoPRMsg) (Model, tea.Cmd) {
	if msg.url == "" {
		return m, notify.Warning("no PR URL found")
	}
	return m, ui.CmdOpenURL(msg.url)
}
