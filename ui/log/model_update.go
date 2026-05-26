package log

import (
	"time"

	"github.com/elentok/gx/git"
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
		return m, childCmd
	case tea.FocusMsg:
		if m.rebaseDidStash {
			m.rebaseDidStash = false
			m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStashPop, yes: true}
		}
		return m, m.cmdReload()
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
	m.err = nil
	m.rows = msg.rows
	m.branchDiverged = msg.branchDiverged
	m.pendingFocusSubject = ""
	if msg.focusSubject != "" {
		m.list.SetSelected(0, len(m.rows))
		for i, r := range m.rows {
			if r.commit.Subject == msg.focusSubject {
				m.list.SetSelected(i, len(m.rows))
				break
			}
		}
		m.flashSubject = msg.focusSubject
		m.flashUntil = time.Now().Add(logFlashDuration)
		m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
		m.recomputeSearchMatches()
		m.jumpToCurrentMatch()
		return m, tea.Batch(append(refreshCmds, cmdFlashClear())...)
	}
	m.list.SetSelected(m.list.Selected(), len(m.rows))
	m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
	m.recomputeSearchMatches()
	m.jumpToCurrentMatch()
	return m, tea.Batch(refreshCmds...)
}

func (m *Model) jumpToTaggedCommit(step int) {
	if len(m.rows) == 0 || step == 0 {
		return
	}
	for i := m.list.Selected() + step; i >= 0 && i < len(m.rows); i += step {
		if rowHasTag(m.rows[i]) {
			m.list.SetSelected(i, len(m.rows))
			m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
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
