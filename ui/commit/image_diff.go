package commit

import (
	"github.com/elentok/gx/ui/imagediff"

	tea "charm.land/bubbletea/v2"
)

// overlaySignature is the set of commit-model fields that, when changed by an
// Update, can invalidate an active image-diff placement's position or content
// (ADR 0010's disrupting events). The Update wrapper compares it before/after to
// decide whether to mark the overlay dirty — this covers in-panel events
// (scroll, file selection, focus, header expand, resize) without instrumenting
// each handler. Ref and screen-origin changes are driven by the container and
// disrupt explicitly via WithRef / WithScreenOrigin.
type overlaySignature struct {
	path         string
	focusDiff    bool
	focusHeader  bool
	bodyExpanded bool
	scroll       int
	width        int
	height       int
	ready        bool
	modalOpen    bool
}

func (m Model) overlaySignature() overlaySignature {
	path := ""
	if file, ok := m.sidebar.selectedCommitFile(); ok {
		path = file.Path
	}
	return overlaySignature{
		path:         path,
		focusDiff:    m.focusDiff,
		focusHeader:  m.focusHeader,
		bodyExpanded: m.header.bodyExpanded,
		scroll:       m.diff.diffModel.Viewport().YOffset(),
		width:        m.width,
		height:       m.height,
		ready:        m.ready,
		modalOpen:    m.ModalOpen(),
	}
}

// handleImageDiffSettle delegates the settled-placement decision to the overlay
// controller, passing the commit Model itself as the SettleHost.
func (m Model) handleImageDiffSettle(msg imagediff.SettleMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.diff.overlay, cmd = m.diff.overlay.HandleSettle(msg, m)
	return m, cmd
}

// OnDeactivate clears any active image-diff placement. The container forwards
// its own deactivation (tab switch away) here so the overlay doesn't float over
// the next page (ADR 0010).
func (m Model) OnDeactivate() tea.Cmd {
	return m.diff.overlay.OnDeactivate()
}

// ModalOpen reports whether a centered modal currently occludes the diff panel.
// A kitty placement would paint over it at the graphics layer, so the overlay
// waits for the modal to close (ADR 0010).
func (m Model) ModalOpen() bool {
	return m.commitOps.IsOpen() || m.help.IsOpen
}

// SelectedImageFile implements imagediff.SettleHost: the selected file's path
// when (and only when) the detail panel is visible and that file is an
// image-diff candidate.
func (m Model) SelectedImageFile() (string, bool) {
	if !m.diff.screenVisible {
		return "", false
	}
	file, ok := m.sidebar.selectedCommitFile()
	if !ok || !m.diff.isImageDiffEligible(m.settings.ImageDiffs, file) {
		return "", false
	}
	return file.Path, true
}

// PanelGeometry implements imagediff.SettleHost: the diff body rect in absolute
// screen cells, computed relative to the panel's own (0,0) then offset by the
// injected screen origin (the panel never learns its origin from lipgloss.Join*).
func (m Model) PanelGeometry() (originCol, originRow, availCols, availRows int, ok bool) {
	if !m.diff.screenVisible {
		return 0, 0, 0, 0, false
	}
	relCol, relRow, w, h, ok := m.diffPaneBodyRect()
	if !ok {
		return 0, 0, 0, 0, false
	}
	return m.diff.screenCol + relCol, m.diff.screenRow + relRow, w, h, true
}

// FetchBlobs implements imagediff.SettleHost: the selected file's old/new image
// bytes, resolved against the current ref by the shared endpoint helper.
func (m Model) FetchBlobs() (old, newBytes []byte, oldOK, newOK bool) {
	file, ok := m.sidebar.selectedCommitFile()
	if !ok {
		return nil, nil, false, false
	}
	return m.diff.fetchImageDiffBlobs(m.ref, file)
}

// diffPaneBodyRect returns the diff pane's body area (inside its border) in
// cells relative to the commit panel's own top-left, mirroring contentView's
// layout exactly so the reserved blank lines and the overlay line up.
func (m Model) diffPaneBodyRect() (col, row, w, h int, ok bool) {
	if !m.ready || len(m.sidebar.fileTreeModel.Entries()) == 0 {
		return 0, 0, 0, 0, false
	}
	l := m.layout()

	var paneX, paneY, paneW, paneH int
	if l.narrow() {
		filesH, diffH := l.narrowHeights()
		paneX, paneY, paneW, paneH = 0, l.bodyH+filesH+1, l.width, diffH
	} else {
		paneX, paneY, paneW, paneH = l.filesW, l.bodyH, l.width-l.filesW, l.contentH
	}
	if paneW <= 2 || paneH <= 2 {
		return 0, 0, 0, 0, false
	}
	// The pane is rendered with 1-cell horizontal padding; the body starts
	// just inside it (see renderDiffPane / RenderPanel). PanelOptionsFor
	// always sets PaddingY: 0 (ui/panel.go), so RenderPanel emits no margin
	// row between header and body — the body starts right after the single
	// header row, hence +1 (see ADR 0013).
	return paneX + 1, paneY + 1, paneW - 2, paneH - 2, true
}
