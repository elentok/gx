package status

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
)

import tea "charm.land/bubbletea/v2"

func (m Model) handleChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	shiftG := (msg.Mod&tea.ModShift) != 0 && (msg.Code == 'g' || msg.Code == 'G' || msg.Text == "g" || msg.Text == "G")
	isUpperG := key == "G" || key == "shift+g" || msg.Text == "G" || msg.ShiftedCode == 'G' || shiftG
	if m.keyPrefix == "c" {
		m.keyPrefix = ""
		if key == "c" {
			m.setStatus(ui.MessageOpening("git commit"))
			return m, cmdGitCommit(m.worktreeRoot, m.settings.Terminal), true
		}
		if key == "esc" {
			m.clearStatus()
			return m, nil, true
		}
	}
	if m.keyPrefix == "y" {
		m.keyPrefix = ""
		switch key {
		case "l":
			m.yankLocationOnly()
			return m, nil, true
		case "a":
			m.yankAllContext()
			return m, nil, true
		case "f":
			m.yankFilename()
			return m, nil, true
		case "y":
			m.yankContentOnly()
			return m, nil, true
		case "esc":
			m.clearStatus()
			return m, nil, true
		}
	}
	if key == "c" {
		m.keyPrefix = "c"
		m.setStatus(m.inlineHints(stageKeyCommit))
		return m, nil, true
	}
	if key == "y" {
		m.keyPrefix = "y"
		m.setStatus(m.inlineHints(stageKeyYankText, stageKeyYankPath, stageKeyYankAll, stageKeyYankName))
		return m, nil, true
	}
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		switch key {
		case "g":
			m.jumpToTop()
			m.clearStatus()
			if m.focus == focusStatus {
				return m, m.scheduleDiffReload(), true
			}
			return m, nil, true
		case "o":
			if m.outputContent == "" {
				m.setStatus(ui.MessageNoOutput())
				return m, nil, true
			}
			m.openOutputModal()
			return m, nil, true
		case "l":
			if m.settings.EnableNavigation {
				m.clearStatus()
				return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot}), true
			}
			return m, nil, true
		case "s":
			if m.settings.EnableNavigation {
				m.clearStatus()
				return m, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot}), true
			}
			return m, nil, true
		case "w":
			if m.settings.EnableNavigation {
				m.clearStatus()
				return m, nav.Replace(nav.Route{Kind: nav.RouteWorktrees}), true
			}
			return m, nil, true
		case "esc":
			m.clearStatus()
			return m, nil, true
		default:
			m.clearStatus()
			return m, nil, true
		}
	}
	if key == "g" && !isUpperG {
		m.keyPrefix = "g"
		m.setStatus(m.inlineHints(stageKeyTop, stageKeyOutput, stageKeyGoWorktree, stageKeyGoLog, stageKeyGoStatus))
		return m, nil, true
	}
	if key == "L" {
		m.setStatus(ui.MessageOpening("lazygit log"))
		return m, cmdLazygitLog(m.worktreeRoot), true
	}
	if isUpperG {
		m.keyPrefix = ""
		m.jumpToBottom()
		if m.focus == focusStatus {
			return m, m.scheduleDiffReload(), true
		}
		return m, nil, true
	}
	m.keyPrefix = ""
	return m, nil, false
}
