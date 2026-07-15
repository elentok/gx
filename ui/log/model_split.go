package log

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/commit"

	tea "charm.land/bubbletea/v2"
)

// selectedPushState computes the push/pull state of the currently selected
// commit row, for display in the commit detail header.
func (m Model) selectedPushState() ui.PushState {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if cursor < 0 || cursor >= len(rows) || rows[cursor].kind != rowCommit {
		return ui.PushState{}
	}
	return ui.CommitPushState(rows[cursor].class, m.branchDiverged)
}

// withSyncedDetailSize resizes commitDetail to match the current split layout.
func (m Model) withSyncedDetailSize() Model {
	dw, dh := m.split.DetailSize()
	if dw > 0 && dh > 0 {
		updated, _ := m.commitDetail.Update(tea.WindowSizeMsg{Width: dw, Height: dh})
		m.commitDetail = updated.(commit.Model)
	}
	return m
}

// withSyncedListSize resizes listPanel to match the current split layout.
func (m Model) withSyncedListSize() Model {
	lw, lh := m.split.ListSize()
	if lw > 0 && lh > 0 {
		updated, _ := m.listPanel.Update(tea.WindowSizeMsg{Width: lw, Height: lh})
		m.listPanel = updated.(listPanel)
	}
	return m
}

// syncSplitSize informs the split container of the current window dimensions
// and resizes both panels accordingly.
func (m Model) syncSplitSize() (Model, tea.Cmd) {
	if m.width == 0 || m.height == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.split, cmd = m.split.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m = m.withSyncedDetailSize()
	m = m.withSyncedListSize()
	return m, cmd
}

// routeKeyToSplit forwards a key event to the split container and syncs the
// detail panel size afterwards. Returns the updated model and any commands.
func (m Model) routeKeyToSplit(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.split, cmd = m.split.Update(msg)
	m = m.withSyncedDetailSize()
	m = m.withSyncedListSize()
	return m, cmd
}

// handleSelectionChange updates the commit detail panel when the list cursor
// moves while in Split mode. prevRef is the ref before the navigation. The ref
// swap is an image-diff disrupting event (ADR 0010), so the WithRef disrupt
// command is returned for the caller to batch.
func (m Model) handleSelectionChange(prevRef string) (Model, tea.Cmd) {
	newRef := m.SelectedRef()
	m.split = m.split.WithListRef(newRef)
	if m.split.IsSplit() && newRef != prevRef && newRef != "" {
		var cmd tea.Cmd
		m.commitDetail, cmd = m.commitDetail.WithRef(newRef)
		m.commitDetail = m.commitDetail.WithPushState(m.selectedPushState())
		m = m.withSyncedDetailSize()
		return m, cmd
	}
	return m, nil
}

// OnPageDeactivated is called by the app shell when the user switches away from
// the log tab. It clears any active image-diff overlay in the detail panel so it
// doesn't float over the next tab (ADR 0010).
func (m Model) OnPageDeactivated() tea.Cmd {
	return m.commitDetail.OnDeactivate()
}

// withSyncedDetailOrigin pushes the detail panel's absolute screen origin (and
// visibility) into commitDetail so its image-diff kitty overlay lands where the
// panel is composed (ADR 0010). The detail is treated as not visible whenever a
// log-level modal is open, since a centered modal occludes it. WithScreenOrigin
// no-ops when nothing changed, so this is cheap to call after every Update.
func (m Model) withSyncedDetailOrigin() (Model, tea.Cmd) {
	col, row, visible := m.split.DetailOrigin()
	visible = visible && !m.ModalOpen() && !m.help.IsOpen && !m.rebaseConfirm.isOpen()
	var cmd tea.Cmd
	m.commitDetail, cmd = m.commitDetail.WithScreenOrigin(col, row, visible)
	return m, cmd
}

// listWidth returns the pixel width available to the log list panel, taking
// the split layout into account.
func (m Model) listWidth() int {
	if m.split.IsSplit() {
		lw, _ := m.split.ListSize()
		if lw > 0 {
			return lw
		}
	}
	return m.width
}

// listHeight returns the pixel height available to the log list panel.
func (m Model) listHeight() int {
	if m.split.IsSplit() {
		_, lh := m.split.ListSize()
		if lh > 0 {
			return lh
		}
	}
	return m.height
}
