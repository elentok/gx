package worktrees

import (
	"fmt"
	"time"

	"gx/git"
	"gx/ui"
	"gx/ui/components"

	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
)

func (m Model) enterPushDivergedMode(wt git.Worktree, div *git.PushDivergence) Model {
	m.mode = modePushDiverged
	m.pushDivergence = div
	w := wt
	m.pushDivergenceWT = &w
	m.pushMenu = components.MenuState{
		Items: []components.MenuItem{
			{Label: "Rebase", Value: "rebase"},
			{Label: "Push --force", Value: "force"},
			{Label: "Abort", Value: "abort"},
		},
		Cursor: 0,
	}
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

	next, decided, accepted, handled := components.UpdateMenu(msg, m.pushMenu)
	if !handled {
		return m, nil
	}
	m.pushMenu = next
	if !decided {
		return m, nil
	}
	if !accepted {
		m.mode = modeNormal
		m.statusGen++
		m.statusMsg = "Push aborted"
		return m, cmdClearStatus(m.statusGen)
	}
	choice := m.pushMenu.Items[m.pushMenu.Cursor].Value
	switch choice {
	case "rebase":
		m.mode = modeNormal
		m.spinnerActive = true
		m.spinnerLabel = "Rebasing " + wt.Name + "..."
		return m, tea.Batch(cmdRebaseRef(wt, div.Upstream, m.lastJobLog), m.spinner.Tick)
	case "force":
		m.mode = modeNormal
		return m, cmdStartPromptableJob(promptableJobForcePush, wt, m.lastJobLog, false)
	default:
		m.mode = modeNormal
		m.statusGen++
		m.statusMsg = "Push aborted"
		return m, cmdClearStatus(m.statusGen)
	}
}

func (m Model) pushDivergedModalView() string {
	if m.pushDivergence == nil {
		return ""
	}
	d := m.pushDivergence
	prompt := fmt.Sprintf(
		"Branch %s has diverged from the remote branch:\n\nLast local commit: %s\n  %s %s\n\nLast remote commit: %s\n  %s %s",
		d.Branch,
		humanizeOrUnknownTime(d.Local.Date),
		d.Local.Hash,
		d.Local.Message,
		humanizeOrUnknownTime(d.RemoteHead.Date),
		d.RemoteHead.Hash,
		d.RemoteHead.Message,
	)
	return components.RenderMenuModal(
		"Push Diverged",
		prompt,
		m.pushMenu,
		"",
		ui.ColorBorder,
		ui.ColorBorder,
		ui.ColorGray,
		ui.ColorGreen,
		0,
	)
}

func humanizeOrUnknownTime(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return humanize.Time(t)
}
