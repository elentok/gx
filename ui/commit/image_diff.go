package commit

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
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
	if file, ok := m.selectedCommitFile(); ok {
		path = file.Path
	}
	return overlaySignature{
		path:         path,
		focusDiff:    m.focusDiff,
		focusHeader:  m.focusHeader,
		bodyExpanded: m.bodyExpanded,
		scroll:       m.diffModel.Viewport().YOffset(),
		width:        m.width,
		height:       m.height,
		ready:        m.ready,
		modalOpen:    m.ModalOpen(),
	}
}

// isImageDiffEligible reports whether file is a candidate for inline image-diff
// rendering in the commit detail panel: extension allowlisted, config enabled,
// and the host terminal supports kitty graphics. Otherwise behavior is the plain
// "binary file" summary line.
func (m *Model) isImageDiffEligible(file git.CommitFile) bool {
	if !m.settings.ImageDiffs {
		return false
	}
	if !imagediff.HasImageExtension(file.Path) {
		return false
	}
	return m.overlay.Capability().Supported
}

// binaryDiffLines returns the lines the diff pane renders for a binary file.
// When the selected file is an image-diff candidate (and its last settle didn't
// fall back), it reserves bodyH blank lines so bubbletea's layout accounts for
// the overlay area — the kitty graphics are placed as a side effect, never
// embedded here (ADR 0010). Otherwise it is the plain "binary file" summary.
func (m Model) binaryDiffLines(bodyH, innerW int) []string {
	if file, ok := m.selectedCommitFile(); ok && m.isImageDiffEligible(file) && m.overlay.FallbackPath() != file.Path {
		blank := strings.Repeat(" ", innerW)
		lines := make([]string, 0, bodyH)
		for range bodyH {
			lines = append(lines, blank)
		}
		return lines
	}
	return []string{ui.StyleMuted.Render("binary file")}
}

// handleImageDiffSettle delegates the settled-placement decision to the overlay
// controller, passing the commit Model itself as the SettleHost.
func (m Model) handleImageDiffSettle(msg imagediff.SettleMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.overlay, cmd = m.overlay.HandleSettle(msg, m)
	return m, cmd
}

// OnDeactivate clears any active image-diff placement. The container forwards
// its own deactivation (tab switch away) here so the overlay doesn't float over
// the next page (ADR 0010).
func (m Model) OnDeactivate() tea.Cmd {
	return m.overlay.OnDeactivate()
}

// ModalOpen reports whether a centered modal currently occludes the diff panel.
// A kitty placement would paint over it at the graphics layer, so the overlay
// waits for the modal to close (ADR 0010).
func (m Model) ModalOpen() bool {
	return m.amendConfirm.IsOpen || m.reword.IsOpen || m.help.IsOpen
}

// SelectedImageFile implements imagediff.SettleHost: the selected file's path
// when (and only when) the detail panel is visible and that file is an
// image-diff candidate.
func (m Model) SelectedImageFile() (string, bool) {
	if !m.screenVisible {
		return "", false
	}
	file, ok := m.selectedCommitFile()
	if !ok || !m.isImageDiffEligible(file) {
		return "", false
	}
	return file.Path, true
}

// PanelGeometry implements imagediff.SettleHost: the diff body rect in absolute
// screen cells, computed relative to the panel's own (0,0) then offset by the
// injected screen origin (the panel never learns its origin from lipgloss.Join*).
func (m Model) PanelGeometry() (originCol, originRow, availCols, availRows int, ok bool) {
	if !m.screenVisible {
		return 0, 0, 0, 0, false
	}
	relCol, relRow, w, h, ok := m.diffPaneBodyRect()
	if !ok {
		return 0, 0, 0, 0, false
	}
	return m.screenCol + relCol, m.screenRow + relRow, w, h, true
}

// FetchBlobs implements imagediff.SettleHost: the selected file's old/new image
// bytes, resolved against the current ref by the shared endpoint helper.
func (m Model) FetchBlobs() (old, newBytes []byte, oldOK, newOK bool) {
	file, ok := m.selectedCommitFile()
	if !ok {
		return nil, nil, false, false
	}
	return m.fetchImageDiffBlobs(m.ref, file)
}

// diffPaneBodyRect returns the diff pane's body area (inside its border) in
// cells relative to the commit panel's own top-left, mirroring contentView's
// layout exactly so the reserved blank lines and the overlay line up.
func (m Model) diffPaneBodyRect() (col, row, w, h int, ok bool) {
	if !m.ready || len(m.fileTreeModel.Entries()) == 0 {
		return 0, 0, 0, 0, false
	}
	bodyH, contentH := m.layoutHeights()

	var paneX, paneY, paneW, paneH int
	if m.width < 90 {
		filesH, diffH := m.narrowPaneHeights(contentH)
		paneX, paneY, paneW, paneH = 0, bodyH+filesH+1, m.width, diffH
	} else {
		leftW := m.filesPaneWidth(contentH)
		paneX, paneY, paneW, paneH = leftW, bodyH, m.width-leftW, contentH
	}
	if paneW <= 2 || paneH <= 2 {
		return 0, 0, 0, 0, false
	}
	// The pane is rendered with 1-cell horizontal padding; the body starts
	// just inside it (see renderDiffPane / RenderPanel).
	//
	// TODO: paneY+1 assumes a 1-row offset into the pane, matching the old
	// bordered frame. The frame-free header renders as a header row plus a
	// 1-cell margin row before the body, so the real offset may need to be
	// +2 — unverified against a real kitty-graphics terminal (ADR 0013).
	return paneX + 1, paneY + 1, paneW - 2, paneH - 2, true
}
