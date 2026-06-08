package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/imagediff"
	"github.com/elentok/gx/ui/reword"
)

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	// ctrl+c quits unconditionally even when a modal is open.
	if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Centralizes ADR 0010's lifecycle rule for in-panel disrupting events
	// (scroll, file selection, focus, header expand, resize, modal open/close):
	// if the overlay signature changed across this Update, mark the overlay dirty
	// and turn that into the eager-clear / debounced-replace command, regardless
	// of which code path produced the final model. Ref and screen-origin changes
	// are driven by the container via WithRef / WithScreenOrigin instead.
	before := m.overlaySignature()
	defer func() {
		model, ok := next.(Model)
		if !ok {
			return
		}
		if model.overlaySignature() != before {
			model.overlay.MarkDirty()
		}
		if !model.overlay.Dirty() {
			return
		}
		var disruptCmd tea.Cmd
		model.overlay, disruptCmd = model.overlay.Disrupt(model.settings.ImageDiffs)
		cmd = tea.Batch(cmd, disruptCmd)
		next = model
	}()

	// Delegate all messages to amend.Model while it's open.
	if m.amendConfirm.IsOpen {
		return m.handleAmendUpdate(msg)
	}
	// Delegate all messages to reword.Model while it's running.
	if m.reword.IsOpen {
		return m.handleRewordRunningUpdate(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case gotoPRMsg:
		return m.handleGotoPR(msg)
	case imagediff.SettleMsg:
		return m.handleImageDiffSettle(msg)
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)
	case editCommentFinishedMsg:
		return m.handleEditCommentFinished(msg)
	case reword.EditorFinishedMsg:
		return m.handleRewordEditorDone(msg.Err)
	}
	return m, nil
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true
	m.syncDiffViewport()
	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.help.IsOpen {
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	}
	if m.focusDiff {
		if len(m.keys.Prefix()) == 0 || m.diffModel.HasPendingChord() {
			updated, diffCmd, diffResult := m.diffModel.Update(msg)
			m.diffModel = updated
			if diffResult.Handled && !diffResult.ChordInProgress {
				m.keys.Reset()
				if diffResult.NeedsReload {
					m.refreshDiff()
					m.syncDiffViewport()
				}
				return m, diffCmd
			}
		}
	} else if !m.focusHeader && (len(m.keys.Prefix()) == 0 || m.fileTreeModel.HasPendingChord()) {
		var ftCmd tea.Cmd
		var ftResult filetree.Result
		m.fileTreeModel, ftCmd, ftResult = m.fileTreeModel.Update(msg)
		if ftResult.Handled {
			if ftResult.SearchQueryChanged {
				m.fileTreeModel.RecomputeSearchMatches(m.fileEntrySearchText)
			}
			if ftResult.SearchQueryChanged || ftResult.SearchCursorChanged {
				if m.fileTreeModel.FocusCurrentSearchMatch() {
					m.refreshDiff()
				}
			}
			if ftResult.OpenSelected {
				m.focusDiff = true
				m.ensureActiveVisible()
			}
			if ftResult.RebuildRequested {
				m.rebuildCommitFiletree()
				if m.fileTreeModel.Search().HasQuery() {
					m.fileTreeModel.RecomputeSearchMatches(m.fileEntrySearchText)
				}
			}
			if ftResult.SelectionChanged {
				m.refreshDiff()
			}
			return m, ftCmd
		}
	}
	match, consumed := m.keys.Process(msg)
	if match != nil {
		return m.dispatchBinding(match.ID)
	}
	if consumed {
		return m, nil
	}
	return m, nil
}
