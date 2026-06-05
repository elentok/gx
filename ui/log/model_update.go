package log

import (
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/reword"

	tea "charm.land/bubbletea/v2"
)

type flashClearMsg struct{}

func cmdFlashClear() tea.Cmd {
	return tea.Tick(logFlashDuration, func(time.Time) tea.Msg {
		return flashClearMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	var childCmd tea.Cmd

	// ctrl+c quits unconditionally even when a modal is open.
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "ctrl+c" {
		return m, tea.Quit
	}
	// Delegate to rebase confirm modal while it's open.
	if m.rebaseConfirm.isOpen() {
		return m.handleRebaseConfirmUpdate(msg)
	}
	// Delegate all messages to amend.Model while it's open.
	if m.amendConfirm.IsOpen {
		return m.handleAmendUpdate(msg)
	}
	// Delegate all messages to bump.Model while it's open.
	if m.bump.IsOpen {
		return m.handleBumpUpdate(msg)
	}
	// Delegate all messages to push.Model while it's open.
	if m.push.IsOpen {
		return m.handlePushUpdate(msg)
	}
	// Delegate all messages to pull.Model while it's open.
	if m.pull.IsOpen {
		return m.handlePullUpdate(msg)
	}
	// Delegate all messages to reword.Model while it's running.
	if m.reword.IsOpen {
		return m.handleRewordRunningUpdate(msg)
	}

	// Delegate output modal keys.
	if m.output.IsOpen {
		next, childCmd := m.output.Update(msg)
		m.output = next
		return m, childCmd
	}

	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case gotoPRMsg:
		return m.handleGotoPR(msg)
	case reloadMsg:
		return m.handleReload(msg)
	case worktreeStatusMsg:
		return m.handleWorktreeStatus(msg)
	case rewordDetailsMsg:
		return m.handleRewordDetails(msg)
	case reword.EditorFinishedMsg:
		return m.handleRewordEditorDone(msg.Err)
	case rebaseFinishedMsg:
		return m.handleRebaseFinished(msg)
	case rebaseStashMsg:
		return m.handleRebaseStash(msg)
	case rebaseStashPopMsg:
		return m.handleRebaseStashPop(msg)
	case flashClearMsg:
		m.flashSubject = ""
		m.flashUntil = time.Time{}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help, childCmd = m.help.Update(msg)
		var splitCmd tea.Cmd
		m, splitCmd = m.syncSplitSize()
		return m, tea.Batch(childCmd, splitCmd)
	case tea.FocusMsg:
		if m.rebaseDidStash {
			m.rebaseDidStash = false
			m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: true}
		}
		return m, tea.Batch(m.cmdReload(), m.cmdLoadStatus())
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// If focus events aren't working (e.g. tmux without focus-events on),
		// catch the pending stash pop on the first key press instead.
		if m.rebaseDidStash {
			m.rebaseDidStash = false
			m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: true}
			return m, nil
		}
		if m.help.IsOpen {
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}

		// Route split-specific keys before the log key manager so they take
		// precedence when the split is active. Only when the log's own key
		// manager has no pending chord prefix (so ]t, [t etc. are not stolen).
		key := msg.String()
		logHasPrefix := len(m.keys.Prefix()) > 0
		if m.split.HasChord() && !logHasPrefix {
			// Second key of the "to" chord belongs to the split.
			return m.routeKeyToSplit(msg)
		}
		if key == "h" && m.split.IsSplit() && m.split.IsDetailFocused() && (m.commitDetail.IsFileTreeFocused() || m.commitDetail.IsHeaderFocused()) {
			return m.routeKeyToSplit(tea.KeyPressMsg{Code: tea.KeyEsc})
		}
		if (key == "esc" || key == "q") && !m.split.IsCollapsed() {
			if m.split.IsDetailFocused() && m.commitDetail.HasInternalFocus() {
				updated, detailCmd := m.commitDetail.Update(msg)
				m.commitDetail = updated.(commit.Model)
				return m, detailCmd
			}
			return m.routeKeyToSplit(msg)
		}
		if key == "f" && !m.split.IsCollapsed() {
			// Fullscreen toggle has priority over detail routing when split is active.
			return m.routeKeyToSplit(msg)
		}
		if key == "t" && !logHasPrefix {
			// Start of "to" orientation-toggle chord; pass to split container.
			return m.routeKeyToSplit(msg)
		}

		// When the detail panel is focused, route remaining keys there.
		if m.split.IsDetailFocused() {
			updated, detailCmd := m.commitDetail.Update(msg)
			m.commitDetail = updated.(commit.Model)
			return m, detailCmd
		}

		if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
			m.search = nextSearch
			if result.QueryChanged {
				m.recomputeSearchMatches()
			}
			if result.QueryChanged || result.CursorChanged {
				m.jumpToCurrentMatch()
			}
			return m, cmd
		}
		match, consumed := m.keys.Process(msg)
		if match != nil {
			return m.dispatchBinding(match.ID)
		}
		if consumed {
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleWorktreeStatus(msg worktreeStatusMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.statusLoaded = true
	m.statusStaged = msg.staged
	m.statusUnstaged = msg.unstaged
	m.statusUntracked = msg.untracked
	// Update the pseudo-log-line row in-place if rows are already loaded.
	rows := m.listPanel.Rows()
	if len(rows) > 0 && rows[0].kind == rowPseudoStatus {
		rows[0].detail = m.pseudoStatusDetail()
		m.listPanel = m.listPanel.WithRows(rows)
	}
	return m, nil
}

func (m Model) handleReload(msg reloadMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	var refreshCmds []tea.Cmd
	if m.refreshing {
		m.refreshing = false
		refreshCmds = []tea.Cmd{notify.Close("refresh"), notify.Success("refreshed")}
	}
	isInitialLoad := m.listPanel.Rows() == nil
	m.err = nil
	rows := msg.rows
	// Update pseudo-log-line with current status in case it was loaded after cmdReload started.
	if len(rows) > 0 && rows[0].kind == rowPseudoStatus {
		rows[0].detail = m.pseudoStatusDetail()
	}
	m.listPanel = m.listPanel.WithRows(rows)
	m.branchDiverged = msg.branchDiverged
	if m.pendingFocusRef != "" {
		ref := m.pendingFocusRef
		m.pendingFocusRef = ""
		for i, r := range rows {
			if r.kind == rowCommit && r.commit.FullHash == ref {
				m.listPanel = m.listPanel.SetSelected(i)
				break
			}
		}
	}
	m.pendingFocusSubject = ""
	if msg.focusSubject != "" {
		targetIdx := 0
		for i, r := range rows {
			if r.commit.Subject == msg.focusSubject {
				targetIdx = i
				break
			}
		}
		m.listPanel = m.listPanel.SetSelected(targetIdx)
		m.flashSubject = msg.focusSubject
		m.flashUntil = time.Now().Add(logFlashDuration)
		m.recomputeSearchMatches()
		m.jumpToCurrentMatch()
		return m, tea.Batch(append(refreshCmds, cmdFlashClear())...)
	}
	sel := m.listPanel.Selected()
	// On the initial load, start on the first commit (skip the pseudo-line).
	if isInitialLoad && len(rows) > 1 && rows[0].kind == rowPseudoStatus {
		sel = 1
	}
	m.listPanel = m.listPanel.SetSelected(sel)
	m.recomputeSearchMatches()
	m.jumpToCurrentMatch()
	return m, tea.Batch(refreshCmds...)
}

func (m *Model) jumpToTaggedCommit(step int) {
	rows := m.listPanel.Rows()
	if len(rows) == 0 || step == 0 {
		return
	}
	for i := m.listPanel.Selected() + step; i >= 0 && i < len(rows); i += step {
		if rowHasTag(rows[i]) {
			m.listPanel = m.listPanel.SetSelected(i)
			return
		}
	}
}

func rowHasTag(r row) bool {
	for _, decoration := range r.commit.Decorations {
		if decoration.Kind == git.RefDecorationTag {
			return true
		}
	}
	return false
}
