package commit

import (
	"github.com/elentok/gx/ui/explorer"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.syncDiffViewport()
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		shiftG := (msg.Mod&tea.ModShift) != 0 && (msg.Code == 'g' || msg.Code == 'G' || msg.Text == "g" || msg.Text == "G")
		isUpperG := key == "G" || key == "shift+g" || msg.Text == "G" || msg.ShiftedCode == 'G' || shiftG
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.helpOpen {
			return m.handleHelpKey(msg)
		}
		if handled, cmd := m.handleSearchKey(msg); handled {
			return m, cmd
		}
		if next, cmd, handled := m.handleChordKey(msg); handled {
			return next, cmd
		}
		if m.handleSearchNavigateKey(msg) {
			return m, nil
		}
		if msg.Code == tea.KeyTab || msg.Text == "\t" {
			if m.focusHeader {
				m.focusHeader = false
				m.focusDiff = false
				return m, nil
			}
			if _, ok := m.selectedCommitFile(); !ok {
				m.focusHeader = true
				m.focusDiff = false
				return m, nil
			}
			if m.focusDiff {
				m.focusDiff = false
				m.focusHeader = true
			} else {
				m.focusDiff = true
				m.focusHeader = false
				m.ensureActiveVisible()
			}
			return m, nil
		}
		switch key {
		case "?":
			return m.enterHelpMode(), nil
		case "q", "esc":
			if len(m.searchMatches) > 0 {
				m.clearSearch()
				return m, nil
			}
			if m.focusDiff {
				m.focusDiff = false
				return m, nil
			}
			if m.focusHeader {
				m.focusHeader = false
				return m, nil
			}
			return m, nav.Back()
		case "/":
			m.enterSearchMode()
			return m, nil
		case "b":
			m.bodyExpanded = !m.bodyExpanded
			m.scrollHeader(0)
			m.syncDiffViewport()
			return m, nil
		case "a":
			if !m.focusDiff {
				return m, nil
			}
			if m.diffNavMode == explorer.NavHunk {
				m.diffNavMode = explorer.NavLine
			} else {
				m.diffNavMode = explorer.NavHunk
			}
			m.ensureActiveVisible()
			return m, nil
		case "w":
			if !m.focusDiff {
				return m, nil
			}
			m.wrapSoft = !m.wrapSoft
			m.syncDiffViewport()
			return m, nil
		case "j", "down":
			if m.focusHeader {
				m.scrollHeader(1)
				return m, nil
			}
			if m.focusDiff {
				m.moveDiffActive(1)
				return m, nil
			}
			m.moveSidebar(1)
			return m, nil
		case "k", "up":
			if m.focusHeader {
				m.scrollHeader(-1)
				return m, nil
			}
			if m.focusDiff {
				m.moveDiffActive(-1)
				return m, nil
			}
			m.moveSidebar(-1)
			return m, nil
		case "J":
			if m.focusHeader {
				m.scrollHeader(1)
			} else if m.focusDiff {
				m.diffViewport.ScrollDown(3)
			}
			return m, nil
		case "K":
			if m.focusHeader {
				m.scrollHeader(-1)
			} else if m.focusDiff {
				m.diffViewport.ScrollUp(3)
			}
			return m, nil
		case "ctrl+d":
			if m.focusHeader {
				m.scrollHeaderPage(1)
			} else if m.focusDiff {
				m.scrollDiffPage(1)
			}
			return m, nil
		case "ctrl+u":
			if m.focusHeader {
				m.scrollHeaderPage(-1)
			} else if m.focusDiff {
				m.scrollDiffPage(-1)
			}
			return m, nil
		case ".":
			if m.focusDiff {
				m.moveToAdjacentFile(1)
			} else {
				m.moveToAdjacentCommit(-1)
			}
			return m, nil
		case ",":
			if m.focusDiff {
				m.moveToAdjacentFile(-1)
			} else {
				m.moveToAdjacentCommit(1)
			}
			return m, nil
		case "G":
			if isUpperG {
				if m.focusDiff {
					m.jumpDiffBottom()
				} else {
					m.jumpSidebarBottom()
				}
			}
			return m, nil
		case "enter":
			if m.focusHeader {
				m.focusHeader = false
				return m, nil
			}
			if m.toggleDirOnEnter() {
				return m, nil
			}
			if _, ok := m.selectedCommitFile(); ok {
				m.focusDiff = true
				m.ensureActiveVisible()
			}
			return m, nil
		case "l", "right":
			if m.focusHeader {
				m.focusHeader = false
				return m, nil
			}
			if !m.focusDiff && m.expandSelectedDir() {
				return m, nil
			}
			if _, ok := m.selectedCommitFile(); ok {
				m.focusDiff = true
				m.ensureActiveVisible()
			}
			return m, nil
		case "h", "left":
			if m.focusDiff {
				m.focusDiff = false
				return m, nil
			}
			if m.focusHeader {
				m.focusHeader = false
				return m, nil
			}
			if m.focusParentInSidebar() {
				m.refreshDiff()
				return m, nil
			}
			m.collapseSelectedDir()
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	shiftG := (msg.Mod&tea.ModShift) != 0 && (msg.Code == 'g' || msg.Code == 'G' || msg.Text == "g" || msg.Text == "G")
	isUpperG := key == "G" || key == "shift+g" || msg.Text == "G" || msg.ShiftedCode == 'G' || shiftG
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
			if m.focusHeader {
				m.yankCommitBody()
			} else {
				m.yankContentOnly()
			}
			return m, nil, true
		case "esc":
			m.clearStatus()
			return m, nil, true
		}
		return m, nil, true
	}
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		switch key {
		case "g":
			if m.focusDiff {
				m.jumpDiffTop()
			} else {
				m.jumpSidebarTop()
			}
			return m, nil, true
		case "w":
			return m, nav.Replace(nav.Route{Kind: nav.RouteWorktrees}), true
		case "l":
			return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot, Ref: m.ref}), true
		case "s":
			return m, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot}), true
		case "esc":
			return m, nil, true
		}
		return m, nil, true
	}
	if key == "y" {
		m.keyPrefix = "y"
		return m, nil, true
	}
	if key == "g" && !isUpperG {
		m.keyPrefix = "g"
		return m, nil, true
	}
	if isUpperG {
		if m.focusDiff {
			m.jumpDiffBottom()
		} else {
			m.jumpSidebarBottom()
		}
		return m, nil, true
	}
	return m, nil, false
}
