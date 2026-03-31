package worktrees

import (
	"fmt"

	"gx/git"
	"gx/ui"
	"gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

func (m Model) enterPushDivergedMode(wt git.Worktree, div *git.PushDivergence) Model {
	m.mode = modePushDiverged
	m.pushDivergence = div
	w := wt
	m.pushDivergenceWT = &w
	m.statusMsg = ""
	return m
}

func (m Model) handlePushDivergedKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.pushDivergence == nil || m.pushDivergenceWT == nil {
		m.mode = modeNormal
		return m, nil
	}
	wt := *m.pushDivergenceWT
	div := m.pushDivergence

	switch msg.String() {
	case "1":
		m.mode = modeNormal
		m.spinnerActive = true
		m.spinnerLabel = "Rebasing " + wt.Name + "..."
		return m, tea.Batch(cmdRebaseRef(wt, div.Upstream), m.spinner.Tick)
	case "2":
		m.mode = modeNormal
		m.spinnerActive = true
		m.spinnerLabel = "Force-pushing " + wt.Name + "..."
		return m, tea.Batch(cmdForcePush(m.repo, wt), m.spinner.Tick)
	case "3", "esc":
		m.mode = modeNormal
		m.statusGen++
		m.statusMsg = "Push aborted"
		return m, cmdClearStatus(m.statusGen)
	case "enter":
		// Default to Abort for safety.
		m.mode = modeNormal
		m.statusGen++
		m.statusMsg = "Push aborted"
		return m, cmdClearStatus(m.statusGen)
	default:
		return m, nil
	}
}

func (m Model) pushDivergedModalView() string {
	if m.pushDivergence == nil {
		return ""
	}
	d := m.pushDivergence
	prompt := fmt.Sprintf(
		"Branch %s has diverged from the remote branch:\n\n  Last local commit: %s %s\n  Last remote commit: %s %s\n\n1. Rebase\n2. Push --force\n3. Abort\n\nPress 1, 2, or 3",
		d.Branch,
		d.Local.Hash,
		d.Local.Message,
		d.RemoteHead.Hash,
		d.RemoteHead.Message,
	)
	return components.RenderOutputModal(
		"Push Diverged",
		prompt,
		"1 rebase · 2 force push · 3 abort",
		ui.ColorBorder,
		ui.ColorBorder,
		ui.ColorGray,
		0,
	)
}
