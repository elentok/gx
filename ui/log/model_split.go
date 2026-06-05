package log

import (
	"github.com/elentok/gx/ui/commit"

	tea "charm.land/bubbletea/v2"
)

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
	return m, cmd
}

// handleSelectionChange updates the commit detail panel when the list cursor
// moves while in Split mode. prevRef is the ref before the navigation.
func (m Model) handleSelectionChange(prevRef string) Model {
	newRef := m.SelectedRef()
	m.split = m.split.WithListRef(newRef)
	if m.split.IsSplit() && newRef != prevRef && newRef != "" {
		m.commitDetail = m.commitDetail.WithRef(newRef)
		m = m.withSyncedDetailSize()
	}
	return m
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
