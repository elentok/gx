package log

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/pushstate"
	"github.com/elentok/gx/ui/splitview"

	tea "charm.land/bubbletea/v2"
)

// splitPane owns the split container and the commit detail panel it hosts.
// The list panel stays on Model; methods that resize it take and return it.
type splitPane struct {
	split        splitview.Model
	commitDetail commit.Model
}

// selectedPushState computes the push/pull state of the currently selected
// commit row, for display in the commit detail header.
func (m Model) selectedPushState() pushstate.PushState {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if cursor < 0 || cursor >= len(rows) || rows[cursor].kind != rowCommit {
		return pushstate.PushState{}
	}
	return pushstate.CommitPushState(rows[cursor].class, m.branchDiverged)
}

// followSelection updates the detail panel after the list cursor moved from
// prevRef.
func (m Model) followSelection(prevRef string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.splitPane, cmd = m.handleSelectionChange(prevRef, m.SelectedRef(), m.selectedPushState())
	return m, cmd
}

// withSyncedDetailSize resizes commitDetail to match the current split layout.
func (p splitPane) withSyncedDetailSize() splitPane {
	dw, dh := p.split.DetailSize()
	if dw > 0 && dh > 0 {
		updated, _ := p.commitDetail.Update(tea.WindowSizeMsg{Width: dw, Height: dh})
		p.commitDetail = updated.(commit.Model)
	}
	return p
}

// withSyncedListSize resizes the list panel to match the current split layout.
func (p splitPane) withSyncedListSize(lp listPanel) listPanel {
	lw, lh := p.split.ListSize()
	if lw > 0 && lh > 0 {
		updated, _ := lp.Update(tea.WindowSizeMsg{Width: lw, Height: lh})
		lp = updated.(listPanel)
	}
	return lp
}

// syncSplitSize informs the split container of the current window dimensions
// and resizes both panels accordingly.
func (p splitPane) syncSplitSize(width, height int, lp listPanel) (splitPane, listPanel, tea.Cmd) {
	if width == 0 || height == 0 {
		return p, lp, nil
	}
	var cmd tea.Cmd
	p.split, cmd = p.split.Update(tea.WindowSizeMsg{Width: width, Height: height})
	p = p.withSyncedDetailSize()
	lp = p.withSyncedListSize(lp)
	return p, lp, cmd
}

// routeKeyToSplit forwards a key event to the split container and syncs the
// panel sizes afterwards.
func (p splitPane) routeKeyToSplit(msg tea.KeyPressMsg, lp listPanel) (splitPane, listPanel, tea.Cmd) {
	var cmd tea.Cmd
	p.split, cmd = p.split.Update(msg)
	p = p.withSyncedDetailSize()
	lp = p.withSyncedListSize(lp)
	return p, lp, cmd
}

// handleSelectionChange updates the commit detail panel when the list cursor
// moves while in Split mode. prevRef is the ref before the navigation. The ref
// swap is an image-diff disrupting event (ADR 0010), so the WithRef disrupt
// command is returned for the caller to batch.
func (p splitPane) handleSelectionChange(prevRef, newRef string, ps pushstate.PushState) (splitPane, tea.Cmd) {
	p.split = p.split.WithListRef(newRef)
	if p.split.IsSplit() && newRef != prevRef && newRef != "" {
		var cmd tea.Cmd
		p.commitDetail, cmd = p.commitDetail.WithRef(newRef)
		p.commitDetail = p.commitDetail.WithPushState(ps)
		p = p.withSyncedDetailSize()
		return p, cmd
	}
	return p, nil
}

// OnPageDeactivated is called by the app shell when the user switches away from
// the log tab. It clears any active image-diff overlay in the detail panel so it
// doesn't float over the next tab (ADR 0010).
func (p splitPane) OnPageDeactivated() tea.Cmd {
	return p.commitDetail.OnDeactivate()
}

// withSyncedDetailOrigin pushes the detail panel's absolute screen origin (and
// visibility) into commitDetail so its image-diff kitty overlay lands where the
// panel is composed (ADR 0010). occluded is true whenever a log-level modal is
// open, since a centered modal hides the detail. WithScreenOrigin no-ops when
// nothing changed, so this is cheap to call after every Update.
func (p splitPane) withSyncedDetailOrigin(occluded bool) (splitPane, tea.Cmd) {
	col, row, visible := p.split.DetailOrigin()
	var cmd tea.Cmd
	p.commitDetail, cmd = p.commitDetail.WithScreenOrigin(col, ui.ChromeRows+row, visible && !occluded)
	return p, cmd
}

// listWidth returns the width available to the log list panel, taking the
// split layout into account. fallback is the full window width.
func (p splitPane) listWidth(fallback int) int {
	if p.split.IsSplit() {
		lw, _ := p.split.ListSize()
		if lw > 0 {
			return lw
		}
	}
	return fallback
}

// listHeight returns the height available to the log list panel. fallback is
// the full window height.
func (p splitPane) listHeight(fallback int) int {
	if p.split.IsSplit() {
		_, lh := p.split.ListSize()
		if lh > 0 {
			return lh
		}
	}
	return fallback
}

// listActive reports whether the log list pane has focus inside a split.
func (p splitPane) listActive() bool {
	return p.split.IsSplit() && p.split.IsListFocused()
}
